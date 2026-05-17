package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/core/ports"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	coreworkflow "github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestExecutePersistsTagInRunJSON(t *testing.T) {
	tmp := t.TempDir()
	repo := runrepo.New(tmp)
	events := &mockEventSink{}

	plan := coreworkflow.ExecutionPlan{
		Workflow: coreworkflow.WorkflowSpec{
			Version: "1",
			Name:    "tag-test",
			Nodes:   []coreworkflow.NodeSpec{{ID: "ok", Kind: coreworkflow.NodeKindNoop}},
		},
		Order: []string{"ok"},
		Nodes: map[string]coreworkflow.PlannedNode{"ok": {Spec: coreworkflow.NodeSpec{ID: "ok", Kind: coreworkflow.NodeKindNoop}}},
	}

	svc := Services{
		Workflows: nil,
		Runs:      repo,
		Events:    events,
		Agents:    ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{}),
		Shell:     nil,
		Worktrees: ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{}),
	}

	_, err := Execute(context.Background(), svc, ExecutionRequest{
		RunID:              "run-abc",
		WorkflowSourcePath: "/tmp/wf.yaml",
		Plan:               plan,
		Inputs:             map[string]any{},
		WorkingDir:         ".",
		Tag:                "smoke-test",
	})
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	runPath := filepath.Join(tmp, "run-abc", "run.json")
	data, err := os.ReadFile(runPath)
	if err != nil {
		t.Fatalf("read run.json: %v", err)
	}
	var meta corerun.RunMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal run.json: %v", err)
	}
	if meta.Tag != "smoke-test" {
		t.Fatalf("expected tag smoke-test, got %q", meta.Tag)
	}
}

type mockEventSink struct{}

func (m *mockEventSink) Emit(ctx context.Context, event corerun.Event) error { return nil }
func (m *mockEventSink) Open(path string) error                              { return nil }
func (m *mockEventSink) Close(ctx context.Context) error                     { return nil }

func TestNewRunIDProducesShortIdentifier(t *testing.T) {
	now := time.Date(2026, time.May, 15, 12, 34, 56, 789000000, time.UTC)
	id := NewRunID("build workflow", now)
	if len(id) != 6 {
		t.Fatalf("expected 6-char run id, got %q", id)
	}
	if len(id) > 6 {
		t.Fatalf("expected short run id, got %q", id)
	}
	id2 := NewRunID("build workflow", now.Add(time.Second))
	if id == id2 {
		t.Fatalf("expected distinct ids for different timestamps, got %q", id)
	}
}
