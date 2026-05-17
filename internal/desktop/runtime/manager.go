package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/diasYuri/agentflow/internal/app"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

// Manager gerencia runs ativos no processo desktop.
type Manager struct {
	runRoot    string
	codexPath  string
	claudePath string
	piPath     string
	logFormat  string
	ucFactory  func(runRoot, eventsJSONL string) (*runworkflow.RunWorkflowUseCase, error)

	mu     sync.Mutex
	active map[string]*activeRun
}

// activeRun representa uma execucao em andamento.
type activeRun struct {
	runID      string
	workflow   string
	status     corerun.RunStatus
	runDir     string
	startedAt  time.Time
	finishedAt time.Time
	err        string
	tag        string
	cancel     context.CancelFunc
	mu         sync.Mutex
}

// RunRequest parametriza o inicio de uma run.
type RunRequest struct {
	WorkflowRef    string
	Inputs         map[string]any
	Vars           map[string]any
	MaxConcurrency int
	WorkingDir     string
	Tag            string
}

// RunSummary resume uma execucao de workflow.
type RunSummary struct {
	ID             string
	Workflow       string
	Status         corerun.RunStatus
	RunDir         string
	StartedAt      time.Time
	FinishedAt     time.Time
	Error          string
	CurrentStep    string
	CompletedSteps []string
	PendingSteps   []string
	TotalSteps     int
	Tag            string
	DurationMS     int64
	FailedNodes    int
	Retries        int
	AgentCalls     int
	BashCalls      int
	ArtifactCount  int
	NodeCount      int
	FirstError     string
	SlowestNodes   []corerun.SlowestNode
	AgentUsage     []corerun.AgentUsage
}

// NewManager cria um gerenciador de runs desktop.
func NewManager(runRoot, codexPath, claudePath, piPath, logFormat string) *Manager {
	if runRoot == "" {
		runRoot = defaultRunRoot()
	}
	m := &Manager{
		runRoot:    runRoot,
		codexPath:  codexPath,
		claudePath: claudePath,
		piPath:     piPath,
		logFormat:  logFormat,
		active:     make(map[string]*activeRun),
	}
	m.ucFactory = m.newUseCase
	return m
}

// NewManagerWithFactory permite injetar uma factory de use-case para testes.
func NewManagerWithFactory(runRoot string, factory func(runRoot, eventsJSONL string) (*runworkflow.RunWorkflowUseCase, error)) *Manager {
	if runRoot == "" {
		runRoot = defaultRunRoot()
	}
	return &Manager{
		runRoot:   runRoot,
		ucFactory: factory,
		active:    make(map[string]*activeRun),
	}
}

// Configure atualiza preferencias usadas por novas execucoes.
func (m *Manager) Configure(codexPath, claudePath, piPath, logFormat string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codexPath = codexPath
	m.claudePath = claudePath
	m.piPath = piPath
	m.logFormat = logFormat
}

func (m *Manager) newUseCase(runRoot, eventsJSONL string) (*runworkflow.RunWorkflowUseCase, error) {
	m.mu.Lock()
	codexPath := m.codexPath
	claudePath := m.claudePath
	piPath := m.piPath
	logFormat := m.logFormat
	m.mu.Unlock()

	return app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   codexPath,
		ClaudePath:  claudePath,
		PiPath:      piPath,
		LogFormat:   logFormat,
		EventsJSONL: eventsJSONL,
		RunRoot:     runRoot,
	})
}

func defaultRunRoot() string {
	root := filepath.Join(".agentflow", "runs")
	if home, err := os.UserHomeDir(); err == nil {
		root = filepath.Join(home, ".agentflow", "runs")
	}
	return filepath.Clean(root)
}

