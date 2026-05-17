package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Schedule describes a workflow execution schedule persisted locally.
type Schedule struct {
	ID              string         `json:"id"`
	WorkflowRef     string         `json:"workflow_ref"`
	ScheduleType    string         `json:"schedule_type"`
	Cron            string         `json:"cron,omitempty"`
	Every           string         `json:"every,omitempty"`
	Inputs          map[string]any `json:"inputs,omitempty"`
	Vars            map[string]any `json:"vars,omitempty"`
	MaxConcurrency  int            `json:"max_concurrency,omitempty"`
	WorkingDir      string         `json:"working_dir,omitempty"`
	CodexPath       string         `json:"codex_path,omitempty"`
	ClaudePath      string         `json:"claude_path,omitempty"`
	PiPath          string         `json:"pi_path,omitempty"`
	LogFormat       string         `json:"log_format,omitempty"`
	EventsJSONL     string         `json:"events_jsonl,omitempty"`
	Tag             string         `json:"tag,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LastTriggeredAt time.Time      `json:"last_triggered_at,omitempty"`
	NextRunAt       time.Time      `json:"next_run_at,omitempty"`
	Enabled         bool           `json:"enabled"`
}

// ScheduleStore persists schedule entries locally.
type ScheduleStore interface {
	Load() ([]Schedule, error)
	Save([]Schedule) error
}

// JSONScheduleStore persists schedules in a JSON file.
type JSONScheduleStore struct {
	path string
	mu   sync.RWMutex
}

// NewJSONScheduleStore creates a JSON-backed schedule store.
func NewJSONScheduleStore(path string) *JSONScheduleStore {
	return &JSONScheduleStore{path: path}
}

// Load reads schedules from disk or returns an empty list.
func (s *JSONScheduleStore) Load() ([]Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Schedule{}, nil
		}
		return nil, err
	}

	var schedules []Schedule
	if err := json.Unmarshal(data, &schedules); err != nil {
		return nil, err
	}
	return normalizeSchedules(schedules), nil
}

// Save writes schedules to disk.
func (s *JSONScheduleStore) Save(schedules []Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalizeSchedules(schedules), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o644)
}

// DefaultSchedulesPath returns the default local schedule registry path.
func DefaultSchedulesPath() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".agentflow", "schedules.json")
	}
	return filepath.Join(".agentflow", "schedules.json")
}

// ScheduleRegistry provides CRUD and lookup operations for schedules.
type ScheduleRegistry struct {
	store ScheduleStore
	mu    sync.RWMutex
}

// NewScheduleRegistry creates a registry backed by the provided store.
func NewScheduleRegistry(store ScheduleStore) *ScheduleRegistry {
	if store == nil {
		store = NewJSONScheduleStore(DefaultSchedulesPath())
	}
	return &ScheduleRegistry{store: store}
}

// List returns all configured schedules.
func (r *ScheduleRegistry) List() ([]Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schedules, err := r.store.Load()
	if err != nil {
		return nil, err
	}
	return normalizeSchedules(schedules), nil
}

// Add inserts a new schedule after normalizing it.
func (r *ScheduleRegistry) Add(schedule Schedule) (Schedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	schedules, err := r.store.Load()
	if err != nil {
		return Schedule{}, err
	}
	schedule = normalizeSchedule(schedule)
	if schedule.ID == "" {
		return Schedule{}, fmt.Errorf("schedule id is required")
	}
	if err := validateSchedule(schedule); err != nil {
		return Schedule{}, err
	}
	for _, existing := range schedules {
		if existing.ID == schedule.ID {
			return Schedule{}, fmt.Errorf("schedule %q already exists", schedule.ID)
		}
	}
	schedules = append(schedules, schedule)
	if err := r.store.Save(schedules); err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

// Remove deletes a schedule by ID.
func (r *ScheduleRegistry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("schedule id is required")
	}
	schedules, err := r.store.Load()
	if err != nil {
		return err
	}
	filtered := schedules[:0]
	removed := false
	for _, schedule := range schedules {
		if schedule.ID == id {
			removed = true
			continue
		}
		filtered = append(filtered, schedule)
	}
	if !removed {
		return fmt.Errorf("schedule %q not found", id)
	}
	return r.store.Save(filtered)
}

// Update replaces an existing schedule entry.
func (r *ScheduleRegistry) Update(schedule Schedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	schedule = normalizeSchedule(schedule)
	if err := validateSchedule(schedule); err != nil {
		return err
	}
	schedules, err := r.store.Load()
	if err != nil {
		return err
	}
	replaced := false
	for i := range schedules {
		if schedules[i].ID == schedule.ID {
			schedules[i] = schedule
			replaced = true
			break
		}
	}
	if !replaced {
		return fmt.Errorf("schedule %q not found", schedule.ID)
	}
	return r.store.Save(schedules)
}

// Get returns a schedule by ID.
func (r *ScheduleRegistry) Get(id string) (Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id = strings.TrimSpace(id)
	if id == "" {
		return Schedule{}, fmt.Errorf("schedule id is required")
	}
	schedules, err := r.store.Load()
	if err != nil {
		return Schedule{}, err
	}
	for _, schedule := range schedules {
		if schedule.ID == id {
			return normalizeSchedule(schedule), nil
		}
	}
	return Schedule{}, fmt.Errorf("schedule %q not found", id)
}

func normalizeSchedules(schedules []Schedule) []Schedule {
	result := make([]Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		result = append(result, normalizeSchedule(schedule))
	}
	return result
}

func normalizeSchedule(schedule Schedule) Schedule {
	schedule.ID = strings.TrimSpace(schedule.ID)
	schedule.WorkflowRef = strings.TrimSpace(schedule.WorkflowRef)
	schedule.ScheduleType = strings.TrimSpace(schedule.ScheduleType)
	schedule.Cron = strings.TrimSpace(schedule.Cron)
	schedule.Every = strings.TrimSpace(schedule.Every)
	schedule.WorkingDir = strings.TrimSpace(schedule.WorkingDir)
	schedule.CodexPath = strings.TrimSpace(schedule.CodexPath)
	schedule.ClaudePath = strings.TrimSpace(schedule.ClaudePath)
	schedule.PiPath = strings.TrimSpace(schedule.PiPath)
	schedule.LogFormat = strings.TrimSpace(schedule.LogFormat)
	schedule.EventsJSONL = strings.TrimSpace(schedule.EventsJSONL)
	schedule.Tag = strings.TrimSpace(schedule.Tag)
	if schedule.Inputs == nil {
		schedule.Inputs = map[string]any{}
	}
	if schedule.Vars == nil {
		schedule.Vars = map[string]any{}
	}
	return schedule
}

func validateSchedule(schedule Schedule) error {
	if schedule.ID == "" {
		return fmt.Errorf("schedule id is required")
	}
	if schedule.WorkflowRef == "" {
		return fmt.Errorf("workflow ref is required")
	}
	switch schedule.ScheduleType {
	case "cron":
		if schedule.Cron == "" {
			return fmt.Errorf("cron expression is required")
		}
	case "every":
		if schedule.Every == "" {
			return fmt.Errorf("every duration is required")
		}
	default:
		return fmt.Errorf("unsupported schedule type %q", schedule.ScheduleType)
	}
	return nil
}
