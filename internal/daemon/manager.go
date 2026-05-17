package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		Tag:       req.Tag,
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

func (m *Manager) workflowRunSnapshot(runID string) (WorkflowRun, bool) {
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
	logMemStats(m.logger, "WorkflowLogs start", slog.String("run_id", runID))
	defer func() {
		logMemStats(m.logger, "WorkflowLogs end", slog.String("run_id", runID))
	}()
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

func (m *Manager) WorkflowEvents(runID string, cursor int, limit int) (WorkflowEventsResponse, error) {
	logMemStats(m.logger, "WorkflowEvents start", slog.String("run_id", runID), slog.Int("cursor", cursor), slog.Int("limit", limit))
	defer func() {
		logMemStats(m.logger, "WorkflowEvents end", slog.String("run_id", runID))
	}()
	if limit <= 0 {
		limit = defaultEventLimit
	}
	if limit > maxEventLimit {
		limit = maxEventLimit
	}
	if cursor < 0 {
		cursor = 0
	}

	run, ok := m.workflowRunSnapshot(runID)
	if !ok {
		return WorkflowEventsResponse{}, os.ErrNotExist
	}

	response := WorkflowEventsResponse{
		RunID:      runID,
		NextCursor: cursor,
		Events:     make([]WorkflowEventDTO, 0, limit),
	}

	eventsPath := filepath.Join(run.RunDir, "events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return response, nil
		}
		return WorkflowEventsResponse{}, err
	}
	defer file.Close()

	var masker *corerun.SecretMasker
	if run.Request != nil && len(run.Request.Vars) > 0 {
		m := corerun.NewSecretMasker(run.Request.Vars)
		masker = &m
	}

	scanner := bufio.NewScanner(file)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	lineIndex := 0
	collected := 0

	for scanner.Scan() {
		if lineIndex < cursor {
			lineIndex++
			continue
		}

		if collected >= limit {
			response.HasMore = true
			response.NextCursor = lineIndex
			break
		}

		line := scanner.Bytes()
		var event corerun.Event
		if err := json.Unmarshal(line, &event); err != nil {
			lineIndex++
			response.NextCursor = lineIndex
			continue
		}

		if masker != nil {
			event = masker.MaskEvent(event)
		}

		dto := WorkflowEventDTO{
			Cursor:     lineIndex,
			Timestamp:  event.Timestamp,
			RunID:      event.RunID,
			Type:       event.Type,
			NodeID:     event.NodeID,
			InstanceID: event.InstanceID,
			Path:       event.Path,
			Attempt:    event.Attempt,
			Data:       event.Data,
		}
		response.Events = append(response.Events, dto)
		collected++
		lineIndex++
		response.NextCursor = lineIndex
	}

	if err := scanner.Err(); err != nil {
		return WorkflowEventsResponse{}, err
	}

	return response, nil
}

func (m *Manager) refreshRun(run WorkflowRun) WorkflowRun {
	logMemStats(m.logger, "refreshRun start", slog.String("run_id", run.ID))
	progress, err := loadProgress(run.RunDir)
	logMemStats(m.logger, "refreshRun end", slog.String("run_id", run.ID), slog.Int("total_steps", progress.TotalSteps), slog.Int("completed_steps", len(progress.CompletedSteps)), slog.Int("pending_steps", len(progress.PendingSteps)), slog.Int("recent_events", len(progress.RecentEvents)))
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
		if isTerminalRunStatus(run.Status) {
			return
		}
		run.Status = corerun.RunRunning
		run.CurrentStep = ""
		run.TerminalError = ""
	})
}

