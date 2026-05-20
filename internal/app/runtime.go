package app

import (
	"io"

	claudeagent "github.com/diasYuri/agentflow/internal/adapters/agent/claude"
	codexagent "github.com/diasYuri/agentflow/internal/adapters/agent/codex"
	fakeagent "github.com/diasYuri/agentflow/internal/adapters/agent/fake"
	piagent "github.com/diasYuri/agentflow/internal/adapters/agent/pi"
	"github.com/diasYuri/agentflow/internal/adapters/events/jsonl"
	"github.com/diasYuri/agentflow/internal/adapters/events/multi"
	"github.com/diasYuri/agentflow/internal/adapters/events/stdout"
	extensionrpc "github.com/diasYuri/agentflow/internal/adapters/extension/rpc"
	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	gitworktree "github.com/diasYuri/agentflow/internal/adapters/worktree/git"
	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

type RuntimeOptions struct {
	CodexPath        string
	ClaudePath       string
	PiPath           string
	FakeProviderPath string
	LogFormat        string
	EventsJSONL      string
	EventWriter      io.Writer
	RunRoot          string
	Workflows        ports.WorkflowRepository
}

func NewRunWorkflowUseCase(opts RuntimeOptions) (*runworkflow.RunWorkflowUseCase, error) {
	eventSink, err := jsonl.New(opts.EventsJSONL)
	if err != nil {
		return nil, err
	}
	var sink ports.EventSink = eventSink
	if opts.EventWriter != nil {
		logFormat := opts.LogFormat
		if logFormat == "" {
			logFormat = "text"
		}
		sink = multi.New(eventSink, stdout.New(opts.EventWriter, logFormat))
	}
	providers := map[string]ports.AgentProvider{
		"codex":  codexagent.New(opts.CodexPath),
		"claude": claudeagent.New(opts.ClaudePath),
		"pi":     piagent.New(opts.PiPath),
	}
	if opts.FakeProviderPath != "" {
		fakeProv, err := fakeagent.NewFromPath(opts.FakeProviderPath)
		if err != nil {
			return nil, err
		}
		providers["fake"] = fakeProv
	}
	registry := ports.NewStaticAgentProviderRegistry(providers)
	worktreeRegistry := ports.NewStaticWorktreeProviderRegistry(map[string]ports.WorktreeProvider{
		"git": gitworktree.New(shell.NewRunner()),
	})
	workflows := opts.Workflows
	if workflows == nil {
		workflows = yamlrepo.NewWorkflowRepository()
	}
	return &runworkflow.RunWorkflowUseCase{
		Workflows:  workflows,
		Runs:       runrepo.New(opts.RunRoot),
		Events:     sink,
		Agents:     registry,
		Shell:      shell.NewRunner(),
		Extensions: extensionrpc.New(""),
		Worktrees:  worktreeRegistry,
	}, nil
}
