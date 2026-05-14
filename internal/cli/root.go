package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	codexagent "github.com/diasYuri/agentflow/internal/adapters/agent/codex"
	"github.com/diasYuri/agentflow/internal/adapters/events/jsonl"
	"github.com/diasYuri/agentflow/internal/adapters/events/multi"
	"github.com/diasYuri/agentflow/internal/adapters/events/stdout"
	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type options struct {
	inputs         []string
	inputJSON      string
	vars           []string
	maxConcurrency int
	workingDir     string
	outputDir      string
	codexPath      string
	logFormat      string
	eventsJSONL    string
	graphFormat    string
	dryRun         bool
	noColor        bool
}

func NewRootCommand() *cobra.Command {
	opts := &options{logFormat: "text"}
	cmd := &cobra.Command{
		Use:   "agentflow",
		Short: "Run local YAML workflows for agent coding",
	}
	cmd.AddCommand(newValidateCommand(opts), newDryRunCommand(opts), newRunCommand(opts), newGraphCommand(opts))
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
				WorkingDir: local.workingDir, OutputDir: local.outputDir,
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
		Short: "Run a workflow",
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
			result, err := uc.Run(cmd.Context(), runworkflow.RunOptions{
				WorkflowRef: args[0], Inputs: inputs, Vars: vars, MaxConcurrency: local.maxConcurrency,
				WorkingDir: local.workingDir, OutputDir: local.outputDir, DryRun: local.dryRun,
			})
			if result.RunID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\nrun_dir: %s\nstatus: %s\n", result.RunID, result.RunDir, result.Status)
			}
			return err
		},
	}
	addCommonFlags(cmd, &local)
	cmd.Flags().BoolVar(&local.dryRun, "dry-run", false, "validate and plan without executing")
	return cmd
}

func addCommonFlags(cmd *cobra.Command, opts *options) {
	cmd.Flags().StringArrayVar(&opts.inputs, "input", nil, "workflow input key=value, repeatable")
	cmd.Flags().StringVar(&opts.inputJSON, "input-json", "", "JSON file with workflow inputs")
	cmd.Flags().StringArrayVar(&opts.vars, "var", nil, "workflow var override key=value, repeatable")
	cmd.Flags().IntVar(&opts.maxConcurrency, "max-concurrency", 0, "override execution.max_concurrency")
	cmd.Flags().StringVar(&opts.workingDir, "working-dir", ".", "base working directory for the run")
	cmd.Flags().StringVar(&opts.outputDir, "output-dir", "", "run output directory (ignored)")
	cmd.Flags().StringVar(&opts.codexPath, "codex-path", "", "path to codex binary")
	cmd.Flags().StringVar(&opts.logFormat, "log-format", "text", "text or json")
	cmd.Flags().StringVar(&opts.eventsJSONL, "events-jsonl", "", "events JSONL path")
	cmd.Flags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
}

func buildUseCase(opts *options) (*runworkflow.RunWorkflowUseCase, error) {
	eventSink, err := jsonl.New(opts.eventsJSONL)
	if err != nil {
		return nil, err
	}
	sink := multi.New(eventSink, stdout.New(os.Stdout, opts.logFormat))
	registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
		"codex": codexagent.New(opts.codexPath),
	})
	return &runworkflow.RunWorkflowUseCase{
		Workflows: yamlrepo.NewWorkflowRepository(),
		Runs:      runrepo.New(""),
		Events:    sink,
		Agents:    registry,
		Shell:     shell.NewRunner(),
	}, nil
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
