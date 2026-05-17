package client

import (
	"errors"
	"testing"

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
