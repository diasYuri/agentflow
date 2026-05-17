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

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/diasYuri/agentflow/internal/app"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
	tuiapp "github.com/diasYuri/agentflow/internal/tui/app"
	"github.com/diasYuri/agentflow/internal/tui/theme"
)

type options struct {
	inputs         []string
	inputJSON      string
	vars           []string
	maxConcurrency int
	workingDir     string
	project        string
	codexPath      string
	claudePath     string
	piPath         string
	logFormat      string
	eventsJSONL    string
	graphFormat    string
	dryRun         bool
	interactive    bool
	noColor        bool
	tag            string
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
	ApproveWorkflow(context.Context, string) (daemon.ApproveWorkflowResponse, error)
	RejectWorkflow(context.Context, string) (daemon.RejectWorkflowResponse, error)
	WorkflowArtifacts(context.Context, string) (daemon.WorkflowArtifactsResponse, error)
	WorkflowArtifact(context.Context, string, string) (daemon.WorkflowArtifactResponse, error)
	WorkflowArtifactPath(context.Context, string, string) (string, error)
	WorkflowSummary(context.Context, string) (daemon.WorkflowSummaryResponse, error)
	WorkflowTimeline(context.Context, string, int, int) (daemon.WorkflowTimelineResponse, error)
	WorkflowInspect(context.Context, string) (daemon.WorkflowInspectResponse, error)
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
		newMigrateCommand(),
		newProjectCommand(),
		newDaemonCommand(opts),
		newWorkflowCommand(opts),
		newTUICommand(),
		newVersionCommand(),
		newCompletionCommand(cmd),
		newDoctorCommand(),
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
			ref, err := resolveWorkflowRefForCLI(args[0], opts.project)
			if err != nil {
				return err
			}
			plan, err := uc.Validate(cmd.Context(), ref)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "valid: %s (%d nodes)\n", plan.Workflow.Name, len(plan.Order))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.project, "project", "", "project name to resolve the workflow within")
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
			ref, err := resolveWorkflowRefForCLI(args[0], local.project)
			if err != nil {
				return err
			}
			plan, err := uc.Validate(cmd.Context(), ref)
			if err != nil {
				return err
			}
			return workflow.WriteMermaidGraph(cmd.OutOrStdout(), plan)
		},
	}
	cmd.Flags().StringVar(&local.project, "project", "", "project name to resolve the workflow within")
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
			ref, workingDir, err := resolveWorkflowRunContext(cmd, args[0], &local)
			if err != nil {
				return err
			}
			plan, resolved, err := uc.DryRun(cmd.Context(), runworkflow.RunOptions{
				WorkflowRef: ref, Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
				WorkingDir: workingDir,
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
			ref, workingDir, err := resolveWorkflowRunContext(cmd, args[0], &local)
			if err != nil {
				return err
			}
			result, err := uc.Run(cmd.Context(), runworkflow.RunOptions{
				WorkflowRef: ref, Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
				WorkingDir: workingDir, DryRun: local.dryRun, Tag: local.tag,
			})
			if result.RunID != "" {
				printRun(cmd, daemon.WorkflowRun{
					ID:            result.RunID,
					RunDir:        result.RunDir,
					Status:        corerun.RunStatus(result.Status),
					Tag:           result.Summary.Tag,
					PauseReason:   string(result.PauseReason),
					FailureReason: result.FailureReason,
				}, isInteractiveWriter(cmd.OutOrStdout()))
			}
			return err
		},
	}
	addCommonFlags(cmd, &local)
	cmd.Flags().BoolVar(&local.dryRun, "dry-run", false, "validate and plan without executing")
	cmd.Flags().StringVar(&local.tag, "tag", "", "friendly name for this workflow run")
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
		newWorkflowArtifactsCommand(),
		newWorkflowArtifactCommand(),
		newWorkflowScheduleCommand(opts),
		newWorkflowCancelCommand(),
		newWorkflowPauseCommand(),
		newWorkflowResumeCommand(),
		newWorkflowApproveCommand(),
		newWorkflowRejectCommand(),
		newWorkflowSummaryCommand(),
		newWorkflowTimelineCommand(),
		newWorkflowInspectCommand(),
	)
	return cmd
}

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage named project roots",
	}
	cmd.AddCommand(newProjectAddCommand(), newProjectListCommand(), newProjectRemoveCommand())
	return cmd
}

func newProjectAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register a project root",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := newCLIProjectRegistry()
			if err := registry.Add(args[0], args[1]); err != nil {
				return err
			}
			project, err := registry.Resolve(args[0])
			if err != nil {
				return err
			}
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("Project added"),
				f.keyValueLines([][2]string{
					{"name", project.Name},
					{"path", project.Path},
				}),
			))
			return nil
		},
	}
	return cmd
}

func newProjectListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := newCLIProjectRegistry()
			projects, err := registry.List()
			if err != nil {
				return err
			}
			if len(projects) == 0 {
				f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
				fmt.Fprintln(cmd.OutOrStdout(), f.note("No projects"))
				return nil
			}
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			for _, project := range projects {
				fmt.Fprintln(cmd.OutOrStdout(), f.block(
					f.section("Project"),
					f.keyValueLines([][2]string{
						{"name", project.Name},
						{"path", project.Path},
					}),
				))
			}
			return nil
		},
	}
	return cmd
}

func newProjectRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a registered project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := newCLIProjectRegistry()
			if err := registry.Remove(args[0]); err != nil {
				return err
			}
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("Project removed"),
				[]string{f.labelValue("name", args[0])},
			))
			return nil
		},
	}
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
				ref, workingDir, err := resolveWorkflowRunContext(cmd, args[0], &local)
				if err != nil {
					return err
				}
				result, err := uc.Run(cmd.Context(), runworkflow.RunOptions{
					WorkflowRef: ref, Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
					WorkingDir: workingDir, DryRun: local.dryRun, Tag: local.tag,
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
	cmd.Flags().StringVar(&local.tag, "tag", "", "friendly name for this workflow run")
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

func newWorkflowArtifactsCommand() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "artifacts <run_id>",
		Short: "List artifacts for a workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowArtifacts(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderWorkflowArtifacts(cmd.OutOrStdout(), resp, outputFormat)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	return cmd
}

func newWorkflowArtifactCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Inspect a workflow artifact",
	}
	cmd.AddCommand(newWorkflowArtifactShowCommand(), newWorkflowArtifactPathCommand())
	return cmd
}

func newWorkflowArtifactShowCommand() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "show <run_id> <artifact_id>",
		Short: "Show artifact content and metadata",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowArtifact(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return renderWorkflowArtifact(cmd.OutOrStdout(), resp, outputFormat)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	return cmd
}

func newWorkflowArtifactPathCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path <run_id> <artifact_id>",
		Short: "Print local filesystem path for an artifact",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := newDaemonClient("").WorkflowArtifactPath(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	return cmd
}

func renderWorkflowArtifacts(w io.Writer, resp daemon.WorkflowArtifactsResponse, format string) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(resp.Artifacts) == 0 {
		_, err := fmt.Fprintln(w, newCLIFormat(isInteractiveWriter(w)).note("No artifacts"))
		return err
	}
	f := newCLIFormat(isInteractiveWriter(w))
	headers := []string{"ID", "NAME", "TYPE", "SIZE", "NODE", "INSTANCE"}
	widths := []int{28, 24, 18, 10, 14, 14}
	rows := make([][]string, 0, len(resp.Artifacts))
	for _, a := range resp.Artifacts {
		rows = append(rows, []string{
			a.ID,
			a.Name,
			a.MediaType,
			humanizeBytes(a.SizeBytes),
			firstNonEmpty(a.NodeID, "-"),
			firstNonEmpty(a.InstanceID, "-"),
		})
	}
	_, err := fmt.Fprint(w, f.table(headers, rows, widths))
	return err
}

