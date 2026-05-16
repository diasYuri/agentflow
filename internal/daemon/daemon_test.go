package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thejerf/suture/v4"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
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
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(run.ID)
		if got.Status != "cancelled" {
			t.Fatalf("expected cancellation to remain terminal, got %#v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestManagerCancelWinsOverLatePausedFinish(t *testing.T) {
	dir := shortTempDir(t)
	manager := NewManager(Config{RunRoot: filepath.Join(dir, "runs")}, nil, nil)
	runID := "run-cancel-race"
	manager.records[runID] = &runRecord{run: WorkflowRun{
		ID:          runID,
		Workflow:    "race",
		RunDir:      filepath.Join(dir, "runs", runID),
		Status:      corerun.RunPaused,
		StartedAt:   time.Now(),
		PausedAt:    time.Now(),
		PauseReason: string(corerun.PauseReasonManual),
	}}

	cancelled, err := manager.CancelWorkflow(runID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != corerun.RunCancelled {
		t.Fatalf("expected cancelled status, got %#v", cancelled)
	}

	manager.finish(runID, runworkflow.RunResult{
		RunID:  runID,
		RunDir: filepath.Join(dir, "runs", runID),
		Status: corerun.RunPaused,
		Summary: corerun.Summary{
			RunID:  runID,
			Status: corerun.RunPaused,
			Nodes: map[string]corerun.NodeResult{
				"wait": {RunID: runID, NodeID: "wait", Status: corerun.NodeSuccess},
			},
		},
	}, nil)

	got, _ := manager.WorkflowStatus(runID)
	if got.Status != corerun.RunCancelled {
		t.Fatalf("expected cancelled status to survive late paused finish, got %#v", got)
	}
	if !got.PausedAt.IsZero() || got.PauseReason != "" {
		t.Fatalf("expected pause metadata to be cleared after cancel, got %#v", got)
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

func TestManagerWorkflowEventsPagination(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-123"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	events := []corerun.Event{
		{Timestamp: time.Now(), RunID: runID, Type: "run.started", Data: map[string]any{"key": "value1"}},
		{Timestamp: time.Now(), RunID: runID, Type: "node.started", NodeID: "node1"},
		{Timestamp: time.Now(), RunID: runID, Type: "node.completed", NodeID: "node1"},
		{Timestamp: time.Now(), RunID: runID, Type: "run.completed"},
		{Timestamp: time.Now(), RunID: runID, Type: "run.summary"},
	}
	f, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowEvents(runID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}
	if resp.Events[0].Type != "run.started" {
		t.Fatalf("expected first event run.started, got %s", resp.Events[0].Type)
	}
	if resp.Events[1].Type != "node.started" {
		t.Fatalf("expected second event node.started, got %s", resp.Events[1].Type)
	}
	if resp.NextCursor != 2 {
		t.Fatalf("expected next_cursor 2, got %d", resp.NextCursor)
	}
	if !resp.HasMore {
		t.Fatal("expected has_more true")
	}

	resp2, err := manager.WorkflowEvents(runID, resp.NextCursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp2.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp2.Events))
	}
	if resp2.Events[0].Type != "node.completed" {
		t.Fatalf("expected first event node.completed, got %s", resp2.Events[0].Type)
	}
	if resp2.NextCursor != 4 {
		t.Fatalf("expected next_cursor 4, got %d", resp2.NextCursor)
	}
	if !resp2.HasMore {
		t.Fatal("expected has_more true")
	}

	resp3, err := manager.WorkflowEvents(runID, resp2.NextCursor, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp3.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp3.Events))
	}
	if resp3.Events[0].Type != "run.summary" {
		t.Fatalf("expected run.summary, got %s", resp3.Events[0].Type)
	}
	if resp3.HasMore {
		t.Fatal("expected has_more false")
	}
	if resp3.NextCursor != 5 {
		t.Fatalf("expected next_cursor 5, got %d", resp3.NextCursor)
	}
}

