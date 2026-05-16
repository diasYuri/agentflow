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
	LogFormat      string         `json:"log_format,omitempty"`
	EventsJSONL    string         `json:"events_jsonl,omitempty"`
	RunRoot        string         `json:"run_root,omitempty"`
	OutputDir      string         `json:"output_dir,omitempty"`
	DryRun         bool           `json:"dry_run,omitempty"`
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
	}
}

func resumeOptions(req RunWorkflowRequest, runID string) runworkflow.RunOptions {
	return runworkflow.RunOptions{
		ResumeRunID: runID,
		WorkingDir:  req.WorkingDir,
	}
}
