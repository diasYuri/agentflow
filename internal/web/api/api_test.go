package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/chatagent"
	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	corerun "github.com/diasYuri/agentflow/internal/core/run"
	"github.com/diasYuri/agentflow/internal/core/workflow"
	"github.com/diasYuri/agentflow/internal/daemon"
	"github.com/diasYuri/agentflow/internal/web/api"
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

func TestWorkflowRunProxyListsAndInspectsRuns(t *testing.T) {
	svc, mux, _ := newTestService(t)
	client := newFakeWorkflowRuns()
	svc.WorkflowRuns = client

	rec := doReq(t, mux, http.MethodGet, "/api/v1/workflows", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list daemon.ListWorkflowsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Runs) != 1 || list.Runs[0].ID != "run-1" {
		t.Fatalf("unexpected runs: %+v", list.Runs)
	}

	rec = doReq(t, mux, http.MethodGet, "/api/v1/workflows/run-1/inspect", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inspect daemon.WorkflowInspectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &inspect); err != nil {
		t.Fatalf("decode inspect: %v", err)
	}
	if inspect.RunID != "run-1" || inspect.AgentCalls != 2 {
		t.Fatalf("unexpected inspect: %+v", inspect)
	}
}

func TestWorkflowRunProxyStartsRun(t *testing.T) {
	svc, mux, _ := newTestService(t)
	client := newFakeWorkflowRuns()
	svc.WorkflowRuns = client

	rec := doReq(t, mux, http.MethodPost, "/api/v1/workflows", map[string]any{
		"workflow_ref": "build",
		"inputs":       map[string]any{"query": "ship"},
		"tag":          "web",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("run status=%d body=%s", rec.Code, rec.Body.String())
	}
	if client.lastAction != "run:build" {
		t.Fatalf("unexpected action %q", client.lastAction)
	}
	var got daemon.RunWorkflowResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if got.Run.ID != "run-new" || got.Run.Workflow != "build" {
		t.Fatalf("unexpected run: %+v", got.Run)
	}
}

func TestWorkflowRunProxyActions(t *testing.T) {
	svc, mux, _ := newTestService(t)
	client := newFakeWorkflowRuns()
	svc.WorkflowRuns = client

	rec := doReq(t, mux, http.MethodPost, "/api/v1/workflows/run-1/pause", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status=%d body=%s", rec.Code, rec.Body.String())
	}
	if client.lastAction != "pause:run-1" {
		t.Fatalf("unexpected action %q", client.lastAction)
	}

	rec = doReq(t, mux, http.MethodPost, "/api/v1/workflows/run-1/approve", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", rec.Code, rec.Body.String())
	}
	if client.lastAction != "approve:run-1" {
		t.Fatalf("unexpected action %q", client.lastAction)
	}
}

func TestWorkflowRunProxyRequiresDaemonClient(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodGet, "/api/v1/workflows", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
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

type fakeWorkflowRuns struct {
	run        daemon.WorkflowRun
	lastAction string
}

func newFakeWorkflowRuns() *fakeWorkflowRuns {
	startedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	return &fakeWorkflowRuns{
		run: daemon.WorkflowRun{
			ID:             "run-1",
			Workflow:       "demo-workflow",
			Status:         corerun.RunRunning,
			StartedAt:      startedAt,
			CurrentStep:    "build",
			CompletedSteps: []string{"plan"},
			PendingSteps:   []string{"test"},
			TotalSteps:     3,
			Tag:            "release",
		},
	}
}

func (f *fakeWorkflowRuns) RunWorkflow(_ context.Context, req daemon.RunWorkflowRequest) (daemon.RunWorkflowResponse, error) {
	run := daemon.WorkflowRun{
		ID:        "run-new",
		Workflow:  req.WorkflowRef,
		Status:    corerun.RunRunning,
		StartedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	}
	f.lastAction = "run:" + req.WorkflowRef
	f.run = run
	return daemon.RunWorkflowResponse{Run: run}, nil
}

func (f *fakeWorkflowRuns) ListWorkflows(context.Context) (daemon.ListWorkflowsResponse, error) {
	return daemon.ListWorkflowsResponse{Runs: []daemon.WorkflowRun{f.run}}, nil
}

func (f *fakeWorkflowRuns) WorkflowStatus(context.Context, string) (daemon.RunWorkflowResponse, error) {
	return daemon.RunWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) WorkflowEvents(context.Context, string, int, int) (daemon.WorkflowEventsResponse, error) {
	return daemon.WorkflowEventsResponse{RunID: f.run.ID}, nil
}

func (f *fakeWorkflowRuns) CancelWorkflow(_ context.Context, runID string) (daemon.CancelWorkflowResponse, error) {
	f.lastAction = "cancel:" + runID
	f.run.Status = corerun.RunCancelled
	return daemon.CancelWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) PauseWorkflow(_ context.Context, runID string) (daemon.PauseWorkflowResponse, error) {
	f.lastAction = "pause:" + runID
	f.run.Status = corerun.RunPaused
	return daemon.PauseWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) ResumeWorkflow(_ context.Context, runID string) (daemon.ResumeWorkflowResponse, error) {
	f.lastAction = "resume:" + runID
	f.run.Status = corerun.RunRunning
	return daemon.ResumeWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) ApproveWorkflow(_ context.Context, runID string) (daemon.ApproveWorkflowResponse, error) {
	f.lastAction = "approve:" + runID
	f.run.Status = corerun.RunRunning
	return daemon.ApproveWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) RejectWorkflow(_ context.Context, runID string) (daemon.RejectWorkflowResponse, error) {
	f.lastAction = "reject:" + runID
	f.run.Status = corerun.RunCancelled
	return daemon.RejectWorkflowResponse{Run: f.run}, nil
}

func (f *fakeWorkflowRuns) WorkflowArtifacts(context.Context, string) (daemon.WorkflowArtifactsResponse, error) {
	return daemon.WorkflowArtifactsResponse{
		RunID: f.run.ID,
		Artifacts: []daemon.WorkflowArtifactDTO{
			{ID: "summary", Name: "summary.json", SizeBytes: 128, Kind: corerun.ArtifactKindSummary},
		},
	}, nil
}

func (f *fakeWorkflowRuns) WorkflowNodes(context.Context, string) (daemon.WorkflowNodesResponse, error) {
	return daemon.WorkflowNodesResponse{
		RunID: f.run.ID,
		Nodes: []daemon.WorkflowNodeResultDTO{
			{NodeID: "plan", Status: string(corerun.NodeSuccess), Duration: 1200, Attempts: 1},
			{NodeID: "build", Status: string(corerun.NodeRunning), Attempts: 1},
		},
	}, nil
}

func (f *fakeWorkflowRuns) WorkflowSummary(context.Context, string) (daemon.WorkflowSummaryResponse, error) {
	return daemon.WorkflowSummaryResponse{
		RunID: f.run.ID,
		Summary: corerun.Summary{
			RunID:      f.run.ID,
			Workflow:   f.run.Workflow,
			Status:     f.run.Status,
			StartedAt:  f.run.StartedAt,
			AgentCalls: 2,
		},
	}, nil
}

func (f *fakeWorkflowRuns) WorkflowTimeline(context.Context, string, int, int) (daemon.WorkflowTimelineResponse, error) {
	return daemon.WorkflowTimelineResponse{
		RunID: f.run.ID,
		Entries: []corerun.TimelineEntry{
			{Timestamp: f.run.StartedAt, Type: "run.started"},
		},
	}, nil
}

func (f *fakeWorkflowRuns) WorkflowInspect(context.Context, string) (daemon.WorkflowInspectResponse, error) {
	return daemon.WorkflowInspectResponse{
		RunID:          f.run.ID,
		Workflow:       f.run.Workflow,
		Status:         f.run.Status,
		StartedAt:      f.run.StartedAt,
		DurationMS:     2400,
		CurrentStep:    f.run.CurrentStep,
		CompletedSteps: f.run.CompletedSteps,
		PendingSteps:   f.run.PendingSteps,
		TotalSteps:     f.run.TotalSteps,
		AgentCalls:     2,
		BashCalls:      1,
		ArtifactCount:  1,
		NodeCount:      2,
	}, nil
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

func TestAppendMessageSchedulesChatAgentAndPublishesAssistant(t *testing.T) {
	svc, mux, broker := newTestServiceWithAgent(t, &fakeChatAgent{
		resp: chatagent.RunResponse{
			Text: "assistant reply",
			Metadata: map[string]any{
				"provider": "fake",
			},
		},
	})
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)

	sub := broker.Subscribe(session.ID)
	defer sub.Close()

	rec = doReq(t, mux, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", map[string]any{
		"role":    "user",
		"content": "hello",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("append message: %d body=%s", rec.Code, rec.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	assistantSeen := false
	for time.Now().Before(deadline) {
		select {
		case ev := <-sub.C:
			msg, ok := ev.Payload.(persistence.Message)
			if !ok {
				continue
			}
			if ev.Kind == events.KindMessage && msg.Role == persistence.MessageRoleAssistant && msg.Content == "assistant reply" {
				assistantSeen = true
				break
			}
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if assistantSeen {
			break
		}
	}
	if !assistantSeen {
		t.Fatal("did not receive assistant SSE message")
	}

	messages, err := svc.Sessions.ListMessages(context.Background(), session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 || messages[1].Role != persistence.MessageRoleAssistant || messages[1].Content != "assistant reply" {
		t.Fatalf("unexpected messages: %+v", messages)
	}

	calls, err := svc.Sessions.ListToolCalls(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "agentflow.chat" || calls[0].Status != persistence.ToolCallStatusSucceeded {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}
}

func TestAppendMessageChatAgentFailureMarksToolCallAndDiagnostic(t *testing.T) {
	svc, mux, _ := newTestServiceWithAgent(t, &fakeChatAgent{err: errors.New("boom")})
	rec := doReq(t, mux, http.MethodPost, "/api/v1/projects/demo/sessions", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d body=%s", rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)

	rec = doReq(t, mux, http.MethodPost, "/api/v1/sessions/"+session.ID+"/messages", map[string]any{
		"role":    "user",
		"content": "hello",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("append message: %d body=%s", rec.Code, rec.Body.String())
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		calls, err := svc.Sessions.ListToolCalls(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("list tool calls: %v", err)
		}
		if len(calls) == 1 && calls[0].Status == persistence.ToolCallStatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	calls, err := svc.Sessions.ListToolCalls(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("list tool calls: %v", err)
	}
	if len(calls) != 1 || calls[0].Status != persistence.ToolCallStatusFailed || calls[0].Name != "agentflow.chat" {
		t.Fatalf("unexpected tool calls: %+v", calls)
	}

	diags, err := svc.Diagnostics.ListBySession(context.Background(), session.ID, 10)
	if err != nil {
		t.Fatalf("list diagnostics: %v", err)
	}
	if len(diags) == 0 || diags[0].Level != persistence.DiagnosticLevelError {
		t.Fatalf("unexpected diagnostics: %+v", diags)
	}

	messages, err := svc.Sessions.ListMessages(context.Background(), session.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != persistence.MessageRoleUser {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

type fakeChatAgent struct {
	resp chatagent.RunResponse
	err  error
}

func (f *fakeChatAgent) Run(context.Context, chatagent.RunRequest) (chatagent.RunResponse, error) {
	return f.resp, f.err
}
