package app

import (
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleRegistryPersistsEntries(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONScheduleStore(filepath.Join(dir, "schedules.json"))
	registry := NewScheduleRegistry(store)

	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	schedule := Schedule{
		ID:           "schedule-1",
		WorkflowRef:  "/tmp/workflow.yaml",
		ScheduleType: "every",
		Every:        "15m0s",
		WorkingDir:   "/tmp",
		Tag:          "nightly",
		CreatedAt:    now,
		UpdatedAt:    now,
		Enabled:      true,
		NextRunAt:    now.Add(15 * time.Minute),
		Inputs:       map[string]any{"flag": true},
		Vars:         map[string]any{"env": "prod"},
	}

	stored, err := registry.Add(schedule)
	if err != nil {
		t.Fatalf("add schedule: %v", err)
	}
	if stored.ID != schedule.ID {
		t.Fatalf("expected stored id %q, got %q", schedule.ID, stored.ID)
	}

	list, err := registry.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(list) != 1 || list[0].ID != schedule.ID {
		t.Fatalf("unexpected schedules: %#v", list)
	}
	if list[0].Inputs["flag"] != true {
		t.Fatalf("expected stored input, got %#v", list[0].Inputs)
	}

	got, err := registry.Get(schedule.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if got.Tag != "nightly" || got.Every != "15m0s" {
		t.Fatalf("unexpected schedule data: %#v", got)
	}

	if err := registry.Remove(schedule.ID); err != nil {
		t.Fatalf("remove schedule: %v", err)
	}
	list, err = registry.List()
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty registry, got %#v", list)
	}
}
