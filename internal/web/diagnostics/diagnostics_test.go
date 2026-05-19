package diagnostics_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diasYuri/agentflow/internal/web/diagnostics"
	"github.com/diasYuri/agentflow/internal/web/persistence"
)

func TestDefaultPolicyMasksSecrets(t *testing.T) {
	policy := diagnostics.DefaultPolicy()
	masked := policy.RedactMap(map[string]any{
		"username":      "ada",
		"password":      "hunter2",
		"Authorization": "Bearer abcdef1234567890",
	})
	if masked["password"] != "[redacted]" {
		t.Fatalf("expected password to be redacted, got %v", masked["password"])
	}
	if masked["Authorization"] != "[redacted]" {
		t.Fatalf("expected Authorization to be redacted, got %v", masked["Authorization"])
	}
	if masked["username"] != "ada" {
		t.Fatalf("expected username to remain, got %v", masked["username"])
	}
}

func TestDefaultPolicyMasksBearerInValues(t *testing.T) {
	policy := diagnostics.DefaultPolicy()
	got := policy.Redact("Bearer abcdef1234567890")
	if got != "[redacted]" {
		t.Fatalf("expected bearer to be redacted, got %v", got)
	}
}

func TestRedactTruncatesLongValues(t *testing.T) {
	policy := diagnostics.RedactionPolicy{MaxValueBytes: 10}
	got := policy.Redact("0123456789ABCDEFG")
	str, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}
	if !strings.Contains(str, "[truncated") {
		t.Fatalf("expected truncation marker, got %q", str)
	}
}

func TestRecorderAppliesRedaction(t *testing.T) {
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "web.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	rec, err := diagnostics.NewRecorder(diagnostics.Options{DB: db})
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	diag, err := rec.Record(context.Background(), persistence.Diagnostic{
		Level:   persistence.DiagnosticLevelError,
		Source:  "server",
		Message: "leak",
		Context: map[string]any{"api_key": "sk-abcdef1234567890"},
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if diag.Context["api_key"] != "[redacted]" {
		t.Fatalf("expected api_key redacted, got %v", diag.Context)
	}
	if diag.CorrelationID == "" {
		t.Fatalf("expected correlation id to be issued")
	}
}

func TestBundleExporterRespectsRedactionAndOmits(t *testing.T) {
	dir := t.TempDir()
	db, err := persistence.Open(context.Background(), filepath.Join(dir, "web.sqlite"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	sessions := persistence.NewSessionRepository(db)
	messages := persistence.NewMessageRepository(db)
	tools := persistence.NewToolCallRepository(db)
	approvals := persistence.NewApprovalRepository(db)
	diagsRepo := persistence.NewDiagnosticRepository(db)
	events := persistence.NewFrontendEventRepository(db)
	payloads := persistence.NewPayloadStore(db)
	ctx := context.Background()
	session, err := sessions.Create(ctx, persistence.Session{ID: "s1", ProjectName: "demo", ProjectPath: "/p"})
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if _, err := messages.Append(ctx, persistence.Message{SessionID: session.ID, Role: persistence.MessageRoleUser, Content: "Bearer abcdef1234567890"}); err != nil {
		t.Fatalf("message: %v", err)
	}
	desc, err := payloads.Put(ctx, []byte(strings.Repeat("x", 5000)), "text/plain")
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if _, err := messages.Append(ctx, persistence.Message{SessionID: session.ID, Role: persistence.MessageRoleAssistant, PayloadRef: desc.ID}); err != nil {
		t.Fatalf("payload msg: %v", err)
	}
	if _, err := diagsRepo.Insert(ctx, persistence.Diagnostic{SessionID: session.ID, Level: persistence.DiagnosticLevelInfo, Source: "test", Message: "ok"}); err != nil {
		t.Fatalf("diag: %v", err)
	}
	exporter := diagnostics.NewBundleExporter(diagnostics.BundleSources{
		Sessions:    sessions,
		Messages:    messages,
		Tools:       tools,
		Approvals:   approvals,
		Diagnostics: diagsRepo,
		Events:      events,
		Payloads:    payloads,
	}, diagnostics.DefaultPolicy())
	var buf bytes.Buffer
	if err := exporter.Write(ctx, &buf, diagnostics.BundleOptions{SessionID: session.ID, IncludePayloads: true, MaxPayloadBytes: 1000}); err != nil {
		t.Fatalf("export: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}
	files := make(map[string][]byte)
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		files[f.Name] = body
	}
	if _, ok := files["session.json"]; !ok {
		t.Fatalf("missing session.json: %+v", filesNames(files))
	}
	if _, ok := files["manifest.toml"]; !ok {
		t.Fatalf("missing manifest.toml")
	}
	if _, ok := files["README.md"]; !ok {
		t.Fatalf("missing README.md")
	}
	msgs := files["messages.jsonl"]
	if !strings.Contains(string(msgs), "[redacted]") {
		t.Fatalf("expected message content to be redacted: %s", msgs)
	}
	if strings.Contains(string(msgs), "Bearer abcdef") {
		t.Fatalf("found unredacted secret in bundle: %s", msgs)
	}
	manifestData := files["payloads/manifest.json"]
	var manifest []map[string]any
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("manifest decode: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("expected 1 manifest entry, got %d", len(manifest))
	}
	if manifest[0]["omitted_reason"] == nil {
		t.Fatalf("expected omitted_reason on oversized payload, got %v", manifest[0])
	}
	if _, ok := files["payloads/"+desc.ID+".txt"]; ok {
		t.Fatalf("oversized payload should be omitted")
	}
}

func filesNames(in map[string][]byte) []string {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	return keys
}
