package adapter

import (
	"context"
	"time"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/desktop/runtime"
)

// RunSummary resume uma execucao de workflow.
type RunSummary struct {
	ID             string            `json:"id"`
	Workflow       string            `json:"workflow"`
	Status         corerun.RunStatus `json:"status"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at,omitempty"`
	Error          string            `json:"error,omitempty"`
	CurrentStep    string            `json:"current_step,omitempty"`
	CompletedSteps []string          `json:"completed_steps,omitempty"`
	PendingSteps   []string          `json:"pending_steps,omitempty"`
	TotalSteps     int               `json:"total_steps,omitempty"`
	Tag            string            `json:"tag,omitempty"`
}

// RunWorkflowRequest inicia uma execucao de workflow.
type RunWorkflowRequest struct {
	WorkflowRef    string         `json:"workflow_ref"`
	Inputs         map[string]any `json:"inputs,omitempty"`
	Vars           map[string]any `json:"vars,omitempty"`
	MaxConcurrency int            `json:"max_concurrency,omitempty"`
	WorkingDir     string         `json:"working_dir,omitempty"`
	Tag            string         `json:"tag,omitempty"`
}

// ListRunsResponse lista execucoes.
type ListRunsResponse struct {
	Runs []RunSummary `json:"runs"`
}

func toRunSummary(s runtime.RunSummary) RunSummary {
	return RunSummary{
		ID:             s.ID,
		Workflow:       s.Workflow,
		Status:         s.Status,
		StartedAt:      s.StartedAt,
		FinishedAt:     s.FinishedAt,
		Error:          s.Error,
		CurrentStep:    s.CurrentStep,
		CompletedSteps: s.CompletedSteps,
		PendingSteps:   s.PendingSteps,
		TotalSteps:     s.TotalSteps,
		Tag:            s.Tag,
	}
}

// RunWorkflow inicia uma execucao de workflow.
func (a *Adapter) RunWorkflow(ctx context.Context, req RunWorkflowRequest) (RunSummary, error) {
	if a.runtime == nil {
		return RunSummary{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	s, err := a.runtime.RunWorkflow(ctx, runtime.RunRequest{
		WorkflowRef:    req.WorkflowRef,
		Inputs:         req.Inputs,
		Vars:           req.Vars,
		MaxConcurrency: req.MaxConcurrency,
		WorkingDir:     req.WorkingDir,
		Tag:            req.Tag,
	})
	if err != nil {
		return RunSummary{}, normalizeError(err)
	}
	return toRunSummary(s), nil
}

// CancelRun cancela uma execucao ativa.
func (a *Adapter) CancelRun(runID string) (RunSummary, error) {
	if a.runtime == nil {
		return RunSummary{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	s, err := a.runtime.CancelRun(runID)
	if err != nil {
		return RunSummary{}, normalizeError(err)
	}
	return toRunSummary(s), nil
}

// ListRuns lista execucoes ativas e persistidas.
func (a *Adapter) ListRuns() (ListRunsResponse, error) {
	if a.runtime == nil {
		return ListRunsResponse{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runs, err := a.runtime.ListRuns()
	if err != nil {
		return ListRunsResponse{}, normalizeError(err)
	}
	resp := ListRunsResponse{
		Runs: make([]RunSummary, len(runs)),
	}
	for i, r := range runs {
		resp.Runs[i] = toRunSummary(r)
	}
	return resp, nil
}

// GetRun retorna uma execucao especifica.
func (a *Adapter) GetRun(runID string) (RunSummary, error) {
	if a.runtime == nil {
		return RunSummary{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	s, err := a.runtime.GetRun(runID)
	if err != nil {
		return RunSummary{}, normalizeError(err)
	}
	return toRunSummary(s), nil
}
