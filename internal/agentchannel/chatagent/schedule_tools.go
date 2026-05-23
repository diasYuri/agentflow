package chatagent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/app"
)

type ScheduleRegistry interface {
	List() ([]app.Schedule, error)
	Add(app.Schedule) (app.Schedule, error)
	Remove(string) error
	Get(string) (app.Schedule, error)
	Update(app.Schedule) error
}

type scheduleDispatcher interface {
	Dispatch(context.Context, app.Schedule) error
}

var newScheduleDispatcher = func() scheduleDispatcher {
	return &execScheduleDispatcher{}
}

type execScheduleDispatcher struct{}

func (d *execScheduleDispatcher) Dispatch(ctx context.Context, schedule app.Schedule) error {
	agentflowPath, err := findAgentflowBinary()
	if err != nil {
		return err
	}
	args, err := scheduleRunArgs(schedule)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, agentflowPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

type scheduleListOutput struct {
	Schedules []app.Schedule `json:"schedules"`
}

type scheduleAddInput struct {
	WorkflowRef    string         `json:"workflow_ref"`
	Cron           string         `json:"cron,omitempty"`
	Every          string         `json:"every,omitempty"`
	Inputs         map[string]any `json:"inputs,omitempty"`
	Vars           map[string]any `json:"vars,omitempty"`
	MaxConcurrency int            `json:"max_concurrency,omitempty"`
	WorkingDir     string         `json:"working_dir,omitempty"`
	CodexPath      string         `json:"codex_path,omitempty"`
	ClaudePath     string         `json:"claude_path,omitempty"`
	PiPath         string         `json:"pi_path,omitempty"`
	LogFormat      string         `json:"log_format,omitempty"`
	EventsJSONL    string         `json:"events_jsonl,omitempty"`
	Tag            string         `json:"tag,omitempty"`
}

type scheduleAddOutput struct {
	Schedule app.Schedule `json:"schedule"`
}

type scheduleRemoveInput struct {
	ID string `json:"id"`
}

type scheduleRemoveOutput struct {
	Schedule app.Schedule `json:"schedule"`
}

type scheduleTickOutput struct {
	Dispatched []app.Schedule `json:"dispatched"`
}

func newScheduleListTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{}
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		if env.Schedules == nil {
			return nil, errors.New("schedule registry is not configured")
		}
		schedules, err := env.Schedules.List()
		if err != nil {
			return nil, fmt.Errorf("list schedules: %w", err)
		}
		return scheduleListOutput{Schedules: schedules}, nil
	}
	return Tool{
		Name:        "agentflow.schedule_list",
		Description: "List configured workflow schedules.",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

func newScheduleAddTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Schedules == nil {
			return nil, errors.New("schedule registry is not configured")
		}
		var in scheduleAddInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		workflowRef := strings.TrimSpace(in.WorkflowRef)
		if workflowRef == "" {
			return nil, errors.New("workflow_ref is required")
		}
		if strings.TrimSpace(in.Cron) == "" && strings.TrimSpace(in.Every) == "" {
			return nil, errors.New("either cron or every is required")
		}
		if strings.TrimSpace(in.Cron) != "" && strings.TrimSpace(in.Every) != "" {
			return nil, errors.New("cron and every are mutually exclusive")
		}

		resolvedRef, err := resolveScheduleWorkflowRef(ctx, env, workflowRef)
		if err != nil {
			return nil, err
		}
		workingDir := strings.TrimSpace(in.WorkingDir)
		if workingDir == "" {
			workingDir = strings.TrimSpace(env.ProjectPath)
		}
		workingDir, err = resolveScheduleWorkingDir(workingDir)
		if err != nil {
			return nil, err
		}
		schedule, err := buildScheduleSpec(resolvedRef, workingDir, in.Cron, in.Every, in.Inputs, in.Vars, in.MaxConcurrency, in.CodexPath, in.ClaudePath, in.PiPath, in.LogFormat, in.EventsJSONL, in.Tag)
		if err != nil {
			return nil, err
		}
		stored, err := env.Schedules.Add(schedule)
		if err != nil {
			return nil, fmt.Errorf("add schedule: %w", err)
		}
		return scheduleAddOutput{Schedule: stored}, nil
	}
	return Tool{
		Name:        "agentflow.schedule_add",
		Description: "Create a workflow schedule. Provide exactly one of cron or every.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workflow_ref":    map[string]any{"type": "string"},
				"cron":            map[string]any{"type": "string"},
				"every":           map[string]any{"type": "string"},
				"inputs":          map[string]any{"type": "object"},
				"vars":            map[string]any{"type": "object"},
				"max_concurrency": map[string]any{"type": "integer"},
				"working_dir":     map[string]any{"type": "string"},
				"codex_path":      map[string]any{"type": "string"},
				"claude_path":     map[string]any{"type": "string"},
				"pi_path":         map[string]any{"type": "string"},
				"log_format":      map[string]any{"type": "string"},
				"events_jsonl":    map[string]any{"type": "string"},
				"tag":             map[string]any{"type": "string"},
			},
			"required":             []string{"workflow_ref"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

func newScheduleRemoveTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in scheduleRemoveInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		if env.Schedules == nil {
			return nil, errors.New("schedule registry is not configured")
		}
		id := strings.TrimSpace(in.ID)
		if id == "" {
			return nil, errors.New("id is required")
		}
		schedule, err := env.Schedules.Get(id)
		if err != nil {
			return nil, fmt.Errorf("get schedule %s: %w", id, err)
		}
		if err := env.Schedules.Remove(id); err != nil {
			return nil, fmt.Errorf("remove schedule %s: %w", id, err)
		}
		return scheduleRemoveOutput{Schedule: schedule}, nil
	}
	return Tool{
		Name:        "agentflow.schedule_remove",
		Description: "Remove a workflow schedule by id.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

func newScheduleTickTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct{}
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		if env.Schedules == nil {
			return nil, errors.New("schedule registry is not configured")
		}
		dispatched, err := dispatchDueSchedules(ctx, env.Schedules, newScheduleDispatcher())
		if err != nil {
			return nil, err
		}
		return scheduleTickOutput{Dispatched: dispatched}, nil
	}
	return Tool{
		Name:        "agentflow.schedule_tick",
		Description: "Dispatch all due workflow schedules now.",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}

