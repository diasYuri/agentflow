//go:build e2e && integration

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemon_Lifecycle(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	// Start
	res := h.Run("daemon", "start")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "agentflowd started") && !strings.Contains(res.Stdout, "agentflowd starting") {
		t.Fatalf("unexpected daemon start output:\n%s", res.Stdout)
	}

	// Status
	res = h.Run("daemon", "status")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "running: true") {
		t.Fatalf("expected daemon running status, got:\n%s", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "socket:") {
		t.Fatalf("expected socket in status, got:\n%s", res.Stdout)
	}

	// Verify socket and PID files exist inside the isolated home
	socketPath := filepath.Join(h.Home, ".agentflow", "agentflowd.sock")
	pidPath := filepath.Join(h.Home, ".agentflow", "agentflowd.pid")
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("socket file not found at %s: %v", socketPath, err)
	}
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("pid file not found at %s: %v", pidPath, err)
	}

	// Stop
	res = h.Run("daemon", "stop")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "agentflowd stopping") {
		t.Fatalf("unexpected daemon stop output:\n%s", res.Stdout)
	}
}

func TestDaemon_WorkflowSubmitAndQuery(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	start := h.Run("daemon", "start")
	start.AssertSuccess(t)
	t.Cleanup(func() {
		h.Run("daemon", "stop")
	})

	root := repoRoot(t)
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "hello.yaml"), "hello.yaml")
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "inputs.json"), "inputs.json")
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "fake-responses.json"), "fake.json")

	// Submit workflow
	res := h.Run("run", "hello.yaml",
		"--input-json", filepath.Join(h.Workspace, "inputs.json"),
		"--fake-provider-path", filepath.Join(h.Workspace, "fake.json"),
	)
	res.AssertSuccess(t)
	runID := extractRunID(t, res.Stdout)

	// Wait for completion
	pollWorkflowStatus(t, h, runID, "success", 15*time.Second)

	// List should contain the run
	res = h.Run("workflow", "list", "--no-color")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, runID) {
		t.Fatalf("expected run %s in list output:\n%s", runID, res.Stdout)
	}

	// Status should show success
	res = h.Run("workflow", "status", runID, "--no-color")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "status: success") {
		t.Fatalf("expected success status:\n%s", res.Stdout)
	}

	// Logs should contain run events
	res = h.Run("workflow", "logs", runID)
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "run.started") {
		t.Fatalf("expected run.started in logs:\n%s", res.Stdout)
	}

	// Watch should return immediately because the run is already finished
	res = h.Run("workflow", "watch", runID, "--no-color")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "status: success") {
		t.Fatalf("expected success in watch output:\n%s", res.Stdout)
	}
}

func TestDaemon_RelativeDaemonPathsUseCallerCwd(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	start := h.Run("daemon", "start")
	start.AssertSuccess(t)
	t.Cleanup(func() {
		h.Run("daemon", "stop")
	})

	root := repoRoot(t)
	workflowPath := h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "hello.yaml"), "hello.yaml")

	cwd := t.TempDir()
	fakePath := filepath.Join(cwd, "fake.json")
	if err := os.WriteFile(fakePath, []byte(`{"greet":{"text":"Hello, caller!"}}`), 0o644); err != nil {
		t.Fatalf("write fake provider config: %v", err)
	}

	res := h.RunInDir(cwd, "run", workflowPath,
		"--fake-provider-path", "fake.json",
		"--events-jsonl", "events.jsonl",
	)
	res.AssertSuccess(t)
	runID := extractRunID(t, res.Stdout)
	if runID == "" {
		t.Fatal("expected non-empty run id")
	}

	pollWorkflowStatus(t, h, runID, "success", 10*time.Second)

	eventsPath := filepath.Join(cwd, "events.jsonl")
	if _, err := os.Stat(eventsPath); err != nil {
		t.Fatalf("expected events file at caller cwd %s: %v", eventsPath, err)
	}
	if _, err := os.Stat(filepath.Join(h.Workspace, "events.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("expected no events file in daemon workspace, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(h.Workspace, "fake.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no fake provider file in daemon workspace, got err=%v", err)
	}
}

func TestDaemon_CancelWorkflow(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	start := h.Run("daemon", "start")
	start.AssertSuccess(t)
	t.Cleanup(func() {
		h.Run("daemon", "stop")
	})

	root := repoRoot(t)
	h.StageFile(t, filepath.Join(root, "test", "e2e", "testdata", "slow.yaml"), "slow.yaml")

	// Submit slow workflow
	res := h.Run("run", "slow.yaml")
	res.AssertSuccess(t)
	runID := extractRunID(t, res.Stdout)

	// Wait for it to start running
	pollWorkflowStatus(t, h, runID, "running", 10*time.Second)

	// Cancel
	res = h.Run("workflow", "cancel", runID)
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "status: cancelled") && !strings.Contains(res.Stdout, "status: canceled") {
		t.Fatalf("expected cancelled status:\n%s", res.Stdout)
	}

	// Verify final status
	pollWorkflowStatus(t, h, runID, "cancelled", 10*time.Second)
}

func TestDaemon_PauseAndResume(t *testing.T) {
	h := New(t)
	h.Build()
	h.Setenv("AGENTFLOWD_PATH", h.AgentflowdPath)

	start := h.Run("daemon", "start")
	start.AssertSuccess(t)
	t.Cleanup(func() {
		h.Run("daemon", "stop")
	})

	root := repoRoot(t)
	h.StageFile(t, filepath.Join(root, "samples", "workflows", "pause-on-failure.yaml"), "pause.yaml")
	flagFile := filepath.Join(h.Workspace, "flag.txt")

	// Run without creating the flag file; gate node will fail and pause
	res := h.Run("run", "pause.yaml", "--input", "flag_file="+flagFile)
	res.AssertSuccess(t)
	runID := extractRunID(t, res.Stdout)

	// Poll for paused status
	pollWorkflowStatus(t, h, runID, "paused", 10*time.Second)

	// Create flag file so resume can succeed
	if err := os.WriteFile(flagFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("create flag file: %v", err)
	}

	// Resume
	res = h.Run("workflow", "resume", runID)
	res.AssertSuccess(t)

	// Poll for final success
	pollWorkflowStatus(t, h, runID, "success", 10*time.Second)

	// Verify resume count is reflected in status
	res = h.Run("workflow", "status", runID, "--no-color")
	res.AssertSuccess(t)
	if !strings.Contains(res.Stdout, "resume_count: 1") {
		t.Fatalf("expected resume_count in status:\n%s", res.Stdout)
	}
}
