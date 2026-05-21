package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
)

func testTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/Users/yuri/git/diasYuri/agentflow/.tmp", "queue-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestDefaultConfigUsesThreeConcurrentRuns(t *testing.T) {
	if got := DefaultConfig().MaxConcurrentRuns; got != 3 {
		t.Fatalf("expected default max concurrent runs 3, got %d", got)
	}
}

func TestManagerStartsWorkflowAsQueued(t *testing.T) {
	dir := testTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "noop.yaml"), []byte(`
version: "1"
name: noop
nodes:
  - id: ok
    kind: bash
    command: "sleep 5"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-queue")
	go func() {
		_ = runSupervisor.Serve(ctx)
	}()

	manager := NewManager(cfg, runSupervisor, nil)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "noop", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != corerun.RunQueued {
		t.Fatalf("expected status queued, got %s", run.Status)
	}
	if run.Priority != 0 {
		t.Fatalf("expected priority 0, got %d", run.Priority)
	}
}

func TestManagerQueueRespectsPriority(t *testing.T) {
	dir := testTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "noop.yaml"), []byte(`
version: "1"
name: noop
nodes:
  - id: ok
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SocketPath:        filepath.Join(dir, "agentflowd.sock"),
		PIDPath:           filepath.Join(dir, "agentflowd.pid"),
		LogPath:           filepath.Join(dir, "agentflowd.log"),
		RunRoot:           filepath.Join(dir, "runs"),
		DBPath:            filepath.Join(dir, "agentflowd.sqlite"),
		MaxConcurrentRuns: 1,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-queue-priority")
	go func() {
		_ = runSupervisor.Serve(ctx)
	}()

	manager := NewManager(cfg, runSupervisor, nil)

	run1, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "noop", WorkingDir: dir, Priority: 1})
	if err != nil {
		t.Fatal(err)
	}

	run2, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "noop", WorkingDir: dir, Priority: 5})
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	queue := make([]string, len(manager.queue))
	copy(queue, manager.queue)
	manager.mu.Unlock()

	if len(queue) != 1 {
		t.Fatalf("expected queue length 1 after first promotion, got %d", len(queue))
	}
	if queue[0] != run2.ID {
		t.Fatalf("expected high-priority run %s in queue, got %s", run2.ID, queue[0])
	}
	_ = run1
}

