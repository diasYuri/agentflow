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
	cmd.Stdin = strings.NewReader(req.Prompt)

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
	args := []string{"-p", "--output-format", "json", "--no-session-persistence"}
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
		return []string{"--tools", "Read,Glob,Grep,LS"}
	case "workspace-write":
		return []string{"--permission-mode", "acceptEdits"}
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

	if isErr, _ := payload["is_error"].(bool); isErr {
		return ports.AgentResult{}, claudeError(payload)
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
	if denials, ok := payload["permission_denials"].([]any); ok && len(denials) > 0 {
		result.Metadata["permission_denials"] = denials
	}
	if cost, ok := numberField(payload, "total_cost_usd", "totalCostUsd"); ok {
		result.Metadata["claude_cost_usd"] = cost
	}
	if usage, raw := extractUsage(payload); usage != nil {
		result.Usage = usage
		if raw != nil {
			result.Metadata["claude_usage"] = raw
		}
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

func claudeError(payload map[string]any) error {
	subtype := stringField(payload, "subtype")
	text := extractText(payload)
	parts := []string{"claude reported error"}
	if subtype != "" {
		parts = append(parts, "subtype="+subtype)
	}
	if text != "" {
		parts = append(parts, "result="+truncateOutput(text))
	}
	if denials, ok := payload["permission_denials"].([]any); ok && len(denials) > 0 {
		parts = append(parts, fmt.Sprintf("permission_denials=%d", len(denials)))
	}
	return errors.New(strings.Join(parts, " "))
}

func extractText(payload map[string]any) string {
	for _, key := range []string{"result", "text", "message", "final_response", "finalResponse"} {
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

func extractUsage(payload map[string]any) (*ports.Usage, map[string]any) {
	rawUsage, ok := payload["usage"].(map[string]any)
	if !ok {
		return nil, nil
	}
	input := intField(rawUsage, "input_tokens", "inputTokens")
	output := intField(rawUsage, "output_tokens", "outputTokens")
	cacheRead := intField(rawUsage, "cache_read_input_tokens", "cacheReadInputTokens")
	cacheCreate := intField(rawUsage, "cache_creation_input_tokens", "cacheCreationInputTokens")
	total := intField(rawUsage, "total_tokens", "totalTokens")
	if total == 0 {
		total = input + cacheRead + cacheCreate + output
	}
	if input == 0 && output == 0 && cacheRead == 0 && cacheCreate == 0 && total == 0 {
		return nil, nil
	}
	usage := &ports.Usage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
	}
	return usage, rawUsage
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

func numberField(payload map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := payload[key].(type) {
		case float64:
			return value, true
		case int:
			return float64(value), true
		case json.Number:
			number, err := value.Float64()
			if err == nil {
				return number, true
			}
		}
	}
	return 0, false
}

func truncateOutput(value string) string {
	if len(value) <= maxErrorOutputBytes {
		return value
	}
	return value[:maxErrorOutputBytes] + "...(truncated)"
}
