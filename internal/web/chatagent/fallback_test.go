package chatagent

import (
	"context"
	"strings"
	"testing"
)

func TestFallbackAgentReturnsHelpfulResponse(t *testing.T) {
	agent := NewFallbackAgent("chat agent is not configured")
	resp, err := agent.Run(context.Background(), RunRequest{
		ProjectName: "demo",
		UserMessage: "Hello, AgentFlow!",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if resp.Metadata["provider"] != "fallback" {
		t.Fatalf("provider=%v", resp.Metadata["provider"])
	}
	if !strings.Contains(resp.Text, "Fallback chat mode is active for demo") {
		t.Fatalf("response did not mention fallback project: %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "Hello, AgentFlow!") {
		t.Fatalf("response did not echo the user message: %q", resp.Text)
	}
}
