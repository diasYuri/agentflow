package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

func (e *Executor) finalizeWorktree(ctx context.Context, state *ExecutionState) (corerun.WorktreeMetadata, error) {
	meta := e.initialWorktreeMetadata(ctx, state)
	provider, ok := e.svc.Worktrees.Get(state.worktreeProvider)
	if !ok {
		meta.MergeStatus = corerun.WorktreeMergeFailed
		_ = e.saveWorktreeStatus(ctx, state, meta)
		return meta, fmt.Errorf("worktree provider %q not available during finalize", state.worktreeProvider)
	}

	// 1. Status
	wtStatus, err := provider.Status(ctx, state.worktree)
	if err != nil {
		meta.MergeStatus = corerun.WorktreeMergeFailed
		meta.Commands = appendGitCommands(meta.Commands, wtStatus.Commands)
		_ = e.saveWorktreeStatus(ctx, state, meta)
		return meta, fmt.Errorf("worktree status failed: %w", err)
	}
	meta.Commands = appendGitCommands(meta.Commands, wtStatus.Commands)

	if wtStatus.Clean {
		meta.MergeStatus = corerun.WorktreeMergeNoChanges
		_ = e.saveWorktreeStatus(ctx, state, meta)
		return meta, nil
	}

	// 2. Diff
	changeSet, err := provider.Diff(ctx, state.worktree)
	if err != nil {
		meta.MergeStatus = corerun.WorktreeMergeFailed
		meta.Commands = appendGitCommands(meta.Commands, changeSet.Commands)
		_ = e.saveWorktreeStatus(ctx, state, meta)
		return meta, fmt.Errorf("worktree diff failed: %w", err)
	}
	meta.Commands = appendGitCommands(meta.Commands, changeSet.Commands)
	meta.ChangedFiles = toSortedChangedFiles(changeSet.Files)

	if changeSet.Empty {
		meta.MergeStatus = corerun.WorktreeMergeNoChanges
		_ = e.saveWorktreeStatus(ctx, state, meta)
		return meta, nil
	}

	if err := e.saveArtifact(ctx, state, "worktree/diff.patch", []byte(changeSet.Diff)); err != nil {
		return meta, fmt.Errorf("failed to save diff.patch: %w", err)
	}

	// 3. Apply
	mergeResult, err := provider.Apply(ctx, coreports.ApplyWorktreeRequest{
		Worktree:   state.worktree,
		TargetDir:  state.destinationWorkingDir,
		BaseCommit: state.worktreeBaseCommit,
		Diff:       changeSet.Diff,
	})
	meta.Commands = appendGitCommands(meta.Commands, mergeResult.Commands)

	if err != nil {
		// Classify error
		if errors.Is(err, coreports.ErrWorktreeStructural) {
			meta.MergeStatus = corerun.WorktreeMergeFailed
			meta.Conflicts = toWorktreeConflicts(mergeResult.Conflicts)
			_ = e.saveWorktreeStatus(ctx, state, meta)
			_ = e.saveConflictsArtifact(ctx, state, meta)
			return meta, fmt.Errorf("worktree apply structural error: %w", err)
		}
		// Resolvable conflict or unknown -> attempt agent resolution if configured
		meta.MergeStatus = corerun.WorktreeMergeConflict
		meta.Conflicts = toWorktreeConflicts(mergeResult.Conflicts)
		_ = e.saveWorktreeStatus(ctx, state, meta)
		_ = e.saveConflictsArtifact(ctx, state, meta)

		if state.plan.Workflow.Worktree.Merge.OnConflict == "agent" {
			resolved, resolveErr := e.requestConflictResolution(ctx, state, meta, changeSet)
			if resolveErr != nil {
				meta.AgentResolutionError = resolveErr.Error()
				_ = e.saveWorktreeStatus(ctx, state, meta)
				return meta, fmt.Errorf("worktree conflict resolution failed: %w", resolveErr)
			}
			meta = resolved
		}
		return meta, nil
	}

	if mergeResult.Success {
		meta.MergeStatus = corerun.WorktreeMergeMerged
		meta.DestinationCommitAfter = e.currentGitHead(ctx, state.destinationWorkingDir)
		_ = e.saveMergeLogArtifact(ctx, state, meta)
		_ = e.emitState(ctx, state, corerun.Event{Type: "worktree.merged", Data: map[string]any{
			"changed_files": len(meta.ChangedFiles),
		}})
	} else {
		meta.MergeStatus = corerun.WorktreeMergeConflict
		meta.Conflicts = toWorktreeConflicts(mergeResult.Conflicts)
		_ = e.saveConflictsArtifact(ctx, state, meta)
		if state.plan.Workflow.Worktree.Merge.OnConflict == "agent" {
			resolved, resolveErr := e.requestConflictResolution(ctx, state, meta, changeSet)
			if resolveErr != nil {
				meta.AgentResolutionError = resolveErr.Error()
				_ = e.saveWorktreeStatus(ctx, state, meta)
				return meta, fmt.Errorf("worktree conflict resolution failed: %w", resolveErr)
			}
			meta = resolved
		}
	}

	_ = e.saveWorktreeStatus(ctx, state, meta)
	return meta, nil
}

