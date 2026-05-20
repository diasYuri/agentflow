package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/web/api"
	"github.com/diasYuri/agentflow/internal/web/events"
	"github.com/diasYuri/agentflow/internal/web/persistence"
)

func TestProjectsListEndpoint(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodGet, "/api/v1/projects", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Projects []api.ProjectResponse `json:"projects"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Projects) != 1 || payload.Projects[0].Name != "demo" {
		t.Fatalf("unexpected projects: %+v", payload.Projects)
	}
}

func TestCreateProjectEndpoint(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects", map[string]any{
		"name": "new-project",
		"path": "/tmp/new-project",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doReq(t, mux, http.MethodGet, "/api/v1/projects/new-project", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get project: %d body=%s", rec.Code, rec.Body.String())
	}
	var got api.ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "new-project" || got.Path != "/tmp/new-project" {
		t.Fatalf("unexpected project: %+v", got)
	}
}

func TestPickProjectFolderEndpoint(t *testing.T) {
	svc, mux, _ := newTestService(t)
	svc.FolderPicker = staticFolderPicker("/tmp/picked-project")
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/pick-folder", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != "/tmp/picked-project" || got.Name != "picked-project" {
		t.Fatalf("unexpected picker response: %+v", got)
	}
}

func TestWorkflowDefinitionProxyCreatesFromYAML(t *testing.T) {
	svc, mux, _ := newTestService(t)
	client := &fakeWorkflowDefinitions{}
	svc.WorkflowDefinitions = client

	rec := doReq(t, mux, http.MethodPost, "/api/v1/workflow-definitions", map[string]any{
		"yaml": "name: demo-workflow\nnodes:\n  - id: start\n    kind: agent\n",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if client.created.Name != "demo-workflow" || len(client.created.Nodes) != 1 {
		t.Fatalf("unexpected spec: %+v", client.created)
	}

	rec = doReq(t, mux, http.MethodGet, "/api/v1/workflow-definitions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSessionLifecycleEndpoints(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", map[string]any{"title": "kickoff"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)
	if session.ProjectName != "demo" || session.ProjectPath != "/p" {
		t.Fatalf("unexpected snapshot: %+v", session)
	}
	// Append a message.
	rec = doReq(t, mux, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", map[string]any{
		"role":    "user",
		"content": "hello",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("append message: %d body=%s", rec.Code, rec.Body.String())
	}
	// List messages back.
	rec = doReq(t, mux, http.MethodGet, "/api/v1/sessions/"+session.ID+"/messages", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list messages: %d", rec.Code)
	}
	var msgPayload struct {
		Messages []persistence.Message `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &msgPayload); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(msgPayload.Messages) != 1 || msgPayload.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %+v", msgPayload.Messages)
	}
	// Patch title.
	rec = doReq(t, mux, http.MethodPatch, "/api/v1/sessions/"+session.ID, map[string]any{"title": "renamed"})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %d body=%s", rec.Code, rec.Body.String())
	}
	// Archive.
	rec = doReq(t, mux, http.MethodPatch, "/api/v1/sessions/"+session.ID, map[string]any{"status": "archived"})
	if rec.Code != http.StatusOK {
		t.Fatalf("archive: %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doReq(t, mux, http.MethodGet, "/api/v1/sessions/"+session.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get after archive: %d", rec.Code)
	}
	got := decodeSession(t, rec)
	if got.Status != persistence.SessionStatusArchived {
		t.Fatalf("expected archived, got %q", got.Status)
	}
	// Delete.
	rec = doReq(t, mux, http.MethodDelete, "/api/v1/sessions/"+session.ID, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	rec = doReq(t, mux, http.MethodGet, "/api/v1/sessions/"+session.ID, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rec.Code)
	}
}