// RunWorkflow inicia uma execucao em background e retorna o resumo inicial.
func (m *Manager) RunWorkflow(ctx context.Context, req RunRequest) (RunSummary, error) {
	if req.WorkflowRef == "" {
		return RunSummary{}, fmt.Errorf("workflow ref is required")
	}

	now := time.Now()
	runID := runworkflow.NewRunID(req.WorkflowRef, now)
	runDir := filepath.Join(m.runRoot, runID)

	ar := &activeRun{
		runID:     runID,
		workflow:  req.WorkflowRef,
		status:    corerun.RunCreated,
		runDir:    runDir,
		startedAt: now,
		tag:       req.Tag,
	}

	m.mu.Lock()
	m.active[runID] = ar
	m.mu.Unlock()

	eventsJSONL := filepath.Join(runDir, "events.jsonl")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		m.mu.Lock()
		delete(m.active, runID)
		m.mu.Unlock()
		return RunSummary{}, err
	}
	uc, err := m.ucFactory(m.runRoot, eventsJSONL)
	if err != nil {
		m.mu.Lock()
		delete(m.active, runID)
		m.mu.Unlock()
		return RunSummary{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	ar.cancel = cancel

	go func() {
		defer func() {
			ar.mu.Lock()
			ar.cancel = nil
			ar.mu.Unlock()
			cancel()
		}()

		ar.mu.Lock()
		ar.status = corerun.RunRunning
		ar.mu.Unlock()

		result, err := uc.Run(runCtx, runworkflow.RunOptions{
			WorkflowRef:    req.WorkflowRef,
			RunID:          runID,
			Inputs:         req.Inputs,
			Vars:           req.Vars,
			MaxConcurrency: req.MaxConcurrency,
			WorkingDir:     req.WorkingDir,
			Tag:            req.Tag,
		})

		ar.mu.Lock()
		defer ar.mu.Unlock()

		if ar.status == corerun.RunCancelled {
			// Mantem cancelado pelo usuario
		} else if err != nil {
			ar.status = corerun.RunFailed
			ar.err = err.Error()
		} else {
			ar.status = result.Status
			if ar.status == "" {
				ar.status = corerun.RunSuccess
			}
		}
		ar.finishedAt = time.Now()
	}()

	return m.getRunSummary(runID)
}

// CancelRun cancela uma run ativa pelo contexto.
func (m *Manager) CancelRun(runID string) (RunSummary, error) {
	m.mu.Lock()
	ar, ok := m.active[runID]
	m.mu.Unlock()

	if !ok {
		return RunSummary{}, os.ErrNotExist
	}

	ar.mu.Lock()
	if ar.status == corerun.RunCreated || ar.status == corerun.RunRunning {
		ar.status = corerun.RunCancelled
		ar.finishedAt = time.Now()
		if ar.cancel != nil {
			ar.cancel()
		}
	}
	ar.mu.Unlock()

	return m.getRunSummary(runID)
}

// ListRuns retorna runs ativas e persistidas, ordenadas por data decrescente.
func (m *Manager) ListRuns() ([]RunSummary, error) {
	m.mu.Lock()
	activeIDs := make([]string, 0, len(m.active))
	for id := range m.active {
		activeIDs = append(activeIDs, id)
	}
	m.mu.Unlock()

	seen := make(map[string]struct{})
	var result []RunSummary

	for _, id := range activeIDs {
		seen[id] = struct{}{}
		summary, err := m.getRunSummary(id)
		if err != nil {
			continue
		}
		result = append(result, summary)
	}

	entries, err := os.ReadDir(m.runRoot)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return result, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		summary, err := m.loadPersistedRun(id)
		if err != nil {
			continue
		}
		result = append(result, summary)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt.After(result[j].StartedAt)
	})

	return result, nil
}

// GetRun retorna uma run ativa ou persistida.
func (m *Manager) GetRun(runID string) (RunSummary, error) {
	m.mu.Lock()
	_, active := m.active[runID]
	m.mu.Unlock()

	if active {
		return m.getRunSummary(runID)
	}
	return m.loadPersistedRun(runID)
}

// RunDir retorna o diretorio de uma run se existir.
func (m *Manager) RunDir(runID string) (string, bool) {
	m.mu.Lock()
	ar, ok := m.active[runID]
	m.mu.Unlock()
	if ok {
		return ar.runDir, true
	}
	dir := filepath.Join(m.runRoot, runID)
	if _, err := os.Stat(dir); err == nil {
		return dir, true
	}
	return "", false
}

