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
	RunCreated         RunStatus = "created"
	RunValidating      RunStatus = "validating"
	RunPlanned         RunStatus = "planned"
	RunQueued          RunStatus = "queued"
	RunRunning         RunStatus = "running"
	RunPaused          RunStatus = "paused"
	RunWaitingApproval RunStatus = "wait_approval"
	RunSuccess         RunStatus = "success"
	RunFailed          RunStatus = "failed"
	RunCancelled       RunStatus = "cancelled"
)

type PauseReason string

const (
	PauseReasonManual        PauseReason = "manual"
	PauseReasonPauseWhenFail PauseReason = "pause_when_fail"
	PauseReasonWorktreeMerge PauseReason = "worktree_merge"
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

type ArtifactRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
}

type NodeResult struct {
	RunID           string         `json:"run_id,omitempty"`
	NodeID          string         `json:"node_id"`
	InstanceID      string         `json:"instance_id,omitempty"`
	Path            []string       `json:"path,omitempty"`
	Index           *int           `json:"index,omitempty"`
	Status          NodeStatus     `json:"status"`
	Output          any            `json:"output,omitempty"`
	Outputs         []any          `json:"outputs,omitempty"`
	DeclaredOutputs map[string]any `json:"declared_outputs,omitempty"`
	Stdout          string         `json:"stdout,omitempty"`
	Stderr          string         `json:"stderr,omitempty"`
	ExitCode        *int           `json:"exit_code,omitempty"`
	Error           string         `json:"error,omitempty"`
	Duration        time.Duration  `json:"duration,omitempty"`
	Attempts        int            `json:"attempts,omitempty"`
	Artifacts       []ArtifactRef  `json:"artifacts,omitempty"`
}

type RunMetadata struct {
	RunID        string    `json:"run_id"`
	Workflow     string    `json:"workflow"`
	WorkflowPath string    `json:"workflow_path"`
	StartedAt    time.Time `json:"started_at"`
	OutputDir    string    `json:"output_dir"`
	Tag          string    `json:"tag,omitempty"`
}

type RunHandle struct {
	RunID string `json:"run_id"`
	Dir   string `json:"dir"`
}

