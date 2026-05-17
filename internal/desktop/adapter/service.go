package adapter

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/runtime/handlers"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/desktop/runtime"
)

// Adapter normaliza DTOs e erros para um contrato estavel entre UI e core.
type Adapter struct {
	workflows ports.WorkflowRepository
	ucFactory func() (*runworkflow.RunWorkflowUseCase, error)
	store     SettingsStore
	projects  *app.ProjectRegistry
	fs        FileSystem
	runtime   *runtime.Manager
	recentMu  sync.Mutex
}

// NewAdapter cria um adapter com dependencias explicitas.
func NewAdapter(
	workflows ports.WorkflowRepository,
	ucFactory func() (*runworkflow.RunWorkflowUseCase, error),
	store SettingsStore,
	projects *app.ProjectRegistry,
	fs FileSystem,
	rt *runtime.Manager,
) *Adapter {
	if fs == nil {
		fs = osFS{}
	}
	return &Adapter{
		workflows: workflows,
		ucFactory: ucFactory,
		store:     store,
		projects:  projects,
		fs:        fs,
		runtime:   rt,
	}
}

// NewDefaultAdapter cria um adapter com defaults do projeto.
func NewDefaultAdapter() *Adapter {
	workflows := yamlrepo.NewWorkflowRepository()
	store := NewJSONSettingsStore(DefaultSettingsPath())
	projects := app.NewProjectRegistry(app.NewJSONProjectStore(app.DefaultProjectsPath()))
	ucFactory := func() (*runworkflow.RunWorkflowUseCase, error) {
		settings, err := store.Load()
		if err != nil {
			return nil, err
		}
		return app.NewRunWorkflowUseCase(runtimeOptionsFromSettings(settings))
	}
	settings, _ := store.Load()
	rt := runtime.NewManager("", settings.CodexPath, settings.ClaudePath, settings.PiPath, settings.LogFormat)
	return NewAdapter(workflows, ucFactory, store, projects, nil, rt)
}

// LoadWorkflow le um workflow por path e retorna metadados e conteudo bruto.
func (a *Adapter) LoadWorkflow(ctx context.Context, path string) (LoadedWorkflow, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return LoadedWorkflow{}, DesktopError{Message: "workflow path is required", Code: ErrCodeInvalidPath}
	}

	data, err := a.fs.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return LoadedWorkflow{}, DesktopError{Message: fmt.Sprintf("file not found: %s", path), Code: ErrCodeWorkflowNotFound}
		}
		return LoadedWorkflow{}, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: path}
	}

	spec, sourcePath, err := a.workflows.Load(ctx, path)
	if err != nil {
		return LoadedWorkflow{}, DesktopError{Message: err.Error(), Code: ErrCodeYAMLError, Context: path}
	}

	result := LoadedWorkflow{
		Name:        spec.Name,
		Description: spec.Description,
		Version:     spec.Version,
		SourcePath:  sourcePath,
		RawYAML:     string(data),
		Inputs:      make(map[string]InputDocument, len(spec.Inputs)),
		Nodes:       make([]NodeSummary, 0, len(spec.Nodes)),
	}

	for name, input := range spec.Inputs {
		result.Inputs[name] = InputDocument{
			Name:     name,
			Type:     input.Type,
			Required: input.Required,
			Default:  input.Default,
		}
	}

	for _, node := range spec.Nodes {
		result.Nodes = append(result.Nodes, NodeSummary{
			ID:        node.ID,
			Kind:      string(node.Kind),
			DependsOn: append([]string(nil), node.DependsOn...),
		})
	}

	a.addRecent(path)

	return result, nil
}

// ValidateWorkflow valida um workflow e retorna resultado normalizado.
func (a *Adapter) ValidateWorkflow(ctx context.Context, path string) ValidationResult {
	uc, err := a.ucFactory()
	if err != nil {
		return ValidationResult{Valid: false, Errors: []DesktopError{{Message: err.Error(), Code: ErrCodeInternalError}}}
	}

	plan, err := uc.Validate(ctx, path)
	if err != nil {
		return ValidationResult{
			Valid:  false,
			Name:   filepath.Base(path),
			Errors: []DesktopError{normalizeError(err)},
		}
	}

	return ValidationResult{
		Valid:     true,
		Name:      plan.Workflow.Name,
		NodeCount: len(plan.Order),
	}
}

