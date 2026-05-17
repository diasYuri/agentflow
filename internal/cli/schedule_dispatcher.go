package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/diasYuri/agentflow/internal/app"
)

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

func dispatchDueSchedules(ctx context.Context, registry *app.ScheduleRegistry, dispatcher scheduleDispatcher) error {
	lock, err := acquireScheduleLock()
	if err != nil {
		return err
	}
	defer lock.Close()

	schedules, err := registry.List()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	currentMinute := now.Truncate(time.Minute)
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
		}
	}
	if len(dispatchErrs) > 0 {
		return fmt.Errorf("dispatch schedules: %w", joinErrors(dispatchErrs))
	}
	return nil
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
		// no extra scheduling args
	case "every":
		// no extra scheduling args
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
	sortStrings(keys)
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

// sortStrings keeps the implementation local without importing sort in the CLI package multiple times.
func sortStrings(values []string) {
	for i := 0; i < len(values)-1; i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
