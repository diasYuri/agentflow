package handlers

import (
	"testing"
	"time"
)

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
