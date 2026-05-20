package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type dispatchOutput struct {
	Output   any
	Stdout   string
	Stderr   string
	ExitCode *int
}

type nodeDispatcher func(
	ctx context.Context,
	e *Executor,
	state *ExecutionState,
	node coreworkflow.NodeSpec,
	evalCtx coreworkflow.EvalContext,
	instanceID string,
	index *int,
	total *int,
	item any,
	attempt int,
) (dispatchOutput, corerun.NodeStatus, error)

var nodeDispatchers = map[coreworkflow.NodeKind]nodeDispatcher{
	coreworkflow.NodeKindNoop:      dispatchNoopNode,
	coreworkflow.NodeKindTransform: dispatchTransformNode,
	coreworkflow.NodeKindBash:      dispatchBashNode,
	coreworkflow.NodeKindExtension: dispatchExtensionNode,
	coreworkflow.NodeKindAgent:     dispatchAgentNode,
}

func (e *Executor) dispatch(ctx context.Context, state *ExecutionState, node coreworkflow.NodeSpec, instanceID string, index *int, total *int, item any, attempt int) (dispatchOutput, corerun.NodeStatus, error) {
	evalCtx := state.evalContext(index, total, item)
	dispatcher, ok := nodeDispatchers[node.Kind]
	if !ok {
		return dispatchOutput{}, corerun.NodeFailed, fmt.Errorf("unsupported node kind %q", node.Kind)
	}
	return dispatcher(ctx, e, state, node, evalCtx, instanceID, index, total, item, attempt)
}

func dispatchNoopNode(_ context.Context, _ *Executor, _ *ExecutionState, _ coreworkflow.NodeSpec, _ coreworkflow.EvalContext, _ string, _ *int, _ *int, _ any, _ int) (dispatchOutput, corerun.NodeStatus, error) {
	return dispatchOutput{Output: map[string]any{"status": "ok"}}, corerun.NodeSuccess, nil
}

func dispatchTransformNode(_ context.Context, _ *Executor, _ *ExecutionState, node coreworkflow.NodeSpec, evalCtx coreworkflow.EvalContext, _ string, _ *int, _ *int, _ any, _ int) (dispatchOutput, corerun.NodeStatus, error) {
	input, err := coreworkflow.EvalTemplateValue(node.Input, evalCtx)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	out, err := coreworkflow.ApplyTransform(node.Operation, input, node.With)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	return dispatchOutput{Output: out}, corerun.NodeSuccess, nil
}

func dispatchBashNode(ctx context.Context, e *Executor, state *ExecutionState, node coreworkflow.NodeSpec, evalCtx coreworkflow.EvalContext, instanceID string, _ *int, _ *int, _ any, attempt int) (dispatchOutput, corerun.NodeStatus, error) {
	command, err := coreworkflow.RenderTemplate(node.Command, evalCtx)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	shell := effectiveShell(node)
	workingDir := resolvePath(state.baseWorkingDir, effectiveWorkingDir(state.plan.Workflow, node))
	_ = e.emitState(ctx, state, corerun.Event{
		Type:       "node.bash.warning",
		NodeID:     node.ID,
		InstanceID: instanceID,
		Attempt:    attempt,
		Data: map[string]any{
			"warning":     "executing bash command; workflow authors can run arbitrary local commands",
			"shell":       shell,
			"working_dir": workingDir,
			"command":     command,
		},
	})
	state.incrementBashCalls(node.ID)
	result, err := e.svc.Shell.Run(ctx, coreports.ShellRequest{
		Command: command, Shell: shell, WorkingDir: workingDir,
		Env: node.Env, MaxOutputBytes: maxOutputBytes(state.plan.Workflow),
	})
	exitCode := result.ExitCode
	out := map[string]any{"stdout": result.Stdout, "stderr": result.Stderr, "exit_code": result.ExitCode}
	status := corerun.NodeSuccess
	if err != nil || result.ExitCode != 0 {
		status = corerun.NodeFailed
		if err == nil {
			err = fmt.Errorf("command exited with code %d", result.ExitCode)
		}
	}
	return dispatchOutput{Output: out, Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: &exitCode}, status, err
}

