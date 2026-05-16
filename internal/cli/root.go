package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/diasYuri/agentflow/internal/app"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
)

type options struct {
	inputs         []string
	inputJSON      string
	vars           []string
	maxConcurrency int
	workingDir     string
	codexPath      string
	claudePath     string
	logFormat      string
	eventsJSONL    string
	graphFormat    string
	dryRun         bool
	interactive    bool
	noColor        bool
}

type workflowRunClient interface {
	RunWorkflow(context.Context, daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error)
}

type workflowDaemonClient interface {
	ListWorkflows(context.Context) (daemon.ListWorkflowsResponse, error)
	WorkflowStatus(context.Context, string) (daemon.RunWorkflowResponse, error)
	WorkflowLogs(context.Context, string) (daemon.LogsResponse, error)
	CancelWorkflow(context.Context, string) (daemon.CancelWorkflowResponse, error)
	PauseWorkflow(context.Context, string) (daemon.PauseWorkflowResponse, error)
	ResumeWorkflow(context.Context, string) (daemon.ResumeWorkflowResponse, error)
	Status(context.Context) (daemon.DaemonStatus, error)
	Stop(context.Context) (daemon.StopResponse, error)
}

var newWorkflowRunClient = func(socketPath string) workflowRunClient {
	return daemon.NewClient(socketPath)
}

var newDaemonClient = func(socketPath string) workflowDaemonClient {
	return daemon.NewClient(socketPath)
}

var workflowWatchInterval = 5 * time.Second

func NewRootCommand() *cobra.Command {
	opts := &options{logFormat: "text"}
	cmd := &cobra.Command{
		Use:   "agentflow",
		Short: "Run local YAML workflows for agent coding",
	}
	cmd.AddCommand(
		newValidateCommand(opts),
		newDryRunCommand(opts),
		newRunCommand(opts),
		newGraphCommand(opts),
		newDaemonCommand(opts),
		newWorkflowCommand(opts),
	)
	return cmd
}

func newValidateCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <workflow>",
		Short: "Validate a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uc, err := buildUseCase(opts)
			if err != nil {
				return err
			}
			plan, err := uc.Validate(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "valid: %s (%d nodes)\n", plan.Workflow.Name, len(plan.Order))
			return nil
		},
	}
	return cmd
}

func newGraphCommand(opts *options) *cobra.Command {
	local := *opts
	local.graphFormat = "mermaid"
	cmd := &cobra.Command{
		Use:   "graph <workflow>",
		Short: "Validate and print the workflow graph",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if local.graphFormat != "mermaid" {
				return fmt.Errorf("unsupported graph format %q", local.graphFormat)
			}
			uc, err := buildUseCase(&local)
			if err != nil {
				return err
			}
			plan, err := uc.Validate(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return workflow.WriteMermaidGraph(cmd.OutOrStdout(), plan)
		},
	}
	cmd.Flags().StringVar(&local.graphFormat, "format", "mermaid", "graph output format (mermaid)")
	return cmd
}

func newDryRunCommand(opts *options) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "dry-run <workflow>",
		Short: "Validate and print the execution plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			uc, err := buildUseCase(&local)
			if err != nil {
				return err
			}
			inputs, vars, err := parseInputsAndVars(&local)
			if err != nil {
				return err
			}
			plan, resolved, err := uc.DryRun(cmd.Context(), runworkflow.RunOptions{
				WorkflowRef: args[0], Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
				WorkingDir: local.workingDir,
			})
			if err != nil {
				return err
			}
			out := map[string]any{"workflow": plan.Workflow.Name, "inputs": resolved, "order": plan.Order, "nodes": plan.Nodes}
			data, _ := json.MarshalIndent(out, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	addCommonFlags(cmd, &local)
	return cmd
}

func newRunCommand(opts *options) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "run <workflow>",
		Short: "Run a workflow through agentflowd unless -it is set",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !local.interactive {
				return runWorkflowViaDaemon(cmd, args[0], &local)
			}
			uc, err := buildUseCase(&local)
			if err != nil {
				return err
			}
			inputs, vars, err := parseInputsAndVars(&local)
			if err != nil {
				return err
			}
			result, err := uc.Run(cmd.Context(), runworkflow.RunOptions{
				WorkflowRef: args[0], Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
				WorkingDir: local.workingDir, DryRun: local.dryRun,
			})
			if result.RunID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\nrun_dir: %s\nstatus: %s\n", result.RunID, result.RunDir, result.Status)
			}
			return err
		},
	}
	addCommonFlags(cmd, &local)
	cmd.Flags().BoolVar(&local.dryRun, "dry-run", false, "validate and plan without executing")
	addInteractiveFlags(cmd, &local)
	return cmd
}

func newWorkflowCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Control workflows through agentflowd",
	}
	cmd.AddCommand(
		newWorkflowRunCommand(opts),
		newWorkflowListCommand(),
		newWorkflowStatusCommand(),
		newWorkflowWatchCommand(),
		newWorkflowLogsCommand(),
		newWorkflowCancelCommand(),
		newWorkflowPauseCommand(),
		newWorkflowResumeCommand(),
	)
	return cmd
}

func newWorkflowRunCommand(opts *options) *cobra.Command {
	local := *opts
	cmd := &cobra.Command{
		Use:   "run <workflow>",
		Short: "Start a workflow in agentflowd",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if local.interactive {
				uc, err := buildUseCase(&local)
				if err != nil {
					return err
				}
				inputs, vars, err := parseInputsAndVars(&local)
				if err != nil {
					return err
				}
				result, err := uc.Run(cmd.Context(), runworkflow.RunOptions{
					WorkflowRef: args[0], Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
					WorkingDir: local.workingDir, DryRun: local.dryRun,
				})
				if result.RunID != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\nrun_dir: %s\nstatus: %s\n", result.RunID, result.RunDir, result.Status)
				}
				return err
			}
			return runWorkflowViaDaemon(cmd, args[0], &local)
		},
	}
	addCommonFlags(cmd, &local)
	cmd.Flags().BoolVar(&local.dryRun, "dry-run", false, "validate and plan without executing")
	addInteractiveFlags(cmd, &local)
	return cmd
}

func newWorkflowListCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").ListWorkflows(cmd.Context())
			if err != nil {
				return err
			}
			return renderWorkflowList(cmd.OutOrStdout(), resp.Runs, outputFormat, noColor, isInteractiveWriter(cmd.OutOrStdout()))
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	return cmd
}

func newWorkflowStatusCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	var watch bool
	cmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show workflow run status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if watch {
				return watchWorkflow(cmd, args[0], outputFormat, noColor)
			}
			resp, err := newDaemonClient("").WorkflowStatus(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderWorkflowStatus(cmd.OutOrStdout(), resp.Run, outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	cmd.Flags().BoolVar(&watch, "watch", false, "watch the workflow run until it finishes")
	return cmd
}

func newWorkflowWatchCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	cmd := &cobra.Command{
		Use:   "watch <id>",
		Short: "Watch a workflow run until it finishes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return watchWorkflow(cmd, args[0], outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	return cmd
}

func newWorkflowLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <id>",
		Short: "Print workflow run event logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowLogs(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, line := range resp.Lines {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
}

func newWorkflowCancelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a running workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").CancelWorkflow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run)
			return nil
		},
	}
}

func newWorkflowPauseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "pause <id>",
		Short: "Request a graceful pause of a workflow run",
		Long: "Signal the daemon to pause the run at the next checkpoint-safe boundary. " +
			"A node currently in flight finishes (or fails) before the run halts. " +
			"Use resume to continue from the checkpoint.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").PauseWorkflow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run)
			return nil
		},
	}
}

func newWorkflowResumeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <id>",
		Short: "Resume a paused workflow run from its checkpoint",
		Long: "Restart the run from the last recorded checkpoint. " +
			"For runs paused via execution.pause_when_fail the failed node is re-executed before the run continues. " +
			"Inputs and vars are reused from the original request; no overrides are accepted to keep results reproducible.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").ResumeWorkflow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run)
			return nil
		},
	}
}

func newDaemonCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Control agentflowd",
	}
	cmd.AddCommand(newDaemonStartCommand(opts), newDaemonStopCommand(), newDaemonStatusCommand())
	return cmd
}

func newDaemonStartCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start agentflowd in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newDaemonClient("")
			if status, err := client.Status(cmd.Context()); err == nil && status.Running {
				fmt.Fprintf(cmd.OutOrStdout(), "agentflowd already running\npid: %d\nsocket: %s\n", status.PID, status.Socket)
				return nil
			}
			path, err := findAgentflowd()
			if err != nil {
				return err
			}
			cfg := daemon.DefaultConfig()
			if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
				return err
			}
			logFile, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return err
			}
			proc := exec.Command(path)
			proc.Stdout = logFile
			proc.Stderr = logFile
			proc.Env = os.Environ()
			if opts.codexPath != "" {
				proc.Env = append(proc.Env, "AGENTFLOW_CODEX_PATH="+opts.codexPath)
			}
			if opts.claudePath != "" {
				proc.Env = append(proc.Env, "AGENTFLOW_CLAUDE_PATH="+opts.claudePath)
			}
			if err := proc.Start(); err != nil {
				_ = logFile.Close()
				return err
			}
			_ = logFile.Close()
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				status, err := client.Status(cmd.Context())
				if err == nil && status.Running {
					fmt.Fprintf(cmd.OutOrStdout(), "agentflowd started\npid: %d\nsocket: %s\n", status.PID, status.Socket)
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "agentflowd starting\npid: %d\nsocket: %s\n", proc.Process.Pid, cfg.SocketPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.codexPath, "codex-path", "", "path to codex binary for daemon workflow runs")
	cmd.Flags().StringVar(&opts.claudePath, "claude-path", "", "path to claude binary for daemon workflow runs")
	return cmd
}

func newDaemonStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop agentflowd",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := newDaemonClient("").Stop(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "agentflowd stopping")
			return nil
		},
	}
}

func newDaemonStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show agentflowd status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := newDaemonClient("").Status(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "running: %t\npid: %d\nsocket: %s\nruns: %d\n", status.Running, status.PID, status.Socket, status.Runs)
			return nil
		},
	}
}

func addCommonFlags(cmd *cobra.Command, opts *options) {
	cmd.Flags().StringArrayVar(&opts.inputs, "input", nil, "workflow input key=value, repeatable")
	cmd.Flags().StringVar(&opts.inputJSON, "input-json", "", "JSON file with workflow inputs")
	cmd.Flags().StringArrayVar(&opts.vars, "var", nil, "workflow var override key=value, repeatable")
	cmd.Flags().IntVar(&opts.maxConcurrency, "max-concurrency", 0, "override execution.max_concurrency")
	cmd.Flags().StringVar(&opts.workingDir, "working-dir", ".", "base working directory for the run")
	cmd.Flags().StringVar(&opts.codexPath, "codex-path", "", "path to codex binary")
	cmd.Flags().StringVar(&opts.claudePath, "claude-path", "", "path to claude binary")
	cmd.Flags().StringVar(&opts.logFormat, "log-format", "text", "text or json")
	cmd.Flags().StringVar(&opts.eventsJSONL, "events-jsonl", "", "events JSONL path")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
}

func buildUseCase(opts *options) (*runworkflow.RunWorkflowUseCase, error) {
	return app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   opts.codexPath,
		ClaudePath:  opts.claudePath,
		LogFormat:   opts.logFormat,
		EventsJSONL: opts.eventsJSONL,
		EventWriter: os.Stdout,
	})
}

func addInteractiveFlags(cmd *cobra.Command, opts *options) {
	cmd.Flags().BoolP("it", "i", false, "run locally in the foreground")
	cmd.Flags().BoolP("tty", "t", false, "run locally in the foreground")
	_ = cmd.Flags().MarkHidden("tty")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		it, _ := cmd.Flags().GetBool("it")
		tty, _ := cmd.Flags().GetBool("tty")
		opts.interactive = it || tty
		return nil
	}
}

func runWorkflowViaDaemon(cmd *cobra.Command, workflowRef string, opts *options) error {
	inputs, vars, err := parseInputsAndVars(opts)
	if err != nil {
		return err
	}
	resp, err := newWorkflowRunClient("").RunWorkflow(cmd.Context(), daemon.RunWorkflowRequest{
		WorkflowRef:    workflowRef,
		Inputs:         inputs,
		Vars:           vars,
		MaxConcurrency: opts.maxConcurrency,
		WorkingDir:     opts.workingDir,
		CodexPath:      opts.codexPath,
		ClaudePath:     opts.claudePath,
		LogFormat:      opts.logFormat,
		EventsJSONL:    opts.eventsJSONL,
		DryRun:         opts.dryRun,
	})
	if err != nil {
		return err
	}
	printRun(cmd, resp.Run)
	return nil
}

