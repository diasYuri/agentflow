package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

type Provider struct {
	Responses map[string]ports.AgentResult
}

func New() *Provider {
	return &Provider{Responses: map[string]ports.AgentResult{}}
}

func NewFromPath(path string) (*Provider, error) {
	p := New()
	if path == "" {
		return p, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fake provider config %q: %w", path, err)
	}
	var responses map[string]ports.AgentResult
	if err := json.Unmarshal(data, &responses); err != nil {
		return nil, fmt.Errorf("parse fake provider config %q: %w", path, err)
	}
	p.Responses = responses
	return p, nil
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
