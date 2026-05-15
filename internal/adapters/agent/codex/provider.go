package codex

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	codexsdk "github.com/diasYuri/go-codex-sdk"

	"github.com/diasYuri/agentflow/internal/core/ports"
)

type Provider struct {
	codexPath string
}

func New(codexPath string) *Provider {
	return &Provider{codexPath: codexPath}
}

func (p *Provider) Run(ctx context.Context, req ports.AgentRequest) (ports.AgentResult, error) {
	client, err := codexsdk.New(&codexsdk.CodexOptions{
		CodexPathOverride: firstNonEmpty(p.codexPath, os.Getenv("CODEX_PATH")),
		APIKey:            os.Getenv("OPENAI_API_KEY"),
		Env:               mergeCodexEnv(req.Env),
	})
	if err != nil {
		return ports.AgentResult{}, err
	}
	thread := client.StartThread(&codexsdk.ThreadOptions{
		Model:            firstNonEmpty(req.Model, os.Getenv("CODEX_MODEL")),
		SandboxMode:      normalizeSandboxMode(firstNonEmpty(req.Sandbox.Mode, os.Getenv("CODEX_SANDBOX"))),
		ApprovalPolicy:   codexsdk.ApprovalModeNever,
		WorkingDirectory: req.WorkingDir,
		SkipGitRepoCheck: true,
	})
	prompt := req.Prompt
	if strings.TrimSpace(req.System) != "" {
		prompt = "System:\n" + strings.TrimSpace(req.System) + "\n\nUser:\n" + prompt
	}
	turn, err := thread.Run(ctx, codexsdk.TextInput(prompt), &codexsdk.TurnOptions{OutputSchema: req.OutputSchema})
	if err != nil {
		return ports.AgentResult{}, err
	}
	result := ports.AgentResult{
		Text:     turn.FinalResponse,
		Metadata: map[string]any{},
	}
	if turn.Usage != nil {
		result.Usage = &ports.Usage{
			InputTokens:  turn.Usage.InputTokens,
			OutputTokens: turn.Usage.OutputTokens,
			TotalTokens:  turn.Usage.InputTokens + turn.Usage.OutputTokens,
		}
	}
	if req.OutputSchema != nil && turn.FinalResponse != "" {
		var parsed any
		if err := json.Unmarshal([]byte(turn.FinalResponse), &parsed); err == nil {
			result.JSON = parsed
		}
	}
	for _, item := range turn.Items {
		data, _ := json.Marshal(item)
		var asMap map[string]any
		_ = json.Unmarshal(data, &asMap)
		result.RawEvents = append(result.RawEvents, ports.AgentEvent{Type: item.GetType(), Data: asMap})
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeSandboxMode(value string) codexsdk.SandboxMode {
	switch strings.TrimSpace(value) {
	case "":
		return ""
	case string(codexsdk.SandboxModeReadOnly):
		return codexsdk.SandboxModeReadOnly
	case string(codexsdk.SandboxModeWorkspaceWrite), "seatbelt":
		return codexsdk.SandboxModeWorkspaceWrite
	case string(codexsdk.SandboxModeDangerFullAccess):
		return codexsdk.SandboxModeDangerFullAccess
	default:
		return ""
	}
}

func mergeCodexEnv(overrides map[string]string) map[string]string {
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
