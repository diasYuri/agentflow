package ports

import (
	"context"
	"time"

	"github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

type AgentProvider interface {
	Run(ctx context.Context, req AgentRequest) (AgentResult, error)
}

type AgentRequest struct {
	RunID        string
	NodeID       string
	InstanceID   string
	Attempt      int
	Provider     string
	Model        string
	System       string
	Prompt       string
	WorkingDir   string
	Env          map[string]string
	OutputSchema map[string]any
	Metadata     map[string]any
	Sandbox      workflow.SandboxSpec
}

type AgentResult struct {
	Text      string
	JSON      any
	RawEvents []AgentEvent
	Usage     *Usage
	Metadata  map[string]any
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type AgentEvent struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type AgentProviderRegistry interface {
	Get(name string) (AgentProvider, bool)
	HasProvider(name string) bool
}

type ShellRunner interface {
	Run(ctx context.Context, req ShellRequest) (ShellResult, error)
}

type ShellRequest struct {
	Command        string
	Shell          string
	WorkingDir     string
	Env            map[string]string
	MaxOutputBytes int64
}

type ShellResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

type EventSink interface {
	Emit(ctx context.Context, event run.Event) error
	Close(ctx context.Context) error
}

type WorkflowRepository interface {
	Load(ctx context.Context, ref string) (*workflow.WorkflowSpec, string, error)
}

type RunRepository interface {
	CreateRun(ctx context.Context, meta run.RunMetadata) (run.RunHandle, error)
	SaveWorkflow(ctx context.Context, runID string, sourcePath string, normalized any, plan any) error
	SaveNodeResult(ctx context.Context, runID string, result run.NodeResult) error
	SaveArtifact(ctx context.Context, runID string, name string, data []byte) error
	FinalizeRun(ctx context.Context, runID string, summary run.Summary) error
	RunDir(runID string) (string, bool)
}