func (m *Manager) finish(runID string, result runworkflow.RunResult, err error) {
	m.mark(runID, func(run *WorkflowRun) {
		wasCancelled := run.Status == corerun.RunCancelled
		if result.RunDir != "" {
			run.RunDir = result.RunDir
		}
		if wasCancelled {
			applyProgress(run, result.Summary)
			run.Status = corerun.RunCancelled
			if run.FinishedAt.IsZero() {
				run.FinishedAt = time.Now()
			}
			run.PausedAt = time.Time{}
			run.PauseReason = ""
			if err != nil && run.Error == "" {
				run.Error = err.Error()
				run.TerminalError = err.Error()
			}
			return
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

func isTerminalRunStatus(status corerun.RunStatus) bool {
	return status == corerun.RunSuccess || status == corerun.RunFailed || status == corerun.RunCancelled
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

func safeJoin(base, rel string) (string, error) {
	if err := validateArtifactRelativePath(rel); err != nil {
		return "", err
	}
	if err := validateArtifactAncestors(base, rel); err != nil {
		return "", err
	}
	return filepath.Join(base, filepath.FromSlash(rel)), nil
}

func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

func detectFileContentType(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	var sample [512]byte
	n, err := file.Read(sample[:])
	if err != nil && n == 0 {
		return ""
	}
	return http.DetectContentType(sample[:n])
}

func (m *Manager) maskerForRun(run WorkflowRun) *corerun.SecretMasker {
	if run.Request != nil && len(run.Request.Vars) > 0 {
		masker := corerun.NewSecretMasker(run.Request.Vars)
		return &masker
	}
	return nil
}

func (m *Manager) loadArtifactIndex(runDir string) (map[string]corerun.Artifact, error) {
	indexPath := filepath.Join(runDir, "artifacts", "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var index map[string]corerun.Artifact
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index == nil {
		return map[string]corerun.Artifact{}, nil
	}
	return index, nil
}

func artifactToDTO(a corerun.Artifact) WorkflowArtifactDTO {
	return WorkflowArtifactDTO{
		ID:           a.ID,
		Name:         a.Name,
		Path:         a.ID,
		Size:         a.SizeBytes,
		ContentType:  a.MediaType,
		RunID:        a.RunID,
		NodeID:       a.NodeID,
		InstanceID:   a.InstanceID,
		RelativePath: a.RelativePath,
		MediaType:    a.MediaType,
		SizeBytes:    a.SizeBytes,
		CreatedAt:    a.CreatedAt,
		Kind:         a.Kind,
		Description:  a.Description,
	}
}

func (m *Manager) fallbackScanArtifacts(runDir string) ([]WorkflowArtifactDTO, error) {
	var artifacts []WorkflowArtifactDTO
	artifactsDir := filepath.Join(runDir, "artifacts")
	if err := filepath.WalkDir(artifactsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(artifactsDir, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, WorkflowArtifactDTO{
			ID:          rel,
			Name:        filepath.Base(rel),
			Path:        rel,
			Size:        info.Size(),
			ContentType: detectFileContentType(path),
			ModifiedAt:  info.ModTime(),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return artifacts, nil
}

func (m *Manager) WorkflowArtifacts(runID string) (WorkflowArtifactsResponse, error) {
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return WorkflowArtifactsResponse{}, os.ErrNotExist
	}
	resp := WorkflowArtifactsResponse{
		RunID:     runID,
		Artifacts: []WorkflowArtifactDTO{},
	}
	index, err := m.loadArtifactIndex(run.RunDir)
	if err != nil {
		return WorkflowArtifactsResponse{}, err
	}
	if len(index) > 0 {
		for _, a := range index {
			resp.Artifacts = append(resp.Artifacts, artifactToDTO(a))
		}
		sort.Slice(resp.Artifacts, func(i, j int) bool {
			return resp.Artifacts[i].ID < resp.Artifacts[j].ID
		})
		return resp, nil
	}
	// Fallback to filesystem scan for old runs without index.
	artifacts, err := m.fallbackScanArtifacts(run.RunDir)
	if err != nil {
		return WorkflowArtifactsResponse{}, err
	}
	resp.Artifacts = artifacts
	return resp, nil
}

func isTextMediaType(mt string) bool {
	return strings.HasPrefix(mt, "text/") ||
		mt == "application/json" ||
		mt == "application/x-yaml" ||
		mt == "application/javascript" ||
		mt == "application/xml" ||
		mt == "application/sql"
}

func (m *Manager) WorkflowArtifact(runID, artifactID string) (WorkflowArtifactResponse, error) {
	logMemStats(m.logger, "WorkflowArtifact start", slog.String("run_id", runID), slog.String("artifact_id", artifactID))
	defer func() {
		logMemStats(m.logger, "WorkflowArtifact end", slog.String("run_id", runID), slog.String("artifact_id", artifactID))
	}()
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return WorkflowArtifactResponse{}, os.ErrNotExist
	}
	index, err := m.loadArtifactIndex(run.RunDir)
	if err != nil {
		return WorkflowArtifactResponse{}, err
	}
	var artifact corerun.Artifact
	if len(index) > 0 {
		var found bool
		artifact, found = index[artifactID]
		if !found {
			return WorkflowArtifactResponse{}, os.ErrNotExist
		}
	}
	artifactsDir := filepath.Join(run.RunDir, "artifacts")
	var path string
	if artifact.ID != "" {
		path, err = safeJoin(artifactsDir, artifact.RelativePath)
		if err != nil {
			return WorkflowArtifactResponse{}, err
		}
	} else {
		path, err = safeJoin(artifactsDir, artifactID)
		if err != nil {
			return WorkflowArtifactResponse{}, fmt.Errorf("invalid artifact id: %w", err)
		}
	}
	if isSymlink(path) {
		return WorkflowArtifactResponse{}, fmt.Errorf("symlink not allowed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return WorkflowArtifactResponse{}, os.ErrNotExist
		}
		return WorkflowArtifactResponse{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return WorkflowArtifactResponse{}, err
	}
	masker := m.maskerForRun(run)
	if masker != nil {
		data = []byte(masker.MaskString(string(data)))
	}
	mt := artifact.MediaType
	if mt == "" {
		mt = http.DetectContentType(data)
	}
	text := isTextMediaType(mt)
	resp := WorkflowArtifactResponse{
		ID:           artifactID,
		Name:         filepath.Base(artifactID),
		Path:         artifactID,
		Size:         info.Size(),
		ContentType:  mt,
		RunID:        runID,
		NodeID:       artifact.NodeID,
		InstanceID:   artifact.InstanceID,
		RelativePath: artifact.RelativePath,
		MediaType:    mt,
		SizeBytes:    info.Size(),
		CreatedAt:    artifact.CreatedAt,
		Kind:         artifact.Kind,
		Description:  artifact.Description,
		IsText:       text,
	}
	if text {
		resp.Encoding = "text"
		if len(data) > MaxArtifactInline {
			resp.Truncated = true
			resp.TextContent = string(data[:MaxArtifactInline])
		} else {
			resp.TextContent = string(data)
		}
	} else {
		resp.Encoding = "base64"
		resp.Truncated = true
	}
	return resp, nil
}

func validateArtifactRelativePath(rel string) error {
	if rel == "" {
		return fmt.Errorf("path is empty")
	}
	clean := filepath.Clean(rel)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("path is absolute")
	}
	if strings.Contains(clean, "..") {
		return fmt.Errorf("path contains ..")
	}
	if strings.HasPrefix(filepath.ToSlash(clean), "..") {
		return fmt.Errorf("path escapes base directory")
	}
	return nil
}

func validateArtifactAncestors(baseDir, rel string) error {
	current := baseDir
	parts := strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/")
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact path is a symlink")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("path is not a directory")
		}
	}
	return nil
}

func (m *Manager) WorkflowArtifactPath(runID, artifactID string) (string, error) {
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return "", os.ErrNotExist
	}
	index, err := m.loadArtifactIndex(run.RunDir)
	if err != nil {
		return "", err
	}
	if len(index) == 0 {
		return "", fmt.Errorf("artifact index not available for run %s", runID)
	}
	artifact, found := index[artifactID]
	if !found {
		return "", os.ErrNotExist
	}
	artifactsDir := filepath.Join(run.RunDir, "artifacts")
	path, err := safeJoin(artifactsDir, artifact.RelativePath)
	if err != nil {
		return "", err
	}
	if isSymlink(path) {
		return "", fmt.Errorf("symlink not allowed")
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	return path, nil
}

func nodeResultToDTO(result corerun.NodeResult) WorkflowNodeResultDTO {
	dto := WorkflowNodeResultDTO{
		NodeID:     result.NodeID,
		InstanceID: result.InstanceID,
		Path:       result.Path,
		Index:      result.Index,
		Status:     string(result.Status),
		Output:     result.Output,
		Outputs:    result.Outputs,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		Error:      result.Error,
		ExitCode:   result.ExitCode,
		Attempts:   result.Attempts,
	}
	if result.Duration > 0 {
		dto.Duration = result.Duration.Milliseconds()
	}
	return dto
}

func (m *Manager) WorkflowNodes(runID string) (WorkflowNodesResponse, error) {
	logMemStats(m.logger, "WorkflowNodes start", slog.String("run_id", runID))
	defer func(start time.Time) {
		logMemStats(m.logger, "WorkflowNodes end", slog.String("run_id", runID), slog.Duration("elapsed", time.Since(start)))
	}(time.Now())
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return WorkflowNodesResponse{}, os.ErrNotExist
	}
	resp := WorkflowNodesResponse{
		RunID: runID,
		Nodes: []WorkflowNodeResultDTO{},
	}
	nodesDir := filepath.Join(run.RunDir, "nodes")
	masker := m.maskerForRun(run)
	if err := filepath.WalkDir(nodesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result corerun.NodeResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		if masker != nil {
			result = masker.MaskNodeResult(result)
		}
		resp.Nodes = append(resp.Nodes, nodeResultToDTO(result))
		return nil
	}); err != nil {
		return WorkflowNodesResponse{}, err
	}
	sort.Slice(resp.Nodes, func(i, j int) bool {
		if resp.Nodes[i].NodeID != resp.Nodes[j].NodeID {
			return resp.Nodes[i].NodeID < resp.Nodes[j].NodeID
		}
		if resp.Nodes[i].InstanceID != resp.Nodes[j].InstanceID {
			return resp.Nodes[i].InstanceID < resp.Nodes[j].InstanceID
		}
		ii, ji := -1, -1
		if resp.Nodes[i].Index != nil {
			ii = *resp.Nodes[i].Index
		}
		if resp.Nodes[j].Index != nil {
			ji = *resp.Nodes[j].Index
		}
		return ii < ji
	})
	return resp, nil
}

func (m *Manager) WorkflowNode(runID, nodeID string) (WorkflowNodeResponse, error) {
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return WorkflowNodeResponse{}, os.ErrNotExist
	}
	resp := WorkflowNodeResponse{
		RunID:  runID,
		NodeID: nodeID,
	}
	nodesDir := filepath.Join(run.RunDir, "nodes")
	masker := m.maskerForRun(run)
	found := false
	if err := filepath.WalkDir(nodesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() || d.Name() != "result.json" {
			return nil
		}
		if isSymlink(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var result corerun.NodeResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil
		}
		if result.NodeID != nodeID {
			return nil
		}
		found = true
		if masker != nil {
			result = masker.MaskNodeResult(result)
		}
		resp.Instances = append(resp.Instances, nodeResultToDTO(result))
		return nil
	}); err != nil {
		return WorkflowNodeResponse{}, err
	}
	if !found {
		return WorkflowNodeResponse{}, os.ErrNotExist
	}
	sort.Slice(resp.Instances, func(i, j int) bool {
		if resp.Instances[i].InstanceID != resp.Instances[j].InstanceID {
			return resp.Instances[i].InstanceID < resp.Instances[j].InstanceID
		}
		ii, ji := -1, -1
		if resp.Instances[i].Index != nil {
			ii = *resp.Instances[i].Index
		}
		if resp.Instances[j].Index != nil {
			ji = *resp.Instances[j].Index
		}
		return ii < ji
	})
	return resp, nil
}

func (m *Manager) WorkflowPlan(runID string) (WorkflowPlanResponse, error) {
	run, ok := m.WorkflowStatus(runID)
	if !ok {
		return WorkflowPlanResponse{}, os.ErrNotExist
	}
	resp := WorkflowPlanResponse{RunID: runID}
	masker := m.maskerForRun(run)

	if data, err := os.ReadFile(filepath.Join(run.RunDir, "run.json")); err == nil {
		var meta map[string]any
		if err := json.Unmarshal(data, &meta); err == nil {
			resp.Metadata = meta
		}
	}
	if data, err := os.ReadFile(filepath.Join(run.RunDir, "workflow.yaml")); err == nil {
		workflow := string(data)
		if masker != nil {
			workflow = masker.MaskString(workflow)
		}
		resp.Workflow = workflow
	}
	if data, err := os.ReadFile(filepath.Join(run.RunDir, "normalized.json")); err == nil {
		var norm map[string]any
		if err := json.Unmarshal(data, &norm); err == nil {
			if masker != nil {
				norm = maskMap(*masker, norm)
			}
			resp.Normalized = norm
		}
	}
	if data, err := os.ReadFile(filepath.Join(run.RunDir, "plan.json")); err == nil {
		var plan map[string]any
		if err := json.Unmarshal(data, &plan); err == nil {
			if masker != nil {
				plan = maskMap(*masker, plan)
			}
			resp.Plan = plan
		}
	}
	return resp, nil
}

func maskMap(masker corerun.SecretMasker, value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = masker.MaskValue(item)
	}
	return out
}
