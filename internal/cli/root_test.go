package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestWorkflowListRendersTableAndJson(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			list: func(context.Context) (daemon.ListWorkflowsResponse, error) {
				return daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{{ID: "run-1", Workflow: "build", Status: "running", RunDir: "/tmp/run-1", StartedAt: time.Unix(100, 0)}}}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "list", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "CONCLUÍDOS") || !strings.Contains(got, "TOTAL") || !strings.Contains(got, "run-1") {
		t.Fatalf("unexpected list output: %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"run-1"`) {
		t.Fatalf("unexpected json output: %q", out.String())
	}
}

func TestWorkflowStatusAndWatchRenderProgress(t *testing.T) {
	oldClient := newDaemonClient
	oldInterval := workflowWatchInterval
	workflowWatchInterval = time.Millisecond
	defer func() {
		newDaemonClient = oldClient
		workflowWatchInterval = oldInterval
	}()
	var calls int
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			status: func(context.Context) (daemon.DaemonStatus, error) { return daemon.DaemonStatus{}, nil },
			workflowStatus: func(context.Context, string) (daemon.RunWorkflowResponse, error) {
				calls++
				status := daemon.WorkflowRun{ID: "run-1", Workflow: "build", Status: "running", CurrentStep: "plan", RunDir: "/tmp/run-1", CompletedSteps: []string{"setup"}, PendingSteps: []string{"plan", "ship"}, TotalSteps: 3}
				if calls > 1 {
					status.Status = "success"
					status.CurrentStep = "ship"
				}
				return daemon.RunWorkflowResponse{Run: status}, nil
			},
		}
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "status", "run-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "workflow: build") || !strings.Contains(out.String(), "completed: 1/3") {
		t.Fatalf("unexpected status output: %q", out.String())
	}

	calls = 0
	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "status", "run-1", "--watch", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "status:") < 2 || !strings.Contains(out.String(), "status: success") {
		t.Fatalf("unexpected watch output: %q", out.String())
	}
	if !strings.Contains(out.String(), "\x1b[H\x1b[2Jid:") {
		t.Fatalf("expected watch output to clear before refresh, got %q", out.String())
	}

	calls = 0
	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "watch", "run-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); strings.Count(got, "\n") < 1 || strings.Contains(got, "\n\n") {
		t.Fatalf("unexpected json watch output: %q", out.String())
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

func TestWorkflowPauseCommandPrintsPauseReason(t *testing.T) {
	var got string
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			pauseWorkflow: func(ctx context.Context, id string) (daemon.PauseWorkflowResponse, error) {
				got = id
				return daemon.PauseWorkflowResponse{Run: daemon.WorkflowRun{
					ID:          "run-1",
					RunDir:      "/tmp/run-1",
					Status:      "paused",
					PauseReason: "manual",
				}}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "pause", "run-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got != "run-1" {
		t.Fatalf("expected to pass run-1, got %q", got)
	}
	output := out.String()
	if !strings.Contains(output, "status: paused") {
		t.Fatalf("expected paused status in output, got %q", output)
	}
	if !strings.Contains(output, "pause_reason: manual") {
		t.Fatalf("expected pause_reason in output, got %q", output)
	}
}

func TestWorkflowResumeCommandRendersResumedRun(t *testing.T) {
	var got string
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			resumeWorkflow: func(ctx context.Context, id string) (daemon.ResumeWorkflowResponse, error) {
				got = id
				return daemon.ResumeWorkflowResponse{Run: daemon.WorkflowRun{
					ID:          "run-1",
					RunDir:      "/tmp/run-1",
					Status:      "running",
					ResumeCount: 1,
				}}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "resume", "run-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got != "run-1" {
		t.Fatalf("expected to pass run-1, got %q", got)
	}
	output := out.String()
	if !strings.Contains(output, "status: running") {
		t.Fatalf("expected running status, got %q", output)
	}
	if !strings.Contains(output, "resume_count: 1") {
		t.Fatalf("expected resume_count in output, got %q", output)
	}
}

