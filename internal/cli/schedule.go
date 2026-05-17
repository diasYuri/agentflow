package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/app"
)

type scheduleOutputFormat string

const (
	scheduleOutputText scheduleOutputFormat = "text"
	scheduleOutputJSON scheduleOutputFormat = "json"
)

var newScheduleRegistry = func() *app.ScheduleRegistry {
	return app.NewScheduleRegistry(app.NewJSONScheduleStore(app.DefaultSchedulesPath()))
}

func newWorkflowScheduleCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage workflow schedules",
	}
	cmd.AddCommand(
		newWorkflowScheduleAddCommand(opts),
		newWorkflowScheduleListCommand(),
		newWorkflowScheduleRemoveCommand(),
		newWorkflowScheduleTickCommand(),
	)
	return cmd
}

func newWorkflowScheduleAddCommand(opts *options) *cobra.Command {
	local := *opts
	var cronExpr string
	var every string
	cmd := &cobra.Command{
		Use:   "add <workflow>",
		Short: "Create a schedule for a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(cronExpr) == "" && strings.TrimSpace(every) == "" {
				return fmt.Errorf("either --cron or --every is required")
			}
			if strings.TrimSpace(cronExpr) != "" && strings.TrimSpace(every) != "" {
				return fmt.Errorf("--cron and --every are mutually exclusive")
			}

			workflowRef, workingDir, err := resolveScheduledWorkflowRef(cmd, args[0], &local)
			if err != nil {
				return err
			}
			inputs, vars, err := parseInputsAndVars(&local)
			if err != nil {
				return err
			}
			schedule, err := buildScheduleSpec(workflowRef, workingDir, cronExpr, every, inputs, vars, &local)
			if err != nil {
				return err
			}

			registry := newScheduleRegistry()
			stored, err := registry.Add(schedule)
			if err != nil {
				return err
			}
			if err := ensureScheduleDispatcherInstalled(cmd.Context()); err != nil {
				_ = registry.Remove(stored.ID)
				return err
			}

			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("Schedule added"),
				f.keyValueLines(scheduleLines(stored)),
			))
			return nil
		},
	}
	addCommonFlags(cmd, &local)
	cmd.Flags().StringVar(&local.tag, "tag", "", "friendly name for this workflow schedule")
	cmd.Flags().StringVar(&cronExpr, "cron", "", "cron expression (minute hour day month weekday)")
	cmd.Flags().StringVar(&every, "every", "", "fixed interval such as 15m or 1h")
	return cmd
}

func newWorkflowScheduleListCommand() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured workflow schedules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			schedules, err := newScheduleRegistry().List()
			if err != nil {
				return err
			}
			return renderWorkflowSchedules(cmd.OutOrStdout(), schedules, outputFormat)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	return cmd
}

func newWorkflowScheduleRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a workflow schedule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := newScheduleRegistry()
			schedule, err := registry.Get(args[0])
			if err != nil {
				return err
			}
			if err := registry.Remove(args[0]); err != nil {
				return err
			}
			if schedules, err := registry.List(); err == nil && len(schedules) == 0 {
				_ = removeScheduleDispatcher(cmd.Context())
			}
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("Schedule removed"),
				f.keyValueLines(scheduleLines(schedule)),
			))
			return nil
		},
	}
	return cmd
}

func newWorkflowScheduleTickCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "tick",
		Short:  "Dispatch due workflow schedules",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return dispatchDueSchedules(cmd.Context(), newScheduleRegistry(), newScheduleDispatcher())
		},
	}
	return cmd
}

