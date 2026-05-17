package client

import (
	"context"
	"fmt"
	"time"

	"github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/daemon"
)

// HTTPDaemonClient implements the Client interfaces over the daemon HTTP API.
type HTTPDaemonClient struct {
	inner *daemon.Client
}

// NewDaemonClient creates a new TUI daemon client.
func NewDaemonClient(socketPath string) *HTTPDaemonClient {
	return &HTTPDaemonClient{
		inner: daemon.NewClient(socketPath),
	}
}

// DaemonStatus returns the current daemon state.
func (c *HTTPDaemonClient) DaemonStatus(ctx context.Context) (DaemonState, error) {
	status, err := c.inner.Status(ctx)
	if err != nil {
		if daemon.IsDaemonUnavailable(err) {
			return DaemonState{Status: DaemonUnavailable}, ErrDaemonUnavailable
		}
		return DaemonState{Status: DaemonUnknown}, &DaemonError{Status: DaemonUnknown, Err: err}
	}
	return DaemonState{
		Status:    DaemonAvailable,
		Running:   status.Running,
		PID:       status.PID,
		Socket:    status.Socket,
		Runs:      status.Runs,
		StartedAt: status.StartedAt,
		LastCheck: time.Now(),
	}, nil
}

// ListRuns lists all workflow runs.
func (c *HTTPDaemonClient) ListRuns(ctx context.Context) ([]RunSummary, error) {
	resp, err := c.inner.ListWorkflows(ctx)
	if err != nil {
		return nil, mapDaemonError(err)
	}
	out := make([]RunSummary, len(resp.Runs))
	for i, r := range resp.Runs {
		out[i] = runSummaryFromDaemon(r)
	}
	return out, nil
}

// GetRun fetches a single run.
func (c *HTTPDaemonClient) GetRun(ctx context.Context, runID string) (RunSummary, error) {
	resp, err := c.inner.WorkflowStatus(ctx, runID)
	if err != nil {
		return RunSummary{}, mapDaemonError(err)
	}
	return runSummaryFromDaemon(resp.Run), nil
}

// GetRunLogs fetches logs for a run.
func (c *HTTPDaemonClient) GetRunLogs(ctx context.Context, runID string) ([]string, error) {
	resp, err := c.inner.WorkflowLogs(ctx, runID)
	if err != nil {
		return nil, mapDaemonError(err)
	}
	return resp.Lines, nil
}

// GetRunEvents fetches events for a run with cursor-based pagination.
func (c *HTTPDaemonClient) GetRunEvents(ctx context.Context, runID string, cursor int, limit int) (EventBatch, error) {
	if limit <= 0 {
		limit = 100
	}
	resp, err := c.inner.WorkflowEvents(ctx, runID, cursor, limit)
	if err != nil {
		return EventBatch{}, mapDaemonError(err)
	}
	events := make([]EventLine, len(resp.Events))
	for i, e := range resp.Events {
		msg := ""
		if e.Data != nil {
			if t, ok := e.Data["message"].(string); ok {
				msg = t
			}
		}
		events[i] = EventLine{
			Cursor:    e.Cursor,
			Timestamp: e.Timestamp,
			RunID:     e.RunID,
			Type:      e.Type,
			NodeID:    e.NodeID,
			Message:   msg,
		}
	}
	return EventBatch{
		Events:     events,
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
	}, nil
}

// GetRunNodes fetches node results for a run.
func (c *HTTPDaemonClient) GetRunNodes(ctx context.Context, runID string) ([]NodeSummary, error) {
	resp, err := c.inner.WorkflowNodes(ctx, runID)
	if err != nil {
		return nil, mapDaemonError(err)
	}
	out := make([]NodeSummary, len(resp.Nodes))
	for i, n := range resp.Nodes {
		out[i] = NodeSummary{
			NodeID:     n.NodeID,
			InstanceID: n.InstanceID,
			Status:     n.Status,
			Output:     n.Output,
			Outputs:    n.Outputs,
			Stdout:     n.Stdout,
			Stderr:     n.Stderr,
			Error:      n.Error,
			ExitCode:   n.ExitCode,
			Duration:   n.Duration,
			Attempts:   n.Attempts,
		}
	}
	return out, nil
}

