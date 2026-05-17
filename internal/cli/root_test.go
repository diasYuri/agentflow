package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/spf13/cobra"
)

func TestTUICommandExists(t *testing.T) {
	cmd := NewRootCommand()
	if cmd := findSubcommand(cmd, "tui"); cmd == nil {
		t.Fatal("expected tui command to be registered")
	}
}

func TestTUICommandAcceptsFlags(t *testing.T) {
	cmd := NewRootCommand()
	tui := findSubcommand(cmd, "tui")
	if tui == nil {
		t.Fatal("expected tui command")
	}
	if err := tui.ParseFlags([]string{"--workflow", "build", "--run", "run-1", "--no-mouse", "--theme", "dark"}); err != nil {
		t.Fatalf("unexpected flag parse error: %v", err)
	}
}

func TestTUICommandHelp(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"tui", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"tui", "--workflow", "--run", "--daemon", "--no-mouse", "--theme"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in help output:\n%s", want, got)
		}
	}
}

func findSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

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
				return daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{{ID: "run-1", Workflow: "build", Status: "running", RunDir: "/tmp/run-1", StartedAt: time.Unix(100, 0), Tag: "smoke-test"}}}, nil
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
	if !strings.Contains(got, "smoke-test") {
		t.Fatalf("expected tag in list output: %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"run-1"`) || !strings.Contains(out.String(), `"tag":"smoke-test"`) {
		t.Fatalf("unexpected json output: %q", out.String())
	}
}

func TestWorkflowListRendersElapsedTime(t *testing.T) {
	started := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	runningStarted := time.Now().Add(-2 * time.Hour)
	runs := []daemon.WorkflowRun{
		{
			ID:         "run-success",
			Workflow:   "build",
			Status:     "success",
			RunDir:     "/tmp/run-success",
			StartedAt:  started,
			FinishedAt: started.Add(5 * time.Minute),
		},
		{
			ID:         "run-failed",
			Workflow:   "deploy",
			Status:     "failed",
			RunDir:     "/tmp/run-failed",
			StartedAt:  started,
			FinishedAt: started.Add(7 * time.Second),
		},
		{
			ID:         "run-paused",
			Workflow:   "review",
			Status:     "paused",
			RunDir:     "/tmp/run-paused",
			StartedAt:  started,
			FinishedAt: started.Add(3 * time.Second),
		},
		{
			ID:        "run-live",
			Workflow:  "test",
			Status:    "running",
			RunDir:    "/tmp/run-live",
			StartedAt: runningStarted,
		},
	}

	var out bytes.Buffer
	if err := renderWorkflowList(&out, runs, string(workflowOutputText), true, false); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"TEMPO", "5m0s", "7s", "3s", "2h0m0s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output: %q", want, got)
		}
	}
	if strings.Contains(got, "IDADE") {
		t.Fatalf("expected IDADE header to be renamed, got %q", got)
	}

	out.Reset()
	if err := renderWorkflowList(&out, runs[:1], string(workflowOutputJSON), true, false); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	if strings.Contains(got, "elapsed") || !strings.Contains(got, `"started_at"`) || !strings.Contains(got, `"finished_at"`) {
		t.Fatalf("unexpected json output: %q", got)
	}
}

func TestWorkflowListColorsStatusWithoutLeakingANSI(t *testing.T) {
	runs := []daemon.WorkflowRun{{
		ID:          "run-live",
		Workflow:    "build",
		Status:      "running",
		CurrentStep: "implement",
		RunDir:      "/tmp/run-live",
		StartedAt:   time.Now(),
	}}

	var out bytes.Buffer
	if err := renderWorkflowList(&out, runs, string(workflowOutputText), false, true); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "running") || !strings.Contains(got, "implement") {
		t.Fatalf("expected plain list output with status and step, got %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected no ANSI in buffer output, got %q", got)
	}
}