func dispatchExtensionNode(ctx context.Context, e *Executor, state *ExecutionState, node coreworkflow.NodeSpec, evalCtx coreworkflow.EvalContext, instanceID string, index *int, total *int, item any, attempt int) (dispatchOutput, corerun.NodeStatus, error) {
	if e.svc.Extensions == nil {
		return dispatchOutput{}, corerun.NodeFailed, fmt.Errorf("extension runner is not configured")
	}
	extensionDir, err := resolveExtensionDir(state.baseWorkingDir, node.Extension)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	scriptPath, err := resolveExtensionScript(extensionDir, node.Script)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	withValues, err := evalTemplateMap(node.With, evalCtx)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, fmt.Errorf("with: %w", err)
	}
	env, err := renderEnv(node.Env, evalCtx)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, fmt.Errorf("env: %w", err)
	}
	workingDir := resolvePath(state.baseWorkingDir, effectiveWorkingDir(state.plan.Workflow, node))
	payload := map[string]any{
		"version": "agentflow.extension.v1",
		"run": map[string]any{
			"id":       state.runID,
			"workflow": state.plan.Workflow.Name,
		},
		"node": map[string]any{
			"id":          node.ID,
			"attempt":     attempt,
			"instance_id": instanceID,
			"index":       index,
			"total":       total,
		},
		"context": map[string]any{
			"inputs":  evalCtx.Inputs,
			"vars":    evalCtx.Vars,
			"secrets": evalCtx.Secrets,
			"nodes":   evalCtx.Nodes,
			"item":    item,
		},
		"with": withValues,
		"extension": map[string]any{
			"name":   node.Extension,
			"dir":    extensionDir,
			"script": scriptPath,
		},
		"working_dir": workingDir,
	}
	runtime := node.Runtime
	if runtime == "" {
		runtime = "bun"
	}
	mode := node.Mode
	if mode == "" {
		mode = "oneshot"
	}
	_ = e.emitState(ctx, state, corerun.Event{
		Type:       "node.extension.warning",
		NodeID:     node.ID,
		InstanceID: instanceID,
		Attempt:    attempt,
		Data: map[string]any{
			"warning":       "executing extension script with Bun RPC adapter; workflow authors can run arbitrary local code",
			"extension":     node.Extension,
			"extension_dir": extensionDir,
			"script":        scriptPath,
			"runtime":       runtime,
			"mode":          mode,
			"runner":        "agentflow-extension-rpc",
			"working_dir":   workingDir,
		},
	})
	result, err := e.svc.Extensions.Run(ctx, coreports.ExtensionRequest{
		RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Attempt: attempt,
		Extension: node.Extension, ExtensionDir: extensionDir, Script: scriptPath,
		Operation: node.Operation, Runtime: runtime, Mode: mode, WorkingDir: workingDir,
		Env: env, Payload: payload, MaxOutputBytes: maxOutputBytes(state.plan.Workflow),
	})
	exitCode := result.ExitCode
	out := dispatchOutput{Output: result.Output, Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: &exitCode}
	if err != nil {
		return out, corerun.NodeFailed, err
	}
	return out, corerun.NodeSuccess, nil
}

func dispatchAgentNode(ctx context.Context, e *Executor, state *ExecutionState, node coreworkflow.NodeSpec, evalCtx coreworkflow.EvalContext, instanceID string, _ *int, _ *int, _ any, attempt int) (dispatchOutput, corerun.NodeStatus, error) {
	prompt, err := coreworkflow.RenderTemplate(node.Prompt, evalCtx)
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	providerName := node.Provider
	if providerName == "" {
		providerName = "codex"
	}
	provider, ok := e.svc.Agents.Get(providerName)
	if !ok {
		return dispatchOutput{}, corerun.NodeFailed, fmt.Errorf("unknown agent provider %q", providerName)
	}
	state.incrementAgentCalls(node.ID)
	sandbox := effectiveAgentSandbox(node)
	result, err := provider.Run(ctx, coreports.AgentRequest{
		RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Attempt: attempt, Provider: providerName,
		Model: node.Model, System: node.System, Prompt: prompt, WorkingDir: resolvePath(state.baseWorkingDir, effectiveWorkingDir(state.plan.Workflow, node)),
		Env: node.Env, OutputSchema: node.OutputSchema, Sandbox: sandbox,
	})
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	var costUSD float64
	if result.Metadata != nil {
		if c, ok := result.Metadata["claude_cost_usd"].(float64); ok {
			costUSD = c
		} else if c, ok := result.Metadata["cost_usd"].(float64); ok {
			costUSD = c
		}
	}
	state.recordAgentUsage(providerName, node.Model, result.Usage, costUSD)
	if result.Usage != nil {
		_ = e.emitState(ctx, state, corerun.Event{Type: "agent.usage", NodeID: node.ID, InstanceID: instanceID, Data: map[string]any{
			"provider":      providerName,
			"model":         node.Model,
			"input_tokens":  result.Usage.InputTokens,
			"output_tokens": result.Usage.OutputTokens,
			"total_tokens":  result.Usage.TotalTokens,
			"cost_usd":      costUSD,
		}})
	}
	if result.JSON != nil {
		return dispatchOutput{Output: result.JSON}, corerun.NodeSuccess, nil
	}
	return dispatchOutput{Output: result.Text}, corerun.NodeSuccess, nil
}

func resolveExtensionDir(baseWorkingDir string, name string) (string, error) {
	candidates := []string{filepath.Join(baseWorkingDir, ".agentflow", "extensions", name)}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".agentflow", "extensions", name))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() {
			return filepath.Clean(candidate), nil
		}
	}
	return "", fmt.Errorf("extension %q not found in .agentflow/extensions or ~/.agentflow/extensions", name)
}

func resolveExtensionScript(extensionDir string, script string) (string, error) {
	clean := filepath.Clean(script)
	path := filepath.Join(extensionDir, clean)
	rel, err := filepath.Rel(extensionDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("extension script must not escape the extension directory")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("extension script %q not found: %w", script, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("extension script %q is a directory", script)
	}
	return filepath.Clean(path), nil
}

func evalTemplateMap(input map[string]any, ctx coreworkflow.EvalContext) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for key, value := range input {
		evaluated, err := evalTemplateAny(value, ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = evaluated
	}
	return out, nil
}

func evalTemplateAny(value any, ctx coreworkflow.EvalContext) (any, error) {
	switch typed := value.(type) {
	case string:
		return coreworkflow.EvalTemplateValue(typed, ctx)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			evaluated, err := evalTemplateAny(item, ctx)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = evaluated
		}
		return out, nil
	case map[string]any:
		return evalTemplateMap(typed, ctx)
	default:
		return value, nil
	}
}

func renderEnv(env map[string]string, ctx coreworkflow.EvalContext) (map[string]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		rendered, err := coreworkflow.RenderTemplate(value, ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = rendered
	}
	return out, nil
}
