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
	GetRunDiagnostics(ctx context.Context, runID string) (RunDiagnosticSummary, error)
	GetRunTimeline(ctx context.Context, runID string, cursor int, limit int) (RunTimeline, error)
	GetRunChartSeries(ctx context.Context, runID string) ([]ChartSeries, error)
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

// AgentUsage tracks token and cost usage reported by an agent provider.
type AgentUsage struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model,omitempty"`
	InputTokens  int64   `json:"input_tokens,omitempty"`
	OutputTokens int64   `json:"output_tokens,omitempty"`
	TotalTokens  int64   `json:"total_tokens,omitempty"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

// SlowestNode is a lightweight snapshot for the summary top list.
type SlowestNode struct {
	NodeID     string `json:"node_id"`
	DurationMS int64  `json:"duration_ms"`
}

// RunDiagnosticSummary holds aggregated diagnostic metrics for a run.
type RunDiagnosticSummary struct {
	DurationMS    int64         `json:"duration_ms"`
	FailedNodes   int           `json:"failed_nodes"`
	Retries       int           `json:"retries"`
	AgentCalls    int           `json:"agent_calls"`
	BashCalls     int           `json:"bash_calls"`
	ArtifactCount int           `json:"artifact_count"`
	NodeCount     int           `json:"node_count"`
	FirstError    string        `json:"first_error,omitempty"`
	SlowestNodes  []SlowestNode `json:"slowest_nodes,omitempty"`
	AgentUsage    []AgentUsage  `json:"agent_usage,omitempty"`
}

// TimelineEntry represents a single event in the run timeline.
type TimelineEntry struct {
	Timestamp  time.Time `json:"ts"`
	Type       string    `json:"type"`
	NodeID     string    `json:"node_id,omitempty"`
	InstanceID string    `json:"instance_id,omitempty"`
	Attempt    int       `json:"attempt,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// RunTimeline holds a page of timeline entries with pagination info.
type RunTimeline struct {
	Entries    []TimelineEntry
	NextCursor int
	HasMore    bool
}

// ChartSeries holds chart-ready data points for a run metric.
type ChartSeries struct {
	Name   string    `json:"name"`
	Labels []string  `json:"labels"`
	Values []float64 `json:"values"`
}

// RunSummary is a TUI-friendly representation of a workflow run.
type RunSummary struct {
	ID                string
	Workflow          string
	RunDir            string
	Status            string
	StartedAt         time.Time
	FinishedAt        time.Time
	PausedAt          time.Time
	PauseReason       string
	ResumeCount       int
	CurrentStep       string
	CompletedSteps    []string
	PendingSteps      []string
	TotalSteps        int
	Error             string
	TerminalError     string
	FailureReason     string
	Tag               string
	DiagnosticSummary *RunDiagnosticSummary
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
