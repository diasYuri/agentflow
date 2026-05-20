package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	approvalNodeID        string
	approvalMessage       string
}

type executionMetrics struct {
	mu            sync.Mutex
	agentCalls    int
	bashCalls     int
	retries       int
	nodeMetrics   map[string]corerun.NodeMetrics
	agentUsage    []corerun.AgentUsage
	timeline      []corerun.TimelineEntry
	artifactCount int
	firstError    string
}

func newExecutionState(runID string, plan coreworkflow.ExecutionPlan, inputs map[string]any, secrets map[string]any, masker corerun.SecretMasker, startedAt time.Time) *ExecutionState {
	return &ExecutionState{
		runID: runID, plan: plan, inputs: inputs, vars: plan.Workflow.Vars, secrets: secrets, masker: masker,
		nodes: map[string]any{}, results: map[string]corerun.NodeResult{}, startedAt: startedAt,
		metrics: &executionMetrics{nodeMetrics: map[string]corerun.NodeMetrics{}},
	}
}

func (s *ExecutionState) incrementAgentCalls(nodeID string) {
	s.metrics.mu.Lock()
	s.metrics.agentCalls++
	if nm, ok := s.metrics.nodeMetrics[nodeID]; ok {
		nm.AgentCalls++
		s.metrics.nodeMetrics[nodeID] = nm
	} else {
		s.metrics.nodeMetrics[nodeID] = corerun.NodeMetrics{NodeID: nodeID, AgentCalls: 1}
	}
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) incrementBashCalls(nodeID string) {
	s.metrics.mu.Lock()
	s.metrics.bashCalls++
	if nm, ok := s.metrics.nodeMetrics[nodeID]; ok {
		nm.BashCalls++
		s.metrics.nodeMetrics[nodeID] = nm
	} else {
		s.metrics.nodeMetrics[nodeID] = corerun.NodeMetrics{NodeID: nodeID, BashCalls: 1}
	}
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

func (s *ExecutionState) recordNodeMetrics(result corerun.NodeResult) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	key := result.NodeID
	metrics := corerun.NodeMetrics{
		NodeID:        result.NodeID,
		InstanceID:    result.InstanceID,
		DurationMS:    result.Duration.Milliseconds(),
		Attempts:      result.Attempts,
		Retries:       0,
		StdoutBytes:   int64(len(result.Stdout)),
		StderrBytes:   int64(len(result.Stderr)),
		ArtifactCount: len(result.Artifacts),
		FirstError:    result.Error,
	}
	if result.Attempts > 0 {
		metrics.Retries = result.Attempts - 1
	}
	if existing, ok := s.metrics.nodeMetrics[key]; ok {
		existing.DurationMS += metrics.DurationMS
		existing.Attempts += metrics.Attempts
		existing.Retries += metrics.Retries
		existing.StdoutBytes += metrics.StdoutBytes
		existing.StderrBytes += metrics.StderrBytes
		existing.ArtifactCount += metrics.ArtifactCount
		existing.BashCalls = max(existing.BashCalls, metrics.BashCalls)
		existing.AgentCalls = max(existing.AgentCalls, metrics.AgentCalls)
		if existing.FirstError == "" && metrics.FirstError != "" {
			existing.FirstError = metrics.FirstError
		}
		s.metrics.nodeMetrics[key] = existing
	} else {
		s.metrics.nodeMetrics[key] = metrics
	}
}

func (s *ExecutionState) recordFirstError(message string) {
	if message == "" {
		return
	}
	s.metrics.mu.Lock()
	if s.metrics.firstError == "" {
		s.metrics.firstError = message
	}
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) recordAgentUsage(provider, model string, usage *coreports.Usage, costUSD float64) {
	if usage == nil {
		return
	}
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	s.metrics.agentUsage = append(s.metrics.agentUsage, corerun.AgentUsage{
		Provider:     provider,
		Model:        model,
		InputTokens:  int64(usage.InputTokens),
		OutputTokens: int64(usage.OutputTokens),
		TotalTokens:  int64(usage.TotalTokens),
		CostUSD:      costUSD,
	})
}

func (s *ExecutionState) recordArtifact(count int) {
	s.metrics.mu.Lock()
	s.metrics.artifactCount += count
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) addTimeline(entry corerun.TimelineEntry) {
	s.metrics.mu.Lock()
	s.metrics.timeline = append(s.metrics.timeline, entry)
	s.metrics.mu.Unlock()
}