func resolveScheduleWorkflowRef(ctx context.Context, env *ToolEnvironment, workflowRef string) (string, error) {
	workflowRef = strings.TrimSpace(workflowRef)
	if workflowRef == "" {
		return "", errors.New("workflow_ref is required")
	}
	if isScheduleWorkflowPath(workflowRef) {
		abs, err := filepath.Abs(workflowRef)
		if err != nil {
			return "", fmt.Errorf("resolve workflow path %q: %w", workflowRef, err)
		}
		return filepath.Clean(abs), nil
	}
	projectPath := strings.TrimSpace(env.ProjectPath)
	roots := []string{}
	if projectPath != "" {
		roots = append(roots, filepath.Join(projectPath, ".agentflow", "workflows"))
	}
	repo := yamlrepo.NewWorkflowRepository(roots...)
	_, sourcePath, err := repo.Load(ctx, workflowRef)
	if err != nil {
		return "", fmt.Errorf("resolve workflow %q: %w", workflowRef, err)
	}
	return filepath.Clean(sourcePath), nil
}

func resolveScheduleWorkingDir(workingDir string) (string, error) {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return "", nil
	}
	if filepath.IsAbs(workingDir) {
		return filepath.Clean(workingDir), nil
	}
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve working dir %q: %w", workingDir, err)
	}
	return filepath.Clean(abs), nil
}