func printRun(cmd *cobra.Command, run daemon.WorkflowRun) {
	fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\nrun_dir: %s\nstatus: %s\n", run.ID, run.RunDir, run.Status)
	if run.CurrentStep != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "step: %s\n", run.CurrentStep)
	}
	if run.PauseReason != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "pause_reason: %s\n", run.PauseReason)
	}
	if run.ResumeCount > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "resume_count: %d\n", run.ResumeCount)
	}
	if run.Error != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "error: %s\n", run.Error)
	}
}

type workflowOutputFormat string

const (
	workflowOutputText workflowOutputFormat = "text"
	workflowOutputJSON workflowOutputFormat = "json"
)

func renderWorkflowList(w io.Writer, runs []daemon.WorkflowRun, format string, noColor bool, interactive bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(runs)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(runs) == 0 {
		_, err := fmt.Fprintln(w, "no workflow runs")
		return err
	}
	effectiveNoColor := noColor || !interactive
	rows := make([]workflowListRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, workflowListRow{
			ID:        run.ID,
			Status:    normalizeStatus(run.Status),
			Workflow:  run.Workflow,
			Step:      firstNonEmpty(run.CurrentStep, "-"),
			Completed: fmt.Sprintf("%d", len(run.CompletedSteps)),
			Total:     fmt.Sprintf("%d", run.TotalSteps),
			Dir:       run.RunDir,
			Age:       formatAge(run.StartedAt),
		})
	}
	cols := []string{"ID", "WORKFLOW", "STATUS", "STEP ATUAL", "CONCLUÍDOS", "TOTAL", "IDADE", "RUN DIR"}
	widths := []int{6, 20, 12, 18, 10, 5, 8, 0}
	maxWidth := terminalWidth(w)
	if !interactive {
		maxWidth = 0
	}
	for i, col := range cols {
		if len(col) > widths[i] {
			widths[i] = len(col)
		}
	}
	if maxWidth > 0 {
		widths = fitWidths(widths, maxWidth, 2)
	}
	fmt.Fprintln(w, strings.Join(renderHeader(cols, widths), "  "))
	for _, row := range rows {
		line := []string{
			padOrTruncate(row.ID, widths[0]),
			padOrTruncate(row.Workflow, widths[1]),
			padOrTruncate(colorizeStatus(row.Status, effectiveNoColor), widths[2]),
			padOrTruncate(row.Step, widths[3]),
			padOrTruncate(row.Completed, widths[4]),
			padOrTruncate(row.Total, widths[5]),
			padOrTruncate(row.Age, widths[6]),
			padOrTruncate(row.Dir, widths[7]),
		}
		fmt.Fprintln(w, strings.Join(line, "  "))
	}
	return nil
}

type workflowListRow struct {
	ID, Status, Workflow, Step, Completed, Total, Age, Dir string
}