// NodeMetrics holds aggregated metrics for a single node execution.
type NodeMetrics struct {
	NodeID        string `json:"node_id"`
	InstanceID    string `json:"instance_id,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	Attempts      int    `json:"attempts"`
	Retries       int    `json:"retries"`
	BashCalls     int    `json:"bash_calls"`
	AgentCalls    int    `json:"agent_calls"`
	StdoutBytes   int64  `json:"stdout_bytes"`
	StderrBytes   int64  `json:"stderr_bytes"`
	ArtifactCount int    `json:"artifact_count"`
	FirstError    string `json:"first_error,omitempty"`
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

// TimelineEntry represents a single event in the run timeline.
type TimelineEntry struct {
	Timestamp  time.Time `json:"ts"`
	Type       string    `json:"type"`
	NodeID     string    `json:"node_id,omitempty"`
	InstanceID string    `json:"instance_id,omitempty"`
	Attempt    int       `json:"attempt,omitempty"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// SlowestNode is a lightweight snapshot for the summary top list.
type SlowestNode struct {
	NodeID     string `json:"node_id"`
	DurationMS int64  `json:"duration_ms"`
}

type Summary struct {
	RunID         string                `json:"run_id"`
	Workflow      string                `json:"workflow"`
	Status        RunStatus             `json:"status"`
	StartedAt     time.Time             `json:"started_at"`
	FinishedAt    time.Time             `json:"finished_at"`
	DurationMS    int64                 `json:"duration_ms"`
	AgentCalls    int                   `json:"agent_calls"`
	BashCalls     int                   `json:"bash_calls"`
	FailedNodes   int                   `json:"failed_nodes"`
	Retries       int                   `json:"retries"`
	Nodes         map[string]NodeResult `json:"nodes"`
	Tag           string                `json:"tag,omitempty"`
	SlowestNodes  []SlowestNode         `json:"slowest_nodes,omitempty"`
	AgentUsage    []AgentUsage          `json:"agent_usage,omitempty"`
	Timeline      []TimelineEntry       `json:"timeline,omitempty"`
	ArtifactCount int                   `json:"artifact_count"`
	FirstError    string                `json:"first_error,omitempty"`
}

type CheckpointMetrics struct {
	AgentCalls    int                    `json:"agent_calls"`
	BashCalls     int                    `json:"bash_calls"`
	Retries       int                    `json:"retries"`
	NodeMetrics   map[string]NodeMetrics `json:"node_metrics,omitempty"`
	AgentUsage    []AgentUsage           `json:"agent_usage,omitempty"`
	Timeline      []TimelineEntry        `json:"timeline,omitempty"`
	ArtifactCount int                    `json:"artifact_count"`
	FirstError    string                 `json:"first_error,omitempty"`
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

type WorktreeAgentResolutionStatus string

const (
	WorktreeAgentResolutionNotAttempted WorktreeAgentResolutionStatus = "not_attempted"
	WorktreeAgentResolutionRequested    WorktreeAgentResolutionStatus = "requested"
	WorktreeAgentResolutionResolved     WorktreeAgentResolutionStatus = "resolved"
	WorktreeAgentResolutionFailed       WorktreeAgentResolutionStatus = "failed"
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
	Enabled                 bool                          `json:"enabled"`
	Provider                string                        `json:"provider"`
	GitProvider             string                        `json:"git_provider,omitempty"`
	Name                    string                        `json:"name"`
	MergeStatus             WorktreeMergeStatus           `json:"merge_status"`
	CleanupStatus           WorktreeCleanupStatus         `json:"cleanup_status"`
	ChangedFiles            []WorktreeChangedFile         `json:"changed_files,omitempty"`
	BaseCommit              string                        `json:"base_commit"`
	DestinationCommitBefore string                        `json:"destination_commit_before_merge,omitempty"`
	DestinationCommitAfter  string                        `json:"destination_commit_after_merge,omitempty"`
	WorktreePath            string                        `json:"path"`
	Destination             string                        `json:"destination"`
	Conflicts               []WorktreeConflict            `json:"conflicts,omitempty"`
	Commands                []WorktreeGitCommand          `json:"commands,omitempty"`
	MergeFailureCause       string                        `json:"merge_failure_cause,omitempty"`
	AgentResolutionStatus   WorktreeAgentResolutionStatus `json:"agent_resolution_status,omitempty"`
	AgentResolutionProvider string                        `json:"agent_resolution_provider,omitempty"`
	AgentResolutionError    string                        `json:"agent_resolution_error,omitempty"`
}

type WorktreeCheckpoint struct {
	Enabled               bool   `json:"enabled"`
	Provider              string `json:"provider"`
	AgentProvider         string `json:"agent_provider,omitempty"`
	ID                    string `json:"id,omitempty"`
	Name                  string `json:"name"`
	Path                  string `json:"path"`
	Branch                string `json:"branch,omitempty"`
	BaseCommit            string `json:"base_commit"`
	WorkflowName          string `json:"workflow_name"`
	DestinationWorkingDir string `json:"destination_working_dir"`
}

type ApprovalCheckpoint struct {
	NodeID  string `json:"node_id"`
	Message string `json:"message"`
}

type ArtifactKind string

const (
	ArtifactKindFile    ArtifactKind = "file"
	ArtifactKindStdout  ArtifactKind = "stdout"
	ArtifactKindStderr  ArtifactKind = "stderr"
	ArtifactKindResult  ArtifactKind = "result"
	ArtifactKindSummary ArtifactKind = "summary"
	ArtifactKindCustom  ArtifactKind = "custom"
)

// Artifact represents a run artifact with public metadata.
type Artifact struct {
	ID           string       `json:"id"`
	RunID        string       `json:"run_id"`
	NodeID       string       `json:"node_id,omitempty"`
	InstanceID   string       `json:"instance_id,omitempty"`
	Name         string       `json:"name"`
	RelativePath string       `json:"relative_path"`
	MediaType    string       `json:"media_type,omitempty"`
	SizeBytes    int64        `json:"size_bytes"`
	CreatedAt    time.Time    `json:"created_at"`
	Kind         ArtifactKind `json:"kind"`
	Description  string       `json:"description,omitempty"`
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
	Tag          string                `json:"tag,omitempty"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Metrics      CheckpointMetrics     `json:"metrics"`
	Nodes        map[string]NodeResult `json:"nodes,omitempty"`
	Worktree     *WorktreeCheckpoint   `json:"worktree,omitempty"`
	Approval     *ApprovalCheckpoint   `json:"approval,omitempty"`
}
