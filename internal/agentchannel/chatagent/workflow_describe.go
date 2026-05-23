package chatagent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
)

func describeLocalWorkflowDefinition(ctx context.Context, env *ToolEnvironment, ref string) (describeWorkflowOutput, bool, error) {
	if env == nil {
		return describeWorkflowOutput{}, false, nil
	}
	projectPath := strings.TrimSpace(env.ProjectPath)
	if projectPath == "" {
		return describeWorkflowOutput{}, false, nil
	}

	roots := []string{
		filepath.Join(projectPath, ".agentflow", "workflows"),
		filepath.Join(projectPath, "samples", "workflows"),
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots, filepath.Join(home, ".agentflow", "workflows"))
	}

	repo := yamlrepo.NewWorkflowRepository(roots...)
	spec, sourcePath, err := repo.Load(ctx, ref)
	if err != nil {
		if isMissingWorkflowDefinitionError(err) {
			return describeWorkflowOutput{}, false, nil
		}
		return describeWorkflowOutput{}, false, fmt.Errorf("describe workflow %s: %w", ref, err)
	}

	plan, err := workflow.BuildPlan(*spec)
	if err != nil {
		return describeWorkflowOutput{}, false, fmt.Errorf("describe workflow %s: %w", ref, err)
	}

	var graph bytes.Buffer
	if err := workflow.WriteMermaidGraph(&graph, plan); err != nil {
		return describeWorkflowOutput{}, false, fmt.Errorf("describe workflow %s: %w", ref, err)
	}

	return describeWorkflowOutput{
		Definition: daemon.WorkflowDefinition{
			ID:          sourcePath,
			Name:        spec.Name,
			Version:     spec.Version,
			Description: spec.Description,
			Inputs:      spec.Inputs,
			Outputs:     spec.Outputs,
			Graph:       graph.String(),
			Order:       plan.Order,
			Spec:        *spec,
		},
		RequiredInputs: requiredWorkflowInputs(spec.Inputs),
	}, true, nil
}

func loadWorkflowDefinitionForTool(ctx context.Context, env *ToolEnvironment, ref string) (daemon.WorkflowDefinition, bool, error) {
	if detail, ok, err := describeLocalWorkflowDefinition(ctx, env, ref); err != nil {
		return daemon.WorkflowDefinition{}, false, err
	} else if ok {
		return detail.Definition, true, nil
	}

	if env == nil || env.Definitions == nil {
		return daemon.WorkflowDefinition{}, false, nil
	}

	resp, err := env.Definitions.WorkflowDefinition(ctx, ref)
	if err != nil {
		return daemon.WorkflowDefinition{}, false, fmt.Errorf("load workflow definition %s: %w", ref, err)
	}
	return resp.WorkflowDefinition, true, nil
}

func requiredWorkflowInputs(inputs map[string]workflow.InputSpec) []string {
	if len(inputs) == 0 {
		return nil
	}
	required := make([]string, 0, len(inputs))
	for name, input := range inputs {
		if input.Required {
			required = append(required, name)
		}
	}
	sort.Strings(required)
	return required
}

func isMissingWorkflowDefinitionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "does not exist") || os.IsNotExist(err)
}