// GetRunPlan fetches the execution plan for a run.
func (c *HTTPDaemonClient) GetRunPlan(ctx context.Context, runID string) (PlanSummary, error) {
	resp, err := c.inner.WorkflowPlan(ctx, runID)
	if err != nil {
		return PlanSummary{}, mapDaemonError(err)
	}
	return PlanSummary{
		WorkflowName: resp.Workflow,
		Order:        planOrder(resp.Plan),
		Nodes:        resp.Plan,
	}, nil
}

// ListArtifacts lists artifacts for a run.
func (c *HTTPDaemonClient) ListArtifacts(ctx context.Context, runID string) ([]ArtifactSummary, error) {
	resp, err := c.inner.WorkflowArtifacts(ctx, runID)
	if err != nil {
		return nil, mapDaemonError(err)
	}
	out := make([]ArtifactSummary, len(resp.Artifacts))
	for i, a := range resp.Artifacts {
		out[i] = ArtifactSummary{
			ID:          a.ID,
			Name:        a.Name,
			Path:        a.Path,
			Size:        a.Size,
			ContentType: a.ContentType,
			ModifiedAt:  a.ModifiedAt,
		}
	}
	return out, nil
}

// GetArtifact fetches a single artifact's content.
func (c *HTTPDaemonClient) GetArtifact(ctx context.Context, runID, artifactID string) (ArtifactSummary, error) {
	resp, err := c.inner.WorkflowArtifact(ctx, runID, artifactID)
	if err != nil {
		return ArtifactSummary{}, mapDaemonError(err)
	}
	return ArtifactSummary{
		ID:          resp.ID,
		Name:        resp.Name,
		Path:        resp.Path,
		Size:        resp.Size,
		ContentType: resp.ContentType,
		Encoding:    resp.Encoding,
		Content:     resp.Content,
	}, nil
}

// GetRunDiagnostics fetches aggregated diagnostic metrics for a run.
func (c *HTTPDaemonClient) GetRunDiagnostics(ctx context.Context, runID string) (RunDiagnosticSummary, error) {
	resp, err := c.inner.WorkflowInspect(ctx, runID)
	if err != nil {
		return RunDiagnosticSummary{}, mapDaemonError(err)
	}
	return RunDiagnosticSummary{
		DurationMS:    resp.DurationMS,
		FailedNodes:   resp.FailedNodes,
		Retries:       resp.Retries,
		AgentCalls:    resp.AgentCalls,
		BashCalls:     resp.BashCalls,
		ArtifactCount: resp.ArtifactCount,
		NodeCount:     resp.NodeCount,
		FirstError:    firstNonEmpty(resp.FirstError, resp.Error, resp.TerminalError),
		SlowestNodes:  slowestNodesFromDaemon(resp.SlowestNodes),
		AgentUsage:    agentUsageFromDaemon(resp.AgentUsage),
	}, nil
}

// GetRunTimeline fetches timeline entries for a run.
func (c *HTTPDaemonClient) GetRunTimeline(ctx context.Context, runID string, cursor int, limit int) (RunTimeline, error) {
	if limit <= 0 {
		limit = 100
	}
	resp, err := c.inner.WorkflowTimeline(ctx, runID, cursor, limit)
	if err != nil {
		return RunTimeline{}, mapDaemonError(err)
	}
	entries := make([]TimelineEntry, len(resp.Entries))
	for i, e := range resp.Entries {
		entries[i] = TimelineEntry{
			Timestamp:  e.Timestamp,
			Type:       e.Type,
			NodeID:     e.NodeID,
			InstanceID: e.InstanceID,
			Attempt:    e.Attempt,
			DurationMS: e.DurationMS,
		}
	}
	return RunTimeline{
		Entries:    entries,
		NextCursor: resp.NextCursor,
		HasMore:    resp.HasMore,
	}, nil
}

