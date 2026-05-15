package daemon

import (
	"context"
	"os"
	"path/filepath"
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
