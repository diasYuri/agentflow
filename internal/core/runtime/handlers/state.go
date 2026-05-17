package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type ExecutionState struct {
	runID                 string
	workflowPath          string
	plan                  coreworkflow.ExecutionPlan
	inputs                map[string]any
	vars                  map[string]any
	secrets               map[string]any
	masker                corerun.SecretMasker
	nodes                 map[string]any
	results               map[string]corerun.NodeResult
	global                *semaphore.Weighted
	startedAt             time.Time
	failed                bool
	parent                *ExecutionState
	path                  []string
	baseWorkingDir        string
	item                  any
	index                 *int
	total                 *int
	metrics               *executionMetrics
	cursor                int
	pause                 PauseSignaller
	worktreeEnabled       bool
	worktreeProvider      string
	worktreeAgentProvider string
	worktreePath          string
	destinationWorkingDir string
	worktreeBaseCommit    string
	worktree              coreports.Worktree
	tag                   string
}

type executionMetrics struct {
	mu         sync.Mutex
	agentCalls int
	bashCalls  int
	retries    int
}

func newExecutionState(runID string, plan coreworkflow.ExecutionPlan, inputs map[string]any, secrets map[string]any, masker corerun.SecretMasker, startedAt time.Time) *ExecutionState {
	return &ExecutionState{
		runID: runID, plan: plan, inputs: inputs, vars: plan.Workflow.Vars, secrets: secrets, masker: masker,
		nodes: map[string]any{}, results: map[string]corerun.NodeResult{}, startedAt: startedAt,
		metrics: &executionMetrics{},
	}
}

func (s *ExecutionState) incrementAgentCalls() {
	s.metrics.mu.Lock()
	s.metrics.agentCalls++
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) incrementBashCalls() {
	s.metrics.mu.Lock()
	s.metrics.bashCalls++
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) incrementRetries() {
	s.metrics.mu.Lock()
	s.metrics.retries++
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) agentCalls() int {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	return s.metrics.agentCalls
}

func (s *ExecutionState) bashCalls() int {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	return s.metrics.bashCalls
}

func (s *ExecutionState) retries() int {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	return s.metrics.retries
}

func (s *ExecutionState) restoreMetrics(metrics corerun.CheckpointMetrics) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	s.metrics.agentCalls = metrics.AgentCalls
	s.metrics.bashCalls = metrics.BashCalls
	s.metrics.retries = metrics.Retries
}

func (s *ExecutionState) set(id string, result corerun.NodeResult) {
	if len(result.Path) == 0 {
		result.Path = append([]string(nil), s.path...)
	}
	s.results[id] = result
	s.nodes[id] = map[string]any{
		"status": string(result.Status), "output": result.Output, "outputs": result.Outputs, "stdout": result.Stdout,
		"stderr": result.Stderr, "exit_code": result.ExitCode, "error": result.Error, "path": result.Path,
	}
}

func (s *ExecutionState) dependenciesReady(node coreworkflow.NodeSpec) error {
	for _, dep := range node.DependsOn {
		if !s.hasResult(dep) {
			return fmt.Errorf("dependency %q has not completed", dep)
		}
	}
	return nil
}

func (s *ExecutionState) evalContext(index *int, total *int, item any) coreworkflow.EvalContext {
	if index == nil {
		index = s.index
	}
	if total == nil {
		total = s.total
	}
	if item == nil {
		item = s.item
	}
	return coreworkflow.EvalContext{
		Inputs: s.inputs, Vars: s.vars, Secrets: s.secrets, Nodes: s.mergedNodes(), Item: item, Index: index, Total: total,
		Run: map[string]any{"id": s.runID, "workflow": s.plan.Workflow.Name},
	}
}

func (s *ExecutionState) failFast(node coreworkflow.NodeSpec) bool {
	if node.FailFast != nil {
		return *node.FailFast
	}
	if s.plan.Workflow.Execution.FailFast != nil {
		return *s.plan.Workflow.Execution.FailFast
	}
	return true
}

func (s *ExecutionState) hasResult(id string) bool {
	if _, ok := s.results[id]; ok {
		return true
	}
	if s.parent != nil {
		return s.parent.hasResult(id)
	}
	return false
}