func (e *Executor) requestConflictResolution(ctx context.Context, state *ExecutionState, meta corerun.WorktreeMetadata, changeSet coreports.ChangeSet) (corerun.WorktreeMetadata, error) {
	providerName := "pi"
	if !e.svc.Agents.HasProvider(providerName) {
		providerName = "codex"
	}
	provider, ok := e.svc.Agents.Get(providerName)
	if !ok {
		return meta, fmt.Errorf("no agent provider available for conflict resolution")
	}

	prompt := buildConflictResolutionPrompt(state, meta, changeSet)
	_ = e.emitState(ctx, state, corerun.Event{Type: "worktree.resolution_agent.requested", Data: map[string]any{
		"provider": providerName,
		"files":    len(meta.Conflicts),
	}})

	_, err := provider.Run(ctx, coreports.AgentRequest{
		RunID:      state.runID,
		NodeID:     "worktree-resolution",
		Provider:   providerName,
		Prompt:     prompt,
		WorkingDir: state.destinationWorkingDir,
		Sandbox:    coreworkflow.SandboxSpec{Mode: "workspace-write"},
	})
	if err != nil {
		return meta, err
	}

	// Re-evaluate status after agent attempted resolution
	wtProvider, _ := e.svc.Worktrees.Get(state.worktreeProvider)
	if wtProvider == nil {
		meta.MergeStatus = corerun.WorktreeMergeFailed
		return meta, fmt.Errorf("worktree provider unavailable after resolution")
	}

	wtStatus, statusErr := wtProvider.Status(ctx, state.worktree)
	if statusErr != nil {
		meta.MergeStatus = corerun.WorktreeMergeFailed
		return meta, fmt.Errorf("status check after resolution failed: %w", statusErr)
	}

	if wtStatus.Clean {
		meta.MergeStatus = corerun.WorktreeMergeNoChanges
		return meta, nil
	}

	// Re-apply after resolution
	mergeResult, applyErr := wtProvider.Apply(ctx, coreports.ApplyWorktreeRequest{
		Worktree:   state.worktree,
		TargetDir:  state.destinationWorkingDir,
		BaseCommit: state.worktreeBaseCommit,
		Diff:       changeSet.Diff,
	})
	meta.Commands = appendGitCommands(meta.Commands, mergeResult.Commands)

	if applyErr != nil {
		if errors.Is(applyErr, coreports.ErrWorktreeStructural) {
			meta.MergeStatus = corerun.WorktreeMergeFailed
		} else {
			meta.MergeStatus = corerun.WorktreeMergeConflict
		}
		meta.Conflicts = toWorktreeConflicts(mergeResult.Conflicts)
		return meta, applyErr
	}

	if mergeResult.Success {
		meta.MergeStatus = corerun.WorktreeMergeMerged
		meta.Conflicts = nil
		meta.DestinationCommitAfter = e.currentGitHead(ctx, state.destinationWorkingDir)
	} else {
		meta.MergeStatus = corerun.WorktreeMergeConflict
		meta.Conflicts = toWorktreeConflicts(mergeResult.Conflicts)
	}
	return meta, nil
}

func (e *Executor) initialWorktreeMetadata(ctx context.Context, state *ExecutionState) corerun.WorktreeMetadata {
	meta := corerun.WorktreeMetadata{
		Enabled:                 true,
		Provider:                state.worktreeProvider,
		Name:                    state.worktree.Name,
		BaseCommit:              state.worktreeBaseCommit,
		DestinationCommitBefore: e.currentGitHead(ctx, state.destinationWorkingDir),
		WorktreePath:            state.worktreePath,
		Destination:             state.destinationWorkingDir,
	}
	if meta.Name == "" {
		meta.Name = state.plan.Workflow.Name
	}
	return meta
}

func (e *Executor) currentGitHead(ctx context.Context, workingDir string) string {
	if e.svc.Shell == nil || workingDir == "" {
		return ""
	}
	res, err := e.svc.Shell.Run(ctx, coreports.ShellRequest{
		Command:    "git rev-parse HEAD",
		WorkingDir: workingDir,
	})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(res.Stdout)
}

func buildConflictResolutionPrompt(state *ExecutionState, meta corerun.WorktreeMetadata, changeSet coreports.ChangeSet) string {
	var b strings.Builder
	b.WriteString("Resolve worktree merge conflicts for workflow ")
	b.WriteString(state.plan.Workflow.Name)
	b.WriteString(".\n\nBase commit: ")
	b.WriteString(meta.BaseCommit)
	b.WriteString("\nDestination: ")
	b.WriteString(meta.Destination)
	b.WriteString("\nWorktree: ")
	b.WriteString(meta.WorktreePath)
	b.WriteString("\n\nChanged files:\n")
	for _, f := range meta.ChangedFiles {
		b.WriteString("- ")
		b.WriteString(f.Path)
		b.WriteString(" (")
		b.WriteString(f.Status)
		b.WriteString(")\n")
	}
	if len(meta.Conflicts) > 0 {
		b.WriteString("\nConflicts:\n")
		for _, c := range meta.Conflicts {
			b.WriteString("- ")
			b.WriteString(c.Path)
			b.WriteString(": ")
			b.WriteString(c.Reason)
			b.WriteString("\n")
		}
	}
	b.WriteString("\nApply the changes from the worktree to the destination and resolve any conflicts.")
	return b.String()
}

