package chatagent

import (
	"context"
	"fmt"
	"strings"
)

const fallbackMessageLimit = 240

// FallbackAgent returns a deterministic response when no model-backed
// chat backend is configured. It keeps the conversation responsive and
// explains what the user needs to set up for real replies.
type FallbackAgent struct {
	reason string
}

// NewFallbackAgent creates a fallback agent with an optional setup reason.
func NewFallbackAgent(reason string) *FallbackAgent {
	return &FallbackAgent{reason: strings.TrimSpace(reason)}
}

func (a *FallbackAgent) Run(_ context.Context, req RunRequest) (RunResponse, error) {
	reason := strings.TrimSpace(a.reason)
	if reason == "" {
		reason = "no chat backend is configured"
	}

	userText := strings.TrimSpace(req.UserMessage)
	if userText == "" {
		userText = "(empty message)"
	}
	if len(userText) > fallbackMessageLimit {
		userText = userText[:fallbackMessageLimit] + "..."
	}

	project := strings.TrimSpace(req.ProjectName)
	if project == "" {
		project = "this session"
	}

	text := fmt.Sprintf(
		"Fallback chat mode is active for %s because %s.\n\nYour message was:\n%s\n\nTo enable model-backed replies, configure [chat_agent] in ~/.agentflow/settings.toml with a provider, model, and provider settings.",
		project,
		reason,
		userText,
	)
	return RunResponse{
		Text: text,
		Metadata: map[string]any{
			"provider": "fallback",
			"model":    "fallback",
			"reason":   reason,
		},
	}, nil
}
