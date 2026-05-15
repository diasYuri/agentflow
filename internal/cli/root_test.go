package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/daemon"
)

func TestGraphCommandPrintsMermaid(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowRef := "graph-test"
	workflowPath := filepath.Join(workflowDir, workflowRef+".yaml")
	err = os.WriteFile(workflowPath, []byte(`
version: "1"
name: graph-test
nodes:
  - id: plan
    kind: noop
  - id: implement
    kind: noop
    depends_on: [plan]
  - id: isolated
    kind: noop
`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"graph", workflowRef, "--format", "mermaid"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	const want = "graph TD\n  plan --> implement\n  isolated\n"
	if got := out.String(); got != want {
		t.Fatalf("unexpected graph output:\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestWorkflowListReportsDaemonUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "list"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected daemon unavailable error")
	}
	if !strings.Contains(err.Error(), "agentflowd is not running") {
		t.Fatalf("expected daemon unavailable message, got %v", err)
	}
}

func TestRunCommandDoesNotExposeOutputDirFlag(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"run", "cli-run", "-it", "--output-dir", "ignored"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --output-dir") {
		t.Fatalf("expected unknown output-dir flag, got %v", err)
	}
}

func TestRunCommandSendsRuntimeOptionsToDaemon(t *testing.T) {
	var got daemon.RunWorkflowRequest
	oldClient := newWorkflowRunClient
	newWorkflowRunClient = func(socketPath string) workflowRunClient {
		return workflowRunClientFunc(func(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
			got = req
			return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{ID: "run-1", RunDir: "/tmp/run-1", Status: "created"}}, nil
		})
	}
	t.Cleanup(func() {
		newWorkflowRunClient = oldClient
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"run", "daemon-run",
		"--codex-path", "/tmp/codex",
		"--claude-path", "/tmp/claude",
		"--events-jsonl", "/tmp/events.jsonl",
		"--log-format", "json",
		"--working-dir", "/tmp/work",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.WorkflowRef != "daemon-run" {
		t.Fatalf("workflow ref mismatch: got %q", got.WorkflowRef)
	}
	if got.CodexPath != "/tmp/codex" {
		t.Fatalf("codex path mismatch: got %q", got.CodexPath)
	}
	if got.ClaudePath != "/tmp/claude" {
		t.Fatalf("claude path mismatch: got %q", got.ClaudePath)
	}
	if got.EventsJSONL != "/tmp/events.jsonl" {
		t.Fatalf("events jsonl mismatch: got %q", got.EventsJSONL)
	}
	if got.LogFormat != "json" {
		t.Fatalf("log format mismatch: got %q", got.LogFormat)
	}
	if got.WorkingDir != "/tmp/work" {
		t.Fatalf("working dir mismatch: got %q", got.WorkingDir)
	}
}

type workflowRunClientFunc func(context.Context, daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error)

func (f workflowRunClientFunc) RunWorkflow(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
	return f(ctx, req)
}