// GetRunChartSeries builds chart-ready series from node results.
func (c *HTTPDaemonClient) GetRunChartSeries(ctx context.Context, runID string) ([]ChartSeries, error) {
	nodes, err := c.GetRunNodes(ctx, runID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	var durLabels []string
	var durValues []float64
	var retryLabels []string
	var retryValues []float64
	for _, n := range nodes {
		durLabels = append(durLabels, n.NodeID)
		durValues = append(durValues, float64(n.Duration))
		retryLabels = append(retryLabels, n.NodeID)
		retryValues = append(retryValues, float64(n.Attempts))
	}

	series := []ChartSeries{
		{Name: "Duration (ms)", Labels: durLabels, Values: durValues},
		{Name: "Retries", Labels: retryLabels, Values: retryValues},
	}

	diagnostics, err := c.GetRunDiagnostics(ctx, runID)
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

// CancelRun cancels a run.
func (c *HTTPDaemonClient) CancelRun(ctx context.Context, runID string) error {
	_, err := c.inner.CancelWorkflow(ctx, runID)
	return mapDaemonError(err)
}

// PauseRun pauses a run.
func (c *HTTPDaemonClient) PauseRun(ctx context.Context, runID string) error {
	_, err := c.inner.PauseWorkflow(ctx, runID)
	return mapDaemonError(err)
}

// ResumeRun resumes a run.
func (c *HTTPDaemonClient) ResumeRun(ctx context.Context, runID string) error {
	_, err := c.inner.ResumeWorkflow(ctx, runID)
	return mapDaemonError(err)
}

// ListLocalWorkflows is not supported by the daemon client.
func (c *HTTPDaemonClient) ListLocalWorkflows(ctx context.Context) ([]LocalWorkflow, error) {
	return nil, fmt.Errorf("daemon client does not support local workflow listing: %w", ErrDaemonUnavailable)
}

// ValidateWorkflow is not supported by the daemon client.
func (c *HTTPDaemonClient) ValidateWorkflow(ctx context.Context, ref string) error {
	return fmt.Errorf("daemon client does not support local validation: %w", ErrDaemonUnavailable)
}

// GraphWorkflow is not supported by the daemon client.
func (c *HTTPDaemonClient) GraphWorkflow(ctx context.Context, ref string) (string, error) {
	return "", fmt.Errorf("daemon client does not support local graph: %w", ErrDaemonUnavailable)
}

// DryRunWorkflow is not supported by the daemon client.
func (c *HTTPDaemonClient) DryRunWorkflow(ctx context.Context, ref string, inputs, vars map[string]any) (string, error) {
	return "", fmt.Errorf("daemon client does not support local dry-run: %w", ErrDaemonUnavailable)
}

func runSummaryFromDaemon(r daemon.WorkflowRun) RunSummary {
	return RunSummary{
		ID:             r.ID,
		Workflow:       r.Workflow,
		RunDir:         r.RunDir,
		Status:         string(r.Status),
		StartedAt:      r.StartedAt,
		FinishedAt:     r.FinishedAt,
		PausedAt:       r.PausedAt,
		PauseReason:    r.PauseReason,
		ResumeCount:    r.ResumeCount,
		CurrentStep:    r.CurrentStep,
		CompletedSteps: r.CompletedSteps,
		PendingSteps:   r.PendingSteps,
		TotalSteps:     r.TotalSteps,
		Error:          r.Error,
		TerminalError:  r.TerminalError,
		FailureReason:  r.FailureReason,
		Tag:            r.Tag,
	}
}

func slowestNodesFromDaemon(in []run.SlowestNode) []SlowestNode {
	if len(in) == 0 {
		return nil
	}
	out := make([]SlowestNode, len(in))
	for i, n := range in {
		out[i] = SlowestNode{NodeID: n.NodeID, DurationMS: n.DurationMS}
	}
	return out
}

func agentUsageFromDaemon(in []run.AgentUsage) []AgentUsage {
	if len(in) == 0 {
		return nil
	}
	out := make([]AgentUsage, len(in))
	for i, u := range in {
		out[i] = AgentUsage{
			Provider:     u.Provider,
			Model:        u.Model,
			InputTokens:  u.InputTokens,
			OutputTokens: u.OutputTokens,
			TotalTokens:  u.TotalTokens,
			CostUSD:      u.CostUSD,
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func mapDaemonError(err error) error {
	if err == nil {
		return nil
	}
	if daemon.IsDaemonUnavailable(err) {
		return ErrDaemonUnavailable
	}
	return &DaemonError{Status: DaemonUnavailable, Err: err}
}

func planOrder(plan map[string]any) []string {
	if plan == nil {
		return nil
	}
	orderRaw, ok := plan["order"]
	if !ok {
		return nil
	}
	order, ok := orderRaw.([]string)
	if ok {
		return order
	}
	if orderAny, ok := orderRaw.([]any); ok {
		out := make([]string, len(orderAny))
		for i, v := range orderAny {
			if s, ok := v.(string); ok {
				out[i] = s
			}
		}
		return out
	}
	return nil
}