// GenerateGraph valida o workflow e retorna o grafo Mermaid.
func (a *Adapter) GenerateGraph(ctx context.Context, path string) GraphResult {
	uc, err := a.ucFactory()
	if err != nil {
		return GraphResult{Valid: false, Errors: []DesktopError{{Message: err.Error(), Code: ErrCodeInternalError}}}
	}

	plan, err := uc.Validate(ctx, path)
	if err != nil {
		return GraphResult{Valid: false, Errors: []DesktopError{normalizeError(err)}}
	}

	var buf bytes.Buffer
	if err := workflow.WriteMermaidGraph(&buf, plan); err != nil {
		return GraphResult{Valid: false, Errors: []DesktopError{{Message: err.Error(), Code: ErrCodeInternalError}}}
	}

	return GraphResult{
		Mermaid: buf.String(),
		Valid:   true,
	}
}

// ResolveInput resolve os inputs fornecidos contra as definicoes do workflow.
func (a *Adapter) ResolveInput(ctx context.Context, path string, inputs map[string]any) (map[string]any, error) {
	spec, _, err := a.workflows.Load(ctx, path)
	if err != nil {
		return nil, DesktopError{Message: err.Error(), Code: ErrCodeYAMLError, Context: path}
	}

	resolved, err := handlers.ResolveInputs(*spec, inputs)
	if err != nil {
		return nil, DesktopError{Message: err.Error(), Code: ErrCodeInvalidInput, Context: path}
	}

	if err := workflow.ValidateInputValues(*spec, resolved); err != nil {
		return nil, DesktopError{Message: err.Error(), Code: ErrCodeInvalidInput, Context: path}
	}

	return resolved, nil
}

// DryRunWorkflow executa um dry-run e retorna o plano normalizado.
func (a *Adapter) DryRunWorkflow(ctx context.Context, path string, inputs map[string]any, vars map[string]any, maxConcurrency int, workingDir string) DryRunResult {
	uc, err := a.ucFactory()
	if err != nil {
		return DryRunResult{Valid: false, Errors: []DesktopError{{Message: err.Error(), Code: ErrCodeInternalError}}}
	}

	plan, resolved, err := uc.DryRun(ctx, runworkflow.RunOptions{
		WorkflowRef:    path,
		Inputs:         inputs,
		Vars:           vars,
		MaxConcurrency: maxConcurrency,
		WorkingDir:     workingDir,
	})
	if err != nil {
		return DryRunResult{Valid: false, Errors: []DesktopError{normalizeError(err)}}
	}

	nodes := make(map[string]NodePlan, len(plan.Nodes))
	for id, node := range plan.Nodes {
		nodes[id] = NodePlan{
			ID:           id,
			Dependencies: append([]string(nil), node.Dependencies...),
			Dependents:   append([]string(nil), node.Dependents...),
			Kind:         string(node.Spec.Kind),
		}
	}

	return DryRunResult{
		Workflow: plan.Workflow.Name,
		Inputs:   resolved,
		Order:    plan.Order,
		Nodes:    nodes,
		Valid:    true,
	}
}

// SaveWorkflow grava um arquivo de workflow com criacao segura de diretorios.
func (a *Adapter) SaveWorkflow(path string, content string) error {
	if path == "" {
		return DesktopError{Message: "path is required", Code: ErrCodeInvalidPath}
	}

	dir := filepath.Dir(path)
	if err := a.fs.MkdirAll(dir, 0o755); err != nil {
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: dir}
	}

	if err := a.fs.WriteFile(path, []byte(content), 0o644); err != nil {
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: path}
	}

	a.addRecent(path)
	return nil
}

// SaveInput grava um arquivo de input com criacao segura de diretorios.
func (a *Adapter) SaveInput(path string, content string) error {
	if path == "" {
		return DesktopError{Message: "path is required", Code: ErrCodeInvalidPath}
	}

	dir := filepath.Dir(path)
	if err := a.fs.MkdirAll(dir, 0o755); err != nil {
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: dir}
	}

	if err := a.fs.WriteFile(path, []byte(content), 0o644); err != nil {
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: path}
	}

	return nil
}

// GetAppSettings retorna as configuracoes locais.
func (a *Adapter) GetAppSettings() (AppSettings, error) {
	return a.store.Load()
}

// UpdateAppSettings atualiza e persiste as configuracoes locais.
func (a *Adapter) UpdateAppSettings(settings AppSettings) error {
	if err := a.store.Save(settings); err != nil {
		return err
	}
	if a.runtime != nil {
		a.runtime.Configure(settings.CodexPath, settings.ClaudePath, settings.PiPath, settings.LogFormat)
	}
	return nil
}

