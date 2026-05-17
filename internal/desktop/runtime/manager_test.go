package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	jsonlevents "github.com/diasYuri/agentflow/internal/adapters/events/jsonl"
	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

func TestManagerRunWorkflowAndCancel(t *testing.T) {
	tmp := t.TempDir()
	workflowPath := filepath.Join(tmp, "test.yaml")
	workflowYAML := `version: "1"
name: test-workflow
nodes:
  - id: hello
    kind: bash
    command: echo hello
`
	if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ucFactory := func(runRoot, eventsJSONL string) (*runworkflow.RunWorkflowUseCase, error) {
		eventSink, err := jsonlevents.New(eventsJSONL)
		if err != nil {
			return nil, err
		}
		return &runworkflow.RunWorkflowUseCase{
			Workflows: yamlrepo.NewWorkflowRepository(),
			Runs:      runrepo.New(runRoot),
			Events:    eventSink,
			Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{}),
			Shell:     shell.NewRunner(),
			Worktrees: ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{}),
		}, nil
	}

	m := NewManagerWithFactory(tmp, ucFactory)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	summary, err := m.RunWorkflow(ctx, RunRequest{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}
	if summary.ID == "" {
		t.Fatal("expected run id")
	}
	if summary.Status != "created" && summary.Status != "running" {
		t.Fatalf("unexpected initial status: %s", summary.Status)
	}

	// Cancelamento imediato
	cancelled, err := m.CancelRun(summary.ID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %s", cancelled.Status)
	}
}

func TestManagerListRuns(t *testing.T) {
	tmp := t.TempDir()

	// Simular run persistida
	runDir := filepath.Join(tmp, "persisted-run")
	_ = os.MkdirAll(filepath.Join(runDir, "nodes"), 0o755)
	meta := `{"run_id":"persisted-run","workflow":"wf","started_at":"2024-01-01T00:00:00Z","output_dir":"` + runDir + `","tag":"prod-deploy"}`
	_ = os.WriteFile(filepath.Join(runDir, "run.json"), []byte(meta), 0o644)
	plan := `{"order":["a","b"]}`
	_ = os.WriteFile(filepath.Join(runDir, "plan.json"), []byte(plan), 0o644)

	m := NewManagerWithFactory(tmp, nil)

	runs, err := m.ListRuns()
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "persisted-run" {
		t.Fatalf("unexpected run id: %s", runs[0].ID)
	}
	if runs[0].Tag != "prod-deploy" {
		t.Fatalf("expected tag prod-deploy, got %s", runs[0].Tag)
	}
}

func TestManagerGetRunEvents(t *testing.T) {
	tmp := t.TempDir()
	runDir := filepath.Join(tmp, "run-1")
	_ = os.MkdirAll(runDir, 0o755)

	events := []string{
		`{"ts":"2024-01-01T00:00:00Z","run_id":"run-1","type":"run.started"}`,
		`{"ts":"2024-01-01T00:00:01Z","run_id":"run-1","type":"node.started","node_id":"a"}`,
		`{"ts":"2024-01-01T00:00:02Z","run_id":"run-1","type":"node.completed","node_id":"a"}`,
	}
	var data []byte
	for _, line := range events {
		data = append(data, []byte(line+"\n")...)
	}
	_ = os.WriteFile(filepath.Join(runDir, "events.jsonl"), data, 0o644)

	resp, err := GetRunEvents(runDir, 0, 10)
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(resp.Events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(resp.Events))
	}
	if resp.Events[0].Type != "run.started" {
		t.Fatalf("unexpected first event type: %s", resp.Events[0].Type)
	}
	if resp.NextCursor != 3 {
		t.Fatalf("expected next cursor 3, got %d", resp.NextCursor)
	}

	// Paginacao
	resp2, err := GetRunEvents(runDir, 1, 1)
	if err != nil {
		t.Fatalf("get events page: %v", err)
	}
	if len(resp2.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(resp2.Events))
	}
	if resp2.Events[0].Type != "node.started" {
		t.Fatalf("unexpected event type: %s", resp2.Events[0].Type)
	}
	if !resp2.HasMore {
		t.Fatal("expected has_more true")
	}
	if resp2.NextCursor != 2 {
		t.Fatalf("expected next cursor 2, got %d", resp2.NextCursor)
	}
}

func TestManagerConfigureUpdatesOptionsForNewRuns(t *testing.T) {
	m := NewManager(t.TempDir(), "old-codex", "old-claude", "old-pi", "text")

	m.Configure("new-codex", "new-claude", "new-pi", "json")

	if m.codexPath != "new-codex" {
		t.Errorf("expected codex path to be updated, got %s", m.codexPath)
	}
	if m.claudePath != "new-claude" {
		t.Errorf("expected claude path to be updated, got %s", m.claudePath)
	}
	if m.piPath != "new-pi" {
		t.Errorf("expected pi path to be updated, got %s", m.piPath)
	}
	if m.logFormat != "json" {
		t.Errorf("expected log format json, got %s", m.logFormat)
	}
}

func TestManagerShutdown(t *testing.T) {
	tmp, err := os.MkdirTemp("", "agentflow-shutdown-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	workflowPath := filepath.Join(tmp, "test.yaml")
	workflowYAML := `version: "1"
name: test-workflow
nodes:
  - id: sleep
    kind: bash
    command: sleep 2
`
	if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ucFactory := func(runRoot, eventsJSONL string) (*runworkflow.RunWorkflowUseCase, error) {
		eventSink, err := jsonlevents.New(eventsJSONL)
		if err != nil {
			return nil, err
		}
		return &runworkflow.RunWorkflowUseCase{
			Workflows: yamlrepo.NewWorkflowRepository(),
			Runs:      runrepo.New(runRoot),
			Events:    eventSink,
			Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{}),
			Shell:     shell.NewRunner(),
			Worktrees: ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{}),
		}, nil
	}

	m := NewManagerWithFactory(tmp, ucFactory)
	ctx := context.Background()

	summary, err := m.RunWorkflow(ctx, RunRequest{WorkflowRef: workflowPath})
	if err != nil {
		t.Fatalf("run workflow: %v", err)
	}

	// Dar um tempo para iniciar
	time.Sleep(100 * time.Millisecond)

	m.Shutdown()

	// Aguardar a goroutine reconhecer o cancelamento
	time.Sleep(300 * time.Millisecond)

	// Verificar que foi cancelada
	m.mu.Lock()
	ar, ok := m.active[summary.ID]
	m.mu.Unlock()
	if !ok {
		t.Fatal("run should still be active")
	}

	ar.mu.Lock()
	status := ar.status
	ar.mu.Unlock()
	if status != "cancelled" {
		t.Fatalf("expected cancelled after shutdown, got %s", status)
	}
}