func buildScheduleSpec(workflowRef, workingDir, cronExpr, every string, inputs, vars map[string]any, maxConcurrency int, codexPath, claudePath, piPath, logFormat, eventsJSONL, tag string) (app.Schedule, error) {
	now := time.Now().UTC()
	schedule := app.Schedule{
		ID:             newScheduleID(),
		WorkflowRef:    workflowRef,
		Inputs:         inputs,
		Vars:           vars,
		MaxConcurrency: maxConcurrency,
		WorkingDir:     strings.TrimSpace(workingDir),
		CodexPath:      strings.TrimSpace(codexPath),
		ClaudePath:     strings.TrimSpace(claudePath),
		PiPath:         strings.TrimSpace(piPath),
		LogFormat:      strings.TrimSpace(logFormat),
		EventsJSONL:    strings.TrimSpace(eventsJSONL),
		Tag:            strings.TrimSpace(tag),
		CreatedAt:      now,
		UpdatedAt:      now,
		Enabled:        true,
	}
	switch {
	case strings.TrimSpace(cronExpr) != "":
		if err := validateCronExpression(cronExpr); err != nil {
			return app.Schedule{}, err
		}
		schedule.ScheduleType = "cron"
		schedule.Cron = normalizeCronExpression(cronExpr)
	case strings.TrimSpace(every) != "":
		duration, err := time.ParseDuration(strings.TrimSpace(every))
		if err != nil {
			return app.Schedule{}, fmt.Errorf("parse every duration %q: %w", every, err)
		}
		if duration < time.Minute {
			return app.Schedule{}, fmt.Errorf("every must be at least 1m")
		}
		schedule.ScheduleType = "every"
		schedule.Every = duration.String()
		schedule.NextRunAt = ceilMinute(now.Add(duration))
	default:
		return app.Schedule{}, errors.New("either cron or every is required")
	}
	return schedule, nil
}

func dispatchDueSchedules(ctx context.Context, registry ScheduleRegistry, dispatcher scheduleDispatcher) ([]app.Schedule, error) {
	lock, err := acquireScheduleLock()
	if err != nil {
		return nil, err
	}
	defer lock.Close()

	schedules, err := registry.List()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	currentMinute := now.Truncate(time.Minute)
	var dispatched []app.Schedule
	var dispatchErrs []error
	for _, schedule := range schedules {
		if !schedule.Enabled {
			continue
		}
		due, normalized, err := scheduleDue(schedule, now)
		if err != nil {
			dispatchErrs = append(dispatchErrs, fmt.Errorf("%s: %w", schedule.ID, err))
			continue
		}
		if !due {
			continue
		}
		if err := dispatcher.Dispatch(ctx, schedule); err != nil {
			dispatchErrs = append(dispatchErrs, fmt.Errorf("%s: dispatch: %w", schedule.ID, err))
			continue
		}
		schedule.LastTriggeredAt = currentMinute
		schedule.NextRunAt = normalized.NextRunAt
		schedule.UpdatedAt = now
		if err := registry.Update(schedule); err != nil {
			dispatchErrs = append(dispatchErrs, fmt.Errorf("%s: persist dispatch state: %w", schedule.ID, err))
			continue
		}
		dispatched = append(dispatched, schedule)
	}
	if len(dispatchErrs) > 0 {
		return dispatched, fmt.Errorf("dispatch schedules: %w", joinErrors(dispatchErrs))
	}
	return dispatched, nil
}