// Shutdown cancela todas as runs ativas.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	runs := make([]*activeRun, 0, len(m.active))
	for _, ar := range m.active {
		runs = append(runs, ar)
	}
	m.mu.Unlock()

	for _, ar := range runs {
		ar.mu.Lock()
		if ar.status == corerun.RunCreated || ar.status == corerun.RunRunning {
			ar.status = corerun.RunCancelled
			ar.finishedAt = time.Now()
			if ar.cancel != nil {
				ar.cancel()
			}
		}
		ar.mu.Unlock()
	}
}

func (m *Manager) getRunSummary(runID string) (RunSummary, error) {
	m.mu.Lock()
	ar, ok := m.active[runID]
	m.mu.Unlock()

	if !ok {
		return RunSummary{}, os.ErrNotExist
	}

	ar.mu.Lock()
	summary := RunSummary{
		ID:        ar.runID,
		Workflow:  ar.workflow,
		RunDir:    ar.runDir,
		Status:    ar.status,
		StartedAt: ar.startedAt,
		Tag:       ar.tag,
	}
	if !ar.finishedAt.IsZero() {
		summary.FinishedAt = ar.finishedAt
	}
	if ar.err != "" {
		summary.Error = ar.err
	}
	ar.mu.Unlock()

	progress, _ := loadProgress(summary.RunDir)
	summary.CurrentStep = progress.CurrentStep
	summary.CompletedSteps = progress.CompletedSteps
	summary.PendingSteps = progress.PendingSteps
	summary.TotalSteps = progress.TotalSteps
	if summary.Error == "" && progress.TerminalError != "" {
		summary.Error = progress.TerminalError
	}

	m.applyDiagnosticsFromSummary(&summary)
	return summary, nil
}

func (m *Manager) loadPersistedRun(runID string) (RunSummary, error) {
	runDir := filepath.Join(m.runRoot, runID)
	if _, err := os.Stat(runDir); err != nil {
		return RunSummary{}, os.ErrNotExist
	}

	summary := RunSummary{
		ID:     runID,
		RunDir: runDir,
		Status: corerun.RunCreated,
	}

	if data, err := os.ReadFile(filepath.Join(runDir, "run.json")); err == nil {
		var meta corerun.RunMetadata
		if err := json.Unmarshal(data, &meta); err == nil {
			summary.Workflow = meta.Workflow
			summary.StartedAt = meta.StartedAt
			summary.Tag = meta.Tag
		}
	}

	if data, err := os.ReadFile(filepath.Join(runDir, "summary.json")); err == nil {
		var s corerun.Summary
		if err := json.Unmarshal(data, &s); err == nil {
			summary.Status = s.Status
			summary.FinishedAt = s.FinishedAt
		}
	}

	progress, _ := loadProgress(runDir)
	summary.CurrentStep = progress.CurrentStep
	summary.CompletedSteps = progress.CompletedSteps
	summary.PendingSteps = progress.PendingSteps
	summary.TotalSteps = progress.TotalSteps
	if progress.TerminalError != "" {
		summary.Error = progress.TerminalError
	}

	m.applyDiagnosticsFromSummary(&summary)
	return summary, nil
}

func (m *Manager) applyDiagnosticsFromSummary(summary *RunSummary) {
	if summary.RunDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(summary.RunDir, "summary.json"))
	if err != nil {
		return
	}
	var s corerun.Summary
	if err := json.Unmarshal(data, &s); err != nil {
		return
	}
	summary.DurationMS = s.DurationMS
	summary.FailedNodes = s.FailedNodes
	summary.Retries = s.Retries
	summary.AgentCalls = s.AgentCalls
	summary.BashCalls = s.BashCalls
	summary.ArtifactCount = s.ArtifactCount
	summary.FirstError = s.FirstError
	if len(s.SlowestNodes) > 0 {
		summary.SlowestNodes = append([]corerun.SlowestNode(nil), s.SlowestNodes...)
	}
	if len(s.AgentUsage) > 0 {
		summary.AgentUsage = append([]corerun.AgentUsage(nil), s.AgentUsage...)
	}

	nodesDir := filepath.Join(summary.RunDir, "nodes")
	nodeCount := 0
	_ = filepath.WalkDir(nodesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == "result.json" {
			nodeCount++
		}
		return nil
	})
	summary.NodeCount = nodeCount
}