func TestWorkflowStatusRendersTag(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowStatus: func(context.Context, string) (daemon.RunWorkflowResponse, error) {
				return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{
					ID:            "run-1",
					Workflow:      "build",
					Status:        "failed",
					Tag:           "release-123",
					RunDir:        "/tmp/run-1",
					FailureReason: "node flaky failed",
					TerminalError: "node flaky failed",
					Error:         "node flaky failed",
				}}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "status", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "tag: release-123") {
		t.Fatalf("expected tag in status output: %q", got)
	}
	if !strings.Contains(got, "failure_reason: node flaky failed") {
		t.Fatalf("expected failure_reason in status output: %q", got)
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
	if !strings.Contains(out.String(), "\x1b[H\x1b[2J") || strings.Count(out.String(), "Workflow status") < 2 {
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

func TestWorkflowWatchRendersFailureReason(t *testing.T) {
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
			workflowStatus: func(context.Context, string) (daemon.RunWorkflowResponse, error) {
				calls++
				if calls == 1 {
					return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{
						ID:          "run-1",
						Workflow:    "build",
						Status:      "running",
						CurrentStep: "plan",
						RunDir:      "/tmp/run-1",
					}}, nil
				}
				return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{
					ID:            "run-1",
					Workflow:      "build",
					Status:        "failed",
					CurrentStep:   "plan",
					RunDir:        "/tmp/run-1",
					FailureReason: "node plan failed",
					TerminalError: "node plan failed",
					Error:         "node plan failed",
				}}, nil
			},
		}
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "watch", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "failure_reason: node plan failed") {
		t.Fatalf("expected failure_reason in watch output, got %q", got)
	}
	if !strings.Contains(got, "status: failed") {
		t.Fatalf("expected failed status in watch output, got %q", got)
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

func TestRunCommandSendsTagToDaemon(t *testing.T) {
	var got daemon.RunWorkflowRequest
	oldClient := newWorkflowRunClient
	newWorkflowRunClient = func(socketPath string) workflowRunClient {
		return workflowRunClientFunc(func(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
			got = req
			return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{ID: "run-1", RunDir: "/tmp/run-1", Status: "created"}}, nil
		})
	}
	t.Cleanup(func() { newWorkflowRunClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"run", "daemon-run", "--tag", "release-123"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Tag != "release-123" {
		t.Fatalf("tag mismatch: got %q", got.Tag)
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
		"--pi-path", "/tmp/pi",
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
	if got.PiPath != "/tmp/pi" {
		t.Fatalf("pi path mismatch: got %q", got.PiPath)
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

func TestRunCommandSendsAbsoluteDefaultWorkingDirToDaemon(t *testing.T) {
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
	cmd.SetArgs([]string{"run", "daemon-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkingDir != want {
		t.Fatalf("working dir mismatch: got %q want %q", got.WorkingDir, want)
	}
}

func TestDaemonProviderEnvIncludesPiPath(t *testing.T) {
	env := daemonProviderEnv([]string{"PATH=/usr/bin"}, &options{
		codexPath:  "/tmp/codex",
		claudePath: "/tmp/claude",
		piPath:     "/tmp/pi",
	})

	joined := "\n" + strings.Join(env, "\n") + "\n"
	for _, want := range []string{
		"\nPATH=/usr/bin\n",
		"\nAGENTFLOW_CODEX_PATH=/tmp/codex\n",
		"\nAGENTFLOW_CLAUDE_PATH=/tmp/claude\n",
		"\nAGENTFLOW_PI_PATH=/tmp/pi\n",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected env %q in %#v", strings.TrimSpace(want), env)
		}
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

func TestWorkflowArtifactsRendersTextAndJson(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowArtifacts: func(ctx context.Context, id string) (daemon.WorkflowArtifactsResponse, error) {
				return daemon.WorkflowArtifactsResponse{
					RunID: id,
					Artifacts: []daemon.WorkflowArtifactDTO{
						{ID: "nodes/n1/stdout.txt", Name: "stdout.txt", MediaType: "text/plain", SizeBytes: 12, NodeID: "n1"},
						{ID: "nodes/n1/result.json", Name: "result.json", MediaType: "application/json", SizeBytes: 256, NodeID: "n1"},
					},
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "artifacts", "run-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "nodes/n1/stdout.txt") {
		t.Fatalf("expected artifact id in output, got %q", got)
	}
	if !strings.Contains(got, "text/plain") {
		t.Fatalf("expected media type in output, got %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "artifacts", "run-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id":"nodes/n1/stdout.txt"`) {
		t.Fatalf("expected json output, got %q", out.String())
	}
}

func TestWorkflowArtifactShowRendersTextContent(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowArtifact: func(ctx context.Context, runID, artifactID string) (daemon.WorkflowArtifactResponse, error) {
				return daemon.WorkflowArtifactResponse{
					ID: artifactID, Name: "stdout.txt", MediaType: "text/plain",
					SizeBytes: 5, IsText: true, TextContent: "hello",
					Encoding: "text", Kind: "stdout", NodeID: "n1",
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "artifact", "show", "run-1", "nodes/n1/stdout.txt"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "hello") {
		t.Fatalf("expected text content, got %q", got)
	}
	if !strings.Contains(got, "media_type: text/plain") {
		t.Fatalf("expected media_type, got %q", got)
	}
}

func TestWorkflowArtifactShowRendersBinaryOmission(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowArtifact: func(ctx context.Context, runID, artifactID string) (daemon.WorkflowArtifactResponse, error) {
				return daemon.WorkflowArtifactResponse{
					ID: artifactID, Name: "image.png", MediaType: "image/png",
					SizeBytes: 2048, IsText: false, Encoding: "base64",
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "artifact", "show", "run-1", "nodes/n1/image.png"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "binary content omitted") {
		t.Fatalf("expected binary omission message, got %q", got)
	}
}

func TestWorkflowArtifactPathPrintsPath(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			artifactPath: func(ctx context.Context, runID, artifactID string) (string, error) {
				return "/tmp/run-1/artifacts/" + artifactID, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "artifact", "path", "run-1", "nodes/n1/stdout.txt"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if got != "/tmp/run-1/artifacts/nodes/n1/stdout.txt" {
		t.Fatalf("expected path, got %q", got)
	}
}

func TestProjectCommandsAndResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectRoot := filepath.Join(t.TempDir(), "demo-project")
	workflowDir := filepath.Join(projectRoot, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(workflowDir, "demo.yaml")
	if err := os.WriteFile(workflowPath, []byte(`version: "1"
name: demo
nodes:
  - id: ok
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"project", "add", "demo", projectRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("project add: %v", err)
	}
	if !strings.Contains(out.String(), "Project added") || !strings.Contains(out.String(), "name: demo") {
		t.Fatalf("unexpected add output: %q", out.String())
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"project", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("project list: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "demo") || !strings.Contains(got, filepath.Clean(projectRoot)) {
		t.Fatalf("unexpected list output: %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", "demo", "--project", "demo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate project workflow: %v", err)
	}
	if !strings.Contains(out.String(), "valid: demo") {
		t.Fatalf("unexpected validate output: %q", out.String())
	}

	var gotReq daemon.RunWorkflowRequest
	oldRunClient := newWorkflowRunClient
	newWorkflowRunClient = func(socketPath string) workflowRunClient {
		return workflowRunClientFunc(func(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
			gotReq = req
			return daemon.RunWorkflowResponse{Run: daemon.WorkflowRun{ID: "run-1", RunDir: "/tmp/run-1", Status: "created"}}, nil
		})
	}
	t.Cleanup(func() { newWorkflowRunClient = oldRunClient })

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"run", "demo", "--project", "demo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run project workflow: %v", err)
	}
	if gotReq.WorkflowRef != workflowPath {
		t.Fatalf("expected resolved workflow path %q, got %q", workflowPath, gotReq.WorkflowRef)
	}
	if gotReq.WorkingDir != filepath.Clean(projectRoot) {
		t.Fatalf("expected working dir %q, got %q", filepath.Clean(projectRoot), gotReq.WorkingDir)
	}
}

type workflowRunClientFunc func(context.Context, daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error)

func (f workflowRunClientFunc) RunWorkflow(ctx context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
	return f(ctx, req)
}

type workflowDaemonClientFunc struct {
	list              func(context.Context) (daemon.ListWorkflowsResponse, error)
	workflowStatus    func(context.Context, string) (daemon.RunWorkflowResponse, error)
	workflowLogs      func(context.Context, string) (daemon.LogsResponse, error)
	workflowArtifacts func(context.Context, string) (daemon.WorkflowArtifactsResponse, error)
	workflowArtifact  func(context.Context, string, string) (daemon.WorkflowArtifactResponse, error)
	artifactPath      func(context.Context, string, string) (string, error)
	cancelWorkflow    func(context.Context, string) (daemon.CancelWorkflowResponse, error)
	pauseWorkflow     func(context.Context, string) (daemon.PauseWorkflowResponse, error)
	resumeWorkflow    func(context.Context, string) (daemon.ResumeWorkflowResponse, error)
	workflowSummary   func(context.Context, string) (daemon.WorkflowSummaryResponse, error)
	workflowTimeline  func(context.Context, string, int, int) (daemon.WorkflowTimelineResponse, error)
	workflowInspect   func(context.Context, string) (daemon.WorkflowInspectResponse, error)
	status            func(context.Context) (daemon.DaemonStatus, error)
	stop              func(context.Context) (daemon.StopResponse, error)
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
func (f workflowDaemonClientFunc) WorkflowArtifacts(ctx context.Context, id string) (daemon.WorkflowArtifactsResponse, error) {
	if f.workflowArtifacts == nil {
		return daemon.WorkflowArtifactsResponse{}, nil
	}
	return f.workflowArtifacts(ctx, id)
}
func (f workflowDaemonClientFunc) WorkflowArtifact(ctx context.Context, runID, artifactID string) (daemon.WorkflowArtifactResponse, error) {
	if f.workflowArtifact == nil {
		return daemon.WorkflowArtifactResponse{}, nil
	}
	return f.workflowArtifact(ctx, runID, artifactID)
}
func (f workflowDaemonClientFunc) WorkflowArtifactPath(ctx context.Context, runID, artifactID string) (string, error) {
	if f.artifactPath == nil {
		return "", nil
	}
	return f.artifactPath(ctx, runID, artifactID)
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
func (f workflowDaemonClientFunc) WorkflowSummary(ctx context.Context, id string) (daemon.WorkflowSummaryResponse, error) {
	if f.workflowSummary == nil {
		return daemon.WorkflowSummaryResponse{}, nil
	}
	return f.workflowSummary(ctx, id)
}
func (f workflowDaemonClientFunc) WorkflowTimeline(ctx context.Context, id string, cursor int, limit int) (daemon.WorkflowTimelineResponse, error) {
	if f.workflowTimeline == nil {
		return daemon.WorkflowTimelineResponse{}, nil
	}
	return f.workflowTimeline(ctx, id, cursor, limit)
}
func (f workflowDaemonClientFunc) WorkflowInspect(ctx context.Context, id string) (daemon.WorkflowInspectResponse, error) {
	if f.workflowInspect == nil {
		return daemon.WorkflowInspectResponse{}, nil
	}
	return f.workflowInspect(ctx, id)
}
func (f workflowDaemonClientFunc) Status(ctx context.Context) (daemon.DaemonStatus, error) {
	return f.status(ctx)
}
func (f workflowDaemonClientFunc) Stop(ctx context.Context) (daemon.StopResponse, error) {
	return f.stop(ctx)
}

func TestMigrateCommandRejectsNonV1Source(t *testing.T) {
	root := t.TempDir()
	v2Path := filepath.Join(root, "v2.yaml")
	content := []byte(`version: "2"
name: already-v2
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(v2Path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"migrate", v2Path, "--to", "2"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-v1 source")
	}
	if !strings.Contains(err.Error(), `migration only supports source version "1"`) {
		t.Fatalf("expected source version error, got %v", err)
	}
}

func TestMigrateCommandRejectsUnsupportedTarget(t *testing.T) {
	root := t.TempDir()
	v1Path := filepath.Join(root, "v1.yaml")
	content := []byte(`version: "1"
name: simple
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(v1Path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"migrate", v1Path, "--to", "3"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported target")
	}
	if !strings.Contains(err.Error(), `unsupported target version`) {
		t.Fatalf("expected target version error, got %v", err)
	}
}

func TestMigrateCommandOutputsV2ToStdout(t *testing.T) {
	root := t.TempDir()
	v1Path := filepath.Join(root, "v1.yaml")
	content := []byte(`version: "1"
name: simple
inputs:
  name:
    type: string
vars:
  env: dev
nodes:
  - id: ok
    kind: bash
    command: echo ${inputs.name}
`)
	if err := os.WriteFile(v1Path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"migrate", v1Path, "--to", "2"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `version: "2"`) {
		t.Fatalf("expected version 2 in output, got %q", got)
	}
	if !strings.Contains(got, "name: simple") {
		t.Fatalf("expected name preserved, got %q", got)
	}
	if !strings.Contains(got, "id: ok") {
		t.Fatalf("expected node id preserved, got %q", got)
	}
}

func TestMigrateCommandWritesToFile(t *testing.T) {
	root := t.TempDir()
	v1Path := filepath.Join(root, "v1.yaml")
	outPath := filepath.Join(root, "v2.yaml")
	content := []byte(`version: "1"
name: simple
nodes:
  - id: ok
    kind: noop
`)
	if err := os.WriteFile(v1Path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"migrate", v1Path, "--to", "2", "--out", outPath})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "migrated workflow written to") {
		t.Fatalf("expected written message, got %q", out.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `version: "2"`) {
		t.Fatalf("expected version 2 in file, got %q", string(data))
	}
}

func TestMigrateAndValidateEndToEnd(t *testing.T) {
	root := t.TempDir()
	v1Path := filepath.Join(root, "v1.yaml")
	v2Path := filepath.Join(root, "v2.yaml")
	content := []byte(`version: "1"
name: e2e-migrate
inputs:
  count:
    type: integer
vars:
  env: test
defaults:
  timeout: 60
execution:
  max_concurrency: 2
worktree: true
nodes:
  - id: plan
    kind: bash
    command: echo "plan"
  - id: build
    kind: bash
    command: echo "build"
    depends_on: [plan]
`)
	if err := os.WriteFile(v1Path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"migrate", v1Path, "--to", "2", "--out", v2Path})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	cmd = NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"validate", v2Path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected migrated workflow to validate: %v", err)
	}
	if !strings.Contains(out.String(), "valid: e2e-migrate") {
		t.Fatalf("expected validation success, got %q", out.String())
	}
}

func TestWorkflowSummaryRendersTextAndJson(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowSummary: func(ctx context.Context, id string) (daemon.WorkflowSummaryResponse, error) {
				return daemon.WorkflowSummaryResponse{
					RunID: id,
					Summary: corerun.Summary{
						RunID: id, Workflow: "build", Status: "success",
						AgentCalls: 2, BashCalls: 5, FailedNodes: 0, Retries: 1,
						ArtifactCount: 3, DurationMS: 12345,
					},
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "summary", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "Workflow summary") {
		t.Fatalf("expected summary header, got %q", got)
	}
	if !strings.Contains(got, "agent_calls: 2") {
		t.Fatalf("expected agent_calls, got %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "summary", "run-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"agent_calls":2`) {
		t.Fatalf("expected json output, got %q", out.String())
	}
}

func TestWorkflowTimelineRendersTextAndJson(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowTimeline: func(ctx context.Context, id string, cursor int, limit int) (daemon.WorkflowTimelineResponse, error) {
				return daemon.WorkflowTimelineResponse{
					RunID: id,
					Entries: []corerun.TimelineEntry{
						{Timestamp: time.Unix(100, 0).UTC(), Type: "run.started"},
						{Timestamp: time.Unix(200, 0).UTC(), Type: "node.completed", NodeID: "plan", DurationMS: 1000},
					},
					NextCursor: 2,
					HasMore:    false,
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "timeline", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "run.started") {
		t.Fatalf("expected run.started, got %q", got)
	}
	if !strings.Contains(got, "node.completed") {
		t.Fatalf("expected node.completed, got %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "timeline", "run-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"type":"run.started"`) {
		t.Fatalf("expected json output, got %q", out.String())
	}
}

func TestWorkflowTimelineHandlesEmptyEntries(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowTimeline: func(ctx context.Context, id string, cursor int, limit int) (daemon.WorkflowTimelineResponse, error) {
				return daemon.WorkflowTimelineResponse{RunID: id}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "timeline", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No timeline entries") {
		t.Fatalf("expected empty message, got %q", out.String())
	}
}

func TestWorkflowInspectRendersTextAndJson(t *testing.T) {
	oldClient := newDaemonClient
	newDaemonClient = func(socketPath string) workflowDaemonClient {
		return workflowDaemonClientFunc{
			workflowInspect: func(ctx context.Context, id string) (daemon.WorkflowInspectResponse, error) {
				return daemon.WorkflowInspectResponse{
					RunID: id, Workflow: "build", Status: "failed",
					TotalSteps: 3, FailedNodes: 1, Retries: 2,
					AgentCalls: 1, BashCalls: 4, NodeCount: 3, ArtifactCount: 2,
					FirstError: "node plan failed", Error: "node plan failed",
				}, nil
			},
		}
	}
	t.Cleanup(func() { newDaemonClient = oldClient })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "inspect", "run-1", "--no-color"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "node_count: 3") {
		t.Fatalf("expected node_count, got %q", got)
	}
	if !strings.Contains(got, "first_error: node plan failed") {
		t.Fatalf("expected first_error, got %q", got)
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "inspect", "run-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"failed_nodes":1`) {
		t.Fatalf("expected json output, got %q", out.String())
	}
}

func TestWorkflowScheduleCommandExists(t *testing.T) {
	cmd := NewRootCommand()
	workflow := findSubcommand(cmd, "workflow")
	if workflow == nil {
		t.Fatal("expected workflow command")
	}
	if schedule := findSubcommand(workflow, "schedule"); schedule == nil {
		t.Fatal("expected schedule subcommand")
	}
}

func TestWorkflowScheduleAddListRemove(t *testing.T) {
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
	t.Setenv("HOME", dir)
	t.Setenv("AGENTFLOW_PATH", "/tmp/agentflow")

	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "nightly.yaml"), []byte(`
version: "1"
name: nightly
nodes:
  - id: plan
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeInstaller := &fakeScheduleInstaller{}
	oldInstaller := newScheduleDispatcherInstaller
	newScheduleDispatcherInstaller = func() scheduleDispatcherInstaller { return fakeInstaller }
	t.Cleanup(func() { newScheduleDispatcherInstaller = oldInstaller })

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "schedule", "add", "nightly", "--every", "15m", "--tag", "nightly-run", "--input", "flag=true"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add schedule: %v\n%s", err, out.String())
	}
	if fakeInstaller.ensureCalls != 1 {
		t.Fatalf("expected installer ensure call, got %d", fakeInstaller.ensureCalls)
	}
	if fakeInstaller.binaryPath != "/tmp/agentflow" {
		t.Fatalf("expected agentflow binary path, got %q", fakeInstaller.binaryPath)
	}

	registry := newScheduleRegistry()
	schedules, err := registry.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(schedules))
	}
	scheduleID := schedules[0].ID
	if schedules[0].ScheduleType != "every" || schedules[0].Every != "15m0s" {
		t.Fatalf("unexpected schedule stored: %#v", schedules[0])
	}
	if !strings.Contains(out.String(), scheduleID) {
		t.Fatalf("expected schedule id in add output, got %q", out.String())
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "schedule", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list schedules: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), scheduleID) || !strings.Contains(out.String(), `"schedule_type":"every"`) {
		t.Fatalf("unexpected list output: %q", out.String())
	}

	out.Reset()
	cmd = NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"workflow", "schedule", "remove", scheduleID})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("remove schedule: %v\n%s", err, out.String())
	}
	if fakeInstaller.removeCalls != 1 {
		t.Fatalf("expected installer remove call, got %d", fakeInstaller.removeCalls)
	}
	schedules, err = registry.List()
	if err != nil {
		t.Fatalf("list schedules after remove: %v", err)
	}
	if len(schedules) != 0 {
		t.Fatalf("expected no schedules after remove, got %#v", schedules)
	}
}

type fakeScheduleInstaller struct {
	ensureCalls int
	removeCalls int
	binaryPath  string
}

func (f *fakeScheduleInstaller) Ensure(ctx context.Context, binaryPath string) error {
	f.ensureCalls++
	f.binaryPath = binaryPath
	return nil
}

func (f *fakeScheduleInstaller) Remove(ctx context.Context) error {
	f.removeCalls++
	return nil
}
