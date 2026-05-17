package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type Executor struct {
	svc Services
}

const internalGitWorktreeProvider = "git"

var errRunPaused = errors.New("run paused")

func Execute(ctx context.Context, svc Services, req ExecutionRequest) (Result, error) {
	return (&Executor{svc: svc}).execute(ctx, req)
}

func (e *Executor) execute(ctx context.Context, req ExecutionRequest) (Result, error) {
	if req.ResumeRunID != "" {
		return e.resume(ctx, req)
	}
	now := e.now()
	runID := req.RunID
	if runID == "" {
		runID = NewRunID(req.Plan.Workflow.Name, now)
	}
	secrets, err := loadSecrets(req.Plan.Workflow)
	if err != nil {
		return Result{}, err
	}
	handle, err := e.svc.Runs.CreateRun(ctx, corerun.RunMetadata{
		RunID: runID, Workflow: req.Plan.Workflow.Name, WorkflowPath: req.WorkflowSourcePath, StartedAt: now, Tag: req.Tag,
	})
	if err != nil {
		return Result{}, err
	}
	if err := e.svc.Runs.SaveWorkflow(ctx, runID, req.WorkflowSourcePath, req.Plan.Workflow, req.Plan); err != nil {
		return Result{}, err
	}
	if opener, ok := e.svc.Events.(interface{ Open(string) error }); ok {
		if err := opener.Open(filepath.Join(handle.Dir, "events.jsonl")); err != nil {
			return Result{}, err
		}
	}
	state := newExecutionState(runID, req.Plan, req.Inputs, secrets, corerun.NewSecretMasker(secrets), now)
	state.workflowPath = req.WorkflowSourcePath
	state.pause = req.Pause
	state.tag = req.Tag

	destinationDir := req.WorkingDir
	if destinationDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("unable to determine working directory: %w", err)
		}
		destinationDir = wd
	}
	destinationDir = filepath.Clean(destinationDir)
	state.destinationWorkingDir = destinationDir
	state.baseWorkingDir = destinationDir

	if err := e.emitState(ctx, state, corerun.Event{Type: "run.created", Data: map[string]any{"dir": handle.Dir}}); err != nil {
		return Result{}, err
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "run.started"})

	globalLimit := req.Plan.Workflow.Execution.MaxConcurrency
	if globalLimit <= 0 {
		globalLimit = 1
	}
	state.global = semaphore.NewWeighted(int64(globalLimit))
	if err := e.saveCheckpoint(ctx, state, true, 0, "", ""); err != nil {
		return Result{}, err
	}
	if err := e.runHooks(ctx, state, coreworkflow.HookPhaseBeforeRun); err != nil {
		return e.finish(ctx, req.Plan, state, corerun.RunFailed, err)
	}
	if err := e.setupWorktree(ctx, state, req.Plan, destinationDir); err != nil {
		return e.finish(ctx, req.Plan, state, corerun.RunFailed, err)
	}
	if err := e.saveCheckpoint(ctx, state, true, 0, "", ""); err != nil {
		return Result{}, err
	}
	if err := e.executeNodes(ctx, state, req.Plan, 0); err != nil {
		if errors.Is(err, context.Canceled) {
			return e.finish(ctx, req.Plan, state, corerun.RunCancelled, err)
		}
		if errors.Is(err, errRunPaused) {
			return e.finish(ctx, req.Plan, state, corerun.RunPaused, nil)
		}
		return e.finish(ctx, req.Plan, state, corerun.RunFailed, err)
	}
	return e.finish(ctx, req.Plan, state, corerun.RunSuccess, nil)
}