func (s *ExecutionState) nodeMetricsMap() map[string]corerun.NodeMetrics {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	out := make(map[string]corerun.NodeMetrics, len(s.metrics.nodeMetrics))
	for k, v := range s.metrics.nodeMetrics {
		out[k] = v
	}
	return out
}

func (s *ExecutionState) agentUsage() []corerun.AgentUsage {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	out := make([]corerun.AgentUsage, len(s.metrics.agentUsage))
	copy(out, s.metrics.agentUsage)
	return out
}

func (s *ExecutionState) timeline() []corerun.TimelineEntry {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	out := make([]corerun.TimelineEntry, len(s.metrics.timeline))
	copy(out, s.metrics.timeline)
	return out
}

func (s *ExecutionState) artifactCount() int {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	return s.metrics.artifactCount
}

func (s *ExecutionState) firstError() string {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	return s.metrics.firstError
}

func (s *ExecutionState) restoreMetrics(metrics corerun.CheckpointMetrics) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()
	s.metrics.agentCalls = metrics.AgentCalls
	s.metrics.bashCalls = metrics.BashCalls
	s.metrics.retries = metrics.Retries
	if metrics.NodeMetrics != nil {
		s.metrics.nodeMetrics = metrics.NodeMetrics
	}
	if metrics.AgentUsage != nil {
		s.metrics.agentUsage = metrics.AgentUsage
	}
	if metrics.Timeline != nil {
		s.metrics.timeline = metrics.Timeline
	}
	s.metrics.artifactCount = metrics.ArtifactCount
	s.metrics.firstError = metrics.FirstError
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
	if isFailure(result.Status) && result.Error != "" {
		state.recordFirstError(result.Error)
	}
	state.recordNodeMetrics(result)
	if isFailure(result.Status) {
		state.failed = true
	}
	_ = e.svc.Runs.SaveNodeResult(ctx, state.runID, state.masker.MaskNodeResult(result))
	metrics := state.nodeMetricsMap()[result.NodeID]
	_ = e.emitState(ctx, state, corerun.Event{Type: "node.metrics", NodeID: result.NodeID, InstanceID: result.InstanceID, Data: map[string]any{"metrics": metrics}})
	_ = e.saveCheckpoint(ctx, state, state.plan.Workflow.Execution.PauseWhenFail, state.cursor, "", corerun.PauseReason(""))
}

func (e *Executor) saveCheckpoint(ctx context.Context, state *ExecutionState, enabled bool, cursor int, retryNodeID string, reason corerun.PauseReason) error {
	if !enabled || state == nil || state.parent != nil {
		return nil
	}
	checkpoint := e.buildCheckpoint(state, cursor, retryNodeID, reason)
	return e.svc.Runs.SaveCheckpoint(ctx, state.masker.MaskCheckpoint(checkpoint))
}

func (e *Executor) saveApprovalCheckpoint(ctx context.Context, state *ExecutionState, cursor int, approval corerun.ApprovalCheckpoint) error {
	if state == nil || state.parent != nil {
		return nil
	}
	checkpoint := e.buildCheckpoint(state, cursor, "", "")
	checkpoint.Status = corerun.RunWaitingApproval
	checkpoint.Approval = &approval
	return e.svc.Runs.SaveCheckpoint(ctx, state.masker.MaskCheckpoint(checkpoint))
}

func (e *Executor) buildCheckpoint(state *ExecutionState, cursor int, retryNodeID string, reason corerun.PauseReason) corerun.Checkpoint {
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
			AgentCalls:    state.agentCalls(),
			BashCalls:     state.bashCalls(),
			Retries:       state.retries(),
			NodeMetrics:   state.nodeMetricsMap(),
			AgentUsage:    state.agentUsage(),
			Timeline:      state.timeline(),
			ArtifactCount: state.artifactCount(),
			FirstError:    state.firstError(),
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
	return checkpoint
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
	state.recordArtifact(1)
	_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
	_ = e.emitState(ctx, state, corerun.Event{Type: "workflow.outputs", Data: outputs})
	return nil
}

