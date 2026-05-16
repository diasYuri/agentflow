//go:build claude_integration

package claude

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/ports"
	"github.com/diasYuri/agentflow/internal/core/workflow"
)

func TestProviderRunsAgainstRealClaude(t *testing.T) {
	requireClaudeAvailable(t)

	provider := New(os.Getenv("CLAUDE_PATH"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := provider.Run(ctx, ports.AgentRequest{
		Prompt:  "Return JSON matching the schema. Use the literal string \"ok\" for the answer field.",
		Sandbox: workflow.SandboxSpec{Mode: "read-only"},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
			"required":             []any{"answer"},
			"additionalProperties": false,
		},
	})
	if err != nil {
		t.Fatalf("real claude run failed: %v", err)
	}

	if result.Text == "" {
		t.Fatalf("expected non-empty text, got %+v", result)
	}
	structured, ok := result.JSON.(map[string]any)
	if !ok {
		t.Fatalf("expected structured JSON result, got %#v", result.JSON)
	}
	if _, hasAnswer := structured["answer"].(string); !hasAnswer {
		t.Fatalf("expected answer field in structured output: %#v", structured)
	}
	if result.Usage == nil {
		t.Fatal("expected usage information from real run")
	}
	if result.Usage.InputTokens == 0 && result.Usage.OutputTokens == 0 {
		t.Fatalf("usage looks empty: %#v", result.Usage)
	}
}

func requireClaudeAvailable(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("CLAUDE_PATH")) != "" {
		return
	}
	if _, err := exec.LookPath("claude"); err == nil {
		return
	} else if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("look up claude: %v", err)
	}
	t.Skip("set CLAUDE_PATH or install claude to run claude integration tests")
}