func (e *Executor) setupWorktree(ctx context.Context, state *ExecutionState, plan coreworkflow.ExecutionPlan, destinationDir string) error {
	if !plan.Workflow.Worktree.Enabled {
		return nil
	}
	if e.svc.Worktrees == nil {
		return fmt.Errorf("worktree enabled but no worktree registry configured")
	}
	agentProviderName := plan.Workflow.Worktree.Provider
	if agentProviderName == "" {
		agentProviderName = "codex"
	}
	providerName := internalGitWorktreeProvider
	provider, ok := e.svc.Worktrees.Get(providerName)
	if !ok {
		return fmt.Errorf("worktree git provider %q not available", providerName)
	}
	state.worktreeEnabled = true
	state.worktreeProvider = providerName
	state.worktreeAgentProvider = agentProviderName

	baseCommit, err := e.resolveBaseCommit(ctx, destinationDir, plan.Workflow.Worktree.Base)
	if err != nil {
		return err
	}
	state.worktreeBaseCommit = baseCommit

	wt, err := provider.Create(ctx, coreports.CreateWorktreeRequest{
		WorkflowName: plan.Workflow.Name,
		BaseCommit:   baseCommit,
		WorkingDir:   destinationDir,
		CleanPolicy:  coreports.WorktreeCleanRequire,
	})
	if err != nil {
		return err
	}
	state.worktree = wt
	state.worktreePath = wt.Path
	state.baseWorkingDir = wt.Path

	artifact := map[string]any{
		"enabled":      true,
		"provider":     agentProviderName,
		"git_provider": providerName,
		"name":         wt.Name,
		"path":         wt.Path,
		"base_commit":  baseCommit,
		"destination":  destinationDir,
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal worktree artifact: %w", err)
	}
	maskedData := []byte(state.masker.MaskString(string(data)))
	art := corerun.Artifact{
		ID:           "worktree/status.json",
		Name:         "status.json",
		RelativePath: "worktree/status.json",
		MediaType:    "application/json",
		Kind:         corerun.ArtifactKindCustom,
	}
	if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, maskedData); err != nil {
		return fmt.Errorf("failed to save worktree status artifact: %w", err)
	}

	eventData := map[string]any{
		"enabled":      true,
		"provider":     agentProviderName,
		"git_provider": providerName,
		"name":         wt.Name,
		"path":         wt.Path,
		"base_commit":  baseCommit,
		"destination":  destinationDir,
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "worktree.created", Data: eventData})
	return nil
}