// ListProjects retorna os projetos configurados localmente.
func (a *Adapter) ListProjects() ([]ProjectSummary, error) {
	if a.projects == nil {
		return nil, DesktopError{Message: "project registry not configured", Code: ErrCodeInternalError}
	}
	projects, err := a.projects.List()
	if err != nil {
		return nil, normalizeError(err)
	}
	result := make([]ProjectSummary, len(projects))
	for i, project := range projects {
		result[i] = ProjectSummary{
			Name: project.Name,
			Path: project.Path,
		}
	}
	return result, nil
}

// AddProject adiciona um projeto ao registry local.
func (a *Adapter) AddProject(name, path string) error {
	if a.projects == nil {
		return DesktopError{Message: "project registry not configured", Code: ErrCodeInternalError}
	}
	if err := a.projects.Add(name, path); err != nil {
		return normalizeError(err)
	}
	return nil
}

// RemoveProject remove um projeto do registry local.
func (a *Adapter) RemoveProject(name string) error {
	if a.projects == nil {
		return DesktopError{Message: "project registry not configured", Code: ErrCodeInternalError}
	}
	if err := a.projects.Remove(name); err != nil {
		return normalizeError(err)
	}
	return nil
}

// ListWorkflows lista workflows disponiveis nos diretorios padrao.
func (a *Adapter) ListWorkflows() ([]WorkflowSummary, error) {
	localRoot, globalRoot := workflowRoots()

	var result []WorkflowSummary
	seen := make(map[string]struct{})

	appendWorkflows := func(root string) error {
		entries, err := a.fs.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			path := filepath.Join(root, entry.Name())
			name := entry.Name()
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			data, err := a.fs.ReadFile(path)
			if err != nil {
				result = append(result, WorkflowSummary{Name: name, Path: path})
				continue
			}

			var meta struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal(data, &meta); err != nil {
				result = append(result, WorkflowSummary{Name: name, Path: path})
				continue
			}

			displayName := meta.Name
			if displayName == "" {
				displayName = name
			}
			result = append(result, WorkflowSummary{
				Name:        displayName,
				Description: meta.Description,
				Path:        path,
			})
		}
		return nil
	}

	if err := appendWorkflows(localRoot); err != nil {
		return nil, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: localRoot}
	}
	if err := appendWorkflows(globalRoot); err != nil {
		return nil, DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: globalRoot}
	}

	return result, nil
}

// OpenPath abre um path no gerenciador de arquivos do SO.
func (a *Adapter) OpenPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return DesktopError{Message: "path is required", Code: ErrCodeInvalidPath}
	}
	clean := filepath.Clean(path)
	if _, err := os.Stat(clean); err != nil {
		if os.IsNotExist(err) {
			return DesktopError{Message: fmt.Sprintf("path not found: %s", clean), Code: ErrCodeInvalidPath}
		}
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: clean}
	}

	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", clean)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", clean)
	default:
		cmd = exec.Command("xdg-open", clean)
	}
	if err := cmd.Start(); err != nil {
		return DesktopError{Message: err.Error(), Code: ErrCodeFileSystem, Context: clean}
	}
	return nil
}

func runtimeOptionsFromSettings(settings AppSettings) app.RuntimeOptions {
	return app.RuntimeOptions{
		CodexPath:  settings.CodexPath,
		ClaudePath: settings.ClaudePath,
		PiPath:     settings.PiPath,
		LogFormat:  settings.LogFormat,
	}
}

func (a *Adapter) addRecent(path string) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()

	settings, err := a.store.Load()
	if err != nil {
		return
	}

	var filtered []string
	for _, r := range settings.RecentFiles {
		if r != path {
			filtered = append(filtered, r)
		}
	}

	settings.RecentFiles = append([]string{path}, filtered...)
	if len(settings.RecentFiles) > 20 {
		settings.RecentFiles = settings.RecentFiles[:20]
	}

	_ = a.store.Save(settings)
}

var (
	osGetwd       = os.Getwd
	osUserHomeDir = os.UserHomeDir
)

// Shutdown encerra runs ativas no runtime desktop.
func (a *Adapter) Shutdown() {
	if a.runtime != nil {
		a.runtime.Shutdown()
	}
}

func workflowRoots() (string, string) {
	localRoot := filepath.Join(".agentflow", "workflows")
	if cwd, err := osGetwd(); err == nil {
		localRoot = filepath.Join(cwd, localRoot)
	}

	globalRoot := filepath.Join(".agentflow", "workflows")
	if home, err := osUserHomeDir(); err == nil {
		globalRoot = filepath.Join(home, ".agentflow", "workflows")
	}
	return filepath.Clean(localRoot), filepath.Clean(globalRoot)
}
