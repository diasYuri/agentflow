package chatagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const defaultHTTPTimeout = 60 * time.Second
const maxToolTurns = 8

// ProviderConfig holds the connection and generation settings for one
// OpenAI-compatible backend.
type ProviderConfig struct {
	BaseURL     string
	APIKey      string
	APIKeyEnv   string
	Headers     map[string]string
	Temperature float64
	MaxTokens   int
	TopP        float64
}

// GenkitAgent is a production chat agent backed by Genkit's
// OpenAI-compatible provider plugin.
type GenkitAgent struct {
	provider string
	model    string
	config   ProviderConfig
	genkit   *genkit.Genkit
}

// NewGenkitAgent builds an agent for the selected provider and model.
// The provider map is keyed by provider name (e.g. "openai", "ollama").
func NewGenkitAgent(provider, model string, providers map[string]ProviderConfig, timeout time.Duration) (*GenkitAgent, error) {
	if provider == "" {
		return nil, errors.New("chatagent: provider is required")
	}
	if model == "" {
		return nil, errors.New("chatagent: model is required")
	}
	cfg, ok := providers[provider]
	if !ok {
		return nil, fmt.Errorf("chatagent: provider %q not configured", provider)
	}

	key, err := resolveAPIKey(provider, cfg)
	if err != nil {
		return nil, err
	}
	cfg.APIKey = key

	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	opts := []option.RequestOption{option.WithRequestTimeout(timeout)}
	for k, v := range cfg.Headers {
		opts = append(opts, option.WithHeader(k, v))
	}

	g := genkit.Init(context.Background(), genkit.WithPlugins(&compat_oai.OpenAICompatible{
		Provider: provider,
		APIKey:   cfg.APIKey,
		BaseURL:  normalizeOpenAIBaseURL(cfg.BaseURL),
		Opts:     opts,
	}))

	return &GenkitAgent{
		provider: provider,
		model:    model,
		config:   cfg,
		genkit:   g,
	}, nil
}

func resolveAPIKey(provider string, cfg ProviderConfig) (string, error) {
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		return key, nil
	}

	envName := strings.TrimSpace(cfg.APIKeyEnv)
	if envName == "" {
		envName = defaultAPIKeyEnv(provider)
	}
	if envName == "" {
		return "", fmt.Errorf("chatagent: provider %q requires api_key or api_key_env", provider)
	}

	key, ok := os.LookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("chatagent: provider %q api_key_env %q is not set", provider, envName)
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("chatagent: provider %q api_key_env %q is empty", provider, envName)
	}
	return strings.TrimSpace(key), nil
}

func defaultAPIKeyEnv(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		return "OPENAI_API_KEY"
	default:
		return ""
	}
}

// Run sends the system prompt, history, user message, and available tools to
// Genkit. Genkit owns the model request and tool-calling loop.
func (g *GenkitAgent) Run(ctx context.Context, req RunRequest) (RunResponse, error) {
	messages := buildGenkitMessages(req)
	tools := buildGenkitTools(req.ToolEnvironment)

	opts := []ai.GenerateOption{
		ai.WithModelName(g.provider + "/" + g.model),
		ai.WithSystem(SystemPrompt()),
		ai.WithMessages(messages...),
		ai.WithPrompt(req.UserMessage),
		ai.WithMaxTurns(maxToolTurns),
	}
	if cfg := g.generationConfig(); cfg != nil {
		opts = append(opts, ai.WithConfig(cfg))
	}
	if len(tools) > 0 {
		opts = append(opts, ai.WithTools(tools...))
	}

	resp, err := genkit.Generate(ctx, g.genkit, opts...)
	if err != nil {
		return RunResponse{}, fmt.Errorf("chatagent: generate: %w", err)
	}

	return RunResponse{
		Text: resp.Text(),
		Metadata: map[string]any{
			"provider": g.provider,
			"model":    g.model,
		},
	}, nil
}

func (g *GenkitAgent) generationConfig() *openai.ChatCompletionNewParams {
	cfg := openai.ChatCompletionNewParams{}
	configured := false
	if g.config.Temperature > 0 {
		cfg.Temperature = openai.Float(g.config.Temperature)
		configured = true
	}
	if g.config.MaxTokens > 0 {
		cfg.MaxCompletionTokens = openai.Int(int64(g.config.MaxTokens))
		configured = true
	}
	if g.config.TopP > 0 {
		cfg.TopP = openai.Float(g.config.TopP)
		configured = true
	}
	if !configured {
		return nil
	}
	return &cfg
}

func buildGenkitMessages(req RunRequest) []*ai.Message {
	limit := req.HistoryLimit
	if limit <= 0 {
		limit = 40
	}
	start := 0
	if len(req.History) > limit {
		start = len(req.History) - limit
	}

	messages := make([]*ai.Message, 0, len(req.History[start:]))
	for _, h := range req.History[start:] {
		switch h.Role {
		case "user":
			messages = append(messages, ai.NewUserTextMessage(h.Content))
		case "assistant", "model":
			messages = append(messages, ai.NewModelTextMessage(h.Content))
		case "system":
			messages = append(messages, ai.NewSystemTextMessage(h.Content))
		}
	}
	return messages
}

func buildGenkitTools(env *ToolEnvironment) []ai.ToolRef {
	if env == nil {
		return nil
	}
	tools := BuildTools(env)
	refs := make([]ai.ToolRef, 0, len(tools))
	for _, tool := range tools {
		t := tool
		apiName := openAIToolName(t.Name)
		refs = append(refs, ai.NewTool(apiName, t.Description, func(ctx *ai.ToolContext, input any) (any, error) {
			raw, err := json.Marshal(input)
			if err != nil {
				return nil, fmt.Errorf("marshal tool input %s: %w", apiName, err)
			}
			return t.Invoke(ctx, json.RawMessage(raw))
		}, ai.WithInputSchema(t.Parameters)))
	}
	return refs
}

func openAIToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tool"
	}

	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	out := strings.Trim(b.String(), "_-")
	if out == "" {
		out = "tool"
	}
	if len(out) > 64 {
		out = out[:64]
		out = strings.TrimRight(out, "_-")
		if out == "" {
			out = "tool"
		}
	}
	return out
}

func normalizeOpenAIBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return raw
	}
	if strings.HasSuffix(u.Path, "/v1") {
		return raw
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/v1"
	return u.String()
}
