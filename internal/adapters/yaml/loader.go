package yaml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type rawWorkflowSpec struct {
	Version          string                         `yaml:"version"`
	Name             string                         `yaml:"name"`
	Description      string                         `yaml:"description"`
	Inputs           map[string]workflow.InputSpec  `yaml:"inputs"`
	Vars             map[string]any                 `yaml:"vars"`
	Secrets          map[string]workflow.SecretSpec `yaml:"secrets"`
	Defaults         workflow.DefaultsSpec          `yaml:"defaults"`
	Execution        workflow.ExecutionSpec         `yaml:"execution"`
	Nodes            []workflow.NodeSpec            `yaml:"nodes"`
	Worktree         yaml.Node                      `yaml:"worktree"`
	WorktreeProvider string                         `yaml:"worktree-provider"`

	// V2 fields (modeled, not yet executed)
	Imports []workflow.ImportSpec                `yaml:"imports"`
	Outputs map[string]workflow.OutputSpec       `yaml:"outputs"`
	Hooks   []workflow.HookSpec                  `yaml:"hooks"`
	Steps   map[string]workflow.ReusableStepSpec `yaml:"steps"`
}

type WorkflowRepository struct {
	localRoot  string
	globalRoot string
}

func NewWorkflowRepository(roots ...string) *WorkflowRepository {
	localRoot, globalRoot := defaultWorkflowRoots()
	if len(roots) > 0 && strings.TrimSpace(roots[0]) != "" {
		localRoot = roots[0]
	}
	if len(roots) > 1 && strings.TrimSpace(roots[1]) != "" {
		globalRoot = roots[1]
	}
	return &WorkflowRepository{localRoot: localRoot, globalRoot: globalRoot}
}

func (r *WorkflowRepository) Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error) {
	_ = ctx
	name := strings.TrimSpace(ref)
	if name == "" {
		return nil, "", fmt.Errorf("workflow name is required")
	}
	if isWorkflowPath(name) {
		spec, err := decodeWorkflow(name)
		if err != nil {
			return nil, "", err
		}
		return spec, filepath.Clean(name), nil
	}

	localSpec, localPath, localErrs, err := r.findInScope(ctx, r.localRoot, name)
	if err != nil {
		return nil, "", fmt.Errorf("local workflows: %w", err)
	}
	if localSpec != nil {
		return localSpec, localPath, nil
	}

	globalSpec, globalPath, globalErrs, err := r.findInScope(ctx, r.globalRoot, name)
	if err != nil {
		return nil, "", fmt.Errorf("global workflows: %w", err)
	}
	if globalSpec != nil {
		return globalSpec, globalPath, nil
	}

	var scanErrs []error
	scanErrs = append(scanErrs, localErrs...)
	scanErrs = append(scanErrs, globalErrs...)
	if len(scanErrs) > 0 {
		return nil, "", fmt.Errorf("workflow %q not found: %w", name, errors.Join(scanErrs...))
	}
	return nil, "", fmt.Errorf("workflow %q not found in %q or %q", name, r.localRoot, r.globalRoot)
}

func (r *WorkflowRepository) findInScope(ctx context.Context, root string, name string) (*workflow.WorkflowSpec, string, []error, error) {
	if strings.TrimSpace(root) == "" {
		return nil, "", nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil, nil
		}
		return nil, "", []error{fmt.Errorf("%s: %w", root, err)}, nil
	}

	var (
		matches  []scopeMatch
		scanErrs []error
	)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, "", scanErrs, err
		}
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".yaml", ".yml", ".json":
		default:
			continue
		}
		spec, decodeErr := decodeWorkflow(path)
		if decodeErr != nil {
			scanErrs = append(scanErrs, fmt.Errorf("%s: %w", path, decodeErr))
			continue
		}
		if spec.Name != name {
			continue
		}
		matches = append(matches, scopeMatch{spec: spec, path: path})
	}

	if len(matches) > 1 {
		paths := make([]string, 0, len(matches))
		for _, match := range matches {
			paths = append(paths, match.path)
		}
		return nil, "", scanErrs, fmt.Errorf("duplicate workflow name %q in %q: %s", name, root, strings.Join(paths, ", "))
	}
	if len(matches) == 1 {
		return matches[0].spec, matches[0].path, scanErrs, nil
	}
	return nil, "", scanErrs, nil
}

type scopeMatch struct {
	spec *workflow.WorkflowSpec
	path string
}

func isWorkflowPath(ref string) bool {
	ext := strings.ToLower(filepath.Ext(ref))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return false
	}
	if strings.Contains(ref, string(filepath.Separator)) || filepath.IsAbs(ref) {
		return true
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	return false
}

// DecodeWorkflow loads and decodes a workflow YAML file without resolving imports or expanding macros.
func DecodeWorkflow(path string) (*workflow.WorkflowSpec, error) {
	return decodeWorkflowWithImports(path, nil)
}

