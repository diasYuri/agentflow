package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

func (e *Executor) runHooks(ctx context.Context, state *ExecutionState, phase coreworkflow.HookPhase) error {
	for i, hook := range state.plan.Workflow.Hooks {
		if coreworkflow.HookPhase(hook.Phase) != phase {
			continue
		}
		if err := e.runHook(ctx, state, hook, phase, i); err != nil {
			return fmt.Errorf("hook %s[%d] failed: %w", phase, i, err)
		}
	}
	return nil
}

func (e *Executor) runHook(ctx context.Context, state *ExecutionState, hook coreworkflow.HookSpec, phase coreworkflow.HookPhase, index int) error {
	evalCtx := state.evalContext(nil, nil, nil)

	command, err := coreworkflow.RenderTemplate(hook.Command, evalCtx)
	if err != nil {
		return fmt.Errorf("render command: %w", err)
	}

	workingDir := resolvePath(state.baseWorkingDir, hook.WorkingDir)
	if workingDir == "" {
		workingDir = state.baseWorkingDir
	}

	env := make(map[string]string, len(hook.Env))
	for k, v := range hook.Env {
		rendered, rerr := coreworkflow.RenderTemplate(v, evalCtx)
		if rerr != nil {
			return fmt.Errorf("render env.%s: %w", k, rerr)
		}
		env[k] = rendered
	}

	hookCtx := ctx
	if hook.Timeout > 0 {
		var cancel context.CancelFunc
		hookCtx, cancel = context.WithTimeout(ctx, time.Duration(hook.Timeout)*time.Second)
		defer cancel()
	}

	_ = e.emitState(hookCtx, state, corerun.Event{
		Type: "hook.started",
		Data: map[string]any{
			"phase":       string(phase),
			"kind":        hook.Kind,
			"index":       index,
			"command":     state.masker.MaskString(command),
			"working_dir": workingDir,
		},
	})

	start := e.now()
	result, err := e.svc.Shell.Run(hookCtx, coreports.ShellRequest{
		Command:        command,
		WorkingDir:     workingDir,
		Env:            env,
		MaxOutputBytes: maxOutputBytes(state.plan.Workflow),
	})
	duration := e.now().Sub(start)

	exitCode := result.ExitCode
	if err == nil && result.ExitCode != 0 {
		err = fmt.Errorf("command exited with code %d", result.ExitCode)
	}

	artifactBase := filepath.Join("hooks", sanitizeName(string(phase)), fmt.Sprintf("%03d", index))
	if outErr := e.saveHookArtifact(ctx, state, artifactBase, "stdout.txt", result.Stdout); outErr != nil {
		_ = e.emitState(hookCtx, state, corerun.Event{Type: "hook.artifact_failed", Data: map[string]any{"phase": string(phase), "index": index, "error": outErr.Error()}})
	}
	if outErr := e.saveHookArtifact(ctx, state, artifactBase, "stderr.txt", result.Stderr); outErr != nil {
		_ = e.emitState(hookCtx, state, corerun.Event{Type: "hook.artifact_failed", Data: map[string]any{"phase": string(phase), "index": index, "error": outErr.Error()}})
	}

	resultData := map[string]any{
		"phase":       string(phase),
		"index":       index,
		"kind":        hook.Kind,
		"exit_code":   exitCode,
		"duration_ms": duration.Milliseconds(),
	}
	if err != nil {
		resultData["error"] = err.Error()
	}
	resultJSON, _ := json.MarshalIndent(resultData, "", "  ")
	art := corerun.Artifact{
		ID:           filepath.Join(artifactBase, "result.json"),
		RunID:        state.runID,
		Name:         "result.json",
		RelativePath: filepath.Join(artifactBase, "result.json"),
		MediaType:    "application/json",
		Kind:         corerun.ArtifactKindResult,
	}
	if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, resultJSON); err == nil {
		state.recordArtifact(1)
		_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
	}

	maskedStdout := state.masker.MaskString(result.Stdout)
	maskedStderr := state.masker.MaskString(result.Stderr)

	if err != nil {
		_ = e.emitState(hookCtx, state, corerun.Event{
			Type: "hook.failed",
			Data: map[string]any{
				"phase":       string(phase),
				"kind":        hook.Kind,
				"index":       index,
				"exit_code":   exitCode,
				"error":       state.masker.MaskString(err.Error()),
				"duration_ms": duration.Milliseconds(),
				"stdout":      maskedStdout,
				"stderr":      maskedStderr,
			},
		})
		return err
	}

	_ = e.emitState(hookCtx, state, corerun.Event{
		Type: "hook.finished",
		Data: map[string]any{
			"phase":       string(phase),
			"kind":        hook.Kind,
			"index":       index,
			"exit_code":   exitCode,
			"duration_ms": duration.Milliseconds(),
			"stdout":      maskedStdout,
			"stderr":      maskedStderr,
		},
	})
	return nil
}

func (e *Executor) saveHookArtifact(ctx context.Context, state *ExecutionState, base, name, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	masked := state.masker.MaskString(content)
	art := corerun.Artifact{
		ID:           filepath.Join(base, name),
		RunID:        state.runID,
		Name:         name,
		RelativePath: filepath.Join(base, name),
		MediaType:    "text/plain",
		Kind:         corerun.ArtifactKindCustom,
	}
	if err := e.svc.Runs.SaveArtifact(ctx, state.runID, art, []byte(masked)); err != nil {
		return err
	}
	state.recordArtifact(1)
	_ = e.emitState(ctx, state, corerun.Event{Type: "artifact.created", Data: map[string]any{"id": art.ID, "name": art.Name, "kind": art.Kind, "size_bytes": art.SizeBytes}})
	return nil
}
