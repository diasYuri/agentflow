package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"
)

func TestServerStatusOverUnixSocket(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	runSupervisor := suture.NewSimple("test-workflows")
	manager := NewManager(cfg, runSupervisor, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewServer(cfg, manager, time.Now(), cancel, nil)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	client := NewClient(cfg.SocketPath)
	var status DaemonStatus
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		status, err = client.Status(context.Background())
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !status.Running {
		t.Fatalf("expected daemon status running, got %#v", status)
	}
	if status.Socket != cfg.SocketPath {
		t.Fatalf("expected socket %q, got %q", cfg.SocketPath, status.Socket)
	}
}

func TestManagerRunsWorkflowInBackgroundService(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
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
	if err := os.WriteFile(filepath.Join(workflowDir, "background.yaml"), []byte(`
version: "1"
name: background
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
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	manager := NewManager(cfg, runSupervisor, nil)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "background", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := manager.WorkflowStatus(run.ID)
		if ok && got.Status == "success" {
			if got.RunDir == "" {
				t.Fatal("expected run dir")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := manager.WorkflowStatus(run.ID)
	t.Fatalf("workflow did not complete successfully: %#v", got)
}

func TestManagerAppliesRuntimeOptionsPerWorkflowRun(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
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
	if err := os.WriteFile(filepath.Join(workflowDir, "per-run.yaml"), []byte(`
version: "1"
name: per-run
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
		RunRoot:    filepath.Join(dir, "default-runs"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	manager := NewManager(cfg, runSupervisor, nil)
	runRoot := filepath.Join(dir, "request-runs")
	eventsPath := filepath.Join(dir, "request-events.jsonl")
	run, err := manager.StartWorkflow(RunWorkflowRequest{
		WorkflowRef: "per-run",
		WorkingDir:  dir,
		OutputDir:   runRoot,
		EventsJSONL: eventsPath,
		LogFormat:   "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(run.RunDir, runRoot) {
		t.Fatalf("initial run dir mismatch: got %q want prefix %q", run.RunDir, runRoot)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := manager.WorkflowStatus(run.ID)
		if ok && got.Status == "success" {
			if !strings.HasPrefix(got.RunDir, runRoot) {
				t.Fatalf("finished run dir mismatch: got %q want prefix %q", got.RunDir, runRoot)
			}
			assertFileContains(t, eventsPath, `"type":"run.completed"`)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := manager.WorkflowStatus(run.ID)
	t.Fatalf("workflow did not complete successfully: %#v", got)
}

func TestManagerLoadsPersistedWorkflowRuns(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
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
	if err := os.WriteFile(filepath.Join(workflowDir, "persisted.yaml"), []byte(`
version: "1"
name: persisted
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
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "persisted", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := manager.WorkflowStatus(run.ID)
		if ok && got.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, _ := manager.WorkflowStatus(run.ID)
	if got.Status != "success" {
		t.Fatalf("workflow did not complete successfully: %#v", got)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reloaded := NewManagerWithStore(cfg, runSupervisor, nil, reopened)
	loaded, ok := reloaded.WorkflowStatus(run.ID)
	if !ok {
		t.Fatalf("expected persisted run %q to load", run.ID)
	}
	if loaded.Status != "success" {
		t.Fatalf("expected persisted success, got %#v", loaded)
	}
	if loaded.RunDir == "" {
		t.Fatal("expected persisted run dir")
	}
}

func TestManagerCancelsRunningWorkflow(t *testing.T) {
	dir := shortTempDir(t)
	t.Setenv("HOME", filepath.Join(dir, "home"))
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
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	manager := NewManager(cfg, runSupervisor, nil)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "slow", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(run.ID)
		if got.Status == "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancelled, err := manager.CancelWorkflow(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %#v", cancelled)
	}
}

func TestManagerPauseUnknownRunReturnsNotExist(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	runSupervisor := suture.NewSimple("test-workflows")
	manager := NewManager(cfg, runSupervisor, nil)

	if _, err := manager.PauseWorkflow("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
	if _, err := manager.ResumeWorkflow("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestManagerPausesAndResumesWorkflowOnFailure(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
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
	flagPath := filepath.Join(dir, "fail-once.flag")
	cmd := `if [ ! -f ` + flagPath + ` ]; then touch ` + flagPath + `; exit 1; fi; echo ok`
	if err := os.WriteFile(filepath.Join(workflowDir, "pauseable.yaml"), []byte(`
version: "1"
name: pauseable
execution:
  pause_when_fail: true
nodes:
  - id: flaky
    kind: bash
    retries: 0
    command: "`+cmd+`"
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
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	store, err := OpenSQLiteRunStore(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	manager := NewManagerWithStore(cfg, runSupervisor, nil, store)
	startedRun, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "pauseable", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var paused WorkflowRun
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(startedRun.ID)
		if got.Status == "paused" {
			paused = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if paused.Status != "paused" {
		t.Fatalf("workflow did not reach paused state: %#v", paused)
	}
	if paused.PauseReason == "" {
		t.Fatalf("expected pause reason, got empty")
	}

	resumed, err := manager.ResumeWorkflow(startedRun.ID)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if resumed.Status != "running" {
		t.Fatalf("expected running status after resume, got %s", resumed.Status)
	}

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(startedRun.ID)
		if got.Status == "success" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, _ := manager.WorkflowStatus(startedRun.ID)
	t.Fatalf("workflow did not complete after resume: %#v", got)
}

func TestManagerRejectsResumeOnTerminalRun(t *testing.T) {
	dir := shortTempDir(t)
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
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
	if err := os.WriteFile(filepath.Join(workflowDir, "ok.yaml"), []byte(`
version: "1"
name: ok
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
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runSupervisor := suture.NewSimple("test-workflows")
	done := make(chan error, 1)
	go func() {
		done <- runSupervisor.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	manager := NewManager(cfg, runSupervisor, nil)
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "ok", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(run.ID)
		if got.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := manager.ResumeWorkflow(run.ID); err == nil {
		t.Fatal("expected error resuming successful run")
	}
}

func TestServerStopCancelsContext(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	runSupervisor := suture.NewSimple("test-workflows")
	manager := NewManager(cfg, runSupervisor, nil)
	server := NewServer(cfg, manager, time.Now(), cancel, nil)
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	client := NewClient(cfg.SocketPath)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := client.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("expected stop endpoint to cancel daemon context")
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/private/tmp", "agentflowd-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func assertFileContains(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("expected %s to contain %q, got %q", path, want, string(data))
	}
}