func decodeWorkflow(path string) (*workflow.WorkflowSpec, error) {
	return decodeWorkflowWithImports(path, nil)
}

func decodeWorkflowWithImports(path string, stack []string) (*workflow.WorkflowSpec, error) {
	cleanPath := filepath.Clean(path)
	for _, s := range stack {
		if s == cleanPath {
			return nil, fmt.Errorf("import cycle: %s", formatCyclePath(stack, cleanPath))
		}
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	var raw rawWorkflowSpec
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode workflow yaml: %w", err)
	}

	spec := workflow.WorkflowSpec{
		Version:     raw.Version,
		Name:        raw.Name,
		Description: raw.Description,
		Inputs:      raw.Inputs,
		Vars:        raw.Vars,
		Secrets:     raw.Secrets,
		Defaults:    raw.Defaults,
		Execution:   raw.Execution,
		Nodes:       raw.Nodes,
		Imports:     raw.Imports,
		Outputs:     raw.Outputs,
		Hooks:       raw.Hooks,
		Steps:       raw.Steps,
	}

	if err := normalizeWorktree(&spec, raw.Worktree, raw.WorktreeProvider); err != nil {
		return nil, fmt.Errorf("decode workflow yaml: %w", err)
	}

	if spec.Version == workflow.WorkflowVersion1 && len(spec.Imports) > 0 {
		return nil, fmt.Errorf("imports are not supported in workflow version %q", spec.Version)
	}

	if spec.Version == workflow.WorkflowVersion2 && len(spec.Imports) > 0 {
		baseDir := filepath.Dir(cleanPath)
		imported, err := resolveImports(baseDir, spec.Imports, append(stack, cleanPath))
		if err != nil {
			return nil, err
		}
		merged, err := mergeSpecs(imported, spec)
		if err != nil {
			return nil, err
		}
		spec = merged
	}

	if spec.Version == workflow.WorkflowVersion2 {
		if err := workflow.ExpandMacros(&spec); err != nil {
			return nil, fmt.Errorf("expand macros: %w", err)
		}
	}

	if spec.Inputs == nil {
		spec.Inputs = map[string]workflow.InputSpec{}
	}
	if spec.Vars == nil {
		spec.Vars = map[string]any{}
	}
	return &spec, nil
}

func formatCyclePath(stack []string, current string) string {
	path := append([]string(nil), stack...)
	path = append(path, current)
	out := path[0]
	for _, next := range path[1:] {
		out += " -> " + next
	}
	return out
}

func resolveImports(baseDir string, imports []workflow.ImportSpec, stack []string) (workflow.WorkflowSpec, error) {
	var result workflow.WorkflowSpec
	for _, imp := range imports {
		if strings.TrimSpace(imp.Path) == "" {
			return workflow.WorkflowSpec{}, fmt.Errorf("import path is required")
		}
		impPath := filepath.Clean(filepath.Join(baseDir, imp.Path))
		imported, err := decodeWorkflowWithImports(impPath, stack)
		if err != nil {
			return workflow.WorkflowSpec{}, fmt.Errorf("import %q: %w", imp.Path, err)
		}
		merged, err := mergeSpecs(result, *imported)
		if err != nil {
			return workflow.WorkflowSpec{}, fmt.Errorf("import %q: %w", imp.Path, err)
		}
		result = merged
	}
	return result, nil
}