func (e *Executor) cleanupWorktree(ctx context.Context, state *ExecutionState, meta *corerun.WorktreeMetadata, runStatus corerun.RunStatus) {
	provider, ok := e.svc.Worktrees.Get(state.worktreeProvider)
	if !ok {
		return
	}

	shouldRemove := false
	switch runStatus {
	case corerun.RunSuccess:
		if state.plan.Workflow.Worktree.Cleanup.OnSuccess != nil && *state.plan.Workflow.Worktree.Cleanup.OnSuccess {
			shouldRemove = true
		}
		// no_changes also removes when on_success is true
		if meta.MergeStatus == corerun.WorktreeMergeNoChanges && state.plan.Workflow.Worktree.Cleanup.OnSuccess != nil && *state.plan.Workflow.Worktree.Cleanup.OnSuccess {
			shouldRemove = true
		}
	case corerun.RunFailed, corerun.RunCancelled, corerun.RunPaused:
		if state.plan.Workflow.Worktree.Cleanup.OnFailure == "cleanup" {
			shouldRemove = true
		}
	}

	if shouldRemove {
		// Preserve worktree when merge ended in conflict or failure so user can inspect.
		if runStatus == corerun.RunSuccess && (meta.MergeStatus == corerun.WorktreeMergeConflict || meta.MergeStatus == corerun.WorktreeMergeFailed) {
			meta.CleanupStatus = corerun.WorktreeCleanupKept
			return
		}
		res, _ := provider.Cleanup(ctx, coreports.CleanupWorktreeRequest{Worktree: state.worktree})
		if res.Removed {
			meta.CleanupStatus = corerun.WorktreeCleanupRemoved
		} else {
			meta.CleanupStatus = corerun.WorktreeCleanupKept
		}
	} else {
		meta.CleanupStatus = corerun.WorktreeCleanupKept
	}
}

func (e *Executor) saveWorktreeStatus(ctx context.Context, state *ExecutionState, meta corerun.WorktreeMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	masked := []byte(state.masker.MaskString(string(data)))
	return e.svc.Runs.SaveArtifact(ctx, state.runID, "worktree/status.json", masked)
}

func (e *Executor) saveArtifact(ctx context.Context, state *ExecutionState, name string, data []byte) error {
	masked := []byte(state.masker.MaskString(string(data)))
	return e.svc.Runs.SaveArtifact(ctx, state.runID, name, masked)
}

func (e *Executor) saveConflictsArtifact(ctx context.Context, state *ExecutionState, meta corerun.WorktreeMetadata) error {
	artifact := map[string]any{
		"files":                           meta.Conflicts,
		"base_commit":                     meta.BaseCommit,
		"destination_commit_before_merge": meta.DestinationCommitBefore,
		"destination_commit_after_merge":  meta.DestinationCommitAfter,
		"destination":                     meta.Destination,
		"worktree_path":                   meta.WorktreePath,
		"changed_files":                   meta.ChangedFiles,
		"commands":                        meta.Commands,
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return e.saveArtifact(ctx, state, "worktree/conflicts.json", data)
}

func (e *Executor) saveMergeLogArtifact(ctx context.Context, state *ExecutionState, meta corerun.WorktreeMetadata) error {
	log := map[string]any{
		"merge_status":  meta.MergeStatus,
		"changed_files": meta.ChangedFiles,
		"commands":      meta.Commands,
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return err
	}
	return e.saveArtifact(ctx, state, "worktree/merge.log", data)
}

func toSortedChangedFiles(files []coreports.FileChange) []corerun.WorktreeChangedFile {
	out := make([]corerun.WorktreeChangedFile, len(files))
	for i, f := range files {
		out[i] = corerun.WorktreeChangedFile{
			Path:    f.Path,
			Status:  f.Status,
			OldPath: f.OldPath,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func toWorktreeConflicts(conflicts []coreports.Conflict) []corerun.WorktreeConflict {
	out := make([]corerun.WorktreeConflict, len(conflicts))
	for i, c := range conflicts {
		out[i] = corerun.WorktreeConflict{Path: c.Path, Reason: c.Reason}
	}
	return out
}

func appendGitCommands(dst []corerun.WorktreeGitCommand, src []coreports.GitCommand) []corerun.WorktreeGitCommand {
	for _, c := range src {
		dst = append(dst, corerun.WorktreeGitCommand{
			Command:  c.Command,
			ExitCode: c.ExitCode,
			Stdout:   c.Stdout,
			Stderr:   c.Stderr,
		})
	}
	return dst
}