func TestWorkflowStatusRendersPausedHintAndStopsWatch(t *testing.T) {
	oldClient := newDaemonClient
	oldInterval := workflowWatchInterval
	workflowWatchInterval = time.Millisecond
	defer func() {
		newDaemonClient = oldClient
		workflowWatchInterval = oldInterval
	}()
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowStatus: func(context.Context, string) (daemon.RunWorkflowResponse, error) {
				return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{
					ID:          "run-1",
					Workflow:    "demo",
					Status:      "paused",
					CurrentStep: "flaky",
					RunDir:      "/tmp/run-1",
					PauseReason: "pause_when_fail",
					ResumeCount: 0,
					TotalSteps:  3,
				}}, nil
			},
		}
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "status", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "status: paused") {
		t.Fatalf("expected paused status, got %q", out.String())
	}
	if !strings.Contains(out.String(), "pause_reason: pause_when_fail") {
		t.Fatalf("expected pause_reason in status, got %q", out.String())
	}
	if !strings.Contains(out.String(), "agentflow workflow resume run-1") {
		t.Fatalf("expected resume hint in status, got %q", out.String())
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "watch", "run-1", "--no-color"})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("watch did not stop on paused run")
	}
	if strings.Count(out.String(), "status: paused") < 1 {
		t.Fatalf("expected paused status in watch output, got %q", out.String())
	}
}

type workflowRunClientFunc func(context.Context, daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error)

func (f workflowRunClientFunc) RunWorkflow(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
	return f(ctx, req)
}

type workflowDaemonClientFunc struct {
	list           func(context.Context) (daemon.ListWorkflowsResponse, error)
	workflowStatus func(context.Context, string) (daemon.RunWorkflowResponse, error)
	workflowLogs   func(context.Context, string) (daemon.LogsResponse, error)
	cancelWorkflow func(context.Context, string) (daemon.CancelWorkflowResponse, error)
	pauseWorkflow  func(context.Context, string) (daemon.PauseWorkflowResponse, error)
	resumeWorkflow func(context.Context, string) (daemon.ResumeWorkflowResponse, error)
	status         func(context.Context) (daemon.DaemonStatus, error)
	stop           func(context.Context) (daemon.StopResponse, error)
}

func (f workflowDaemonClientFunc) ListWorkflows(ctx context.Context) (daemon.ListWorkflowsResponse, error) {
	return f.list(ctx)
}
func (f workflowDaemonClientFunc) WorkflowStatus(ctx context.Context, id string) (daemon.RunWorkflowResponse, error) {
	return f.workflowStatus(ctx, id)
}
func (f workflowDaemonClientFunc) WorkflowLogs(ctx context.Context, id string) (daemon.LogsResponse, error) {
	return f.workflowLogs(ctx, id)
}
func (f workflowDaemonClientFunc) CancelWorkflow(ctx context.Context, id string) (daemon.CancelWorkflowResponse, error) {
	return f.cancelWorkflow(ctx, id)
}
func (f workflowDaemonClientFunc) PauseWorkflow(ctx context.Context, id string) (daemon.PauseWorkflowResponse, error) {
	if f.pauseWorkflow == nil {
		return daemon.PauseWorkflowResponse{}, nil
	}
	return f.pauseWorkflow(ctx, id)
}
func (f workflowDaemonClientFunc) ResumeWorkflow(ctx context.Context, id string) (daemon.ResumeWorkflowResponse, error) {
	if f.resumeWorkflow == nil {
		return daemon.ResumeWorkflowResponse{}, nil
	}
	return f.resumeWorkflow(ctx, id)
}
func (f workflowDaemonClientFunc) Status(ctx context.Context) (daemon.DaemonStatus, error) {
	return f.status(ctx)
}
func (f workflowDaemonClientFunc) Stop(ctx context.Context) (daemon.StopResponse, error) {
	return f.stop(ctx)
}
