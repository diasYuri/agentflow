package daemon

import (
	"bufio"
	"context"
	"encoding/json"
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
	effectiveReq := req
	if effectiveReq.RunRoot == "" {
		effectiveReq.RunRoot = runRoot
	}
	storedReq := effectiveReq
	run := WorkflowRun{
		ID:        runID,
		Workflow:  req.WorkflowRef,
		RunDir:    filepath.Join(runRoot, runID),
		Status:    corerun.RunCreated,
		StartedAt: now,
		Request:   &storedReq,
	}
	uc, err := app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   firstNonEmpty(req.CodexPath, m.cfg.CodexPath, os.Getenv("AGENTFLOW_CODEX_PATH")),
		ClaudePath:  firstNonEmpty(req.ClaudePath, m.cfg.ClaudePath, os.Getenv("AGENTFLOW_CLAUDE_PATH")),
		PiPath:      firstNonEmpty(req.PiPath, m.cfg.PiPath, os.Getenv("AGENTFLOW_PI_PATH")),
		LogFormat:   req.LogFormat,
		EventsJSONL: req.EventsJSONL,
		RunRoot:     runRoot,
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	service := NewWorkflowRunService(m, uc, effectiveReq, runID, false)
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
		record.run = m.refreshRun(record.run)
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
	record.run = m.refreshRun(record.run)
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
		switch run.Status {
		case corerun.RunCreated, corerun.RunRunning:
			run.Status = corerun.RunCancelled
			run.FinishedAt = time.Now()
			if record.service == nil {
				run.Error = "workflow is not active in this daemon process"
			}
		case corerun.RunPaused:
			run.Status = corerun.RunCancelled
			run.FinishedAt = time.Now()
			run.PausedAt = time.Time{}
			run.PauseReason = ""
		}
	})
	m.clearCheckpointForRun(runID)
	run, _ := m.WorkflowStatus(runID)
	return run, nil
}

func (m *Manager) PauseWorkflow(runID string) (WorkflowRun, error) {
	m.mu.Lock()
	record, ok := m.records[runID]
	m.mu.Unlock()
	if !ok {
		return WorkflowRun{}, os.ErrNotExist
	}
	current := record.run
	switch current.Status {
	case corerun.RunPaused:
		got, _ := m.WorkflowStatus(runID)
		return got, nil
	case corerun.RunSuccess, corerun.RunFailed, corerun.RunCancelled:
		return WorkflowRun{}, fmt.Errorf("workflow run %q is already %s", runID, current.Status)
	}
	if record.service == nil {
		return WorkflowRun{}, fmt.Errorf("workflow run %q is not active in this daemon process", runID)
	}
	record.service.RequestPause()
	m.mark(runID, func(run *WorkflowRun) {
		if run.PauseReason == "" {
			run.PauseReason = string(corerun.PauseReasonManual)
		}
	})
	run, _ := m.WorkflowStatus(runID)
	return run, nil
}

func (m *Manager) ResumeWorkflow(runID string) (WorkflowRun, error) {
	m.mu.Lock()
	record, ok := m.records[runID]
	m.mu.Unlock()
	if !ok {
		return WorkflowRun{}, os.ErrNotExist
	}
	current := record.run
	if current.Status != corerun.RunPaused {
		return WorkflowRun{}, fmt.Errorf("workflow run %q is %s; only paused runs can be resumed", runID, current.Status)
	}
	if current.Request == nil {
		return WorkflowRun{}, fmt.Errorf("workflow run %q has no persisted request; cannot resume", runID)
	}
	req := *current.Request
	runRoot := firstNonEmpty(req.RunRoot, req.OutputDir, m.cfg.RunRoot)
	uc, err := app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   firstNonEmpty(req.CodexPath, m.cfg.CodexPath, os.Getenv("AGENTFLOW_CODEX_PATH")),
		ClaudePath:  firstNonEmpty(req.ClaudePath, m.cfg.ClaudePath, os.Getenv("AGENTFLOW_CLAUDE_PATH")),
		PiPath:      firstNonEmpty(req.PiPath, m.cfg.PiPath, os.Getenv("AGENTFLOW_PI_PATH")),
		LogFormat:   req.LogFormat,
		EventsJSONL: req.EventsJSONL,
		RunRoot:     runRoot,
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	service := NewWorkflowRunService(m, uc, req, runID, true)
	m.mu.Lock()
	record.service = service
	record.run.Status = corerun.RunRunning
	record.run.ResumeCount++
	record.run.PausedAt = time.Time{}
	record.run.PauseReason = ""
	record.run.Error = ""
	record.run.TerminalError = ""
	record.run.FinishedAt = time.Time{}
	updated := record.run
	m.persistLocked(updated)
	m.mu.Unlock()
	m.runSupervisor.Add(service)
	return updated, nil
}