func (e *Executor) finish(ctx context.Context, plan coreworkflow.ExecutionPlan, state *ExecutionState, status corerun.RunStatus, finalErr error) (Result, error) {
	persistCtx := context.WithoutCancel(ctx)
	if e.svc.Extensions != nil {
		_ = e.svc.Extensions.CloseRun(persistCtx, state.runID)
	}
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
		case corerun.RunFailed, corerun.RunCancelled, corerun.RunPaused, corerun.RunWaitingApproval:
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

	if status != corerun.RunPaused && status != corerun.RunWaitingApproval {
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
	eventType := "run.completed"
	switch status {
	case corerun.RunPaused:
		eventType = "run.paused"
	case corerun.RunWaitingApproval:
		eventType = "run.wait_approval"
	case corerun.RunFailed, corerun.RunCancelled:
		eventType = "run.failed"
	}
	eventData := map[string]any{"status": status}
	if status == corerun.RunPaused && pauseReason != "" {
		eventData["reason"] = pauseReason
	}
	if status == corerun.RunWaitingApproval && state.cursor < len(plan.Order) {
		eventData["cursor"] = state.cursor
		if state.approvalNodeID != "" {
			eventData["node_id"] = state.approvalNodeID
		}
		if state.approvalMessage != "" {
			eventData["message"] = state.approvalMessage
		}
	}
	_ = e.emitState(persistCtx, state, corerun.Event{Type: eventType, Data: eventData})
	state.addTimeline(corerun.TimelineEntry{Timestamp: finished, Type: eventType})
	summary := corerun.Summary{
		RunID: state.runID, Workflow: plan.Workflow.Name, Status: status, StartedAt: state.startedAt,
		FinishedAt: finished, DurationMS: finished.Sub(state.startedAt).Milliseconds(), AgentCalls: state.agentCalls(),
		BashCalls: state.bashCalls(), FailedNodes: countFailedNodes(state.results), Retries: state.retries(), Nodes: state.results,
		Tag:           state.tag,
		SlowestNodes:  computeSlowestNodes(state.nodeMetricsMap()),
		AgentUsage:    state.agentUsage(),
		Timeline:      state.timeline(),
		ArtifactCount: state.artifactCount(),
		FirstError:    state.firstError(),
	}
	failureReason := ""
	switch {
	case finalErr != nil:
		failureReason = finalErr.Error()
	case status == corerun.RunPaused && pauseReason == corerun.PauseReasonWorktreeMerge:
		if wtMeta.MergeFailureCause != "" {
			failureReason = wtMeta.MergeFailureCause
		} else {
			failureReason = firstFailureReason(summary.Nodes)
		}
	case status == corerun.RunPaused && pauseReason == corerun.PauseReasonPauseWhenFail:
		failureReason = firstFailureReason(summary.Nodes)
	case status == corerun.RunWaitingApproval:
		failureReason = ""
	case status == corerun.RunFailed:
		failureReason = firstFailureReason(summary.Nodes)
	}
	if failureReason != "" {
		failureReason = state.masker.MaskString(failureReason)
	}
	publicSummary := state.masker.MaskSummary(summary)
	persistedSummary := publicSummary
	persistedSummary.Timeline = nil
	persistedSummary.AgentUsage = nil
	_ = e.svc.Runs.FinalizeRun(persistCtx, state.runID, persistedSummary)
	switch status {
	case corerun.RunSuccess:
		_ = e.svc.Runs.ClearCheckpoint(persistCtx, state.runID)
	case corerun.RunFailed, corerun.RunCancelled:
		_ = e.svc.Runs.ClearCheckpoint(persistCtx, state.runID)
	}
	_ = e.emitState(persistCtx, state, corerun.Event{Type: "run.summary.updated", Data: map[string]any{"summary": publicSummary}})
	_ = e.svc.Events.Close(persistCtx)
	dir, _ := e.svc.Runs.RunDir(state.runID)
	if status == corerun.RunPaused {
		finalErr = nil
	}
	return Result{
		RunID:           state.runID,
		RunDir:          dir,
		Status:          status,
		PauseReason:     pauseReason,
		ApprovalNodeID:  state.approvalNodeID,
		ApprovalMessage: state.approvalMessage,
		FailureReason:   failureReason,
		Summary:         publicSummary,
		Plan:            plan,
	}, maskError(state.masker, finalErr)
}

func (e *Executor) emit(ctx context.Context, runID string, event corerun.Event) error {
	if e.svc.Events == nil {
		return nil
	}
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

func computeSlowestNodes(nodeMetrics map[string]corerun.NodeMetrics) []corerun.SlowestNode {
	nodes := make([]corerun.SlowestNode, 0, len(nodeMetrics))
	for _, m := range nodeMetrics {
		nodes = append(nodes, corerun.SlowestNode{NodeID: m.NodeID, DurationMS: m.DurationMS})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].DurationMS > nodes[j].DurationMS
	})
	return nodes
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
