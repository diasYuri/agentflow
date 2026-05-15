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

	mu      sync.Mutex
	records map[string]*runRecord
}

type runRecord struct {
	run     WorkflowRun
	service *WorkflowRunService
}

func NewManager(cfg Config, runSupervisor serviceAdder, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		cfg:           cfg,
		runSupervisor: runSupervisor,
		logger:        logger,
		records:       map[string]*runRecord{},
	}
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
	run := WorkflowRun{
		ID:        runID,
		Workflow:  req.WorkflowRef,
		RunDir:    filepath.Join(m.cfg.RunRoot, runID),
		Status:    corerun.RunCreated,
		StartedAt: now,
	}
	uc, err := app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath: m.cfg.CodexPath,
		RunRoot:   m.cfg.RunRoot,
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	service := NewWorkflowRunService(m, uc, req, runID)
	m.mu.Lock()
	m.records[runID] = &runRecord{run: run, service: service}
	m.mu.Unlock()
	m.runSupervisor.Add(service)
	return run, nil
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
	record.service.Cancel()
	m.mark(runID, func(run *WorkflowRun) {
		if run.Status == corerun.RunCreated || run.Status == corerun.RunRunning {
			run.Status = corerun.RunCancelled
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
		record.service.Cancel()
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
