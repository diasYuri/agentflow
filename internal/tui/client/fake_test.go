package client

import (
	"context"
	"errors"
	"testing"
)

func TestFakeClientDefaults(t *testing.T) {
	f := &FakeClient{}
	ctx := context.Background()

	state, err := f.DaemonStatus(ctx)
	if err == nil || state.Status != DaemonUnavailable {
		t.Fatal("expected unavailable")
	}

	runs, err := f.ListRuns(ctx)
	if err != nil || runs != nil {
		t.Fatal("expected nil runs")
	}

	if err := f.CancelRun(ctx, "r1"); err != nil {
		t.Fatal("expected nil")
	}
}

func TestFakeClientOverrides(t *testing.T) {
	f := &FakeClient{
		DaemonStateFunc: func(ctx context.Context) (DaemonState, error) {
			return DaemonState{Status: DaemonAvailable, Running: true}, nil
		},
		ListRunsFunc: func(ctx context.Context) ([]RunSummary, error) {
			return []RunSummary{{ID: "run-1"}}, nil
		},
	}
	ctx := context.Background()

	state, err := f.DaemonStatus(ctx)
	if err != nil || !state.Running {
		t.Fatal("expected available")
	}

	runs, err := f.ListRuns(ctx)
	if err != nil || len(runs) != 1 {
		t.Fatal("expected 1 run")
	}
}

func TestIsDaemonUnavailable(t *testing.T) {
	if IsDaemonUnavailable(nil) {
		t.Fatal("expected false for nil")
	}
	if !IsDaemonUnavailable(ErrDaemonUnavailable) {
		t.Fatal("expected true for sentinel")
	}
	if !IsDaemonUnavailable(&DaemonError{Status: DaemonUnavailable, Err: errors.New("x")}) {
		t.Fatal("expected true for DaemonError")
	}
	if !IsDaemonUnavailable(&DaemonError{Status: DaemonRequiredMissing, Err: errors.New("x")}) {
		t.Fatal("expected true for required missing")
	}
	if IsDaemonUnavailable(errors.New("random")) {
		t.Fatal("expected false for random error")
	}
}
