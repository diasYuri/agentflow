package chatagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
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

// GenkitAgent is a production chat agent that calls OpenAI-compatible
// chat completion APIs over HTTP. The name retains the Genkit intent
// from the plan; the implementation uses standard net/http so it works
// while the Go Genkit SDK is not yet available on the module proxy.
type GenkitAgent struct {
	provider   string
	model      string
	config     ProviderConfig
	httpClient *http.Client
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

	key := cfg.APIKey
	if key == "" && cfg.APIKeyEnv != "" {
		if v, ok := os.LookupEnv(cfg.APIKeyEnv); ok {
			key = v
		}
	}
	if key == "" {
		return nil, fmt.Errorf("chatagent: provider %q requires api_key or api_key_env", provider)
	}
	cfg.APIKey = key

	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	return &GenkitAgent{
		provider:   provider,
		model:      model,
		config:     cfg,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type oaiToolSpec struct {
	Type     string            `json:"type"`
	Function oaiToolDefinition `json:"function"`
}

type oaiToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// oaiRequest is the JSON body for a chat completion call.
type oaiRequest struct {
	Model       string        `json:"model"`
	Messages    []oaiMessage  `json:"messages"`
	Tools       []oaiToolSpec `json:"tools,omitempty"`
	ToolChoice  any           `json:"tool_choice,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
}

// oaiResponse mirrors the essential fields of an OpenAI chat completion.
type oaiResponse struct {
	Choices []struct {
		Message      oaiMessage `json:"message"`
		FinishReason string     `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// Run sends the system prompt, history, and user message to the model and
// returns the assistant text. When the model emits tool calls, the agent
// executes them locally and continues the request/response loop until the
// model returns a final assistant message.
func (g *GenkitAgent) Run(ctx context.Context, req RunRequest) (RunResponse, error) {
	messages, err := buildConversation(req)
	if err != nil {
		return RunResponse{}, err
	}
	tools, toolIndex := buildToolSpecs(req.ToolEnvironment)

	for turn := 0; turn < maxToolTurns; turn++ {
		resp, err := g.complete(ctx, messages, tools)
		if err != nil {
			return RunResponse{}, err
		}
		if len(resp.Choices) == 0 {
			return RunResponse{}, errors.New("chatagent: empty choices from model")
		}

		msg := resp.Choices[0].Message
		if len(msg.ToolCalls) == 0 {
			return RunResponse{
				Text: msg.Content,
				Metadata: map[string]any{
					"provider": g.provider,
					"model":    g.model,
				},
			}, nil
		}

		messages = append(messages, msg)
		for _, call := range msg.ToolCalls {
			tool, ok := toolIndex[call.Function.Name]
			if !ok {
				return RunResponse{}, fmt.Errorf("chatagent: tool %q is not available", call.Function.Name)
			}
			result, err := tool.Invoke(ctx, json.RawMessage(call.Function.Arguments))
			if err != nil {
				return RunResponse{}, fmt.Errorf("tool %s: %w", call.Function.Name, err)
			}
			payload, err := json.Marshal(result)
			if err != nil {
				return RunResponse{}, fmt.Errorf("marshal tool result %s: %w", call.Function.Name, err)
			}
			messages = append(messages, oaiMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    string(payload),
			})
		}
	}

	return RunResponse{}, fmt.Errorf("chatagent: exceeded %d tool turns", maxToolTurns)
}

func buildConversation(req RunRequest) ([]oaiMessage, error) {
	msgs := make([]oaiMessage, 0, len(req.History)+2)
	msgs = append(msgs, oaiMessage{Role: "system", Content: SystemPrompt()})

	limit := req.HistoryLimit
	if limit <= 0 {
		limit = 40
	}
	start := 0
	if len(req.History) > limit {
		start = len(req.History) - limit
	}
	for _, h := range req.History[start:] {
		msgs = append(msgs, oaiMessage{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, oaiMessage{Role: "user", Content: req.UserMessage})
	return msgs, nil
}

func buildToolSpecs(env *ToolEnvironment) ([]oaiToolSpec, map[string]Tool) {
	if env == nil {
		return nil, nil
	}
	tools := BuildTools(env)
	specs := make([]oaiToolSpec, 0, len(tools))
	index := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		specs = append(specs, oaiToolSpec{
			Type: "function",
			Function: oaiToolDefinition{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
		index[tool.Name] = tool
	}
	return specs, index
}

func (g *GenkitAgent) complete(ctx context.Context, messages []oaiMessage, tools []oaiToolSpec) (oaiResponse, error) {
	oaiReq := oaiRequest{Model: g.model, Messages: messages}
	if len(tools) > 0 {
		oaiReq.Tools = tools
		oaiReq.ToolChoice = "auto"
	}
	if g.config.Temperature > 0 {
		oaiReq.Temperature = &g.config.Temperature
	}
	if g.config.MaxTokens > 0 {
		oaiReq.MaxTokens = &g.config.MaxTokens
	}
	if g.config.TopP > 0 {
		oaiReq.TopP = &g.config.TopP
	}

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return oaiResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	baseURL := g.config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	url := baseURL + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return oaiResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+g.config.APIKey)
	for k, v := range g.config.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := g.httpClient.Do(httpReq)
	if err != nil {
		return oaiResponse{}, fmt.Errorf("http post: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return oaiResponse{}, fmt.Errorf("read response: %w", err)
	}
	if httpResp.StatusCode >= 400 {
		return oaiResponse{}, fmt.Errorf("chat completion failed (%d): %s", httpResp.StatusCode, string(respBody))
	}

	var resp oaiResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return oaiResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return oaiResponse{}, fmt.Errorf("model error (%s): %s", resp.Error.Type, resp.Error.Message)
	}
	return resp, nil
}
