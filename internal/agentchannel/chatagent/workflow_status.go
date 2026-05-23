package chatagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/diasYuri/agentflow/internal/daemon"
)

type workflowStatusInput struct {
	RunID string `json:"run_id"`
}

type workflowStatusOutput struct {
	Run daemon.WorkflowRun `json:"run"`
}

func newWorkflowStatusTool(env *ToolEnvironment) Tool {
	invoke := func(ctx context.Context, raw json.RawMessage) (any, error) {
		if env.Runs == nil {
			return nil, errors.New("workflow run client is not configured")
		}
		var in workflowStatusInput
		if err := decodeToolInput(raw, &in); err != nil {
			return nil, err
		}
		runID := strings.TrimSpace(in.RunID)
		if runID == "" {
			return nil, errors.New("run_id is required")
		}
		resp, err := env.Runs.WorkflowStatus(ctx, runID)
		if err != nil {
			return nil, fmt.Errorf("workflow status %s: %w", runID, err)
		}
		return workflowStatusOutput{Run: resp.Run}, nil
	}
	return Tool{
		Name:        "agentflow.workflow_status",
		Description: "Get the current status of a workflow run, including its state, step progress, tag, and run directory.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"run_id": map[string]any{"type": "string"},
			},
			"required":             []string{"run_id"},
			"additionalProperties": false,
		},
		Invoke: invoke,
	}
}
