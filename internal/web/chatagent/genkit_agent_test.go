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
	_, err := NewGenkitAgent("openai", "gpt-4", map[string]ProviderConfig{
		"openai": {},
	}, 0)
	if err == nil {
		t.Fatal("expected error for missing api key")
	}
	if !strings.Contains(err.Error(), "requires api_key or api_key_env") {
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

func TestRunSendsOpenAICompatibleRequest(t *testing.T) {
	var received oaiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %s", auth)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		resp := oaiResponse{
			Choices: []struct {
				Message      oaiMessage `json:"message"`
				FinishReason string     `json:"finish_reason,omitempty"`
			}{
				{Message: oaiMessage{Role: "assistant", Content: "Hello from test"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
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
		t.Fatalf("expected 4 messages (system + 2 history + user), got %d", len(received.Messages))
	}
	if received.Messages[0].Role != "system" {
		t.Fatalf("first message must be system, got %s", received.Messages[0].Role)
	}
	if !strings.Contains(received.Messages[0].Content, "AgentFlow") {
		t.Fatalf("system prompt missing AgentFlow: %s", received.Messages[0].Content)
	}
	if received.Messages[1].Role != "user" || received.Messages[1].Content != "previous" {
		t.Fatalf("history[0] mismatch: %+v", received.Messages[1])
	}
	if received.Messages[2].Role != "assistant" || received.Messages[2].Content != "ok" {
		t.Fatalf("history[1] mismatch: %+v", received.Messages[2])
	}
	if received.Messages[3].Role != "user" || received.Messages[3].Content != "hi" {
		t.Fatalf("user message mismatch: %+v", received.Messages[3])
	}
}

func TestRunAppliesGenerationConfig(t *testing.T) {
	var received oaiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		resp := oaiResponse{
			Choices: []struct {
				Message      oaiMessage `json:"message"`
				FinishReason string     `json:"finish_reason,omitempty"`
			}{{Message: oaiMessage{Role: "assistant", Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
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
	if received.MaxTokens == nil || *received.MaxTokens != 100 {
		t.Fatalf("max_tokens mismatch: %v", received.MaxTokens)
	}
	if received.TopP == nil || *received.TopP != 0.9 {
		t.Fatalf("top_p mismatch: %v", received.TopP)
	}
}

func TestRunRespectsHistoryLimit(t *testing.T) {
	var received oaiRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		resp := oaiResponse{
			Choices: []struct {
				Message      oaiMessage `json:"message"`
				FinishReason string     `json:"finish_reason,omitempty"`
			}{{Message: oaiMessage{Role: "assistant", Content: "ok"}}},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
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
	if received.Messages[1].Content != "msg-40" {
		t.Fatalf("expected first history msg-40, got %s", received.Messages[1].Content)
	}
}

func TestRunExecutesToolsUntilFinalAnswer(t *testing.T) {
	var (
		mu        sync.Mutex
		requests  []oaiRequest
		callCount int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var received oaiRequest
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		mu.Lock()
		requests = append(requests, received)
		callCount++
		current := callCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch current {
		case 1:
			resp := oaiResponse{
				Choices: []struct {
					Message      oaiMessage `json:"message"`
					FinishReason string     `json:"finish_reason,omitempty"`
				}{
					{Message: oaiMessage{
						Role: "assistant",
						ToolCalls: []oaiToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: oaiToolFunction{
								Name:      "agentflow.list_workflows",
								Arguments: `{"include_runs":false}`,
							},
						}},
					}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		case 2:
			resp := oaiResponse{
				Choices: []struct {
					Message      oaiMessage `json:"message"`
					FinishReason string     `json:"finish_reason,omitempty"`
				}{
					{Message: oaiMessage{Role: "assistant", Content: "All done"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
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
	if requests[0].Tools[0].Function.Name != "agentflow.list_workflows" {
		t.Fatalf("unexpected tool name: %s", requests[0].Tools[0].Function.Name)
	}
	if len(requests[1].Messages) == 0 || requests[1].Messages[len(requests[1].Messages)-1].Role != "tool" {
		t.Fatalf("expected tool message in second request: %+v", requests[1].Messages)
	}
}

func TestRunReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
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
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 in error, got: %v", err)
	}
}

func TestRunReturnsErrorOnModelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := oaiResponse{
			Error: &struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			}{Message: "context length exceeded", Type: "invalid_request_error"},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	agent, err := NewGenkitAgent("test", "m", map[string]ProviderConfig{
		"test": {BaseURL: server.URL, APIKey: "k"},
	}, 0)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	_, err = agent.Run(context.Background(), RunRequest{UserMessage: "x"})
	if err == nil {
		t.Fatal("expected error for model error")
	}
	if !strings.Contains(err.Error(), "context length exceeded") {
		t.Fatalf("expected model error message, got: %v", err)
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
