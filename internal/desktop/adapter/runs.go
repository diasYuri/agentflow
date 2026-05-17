package adapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/app"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/desktop/runtime"
)

// RunSummary resume uma execucao de workflow.
type RunSummary struct {
	ID                string                `json:"id"`
	Workflow          string                `json:"workflow"`
	Status            corerun.RunStatus     `json:"status"`
	StartedAt         time.Time             `json:"started_at"`
	FinishedAt        time.Time             `json:"finished_at,omitempty"`
	Error             string                `json:"error,omitempty"`
	CurrentStep       string                `json:"current_step,omitempty"`
	CompletedSteps    []string              `json:"completed_steps,omitempty"`
	PendingSteps      []string              `json:"pending_steps,omitempty"`
	TotalSteps        int                   `json:"total_steps,omitempty"`
	Tag               string                `json:"tag,omitempty"`
	DiagnosticSummary *RunDiagnosticSummary `json:"diagnostic_summary,omitempty"`
}

// RunWorkflowRequest inicia uma execucao de workflow.
type RunWorkflowRequest struct {
	WorkflowRef    string         `json:"workflow_ref"`
	Project        string         `json:"project,omitempty"`
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
	summary := RunSummary{
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
	if s.DurationMS > 0 || s.FailedNodes > 0 || s.AgentCalls > 0 || s.BashCalls > 0 || len(s.SlowestNodes) > 0 || len(s.AgentUsage) > 0 {
		diag := &RunDiagnosticSummary{
			DurationMS:    s.DurationMS,
			FailedNodes:   s.FailedNodes,
			Retries:       s.Retries,
			AgentCalls:    s.AgentCalls,
			BashCalls:     s.BashCalls,
			ArtifactCount: s.ArtifactCount,
			NodeCount:     s.NodeCount,
			FirstError:    s.FirstError,
		}
		if len(s.SlowestNodes) > 0 {
			diag.SlowestNodes = make([]SlowestNode, len(s.SlowestNodes))
			for i, n := range s.SlowestNodes {
				diag.SlowestNodes[i] = SlowestNode{NodeID: n.NodeID, DurationMS: n.DurationMS}
			}
		}
		if len(s.AgentUsage) > 0 {
			diag.AgentUsage = make([]AgentUsage, len(s.AgentUsage))
			for i, u := range s.AgentUsage {
				diag.AgentUsage[i] = AgentUsage{
					Provider:     u.Provider,
					Model:        u.Model,
					InputTokens:  u.InputTokens,
					OutputTokens: u.OutputTokens,
					TotalTokens:  u.TotalTokens,
					CostUSD:      u.CostUSD,
				}
			}
		}
		summary.DiagnosticSummary = diag
	}
	return summary
}

// RunWorkflow inicia uma execucao de workflow.
func (a *Adapter) RunWorkflow(ctx context.Context, req RunWorkflowRequest) (RunSummary, error) {
	if a.runtime == nil {
		return RunSummary{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	resolvedReq, err := a.resolveProjectRunRequest(req)
	if err != nil {
		return RunSummary{}, normalizeError(err)
	}
	s, err := a.runtime.RunWorkflow(ctx, runtime.RunRequest{
		WorkflowRef:    resolvedReq.WorkflowRef,
		Inputs:         resolvedReq.Inputs,
		Vars:           resolvedReq.Vars,
		MaxConcurrency: resolvedReq.MaxConcurrency,
		WorkingDir:     resolvedReq.WorkingDir,
		Tag:            resolvedReq.Tag,
	})
	if err != nil {
		return RunSummary{}, normalizeError(err)
	}
	return toRunSummary(s), nil
}

func (a *Adapter) resolveProjectRunRequest(req RunWorkflowRequest) (RunWorkflowRequest, error) {
	if a.projects == nil {
		req.WorkingDir = normalizeWorkingDir(req.WorkingDir)
		return req, nil
	}

	projectName := strings.TrimSpace(req.Project)
	if projectName == "" {
		req.WorkingDir = normalizeWorkingDir(req.WorkingDir)
		return req, nil
	}

	project, err := a.projects.Resolve(projectName)
	if err != nil {
		return req, err
	}

	if req.WorkflowRef != "" && !isWorkflowPath(req.WorkflowRef) {
		resolved, err := app.ResolveWorkflowRef(app.Project{Name: project.Name, Path: project.Path}, req.WorkflowRef)
		if err != nil {
			return req, err
		}
		req.WorkflowRef = resolved
	}

	if strings.TrimSpace(req.WorkingDir) == "" {
		req.WorkingDir = project.Path
	} else {
		req.WorkingDir = normalizeWorkingDir(req.WorkingDir)
	}
	return req, nil
}

func normalizeWorkingDir(workingDir string) string {
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return ""
	}
	if filepath.IsAbs(workingDir) {
		return filepath.Clean(workingDir)
	}
	abs, err := filepath.Abs(workingDir)
	if err != nil {
		return filepath.Clean(workingDir)
	}
	return abs
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

// GetRunDiagnostics returns aggregated diagnostic metrics for a run.
func (a *Adapter) GetRunDiagnostics(runID string) (RunDiagnosticSummary, error) {
	if a.runtime == nil {
		return RunDiagnosticSummary{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	s, err := a.runtime.GetRun(runID)
	if err != nil {
		return RunDiagnosticSummary{}, normalizeError(err)
	}
	summary := toRunSummary(s)
	if summary.DiagnosticSummary != nil {
		return *summary.DiagnosticSummary, nil
	}
	return RunDiagnosticSummary{}, nil
}

// GetRunTimeline returns timeline entries for a run by reading events.jsonl.
func (a *Adapter) GetRunTimeline(runID string, cursor int, limit int) (RunTimeline, error) {
	if a.runtime == nil {
		return RunTimeline{}, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return RunTimeline{}, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}
	resp, err := runtime.GetRunEvents(runDir, cursor, limit)
	if err != nil {
		return RunTimeline{}, normalizeError(err)
	}
	entries := make([]TimelineEntry, len(resp.Events))
	for i, e := range resp.Events {
		entries[i] = TimelineEntry{
			Timestamp:  e.Timestamp,
			Type:       e.Type,
			NodeID:     e.NodeID,
			InstanceID: e.InstanceID,
			Attempt:    e.Attempt,
		}
	}
	return RunTimeline{
		Entries:    entries,
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
	}, nil
}

// GetRunChartSeries builds chart-ready series from node results.
func (a *Adapter) GetRunChartSeries(runID string) ([]ChartSeries, error) {
	if a.runtime == nil {
		return nil, DesktopError{Message: "runtime not initialized", Code: ErrCodeInternalError}
	}
	runDir, ok := a.runtime.RunDir(runID)
	if !ok {
		return nil, DesktopError{Message: "run not found", Code: ErrCodeWorkflowNotFound}
	}
	nodes, err := a.GetRunNodes(runID)
	if err != nil {
		return nil, normalizeError(err)
	}
	if len(nodes.Nodes) == 0 {
		_ = runDir
		return nil, nil
	}
	var durLabels []string
	var durValues []float64
	var retryLabels []string
	var retryValues []float64
	for _, n := range nodes.Nodes {
		durLabels = append(durLabels, n.NodeID)
		durValues = append(durValues, float64(n.Duration))
		retryLabels = append(retryLabels, n.NodeID)
		retryValues = append(retryValues, float64(n.Attempts))
	}
	series := []ChartSeries{
		{Name: "Duration (ms)", Labels: durLabels, Values: durValues},
		{Name: "Retries", Labels: retryLabels, Values: retryValues},
	}

	diagnostics, err := a.GetRunDiagnostics(runID)
	if err == nil && len(diagnostics.AgentUsage) > 0 {
		var tokenLabels []string
		var tokenValues []float64
		for _, u := range diagnostics.AgentUsage {
			label := u.Provider
			if u.Model != "" {
				label += " " + u.Model
			}
			tokenLabels = append(tokenLabels, label)
			tokenValues = append(tokenValues, float64(u.TotalTokens))
		}
		series = append(series, ChartSeries{Name: "Tokens", Labels: tokenLabels, Values: tokenValues})
	}
	return series, nil
}