func renderWorkflowStatus(w io.Writer, run daemon.WorkflowRun, format string, noColor bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(run)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	effectiveNoColor := noColor || !isInteractiveWriter(w)
	lines := []string{
		fmt.Sprintf("id: %s", run.ID),
		fmt.Sprintf("workflow: %s", run.Workflow),
		fmt.Sprintf("status: %s", colorizeStatus(normalizeStatus(run.Status), effectiveNoColor)),
		fmt.Sprintf("step: %s", firstNonEmpty(run.CurrentStep, "-")),
		fmt.Sprintf("completed: %d/%d", len(run.CompletedSteps), run.TotalSteps),
		fmt.Sprintf("pending: %d", len(run.PendingSteps)),
		fmt.Sprintf("run_dir: %s", run.RunDir),
	}
	if run.PauseReason != "" {
		lines = append(lines, fmt.Sprintf("pause_reason: %s", run.PauseReason))
	}
	if run.ResumeCount > 0 {
		lines = append(lines, fmt.Sprintf("resume_count: %d", run.ResumeCount))
	}
	if run.Error != "" {
		lines = append(lines, fmt.Sprintf("error: %s", run.Error))
	}
	if run.TerminalError != "" {
		lines = append(lines, fmt.Sprintf("terminal_error: %s", run.TerminalError))
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	if normalizeStatus(run.Status) == "paused" {
		_, _ = fmt.Fprintln(w, "hint: run `agentflow workflow resume "+run.ID+"` to continue")
	}
	return nil
}

func watchWorkflow(cmd *cobra.Command, runID string, format string, noColor bool) error {
	client := newDaemonClient("")
	jsonMode := workflowOutputFormat(format) == workflowOutputJSON
	firstRender := true
	for {
		resp, err := client.WorkflowStatus(cmd.Context(), runID)
		if err != nil {
			return err
		}
		if !jsonMode && !firstRender {
			if err := clearWorkflowWatchOutput(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
		if err := renderWorkflowStatus(cmd.OutOrStdout(), resp.Run, format, noColor); err != nil {
			return err
		}
		firstRender = false
		if isTerminalStatus(resp.Run.Status) {
			return nil
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(workflowWatchInterval):
		}
	}
}

func clearWorkflowWatchOutput(w io.Writer) error {
	_, err := fmt.Fprint(w, "\x1b[H\x1b[2J")
	return err
}

func normalizeStatus(status any) string {
	return strings.ToLower(fmt.Sprint(status))
}

func isTerminalStatus(status any) bool {
	switch normalizeStatus(status) {
	case "success", "failed", "cancelled", "canceled", "timeout", "paused":
		return true
	default:
		return false
	}
}

func colorizeStatus(status string, noColor bool) string {
	if noColor {
		return status
	}
	switch status {
	case "success":
		return "\x1b[32m" + status + "\x1b[0m"
	case "failed", "cancelled", "canceled", "timeout":
		return "\x1b[31m" + status + "\x1b[0m"
	case "running":
		return "\x1b[36m" + status + "\x1b[0m"
	case "paused":
		return "\x1b[33m" + status + "\x1b[0m"
	default:
		return status
	}
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t).Round(time.Second)
	if d < 0 {
		d = 0
	}
	return d.String()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func terminalWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if width := os.Getenv("COLUMNS"); width != "" {
			if n, err := strconv.Atoi(width); err == nil {
				return n
			}
		}
		if info, err := f.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			return 120
		}
	}
	return 0
}

func isInteractiveWriter(w io.Writer) bool { return terminalWidth(w) > 0 }

func fitWidths(widths []int, maxWidth int, gap int) []int {
	result := append([]int(nil), widths...)
	fixed := 0
	for i, width := range result {
		if i == len(result)-1 {
			continue
		}
		fixed += width
	}
	fixed += gap * (len(result) - 1)
	remaining := maxWidth - fixed
	if remaining <= 0 {
		return result
	}
	result[len(result)-1] = remaining
	return result
}

func renderHeader(cols []string, widths []int) []string {
	out := make([]string, len(cols))
	for i := range cols {
		out[i] = padOrTruncate(cols[i], widths[i])
	}
	return out
}

func padOrTruncate(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func findAgentflowd() (string, error) {
	if path := os.Getenv("AGENTFLOWD_PATH"); path != "" {
		return path, nil
	}
	if path, err := exec.LookPath("agentflowd"); err == nil {
		return path, nil
	}
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "agentflowd")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("agentflowd binary not found in PATH; build it with go build ./cmd/agentflowd")
}

func parseInputsAndVars(opts *options) (map[string]any, map[string]any, error) {
	inputs := map[string]any{}
	if opts.inputJSON != "" {
		data, err := os.ReadFile(opts.inputJSON)
		if err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(data, &inputs); err != nil {
			return nil, nil, err
		}
	}
	parsedInputs, err := parsePairs(opts.inputs)
	if err != nil {
		return nil, nil, err
	}
	for key, value := range parsedInputs {
		inputs[key] = value
	}
	vars, err := parsePairs(opts.vars)
	if err != nil {
		return nil, nil, err
	}
	return inputs, vars, nil
}

func parsePairs(pairs []string) (map[string]any, error) {
	out := map[string]any{}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid key=value pair %q", pair)
		}
		out[key] = parseScalar(value)
	}
	return out, nil
}

func parseScalar(value string) any {
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return decoded
	}
	return value
}

func Execute(ctx context.Context) error {
	cmd := NewRootCommand()
	cmd.SetContext(ctx)
	return cmd.Execute()
}
