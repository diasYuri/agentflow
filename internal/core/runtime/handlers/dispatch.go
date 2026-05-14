package handlers

import (
	"context"
	"fmt"

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
	state.incrementBashCalls()
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
	state.incrementAgentCalls()
	sandbox := effectiveAgentSandbox(node)
	result, err := provider.Run(ctx, coreports.AgentRequest{
		RunID: state.runID, NodeID: node.ID, InstanceID: instanceID, Attempt: attempt, Provider: providerName,
		Model: node.Model, System: node.System, Prompt: prompt, WorkingDir: resolvePath(state.baseWorkingDir, effectiveWorkingDir(state.plan.Workflow, node)),
		Env: node.Env, OutputSchema: node.OutputSchema, Sandbox: sandbox,
	})
	if err != nil {
		return dispatchOutput{}, corerun.NodeFailed, err
	}
	if result.JSON != nil {
		return dispatchOutput{Output: result.JSON}, corerun.NodeSuccess, nil
	}
	return dispatchOutput{Output: result.Text}, corerun.NodeSuccess, nil
}