func scheduleDue(schedule app.Schedule, now time.Time) (bool, app.Schedule, error) {
	normalized := schedule
	switch schedule.ScheduleType {
	case "cron":
		matched, err := cronMatches(now, schedule.Cron)
		if err != nil {
			return false, normalized, err
		}
		if !matched {
			return false, normalized, nil
		}
		lastMinute := schedule.LastTriggeredAt.UTC().Truncate(time.Minute)
		currentMinute := now.UTC().Truncate(time.Minute)
		if !lastMinute.IsZero() && !lastMinute.Before(currentMinute) {
			return false, normalized, nil
		}
		return true, normalized, nil
	case "every":
		duration, err := time.ParseDuration(schedule.Every)
		if err != nil {
			return false, normalized, fmt.Errorf("parse every duration: %w", err)
		}
		if duration < time.Minute {
			return false, normalized, fmt.Errorf("every duration must be at least 1m")
		}
		next := schedule.NextRunAt
		if next.IsZero() {
			next = ceilMinute(schedule.CreatedAt.Add(duration))
		}
		if now.Before(next) {
			normalized.NextRunAt = next
			return false, normalized, nil
		}
		for !next.After(now) {
			next = ceilMinute(next.Add(duration))
		}
		normalized.NextRunAt = next
		return true, normalized, nil
	default:
		return false, normalized, fmt.Errorf("unsupported schedule type %q", schedule.ScheduleType)
	}
}

func scheduleRunArgs(schedule app.Schedule) ([]string, error) {
	args := []string{"workflow", "run", "-it", schedule.WorkflowRef}
	switch schedule.ScheduleType {
	case "cron":
	case "every":
	default:
		return nil, fmt.Errorf("unsupported schedule type %q", schedule.ScheduleType)
	}
	if schedule.Tag != "" {
		args = append(args, "--tag", schedule.Tag)
	}
	if schedule.WorkingDir != "" {
		args = append(args, "--working-dir", schedule.WorkingDir)
	}
	if schedule.MaxConcurrency > 0 {
		args = append(args, "--max-concurrency", strconv.Itoa(schedule.MaxConcurrency))
	}
	if schedule.CodexPath != "" {
		args = append(args, "--codex-path", schedule.CodexPath)
	}
	if schedule.ClaudePath != "" {
		args = append(args, "--claude-path", schedule.ClaudePath)
	}
	if schedule.PiPath != "" {
		args = append(args, "--pi-path", schedule.PiPath)
	}
	if schedule.LogFormat != "" {
		args = append(args, "--log-format", schedule.LogFormat)
	}
	if schedule.EventsJSONL != "" {
		args = append(args, "--events-jsonl", schedule.EventsJSONL)
	}
	for _, kv := range flattenScheduleMap("--input", schedule.Inputs) {
		args = append(args, kv...)
	}
	for _, kv := range flattenScheduleMap("--var", schedule.Vars) {
		args = append(args, kv...)
	}
	return args, nil
}

func flattenScheduleMap(flag string, values map[string]any) [][]string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([][]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, []string{flag, fmt.Sprintf("%s=%s", key, formatScheduleValue(values[key]))})
	}
	return out
}

func formatScheduleValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprint(v)
	default:
		return mustJSON(value)
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return fmt.Errorf("%s", strings.Join(parts, "; "))
}

func acquireScheduleLock() (*os.File, error) {
	lockPath, err := scheduleLockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("another schedule tick is already running")
	}
	return file, nil
}

func scheduleLockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(home) == "" {
		home = "."
	}
	return filepath.Join(home, ".agentflow", "schedule.lock"), nil
}

func normalizeCronExpression(expr string) string {
	fields := strings.Fields(strings.TrimSpace(expr))
	return strings.Join(fields, " ")
}

func validateCronExpression(expr string) error {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return fmt.Errorf("cron expression must have 5 fields: minute hour day month weekday")
	}
	for idx, field := range fields {
		if _, err := parseCronField(field, cronFieldSpecByIndex(idx)); err != nil {
			return fmt.Errorf("invalid cron field %d (%q): %w", idx+1, field, err)
		}
	}
	return nil
}

type cronFieldSpec struct {
	min int
	max int
}

func cronFieldSpecByIndex(idx int) cronFieldSpec {
	switch idx {
	case 0:
		return cronFieldSpec{min: 0, max: 59}
	case 1:
		return cronFieldSpec{min: 0, max: 23}
	case 2:
		return cronFieldSpec{min: 1, max: 31}
	case 3:
		return cronFieldSpec{min: 1, max: 12}
	case 4:
		return cronFieldSpec{min: 0, max: 7}
	default:
		return cronFieldSpec{min: 0, max: 0}
	}
}

