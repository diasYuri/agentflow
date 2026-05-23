// Package chatagent provides the domain-neutral Agent contract used by the
// web API to turn user messages into assistant responses.  The production
// implementation lives in genkit_agent.go; tests swap it for fakes.
package chatagent

import "context"

// Message is a single turn in the conversation history.
type Message struct {
	Role    string
	Content string
}

// RunRequest bundles everything the agent needs to produce a response.
type RunRequest struct {
	SessionID       string
	ProjectPath     string
	ProjectName     string
	Provider        string
	Model           string
	UserMessage     string
	History         []Message
	HistoryLimit    int
	CorrelationID   string
	ToolEnvironment *ToolEnvironment
}

// RunResponse is what the agent returns to the orchestration layer.
type RunResponse struct {
	Text     string
	Metadata map[string]any
}

// Agent is the contract between the web API and any chat completion
// backend (Genkit, OpenAI-compatible HTTP, or a test fake).
type Agent interface {
	Run(ctx context.Context, req RunRequest) (RunResponse, error)
}
