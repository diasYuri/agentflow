package chatagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/diasYuri/agentflow/internal/daemon"
)

type chatCompletionRequest struct {
	Model               string                `json:"model"`
	Messages            []chatRequestMessage  `json:"messages"`
	Tools               []chatRequestToolSpec `json:"tools,omitempty"`
	Temperature         *float64              `json:"temperature,omitempty"`
	MaxCompletionTokens *int64                `json:"max_completion_tokens,omitempty"`
	TopP                *float64              `json:"top_p,omitempty"`
}

type chatRequestMessage struct {
	Role      string            `json:"role"`
	Content   any               `json:"content,omitempty"`
	ToolCalls []chatToolCall    `json:"tool_calls,omitempty"`
	ToolID    string            `json:"tool_call_id,omitempty"`
	Function  *chatToolFunction `json:"function,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type chatRequestToolSpec struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	} `json:"function"`
}

func TestNewGenkitAgentRequiresProviderAndModel(t *testing.T) {
	_, err := NewGenkitAgent("", "gpt-4", nil, 0)
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = NewGenkitAgent("openai", "", nil, 0)
	if err == nil {
		t.Fatal("expected error for missing model")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewGenkitAgentRequiresConfiguredProvider(t *testing.T) {
	_, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{}, 0)
	if err == nil {
		t.Fatal("expected error for unconfigured provider")
	}
	if !strings.Contains(err.Error(), `provider "openai" not configured`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewGenkitAgentRequiresAPIKey(t *testing.T) {
	_, err := NewGenkitAgent("custom", "gpt-4", map[string]ProviderConfig{
		"custom": {},
	}, 0)
	if err == nil {
		t.Fatal("expected error for missing api key")
	}
	if !strings.Contains(err.Error(), "requires api_key or api_key_env") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewGenkitAgentDefaultsOpenAIAPIKeyEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-default")
	agent, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{
		"openai": {},
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.config.APIKey != "sk-default" {
		t.Fatalf("expected default OpenAI api key env, got %q", agent.config.APIKey)
	}
}

func TestNewGenkitAgentReportsMissingAPIKeyEnv(t *testing.T) {
	_, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{
		"openai": {APIKeyEnv: "MISSING_OPENAI_KEY"},
	}, 0)
	if err == nil {
		t.Fatal("expected error for missing api key env")
	}
	if !strings.Contains(err.Error(), `api_key_env "MISSING_OPENAI_KEY" is not set`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewGenkitAgentResolvesAPIKeyFromEnv(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-test")
	agent, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{
		"openai": {APIKeyEnv: "TEST_OPENAI_KEY"},
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.config.APIKey != "sk-test" {
		t.Fatalf("expected api key from env, got %q", agent.config.APIKey)
	}
}

func TestNewGenkitAgentExplicitAPIKeyWinsOverEnv(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "sk-env")
	agent, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{
		"openai": {APIKey: "sk-explicit", APIKeyEnv: "TEST_OPENAI_KEY"},
	}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.config.APIKey != "sk-explicit" {
		t.Fatalf("expected explicit api key, got %q", agent.config.APIKey)
	}
}

func TestOpenAIToolNameSanitizesInvalidCharacters(t *testing.T) {
	got := openAIToolName("agentflow.list workflows!")
	if got != "agentflow_list_workflows" {
		t.Fatalf("unexpected sanitized name: %q", got)
	}
	if len(openAIToolName(strings.Repeat("a", 80))) > 64 {
		t.Fatalf("expected sanitized name to be capped at 64 chars")
	}
}

func TestNormalizeOpenAIBaseURLAppendsV1(t *testing.T) {
	tests := map[string]string{
		"https://api.openai.com":        "https://api.openai.com/v1",
		"http://localhost:11434/":       "http://localhost:11434/v1",
		"https://openrouter.ai/api/v1":  "https://openrouter.ai/api/v1",
		"https://example.com/custom/v1": "https://example.com/custom/v1",
	}
	for in, want := range tests {
		if got := normalizeOpenAIBaseURL(in); got != want {
			t.Fatalf("normalizeOpenAIBaseURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRunSendsGenkitOpenAICompatibleRequest(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("expected application/json, got %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeChatCompletion(w, "test-model", map[string]any{
			"role":    "assistant",
			"content": "Hello from test",
		})
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "test-model", map[string]ProviderConfig{
		"test": {BaseURL: server.URL, APIKey: "sk-test"},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}

	resp, err := agent.Run(context.Background(), RunRequest{
		UserMessage: "hi",
		History: []Message{
			{Role: "user", Content: "previous"},
			{Role: "assistant", Content: "ok"},
		},
		HistoryLimit: 10,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if resp.Text != "Hello from test" {
		t.Fatalf("unexpected response: %q", resp.Text)
	}
	if resp.Metadata["provider"] != "test" {
		t.Fatalf("unexpected provider metadata: %v", resp.Metadata)
	}
	if received.Model != "test-model" {
		t.Fatalf("model=%q", received.Model)
	}
	if len(received.Messages) != 4 {
		t.Fatalf("expected 4 messages (system + 2 history + user), got %d: %+v", len(received.Messages), received.Messages)
	}
	if received.Messages[0].Role != "system" || !strings.Contains(messageText(received.Messages[0]), "AgentFlow") {
		t.Fatalf("system message mismatch: %+v", received.Messages[0])
	}
	if received.Messages[1].Role != "user" || messageText(received.Messages[1]) != "previous" {
		t.Fatalf("history[0] mismatch: %+v", received.Messages[1])
	}
	if received.Messages[2].Role != "assistant" || messageText(received.Messages[2]) != "ok" {
		t.Fatalf("history[1] mismatch: %+v", received.Messages[2])
	}
	if received.Messages[3].Role != "user" || messageText(received.Messages[3]) != "hi" {
		t.Fatalf("user message mismatch: %+v", received.Messages[3])
	}
}

func TestRunAppliesGenerationConfig(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		writeChatCompletion(w, "m", map[string]any{"role": "assistant", "content": "ok"})
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "m", map[string]ProviderConfig{
		"test": {
			BaseURL:     server.URL,
			APIKey:      "k",
			Temperature: 0.5,
			MaxTokens:   100,
			TopP:        0.9,
		},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	_, err = agent.Run(context.Background(), RunRequest{UserMessage: "x"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if received.Temperature == nil || *received.Temperature != 0.5 {
		t.Fatalf("temperature mismatch: %v", received.Temperature)
	}
	if received.MaxCompletionTokens == nil || *received.MaxCompletionTokens != 100 {
		t.Fatalf("max_completion_tokens mismatch: %v", received.MaxCompletionTokens)
	}
	if received.TopP == nil || *received.TopP != 0.9 {
		t.Fatalf("top_p mismatch: %v", received.TopP)
	}
}

func TestRunRespectsHistoryLimit(t *testing.T) {
	var received chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		writeChatCompletion(w, "m", map[string]any{"role": "assistant", "content": "ok"})
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "m", map[string]ProviderConfig{
		"test": {BaseURL: server.URL, APIKey: "k"},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}

	history := make([]Message, 0, 50)
	for i := 0; i < 50; i++ {
		history = append(history, Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)})
	}

	_, err = agent.Run(context.Background(), RunRequest{
		UserMessage:  "latest",
		History:      history,
		HistoryLimit: 10,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(received.Messages) != 12 {
		t.Fatalf("expected 12 messages with limit 10, got %d", len(received.Messages))
	}
	if messageText(received.Messages[1]) != "msg-40" {
		t.Fatalf("expected first history msg-40, got %s", messageText(received.Messages[1]))
	}
}

func TestRunExecutesToolsUntilFinalAnswer(t *testing.T) {
	var (
		mu        sync.Mutex
		requests  []chatCompletionRequest
		callCount int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, received)
		callCount++
		current := callCount
		mu.Unlock()

		switch current {
		case 1:
			writeChatCompletion(w, "m", map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []map[string]any{{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "agentflow_list_workflows",
						"arguments": `{}`,
					},
				}},
			})
		case 2:
			writeChatCompletion(w, "m", map[string]any{"role": "assistant", "content": "All done"})
		default:
			t.Fatalf("unexpected request count %d", current)
		}
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "m", map[string]ProviderConfig{
		"test": {BaseURL: server.URL, APIKey: "k"},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}

	resp, err := agent.Run(context.Background(), RunRequest{
		UserMessage: "show workflows",
		ToolEnvironment: &ToolEnvironment{
			Definitions: &toolDefsFake{
				defs: []daemon.WorkflowDefinitionSummary{{ID: "wf-1", Name: "demo", Version: "1"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if resp.Text != "All done" {
		t.Fatalf("unexpected text: %q", resp.Text)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requests))
	}
	if len(requests[0].Tools) == 0 {
		t.Fatal("expected tool definitions in first request")
	}
	if !hasToolName(requests[0].Tools, "agentflow_list_workflows") {
		t.Fatalf("expected agentflow_list_workflows tool, got: %+v", requests[0].Tools)
	}
	if len(requests[1].Messages) == 0 || requests[1].Messages[len(requests[1].Messages)-1].Role != "tool" {
		t.Fatalf("expected tool message in second request: %+v", requests[1].Messages)
	}
}

func hasToolName(tools []chatRequestToolSpec, name string) bool {
	for _, tool := range tools {
		if tool.Function.Name == name {
			return true
		}
	}
	return false
}

func TestRunReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "m", map[string]ProviderConfig{
		"test": {BaseURL: server.URL, APIKey: "bad"},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	_, err = agent.Run(context.Background(), RunRequest{UserMessage: "x"})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") && !strings.Contains(err.Error(), "invalid key") {
		t.Fatalf("expected HTTP/model error, got: %v", err)
	}
}

func TestSystemPromptIncludesExpectedInstructions(t *testing.T) {
	p := SystemPrompt()
	checks := []string{
		"AgentFlow",
		"plain, non-technical language",
		"available tools",
		"Do not require the user to run CLI commands",
		"ask for confirmation",
		"slash commands",
		"explicit user intent",
		"which projects are available",
		"workflow-definition listing tool",
		"run-listing or run-inspection tools",
	}
	for _, want := range checks {
		if !strings.Contains(p, want) {
			t.Fatalf("system prompt missing %q", want)
		}
	}
}

type toolDefsFake struct {
	defs []daemon.WorkflowDefinitionSummary
}

func (f *toolDefsFake) ListWorkflowDefinitions(context.Context) (daemon.WorkflowDefinitionsResponse, error) {
	return daemon.WorkflowDefinitionsResponse{Definitions: f.defs}, nil
}

func (f *toolDefsFake) WorkflowDefinition(context.Context, string) (daemon.WorkflowDefinitionResponse, error) {
	return daemon.WorkflowDefinitionResponse{}, nil
}

// fakeAgent is a test double that returns canned responses.
type fakeAgent struct {
	resp RunResponse
	err  error
}

func (f *fakeAgent) Run(ctx context.Context, req RunRequest) (RunResponse, error) {
	return f.resp, f.err
}

func TestFakeAgentSatisfiesInterface(t *testing.T) {
	var _ Agent = (*fakeAgent)(nil)
}

func writeChatCompletion(w http.ResponseWriter, model string, message map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"created": 0,
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": "stop",
		}},
	})
}

func messageText(msg chatRequestMessage) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case []any:
		var b strings.Builder
		for _, part := range c {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := m["text"].(string); ok {
				b.WriteString(text)
			}
		}
		return b.String()
	default:
		return ""
	}
}
