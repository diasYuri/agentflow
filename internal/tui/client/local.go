package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/diasYuri/agentflow/internal/app"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

// LocalClient performs workflow operations locally without the daemon.
type LocalClient struct {
	ucFactory func() (*runworkflow.RunWorkflowUseCase, error)
}

// NewLocalClient creates a new local client.
func NewLocalClient() *LocalClient {
	return &LocalClient{
		ucFactory: func() (*runworkflow.RunWorkflowUseCase, error) {
			return app.NewRunWorkflowUseCase(app.RuntimeOptions{})
		},
	}
}

// ListLocalWorkflows discovers workflows in local directories.
func (c *LocalClient) ListLocalWorkflows(ctx context.Context) ([]LocalWorkflow, error) {
	roots := workflowRoots()
	var out []LocalWorkflow
	seen := make(map[string]struct{})
	for _, root := range roots {
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, &LocalError{Op: "list workflows", Err: fmt.Errorf("read %s: %w", root, err)}
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".yaml" && ext != ".yml" {
				continue
			}
			path := filepath.Join(root, entry.Name())
			spec, err := decodeWorkflowForList(path)
			if err != nil {
				continue
			}
			if _, ok := seen[spec.Name]; ok {
				continue
			}
			seen[spec.Name] = struct{}{}
			out = append(out, LocalWorkflow{
				Name:        spec.Name,
				Path:        path,
				Description: spec.Description,
				NodeCount:   len(spec.Nodes),
				Inputs:      convertInputs(spec.Inputs),
			})
		}
	}
	return out, nil
}

// ValidateWorkflow validates a workflow locally.
func (c *LocalClient) ValidateWorkflow(ctx context.Context, ref string) error {
	uc, err := c.ucFactory()
	if err != nil {
		return &LocalError{Op: "validate workflow", Err: err}
	}
	_, err = uc.Validate(ctx, ref)
	if err != nil {
		return &LocalError{Op: "validate workflow", Err: err}
	}
	return nil
}

// GraphWorkflow generates a Mermaid graph for a workflow.
func (c *LocalClient) GraphWorkflow(ctx context.Context, ref string) (string, error) {
	uc, err := c.ucFactory()
	if err != nil {
		return "", &LocalError{Op: "graph workflow", Err: err}
	}
	plan, err := uc.Validate(ctx, ref)
	if err != nil {
		return "", &LocalError{Op: "graph workflow", Err: err}
	}
	var buf bytes.Buffer
	if err := workflow.WriteMermaidGraph(&buf, plan); err != nil {
		return "", &LocalError{Op: "graph workflow", Err: err}
	}
	return buf.String(), nil
}

// DryRunWorkflow performs a dry-run locally.
func (c *LocalClient) DryRunWorkflow(ctx context.Context, ref string, inputs, vars map[string]any) (string, error) {
	uc, err := c.ucFactory()
	if err != nil {
		return "", &LocalError{Op: "dry-run workflow", Err: err}
	}
	plan, resolved, err := uc.DryRun(ctx, runworkflow.RunOptions{
		WorkflowRef: ref,
		Inputs:      inputs,
		Vars:        vars,
	})
	if err != nil {
		return "", &LocalError{Op: "dry-run workflow", Err: err}
	}
	out := map[string]any{
		"workflow": plan.Workflow.Name,
		"inputs":   resolved,
		"order":    plan.Order,
		"nodes":    plan.Nodes,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", &LocalError{Op: "dry-run workflow", Err: err}
	}
	return string(data), nil
}

func workflowRoots() []string {
	localRoot := filepath.Join(".agentflow", "workflows")
	if cwd, err := os.Getwd(); err == nil {
		localRoot = filepath.Join(cwd, localRoot)
	}
	globalRoot := filepath.Join(".agentflow", "workflows")
	if home, err := os.UserHomeDir(); err == nil {
		globalRoot = filepath.Join(home, ".agentflow", "workflows")
	}
	roots := []string{filepath.Clean(localRoot), filepath.Clean(globalRoot)}
	if _, err := os.Stat("samples/workflows"); err == nil {
		roots = append(roots, "samples/workflows")
	}
	return roots
}

func decodeWorkflowForList(path string) (*workflow.WorkflowSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var spec workflow.WorkflowSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

func convertInputs(in map[string]workflow.InputSpec) map[string]InputSpec {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]InputSpec, len(in))
	for k, v := range in {
		out[k] = InputSpec{Type: v.Type, Required: v.Required, Default: v.Default}
	}
	return out
}
