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

func TestManagerRestartReenqueuesQueued(t *testing.T) {
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
		ID:        "test-queued-restart",
		Workflow:  "noop",
		RunDir:    filepath.Join(dir, "runs", "test-queued-restart"),
		Status:    corerun.RunQueued,
		StartedAt: time.Now().Add(-time.Hour),
		QueuedAt:  time.Now().Add(-time.Hour),
	}
	if err := store.UpsertRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}

	runSupervisor := suture.NewSimple("test-restart-queue")
	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)

	loaded, ok := manager.WorkflowStatus("test-queued-restart")
	if !ok {
		t.Fatal("expected run to be loaded")
	}
	if loaded.Status != corerun.RunQueued {
		t.Fatalf("expected status queued after restart, got %s", loaded.Status)
	}

	manager.mu.Lock()
	queueLen := len(manager.queue)
	manager.mu.Unlock()
	if queueLen != 1 {
		t.Fatalf("expected queue length 1 after restart, got %d", queueLen)
	}
}
