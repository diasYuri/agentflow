package cli

import (
	"context"
	"encoding/json"
	"fmt"
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

var newWorkflowRunClient = func(socketPath string) workflowRunClient {
	return daemon.NewClient(socketPath)
}

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
		newWorkflowLogsCommand(),
		newWorkflowCancelCommand(),
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
	return &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := daemon.NewClient("").ListWorkflows(cmd.Context())
			if err != nil {
				return err
			}
			for _, run := range resp.Runs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", run.ID, run.Status, run.Workflow, run.RunDir)
			}
			return nil
		},
	}
}

func newWorkflowStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show workflow run status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := daemon.NewClient("").WorkflowStatus(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			printRun(cmd, resp.Run)
			return nil
		},
	}
}

func newWorkflowLogsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logs <id>",
		Short: "Print workflow run event logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resp, err := daemon.NewClient("").WorkflowLogs(cmd.Context(), args[0])
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
			resp, err := daemon.NewClient("").CancelWorkflow(cmd.Context(), args[0])
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
			client := daemon.NewClient("")
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
			if _, err := daemon.NewClient("").Stop(cmd.Context()); err != nil {
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
			status, err := daemon.NewClient("").Status(cmd.Context())
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
	if run.Error != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "error: %s\n", run.Error)
	}
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