func TestProjectScopedSessionsReturnsOnlyProjectSessions(t *testing.T) {
	_, mux, _ := newTestService(t)
	doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", map[string]any{"title": "first"})
	rec := doReq(t, mux, http.MethodGet, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var payload struct {
		Sessions []persistence.Session `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Sessions) != 1 || payload.Sessions[0].Title != "first" {
		t.Fatalf("unexpected sessions: %+v", payload.Sessions)
	}
}

func TestRecordDiagnosticAppliesRedaction(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, "/api/v1/sessions/"+session.ID+"/diagnostics", map[string]any{
		"level":   "error",
		"source":  "server",
		"message": "boom",
		"context": map[string]any{"api_key": "sk-abcdef1234567890"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("diag: %d body=%s", rec.Code, rec.Body.String())
	}
	var diag persistence.Diagnostic
	if err := json.Unmarshal(rec.Body.Bytes(), &diag); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if diag.Context["api_key"] != "[redacted]" {
		t.Fatalf("expected api_key redacted, got %v", diag.Context)
	}
}

func TestSSEStreamDeliversPublishedEvents(t *testing.T) {
	_, mux, broker := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", map[string]any{})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)

	w := newCapturingWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+session.ID+"/stream", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(w, req)
		close(done)
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Publish(session.ID, events.KindMessage, map[string]string{"text": "hi"}, "corr")
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(w.Bytes(), []byte("event: message")) {
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf("did not receive event frame, got: %q", w.Bytes())
}

// capturingWriter is a goroutine-safe http.ResponseWriter that buffers
// every Write so SSE tests can inspect output without binding a real
// TCP socket.
type capturingWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	header http.Header
	status int
}

type staticFolderPicker string

func (s staticFolderPicker) PickFolder(*http.Request) (string, error) {
	return string(s), nil
}

type fakeWorkflowDefinitions struct {
	created workflow.WorkflowSpec
}

func (f *fakeWorkflowDefinitions) ListWorkflowDefinitions(context.Context) (daemon.WorkflowDefinitionsResponse, error) {
	return daemon.WorkflowDefinitionsResponse{Definitions: []daemon.WorkflowDefinitionSummary{
		{ID: "wf-1", Name: "demo-workflow", Version: "1"},
	}}, nil
}

func (f *fakeWorkflowDefinitions) CreateWorkflowDefinition(_ context.Context, spec workflow.WorkflowSpec) (daemon.WorkflowDefinitionResponse, error) {
	f.created = spec
	return daemon.WorkflowDefinitionResponse{
		WorkflowDefinition: daemon.WorkflowDefinition{
			ID:   "wf-1",
			Name: spec.Name,
			Spec: spec,
		},
	}, nil
}

func (f *fakeWorkflowDefinitions) WorkflowDefinition(context.Context, string) (daemon.WorkflowDefinitionResponse, error) {
	return daemon.WorkflowDefinitionResponse{}, nil
}

func (f *fakeWorkflowDefinitions) UpdateWorkflowDefinition(_ context.Context, _ string, spec workflow.WorkflowSpec) (daemon.WorkflowDefinitionResponse, error) {
	return daemon.WorkflowDefinitionResponse{WorkflowDefinition: daemon.WorkflowDefinition{ID: "wf-1", Name: spec.Name, Spec: spec}}, nil
}

func (f *fakeWorkflowDefinitions) DeleteWorkflowDefinition(context.Context, string) error {
	return nil
}

func newCapturingWriter() *capturingWriter {
	return &capturingWriter{header: make(http.Header)}
}

func (c *capturingWriter) Header() http.Header { return c.header }

func (c *capturingWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *capturingWriter) WriteHeader(status int) { c.status = status }

func (c *capturingWriter) Flush() {}

func (c *capturingWriter) Bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]byte, c.buf.Len())
	copy(out, c.buf.Bytes())
	return out
}

func TestDebugBundleEndpointReturnsZip(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d", rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodGet, "/api/v1/sessions/"+session.ID+"/debug-bundle", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("unexpected content type: %q", rec.Header().Get("Content-Type"))
	}
	if rec.Body.Len() < 50 {
		t.Fatalf("bundle too small: %d bytes", rec.Body.Len())
	}
}
func TestToolCallLifecycleEndpointRoundTrip(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d", rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, "/api/v1/sessions/"+session.ID+"/tool-calls", map[string]any{"name": "shell.exec"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create tool call: %d body=%s", rec.Code, rec.Body.String())
	}
	var call persistence.ToolCall
	if err := json.Unmarshal(rec.Body.Bytes(), &call); err != nil {
		t.Fatalf("decode tool call: %v", err)
	}
	if call.Name != "shell.exec" || call.Status != persistence.ToolCallStatusPending {
		t.Fatalf("unexpected tool call: %+v", call)
	}
	rec = doReq(t, mux, http.MethodGet, "/api/v1/sessions/"+session.ID+"/tool-calls", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list tool calls: %d", rec.Code)
	}
	var listPayload struct {
		ToolCalls []persistence.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(listPayload.ToolCalls))
	}
	rec = doReq(t, mux, http.MethodPatch, "/api/v1/tool-calls/"+call.ID, map[string]any{"status": "succeeded"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("patch tool call: %d body=%s", rec.Code, rec.Body.String())
	}
}
