package app

import (
	"io"

	claudeagent "github.com/diasYuri/agentflow/internal/adapters/agent/claude"
	codexagent "github.com/diasYuri/agentflow/internal/adapters/agent/codex"
	piagent "github.com/diasYuri/agentflow/internal/adapters/agent/pi"
	"github.com/diasYuri/agentflow/internal/adapters/events/jsonl"
	"github.com/diasYuri/agentflow/internal/adapters/events/multi"
	"github.com/diasYuri/agentflow/internal/adapters/events/stdout"
	runrepo "github.com/diasYuri/agentflow/internal/adapters/runrepo/local"
	"github.com/diasYuri/agentflow/internal/adapters/shell"
	yamlrepo "github.com/diasYuri/agentflow/internal/adapters/yaml"
	"github.com/diasYuri/agentflow/internal/core/ports"
	runworkflow "github.com/diasYuri/agentflow/internal/core/runtime"
)

type RuntimeOptions struct {
	CodexPath   string
	ClaudePath  string
	PiPath      string
	LogFormat   string
	EventsJSONL string
	EventWriter io.Writer
	RunRoot     string
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
	registry := ports.NewStaticAgentProviderRegistry(map[string]ports.AgentProvider{
		"codex":  codexagent.New(opts.CodexPath),
		"claude": claudeagent.New(opts.ClaudePath),
		"pi":     piagent.New(opts.PiPath),
	})
	return &runworkflow.RunWorkflowUseCase{
		Workflows: yamlrepo.NewWorkflowRepository(),
		Runs:      runrepo.New(opts.RunRoot),
		Events:    sink,
		Agents:    registry,
		Shell:     shell.NewRunner(),
	}, nil
}