func renderWorkflowArtifact(w io.Writer, resp daemon.WorkflowArtifactResponse, format string) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	f := newCLIFormat(isInteractiveWriter(w))
	lines := []string{
		f.labelValue("id", resp.ID),
		f.labelValue("name", resp.Name),
		f.labelValue("size", humanizeBytes(resp.SizeBytes)),
		f.labelValue("media_type", resp.MediaType),
		f.labelValue("kind", string(resp.Kind)),
	}
	if resp.NodeID != "" {
		lines = append(lines, f.labelValue("node_id", resp.NodeID))
	}
	if resp.InstanceID != "" {
		lines = append(lines, f.labelValue("instance_id", resp.InstanceID))
	}
	if resp.Description != "" {
		lines = append(lines, f.labelValue("description", resp.Description))
	}
	if resp.Truncated {
		lines = append(lines, f.labelValue("truncated", fmt.Sprintf("true (limit %d bytes)", daemon.MaxArtifactInline)))
	}
	if resp.IsText {
		lines = append(lines, f.section("Content"))
		lines = append(lines, resp.TextContent)
	} else if resp.Content != "" {
		lines = append(lines, f.section("Content"))
		lines = append(lines, f.note(fmt.Sprintf("[binary content, %s encoded, %s]", resp.Encoding, humanizeBytes(resp.SizeBytes))))
	} else {
		lines = append(lines, f.note(fmt.Sprintf("[binary content omitted, size %s]", humanizeBytes(resp.SizeBytes))))
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func humanizeBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1024*1024))
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
			printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
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
			printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
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
			printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
			return nil
		},
	}
}

func newWorkflowApproveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a workflow run waiting on human decision",
		Long:  "Resume a run that is waiting in the approval state. The run must be in wait_approval.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").ApproveWorkflow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
			return nil
		},
	}
}

func newWorkflowRejectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a workflow run waiting on human decision",
		Long:  "Fail a run that is waiting in the approval state. The run must be in wait_approval.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").RejectWorkflow(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
			return nil
		},
	}
}

func newWorkflowSummaryCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	cmd := &cobra.Command{
		Use:   "summary <run_id>",
		Short: "Show workflow run summary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowSummary(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderWorkflowSummary(cmd.OutOrStdout(), resp, outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	return cmd
}

func newWorkflowTimelineCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	var cursor int
	var limit int
	cmd := &cobra.Command{
		Use:   "timeline <run_id>",
		Short: "Show workflow run timeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowTimeline(cmd.Context(), args[0], cursor, limit)
			if err != nil {
				return err
			}
			return renderWorkflowTimeline(cmd.OutOrStdout(), resp, outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	cmd.Flags().IntVar(&cursor, "cursor", 0, "pagination cursor")
	cmd.Flags().IntVar(&limit, "limit", 0, "page limit (default 100, max 1000)")
	return cmd
}

func newWorkflowInspectCommand() *cobra.Command {
	var outputFormat string
	var noColor bool
	cmd := &cobra.Command{
		Use:   "inspect <run_id>",
		Short: "Inspect workflow run diagnostics",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := newDaemonClient("").WorkflowInspect(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return renderWorkflowInspect(cmd.OutOrStdout(), resp, outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	return cmd
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
				f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
				fmt.Fprintln(cmd.OutOrStdout(), f.block(
					f.title("agentflowd already running"),
					f.keyValueLines([][2]string{
						{"pid", fmt.Sprint(status.PID)},
						{"socket", status.Socket},
					}),
				))
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
			proc.Env = daemonProviderEnv(os.Environ(), opts)
			if err := proc.Start(); err != nil {
				_ = logFile.Close()
				return err
			}
			_ = logFile.Close()
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				status, err := client.Status(cmd.Context())
				if err == nil && status.Running {
					f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
					fmt.Fprintln(cmd.OutOrStdout(), f.block(
						f.title("agentflowd started"),
						f.keyValueLines([][2]string{
							{"pid", fmt.Sprint(status.PID)},
							{"socket", status.Socket},
						}),
					))
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("agentflowd starting"),
				f.keyValueLines([][2]string{
					{"pid", fmt.Sprint(proc.Process.Pid)},
					{"socket", cfg.SocketPath},
				}),
			))
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.codexPath, "codex-path", "", "path to codex binary for daemon workflow runs")
	cmd.Flags().StringVar(&opts.claudePath, "claude-path", "", "path to claude binary for daemon workflow runs")
	cmd.Flags().StringVar(&opts.piPath, "pi-path", "", "path to pi binary for daemon workflow runs")
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
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.note("agentflowd stopping"))
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
			f := newCLIFormat(isInteractiveWriter(cmd.OutOrStdout()))
			fmt.Fprintln(cmd.OutOrStdout(), f.block(
				f.title("agentflowd status"),
				f.keyValueLines([][2]string{
					{"running", fmt.Sprint(status.Running)},
					{"pid", fmt.Sprint(status.PID)},
					{"socket", status.Socket},
					{"runs", fmt.Sprint(status.Runs)},
				}),
			))
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
	cmd.Flags().StringVar(&opts.project, "project", "", "project name to resolve the workflow within")
	cmd.Flags().StringVar(&opts.codexPath, "codex-path", "", "path to codex binary")
	cmd.Flags().StringVar(&opts.claudePath, "claude-path", "", "path to claude binary")
	cmd.Flags().StringVar(&opts.piPath, "pi-path", "", "path to pi binary")
	cmd.Flags().StringVar(&opts.logFormat, "log-format", "text", "text or json")
	cmd.Flags().StringVar(&opts.eventsJSONL, "events-jsonl", "", "events JSONL path")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
}

func buildUseCase(opts *options) (*runworkflow.RunWorkflowUseCase, error) {
	return app.NewRunWorkflowUseCase(app.RuntimeOptions{
		CodexPath:   opts.codexPath,
		ClaudePath:  opts.claudePath,
		PiPath:      opts.piPath,
		LogFormat:   opts.logFormat,
		EventsJSONL: opts.eventsJSONL,
		EventWriter: os.Stdout,
	})
}

func daemonProviderEnv(base []string, opts *options) []string {
	env := append([]string{}, base...)
	if opts.codexPath != "" {
		env = append(env, "AGENTFLOW_CODEX_PATH="+opts.codexPath)
	}
	if opts.claudePath != "" {
		env = append(env, "AGENTFLOW_CLAUDE_PATH="+opts.claudePath)
	}
	if opts.piPath != "" {
		env = append(env, "AGENTFLOW_PI_PATH="+opts.piPath)
	}
	return env
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

func newCLIProjectRegistry() *app.ProjectRegistry {
	return app.NewProjectRegistry(app.NewJSONProjectStore(app.DefaultProjectsPath()))
}

func isWorkflowPath(ref string) bool {
	ext := strings.ToLower(filepath.Ext(ref))
	if ext != ".yaml" && ext != ".yml" {
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

func resolveWorkflowRefForCLI(workflowRef, projectName string) (string, error) {
	projectName = strings.TrimSpace(projectName)
	if projectName == "" || isWorkflowPath(workflowRef) {
		return workflowRef, nil
	}
	project, err := newCLIProjectRegistry().Resolve(projectName)
	if err != nil {
		return "", err
	}
	return app.ResolveWorkflowRef(app.Project{Name: project.Name, Path: project.Path}, workflowRef)
}

func resolveWorkflowRunContext(cmd *cobra.Command, workflowRef string, opts *options) (string, string, error) {
	resolvedRef, err := resolveWorkflowRefForCLI(workflowRef, opts.project)
	if err != nil {
		return "", "", err
	}

	workingDir := opts.workingDir
	workingDirExplicit := cmd.Flags().Changed("working-dir")
	projectName := strings.TrimSpace(opts.project)
	if projectName != "" && !workingDirExplicit {
		project, err := newCLIProjectRegistry().Resolve(projectName)
		if err != nil {
			return "", "", err
		}
		workingDir = project.Path
	}
	normalizedWorkingDir, err := daemonWorkingDir(workingDir)
	if err != nil {
		return "", "", err
	}
	return resolvedRef, normalizedWorkingDir, nil
}

func runWorkflowViaDaemon(cmd *cobra.Command, workflowRef string, opts *options) error {
	inputs, vars, err := parseInputsAndVars(opts)
	if err != nil {
		return err
	}
	ref, workingDir, err := resolveWorkflowRunContext(cmd, workflowRef, opts)
	if err != nil {
		return err
	}
	resp, err := newWorkflowRunClient("").RunWorkflow(cmd.Context(), daemon.RunWorkflowRequest{
		WorkflowRef:    ref,
		Inputs:         inputs,
		Vars:           vars,
		MaxConcurrency: opts.maxConcurrency,
		WorkingDir:     workingDir,
		CodexPath:      opts.codexPath,
		ClaudePath:     opts.claudePath,
		PiPath:         opts.piPath,
		LogFormat:      opts.logFormat,
		EventsJSONL:    opts.eventsJSONL,
		DryRun:         opts.dryRun,
		Tag:            opts.tag,
	})
	if err != nil {
		return err
	}
	printRun(cmd, resp.Run, isInteractiveWriter(cmd.OutOrStdout()))
	return nil
}

func daemonWorkingDir(workingDir string) (string, error) {
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
	return abs, nil
}

func printRun(cmd *cobra.Command, run daemon.WorkflowRun, colored bool) {
	f := newCLIFormat(colored)
	lines := []string{
		f.labelValue("run_id", run.ID),
		f.labelValue("run_dir", run.RunDir),
		f.labelValue("status", f.status(string(run.Status))),
	}
	if run.Tag != "" {
		lines = append(lines, f.labelValue("tag", run.Tag))
	}
	if run.CurrentStep != "" {
		lines = append(lines, f.labelValue("step", run.CurrentStep))
	}
	if run.PauseReason != "" {
		lines = append(lines, f.labelValue("pause_reason", run.PauseReason))
	}
	if run.ApprovalNodeID != "" {
		lines = append(lines, f.labelValue("approval_node", run.ApprovalNodeID))
	}
	if run.ApprovalMessage != "" {
		lines = append(lines, f.labelValue("approval_message", run.ApprovalMessage))
	}
	if !run.ApprovalAt.IsZero() {
		lines = append(lines, f.labelValue("approval_at", run.ApprovalAt.Format(time.RFC3339)))
	}
	if run.ResumeCount > 0 {
		lines = append(lines, f.labelValue("resume_count", fmt.Sprint(run.ResumeCount)))
	}
	if run.Error != "" {
		lines = append(lines, f.labelValue("error", run.Error))
	}
	if run.FailureReason != "" {
		lines = append(lines, f.labelValue("failure_reason", run.FailureReason))
	}
	if run.TerminalError != "" {
		lines = append(lines, f.labelValue("terminal_error", run.TerminalError))
	}
	fmt.Fprintln(cmd.OutOrStdout(), f.block(f.title("Run"), lines))
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
		_, err := fmt.Fprintln(w, newCLIFormat(noColor || !interactive).note("No workflow runs"))
		return err
	}
	effectiveNoColor := noColor || !interactive
	f := newCLIFormat(!effectiveNoColor)
	rows := make([]workflowListRow, 0, len(runs))
	for _, run := range runs {
		rows = append(rows, workflowListRow{
			ID:        run.ID,
			Status:    normalizeStatus(run.Status),
			Tag:       firstNonEmpty(run.Tag, "-"),
			Workflow:  run.Workflow,
			Step:      firstNonEmpty(run.CurrentStep, "-"),
			Completed: fmt.Sprintf("%d", len(run.CompletedSteps)),
			Total:     fmt.Sprintf("%d", run.TotalSteps),
			Dir:       run.RunDir,
			Age:       formatWorkflowElapsed(run),
		})
	}
	cols := []string{"ID", "TAG", "WORKFLOW", "STATUS", "STEP ATUAL", "CONCLUÍDOS", "TOTAL", "TEMPO", "RUN DIR"}
	widths := []int{6, 12, 20, 12, 18, 10, 5, 8, 0}
	maxWidth := terminalWidth(w)
	if !interactive {
		maxWidth = 0
	}
	for i, col := range cols {
		if w := lipgloss.Width(col); w > widths[i] {
			widths[i] = w
		}
	}
	if maxWidth > 0 {
		widths = fitWidths(widths, maxWidth, 2)
	}
	tableRows := make([][]string, 0, len(rows))
	for _, row := range rows {
		tableRows = append(tableRows, []string{
			row.ID,
			row.Tag,
			row.Workflow,
			row.Status,
			row.Step,
			row.Completed,
			row.Total,
			row.Age,
			row.Dir,
		})
	}
	_, err := fmt.Fprint(w, f.table(cols, tableRows, widths))
	return err
}

type workflowListRow struct {
	ID, Status, Tag, Workflow, Step, Completed, Total, Age, Dir string
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
	f := newCLIFormat(!(noColor || !isInteractiveWriter(w)))
	lines := []string{
		f.labelValue("id", run.ID),
		f.labelValue("workflow", run.Workflow),
		f.labelValue("tag", firstNonEmpty(run.Tag, "-")),
		f.labelValue("status", f.status(normalizeStatus(run.Status))),
		f.labelValue("step", firstNonEmpty(run.CurrentStep, "-")),
		f.labelValue("completed", fmt.Sprintf("%d/%d", len(run.CompletedSteps), run.TotalSteps)),
		f.labelValue("pending", fmt.Sprintf("%d", len(run.PendingSteps))),
		f.labelValue("run_dir", run.RunDir),
	}
	if run.PauseReason != "" {
		lines = append(lines, f.labelValue("pause_reason", run.PauseReason))
	}
	if run.ResumeCount > 0 {
		lines = append(lines, f.labelValue("resume_count", fmt.Sprint(run.ResumeCount)))
	}
	if run.Error != "" {
		lines = append(lines, f.labelValue("error", run.Error))
	}
	if run.FailureReason != "" {
		lines = append(lines, f.labelValue("failure_reason", run.FailureReason))
	}
	if run.ApprovalNodeID != "" {
		lines = append(lines, f.labelValue("approval_node", run.ApprovalNodeID))
	}
	if run.ApprovalMessage != "" {
		lines = append(lines, f.labelValue("approval_message", run.ApprovalMessage))
	}
	if !run.ApprovalAt.IsZero() {
		lines = append(lines, f.labelValue("approval_at", run.ApprovalAt.Format(time.RFC3339)))
	}
	if run.TerminalError != "" {
		lines = append(lines, f.labelValue("terminal_error", run.TerminalError))
	}
	if _, err := fmt.Fprintln(w, f.block(f.title("Workflow status"), lines)); err != nil {
		return err
	}
	switch normalizeStatus(run.Status) {
	case "paused":
		_, _ = fmt.Fprintln(w, f.note("hint: run `agentflow workflow resume "+run.ID+"` to continue"))
	case "wait_approval":
		_, _ = fmt.Fprintln(w, f.note("hint: run `agentflow workflow approve "+run.ID+"` or `agentflow workflow reject "+run.ID+"`"))
	}
	return nil
}

func renderWorkflowSummary(w io.Writer, resp daemon.WorkflowSummaryResponse, format string, noColor bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	f := newCLIFormat(!(noColor || !isInteractiveWriter(w)))
	s := resp.Summary
	lines := []string{
		f.labelValue("run_id", resp.RunID),
		f.labelValue("workflow", s.Workflow),
		f.labelValue("status", f.status(normalizeStatus(s.Status))),
		f.labelValue("started_at", s.StartedAt.Format(time.RFC3339)),
	}
	if !s.FinishedAt.IsZero() {
		lines = append(lines, f.labelValue("finished_at", s.FinishedAt.Format(time.RFC3339)))
	}
	if s.DurationMS > 0 {
		lines = append(lines, f.labelValue("duration", (time.Duration(s.DurationMS)*time.Millisecond).String()))
	}
	lines = append(lines,
		f.labelValue("agent_calls", fmt.Sprint(s.AgentCalls)),
		f.labelValue("bash_calls", fmt.Sprint(s.BashCalls)),
		f.labelValue("failed_nodes", fmt.Sprint(s.FailedNodes)),
		f.labelValue("retries", fmt.Sprint(s.Retries)),
		f.labelValue("artifact_count", fmt.Sprint(s.ArtifactCount)),
	)
	if s.Tag != "" {
		lines = append(lines, f.labelValue("tag", s.Tag))
	}
	if s.FirstError != "" {
		lines = append(lines, f.labelValue("first_error", s.FirstError))
	}
	if len(s.SlowestNodes) > 0 {
		lines = append(lines, f.section("Slowest nodes"))
		for _, n := range s.SlowestNodes {
			lines = append(lines, f.labelValue(n.NodeID, (time.Duration(n.DurationMS)*time.Millisecond).String()))
		}
	}
	if len(s.AgentUsage) > 0 {
		lines = append(lines, f.section("Agent usage"))
		for _, u := range s.AgentUsage {
			lines = append(lines, f.labelValue(u.Provider, fmt.Sprintf("tokens=%d/%d cost=%.4f", u.InputTokens, u.OutputTokens, u.CostUSD)))
		}
	}
	_, err := fmt.Fprintln(w, f.block(f.title("Workflow summary"), lines))
	return err
}

func renderWorkflowTimeline(w io.Writer, resp daemon.WorkflowTimelineResponse, format string, noColor bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(resp.Entries) == 0 {
		_, err := fmt.Fprintln(w, newCLIFormat(!(noColor || !isInteractiveWriter(w))).note("No timeline entries"))
		return err
	}
	f := newCLIFormat(!(noColor || !isInteractiveWriter(w)))
	lines := []string{f.labelValue("run_id", resp.RunID)}
	for _, entry := range resp.Entries {
		nodeInfo := ""
		if entry.NodeID != "" {
			nodeInfo = fmt.Sprintf(" [%s", entry.NodeID)
			if entry.InstanceID != "" {
				nodeInfo += "/" + entry.InstanceID
			}
			if entry.Attempt > 0 {
				nodeInfo += fmt.Sprintf(" attempt=%d", entry.Attempt)
			}
			nodeInfo += "]"
		}
		duration := ""
		if entry.DurationMS > 0 {
			duration = fmt.Sprintf(" (%s)", (time.Duration(entry.DurationMS) * time.Millisecond).String())
		}
		lines = append(lines, fmt.Sprintf("%s  %s%s%s", entry.Timestamp.Format(time.RFC3339), entry.Type, nodeInfo, duration))
	}
	if resp.HasMore {
		lines = append(lines, f.note(fmt.Sprintf("--more (cursor=%d)", resp.NextCursor)))
	}
	_, err := fmt.Fprintln(w, f.block(f.title("Workflow timeline"), lines))
	return err
}

func renderWorkflowInspect(w io.Writer, resp daemon.WorkflowInspectResponse, format string, noColor bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	f := newCLIFormat(!(noColor || !isInteractiveWriter(w)))
	lines := []string{
		f.labelValue("run_id", resp.RunID),
		f.labelValue("workflow", resp.Workflow),
		f.labelValue("status", f.status(normalizeStatus(resp.Status))),
		f.labelValue("started_at", resp.StartedAt.Format(time.RFC3339)),
	}
	if !resp.FinishedAt.IsZero() {
		lines = append(lines, f.labelValue("finished_at", resp.FinishedAt.Format(time.RFC3339)))
	}
	if resp.DurationMS > 0 {
		lines = append(lines, f.labelValue("duration", (time.Duration(resp.DurationMS)*time.Millisecond).String()))
	}
	lines = append(lines,
		f.labelValue("step", firstNonEmpty(resp.CurrentStep, "-")),
		f.labelValue("completed", fmt.Sprintf("%d/%d", len(resp.CompletedSteps), resp.TotalSteps)),
		f.labelValue("pending", fmt.Sprintf("%d", len(resp.PendingSteps))),
		f.labelValue("failed_nodes", fmt.Sprint(resp.FailedNodes)),
		f.labelValue("retries", fmt.Sprint(resp.Retries)),
		f.labelValue("agent_calls", fmt.Sprint(resp.AgentCalls)),
		f.labelValue("bash_calls", fmt.Sprint(resp.BashCalls)),
		f.labelValue("node_count", fmt.Sprint(resp.NodeCount)),
		f.labelValue("artifact_count", fmt.Sprint(resp.ArtifactCount)),
	)
	if resp.Tag != "" {
		lines = append(lines, f.labelValue("tag", resp.Tag))
	}
	if resp.ApprovalNodeID != "" {
		lines = append(lines, f.labelValue("approval_node", resp.ApprovalNodeID))
	}
	if resp.ApprovalMessage != "" {
		lines = append(lines, f.labelValue("approval_message", resp.ApprovalMessage))
	}
	if !resp.ApprovalAt.IsZero() {
		lines = append(lines, f.labelValue("approval_at", resp.ApprovalAt.Format(time.RFC3339)))
	}
	if resp.FirstError != "" {
		lines = append(lines, f.labelValue("first_error", resp.FirstError))
	}
	if resp.Error != "" {
		lines = append(lines, f.labelValue("error", resp.Error))
	}
	if resp.FailureReason != "" {
		lines = append(lines, f.labelValue("failure_reason", resp.FailureReason))
	}
	if len(resp.SlowestNodes) > 0 {
		lines = append(lines, f.section("Slowest nodes"))
		for _, n := range resp.SlowestNodes {
			lines = append(lines, f.labelValue(n.NodeID, (time.Duration(n.DurationMS)*time.Millisecond).String()))
		}
	}
	_, err := fmt.Fprintln(w, f.block(f.title("Workflow inspect"), lines))
	return err
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

func formatWorkflowElapsed(run daemon.WorkflowRun) string {
	if run.StartedAt.IsZero() {
		return "-"
	}
	if isTerminalStatus(run.Status) && !run.FinishedAt.IsZero() {
		d := run.FinishedAt.Sub(run.StartedAt).Round(time.Second)
		if d < 0 {
			d = 0
		}
		return d.String()
	}
	return formatAge(run.StartedAt)
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

func newTUICommand() *cobra.Command {
	var opts tuiapp.Options
	var noMouse bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			settings := tuiapp.LoadTUISettings()
			opts = tuiapp.ResolveStartupOptions(opts, settings, noMouse)
			p := tuiapp.NewProgram(opts)
			_, err := p.Run()
			return err
		},
	}
	cmd.Flags().StringVar(&opts.Workflow, "workflow", "", "initial workflow to select")
	cmd.Flags().StringVar(&opts.Run, "run", "", "initial run to select")
	cmd.Flags().BoolVar(&opts.Daemon, "daemon", false, "require daemon connection")
	cmd.Flags().BoolVar(&noMouse, "no-mouse", false, "disable mouse support")
	cmd.Flags().Var(&themeValue{mode: &opts.Theme}, "theme", "theme mode (auto, light, dark)")
	return cmd
}

type themeValue struct {
	mode *theme.Mode
}

func (v *themeValue) String() string { return string(*v.mode) }
func (v *themeValue) Set(s string) error {
	switch s {
	case "auto", "light", "dark":
		*v.mode = theme.Mode(s)
		return nil
	default:
		return fmt.Errorf("invalid theme %q", s)
	}
}
func (v *themeValue) Type() string { return "string" }

func Execute(ctx context.Context) error {
	cmd := NewRootCommand()
	cmd.SetContext(ctx)
	return cmd.Execute()
}
