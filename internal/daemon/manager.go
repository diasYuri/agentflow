package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/thejerf/suture/v4"

	"github.com/diasYuri/agentflow/internal/app"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

type serviceAdder interface {
	Add(suture.Service) suture.ServiceToken
}

type Manager struct {
	cfg           Config
	runSupervisor serviceAdder
	logger        *slog.Logger
	store         RunStore

	mu      sync.Mutex
	records map[string]*runRecord
}

type runRecord struct {
	run     WorkflowRun
	service *WorkflowRunService
}

func NewManager(cfg Config, runSupervisor serviceAdder, logger *slog.Logger) *Manager {
	return NewManagerWithStore(cfg, runSupervisor, logger, nil)
}

func NewManagerWithStore(cfg Config, runSupervisor serviceAdder, logger *slog.Logger, store RunStore) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	manager := &Manager{
		cfg:           cfg,
		runSupervisor: runSupervisor,
		logger:        logger,
		store:         store,
		records:       map[string]*runRecord{},
	}
	manager.loadPersistedRuns(context.Background())
	return manager
}

func (m *Manager) Serve(ctx context.Context) error {
	<-ctx.Done()
	m.cancelAll()
	return suture.ErrDoNotRestart
}

func (m *Manager) StartWorkflow(req RunWorkflowRequest) (WorkflowRun, error) {
	if req.WorkflowRef == "" {
		return WorkflowRun{}, fmt.Errorf("workflow ref is required")
	}
	now := time.Now()
	runID := runworkflow.NewRunID(req.WorkflowRef, now)
	runRoot := firstNonEmpty(req.RunRoot, req.OutputDir, m.cfg.RunRoot)
	run := WorkflowRun{
		ID:        runID,
		Workflow:  req.WorkflowRef,
		RunDir:    filepath.Join(runRoot, runID),
		Status:    corerun.RunCreated,
		StartedAt: now,
	}
	uc, err := app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   firstNonEmpty(req.CodexPath, m.cfg.CodexPath),
		LogFormat:   req.LogFormat,
		EventsJSONL: req.EventsJSONL,
		RunRoot:     runRoot,
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	service := NewWorkflowRunService(m, uc, req, runID)
	m.mu.Lock()
	m.records[runID] = &runRecord{run: run, service: service}
	m.mu.Unlock()
	m.persist(run)
	m.runSupervisor.Add(service)
	return run, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (m *Manager) ListWorkflows() []WorkflowRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	runs := make([]WorkflowRun, 0, len(m.records))
	for _, record := range m.records {
		runs = append(runs, record.run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return runs
}

func (m *Manager) WorkflowStatus(runID string) (WorkflowRun, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, ok := m.records[runID]
	if !ok {
		return WorkflowRun{}, false
	}
	return record.run, true
}

func (m *Manager) CancelWorkflow(runID string) (WorkflowRun, error) {
	m.mu.Lock()
	record, ok := m.records[runID]
	m.mu.Unlock()
	if !ok {
		return WorkflowRun{}, os.ErrNotExist
	}
	if record.service != nil {
		record.service.Cancel()
	}
	m.mark(runID, func(run *WorkflowRun) {
		if run.Status == corerun.RunCreated || run.Status == corerun.RunRunning {
			run.Status = corerun.RunCancelled
			run.FinishedAt = time.Now()
			if record.service == nil {
				run.Error = "workflow is not active in this daemon process"
			}
		}
	})
	run, _ := m.WorkflowStatus(runID)
	return run, nil
}

func (m *Manager) WorkflowLogs(runID string) ([]string, error) {
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return nil, os.ErrNotExist
	}
	file, err := os.Open(filepath.Join(run.RunDir, "events.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func (m *Manager) markRunning(runID string) {
	m.mark(runID, func(run *WorkflowRun) {
		run.Status = corerun.RunRunning
	})
}

func (m *Manager) finish(runID string, result runworkflow.RunResult, err error) {
	m.mark(runID, func(run *WorkflowRun) {
		run.FinishedAt = time.Now()
		if result.RunDir != "" {
			run.RunDir = result.RunDir
		}
		if result.Status != "" {
			run.Status = result.Status
		}
		if err != nil {
			run.Error = err.Error()
			if run.Status == "" || run.Status == corerun.RunRunning {
				run.Status = corerun.RunFailed
			}
		}
	})
	if err != nil {
		m.logger.Error("workflow finished with error", "run_id", runID, "error", err)
		return
	}
	m.logger.Info("workflow finished", "run_id", runID, "status", result.Status)
}

func (m *Manager) mark(runID string, update func(*WorkflowRun)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record, ok := m.records[runID]; ok {
		update(&record.run)
		m.persistLocked(record.run)
	}
}

func (m *Manager) cancelAll() {
	m.mu.Lock()
	records := make([]*runRecord, 0, len(m.records))
	for _, record := range m.records {
		records = append(records, record)
	}
	m.mu.Unlock()
	for _, record := range records {
		if record.service != nil {
			record.service.Cancel()
		}
	}
}

func (m *Manager) loadPersistedRuns(ctx context.Context) {
	if m.store == nil {
		return
	}
	runs, err := m.store.LoadRuns(ctx)
	if err != nil {
		m.logger.Error("load persisted workflow runs", "error", err)
		return
	}
	for _, run := range runs {
		if run.Status == corerun.RunCreated || run.Status == corerun.RunRunning {
			run.Status = corerun.RunCancelled
			run.FinishedAt = time.Now()
			run.Error = "daemon stopped before workflow completed"
			m.persist(run)
		}
		m.records[run.ID] = &runRecord{run: run}
	}
}

func (m *Manager) persist(run WorkflowRun) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistLocked(run)
}

func (m *Manager) persistLocked(run WorkflowRun) {
	if m.store == nil {
		return
	}
	if err := m.store.UpsertRun(context.Background(), run); err != nil {
		m.logger.Error("persist workflow run", "run_id", run.ID, "error", err)
	}
}

type WorkflowRunService struct {
	manager *Manager
	uc      *runworkflow.RunWorkflowUseCase
	req     RunWorkflowRequest
	runID   string

	mu              sync.Mutex
	cancel          context.CancelFunc
	cancelRequested bool
}

func NewWorkflowRunService(manager *Manager, uc *runworkflow.RunWorkflowUseCase, req RunWorkflowRequest, runID string) *WorkflowRunService {
	return &WorkflowRunService{manager: manager, uc: uc, req: req, runID: runID}
}

func (s *WorkflowRunService) Serve(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	if s.cancelRequested {
		cancel()
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
		cancel()
	}()

	s.manager.markRunning(s.runID)
	result, err := s.uc.Run(runCtx, runOptions(s.req, s.runID))
	s.manager.finish(s.runID, result, err)
	return suture.ErrDoNotRestart
}

func (s *WorkflowRunService) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelRequested = true
	if s.cancel != nil {
		s.cancel()
	}
}
