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

type WorktreeMergeStatus string

const (
	WorktreeMergeNoChanges WorktreeMergeStatus = "no_changes"
	WorktreeMergeMerged    WorktreeMergeStatus = "merged"
	WorktreeMergeConflict  WorktreeMergeStatus = "conflict"
	WorktreeMergeFailed    WorktreeMergeStatus = "failed"
)

type WorktreeCleanupStatus string

const (
	WorktreeCleanupRemoved WorktreeCleanupStatus = "removed"
	WorktreeCleanupKept    WorktreeCleanupStatus = "kept"
)

type WorktreeChangedFile struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	OldPath string `json:"old_path,omitempty"`
}

type WorktreeConflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type WorktreeGitCommand struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

type WorktreeMetadata struct {
	Enabled                 bool                  `json:"enabled"`
	Provider                string                `json:"provider"`
	Name                    string                `json:"name"`
	MergeStatus             WorktreeMergeStatus   `json:"merge_status"`
	CleanupStatus           WorktreeCleanupStatus `json:"cleanup_status"`
	ChangedFiles            []WorktreeChangedFile `json:"changed_files,omitempty"`
	BaseCommit              string                `json:"base_commit"`
	DestinationCommitBefore string                `json:"destination_commit_before_merge,omitempty"`
	DestinationCommitAfter  string                `json:"destination_commit_after_merge,omitempty"`
	WorktreePath            string                `json:"path"`
	Destination             string                `json:"destination"`
	Conflicts               []WorktreeConflict    `json:"conflicts,omitempty"`
	Commands                []WorktreeGitCommand  `json:"commands,omitempty"`
	AgentResolutionError    string                `json:"agent_resolution_error,omitempty"`
}

type WorktreeCheckpoint struct {
	Enabled               bool   `json:"enabled"`
	Provider              string `json:"provider"`
	ID                    string `json:"id,omitempty"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	Branch                string `json:"branch,omitempty"`
	BaseCommit            string `json:"base_commit"`
	WorkflowName          string `json:"workflow_name"`
	DestinationWorkingDir string `json:"destination_working_dir"`
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
	Worktree     *WorktreeCheckpoint   `json:"worktree,omitempty"`
}
