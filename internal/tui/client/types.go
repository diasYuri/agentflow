package client

import (
	"context"
	"time"
)

// DaemonStatus represents the daemon availability state.
type DaemonStatus string

const (
	DaemonUnknown         DaemonStatus = "unknown"
	DaemonAvailable       DaemonStatus = "available"
	DaemonUnavailable     DaemonStatus = "unavailable"
	DaemonRequiredMissing DaemonStatus = "required_missing"
)

// DaemonClient provides daemon status information.
type DaemonClient interface {
	DaemonStatus(ctx context.Context) (DaemonState, error)
}

// RunClient provides run-related operations.
type RunClient interface {
	ListRuns(ctx context.Context) ([]RunSummary, error)
	GetRun(ctx context.Context, runID string) (RunSummary, error)
	GetRunLogs(ctx context.Context, runID string) ([]string, error)
	GetRunEvents(ctx context.Context, runID string, cursor int, limit int) (EventBatch, error)
	GetRunNodes(ctx context.Context, runID string) ([]NodeSummary, error)
	GetRunPlan(ctx context.Context, runID string) (PlanSummary, error)
}

// WorkflowClient provides workflow-related operations.
type WorkflowClient interface {
	ListLocalWorkflows(ctx context.Context) ([]LocalWorkflow, error)
	ValidateWorkflow(ctx context.Context, ref string) error
	GraphWorkflow(ctx context.Context, ref string) (string, error)
	DryRunWorkflow(ctx context.Context, ref string, inputs, vars map[string]any) (string, error)
}

// ControlClient provides run control operations.
type ControlClient interface {
	CancelRun(ctx context.Context, runID string) error
	PauseRun(ctx context.Context, runID string) error
	ResumeRun(ctx context.Context, runID string) error
}

// Client composes all client capabilities.
type Client interface {
	DaemonClient
	RunClient
	WorkflowClient
	ArtifactClient
	ControlClient
}

// DaemonState holds daemon status information.
type DaemonState struct {
	Status    DaemonStatus
	Running   bool
	PID       int
	Socket    string
	Runs      int
	StartedAt time.Time
	LastCheck time.Time
}

// RunSummary is a TUI-friendly representation of a workflow run.
type RunSummary struct {
	ID             string
	Workflow       string
	RunDir         string
	Status         string
	StartedAt      time.Time
	FinishedAt     time.Time
	PausedAt       time.Time
	PauseReason    string
	ResumeCount    int
	CurrentStep    string
	CompletedSteps []string
	PendingSteps   []string
	TotalSteps     int
	Error          string
	TerminalError  string
	FailureReason  string
	Tag            string
}

// LocalWorkflow represents a locally discovered workflow.
type LocalWorkflow struct {
	Name        string
	Path        string
	Description string
	NodeCount   int
	Inputs      map[string]InputSpec
}

// InputSpec represents a workflow input definition.
type InputSpec struct {
	Type     string
	Required bool
	Default  any
}

// EventLine represents a single workflow event.
type EventLine struct {
	Cursor    int
	Timestamp time.Time
	RunID     string
	Type      string
	NodeID    string
	Message   string
}

// EventBatch holds a page of events with pagination info.
type EventBatch struct {
	Events     []EventLine
	NextCursor int
	HasMore    bool
}

// ArtifactSummary represents a workflow artifact.
type ArtifactSummary struct {
	ID          string
	Name        string
	Path        string
	Size        int64
	ContentType string
	ModifiedAt  time.Time
	Encoding    string
	Content     string
}

// ArtifactClient provides artifact operations.
type ArtifactClient interface {
	ListArtifacts(ctx context.Context, runID string) ([]ArtifactSummary, error)
	GetArtifact(ctx context.Context, runID string, artifactID string) (ArtifactSummary, error)
}

// NodeSummary represents a node execution result.
type NodeSummary struct {
	NodeID     string
	InstanceID string
	Status     string
	Output     any
	Outputs    []any
	Stdout     string
	Stderr     string
	Error      string
	ExitCode   *int
	Duration   int64
	Attempts   int
}

// PlanSummary represents a workflow execution plan.
type PlanSummary struct {
	WorkflowName string
	Order        []string
	Nodes        map[string]any
}