func (s *ExecutionState) mergedNodes() map[string]any {
	out := map[string]any{}
	if s.parent != nil {
		for key, value := range s.parent.mergedNodes() {
			out[key] = value
		}
	}
	for key, value := range s.nodes {
		out[key] = value
	}
	return out
}

func (s *ExecutionState) spawn(plan coreworkflow.ExecutionPlan, path []string) *ExecutionState {
	return &ExecutionState{
		runID:                 s.runID,
		workflowPath:          s.workflowPath,
		plan:                  plan,
		inputs:                s.inputs,
		vars:                  s.vars,
		secrets:               s.secrets,
		masker:                s.masker,
		nodes:                 map[string]any{},
		results:               map[string]corerun.NodeResult{},
		global:                s.global,
		startedAt:             s.startedAt,
		parent:                s,
		path:                  append([]string(nil), path...),
		baseWorkingDir:        s.baseWorkingDir,
		metrics:               s.metrics,
		pause:                 s.pause,
		worktreeEnabled:       s.worktreeEnabled,
		worktreeProvider:      s.worktreeProvider,
		worktreeAgentProvider: s.worktreeAgentProvider,
		worktreePath:          s.worktreePath,
		destinationWorkingDir: s.destinationWorkingDir,
		worktreeBaseCommit:    s.worktreeBaseCommit,
		worktree:              s.worktree,
		tag:                   s.tag,
	}
}

func (e *Executor) recordNode(ctx context.Context, state *ExecutionState, result corerun.NodeResult) {
	if len(result.Path) == 0 {
		result.Path = append([]string(nil), state.path...)
	}
	state.set(result.NodeID, result)
	if isFailure(result.Status) {
		state.failed = true
	}
	_ = e.svc.Runs.SaveNodeResult(ctx, state.runID, state.masker.MaskNodeResult(result))
	_ = e.saveCheckpoint(ctx, state, state.plan.Workflow.Execution.PauseWhenFail, state.cursor, "", corerun.PauseReason(""))
}

func (e *Executor) saveCheckpoint(ctx context.Context, state *ExecutionState, enabled bool, cursor int, retryNodeID string, reason corerun.PauseReason) error {
	if !enabled || state == nil || state.parent != nil {
		return nil
	}
	nodes := make(map[string]corerun.NodeResult, len(state.results))
	for id, result := range state.results {
		nodes[id] = result
	}
	checkpoint := corerun.Checkpoint{
		RunID:        state.runID,
		Workflow:     state.plan.Workflow,
		WorkflowPath: state.workflowPath,
		Status:       corerun.RunRunning,
		Reason:       reason,
		Cursor:       cursor,
		RetryNodeID:  retryNodeID,
		Inputs:       state.inputs,
		StartedAt:    state.startedAt,
		Tag:          state.tag,
		UpdatedAt:    e.now(),
		Metrics: corerun.CheckpointMetrics{
			AgentCalls: state.agentCalls(),
			BashCalls:  state.bashCalls(),
			Retries:    state.retries(),
		},
		Nodes: nodes,
	}
	if state.worktreeEnabled {
		checkpoint.Worktree = &corerun.WorktreeCheckpoint{
			Enabled:               true,
			Provider:              state.worktreeProvider,
			AgentProvider:         state.worktreeAgentProvider,
			ID:                    state.worktree.ID,
			Name:                  state.worktree.Name,
			Path:                  state.worktree.Path,
			Branch:                state.worktree.Branch,
			BaseCommit:            state.worktreeBaseCommit,
			WorkflowName:          state.worktree.WorkflowName,
			DestinationWorkingDir: state.destinationWorkingDir,
		}
	}
	if reason != "" {
		checkpoint.Status = corerun.RunPaused
	}
	return e.svc.Runs.SaveCheckpoint(ctx, state.masker.MaskCheckpoint(checkpoint))
}

