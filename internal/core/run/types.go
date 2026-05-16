package run

import (
	"time"

	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type NodeStatus string

const (
	NodePending   NodeStatus = "pending"
	NodeSkipped   NodeStatus = "skipped"
	NodeRunning   NodeStatus = "running"
	NodeSuccess   NodeStatus = "success"
	NodeFailed    NodeStatus = "failed"
	NodeCancelled NodeStatus = "cancelled"
	NodeTimeout   NodeStatus = "timeout"
)

type RunStatus string

const (
	RunCreated    RunStatus = "created"
	RunValidating RunStatus = "validating"
	RunPlanned    RunStatus = "planned"
	RunRunning    RunStatus = "running"
	RunPaused     RunStatus = "paused"
	RunSuccess    RunStatus = "success"
	RunFailed     RunStatus = "failed"
	RunCancelled  RunStatus = "cancelled"
)

type PauseReason string

const (
	PauseReasonManual        PauseReason = "manual"
	PauseReasonPauseWhenFail PauseReason = "pause_when_fail"
)

type Event struct {
	Timestamp  time.Time      `json:"ts"`
	RunID      string         `json:"run_id"`
	Type       string         `json:"type"`
	NodeID     string         `json:"node_id,omitempty"`
	InstanceID string         `json:"instance_id,omitempty"`
	Path       []string       `json:"path,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

type NodeResult struct {
	RunID      string        `json:"run_id,omitempty"`
	NodeID     string        `json:"node_id"`
	InstanceID string        `json:"instance_id,omitempty"`
	Path       []string      `json:"path,omitempty"`
	Index      *int          `json:"index,omitempty"`
	Status     NodeStatus    `json:"status"`
	Output     any           `json:"output,omitempty"`
	Outputs    []any         `json:"outputs,omitempty"`
	Stdout     string        `json:"stdout,omitempty"`
	Stderr     string        `json:"stderr,omitempty"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	Attempts   int           `json:"attempts,omitempty"`
}

type RunMetadata struct {
	RunID        string    `json:"run_id"`
	Workflow     string    `json:"workflow"`
	WorkflowPath string    `json:"workflow_path"`
	StartedAt    time.Time `json:"started_at"`
	OutputDir    string    `json:"output_dir"`
}

type RunHandle struct {
	RunID string `json:"run_id"`
	Dir   string `json:"dir"`
}

type Summary struct {
	RunID       string                `json:"run_id"`
	Workflow    string                `json:"workflow"`
	Status      RunStatus             `json:"status"`
	StartedAt   time.Time             `json:"started_at"`
	FinishedAt  time.Time             `json:"finished_at"`
	DurationMS  int64                 `json:"duration_ms"`
	AgentCalls  int                   `json:"agent_calls"`
	BashCalls   int                   `json:"bash_calls"`
	FailedNodes int                   `json:"failed_nodes"`
	Retries     int                   `json:"retries"`
	Nodes       map[string]NodeResult `json:"nodes"`
}

type CheckpointMetrics struct {
	AgentCalls int `json:"agent_calls"`
	BashCalls  int `json:"bash_calls"`
	Retries    int `json:"retries"`
}

type Checkpoint struct {
	RunID        string                `json:"run_id"`
	Workflow     workflow.WorkflowSpec `json:"workflow"`
	WorkflowPath string                `json:"workflow_path"`
	Status       RunStatus             `json:"status"`
	Reason       PauseReason           `json:"reason,omitempty"`
	Cursor       int                   `json:"cursor"`
	RetryNodeID  string                `json:"retry_node_id,omitempty"`
	Inputs       map[string]any        `json:"inputs,omitempty"`
	StartedAt    time.Time             `json:"started_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Metrics      CheckpointMetrics     `json:"metrics"`
	Nodes        map[string]NodeResult `json:"nodes,omitempty"`
}
