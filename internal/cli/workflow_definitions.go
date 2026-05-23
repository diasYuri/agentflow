package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type workflowDefinitionRoot struct {
	path   string
	source string
}

type workflowDefinitionSummary struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	NodeCount   int    `json:"node_count"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

type workflowDefinitionMetadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	NodeCount   int    `json:"node_count"`
}

type workflowDefinitionDetail struct {
	Workflow workflowDefinitionMetadata      `json:"workflow"`
	Path     string                          `json:"path"`
	Inputs   map[string]workflow.InputSpec   `json:"inputs"`
	Outputs  map[string]workflow.OutputSpec  `json:"outputs"`
	Graph    string                          `json:"graph"`
	Order    []string                        `json:"order"`
	Nodes    map[string]workflow.PlannedNode `json:"nodes"`
}

func newWorkflowDefinitionsCommand(opts *options) *cobra.Command {
	local := *opts
	var outputFormat string
	var noColor bool
	local.graphFormat = "mermaid"
	cmd := &cobra.Command{
		Use:   "definitions <workflow>",
		Short: "Inspect a local workflow definition",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputFormat != string(workflowOutputText) && outputFormat != string(workflowOutputJSON) {
				return fmt.Errorf("unsupported output format %q", outputFormat)
			}
			if local.graphFormat != "mermaid" {
				return fmt.Errorf("unsupported graph format %q", local.graphFormat)
			}
			detail, err := loadWorkflowDefinitionDetail(cmd.Context(), args[0], &local)
			if err != nil {
				return err
			}
			return renderWorkflowDefinitionDetail(cmd.OutOrStdout(), detail, outputFormat, noColor)
		},
	}
	cmd.Flags().StringVar(&local.project, "project", "", "project name to resolve the workflow within")
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	cmd.Flags().StringVar(&local.graphFormat, "format", "mermaid", "graph output format (mermaid)")
	return cmd
}

func newWorkflowListCommand(opts *options) *cobra.Command {
	local := *opts
	var outputFormat string
	var noColor bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List local workflow definitions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputFormat != string(workflowOutputText) && outputFormat != string(workflowOutputJSON) {
				return fmt.Errorf("unsupported output format %q", outputFormat)
			}
			definitions, err := listWorkflowDefinitions(local.project)
			if err != nil {
				return err
			}
			return renderWorkflowDefinitions(cmd.OutOrStdout(), definitions, outputFormat, noColor, isInteractiveWriter(cmd.OutOrStdout()))
		},
	}
	cmd.Flags().StringVar(&local.project, "project", "", "project name to resolve workflows within")
	cmd.Flags().StringVar(&outputFormat, "output", "text", "output format (text or json)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable color output")
	return cmd
}

func listWorkflowDefinitions(projectName string) ([]workflowDefinitionSummary, error) {
	roots, err := workflowDefinitionRoots(projectName)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var definitions []workflowDefinitionSummary
	for _, root := range roots {
		entries, err := os.ReadDir(root.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read workflow definitions %s: %w", root.path, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !isWorkflowDefinitionFile(entry.Name()) {
				continue
			}
			path := filepath.Join(root.path, entry.Name())
			spec, err := yamlrepo.DecodeWorkflow(path)
			if err != nil {
				continue
			}
			if spec.Name == "" {
				continue
			}
			if _, ok := seen[spec.Name]; ok {
				continue
			}
			seen[spec.Name] = struct{}{}
			definitions = append(definitions, workflowDefinitionSummary{
				Name:        spec.Name,
				Version:     spec.Version,
				Description: spec.Description,
				NodeCount:   len(spec.Nodes),
				Path:        path,
				Source:      root.source,
			})
		}
	}
	sort.SliceStable(definitions, func(i, j int) bool {
		return definitions[i].Name < definitions[j].Name
	})
	return definitions, nil
}

func workflowDefinitionRoots(projectName string) ([]workflowDefinitionRoot, error) {
	projectName = strings.TrimSpace(projectName)
	var roots []workflowDefinitionRoot
	if projectName != "" {
		project, err := newCLIProjectRegistry().Resolve(projectName)
		if err != nil {
			return nil, err
		}
		roots = append(roots, workflowDefinitionRoot{
			path:   filepath.Join(project.Path, ".agentflow", "workflows"),
			source: "project",
		})
	} else {
		localRoot := filepath.Join(".agentflow", "workflows")
		if cwd, err := os.Getwd(); err == nil {
			localRoot = filepath.Join(cwd, localRoot)
		}
		roots = append(roots, workflowDefinitionRoot{path: filepath.Clean(localRoot), source: "local"})
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, workflowDefinitionRoot{path: filepath.Join(home, ".agentflow", "workflows"), source: "global"})
	}
	if _, err := os.Stat("samples/workflows"); err == nil {
		roots = append(roots, workflowDefinitionRoot{path: "samples/workflows", source: "samples"})
	}
	return roots, nil
}

func isWorkflowDefinitionFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func loadWorkflowDefinitionDetail(ctx context.Context, ref string, opts *options) (workflowDefinitionDetail, error) {
	uc, err := buildUseCase(opts)
	if err != nil {
		return workflowDefinitionDetail{}, err
	}
	resolvedRef, err := resolveWorkflowDefinitionRef(ctx, ref, opts.project)
	if err != nil {
		return workflowDefinitionDetail{}, err
	}
	plan, err := uc.Validate(ctx, resolvedRef)
	if err != nil {
		return workflowDefinitionDetail{}, err
	}
	var graph bytes.Buffer
	if err := workflow.WriteMermaidGraph(&graph, plan); err != nil {
		return workflowDefinitionDetail{}, err
	}
	inputs := plan.Workflow.Inputs
	if inputs == nil {
		inputs = map[string]workflow.InputSpec{}
	}
	outputs := plan.Workflow.Outputs
	if outputs == nil {
		outputs = map[string]workflow.OutputSpec{}
	}
	return workflowDefinitionDetail{
		Workflow: workflowDefinitionMetadata{
			Name:        plan.Workflow.Name,
			Version:     plan.Workflow.Version,
			Description: plan.Workflow.Description,
			NodeCount:   len(plan.Workflow.Nodes),
		},
		Path:    filepath.Clean(resolvedRef),
		Inputs:  inputs,
		Outputs: outputs,
		Graph:   graph.String(),
		Order:   plan.Order,
		Nodes:   plan.Nodes,
	}, nil
}

func resolveWorkflowDefinitionRef(ctx context.Context, ref string, projectName string) (string, error) {
	resolvedRef, err := resolveWorkflowRefForCLI(ref, projectName)
	if err == nil && isWorkflowPath(resolvedRef) {
		return resolvedRef, nil
	}
	if err != nil && strings.TrimSpace(projectName) != "" {
		return "", err
	}
	if err == nil {
		repo := yamlrepo.NewWorkflowRepository()
		_, sourcePath, loadErr := repo.Load(ctx, resolvedRef)
		if loadErr == nil {
			return sourcePath, nil
		}
	}
	definitions, listErr := listWorkflowDefinitions(projectName)
	if listErr != nil {
		if err != nil {
			return "", err
		}
		return "", listErr
	}
	for _, definition := range definitions {
		if definition.Name == ref {
			return definition.Path, nil
		}
	}
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("workflow %q not found", ref)
}

func renderWorkflowDefinitions(w io.Writer, definitions []workflowDefinitionSummary, format string, noColor bool, interactive bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(definitions)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	if len(definitions) == 0 {
		_, err := fmt.Fprintln(w, newCLIFormat(noColor || !interactive).note("No workflow definitions"))
		return err
	}
	f := newCLIFormat(!(noColor || !interactive))
	cols := []string{"NAME", "VERSION", "NODES", "SOURCE", "PATH", "DESCRIPTION"}
	widths := []int{18, 7, 5, 8, 24, 28}
	minWidths := []int{8, 7, 5, 6, 12, 12}
	if maxWidth := terminalWidth(w); interactive && maxWidth > 0 {
		if maxWidth > 110 {
			maxWidth = 110
		}
		widths = fitWidths(widths, maxWidth-(len(widths)+3), 2, minWidths)
	}
	rows := make([][]string, 0, len(definitions))
	for _, definition := range definitions {
		rows = append(rows, []string{
			definition.Name,
			definition.Version,
			fmt.Sprint(definition.NodeCount),
			definition.Source,
			definition.Path,
			firstNonEmpty(definition.Description, "-"),
		})
	}
	_, err := fmt.Fprint(w, f.table(cols, rows, widths))
	return err
}

func renderWorkflowDefinitionDetail(w io.Writer, detail workflowDefinitionDetail, format string, noColor bool) error {
	if workflowOutputFormat(format) == workflowOutputJSON {
		data, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(data))
		return err
	}
	f := newCLIFormat(!(noColor || !isInteractiveWriter(w)))
	lines := []string{
		f.labelValue("name", detail.Workflow.Name),
		f.labelValue("version", detail.Workflow.Version),
		f.labelValue("path", detail.Path),
		f.labelValue("nodes", fmt.Sprint(detail.Workflow.NodeCount)),
	}
	if detail.Workflow.Description != "" {
		lines = append(lines, f.labelValue("description", detail.Workflow.Description))
	}
	lines = append(lines, f.section("Inputs"))
	lines = append(lines, renderInputDefinitions(f, detail.Inputs)...)
	lines = append(lines, f.section("Outputs"))
	lines = append(lines, renderOutputDefinitions(f, detail.Outputs)...)
	lines = append(lines, f.section("Graph"))
	lines = append(lines, strings.TrimRight(detail.Graph, "\n"))
	_, err := fmt.Fprintln(w, f.block(f.title("Workflow definition"), lines))
	return err
}

func renderInputDefinitions(f cliFormat, inputs map[string]workflow.InputSpec) []string {
	if len(inputs) == 0 {
		return []string{"-"}
	}
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		spec := inputs[name]
		parts := []string{"type=" + firstNonEmpty(spec.Type, "-")}
		if spec.Required {
			parts = append(parts, "required=true")
		}
		if spec.Default != nil {
			parts = append(parts, "default="+formatDefinitionValue(spec.Default))
		}
		if len(spec.Schema) > 0 {
			parts = append(parts, "schema="+formatDefinitionValue(spec.Schema))
		}
		lines = append(lines, f.labelValue(name, strings.Join(parts, " ")))
	}
	return lines
}

func renderOutputDefinitions(f cliFormat, outputs map[string]workflow.OutputSpec) []string {
	if len(outputs) == 0 {
		return []string{"-"}
	}
	names := make([]string, 0, len(outputs))
	for name := range outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		spec := outputs[name]
		var parts []string
		if spec.Type != "" {
			parts = append(parts, "type="+spec.Type)
		}
		if spec.Value != nil {
			parts = append(parts, "value="+formatDefinitionValue(spec.Value))
		}
		if len(spec.Schema) > 0 {
			parts = append(parts, "schema="+formatDefinitionValue(spec.Schema))
		}
		lines = append(lines, f.labelValue(name, firstNonEmpty(strings.Join(parts, " "), "-")))
	}
	return lines
}

func formatDefinitionValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}
