package handlers

import (
	"context"
	"encoding/json"
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
	artifacts := make(map[string]any, len(result.Artifacts))
	for _, art := range result.Artifacts {
		key := exprSafeKey(art.Name)
		artifacts[key] = map[string]any{"id": art.ID, "name": art.Name, "media_type": art.MediaType}
	}
	s.nodes[id] = map[string]any{
		"status": string(result.Status), "output": result.Output, "outputs": result.Outputs, "declared_outputs": result.DeclaredOutputs,
		"stdout": result.Stdout, "stderr": result.Stderr, "exit_code": result.ExitCode, "error": result.Error, "path": result.Path,
		"artifacts": artifacts,
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

func (e *Executor) recordNode(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec, result corerun.NodeResult) {
	if len(result.Path) == 0 {
		result.Path = append([]string(nil), state.path...)
	}
	result.Artifacts = e.saveNodeArtifacts(ctx, state, node, result)
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

func (e *Executor) evaluateAndPersistWorkflowOutputs(ctx context.Context, state *ExecutionState, workflow coreworkflow.WorkflowSpec) error {
	if len(workflow.Outputs) == 0 {
		return nil
	}
	outputs := make(map[string]any, len(workflow.Outputs))
	evalCtx := state.evalContext(nil, nil, nil)
	for name, spec := range workflow.Outputs {
		value, err := coreworkflow.EvalTemplateValue(fmt.Sprintf("%v", spec.Value), evalCtx)
		if err != nil {
			return fmt.Errorf("outputs.%s: %w", name, err)
		}
		if spec.Type != "" {
			if err := coreworkflow.ValidateSchema(value, map[string]any{"type": spec.Type}, "outputs."+name); err != nil {
				return err
			}
		}
		if len(spec.Schema) > 0 {
			if err := coreworkflow.ValidateSchema(value, spec.Schema, "outputs."+name); err != nil {
				return err
			}
		}
		outputs[name] = value
	}
	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal workflow outputs: %w", err)
	}
	maskedData := []byte(state.masker.MaskString(string(data)))
	art := corerun.Artifact{
		ID:           "workflow/outputs.json",
		RunID:        state.runID,
		Name:         "outputs.json",
		RelativePath: "workflow/outputs.json",
		MediaType:    "application/json",
		Kind:         corerun.ArtifactKindResult,
	}
	if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, maskedData); err != nil {
		return fmt.Errorf("failed to save workflow outputs artifact: %w", err)
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "workflow.outputs", Data: outputs})
	return nil
}

func (e *Executor) finish(ctx context.Context, plan coreworkflow.ExecutionPlan, state *ExecutionState, status corerun.RunStatus, finalErr error) (Result, error) {
	persistCtx := context.WithoutCancel(ctx)
	var wtMeta corerun.WorktreeMetadata
	var pauseReason corerun.PauseReason
	if state.worktreeEnabled && e.svc.Worktrees != nil && state.worktree.Path != "" {
		switch status {
		case corerun.RunSuccess:
			meta, wtErr := e.finalizeWorktree(persistCtx, state)
			wtMeta = meta
			worktreeComplete := wtErr == nil && (meta.MergeStatus == corerun.WorktreeMergeMerged || meta.MergeStatus == corerun.WorktreeMergeNoChanges)
			if wtErr != nil {
				_ = e.emitState(persistCtx, state, corerun.Event{Type: "worktree.finalize_failed", Data: map[string]any{"error": wtErr.Error()}})
			}
			if !worktreeComplete {
				if wtErr == nil {
					msg := wtMeta.MergeFailureCause
					if msg == "" {
						msg = fmt.Sprintf("worktree merge did not complete: %s", wtMeta.MergeStatus)
					}
					_ = e.emitState(persistCtx, state, corerun.Event{Type: "worktree.finalize_failed", Data: map[string]any{"error": msg}})
				}
				status = corerun.RunPaused
				pauseReason = corerun.PauseReasonWorktreeMerge
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
	if status == corerun.RunSuccess {
		if err := e.evaluateAndPersistWorkflowOutputs(persistCtx, state, plan.Workflow); err != nil {
			_ = e.emitState(persistCtx, state, corerun.Event{Type: "workflow.outputs_failed", Data: map[string]any{"error": err.Error()}})
			status = corerun.RunFailed
			finalErr = err
		}
	}

	if status == corerun.RunSuccess {
		if err := e.runHooks(persistCtx, state, coreworkflow.HookPhaseAfterSuccess); err != nil {
			status = corerun.RunFailed
			finalErr = err
		}
	}

	if status == corerun.RunFailed {
		if hookErr := e.runHooks(persistCtx, state, coreworkflow.HookPhaseAfterFailure); hookErr != nil {
			if finalErr != nil {
				finalErr = fmt.Errorf("%w; after_failure hook failed: %v", finalErr, hookErr)
			} else {
				finalErr = hookErr
			}
		}
	}

	if status != corerun.RunPaused {
		if hookErr := e.runHooks(persistCtx, state, coreworkflow.HookPhaseAfterRun); hookErr != nil {
			if status == corerun.RunSuccess {
				status = corerun.RunFailed
			}
			if finalErr != nil {
				finalErr = fmt.Errorf("%w; after_run hook failed: %v", finalErr, hookErr)
			} else {
				finalErr = hookErr
			}
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
	eventData := map[string]any{"status": status}
	if status == corerun.RunPaused && pauseReason != "" {
		eventData["reason"] = pauseReason
	}
	_ = e.emitState(persistCtx, state, corerun.Event{Type: eventType, Data: eventData})
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