func mergeSpecs(base workflow.WorkflowSpec, overlay workflow.WorkflowSpec) (workflow.WorkflowSpec, error) {
	mergedInputs := make(map[string]workflow.InputSpec, len(base.Inputs)+len(overlay.Inputs))
	for k, v := range base.Inputs {
		mergedInputs[k] = v
	}
	for k, v := range overlay.Inputs {
		mergedInputs[k] = v
	}

	mergedVars := make(map[string]any, len(base.Vars)+len(overlay.Vars))
	for k, v := range base.Vars {
		mergedVars[k] = v
	}
	for k, v := range overlay.Vars {
		mergedVars[k] = v
	}

	mergedSecrets := make(map[string]workflow.SecretSpec, len(base.Secrets)+len(overlay.Secrets))
	for k, v := range base.Secrets {
		mergedSecrets[k] = v
	}
	for k, v := range overlay.Secrets {
		mergedSecrets[k] = v
	}

	mergedDefaults := base.Defaults
	if overlay.Defaults.Timeout != 0 {
		mergedDefaults.Timeout = overlay.Defaults.Timeout
	}
	if overlay.Defaults.Retries != 0 {
		mergedDefaults.Retries = overlay.Defaults.Retries
	}
	if overlay.Defaults.WorkingDir != "" {
		mergedDefaults.WorkingDir = overlay.Defaults.WorkingDir
	}

	mergedExecution := base.Execution
	if overlay.Execution.MaxConcurrency != 0 {
		mergedExecution.MaxConcurrency = overlay.Execution.MaxConcurrency
	}
	if overlay.Execution.FailFast != nil {
		mergedExecution.FailFast = overlay.Execution.FailFast
	}
	if overlay.Execution.PauseWhenFail {
		mergedExecution.PauseWhenFail = overlay.Execution.PauseWhenFail
	}
	if overlay.Execution.OutputDir != "" {
		mergedExecution.OutputDir = overlay.Execution.OutputDir
	}
	if overlay.Execution.MaxNodeOutputBytes != 0 {
		mergedExecution.MaxNodeOutputBytes = overlay.Execution.MaxNodeOutputBytes
	}

	mergedWorktree := base.Worktree
	if overlay.Worktree.Enabled {
		mergedWorktree = overlay.Worktree
	}

	mergedOutputs := make(map[string]workflow.OutputSpec, len(base.Outputs)+len(overlay.Outputs))
	for k, v := range base.Outputs {
		mergedOutputs[k] = v
	}
	for k, v := range overlay.Outputs {
		mergedOutputs[k] = v
	}

	mergedHooks := append([]workflow.HookSpec(nil), base.Hooks...)
	mergedHooks = append(mergedHooks, overlay.Hooks...)

	mergedSteps := make(map[string]workflow.ReusableStepSpec, len(base.Steps)+len(overlay.Steps))
	for k, v := range base.Steps {
		mergedSteps[k] = v
	}
	for k, v := range overlay.Steps {
		if _, exists := mergedSteps[k]; exists {
			return workflow.WorkflowSpec{}, fmt.Errorf("step name conflict: %q", k)
		}
		mergedSteps[k] = v
	}

	seen := make(map[string]struct{}, len(base.Nodes)+len(overlay.Nodes))
	for _, n := range base.Nodes {
		seen[n.ID] = struct{}{}
	}
	for _, n := range overlay.Nodes {
		if _, ok := seen[n.ID]; ok {
			return workflow.WorkflowSpec{}, fmt.Errorf("node id conflict after merge: %q", n.ID)
		}
	}
	mergedNodes := append([]workflow.NodeSpec(nil), base.Nodes...)
	mergedNodes = append(mergedNodes, overlay.Nodes...)

	return workflow.WorkflowSpec{
		Version:     overlay.Version,
		Name:        overlay.Name,
		Description: overlay.Description,
		Inputs:      mergedInputs,
		Vars:        mergedVars,
		Secrets:     mergedSecrets,
		Defaults:    mergedDefaults,
		Execution:   mergedExecution,
		Nodes:       mergedNodes,
		Worktree:    mergedWorktree,
		Outputs:     mergedOutputs,
		Hooks:       mergedHooks,
		Steps:       mergedSteps,
	}, nil
}

func normalizeWorktree(spec *workflow.WorkflowSpec, worktreeNode yaml.Node, worktreeProvider string) error {
	// No worktree field at all
	if worktreeNode.IsZero() && worktreeProvider == "" {
		return nil
	}

	var structured workflow.WorktreeSpec
	var hasStructured bool

	if !worktreeNode.IsZero() {
		switch worktreeNode.Kind {
		case yaml.ScalarNode:
			var enabled bool
			if err := worktreeNode.Decode(&enabled); err != nil {
				return fmt.Errorf("worktree: %w", err)
			}
			structured.Enabled = enabled
			hasStructured = true
		case yaml.MappingNode:
			if err := worktreeNode.Decode(&structured); err != nil {
				return fmt.Errorf("worktree: %w", err)
			}
			hasStructured = true
		default:
			return fmt.Errorf("worktree: must be boolean or object")
		}
	}

	if worktreeProvider != "" {
		if hasStructured && structured.Provider != "" && structured.Provider != worktreeProvider {
			return fmt.Errorf("worktree.provider %q conflicts with worktree-provider %q", structured.Provider, worktreeProvider)
		}
		if !hasStructured {
			structured.Enabled = true
			hasStructured = true
		}
		structured.Provider = worktreeProvider
	}

	if hasStructured {
		spec.Worktree = structured
		workflow.ApplyWorktreeDefaults(spec)
	}
	return nil
}

func defaultWorkflowRoots() (string, string) {
	localRoot := filepath.Join(".agentflow", "workflows")
	if cwd, err := os.Getwd(); err == nil {
		localRoot = filepath.Join(cwd, localRoot)
	}

	globalRoot := filepath.Join(".agentflow", "workflows")
	if home, err := os.UserHomeDir(); err == nil {
		globalRoot = filepath.Join(home, ".agentflow", "workflows")
	}
	return filepath.Clean(localRoot), filepath.Clean(globalRoot)
}