func (e *Executor) resolveBaseCommit(ctx context.Context, workingDir string, base string) (string, error) {
	if base != "current" {
		return base, nil
	}
	res, err := e.svc.Shell.Run(ctx, coreports.ShellRequest{
		Command:    "git rev-parse HEAD",
		WorkingDir: workingDir,
	})
	if err != nil || res.ExitCode != 0 {
		return "", fmt.Errorf("unable to resolve current HEAD: %w", err)
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (e *Executor) restoreWorktree(ctx context.Context, state *ExecutionState, checkpoint corerun.Checkpoint) error {
	if checkpoint.Worktree == nil || !checkpoint.Worktree.Enabled {
		return fmt.Errorf("checkpoint for worktree-enabled workflow is missing worktree state")
	}
	wtCheckpoint := checkpoint.Worktree
	if e.svc.Worktrees == nil {
		return fmt.Errorf("worktree enabled but no worktree registry configured")
	}
	providerName := wtCheckpoint.Provider
	if providerName == "" || providerName == "pi" {
		providerName = internalGitWorktreeProvider
	}
	provider, ok := e.svc.Worktrees.Get(providerName)
	if !ok {
		return fmt.Errorf("worktree git provider %q not available", providerName)
	}
	agentProviderName := wtCheckpoint.AgentProvider
	if agentProviderName == "" {
		agentProviderName = checkpoint.Workflow.Worktree.Provider
	}
	if agentProviderName == "" {
		agentProviderName = "codex"
	}
	if wtCheckpoint.Path == "" {
		return fmt.Errorf("checkpoint worktree path is empty")
	}
	info, err := os.Stat(wtCheckpoint.Path)
	if err != nil {
		return fmt.Errorf("checkpoint worktree path %q is not usable: %w", wtCheckpoint.Path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("checkpoint worktree path %q is not a directory", wtCheckpoint.Path)
	}
	if wtCheckpoint.DestinationWorkingDir == "" {
		return fmt.Errorf("checkpoint worktree destination is empty")
	}
	currentHead := e.currentGitHead(ctx, wtCheckpoint.DestinationWorkingDir)
	if currentHead == "" {
		return fmt.Errorf("unable to resolve destination HEAD for %q", wtCheckpoint.DestinationWorkingDir)
	}
	if currentHead != wtCheckpoint.BaseCommit {
		_ = e.emitState(ctx, state, corerun.Event{
			Type: "worktree.resume_drift_detected",
			Data: map[string]any{
				"base_commit":      wtCheckpoint.BaseCommit,
				"destination_head": currentHead,
				"destination":      wtCheckpoint.DestinationWorkingDir,
			},
		})
	}

	wt := coreports.Worktree{
		ID:           wtCheckpoint.ID,
		Name:         wtCheckpoint.Name,
		Path:         wtCheckpoint.Path,
		Branch:       wtCheckpoint.Branch,
		BaseCommit:   wtCheckpoint.BaseCommit,
		WorkflowName: wtCheckpoint.WorkflowName,
	}
	if wt.WorkflowName == "" {
		wt.WorkflowName = checkpoint.Workflow.Name
	}
	if wt.BaseCommit == "" {
		wt.BaseCommit = wtCheckpoint.BaseCommit
	}
	if _, err := provider.Status(ctx, wt); err != nil {
		return fmt.Errorf("worktree status failed during resume: %w", err)
	}

	state.worktreeEnabled = true
	state.worktreeProvider = providerName
	state.worktreeAgentProvider = agentProviderName
	state.worktree = wt
	state.worktreePath = wt.Path
	state.worktreeBaseCommit = wtCheckpoint.BaseCommit
	state.destinationWorkingDir = wtCheckpoint.DestinationWorkingDir
	state.baseWorkingDir = wt.Path
	return nil
}

func (e *Executor) resume(ctx context.Context, req ExecutionRequest) (Result, error) {
	checkpoint, err := e.svc.Runs.LoadCheckpoint(ctx, req.ResumeRunID)
	if err != nil {
		return Result{}, err
	}
	plan, err := coreworkflow.BuildPlan(checkpoint.Workflow)
	if err != nil {
		return Result{}, err
	}
	secrets, err := loadSecrets(checkpoint.Workflow)
	if err != nil {
		return Result{}, err
	}
	tag := req.Tag
	if tag == "" {
		tag = checkpoint.Tag
	}
	handle, err := e.svc.Runs.CreateRun(ctx, corerun.RunMetadata{
		RunID: checkpoint.RunID, Workflow: checkpoint.Workflow.Name, WorkflowPath: checkpoint.WorkflowPath, StartedAt: checkpoint.StartedAt, Tag: tag,
	})
	if err != nil {
		return Result{}, err
	}
	if opener, ok := e.svc.Events.(interface{ Open(string) error }); ok {
		if err := opener.Open(filepath.Join(handle.Dir, "events.jsonl")); err != nil {
			return Result{}, err
		}
	}
	state := newExecutionState(checkpoint.RunID, plan, checkpoint.Inputs, secrets, corerun.NewSecretMasker(secrets), checkpoint.StartedAt)
	state.baseWorkingDir = req.WorkingDir
	state.workflowPath = checkpoint.WorkflowPath
	state.pause = req.Pause
	state.tag = tag
	state.restoreMetrics(checkpoint.Metrics)
	if checkpoint.Workflow.Worktree.Enabled {
		if err := e.restoreWorktree(ctx, state, checkpoint); err != nil {
			return Result{}, err
		}
	}
	for id, result := range checkpoint.Nodes {
		state.set(id, result)
	}
	globalLimit := plan.Workflow.Execution.MaxConcurrency
	if globalLimit <= 0 {
		globalLimit = 1
	}
	state.global = semaphore.NewWeighted(int64(globalLimit))
	startCursor := checkpoint.Cursor
	if checkpoint.RetryNodeID != "" {
		for i, nodeID := range plan.Order {
			if nodeID == checkpoint.RetryNodeID {
				startCursor = i
				break
			}
		}
		delete(state.results, checkpoint.RetryNodeID)
		delete(state.nodes, checkpoint.RetryNodeID)
	}
	resumeData := map[string]any{"cursor": startCursor, "retry_node_id": checkpoint.RetryNodeID, "reason": checkpoint.Reason}
	if state.worktreeEnabled {
		resumeData["worktree_path"] = state.worktreePath
		resumeData["worktree_provider"] = state.worktreeAgentProvider
		resumeData["worktree_git_provider"] = state.worktreeProvider
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "run.resumed", Data: resumeData})
	if err := e.saveCheckpoint(ctx, state, true, startCursor, "", ""); err != nil {
		return Result{}, err
	}
	if err := e.executeNodes(ctx, state, plan, startCursor); err != nil {
		if errors.Is(err, context.Canceled) {
			return e.finish(ctx, plan, state, corerun.RunCancelled, err)
		}
		if errors.Is(err, errRunPaused) {
			return e.finish(ctx, plan, state, corerun.RunPaused, nil)
		}
		return e.finish(ctx, plan, state, corerun.RunFailed, err)
	}
	return e.finish(ctx, plan, state, corerun.RunSuccess, nil)
}

func NewRunID(workflowName string, now time.Time) string {
	_ = workflowName
	bucket := now.UTC().Unix() / 600
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	encoded := make([]byte, 5)
	for i := len(encoded) - 1; i >= 0; i-- {
		encoded[i] = alphabet[bucket%36]
		bucket /= 36
	}
	salt := alphabet[rand.New(rand.NewSource(now.UnixNano())).Intn(36)]
	return fmt.Sprintf("%s%c", string(encoded), salt)
}

func (e *Executor) executeNodes(ctx context.Context, state *ExecutionState, plan coreworkflow.ExecutionPlan, startCursor int) error {
	indexByID := make(map[string]int, len(plan.Order))
	for i, nodeID := range plan.Order {
		indexByID[nodeID] = i
	}
	if startCursor < 0 || startCursor > len(plan.Order) {
		return fmt.Errorf("checkpoint cursor %d is outside execution plan", startCursor)
	}
	for pc := startCursor; pc < len(plan.Order); {
		if err := ctx.Err(); err != nil {
			return err
		}
		if state.parent == nil && state.pause != nil && state.pause.Requested() {
			return e.pauseManual(ctx, state, pc)
		}
		state.cursor = pc
		if err := e.saveCheckpoint(ctx, state, true, pc, "", ""); err != nil {
			return err
		}
		nodeID := plan.Order[pc]
		node := plan.Nodes[nodeID].Spec
		if state.failed && !node.ContinueOnError {
			state.set(nodeID, corerun.NodeResult{RunID: state.runID, NodeID: nodeID, Status: corerun.NodeSkipped, Error: "run already failed", Path: append([]string(nil), state.path...)})
			pc++
			_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
			continue
		}
		if err := state.dependenciesReady(node); err != nil {
			state.set(nodeID, corerun.NodeResult{RunID: state.runID, NodeID: nodeID, Status: corerun.NodeSkipped, Error: err.Error(), Path: append([]string(nil), state.path...)})
			pc++
			_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
			continue
		}
		ok, err := coreworkflow.EvalBool(node.When, state.evalContext(state.index, state.total, state.item))
		if err != nil {
			result := corerun.NodeResult{RunID: state.runID, NodeID: nodeID, Status: corerun.NodeFailed, Error: err.Error(), Path: append([]string(nil), state.path...)}
			e.recordNode(ctx, state, node, result)
			if !node.ContinueOnError {
				if plan.Workflow.Execution.PauseWhenFail {
					return e.pauseOnFailure(ctx, state, pc, nodeID)
				}
				return fmt.Errorf("node %q when failed: %w", nodeID, err)
			}
			pc++
			_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
			continue
		}
		if !ok {
			result := corerun.NodeResult{RunID: state.runID, NodeID: nodeID, Status: corerun.NodeSkipped, Path: append([]string(nil), state.path...)}
			e.recordNode(ctx, state, node, result)
			_ = e.emitState(ctx, state, corerun.Event{Type: "node.skipped", NodeID: nodeID})
			pc++
			_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
			continue
		}
		_ = e.emitState(ctx, state, corerun.Event{Type: "node.ready", NodeID: nodeID})
		result := e.executeNode(ctx, state, node)
		e.recordNode(ctx, state, node, result)
		if isFailure(result.Status) && !node.ContinueOnError {
			if plan.Workflow.Execution.PauseWhenFail {
				return e.pauseOnFailure(ctx, state, pc, nodeID)
			}
			return fmt.Errorf("node %q failed: %s", nodeID, result.Error)
		}
		if node.GoToIf != nil {
			jump, jumpErr := e.resolveGoToIf(node, state, indexByID)
			if jumpErr != nil {
				failedResult := result
				failedResult.Status = corerun.NodeFailed
				failedResult.Error = jumpErr.Error()
				e.recordNode(ctx, state, node, failedResult)
				if !node.ContinueOnError {
					if plan.Workflow.Execution.PauseWhenFail {
						return e.pauseOnFailure(ctx, state, pc, nodeID)
					}
					return fmt.Errorf("node %q go_to_if failed: %w", nodeID, jumpErr)
				}
				pc++
				_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
				continue
			}
			if jump >= 0 {
				pc = jump
				_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
				continue
			}
		}
		pc++
		_ = e.saveCheckpoint(ctx, state, true, pc, "", "")
	}
	return nil
}

func (e *Executor) pauseOnFailure(ctx context.Context, state *ExecutionState, cursor int, nodeID string) error {
	_ = e.emitState(ctx, state, corerun.Event{Type: "run.pausing", NodeID: nodeID, Data: map[string]any{"reason": corerun.PauseReasonPauseWhenFail}})
	if err := e.saveCheckpoint(ctx, state, true, cursor, nodeID, corerun.PauseReasonPauseWhenFail); err != nil {
		return err
	}
	return errRunPaused
}

func (e *Executor) pauseManual(ctx context.Context, state *ExecutionState, cursor int) error {
	_ = e.emitState(ctx, state, corerun.Event{Type: "run.pausing", Data: map[string]any{"reason": corerun.PauseReasonManual, "cursor": cursor}})
	if err := e.saveCheckpoint(ctx, state, true, cursor, "", corerun.PauseReasonManual); err != nil {
		return err
	}
	return errRunPaused
}

func (e *Executor) resolveGoToIf(node coreworkflow.NodeSpec, state *ExecutionState, indexByID map[string]int) (int, error) {
	if node.GoToIf == nil {
		return -1, nil
	}
	ok, err := coreworkflow.EvalBool(node.GoToIf.When, state.evalContext(state.index, state.total, state.item))
	if err != nil {
		return -1, err
	}
	if !ok {
		return -1, nil
	}
	target, ok := indexByID[node.GoToIf.Target]
	if !ok {
		return -1, fmt.Errorf("unknown go_to_if target %q", node.GoToIf.Target)
	}
	return target, nil
}

func (e *Executor) executeNode(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec) corerun.NodeResult {
	if node.Kind == coreworkflow.NodeKindMap {
		return e.executeMap(ctx, state, node)
	}
	if node.ForEach != "" {
		return e.executeExpanded(ctx, state, node)
	}
	return e.executeSingle(ctx, state, node, "", nil, nil, nil)
}

type fanOutRunner func(ctx context.Context, index int, item any, instanceID string) corerun.NodeResult

func (e *Executor) executeFanOut(
	ctx context.Context,
	state *ExecutionState,
	node coreworkflow.NodeSpec,
	items []any,
	localConcurrency int,
	failFast bool,
	cancelResultPath []string,
	finalPath []string,
	instanceEventPath func(index int, instanceID string) []string,
	runItem fanOutRunner,
) corerun.NodeResult {
	local := semaphore.NewWeighted(int64(localConcurrency))
	results := make([]corerun.NodeResult, len(items))
	var wg sync.WaitGroup
	var mu sync.Mutex
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, item := range items {
		instanceID := fmt.Sprintf("%04d", i)
		if failFast && cancelCtx.Err() != nil {
			results[i] = corerun.NodeResult{RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Status: corerun.NodeCancelled, Error: cancelCtx.Err().Error(), Path: append([]string(nil), cancelResultPath...)}
			continue
		}
		if err := local.Acquire(cancelCtx, 1); err != nil {
			results[i] = corerun.NodeResult{RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Status: corerun.NodeCancelled, Error: err.Error(), Path: append([]string(nil), cancelResultPath...)}
			continue
		}
		wg.Add(1)
		go func(index int, item any) {
			defer wg.Done()
			defer local.Release(1)
			instanceID := fmt.Sprintf("%04d", index)
			startedEvent := corerun.Event{Type: "node.instance.started", NodeID: node.ID, InstanceID: instanceID}
			if instanceEventPath != nil {
				startedEvent.Path = instanceEventPath(index, instanceID)
			}
			_ = e.emitState(cancelCtx, state, startedEvent)
			result := runItem(cancelCtx, index, item, instanceID)
			completedEvent := corerun.Event{Type: eventForResult("node.instance.completed", "node.instance.failed", result.Status), NodeID: node.ID, InstanceID: instanceID}
			if instanceEventPath != nil {
				completedEvent.Path = instanceEventPath(index, instanceID)
			}
			_ = e.emitState(cancelCtx, state, completedEvent)
			mu.Lock()
			results[index] = result
			if failFast && isFailure(result.Status) {
				cancel()
			}
			mu.Unlock()
		}(i, item)
	}
	wg.Wait()

	outputs := make([]any, len(results))
	status := corerun.NodeSuccess
	var errs []string
	for i, result := range results {
		outputs[i] = result.Output
		if isFailure(result.Status) {
			status = result.Status
			errs = append(errs, result.Error)
		}
		result.Artifacts = e.saveNodeArtifacts(ctx, state, node, result)
		results[i] = result
		state.set(result.NodeID, result)
		_ = e.svc.Runs.SaveNodeResult(ctx, state.runID, state.masker.MaskNodeResult(result))
	}
	finalResult := corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: status, Outputs: outputs, Error: strings.Join(errs, "; ")}
	if finalPath != nil {
		finalResult.Path = append([]string(nil), finalPath...)
	}
	e.recordNode(ctx, state, node, finalResult)
	return finalResult
}

func (e *Executor) executeMap(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec) corerun.NodeResult {
	items, err := e.forEachItems(ctx, state, node)
	if err != nil {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeFailed, Error: err.Error()}
	}
	if node.MaxItems > 0 && len(items) > node.MaxItems {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeFailed, Error: fmt.Sprintf("for_each produced %d items, max_items is %d", len(items), node.MaxItems)}
	}
	localConcurrency := node.Concurrency
	if localConcurrency <= 0 {
		localConcurrency = len(items)
	}
	failFast := state.failFast(node)
	expansionData := map[string]any{
		"items":       len(items),
		"concurrency": localConcurrency,
		"fail_fast":   failFast,
	}
	if node.MaxItems > 0 {
		expansionData["max_items"] = node.MaxItems
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "node.expanded", NodeID: node.ID, Data: expansionData})
	childPlan := state.plan.Nodes[node.ID].ChildPlan
	if childPlan == nil {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeFailed, Error: "map node is missing nested plan", Path: append([]string(nil), state.path...)}
	}
	if len(items) == 0 {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeSuccess, Outputs: []any{}, Path: append([]string(nil), state.path...)}
	}
	return e.executeFanOut(
		ctx,
		state,
		node,
		items,
		localConcurrency,
		failFast,
		appendPath(state.path, node.ID),
		append([]string(nil), state.path...),
		func(index int, instanceID string) []string {
			return appendPath(state.path, node.ID, instanceID)
		},
		func(ctx context.Context, index int, item any, instanceID string) corerun.NodeResult {
			total := len(items)
			itemPath := appendPath(state.path, node.ID, instanceID)
			childState := state.spawn(*childPlan, itemPath)
			childState.item = item
			childState.index = &index
			childState.total = &total
			err := e.executeNodes(ctx, childState, *childPlan, 0)
			result := corerun.NodeResult{RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Index: &index, Path: appendPath(state.path, node.ID)}
			if err != nil {
				result.Status = corerun.NodeFailed
				result.Error = err.Error()
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					result.Status = corerun.NodeTimeout
				}
			} else {
				result.Status = corerun.NodeSuccess
			}
			if len(childPlan.Order) > 0 {
				last := childPlan.Order[len(childPlan.Order)-1]
				if childResult, ok := childState.results[last]; ok {
					result.Output = resultValue(childResult)
				}
			}
			return result
		},
	)
}