func TestManagerWorkflowEventsMissingFile(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-no-events"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunRunning,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowEvents(runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(resp.Events))
	}
	if resp.NextCursor != 0 {
		t.Fatalf("expected next_cursor 0, got %d", resp.NextCursor)
	}
	if resp.HasMore {
		t.Fatal("expected has_more false")
	}
}

func TestManagerWorkflowEventsLimitBounds(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-limit"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		ev := corerun.Event{Timestamp: time.Now(), RunID: runID, Type: "test"}
		if err := json.NewEncoder(f).Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowEvents(runID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 5 {
		t.Fatalf("expected 5 events with default limit, got %d", len(resp.Events))
	}

	resp2, err := manager.WorkflowEvents(runID, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp2.Events) != 5 {
		t.Fatalf("expected 5 events with clamped limit, got %d", len(resp2.Events))
	}

	resp3, err := manager.WorkflowEvents(runID, 0, -5)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp3.Events) != 5 {
		t.Fatalf("expected 5 events with negative limit, got %d", len(resp3.Events))
	}
}

func TestManagerWorkflowEventsMasksSecrets(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-mask"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	secret := "super-secret-value"
	ev := corerun.Event{
		Timestamp: time.Now(),
		RunID:     runID,
		Type:      "node.completed",
		Data:      map[string]any{"output": "result with " + secret},
	}
	f, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(ev); err != nil {
		t.Fatal(err)
	}
	f.Close()

	req := RunWorkflowRequest{
		WorkflowRef: "mask-test",
		Vars:        map[string]any{"SECRET": secret},
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:      runID,
			RunDir:  runDir,
			Status:  corerun.RunSuccess,
			Request: &req,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowEvents(runID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Events))
	}
	data, ok := resp.Events[0].Data["output"].(string)
	if !ok {
		t.Fatalf("expected data.output string, got %T", resp.Events[0].Data["output"])
	}
	if !strings.Contains(data, corerun.MaskReplacement) {
		t.Fatalf("expected masked output, got %q", data)
	}
	if strings.Contains(data, secret) {
		t.Fatalf("expected secret to be masked, got %q", data)
	}
}

func TestManagerWorkflowEventsDoesNotRefreshRunProgress(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-events-no-refresh"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "plan.json"), []byte(`{"order":["node-a","node-b"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	ev := corerun.Event{Timestamp: time.Now(), RunID: runID, Type: "node.started", NodeID: "node-a"}
	if err := json.NewEncoder(f).Encode(ev); err != nil {
		t.Fatal(err)
	}
	f.Close()

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:          runID,
			RunDir:      runDir,
			Status:      corerun.RunRunning,
			CurrentStep: "snapshot-step",
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowEvents(runID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp.Events))
	}

	manager.mu.Lock()
	currentStep := manager.records[runID].run.CurrentStep
	manager.mu.Unlock()
	if currentStep != "snapshot-step" {
		t.Fatalf("expected WorkflowEvents to avoid progress refresh, got current step %q", currentStep)
	}
}

func TestServerWorkflowEventsEndpoint(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	runSupervisor := suture.NewSimple("test-workflows")
	manager := NewManager(cfg, runSupervisor, nil)

	runID := "test-run-events"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(runDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		ev := corerun.Event{Timestamp: time.Now(), RunID: runID, Type: "test"}
		if err := json.NewEncoder(f).Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err := client.WorkflowEvents(context.Background(), runID, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}
	if !resp.HasMore {
		t.Fatal("expected has_more true")
	}
	if resp.NextCursor != 2 {
		t.Fatalf("expected next_cursor 2, got %d", resp.NextCursor)
	}
}

func TestManagerWorkflowArtifactsListsNestedFiles(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-artifacts"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts", "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "top.txt"), []byte("top"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "subdir", "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowArtifacts(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(resp.Artifacts))
	}
	ids := map[string]bool{}
	for _, a := range resp.Artifacts {
		ids[a.ID] = true
	}
	if !ids["top.txt"] {
		t.Fatal("expected top.txt artifact")
	}
	if !ids[filepath.Join("subdir", "nested.txt")] {
		t.Fatal("expected subdir/nested.txt artifact")
	}
}