func renderWorkflowSchedules(w io.Writer, schedules []app.Schedule, format string) error {
	if scheduleOutputFormat(format) == scheduleOutputJSON {
		data, err := json.Marshal(schedules)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(schedules) == 0 {
		_, err := fmt.Fprintln(w, newCLIFormat(isInteractiveWriter(w)).note("No workflow schedules"))
		return err
	}
	f := newCLIFormat(isInteractiveWriter(w))
	headers := []string{"ID", "TYPE", "SPEC", "WORKFLOW", "NEXT RUN", "LAST RUN", "TAG"}
	widths := []int{18, 8, 20, 24, 20, 20, 16}
	rows := make([][]string, 0, len(schedules))
	for _, schedule := range schedules {
		rows = append(rows, []string{
			schedule.ID,
			schedule.ScheduleType,
			scheduleSpecLabel(schedule),
			baseNameOrPath(schedule.WorkflowRef),
			formatScheduleTime(schedule.NextRunAt),
			formatScheduleTime(schedule.LastTriggeredAt),
			firstNonEmpty(schedule.Tag, "-"),
		})
	}
	_, err := fmt.Fprint(w, f.table(headers, rows, widths))
	return err
}

func buildScheduleSpec(workflowRef, workingDir, cronExpr, every string, inputs, vars map[string]any, opts *options) (app.Schedule, error) {
	now := time.Now().UTC()
	schedule := app.Schedule{
		ID:             newScheduleID(),
		WorkflowRef:    workflowRef,
		Inputs:         inputs,
		Vars:           vars,
		MaxConcurrency: opts.maxConcurrency,
		WorkingDir:     workingDir,
		CodexPath:      opts.codexPath,
		ClaudePath:     opts.claudePath,
		PiPath:         opts.piPath,
		LogFormat:      opts.logFormat,
		EventsJSONL:    opts.eventsJSONL,
		Tag:            opts.tag,
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
			return app.Schedule{}, fmt.Errorf("parse --every duration %q: %w", every, err)
		}
		if duration < time.Minute {
			return app.Schedule{}, fmt.Errorf("--every must be at least 1m")
		}
		schedule.ScheduleType = "every"
		schedule.Every = duration.String()
		schedule.NextRunAt = ceilMinute(now.Add(duration))
	default:
		return app.Schedule{}, fmt.Errorf("either --cron or --every is required")
	}
	return schedule, nil
}

func resolveScheduledWorkflowRef(cmd *cobra.Command, workflowRef string, opts *options) (string, string, error) {
	ref, workingDir, err := resolveWorkflowRunContext(cmd, workflowRef, opts)
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(ref) {
		return filepath.Clean(ref), workingDir, nil
	}
	if isWorkflowPath(ref) {
		abs, err := filepath.Abs(ref)
		if err != nil {
			return "", "", fmt.Errorf("resolve workflow path %q: %w", ref, err)
		}
		return filepath.Clean(abs), workingDir, nil
	}
	repo := yamlrepo.NewWorkflowRepository()
	_, sourcePath, err := repo.Load(cmd.Context(), ref)
	if err != nil {
		return "", "", err
	}
	return sourcePath, workingDir, nil
}

func scheduleLines(schedule app.Schedule) [][2]string {
	lines := [][2]string{
		{"id", schedule.ID},
		{"type", schedule.ScheduleType},
		{"workflow", baseNameOrPath(schedule.WorkflowRef)},
		{"spec", scheduleSpecLabel(schedule)},
		{"working_dir", firstNonEmpty(schedule.WorkingDir, "-")},
		{"tag", firstNonEmpty(schedule.Tag, "-")},
	}
	if !schedule.NextRunAt.IsZero() {
		lines = append(lines, [2]string{"next_run", schedule.NextRunAt.Format(time.RFC3339)})
	}
	if !schedule.LastTriggeredAt.IsZero() {
		lines = append(lines, [2]string{"last_run", schedule.LastTriggeredAt.Format(time.RFC3339)})
	}
	return lines
}

func scheduleSpecLabel(schedule app.Schedule) string {
	switch schedule.ScheduleType {
	case "cron":
		return schedule.Cron
	case "every":
		return schedule.Every
	default:
		return "-"
	}
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func baseNameOrPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if filepath.IsAbs(value) {
		return filepath.Base(value)
	}
	return value
}
