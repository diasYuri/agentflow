package daemon

import (
	"os"
	"path/filepath"
	"time"

	corerun "github.com/diasYuri/agentflow/internal/core/run"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

const SocketName = "agentflowd.sock"
const PIDName = "agentflowd.pid"
const LogName = "agentflowd.log"

type Config struct {
	SocketPath string
	PIDPath    string
	LogPath    string
	RunRoot    string
	DBPath     string
	CodexPath  string
	ClaudePath string
	PiPath     string
}

func DefaultConfig() Config {
	root := defaultAgentFlowRoot()
	return Config{
		SocketPath: filepath.Join(root, SocketName),
		PIDPath:    filepath.Join(root, PIDName),
		LogPath:    filepath.Join(root, LogName),
		RunRoot:    filepath.Join(root, "runs"),
		DBPath:     filepath.Join(root, "agentflowd.sqlite"),
	}
}

func defaultAgentFlowRoot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".agentflow")
	}
	return filepath.Join(".", ".agentflow")
}

type DaemonStatus struct {
	Running   bool      `json:"running"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Socket    string    `json:"socket"`
	Runs      int       `json:"runs"`
}

type RunWorkflowRequest struct {
	WorkflowRef    string         `json:"workflow_ref"`
	Inputs         map[string]any `json:"inputs,omitempty"`
	Vars           map[string]any `json:"vars,omitempty"`
	MaxConcurrency int            `json:"max_concurrency,omitempty"`
	WorkingDir     string         `json:"working_dir,omitempty"`
	CodexPath      string         `json:"codex_path,omitempty"`
	ClaudePath     string         `json:"claude_path,omitempty"`
	PiPath         string         `json:"pi_path,omitempty"`
	LogFormat      string         `json:"log_format,omitempty"`
	EventsJSONL    string         `json:"events_jsonl,omitempty"`
	RunRoot        string         `json:"run_root,omitempty"`
	OutputDir      string         `json:"output_dir,omitempty"`
	DryRun         bool           `json:"dry_run,omitempty"`
	Tag            string         `json:"tag,omitempty"`
}

type WorkflowRun struct {
	ID             string              `json:"id"`
	Workflow       string              `json:"workflow"`
	RunDir         string              `json:"run_dir"`
	Status         corerun.RunStatus   `json:"status"`
	StartedAt      time.Time           `json:"started_at"`
	FinishedAt     time.Time           `json:"finished_at,omitempty"`
	PausedAt       time.Time           `json:"paused_at,omitempty"`
	PauseReason    string              `json:"pause_reason,omitempty"`
	ResumeCount    int                 `json:"resume_count,omitempty"`
	CurrentStep    string              `json:"current_step,omitempty"`
	CompletedSteps []string            `json:"completed_steps,omitempty"`
	PendingSteps   []string            `json:"pending_steps,omitempty"`
	TotalSteps     int                 `json:"total_steps,omitempty"`
	Error          string              `json:"error,omitempty"`
	TerminalError  string              `json:"terminal_error,omitempty"`
	RecentEvents   []string            `json:"recent_events,omitempty"`
	Tag            string              `json:"tag,omitempty"`
	Request        *RunWorkflowRequest `json:"-"`
}

type RunWorkflowResponse struct {
	Run WorkflowRun `json:"run"`
}

type ListWorkflowsResponse struct {
	Runs []WorkflowRun `json:"runs"`
}

type LogsResponse struct {
	RunID string   `json:"run_id"`
	Lines []string `json:"lines"`
}

type WorkflowEventDTO struct {
	Cursor     int            `json:"cursor"`
	Timestamp  time.Time      `json:"timestamp"`
	RunID      string         `json:"run_id"`
	Type       string         `json:"type"`
	NodeID     string         `json:"node_id,omitempty"`
	InstanceID string         `json:"instance_id,omitempty"`
	Path       []string       `json:"path,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type WorkflowEventsResponse struct {
	RunID      string             `json:"run_id"`
	Events     []WorkflowEventDTO `json:"events"`
	NextCursor int                `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

type APIError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type WorkflowArtifactsResponse struct {
	RunID      string                `json:"run_id"`
	Artifacts  []WorkflowArtifactDTO `json:"artifacts"`
	NextCursor string                `json:"next_cursor,omitempty"`
	HasMore    bool                  `json:"has_more,omitempty"`
}

type WorkflowArtifactDTO struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Size         int64                `json:"size"`
	ContentType  string               `json:"content_type,omitempty"`
	ModifiedAt   time.Time            `json:"modified_at,omitempty"`
	RunID        string               `json:"run_id,omitempty"`
	NodeID       string               `json:"node_id,omitempty"`
	InstanceID   string               `json:"instance_id,omitempty"`
	RelativePath string               `json:"relative_path,omitempty"`
	MediaType    string               `json:"media_type,omitempty"`
	SizeBytes    int64                `json:"size_bytes,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`
	Kind         corerun.ArtifactKind `json:"kind,omitempty"`
	Description  string               `json:"description,omitempty"`
}

type WorkflowArtifactResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Path         string               `json:"path"`
	Size         int64                `json:"size"`
	ContentType  string               `json:"content_type,omitempty"`
	Encoding     string               `json:"encoding,omitempty"`
	Content      string               `json:"content,omitempty"`
	RunID        string               `json:"run_id,omitempty"`
	NodeID       string               `json:"node_id,omitempty"`
	InstanceID   string               `json:"instance_id,omitempty"`
	RelativePath string               `json:"relative_path,omitempty"`
	MediaType    string               `json:"media_type,omitempty"`
	SizeBytes    int64                `json:"size_bytes,omitempty"`
	CreatedAt    time.Time            `json:"created_at,omitempty"`
	Kind         corerun.ArtifactKind `json:"kind,omitempty"`
	Description  string               `json:"description,omitempty"`
	TextContent  string               `json:"text_content,omitempty"`
	Truncated    bool                 `json:"truncated,omitempty"`
	IsText       bool                 `json:"is_text,omitempty"`
}

type WorkflowNodesResponse struct {
	RunID string                  `json:"run_id"`
	Nodes []WorkflowNodeResultDTO `json:"nodes"`
}

type WorkflowNodeResponse struct {
	RunID     string                  `json:"run_id"`
	NodeID    string                  `json:"node_id"`
	Instances []WorkflowNodeResultDTO `json:"instances,omitempty"`
}

type WorkflowNodeResultDTO struct {
	NodeID     string   `json:"node_id"`
	InstanceID string   `json:"instance_id,omitempty"`
	Path       []string `json:"path,omitempty"`
	Index      *int     `json:"index,omitempty"`
	Status     string   `json:"status"`
	Output     any      `json:"output,omitempty"`
	Outputs    []any    `json:"outputs,omitempty"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	Error      string   `json:"error,omitempty"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	Duration   int64    `json:"duration_ms,omitempty"`
	Attempts   int      `json:"attempts,omitempty"`
}

type WorkflowPlanResponse struct {
	RunID      string         `json:"run_id"`
	Workflow   string         `json:"workflow,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Normalized map[string]any `json:"normalized,omitempty"`
	Plan       map[string]any `json:"plan,omitempty"`
}

const (
	defaultEventLimit = 100
	maxEventLimit     = 1000
	MaxArtifactInline = 128 * 1024
)

type CancelWorkflowResponse struct {
	Run WorkflowRun `json:"run"`
}

type PauseWorkflowResponse struct {
	Run WorkflowRun `json:"run"`
}

type ResumeWorkflowResponse struct {
	Run WorkflowRun `json:"run"`
}

type StopResponse struct {
	Stopping bool `json:"stopping"`
}

func runOptions(req RunWorkflowRequest, runID string) runworkflow.RunOptions {
	return runworkflow.RunOptions{
		WorkflowRef:    req.WorkflowRef,
		RunID:          runID,
		Inputs:         req.Inputs,
		Vars:           req.Vars,
		MaxConcurrency: req.MaxConcurrency,
		WorkingDir:     req.WorkingDir,
		DryRun:         req.DryRun,
		Tag:            req.Tag,
	}
}

func resumeOptions(req RunWorkflowRequest, runID string) runworkflow.RunOptions {
	return runworkflow.RunOptions{
		ResumeRunID: runID,
		WorkingDir:  req.WorkingDir,
	}
}