func (m *Manager) clearCheckpointForRun(runID string) {
	m.mu.Lock()
	record, ok := m.records[runID]
	m.mu.Unlock()
	if !ok {
		return
	}
	if record.run.RunDir == "" {
		return
	}
	_ = os.Remove(filepath.Join(record.run.RunDir, "checkpoint.json"))
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

func (m *Manager) refreshRun(run WorkflowRun) WorkflowRun {
	progress, err := loadProgress(run.RunDir)
	if err != nil {
		return run
	}
	run.CurrentStep = progress.CurrentStep
	run.CompletedSteps = progress.CompletedSteps
	run.PendingSteps = progress.PendingSteps
	run.TotalSteps = progress.TotalSteps
	if run.TerminalError == "" {
		run.TerminalError = progress.TerminalError
	}
	if run.Error == "" {
		run.Error = progress.TerminalError
	}
	if len(progress.RecentEvents) > 0 {
		run.RecentEvents = progress.RecentEvents
	}
	if progress.Paused && run.Status != corerun.RunCancelled && run.Status != corerun.RunSuccess && run.Status != corerun.RunFailed {
		run.Status = corerun.RunPaused
		if run.PauseReason == "" && progress.PauseReason != "" {
			run.PauseReason = progress.PauseReason
		}
	}
	if run.Status == corerun.RunPaused && run.PauseReason == "" && progress.PauseReason != "" {
		run.PauseReason = progress.PauseReason
	}
	return run
}

func (m *Manager) markRunning(runID string) {
	m.mark(runID, func(run *WorkflowRun) {
		run.Status = corerun.RunRunning
		run.CurrentStep = ""
		run.TerminalError = ""
	})
}

func (m *Manager) finish(runID string, result runworkflow.RunResult, err error) {
	m.mark(runID, func(run *WorkflowRun) {
		if result.RunDir != "" {
			run.RunDir = result.RunDir
		}
		if result.Status != "" {
			run.Status = result.Status
		}
		applyProgress(run, result.Summary)
		if result.Status == corerun.RunPaused {
			run.PausedAt = time.Now()
			if run.PauseReason == "" {
				run.PauseReason = string(corerun.PauseReasonPauseWhenFail)
			}
			run.FinishedAt = time.Time{}
			run.Error = ""
			run.TerminalError = ""
			return
		}
		run.FinishedAt = time.Now()
		run.PausedAt = time.Time{}
		run.PauseReason = ""
		if err != nil {
			run.Error = err.Error()
			run.TerminalError = err.Error()
			if run.Status == "" || run.Status == corerun.RunRunning {
				run.Status = corerun.RunFailed
			}
		} else {
			run.Error = ""
			run.TerminalError = ""
		}
	})
	if err != nil {
		m.logger.Error("workflow finished with error", "run_id", runID, "error", err)
		return
	}
	if result.Status == corerun.RunPaused {
		m.logger.Info("workflow paused", "run_id", runID)
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
			run.TerminalError = run.Error
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
	resume  bool
	pause   *runworkflow.PauseController

	mu              sync.Mutex
	cancel          context.CancelFunc
	cancelRequested bool
}

func NewWorkflowRunService(manager *Manager, uc *runworkflow.RunWorkflowUseCase, req RunWorkflowRequest, runID string, resume bool) *WorkflowRunService {
	return &WorkflowRunService{
		manager: manager,
		uc:      uc,
		req:     req,
		runID:   runID,
		resume:  resume,
		pause:   runworkflow.NewPauseController(),
	}
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
	opts := runOptions(s.req, s.runID)
	if s.resume {
		opts = resumeOptions(s.req, s.runID)
	}
	opts.Pause = s.pause
	result, err := s.uc.Run(runCtx, opts)
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

func (s *WorkflowRunService) RequestPause() {
	if s == nil || s.pause == nil {
		return
	}
	s.pause.Request()
}

func applyProgress(run *WorkflowRun, summary corerun.Summary) {
	run.TotalSteps = len(summary.Nodes)
	run.CompletedSteps = run.CompletedSteps[:0]
	run.PendingSteps = run.PendingSteps[:0]
	run.CurrentStep = ""
	for _, id := range sortedNodeIDs(summary.Nodes) {
		node := summary.Nodes[id]
		switch node.Status {
		case corerun.NodeSuccess, corerun.NodeSkipped, corerun.NodeFailed, corerun.NodeCancelled, corerun.NodeTimeout:
			run.CompletedSteps = append(run.CompletedSteps, id)
		default:
			run.PendingSteps = append(run.PendingSteps, id)
			if run.CurrentStep == "" {
				run.CurrentStep = id
			}
		}
	}
	if run.CurrentStep == "" && len(run.PendingSteps) > 0 {
		run.CurrentStep = run.PendingSteps[0]
	}
}

type runProgress struct {
	CurrentStep    string
	CompletedSteps []string
	PendingSteps   []string
	TotalSteps     int
	TerminalError  string
	RecentEvents   []string
	Paused         bool
	PauseReason    string
}

func loadProgress(runDir string) (runProgress, error) {
	progress := runProgress{}
	if runDir == "" {
		return progress, os.ErrNotExist
	}
	planPath := filepath.Join(runDir, "plan.json")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return progress, err
	}
	var plan struct {
		Order []string `json:"order"`
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return progress, err
	}
	progress.TotalSteps = len(plan.Order)
	progress.PendingSteps = append(progress.PendingSteps, plan.Order...)
	completed := map[string]struct{}{}
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if lines, err := tailLines(eventsPath, 20); err == nil {
		progress.RecentEvents = lines
	}
	if events, err := loadRunEvents(eventsPath); err == nil {
		for _, event := range events {
			switch event.Type {
			case "node.started", "node.instance.started", "node.ready", "node.expanded":
				if event.NodeID != "" && progress.CurrentStep == "" {
					progress.CurrentStep = event.NodeID
				}
			case "node.skipped", "node.completed", "node.failed", "node.instance.completed", "node.instance.failed":
				if event.NodeID != "" {
					completed[event.NodeID] = struct{}{}
				}
			case "run.pausing":
				if event.Data != nil {
					if reason, _ := event.Data["reason"].(string); reason != "" {
						progress.PauseReason = reason
					}
				}
				if event.NodeID != "" {
					progress.CurrentStep = event.NodeID
				}
			case "run.paused":
				progress.Paused = true
				if event.Data != nil {
					if reason, _ := event.Data["reason"].(string); reason != "" {
						progress.PauseReason = reason
					}
				}
			case "run.resumed":
				progress.Paused = false
				progress.PauseReason = ""
			case "run.failed":
				if event.Data != nil {
					if status, _ := event.Data["status"].(string); status != "" {
						progress.TerminalError = status
					}
				}
			}
		}
	}
	progress.CompletedSteps = progress.CompletedSteps[:0]
	progress.PendingSteps = progress.PendingSteps[:0]
	for _, id := range plan.Order {
		if _, ok := completed[id]; ok {
			progress.CompletedSteps = append(progress.CompletedSteps, id)
			continue
		}
		progress.PendingSteps = append(progress.PendingSteps, id)
		if progress.CurrentStep == "" {
			progress.CurrentStep = id
		}
	}
	return progress, nil
}

type jsonlEvent struct {
	Type   string         `json:"type"`
	NodeID string         `json:"node_id"`
	Data   map[string]any `json:"data"`
}

func loadRunEvents(path string) ([]jsonlEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []jsonlEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event jsonlEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events, scanner.Err()
}

func tailLines(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func sortedNodeIDs(nodes map[string]corerun.NodeResult) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