func (e *Executor) executeExpanded(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec) corerun.NodeResult {
	items, err := e.forEachItems(ctx, state, node)
	if err != nil {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeFailed, Error: err.Error()}
	}
	if node.MaxItems > 0 && len(items) > node.MaxItems {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeFailed, Error: fmt.Sprintf("for_each produced %d items, max_items is %d", len(items), node.MaxItems)}
	}
	localConcurrency := node.Concurrency
	if localConcurrency <= 0 {
		localConcurrency = len(items)
	}
	failFast := state.failFast(node)
	expansionData := map[string]any{
		"items":       len(items),
		"concurrency": localConcurrency,
		"fail_fast":   failFast,
	}
	if node.MaxItems > 0 {
		expansionData["max_items"] = node.MaxItems
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: "node.expanded", NodeID: node.ID, Data: expansionData})
	if len(items) == 0 {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, Status: corerun.NodeSuccess, Outputs: []any{}}
	}
	return e.executeFanOut(
		ctx,
		state,
		node,
		items,
		localConcurrency,
		failFast,
		append([]string(nil), state.path...),
		nil,
		nil,
		func(ctx context.Context, index int, item any, instanceID string) corerun.NodeResult {
			total := len(items)
			return e.executeSingle(ctx, state, node, instanceID, &index, &total, item)
		},
	)
}

