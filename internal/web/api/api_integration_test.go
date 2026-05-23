package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/events"
	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

func TestToolCallLifecycleEndpoints(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/tool-calls`, map[string]any{`name`: `shell.exec`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create tool call: %d body=%s`, rec.Code, rec.Body.String())
	}
	var call persistence.ToolCall
	if err := json.Unmarshal(rec.Body.Bytes(), &call); err != nil {
		t.Fatalf(`decode tool call: %v`, err)
	}
	if call.Name != `shell.exec` || call.Status != persistence.ToolCallStatusPending {
		t.Fatalf(`unexpected tool call: %+v`, call)
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/sessions/`+session.ID+`/tool-calls`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`list tool calls: %d`, rec.Code)
	}
	var listPayload struct {
		ToolCalls []persistence.ToolCall `json:"tool_calls"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf(`decode list: %v`, err)
	}
	if len(listPayload.ToolCalls) != 1 {
		t.Fatalf(`expected 1 tool call, got %d`, len(listPayload.ToolCalls))
	}
	rec = doReq(t, mux, http.MethodPatch, `/api/v1/tool-calls/`+call.ID, map[string]any{`status`: `succeeded`})
	if rec.Code != http.StatusNoContent {
		t.Fatalf(`patch tool call: %d body=%s`, rec.Code, rec.Body.String())
	}
}

func TestApprovalLifecycleEndpoints(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/approvals`, map[string]any{`tool_call_id`: `tc-1`, `reason`: `needs review`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create approval: %d body=%s`, rec.Code, rec.Body.String())
	}
	var approval persistence.Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &approval); err != nil {
		t.Fatalf(`decode approval: %v`, err)
	}
	if approval.Status != persistence.ApprovalStatusPending {
		t.Fatalf(`expected pending, got %q`, approval.Status)
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/sessions/`+session.ID+`/approvals`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`list approvals: %d`, rec.Code)
	}
	var listPayload struct {
		Approvals []persistence.Approval `json:"approvals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf(`decode list: %v`, err)
	}
	if len(listPayload.Approvals) != 1 {
		t.Fatalf(`expected 1 approval, got %d`, len(listPayload.Approvals))
	}
	rec = doReq(t, mux, http.MethodPost, `/api/v1/approvals/`+approval.ID+`/decide`, map[string]any{`status`: `approved`, `decided_by`: `test-user`})
	if rec.Code != http.StatusNoContent {
		t.Fatalf(`decide approval: %d body=%s`, rec.Code, rec.Body.String())
	}
}

func TestFrontendEventsEndpoints(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/frontend-events`, map[string]any{`kind`: `click`, `content`: `send-button`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create frontend event: %d body=%s`, rec.Code, rec.Body.String())
	}
	var event persistence.FrontendEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &event); err != nil {
		t.Fatalf(`decode event: %v`, err)
	}
	if event.Kind != `click` {
		t.Fatalf(`unexpected kind: %q`, event.Kind)
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/sessions/`+session.ID+`/frontend-events`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`list frontend events: %d`, rec.Code)
	}
	var listPayload struct {
		Events []persistence.FrontendEvent `json:"frontend_events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf(`decode list: %v`, err)
	}
	if len(listPayload.Events) != 1 || listPayload.Events[0].Kind != `click` {
		t.Fatalf(`unexpected events: %+v`, listPayload.Events)
	}
}

func TestSessionDiagnosticsEndpoint(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/diagnostics`, map[string]any{`level`: `error`, `source`: `test`, `message`: `something broke`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create diagnostic: %d body=%s`, rec.Code, rec.Body.String())
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/sessions/`+session.ID+`/diagnostics`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`list diagnostics: %d body=%s`, rec.Code, rec.Body.String())
	}
	var payload struct {
		Diagnostics []persistence.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf(`decode: %v`, err)
	}
	if len(payload.Diagnostics) != 1 || payload.Diagnostics[0].Message != `something broke` {
		t.Fatalf(`unexpected diagnostics: %+v`, payload.Diagnostics)
	}
}

func TestRecentDiagnosticsEndpoint(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/diagnostics`, map[string]any{`level`: `warning`, `source`: `server`, `message`: `low disk`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create diagnostic: %d`, rec.Code)
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/diagnostics`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`recent diagnostics: %d body=%s`, rec.Code, rec.Body.String())
	}
	var payload struct {
		Diagnostics []persistence.Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf(`decode: %v`, err)
	}
	if len(payload.Diagnostics) != 1 || payload.Diagnostics[0].Message != `low disk` {
		t.Fatalf(`unexpected diagnostics: %+v`, payload.Diagnostics)
	}
}

func TestMessageSinceSequenceReplay(t *testing.T) {
	_, mux, _ := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d`, rec.Code)
	}
	session := decodeSession(t, rec)
	for i, content := range []string{`first`, `second`, `third`} {
		rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/messages`, map[string]any{`role`: `user`, `content`: content})
		if rec.Code != http.StatusCreated {
			t.Fatalf(`append message %d: %d`, i, rec.Code)
		}
	}
	rec = doReq(t, mux, http.MethodGet, `/api/v1/sessions/`+session.ID+`/messages?since_sequence=1`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf(`since_sequence: %d body=%s`, rec.Code, rec.Body.String())
	}
	var payload struct {
		Messages []persistence.Message `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf(`decode: %v`, err)
	}
	if len(payload.Messages) != 2 {
		t.Fatalf(`expected 2 messages after seq 1, got %d`, len(payload.Messages))
	}
	if payload.Messages[0].Content != `second` || payload.Messages[1].Content != `third` {
		t.Fatalf(`unexpected messages: %+v`, payload.Messages)
	}
}

func TestSSEReconnectReplaysMessages(t *testing.T) {
	_, mux, broker := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d body=%s`, rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/messages`, map[string]any{`role`: `user`, `content`: `msg-one`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`append first: %d`, rec.Code)
	}
	rec = doReq(t, mux, http.MethodPost, `/api/v1/sessions/`+session.ID+`/messages`, map[string]any{`role`: `user`, `content`: `msg-two`})
	if rec.Code != http.StatusCreated {
		t.Fatalf(`append second: %d`, rec.Code)
	}
	w := newCapturingWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, `/api/v1/sessions/`+session.ID+`/stream?since_sequence=1`, nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(w, req)
		close(done)
	}()
	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Publish(session.ID, events.KindMessage, map[string]string{`text`: `live`}, `corr`)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		body := w.Bytes()
		if bytes.Contains(body, []byte(`msg-two`)) && bytes.Contains(body, []byte(`live`)) {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf(`did not receive replayed and live events, got: %q`, w.Bytes())
}

func TestGlobalStreamReceivesEvents(t *testing.T) {
	_, mux, broker := newTestService(t)
	rec := doReq(t, mux, http.MethodPost, `/api/v1/projects/demo/sessions`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf(`create session: %d body=%s`, rec.Code, rec.Body.String())
	}
	session := decodeSession(t, rec)
	w := newCapturingWriter()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, `/api/v1/stream`, nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		mux.ServeHTTP(w, req)
		close(done)
	}()
	go func() {
		time.Sleep(50 * time.Millisecond)
		broker.Publish(session.ID, events.KindMessage, map[string]string{`text`: `global`}, `corr`)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(w.Bytes(), []byte(`global`)) {
			cancel()
			<-done
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	t.Fatalf(`did not receive global stream event, got: %q`, w.Bytes())
}
