package client

import "context"

// FakeClient is a test double that implements Client.
type FakeClient struct {
	DaemonStateFunc        func(ctx context.Context) (DaemonState, error)
	ListRunsFunc           func(ctx context.Context) ([]RunSummary, error)
	GetRunFunc             func(ctx context.Context, runID string) (RunSummary, error)
	GetRunLogsFunc         func(ctx context.Context, runID string) ([]string, error)
	GetRunEventsFunc       func(ctx context.Context, runID string, cursor int, limit int) (EventBatch, error)
	GetRunNodesFunc        func(ctx context.Context, runID string) ([]NodeSummary, error)
	GetRunPlanFunc         func(ctx context.Context, runID string) (PlanSummary, error)
	ListArtifactsFunc      func(ctx context.Context, runID string) ([]ArtifactSummary, error)
	GetArtifactFunc        func(ctx context.Context, runID, artifactID string) (ArtifactSummary, error)
	CancelRunFunc          func(ctx context.Context, runID string) error
	PauseRunFunc           func(ctx context.Context, runID string) error
	ResumeRunFunc          func(ctx context.Context, runID string) error
	ListLocalWorkflowsFunc func(ctx context.Context) ([]LocalWorkflow, error)
	ValidateWorkflowFunc   func(ctx context.Context, ref string) error
	GraphWorkflowFunc      func(ctx context.Context, ref string) (string, error)
	DryRunWorkflowFunc     func(ctx context.Context, ref string, inputs, vars map[string]any) (string, error)
}

// DaemonStatus implements DaemonClient.
func (f *FakeClient) DaemonStatus(ctx context.Context) (DaemonState, error) {
	if f.DaemonStateFunc != nil {
		return f.DaemonStateFunc(ctx)
	}
	return DaemonState{Status: DaemonUnavailable}, ErrDaemonUnavailable
}

// ListRuns implements RunClient.
func (f *FakeClient) ListRuns(ctx context.Context) ([]RunSummary, error) {
	if f.ListRunsFunc != nil {
		return f.ListRunsFunc(ctx)
	}
	return nil, nil
}

// GetRun implements RunClient.
func (f *FakeClient) GetRun(ctx context.Context, runID string) (RunSummary, error) {
	if f.GetRunFunc != nil {
		return f.GetRunFunc(ctx, runID)
	}
	return RunSummary{}, nil
}

// GetRunLogs implements RunClient.
func (f *FakeClient) GetRunLogs(ctx context.Context, runID string) ([]string, error) {
	if f.GetRunLogsFunc != nil {
		return f.GetRunLogsFunc(ctx, runID)
	}
	return nil, nil
}

// GetRunEvents implements RunClient.
func (f *FakeClient) GetRunEvents(ctx context.Context, runID string, cursor int, limit int) (EventBatch, error) {
	if f.GetRunEventsFunc != nil {
		return f.GetRunEventsFunc(ctx, runID, cursor, limit)
	}
	return EventBatch{}, nil
}

// GetRunNodes implements RunClient.
func (f *FakeClient) GetRunNodes(ctx context.Context, runID string) ([]NodeSummary, error) {
	if f.GetRunNodesFunc != nil {
		return f.GetRunNodesFunc(ctx, runID)
	}
	return nil, nil
}

// GetRunPlan implements RunClient.
func (f *FakeClient) GetRunPlan(ctx context.Context, runID string) (PlanSummary, error) {
	if f.GetRunPlanFunc != nil {
		return f.GetRunPlanFunc(ctx, runID)
	}
	return PlanSummary{}, nil
}

// ListArtifacts implements ArtifactClient.
func (f *FakeClient) ListArtifacts(ctx context.Context, runID string) ([]ArtifactSummary, error) {
	if f.ListArtifactsFunc != nil {
		return f.ListArtifactsFunc(ctx, runID)
	}
	return nil, nil
}

// GetArtifact implements ArtifactClient.
func (f *FakeClient) GetArtifact(ctx context.Context, runID, artifactID string) (ArtifactSummary, error) {
	if f.GetArtifactFunc != nil {
		return f.GetArtifactFunc(ctx, runID, artifactID)
	}
	return ArtifactSummary{}, nil
}

// CancelRun implements ControlClient.
func (f *FakeClient) CancelRun(ctx context.Context, runID string) error {
	if f.CancelRunFunc != nil {
		return f.CancelRunFunc(ctx, runID)
	}
	return nil
}

// PauseRun implements ControlClient.
func (f *FakeClient) PauseRun(ctx context.Context, runID string) error {
	if f.PauseRunFunc != nil {
		return f.PauseRunFunc(ctx, runID)
	}
	return nil
}

// ResumeRun implements ControlClient.
func (f *FakeClient) ResumeRun(ctx context.Context, runID string) error {
	if f.ResumeRunFunc != nil {
		return f.ResumeRunFunc(ctx, runID)
	}
	return nil
}

// ListLocalWorkflows implements WorkflowClient.
func (f *FakeClient) ListLocalWorkflows(ctx context.Context) ([]LocalWorkflow, error) {
	if f.ListLocalWorkflowsFunc != nil {
		return f.ListLocalWorkflowsFunc(ctx)
	}
	return nil, nil
}

// ValidateWorkflow implements WorkflowClient.
func (f *FakeClient) ValidateWorkflow(ctx context.Context, ref string) error {
	if f.ValidateWorkflowFunc != nil {
		return f.ValidateWorkflowFunc(ctx, ref)
	}
	return nil
}

// GraphWorkflow implements WorkflowClient.
func (f *FakeClient) GraphWorkflow(ctx context.Context, ref string) (string, error) {
	if f.GraphWorkflowFunc != nil {
		return f.GraphWorkflowFunc(ctx, ref)
	}
	return "", nil
}

// DryRunWorkflow implements WorkflowClient.
func (f *FakeClient) DryRunWorkflow(ctx context.Context, ref string, inputs, vars map[string]any) (string, error) {
	if f.DryRunWorkflowFunc != nil {
		return f.DryRunWorkflowFunc(ctx, ref, inputs, vars)
	}
	return "", nil
}
