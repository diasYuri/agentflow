package rpc

import (
	"context"
	"testing"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

func TestParseRPCOutputReturnsExtensionOutput(t *testing.T) {
	output, stderr, err := parseRPCOutput([]byte(`{"jsonrpc":"2.0","id":"1","result":{"ok":true,"output":{"status":"ok"},"stderr":"log\n"}}` + "\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stderr != "log\n" {
		t.Fatalf("unexpected stderr: %q", stderr)
	}
	m, ok := output.(map[string]any)
	if !ok || m["status"] != "ok" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestParseRPCOutputReturnsStructuredError(t *testing.T) {
	_, _, err := parseRPCOutput([]byte(`{"jsonrpc":"2.0","id":"1","error":{"code":"NOT_FOUND","message":"missing"}}` + "\n"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "extension rpc NOT_FOUND: missing" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCloseRunIgnoresOtherRuns(t *testing.T) {
	runner := New("")
	if err := runner.CloseRun(context.Background(), "missing"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRPCParamsIncludesOperationAndPayload(t *testing.T) {
	params := rpcParams(ports.ExtensionRequest{
		Script:    "/tmp/main.ts",
		Operation: "lookup",
		Payload: map[string]any{
			"version": "agentflow.extension.v1",
		},
	})
	if params["script"] != "/tmp/main.ts" || params["operation"] != "lookup" || params["version"] != "agentflow.extension.v1" {
		t.Fatalf("unexpected params: %#v", params)
	}
}

func TestSessionKeyIncludesWorkingDirAndEnv(t *testing.T) {
	a := sessionKey(ports.ExtensionRequest{RunID: "run", Extension: "echo", WorkingDir: "/tmp/a", Env: map[string]string{"B": "2", "A": "1"}})
	b := sessionKey(ports.ExtensionRequest{RunID: "run", Extension: "echo", WorkingDir: "/tmp/b", Env: map[string]string{"A": "1", "B": "2"}})
	c := sessionKey(ports.ExtensionRequest{RunID: "run", Extension: "echo", WorkingDir: "/tmp/a", Env: map[string]string{"A": "1", "B": "3"}})
	if a == b || a == c {
		t.Fatalf("expected distinct session keys, got %q %q %q", a, b, c)
	}
}
