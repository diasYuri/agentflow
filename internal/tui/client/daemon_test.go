package client

import (
	"errors"
	"testing"

	"github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/daemon"
)

func TestMapDaemonErrorNil(t *testing.T) {
	if mapDaemonError(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestMapDaemonErrorUnavailable(t *testing.T) {
	err := mapDaemonError(daemonUnavailableError{socketPath: "/tmp/test.sock", err: errors.New("connection refused")})
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("expected ErrDaemonUnavailable, got %v", err)
	}
}

type daemonUnavailableError struct {
	socketPath string
	err        error
}

func (e daemonUnavailableError) Error() string {
	return "agentflowd is not running; start it with agentflow daemon start (socket: " + e.socketPath + ")"
}

func (e daemonUnavailableError) Unwrap() error {
	return e.err
}

func TestRunSummaryFromDaemon(t *testing.T) {
	r := daemon.WorkflowRun{
		ID:       "run-1",
		Workflow: "test",
		Status:   "running",
		Tag:      "tag1",
	}
	s := runSummaryFromDaemon(r)
	if s.ID != "run-1" {
		t.Fatalf("expected ID run-1, got %s", s.ID)
	}
	if s.Workflow != "test" {
		t.Fatalf("expected workflow test, got %s", s.Workflow)
	}
	if s.Status != "running" {
		t.Fatalf("expected status running, got %s", s.Status)
	}
	if s.Tag != "tag1" {
		t.Fatalf("expected tag tag1, got %s", s.Tag)
	}
}

func TestPlanOrder(t *testing.T) {
	plan := map[string]any{
		"order": []string{"a", "b", "c"},
	}
	order := planOrder(plan)
	if len(order) != 3 || order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestPlanOrderAnySlice(t *testing.T) {
	plan := map[string]any{
		"order": []any{"x", "y"},
	}
	order := planOrder(plan)
	if len(order) != 2 || order[0] != "x" || order[1] != "y" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestPlanOrderNil(t *testing.T) {
	if planOrder(nil) != nil {
		t.Fatal("expected nil")
	}
	if planOrder(map[string]any{}) != nil {
		t.Fatal("expected nil for missing order")
	}
}

func TestSlowestNodesFromDaemon(t *testing.T) {
	in := []run.SlowestNode{{NodeID: "a", DurationMS: 100}, {NodeID: "b", DurationMS: 200}}
	out := slowestNodesFromDaemon(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(out))
	}
	if out[0].NodeID != "a" || out[0].DurationMS != 100 {
		t.Fatalf("unexpected first node: %+v", out[0])
	}
}

func TestSlowestNodesFromDaemonEmpty(t *testing.T) {
	out := slowestNodesFromDaemon(nil)
	if out != nil {
		t.Fatal("expected nil")
	}
}

func TestAgentUsageFromDaemon(t *testing.T) {
	in := []run.AgentUsage{{Provider: "openai", Model: "gpt-4", TotalTokens: 42}}
	out := agentUsageFromDaemon(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 usage, got %d", len(out))
	}
	if out[0].Provider != "openai" || out[0].TotalTokens != 42 {
		t.Fatalf("unexpected usage: %+v", out[0])
	}
}

func TestAgentUsageFromDaemonEmpty(t *testing.T) {
	out := agentUsageFromDaemon(nil)
	if out != nil {
		t.Fatal("expected nil")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Fatalf("expected b, got %s", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Fatalf("expected a, got %s", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}
