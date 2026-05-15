package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

const maxErrorOutputBytes = 4096

type Provider struct {
	claudePath string
}

func New(claudePath string) *Provider {
	return &Provider{claudePath: claudePath}
}

func (p *Provider) Run(ctx context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	args, err := buildArgs(req)
	if err != nil {
		return ports.AgentResult{}, err
	}

	cmd := exec.CommandContext(ctx, resolveClaudePath(p.claudePath), args...)
	cmd.Env = envMapToList(mergeClaudeEnv(req.Env))
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ports.AgentResult{}, fmt.Errorf("run claude: %w: stdout=%q stderr=%q", err, truncateOutput(stdout.String()), truncateOutput(stderr.String()))
	}

	result, err := parseClaudeResult(stdout.Bytes(), req.OutputSchema != nil)
	if err != nil {
		return ports.AgentResult{}, fmt.Errorf("parse claude output: %w: stdout=%q stderr=%q", err, truncateOutput(stdout.String()), truncateOutput(stderr.String()))
	}
	return result, nil
}

func buildArgs(req ports.AgentRequest) ([]string, error) {
	args := []string{"-p", "--output-format", "json"}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "--model", req.Model)
	}
	if strings.TrimSpace(req.System) != "" {
		args = append(args, "--append-system-prompt", req.System)
	}
	if req.OutputSchema != nil {
		schema, err := json.Marshal(req.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal output schema: %w", err)
		}
		args = append(args, "--json-schema", string(schema))
	}
	args = append(args, sandboxArgs(req.Sandbox.Mode)...)
	args = append(args, req.Prompt)
	return args, nil
}

func resolveClaudePath(override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	if envPath := strings.TrimSpace(os.Getenv("CLAUDE_PATH")); envPath != "" {
		return envPath
	}
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	return "claude"
}

func sandboxArgs(mode string) []string {
	switch strings.TrimSpace(mode) {
	case "read-only":
		return []string{"--tools", "Read,Glob,Grep,LS", "--allowedTools", "Read,Glob,Grep,LS"}
	case "workspace-write":
		return []string{"--allowedTools", "Read,Write,Edit,MultiEdit,Bash,Glob,Grep,LS"}
	default:
		return nil
	}
}

func mergeClaudeEnv(overrides map[string]string) map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	for key, value := range overrides {
		env[key] = value
	}
	return env
}

func envMapToList(env map[string]string) []string {
	items := make([]string, 0, len(env))
	for key, value := range env {
		items = append(items, key+"="+value)
	}
	sort.Strings(items)
	return items
}

func parseClaudeResult(data []byte, parseStructuredOutput bool) (ports.AgentResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ports.AgentResult{}, errors.New("empty output")
	}

	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return ports.AgentResult{}, err
	}

	text := extractText(payload)
	result := ports.AgentResult{
		Text: text,
		Metadata: map[string]any{
			"claude": payload,
		},
		RawEvents: []ports.AgentEvent{{
			Type: stringField(payload, "type", "result"),
			Data: payload,
		}},
	}
	if sessionID := stringField(payload, "session_id", "sessionID", "sessionId"); sessionID != "" {
		result.Metadata["session_id"] = sessionID
	}
	if usage := extractUsage(payload); usage != nil {
		result.Usage = usage
	}
	if parseStructuredOutput {
		if structuredOutput, ok := payload["structured_output"]; ok && structuredOutput != nil {
			result.JSON = structuredOutput
			return result, nil
		}
	}
	if parseStructuredOutput && text != "" {
		var parsed any
		if err := json.Unmarshal([]byte(text), &parsed); err == nil {
			result.JSON = parsed
		}
	}
	return result, nil
}

func extractText(payload map[string]any) string {
	for _, key := range []string{"result", "text", "message", "final_response", "finalResponse", "content"} {
		if text, ok := payload[key].(string); ok {
			return text
		}
	}
	if content, ok := payload["content"].([]any); ok {
		var parts []string
		for _, item := range content {
			switch value := item.(type) {
			case string:
				parts = append(parts, value)
			case map[string]any:
				if text, ok := value["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

func extractUsage(payload map[string]any) *ports.Usage {
	rawUsage, ok := payload["usage"].(map[string]any)
	if !ok {
		return nil
	}
	usage := &ports.Usage{
		InputTokens:  intField(rawUsage, "input_tokens", "inputTokens"),
		OutputTokens: intField(rawUsage, "output_tokens", "outputTokens"),
		TotalTokens:  intField(rawUsage, "total_tokens", "totalTokens"),
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	return usage
}

func stringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}

func intField(payload map[string]any, keys ...string) int {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case float64:
			return int(value)
		case int:
			return value
		case json.Number:
			number, _ := value.Int64()
			return int(number)
		}
	}
	return 0
}

func truncateOutput(value string) string {
	if len(value) <= maxErrorOutputBytes {
		return value
	}
	return value[:maxErrorOutputBytes] + "...(truncated)"
}