func (e *Executor) executeSingle(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec, instanceID string, index *int, total *int, item any) corerun.NodeResult {
	attempts := 1 + effectiveRetries(state.plan.Workflow, node)
	var last corerun.NodeResult
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * 250 * time.Millisecond
			state.incrementRetries()
			_ = e.emitState(ctx, state, corerun.Event{
				Type:       "node.retrying",
				NodeID:     node.ID,
				InstanceID: instanceID,
				Attempt:    attempt,
				Data: map[string]any{
					"attempt":         attempt,
					"max_attempts":    attempts,
					"retry":           attempt - 1,
					"max_retries":     attempts - 1,
					"delay_ms":        delay.Milliseconds(),
					"previous_status": last.Status,
					"previous_error":  last.Error,
				},
			})
			_ = e.saveCheckpoint(ctx, state, true, state.cursor, "", "")
			time.Sleep(delay)
		}
		last = e.executeAttempt(ctx, state, node, instanceID, index, total, item, attempt)
		last.Attempts = attempt
		if !isFailure(last.Status) {
			return last
		}
	}
	return last
}

func (e *Executor) executeAttempt(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec, instanceID string, index *int, total *int, item any, attempt int) corerun.NodeResult {
	timeout := effectiveTimeout(state.plan.Workflow, node)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}
	if err := state.global.Acquire(ctx, 1); err != nil {
		return corerun.NodeResult{RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Index: index, Status: corerun.NodeCancelled, Error: err.Error()}
	}
	defer state.global.Release(1)

	start := e.now()
	eventType := "node.started"
	if instanceID != "" {
		eventType = "node.instance.started"
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: eventType, NodeID: node.ID, InstanceID: instanceID, Attempt: attempt})
	result := corerun.NodeResult{RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Index: index, Path: append([]string(nil), state.path...)}
	output, status, err := e.dispatch(ctx, state, node, instanceID, index, total, item, attempt)
	result.Duration = e.now().Sub(start)
	result.Status = status
	result.Output = output.Output
	result.Stdout = output.Stdout
	result.Stderr = output.Stderr
	result.ExitCode = output.ExitCode
	if err == nil && status == corerun.NodeSuccess && len(node.Outputs) > 0 {
		declaredOutputs, outputErr := materializeDeclaredOutputs(node, output.Output)
		if outputErr != nil {
			err = outputErr
			status = corerun.NodeFailed
		} else {
			result.DeclaredOutputs = declaredOutputs
		}
	}
	if err != nil {
		result.Status = status
		result.Error = err.Error()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Status = corerun.NodeTimeout
		} else if errors.Is(ctx.Err(), context.Canceled) {
			result.Status = corerun.NodeCancelled
		}
	}
	_ = e.emitState(ctx, state, corerun.Event{Type: eventForResult("node.completed", "node.failed", result.Status), NodeID: node.ID, InstanceID: instanceID, Attempt: attempt})
	return result
}