func TestManagerWorkflowArtifactBlocksTraversal(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-traversal"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	_, err := manager.WorkflowArtifact(runID, "../run.json")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}

	_, err = manager.WorkflowArtifact(runID, "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}

	_, err = manager.WorkflowArtifact(runID, "foo/../../run.json")
	if err == nil {
		t.Fatal("expected error for escaped clean path")
	}
}

func TestManagerWorkflowArtifactMasksSecretsBeforeEncoding(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-artifact-mask"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := "artifact-secret-token"
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "leak.txt"), []byte("value="+secret), 0o644); err != nil {
		t.Fatal(err)
	}
	req := RunWorkflowRequest{
		WorkflowRef: "artifact-mask-test",
		Vars:        map[string]any{"TOKEN": secret},
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:      runID,
			RunDir:  runDir,
			Status:  corerun.RunSuccess,
			Request: &req,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowArtifact(runID, "leak.txt")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Content)
	if err != nil {
		t.Fatal(err)
	}
	content := string(decoded)
	if strings.Contains(content, secret) {
		t.Fatalf("expected artifact content to be masked, got %q", content)
	}
	if !strings.Contains(content, corerun.MaskReplacement) {
		t.Fatalf("expected artifact content to contain mask replacement, got %q", content)
	}
}

