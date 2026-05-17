package handlers

import (
	"context"
	"testing"

	coreports "github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

type mockAgentProvider struct {
	result coreports.AgentResult
	err    error
}

func (m *mockAgentProvider) Run(_ context.Context, _ coreports.AgentRequest) (coreports.AgentResult, error) {
	return m.result, m.err
}

func TestDispatchAgentNodeUsesOnlyTextJSONAndUsage(t *testing.T) {
	// This test pins the runtime contract: dispatchAgentNode must consume
	// only Text, JSON and Usage from the provider result. RawEvents must not
	// affect the functional output.
	provider := &mockAgentProvider{
		result: coreports.AgentResult{
			Text: "plain text",
			RawEvents: []coreports.AgentEvent{
				{Type: "agent_start", Data: map[string]any{"session": "s1"}},
			},
			Usage: &coreports.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		},
	}
	registry := coreports.NewStaticAgentProviderRegistry(map[string]coreports.AgentProvider{
		"pi": provider,
	})

	e := &Executor{svc: Services{Agents: registry}}
	state := newExecutionState("run-1", coreworkflow.ExecutionPlan{
		Workflow: coreworkflow.WorkflowSpec{Version: "1", Name: "test"},
	}, map[string]any{}, map[string]any{}, corerun.NewSecretMasker(map[string]any{}), e.now())
	state.baseWorkingDir = t.TempDir()

	node := coreworkflow.NodeSpec{
		ID:       "n1",
		Kind:     coreworkflow.NodeKindAgent,
		Provider: "pi",
		Prompt:   "hello",
	}
	evalCtx := coreworkflow.EvalContext{}

	out, status, err := dispatchAgentNode(context.Background(), e, state, node, evalCtx, "", nil, nil, nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != corerun.NodeSuccess {
		t.Fatalf("expected success, got %s", status)
	}
	if out.Output != "plain text" {
		t.Fatalf("expected text output, got %v", out.Output)
	}

	// JSON path
	provider.result = coreports.AgentResult{
		Text: "ignored",
		JSON: map[string]any{"status": "ok"},
		RawEvents: []coreports.AgentEvent{
			{Type: "agent_start", Data: map[string]any{"session": "s1"}},
		},
		Usage: &coreports.Usage{InputTokens: 4, OutputTokens: 5, TotalTokens: 9},
	}
	out, status, err = dispatchAgentNode(context.Background(), e, state, node, evalCtx, "", nil, nil, nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != corerun.NodeSuccess {
		t.Fatalf("expected success, got %s", status)
	}
	m, ok := out.Output.(map[string]any)
	if !ok || m["status"] != "ok" {
		t.Fatalf("expected JSON output, got %v", out.Output)
	}
}