func TestManagerCancelQueuedRun(t *testing.T) {
	dir := testTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "noop.yaml"), []byte(`
version: "1"
name: noop
nodes:
  - id: ok
    kind: noop
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-cancel-queue")
	go func() {
		_ = runSupervisor.Serve(ctx)
	}()

	manager := NewManager(cfg, runSupervisor, nil)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "noop", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := manager.CancelWorkflow(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != corerun.RunCancelled {
		t.Fatalf("expected status cancelled, got %s", cancelled.Status)
	}

	manager.mu.Lock()
	queueLen := len(manager.queue)
	manager.mu.Unlock()
	if queueLen != 0 {
		t.Fatalf("expected empty queue after cancel, got %d", queueLen)
	}
}

func TestManagerPromotesNewRunAfterWorkflowPauses(t *testing.T) {
	dir := testTempDir(t)
	manager, cleanup := newQueueTestManager(t, dir, 1)
	defer cleanup()

	writeQueueWorkflow(t, dir, "pauseable.yaml", `
version: "1"
name: pauseable
execution:
  pause_when_fail: true
nodes:
  - id: fail
    kind: bash
    retries: 0
    command: "exit 1"
`)
	writeQueueWorkflow(t, dir, "slow.yaml", `
version: "1"
name: slow
nodes:
  - id: wait
    kind: bash
    command: "sleep 5"
`)

	pausedRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "pauseable", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForQueueStatus(t, manager, pausedRun.ID, corerun.RunPaused, 3*time.Second)

	nextRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "slow", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForQueueStatus(t, manager, nextRun.ID, corerun.RunRunning, 3*time.Second)
	_, _ = manager.CancelWorkflow(nextRun.ID)
}

func TestManagerQueuesResumeWhenConcurrencyIsFull(t *testing.T) {
	dir := testTempDir(t)
	manager, cleanup := newQueueTestManager(t, dir, 1)
	defer cleanup()

	flagPath := filepath.Join(dir, "fail-once.flag")
	writeQueueWorkflow(t, dir, "pauseable.yaml", `
version: "1"
name: pauseable
execution:
  pause_when_fail: true
nodes:
  - id: flaky
    kind: bash
    retries: 0
    command: "if [ ! -f `+flagPath+` ]; then touch `+flagPath+`; exit 1; fi; echo ok"
`)
	writeQueueWorkflow(t, dir, "slow.yaml", `
version: "1"
name: slow
nodes:
  - id: wait
    kind: bash
    command: "sleep 5"
`)

	pausedRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "pauseable", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForQueueStatus(t, manager, pausedRun.ID, corerun.RunPaused, 3*time.Second)

	activeRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "slow", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitForQueueStatus(t, manager, activeRun.ID, corerun.RunRunning, 3*time.Second)

	resumed, err := manager.ResumeWorkflow(pausedRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != corerun.RunQueued || !resumed.ResumeQueued {
		t.Fatalf("expected resume to queue while capacity is full, got %#v", resumed)
	}

	_, _ = manager.CancelWorkflow(activeRun.ID)
	waitForQueueStatus(t, manager, pausedRun.ID, corerun.RunSuccess, 5*time.Second)
}

func TestManagerRestartMarksRunningAsFailed(t *testing.T) {
	dir := testTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
		DBPath:     filepath.Join(dir, "agentflowd.sqlite"),
	}
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := WorkflowRun{
		ID:        "test-running-restart",
		Workflow:  "noop",
		RunDir:    filepath.Join(dir, "runs", "test-running-restart"),
		Status:    corerun.RunRunning,
		StartedAt: time.Now().Add(-time.Hour),
	}
	if err := store.UpsertRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	runSupervisor := suture.NewSimple("test-restart")
	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)

	loaded, ok := manager.WorkflowStatus("test-running-restart")
	if !ok {
		t.Fatal("expected run to be loaded")
	}
	if loaded.Status != corerun.RunFailed {
		t.Fatalf("expected status failed after restart, got %s", loaded.Status)
	}
	if loaded.FailureReason != "daemon_restarted" {
		t.Fatalf("expected failure_reason daemon_restarted, got %q", loaded.FailureReason)
	}
}

func TestManagerPromotesQueuedRunOnStartupWhenCapacityExists(t *testing.T) {
	dir := testTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workflowDir := filepath.Join(dir, ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "slow.yaml"), []byte(`
version: "1"
name: slow
nodes:
  - id: wait
    kind: bash
    command: "sleep 5"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		SocketPath:        filepath.Join(dir, "agentflowd.sock"),
		PIDPath:           filepath.Join(dir, "agentflowd.pid"),
		LogPath:           filepath.Join(dir, "agentflowd.log"),
		RunRoot:           filepath.Join(dir, "runs"),
		DBPath:            filepath.Join(dir, "agentflowd.sqlite"),
		MaxConcurrentRuns: 3,
	}
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	run := WorkflowRun{
		ID:        "test-queued-startup",
		Workflow:  "slow",
		RunDir:    filepath.Join(dir, "runs", "test-queued-startup"),
		Status:    corerun.RunQueued,
		StartedAt: time.Now().Add(-time.Hour),
		QueuedAt:  time.Now().Add(-time.Hour),
		Request: &RunWorkflowRequest{
			WorkflowRef: "slow",
			WorkingDir:  dir,
		},
	}
	if err := store.UpsertRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-startup-promotion")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		loaded, ok := manager.WorkflowStatus(run.ID)
		if ok && loaded.Status == corerun.RunRunning {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	loaded, ok := manager.WorkflowStatus(run.ID)
	if !ok {
		t.Fatal("expected run to be loaded")
	}
	t.Fatalf("expected queued run to promote on startup, got %#v", loaded)
}

func newQueueTestManager(t *testing.T, dir string, maxConcurrentRuns int) (*Manager, func()) {
	t.Helper()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	cfg := Config{
		SocketPath:        filepath.Join(dir, "agentflowd.sock"),
		PIDPath:           filepath.Join(dir, "agentflowd.pid"),
		LogPath:           filepath.Join(dir, "agentflowd.log"),
		RunRoot:           filepath.Join(dir, "runs"),
		DBPath:            filepath.Join(dir, "agentflowd.sqlite"),
		MaxConcurrentRuns: maxConcurrentRuns,
	}
	ctx, cancel := context.WithCancel(context.Background())
	runSupervisor := suture.NewSimple("test-queue-lifecycle")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	cleanup := func() {
		cancel()
		<-done
	}
	return NewManager(cfg, runSupervisor, nil), cleanup
}

func writeQueueWorkflow(t *testing.T, dir, name, content string) {
	t.Helper()
	workflowDir := filepath.Join(dir, "home", ".agentflow", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func waitForQueueStatus(t *testing.T, manager *Manager, runID string, want corerun.RunStatus, timeout time.Duration) WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got, ok := manager.WorkflowStatus(runID)
		if ok && got.Status == want {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, ok := manager.WorkflowStatus(runID)
	if !ok {
		t.Fatalf("expected run %q to exist", runID)
	}
	t.Fatalf("expected run %q status %s, got %#v", runID, want, got)
	return WorkflowRun{}
}
