package handlers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

type mockShellRunner struct {
	result coreports.ShellResult
	err    error
}

func (m *mockShellRunner) Run(_ context.Context, _ coreports.ShellRequest) (coreports.ShellResult, error) {
	return m.result, m.err
}

type mockExtensionRunner struct {
	req    coreports.ExtensionRequest
	result coreports.ExtensionResult
	err    error
}

func (m *mockExtensionRunner) Run(_ context.Context, req coreports.ExtensionRequest) (coreports.ExtensionResult, error) {
	m.req = req
	return m.result, m.err
}

func (m *mockExtensionRunner) CloseRun(_ context.Context, _ string) error { return nil }

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

func TestDispatchExtensionNodeUsesBunRPCContract(t *testing.T) {
	dir := t.TempDir()
	extensionDir := filepath.Join(dir, ".agentflow", "extensions", "jira")
	if err := os.MkdirAll(extensionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(extensionDir, "main.ts")
	if err := os.WriteFile(scriptPath, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	extensions := &mockExtensionRunner{result: coreports.ExtensionResult{
		Output:   map[string]any{"status": "ok"},
		Stderr:   "log\n",
		ExitCode: 0,
	}}
	e := &Executor{svc: Services{Extensions: extensions}}
	state := newExecutionState("run-1", coreworkflow.ExecutionPlan{
		Workflow: coreworkflow.WorkflowSpec{Version: "1", Name: "test"},
	}, map[string]any{}, map[string]any{}, corerun.NewSecretMasker(map[string]any{}), e.now())
	state.baseWorkingDir = dir

	node := coreworkflow.NodeSpec{
		ID:        "n1",
		Kind:      coreworkflow.NodeKindExtension,
		Extension: "jira",
		Runtime:   "bun",
		Mode:      "server",
		Script:    "main.ts",
		Operation: "lookup",
		With: map[string]any{
			"issue": "AF-123",
		},
	}
	out, status, err := dispatchExtensionNode(context.Background(), e, state, node, coreworkflow.EvalContext{}, "", nil, nil, nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != corerun.NodeSuccess {
		t.Fatalf("expected success, got %s", status)
	}
	if out.Stderr != "log\n" {
		t.Fatalf("expected stderr to be preserved, got %q", out.Stderr)
	}
	if extensions.req.Runtime != "bun" || extensions.req.Mode != "server" || extensions.req.Operation != "lookup" {
		t.Fatalf("unexpected extension request: %#v", extensions.req)
	}
	if extensions.req.Script != scriptPath {
		t.Fatalf("expected script %q, got %q", scriptPath, extensions.req.Script)
	}
	withValues := extensions.req.Payload["with"].(map[string]any)
	if withValues["issue"] != "AF-123" {
		t.Fatalf("unexpected with values: %#v", withValues)
	}
}

func TestDispatchExtensionNodeReportsRunnerError(t *testing.T) {
	dir := t.TempDir()
	extensionDir := filepath.Join(dir, ".agentflow", "extensions", "missingrpc")
	if err := os.MkdirAll(extensionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extensionDir, "main.ts"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &Executor{svc: Services{Extensions: &mockExtensionRunner{
		result: coreports.ExtensionResult{ExitCode: -1},
		err:    errors.New("failed to start extension rpc adapter: executable file not found"),
	}}}
	state := newExecutionState("run-1", coreworkflow.ExecutionPlan{
		Workflow: coreworkflow.WorkflowSpec{Version: "1", Name: "test"},
	}, map[string]any{}, map[string]any{}, corerun.NewSecretMasker(map[string]any{}), e.now())
	state.baseWorkingDir = dir

	node := coreworkflow.NodeSpec{
		ID:        "n1",
		Kind:      coreworkflow.NodeKindExtension,
		Extension: "missingrpc",
		Script:    "main.ts",
	}
	_, status, err := dispatchExtensionNode(context.Background(), e, state, node, coreworkflow.EvalContext{}, "", nil, nil, nil, 1)
	if err == nil {
		t.Fatal("expected missing rpc adapter error")
	}
	if status != corerun.NodeFailed {
		t.Fatalf("expected failed status, got %s", status)
	}
	if !strings.Contains(err.Error(), "failed to start extension rpc adapter") {
		t.Fatalf("expected rpc adapter startup error, got %v", err)
	}
}

func TestResolveExtensionDirPrefersWorkingDir(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	localExtension := filepath.Join(dir, ".agentflow", "extensions", "jira")
	homeExtension := filepath.Join(home, ".agentflow", "extensions", "jira")
	if err := os.MkdirAll(localExtension, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeExtension, 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveExtensionDir(dir, "jira")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != localExtension {
		t.Fatalf("expected local extension %q, got %q", localExtension, resolved)
	}
}