func cronMatches(now time.Time, expr string) (bool, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return false, fmt.Errorf("cron expression must have 5 fields")
	}
	allowed := []int{
		now.Minute(),
		now.Hour(),
		now.Day(),
		int(now.Month()),
		int(now.Weekday()),
	}
	if allowed[4] == 0 {
		allowed[4] = 7
	}
	for idx, field := range fields {
		set, err := parseCronField(field, cronFieldSpecByIndex(idx))
		if err != nil {
			return false, err
		}
		if !set.matches(allowed[idx]) {
			return false, nil
		}
	}
	return true, nil
}

type cronValueSet struct {
	any    bool
	values map[int]struct{}
}

func (s cronValueSet) matches(value int) bool {
	if s.any {
		return true
	}
	_, ok := s.values[value]
	return ok
}

func parseCronField(field string, spec cronFieldSpec) (cronValueSet, error) {
	field = strings.TrimSpace(field)
	if field == "" {
		return cronValueSet{}, fmt.Errorf("empty field")
	}
	if field == "*" {
		return cronValueSet{any: true}, nil
	}
	set := cronValueSet{values: map[int]struct{}{}}
	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return cronValueSet{}, fmt.Errorf("empty list item")
		}
		step := 1
		if head, tail, ok := strings.Cut(part, "/"); ok {
			part = head
			parsedStep, err := strconv.Atoi(tail)
			if err != nil || parsedStep <= 0 {
				return cronValueSet{}, fmt.Errorf("invalid step %q", tail)
			}
			step = parsedStep
		}
		start, end := spec.min, spec.max
		if part != "*" {
			if lo, hi, ok := strings.Cut(part, "-"); ok {
				var err error
				start, err = strconv.Atoi(lo)
				if err != nil {
					return cronValueSet{}, fmt.Errorf("invalid range start %q", lo)
				}
				end, err = strconv.Atoi(hi)
				if err != nil {
					return cronValueSet{}, fmt.Errorf("invalid range end %q", hi)
				}
			} else {
				parsed, err := strconv.Atoi(part)
				if err != nil {
					return cronValueSet{}, fmt.Errorf("invalid value %q", part)
				}
				start, end = parsed, parsed
			}
		}
		if start < spec.min || end > spec.max || start > end {
			return cronValueSet{}, fmt.Errorf("value out of range %d-%d", spec.min, spec.max)
		}
		for value := start; value <= end; value += step {
			set.values[value] = struct{}{}
		}
	}
	return set, nil
}

func ceilMinute(t time.Time) time.Time {
	t = t.UTC()
	ceil := t.Truncate(time.Minute)
	if !ceil.Equal(t) {
		ceil = ceil.Add(time.Minute)
	}
	return ceil
}

func scheduleIdPrefix() string {
	return "schedule-"
}

func newScheduleID() string {
	return scheduleIdPrefix() + time.Now().UTC().Format("20060102T150405") + "-" + randomSuffix()
}

func randomSuffix() string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "000000"
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b[:])
}

func findAgentflowBinary() (string, error) {
	if path := os.Getenv("AGENTFLOW_PATH"); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("agentflow"); err == nil {
		return path, nil
	}
	self, err := os.Executable()
	if err == nil {
		base := filepath.Base(self)
		if base == "agentflow" {
			return self, nil
		}
	}
	return "", fmt.Errorf("agentflow binary not found; build and install it or set AGENTFLOW_PATH")
}

func isScheduleWorkflowPath(ref string) bool {
	ext := strings.ToLower(filepath.Ext(ref))
	if ext != ".yaml" && ext != ".yml" && ext != ".json" {
		return false
	}
	if strings.Contains(ref, string(filepath.Separator)) || filepath.IsAbs(ref) {
		return true
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	return false
}