func TestServerWorkflowArtifactsEndpoint(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{
		SocketPath: filepath.Join(dir, "agentflowd.sock"),
		PIDPath:    filepath.Join(dir, "agentflowd.pid"),
		LogPath:    filepath.Join(dir, "agentflowd.log"),
		RunRoot:    filepath.Join(dir, "runs"),
	}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-artifacts-api"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "data.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	listResp, err := client.WorkflowArtifacts(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listResp.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(listResp.Artifacts))
	}
	if listResp.Artifacts[0].Name != "data.txt" {
		t.Fatalf("expected data.txt, got %s", listResp.Artifacts[0].Name)
	}

	artResp, err := client.WorkflowArtifact(context.Background(), runID, "data.txt")
	if err != nil {
		t.Fatal(err)
	}
	if artResp.Encoding != "base64" {
		t.Fatalf("expected base64 encoding, got %s", artResp.Encoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(artResp.Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "hello" {
		t.Fatalf("expected hello, got %s", string(decoded))
	}
}

func TestManagerWorkflowNodesListsResults(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-nodes"
	runDir := filepath.Join(cfg.RunRoot, runID)
	nodeDir := filepath.Join(runDir, "nodes", "node1")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := corerun.NodeResult{
		NodeID: "node1",
		Status: corerun.NodeSuccess,
		Output: "out1",
	}
	data, _ := json.Marshal(result)
	if err := os.WriteFile(filepath.Join(nodeDir, "result.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowNodes(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	if resp.Nodes[0].NodeID != "node1" {
		t.Fatalf("expected node1, got %s", resp.Nodes[0].NodeID)
	}
	if resp.Nodes[0].Output != "out1" {
		t.Fatalf("expected out1, got %v", resp.Nodes[0].Output)
	}
}

func TestManagerWorkflowNodeReturnsInstances(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-node-instances"
	runDir := filepath.Join(cfg.RunRoot, runID)
	nodeDir := filepath.Join(runDir, "nodes", "fan")
	if err := os.MkdirAll(filepath.Join(nodeDir, "inst-a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(nodeDir, "inst-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	resA := corerun.NodeResult{NodeID: "fan", InstanceID: "inst-a", Status: corerun.NodeSuccess, Output: "a"}
	resB := corerun.NodeResult{NodeID: "fan", InstanceID: "inst-b", Status: corerun.NodeSuccess, Output: "b"}
	aData, _ := json.Marshal(resA)
	bData, _ := json.Marshal(resB)
	if err := os.WriteFile(filepath.Join(nodeDir, "inst-a", "result.json"), aData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nodeDir, "inst-b", "result.json"), bData, 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowNode(runID, "fan")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(resp.Instances))
	}
}

func TestManagerWorkflowNodeNotFound(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-node-missing"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	_, err := manager.WorkflowNode(runID, "missing")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}
}

func TestManagerWorkflowPlanReturnsFiles(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-plan"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte(`{"run_id":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "workflow.yaml"), []byte("name: w"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "normalized.json"), []byte(`{"nodes":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "plan.json"), []byte(`{"order":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:     runID,
			RunDir: runDir,
			Status: corerun.RunSuccess,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowPlan(runID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.RunID != runID {
		t.Fatalf("expected run_id %s, got %s", runID, resp.RunID)
	}
	if resp.Workflow != "name: w" {
		t.Fatalf("expected workflow yaml, got %q", resp.Workflow)
	}
	if resp.Metadata == nil {
		t.Fatal("expected metadata")
	}
	if resp.Normalized == nil {
		t.Fatal("expected normalized")
	}
	if resp.Plan == nil {
		t.Fatal("expected plan")
	}
}

func TestManagerWorkflowNodesMasksSecrets(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-mask-nodes"
	runDir := filepath.Join(cfg.RunRoot, runID)
	nodeDir := filepath.Join(runDir, "nodes", "n1")
	if err := os.MkdirAll(nodeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	secret := "my-secret-token"
	result := corerun.NodeResult{
		NodeID: "n1",
		Status: corerun.NodeSuccess,
		Output: "output with " + secret,
		Stdout: "stdout with " + secret,
		Stderr: "stderr with " + secret,
		Error:  "error with " + secret,
	}
	data, _ := json.Marshal(result)
	if err := os.WriteFile(filepath.Join(nodeDir, "result.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	req := RunWorkflowRequest{
		WorkflowRef: "mask-nodes-test",
		Vars:        map[string]any{"TOKEN": secret},
	}
	manager.mu.Lock()
	manager.records[runID] = &runRecord{
		run: WorkflowRun{
			ID:      runID,
			RunDir:  runDir,
			Status:  corerun.RunSuccess,
			Request: &req,
		},
	}
	manager.mu.Unlock()

	resp, err := manager.WorkflowNodes(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(resp.Nodes))
	}
	node := resp.Nodes[0]
	for _, field := range []string{node.Output.(string), node.Stdout, node.Stderr, node.Error} {
		if !strings.Contains(field, corerun.MaskReplacement) {
			t.Fatalf("expected masked field, got %q", field)
		}
		if strings.Contains(field, secret) {
			t.Fatalf("expected secret to be masked, got %q", field)
		}
	}
}

func TestServerCompatibilityEndpoints(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(workflowDir, "compat.yaml"), []byte(`
version: "1"
name: compat
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
	server := NewServer(cfg, manager, time.Now(), cancel, nil)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-serverDone
	})

	client := NewClient(cfg.SocketPath)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Status(context.Background()); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running {
		t.Fatal("expected daemon running")
	}

	list, err := client.ListWorkflows(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(list.Runs))
	}

	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "compat", WorkingDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := manager.WorkflowStatus(run.ID)
		if got.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	logs, err := client.WorkflowLogs(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if logs.RunID != run.ID {
		t.Fatalf("expected run_id %s, got %s", run.ID, logs.RunID)
	}

	_, err = client.CancelWorkflow(context.Background(), run.ID)
	if err != nil {
		if !strings.Contains(err.Error(), "is already success") {
			t.Fatalf("unexpected cancel error: %v", err)
		}
	}

	_, err = client.PauseWorkflow(context.Background(), run.ID)
	if err != nil {
		if !strings.Contains(err.Error(), "is already success") {
			t.Fatalf("unexpected pause error: %v", err)
		}
	}

	_, err = client.ResumeWorkflow(context.Background(), run.ID)
	if err != nil {
		if !strings.Contains(err.Error(), "only paused runs can be resumed") {
			t.Fatalf("unexpected resume error: %v", err)
		}
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