func (e *Executor) finish(ctx context.Context, plan coreworkflow.ExecutionPlan, state *ExecutionState, status corerun.RunStatus, finalErr error) (Result, error) {
	persistCtx := context.WithoutCancel(ctx)
	var wtMeta corerun.WorktreeMetadata
	if state.worktreeEnabled && e.svc.Worktrees != nil && state.worktree.Path != "" {
		switch status {
		case corerun.RunSuccess:
			meta, wtErr := e.finalizeWorktree(persistCtx, state)
			wtMeta = meta
			if wtErr != nil {
				// Worktree finalize failure does not change run status to failed;
				// it is recorded in metadata and artifacts.
				_ = e.emitState(persistCtx, state, corerun.Event{Type: "worktree.finalize_failed", Data: map[string]any{"error": wtErr.Error()}})
			}
			e.cleanupWorktree(persistCtx, state, &wtMeta, status)
			_ = e.saveWorktreeStatus(persistCtx, state, wtMeta)
		case corerun.RunFailed, corerun.RunCancelled, corerun.RunPaused:
			meta := e.initialWorktreeMetadata(persistCtx, state)
			meta.MergeStatus = corerun.WorktreeMergeFailed
			if provider, ok := e.svc.Worktrees.Get(state.worktreeProvider); ok {
				wtStatus, _ := provider.Status(persistCtx, state.worktree)
				meta.Commands = appendGitCommands(meta.Commands, wtStatus.Commands)
				for _, file := range wtStatus.Files {
					meta.ChangedFiles = append(meta.ChangedFiles, corerun.WorktreeChangedFile{
						Path:   file.Path,
						Status: file.Status,
					})
				}
			}
			e.cleanupWorktree(persistCtx, state, &meta, status)
			_ = e.saveWorktreeStatus(persistCtx, state, meta)
		}
	}
	finished := e.now()
	summary := corerun.Summary{
		RunID: state.runID, Workflow: plan.Workflow.Name, Status: status, StartedAt: state.startedAt,
		FinishedAt: finished, DurationMS: finished.Sub(state.startedAt).Milliseconds(), AgentCalls: state.agentCalls(),
		BashCalls: state.bashCalls(), FailedNodes: countFailedNodes(state.results), Retries: state.retries(), Nodes: state.results,
		Tag: state.tag,
	}
	publicSummary := state.masker.MaskSummary(summary)
	_ = e.svc.Runs.FinalizeRun(persistCtx, state.runID, publicSummary)
	eventType := "run.completed"
	switch status {
	case corerun.RunSuccess:
		_ = e.svc.Runs.ClearCheckpoint(persistCtx, state.runID)
	case corerun.RunPaused:
		eventType = "run.paused"
	case corerun.RunFailed, corerun.RunCancelled:
		eventType = "run.failed"
		_ = e.svc.Runs.ClearCheckpoint(persistCtx, state.runID)
	default:
		eventType = "run.failed"
		_ = e.svc.Runs.ClearCheckpoint(persistCtx, state.runID)
	}
	_ = e.emitState(persistCtx, state, corerun.Event{Type: eventType, Data: map[string]any{"status": status}})
	_ = e.svc.Events.Close(persistCtx)
	dir, _ := e.svc.Runs.RunDir(state.runID)
	if status == corerun.RunPaused {
		finalErr = nil
	}
	return Result{RunID: state.runID, RunDir: dir, Status: status, Summary: publicSummary, Plan: plan}, maskError(state.masker, finalErr)
}

func (e *Executor) emit(ctx context.Context, runID string, event corerun.Event) error {
	event.Timestamp = e.now()
	event.RunID = runID
	return e.svc.Events.Emit(ctx, event)
}

func (e *Executor) emitState(ctx context.Context, state *ExecutionState, event corerun.Event) error {
	if len(event.Path) == 0 {
		event.Path = append([]string(nil), state.path...)
	}
	return e.emit(ctx, state.runID, state.masker.MaskEvent(event))
}

func countFailedNodes(results map[string]corerun.NodeResult) int {
	count := 0
	for _, result := range results {
		if isFailure(result.Status) {
			count++
		}
	}
	return count
}

func maskError(masker corerun.SecretMasker, err error) error {
	if err == nil || masker.Empty() {
		return err
	}
	return errors.New(masker.MaskString(err.Error()))
}

func (e *Executor) now() time.Time {
	if e.svc.Now != nil {
		return e.svc.Now()
	}
	return time.Now()
}
