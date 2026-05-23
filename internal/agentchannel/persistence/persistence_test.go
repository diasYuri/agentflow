package persistence_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

func openTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "web.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return db
}

func TestOpenUpgradesPreChannelSessionSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "web.sqlite")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = legacy.ExecContext(ctx, `
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    project_path TEXT NOT NULL,
    title TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    provider TEXT,
    model TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_message_at TEXT,
    metadata TEXT
)`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	_, err = legacy.ExecContext(ctx, `
CREATE TABLE messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT,
    payload_ref TEXT,
    correlation_id TEXT,
    created_at TEXT NOT NULL,
    metadata TEXT,
    UNIQUE (session_id, sequence)
)`)
	if err != nil {
		t.Fatalf("create legacy messages schema: %v", err)
	}
	_, err = legacy.ExecContext(ctx, `INSERT INTO sessions (id, project_name, project_path, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"legacy-session", "demo", "/p", "open", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
	_, err = legacy.ExecContext(ctx, `INSERT INTO messages (id, session_id, sequence, role, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"legacy-message", "legacy-session", 1, "user", "hello", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert legacy message: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	db, err := persistence.Open(ctx, path)
	if err != nil {
		t.Fatalf("open upgraded db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := persistence.NewSessionRepository(db)
	created, err := repo.Create(ctx, persistence.Session{
		ID:          "slack-session",
		Source:      "slack",
		ExternalKey: "slack:T1:C1:thread",
	})
	if err != nil {
		t.Fatalf("create upgraded session: %v", err)
	}
	got, err := repo.GetByExternalKey(ctx, created.ExternalKey)
	if err != nil {
		t.Fatalf("get by external key: %v", err)
	}
	if got.ID != created.ID || got.Source != "slack" {
		t.Fatalf("unexpected upgraded session: %+v", got)
	}
	if got.ProjectName != "" || got.ProjectPath != "" {
		t.Fatalf("expected nullable project columns, got %+v", got)
	}
	messages, err := persistence.NewMessageRepository(db).ListBySession(ctx, "legacy-session", 0)
	if err != nil {
		t.Fatalf("list legacy messages: %v", err)
	}
	if len(messages) != 1 || messages[0].Content != "hello" {
		t.Fatalf("legacy messages were not preserved: %+v", messages)
	}
}

func TestSessionLifecycle(t *testing.T) {
	db := openTestDB(t)
	repo := persistence.NewSessionRepository(db)
	ctx := context.Background()
	session := persistence.Session{
		ID:          "session-1",
		ProjectName: "demo",
		ProjectPath: "/projects/demo",
		Title:       "kickoff",
		Metadata:    map[string]any{"provider": "claude"},
	}
	created, err := repo.Create(ctx, session)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Status != persistence.SessionStatusOpen {
		t.Fatalf("expected default status open, got %q", created.Status)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ProjectPath != session.ProjectPath {
		t.Fatalf("project_path drifted: %q", got.ProjectPath)
	}
	if got.Metadata["provider"] != "claude" {
		t.Fatalf("metadata mismatch: %v", got.Metadata)
	}
	list, err := repo.ListByProject(ctx, "demo")
	if err != nil || len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list mismatch: err=%v list=%v", err, list)
	}
	if err := repo.SetStatus(ctx, created.ID, persistence.SessionStatusArchived); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, err = repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after archive: %v", err)
	}
	if got.Status != persistence.SessionStatusArchived {
		t.Fatalf("expected archived, got %q", got.Status)
	}
	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, persistence.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := repo.Delete(ctx, created.ID); !errors.Is(err, persistence.ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound on second delete, got %v", err)
	}
}

func TestPendingSessionLifecycle(t *testing.T) {
	db := openTestDB(t)
	repo := persistence.NewSessionRepository(db)
	ctx := context.Background()
	created, err := repo.Create(ctx, persistence.Session{
		ID:          "pending",
		Source:      "slack",
		ExternalKey: "slack:T1:C1:thread",
	})
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	if created.ProjectName != "" || created.ProjectPath != "" {
		t.Fatalf("expected pending project: %+v", created)
	}
	global, err := repo.ListByProject(ctx, "")
	if err != nil || len(global) != 1 {
		t.Fatalf("global list: err=%v list=%+v", err, global)
	}
	byProject, err := repo.ListByProject(ctx, "demo")
	if err != nil {
		t.Fatalf("project list: %v", err)
	}
	if len(byProject) != 0 {
		t.Fatalf("pending session should not be listed by project: %+v", byProject)
	}
	if err := repo.SetProject(ctx, created.ID, "demo", "/projects/demo"); err != nil {
		t.Fatalf("set project: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ProjectName != "demo" || got.ProjectPath != "/projects/demo" {
		t.Fatalf("project not bound: %+v", got)
	}
}

func TestMessageAppendAssignsSequence(t *testing.T) {
	db := openTestDB(t)
	sessions := persistence.NewSessionRepository(db)
	messages := persistence.NewMessageRepository(db)
	ctx := context.Background()
	_, err := sessions.Create(ctx, persistence.Session{
		ID:          "session-2",
		ProjectName: "demo",
		ProjectPath: "/projects/demo",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	first, err := messages.Append(ctx, persistence.Message{
		SessionID: "session-2",
		Role:      persistence.MessageRoleUser,
		Content:   "hi",
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := messages.Append(ctx, persistence.Message{
		SessionID: "session-2",
		Role:      persistence.MessageRoleAssistant,
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("unexpected sequences: %d %d", first.Sequence, second.Sequence)
	}
	got, err := messages.ListBySession(ctx, "session-2", 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("list: err=%v len=%d", err, len(got))
	}
	if got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("order wrong: %d %d", got[0].Sequence, got[1].Sequence)
	}
	since, err := messages.SinceSequence(ctx, "session-2", 1)
	if err != nil || len(since) != 1 || since[0].ID != second.ID {
		t.Fatalf("since: err=%v len=%d", err, len(since))
	}
}

func TestPayloadStoreRoundTrip(t *testing.T) {
	db := openTestDB(t)
	store := persistence.NewPayloadStore(db)
	ctx := context.Background()
	body := []byte(strings.Repeat("xyz", 1000))
	desc, err := store.Put(ctx, body, "application/json")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if desc.SizeBytes != int64(len(body)) {
		t.Fatalf("size mismatch: %d", desc.SizeBytes)
	}
	got, descGet, err := store.Get(ctx, desc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(body) || descGet.Sha256 != desc.Sha256 {
		t.Fatalf("body or hash mismatch")
	}
	if _, _, err := store.Get(ctx, "missing"); !errors.Is(err, persistence.ErrPayloadNotFound) {
		t.Fatalf("expected ErrPayloadNotFound, got %v", err)
	}
	if _, err := store.Put(ctx, nil, ""); err == nil {
		t.Fatalf("expected empty payload to be rejected")
	}
}

func TestToolCallLifecycle(t *testing.T) {
	db := openTestDB(t)
	sessions := persistence.NewSessionRepository(db)
	tools := persistence.NewToolCallRepository(db)
	ctx := context.Background()
	_, err := sessions.Create(ctx, persistence.Session{ID: "s", ProjectName: "p", ProjectPath: "/p"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	call, err := tools.Insert(ctx, persistence.ToolCall{SessionID: "s", Name: "shell.exec"})
	if err != nil {
		t.Fatalf("insert tool call: %v", err)
	}
	if call.Status != persistence.ToolCallStatusPending {
		t.Fatalf("expected pending status, got %q", call.Status)
	}
	if err := tools.UpdateStatus(ctx, call.ID, persistence.ToolCallStatusSucceeded, "payload-1", ""); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, err := tools.Get(ctx, call.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != persistence.ToolCallStatusSucceeded || got.ResponseRef != "payload-1" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if got.FinishedAt.IsZero() {
		t.Fatalf("expected finished_at to be set")
	}
}

func TestDiagnosticsAndFrontendEvents(t *testing.T) {
	db := openTestDB(t)
	diagRepo := persistence.NewDiagnosticRepository(db)
	evRepo := persistence.NewFrontendEventRepository(db)
	ctx := context.Background()
	if _, err := diagRepo.Insert(ctx, persistence.Diagnostic{
		Level:     persistence.DiagnosticLevelError,
		Source:    "server",
		Message:   "boom",
		Context:   map[string]any{"k": "v"},
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}); err != nil {
		t.Fatalf("insert diag: %v", err)
	}
	sessions := persistence.NewSessionRepository(db)
	if _, err := sessions.Create(ctx, persistence.Session{ID: "fe-session", ProjectName: "demo", ProjectPath: "/p"}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := evRepo.Insert(ctx, persistence.FrontendEvent{
		SessionID: "fe-session",
		Kind:      "click",
		Content:   "send",
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	diags, err := diagRepo.ListRecent(ctx, 10)
	if err != nil || len(diags) != 1 || diags[0].Message != "boom" {
		t.Fatalf("diags: err=%v list=%v", err, diags)
	}
	events, err := evRepo.ListBySession(ctx, "fe-session", 10)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 1 || events[0].Kind != "click" {
		t.Fatalf("unexpected events: %v", events)
	}
}
