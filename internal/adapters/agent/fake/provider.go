package fake

import (
	"context"
	"fmt"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

type Provider struct {
	Responses map[string]ports.AgentResult
}

func New() *Provider {
	return &Provider{Responses: map[string]ports.AgentResult{}}
}

func (p *Provider) Run(ctx context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	_ = ctx
	if result, ok := p.Responses[req.NodeID]; ok {
		return result, nil
	}
	text := req.Prompt
	if text == "" {
		text = fmt.Sprintf("fake response for %s", req.NodeID)
	}
	return ports.AgentResult{Text: text}, nil
}
