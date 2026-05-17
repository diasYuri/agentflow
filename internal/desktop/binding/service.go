package binding

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/diasYuri/agentflow/internal/desktop/adapter"
)

// DesktopService expoe metodos ao frontend via Wails v3 bindings.
type DesktopService struct {
	ctx     context.Context
	adapter *adapter.Adapter
}

// NewDesktopService cria uma nova instancia do servico desktop.
func NewDesktopService(a *adapter.Adapter) *DesktopService {
	if a == nil {
		a = adapter.NewDefaultAdapter()
	}
	return &DesktopService{adapter: a}
}

// ServiceStartup e chamado pelo Wails na inicializacao do servico.
func (s *DesktopService) ServiceStartup(ctx context.Context, _ any) error {
	s.ctx = ctx
	return nil
}

// ServiceShutdown e chamado pelo Wails no encerramento do servico.
func (s *DesktopService) ServiceShutdown() error {
	s.adapter.Shutdown()
	return nil
}

// Health retorna um status basico para validar a comunicacao Go <-> UI.
func (s *DesktopService) Health() HealthResponse {
	return HealthResponse{
		Status:    "ok",
		Version:   "0.1.0",
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// HealthResponse representa o status de saude da aplicacao desktop.
type HealthResponse struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Timestamp string `json:"timestamp"`
}

// Greet e um metodo de exemplo que sera removido em planos futuros.
func (s *DesktopService) Greet(name string) string {
	if name == "" {
		name = "World"
	}
	return fmt.Sprintf("Hello, %s!", name)
}

// ListWorkflows lista workflows disponiveis.
func (s *DesktopService) ListWorkflows() ([]adapter.WorkflowSummary, error) {
	return s.adapter.ListWorkflows()
}

// LoadWorkflow carrega um workflow por path.
func (s *DesktopService) LoadWorkflow(path string) (adapter.LoadedWorkflow, error) {
	return s.adapter.LoadWorkflow(s.ctx, path)
}

// ValidateWorkflow valida um workflow.
func (s *DesktopService) ValidateWorkflow(path string) adapter.ValidationResult {
	return s.adapter.ValidateWorkflow(s.ctx, path)
}

// GenerateGraph gera o grafo Mermaid de um workflow.
func (s *DesktopService) GenerateGraph(path string) adapter.GraphResult {
	return s.adapter.GenerateGraph(s.ctx, path)
}

// DryRunWorkflow executa dry-run de um workflow.
func (s *DesktopService) DryRunWorkflow(path string, inputs map[string]any, vars map[string]any, maxConcurrency int, workingDir string) adapter.DryRunResult {
	return s.adapter.DryRunWorkflow(s.ctx, path, inputs, vars, maxConcurrency, workingDir)
}

// ResolveInput resolve inputs contra as definicoes do workflow.
func (s *DesktopService) ResolveInput(path string, inputs map[string]any) (map[string]any, error) {
	return s.adapter.ResolveInput(s.ctx, path, inputs)
}

// SaveWorkflow salva um arquivo de workflow.
func (s *DesktopService) SaveWorkflow(path string, content string) error {
	return s.adapter.SaveWorkflow(path, content)
}

// SaveInput salva um arquivo de input.
func (s *DesktopService) SaveInput(path string, content string) error {
	return s.adapter.SaveInput(path, content)
}

// OpenPath abre um path no gerenciador de arquivos do SO.
func (s *DesktopService) OpenPath(path string) error {
	return s.adapter.OpenPath(path)
}

// GetAppSettings retorna as configuracoes locais.
func (s *DesktopService) GetAppSettings() (adapter.AppSettings, error) {
	return s.adapter.GetAppSettings()
}

// UpdateAppSettings atualiza as configuracoes locais.
func (s *DesktopService) UpdateAppSettings(settings adapter.AppSettings) error {
	return s.adapter.UpdateAppSettings(settings)
}

// ListProjects lista os projetos configurados.
func (s *DesktopService) ListProjects() ([]adapter.ProjectSummary, error) {
	return s.adapter.ListProjects()
}

// AddProject adiciona um projeto local.
func (s *DesktopService) AddProject(name string, path string) error {
	return s.adapter.AddProject(name, path)
}

// RemoveProject remove um projeto local.
func (s *DesktopService) RemoveProject(name string) error {
	return s.adapter.RemoveProject(name)
}

// RunWorkflow inicia uma execucao de workflow.
func (s *DesktopService) RunWorkflow(req adapter.RunWorkflowRequest) (adapter.RunSummary, error) {
	return s.adapter.RunWorkflow(s.ctx, req)
}

// CancelRun cancela uma execucao ativa.
func (s *DesktopService) CancelRun(runID string) (adapter.RunSummary, error) {
	return s.adapter.CancelRun(runID)
}

// ListRuns lista execucoes ativas e persistidas.
func (s *DesktopService) ListRuns() (adapter.ListRunsResponse, error) {
	return s.adapter.ListRuns()
}

// GetRun retorna uma execucao especifica.
func (s *DesktopService) GetRun(runID string) (adapter.RunSummary, error) {
	return s.adapter.GetRun(runID)
}

// GetRunEvents retorna eventos paginados de uma run.
func (s *DesktopService) GetRunEvents(runID string, cursor int, limit int) (adapter.EventsResponse, error) {
	return s.adapter.GetRunEvents(runID, cursor, limit)
}

// GetRunArtifacts lista artefatos de uma run.
func (s *DesktopService) GetRunArtifacts(runID string) (adapter.ArtifactsResponse, error) {
	return s.adapter.GetRunArtifacts(runID)
}

// GetRunArtifact retorna conteudo de um artefato.
func (s *DesktopService) GetRunArtifact(runID, artifactID string) (adapter.ArtifactResponse, error) {
	return s.adapter.GetRunArtifact(runID, artifactID)
}

// GetRunArtifactPath resolve o path absoluto de um artefato indexado para open/export controlado.
func (s *DesktopService) GetRunArtifactPath(runID, artifactID string) (string, error) {
	return s.adapter.GetRunArtifactPath(runID, artifactID)
}

// GetRunNodes lista resultados de nos.
func (s *DesktopService) GetRunNodes(runID string) (adapter.NodesResponse, error) {
	return s.adapter.GetRunNodes(runID)
}

// GetRunNode retorna instancias de um no especifico.
func (s *DesktopService) GetRunNode(runID, nodeID string) (adapter.NodeResponse, error) {
	return s.adapter.GetRunNode(runID, nodeID)
}

// GetRunPlan retorna metadados e plano de uma run.
func (s *DesktopService) GetRunPlan(runID string) (adapter.PlanResponse, error) {
	return s.adapter.GetRunPlan(runID)
}

// GetRunLogs retorna linhas de log (events.jsonl).
func (s *DesktopService) GetRunLogs(runID string) (adapter.LogsResponse, error) {
	return s.adapter.GetRunLogs(runID)
}
