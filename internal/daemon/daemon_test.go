package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestManagerPersistsAndLoadsTag(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(workflowDir, "tagged.yaml"), []byte(`
version: "1"
name: tagged
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
	run, err := manager.StartWorkflow(RunWorkflowRequest{WorkflowRef: "tagged", WorkingDir: dir, Tag: "release-42"})
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
	got, _ := manager.WorkflowStatus(run.ID)
	if got.Status != "success" {
		t.Fatalf("workflow did not complete successfully: %#v", got)
	}
	if got.Tag != "release-42" {
		t.Fatalf("expected tag to be persisted, got %q", got.Tag)
	}
	assertFileContains(t, filepath.Join(got.RunDir, "run.json"), `"tag": "release-42"`)
	assertFileContains(t, filepath.Join(got.RunDir, "summary.json"), `"tag": "release-42"`)
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
	if loaded.Tag != "release-42" {
		t.Fatalf("expected persisted tag, got %q", loaded.Tag)
	}
}

func TestSQLiteRunStoreMigratesRunsWithoutTag(t *testing.T) {
	dir := shortTempDir(t)
	dbPath := filepath.Join(dir, "agentflowd.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE workflow_runs (
	id TEXT PRIMARY KEY,
	workflow TEXT NOT NULL,
	run_dir TEXT NOT NULL,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	error TEXT
);
INSERT INTO workflow_runs (id, workflow, run_dir, status, started_at, finished_at, error)
VALUES ('old-run', 'old-workflow', '/tmp/old-run', 'success', '2026-05-16T12:00:00Z', NULL, '');
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenSQLiteRunStore(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runs, err := store.LoadRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one migrated run, got %d", len(runs))
	}
	if runs[0].ID != "old-run" || runs[0].Tag != "" {
		t.Fatalf("unexpected migrated run: %#v", runs[0])
	}
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
	if resp.Encoding != "text" {
		t.Fatalf("expected text encoding, got %s", resp.Encoding)
	}
	content := resp.TextContent
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
	if artResp.Encoding != "text" {
		t.Fatalf("expected text encoding, got %s", artResp.Encoding)
	}
	if artResp.TextContent != "hello" {
		t.Fatalf("expected hello, got %s", artResp.TextContent)
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

func TestManagerWorkflowArtifactsReadsFromIndex(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-index"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	index := map[string]corerun.Artifact{
		"nodes/n1/stdout.txt": {
			ID: "nodes/n1/stdout.txt", RunID: runID, NodeID: "n1", Name: "stdout.txt",
			RelativePath: "nodes/n1/stdout.txt", MediaType: "text/plain", SizeBytes: 5,
			Kind: corerun.ArtifactKindStdout, CreatedAt: time.Now().UTC(),
		},
		"nodes/n1/result.json": {
			ID: "nodes/n1/result.json", RunID: runID, NodeID: "n1", Name: "result.json",
			RelativePath: "nodes/n1/result.json", MediaType: "application/json", SizeBytes: 256,
			Kind: corerun.ArtifactKindResult, CreatedAt: time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), indexData, 0o644); err != nil {
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
		t.Fatalf("expected 2 artifacts from index, got %d", len(resp.Artifacts))
	}
	for _, a := range resp.Artifacts {
		if a.NodeID != "n1" {
			t.Fatalf("expected node_id n1, got %q", a.NodeID)
		}
	}
}

func TestManagerWorkflowArtifactsFallbackWithoutIndex(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-fallback"
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

	resp, err := manager.WorkflowArtifacts(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact from fallback scan, got %d", len(resp.Artifacts))
	}
	if resp.Artifacts[0].Name != "data.txt" {
		t.Fatalf("expected data.txt, got %s", resp.Artifacts[0].Name)
	}
}

func TestManagerWorkflowArtifactReturnsTextInline(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-text"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts", "nodes", "n1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "nodes", "n1", "stdout.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := map[string]corerun.Artifact{
		"nodes/n1/stdout.txt": {
			ID: "nodes/n1/stdout.txt", RunID: runID, NodeID: "n1", Name: "stdout.txt",
			RelativePath: "nodes/n1/stdout.txt", MediaType: "text/plain", SizeBytes: 11,
			Kind: corerun.ArtifactKindStdout, CreatedAt: time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), indexData, 0o644); err != nil {
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

	resp, err := manager.WorkflowArtifact(runID, "nodes/n1/stdout.txt")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Encoding != "text" {
		t.Fatalf("expected text encoding, got %s", resp.Encoding)
	}
	if resp.TextContent != "hello world" {
		t.Fatalf("expected text content, got %q", resp.TextContent)
	}
	if !resp.IsText {
		t.Fatal("expected is_text true")
	}
}

func TestManagerWorkflowArtifactTruncatesLargeText(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-truncate"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, MaxArtifactInline+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "big.txt"), big, 0o644); err != nil {
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

	resp, err := manager.WorkflowArtifact(runID, "big.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Truncated {
		t.Fatal("expected truncated true")
	}
	if len(resp.TextContent) != MaxArtifactInline {
		t.Fatalf("expected text content length %d, got %d", MaxArtifactInline, len(resp.TextContent))
	}
}

func TestManagerWorkflowArtifactOmitsBinaryByDefault(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-binary"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "image.png"), []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	index := map[string]corerun.Artifact{
		"image.png": {
			ID: "image.png", RunID: runID, Name: "image.png",
			RelativePath: "image.png", MediaType: "image/png", SizeBytes: 4,
			Kind: corerun.ArtifactKindFile, CreatedAt: time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), indexData, 0o644); err != nil {
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

	resp, err := manager.WorkflowArtifact(runID, "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsText {
		t.Fatal("expected is_text false")
	}
	if resp.Content != "" {
		t.Fatalf("expected no inline content for binary, got %d chars", len(resp.Content))
	}
	if !resp.Truncated {
		t.Fatal("expected truncated true for oversized binary")
	}
}

func TestManagerWorkflowArtifactPathRequiresIndex(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-path"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "data.txt"), []byte("x"), 0o644); err != nil {
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

	_, err := manager.WorkflowArtifactPath(runID, "data.txt")
	if err == nil {
		t.Fatal("expected error when index is missing")
	}
}

func TestManagerWorkflowArtifactPathRejectsTraversal(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-path-traversal"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := map[string]corerun.Artifact{
		"../../../run.json": {
			ID: "../../../run.json", RunID: runID, Name: "run.json",
			RelativePath: "../../../run.json", MediaType: "application/json", SizeBytes: 6,
			Kind: corerun.ArtifactKindFile, CreatedAt: time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), indexData, 0o644); err != nil {
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

	_, err := manager.WorkflowArtifactPath(runID, "../../../run.json")
	if err == nil {
		t.Fatal("expected error for traversal path")
	}
}

func TestManagerWorkflowArtifactPathReturnsResolvedPath(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-path-ok"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts", "nodes", "n1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "nodes", "n1", "stdout.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := map[string]corerun.Artifact{
		"nodes/n1/stdout.txt": {
			ID: "nodes/n1/stdout.txt", RunID: runID, NodeID: "n1", Name: "stdout.txt",
			RelativePath: "nodes/n1/stdout.txt", MediaType: "text/plain", SizeBytes: 2,
			Kind: corerun.ArtifactKindStdout, CreatedAt: time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(runDir, "artifacts", "index.json"), indexData, 0o644); err != nil {
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

	p, err := manager.WorkflowArtifactPath(runID, "nodes/n1/stdout.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, "artifacts/nodes/n1/stdout.txt") {
		t.Fatalf("expected path suffix, got %q", p)
	}
}

func TestManagerWorkflowArtifactRejectsSymlinkAncestor(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-symlink-ancestor"
	runDir := filepath.Join(cfg.RunRoot, runID)
	artifactsDir := filepath.Join(runDir, "artifacts")
	targetDir := filepath.Join(dir, "escape")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetDir, filepath.Join(artifactsDir, "link")); err != nil {
		t.Skip("skipping symlink test: " + err.Error())
	}
	index := map[string]corerun.Artifact{
		"link/foo.txt": {
			ID:           "link/foo.txt",
			RunID:        runID,
			Name:         "foo.txt",
			RelativePath: "link/foo.txt",
			MediaType:    "text/plain",
			SizeBytes:    4,
			Kind:         corerun.ArtifactKindFile,
			CreatedAt:    time.Now().UTC(),
		},
	}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(artifactsDir, "index.json"), indexData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "foo.txt"), []byte("data"), 0o644); err != nil {
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

	if _, err := manager.WorkflowArtifact(runID, "link/foo.txt"); err == nil {
		t.Fatal("expected show to reject symlink ancestor")
	}
	if _, err := manager.WorkflowArtifactPath(runID, "link/foo.txt"); err == nil {
		t.Fatal("expected path to reject symlink ancestor")
	}
}

func TestServerArtifactPathRouteSupportsArtifactNamedPath(t *testing.T) {
	dir := shortTempDir(t)
	cfg := Config{RunRoot: filepath.Join(dir, "runs")}
	manager := NewManager(cfg, nil, nil)

	runID := "test-run-artifact-path"
	runDir := filepath.Join(cfg.RunRoot, runID)
	if err := os.MkdirAll(filepath.Join(runDir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	artifactsDir := filepath.Join(runDir, "artifacts")
	artifact := corerun.Artifact{
		ID:           "path",
		RunID:        runID,
		Name:         "path",
		RelativePath: "path",
		MediaType:    "text/plain",
		SizeBytes:    4,
		Kind:         corerun.ArtifactKindFile,
		CreatedAt:    time.Now().UTC(),
	}
	index := map[string]corerun.Artifact{"path": artifact}
	indexData, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(artifactsDir, "index.json"), indexData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "path"), []byte("data"), 0o644); err != nil {
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

	server := NewServer(cfg, manager, time.Now(), func() {}, nil)

	showReq := httptest.NewRequest(http.MethodGet, "/v1/workflows/"+runID+"/artifacts/path", nil)
	showRR := httptest.NewRecorder()
	server.handleWorkflow(showRR, showReq)
	if showRR.Code != http.StatusOK {
		t.Fatalf("expected show route to succeed, got %d: %s", showRR.Code, showRR.Body.String())
	}
	var showResp WorkflowArtifactResponse
	if err := json.Unmarshal(showRR.Body.Bytes(), &showResp); err != nil {
		t.Fatalf("unmarshal show response: %v", err)
	}
	if showResp.ID != "path" {
		t.Fatalf("expected artifact id path, got %q", showResp.ID)
	}

	pathReq := httptest.NewRequest(http.MethodGet, "/v1/workflows/"+runID+"/artifact-path?artifact_id=path", nil)
	pathRR := httptest.NewRecorder()
	server.handleWorkflow(pathRR, pathReq)
	if pathRR.Code != http.StatusOK {
		t.Fatalf("expected path route to succeed, got %d: %s", pathRR.Code, pathRR.Body.String())
	}
	var pathResp map[string]string
	if err := json.Unmarshal(pathRR.Body.Bytes(), &pathResp); err != nil {
		t.Fatalf("unmarshal path response: %v", err)
	}
	if pathResp["artifact_id"] != "path" {
		t.Fatalf("expected artifact_id path, got %q", pathResp["artifact_id"])
	}
	if !strings.HasSuffix(pathResp["path"], filepath.Join("artifacts", "path")) {
		t.Fatalf("expected resolved path, got %q", pathResp["path"])
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
