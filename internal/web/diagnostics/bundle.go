package diagnostics

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/web/persistence"
)

// BundleOptions controls what a debug bundle includes.
type BundleOptions struct {
	SessionID         string
	IncludePayloads   bool
	MaxPayloadBytes   int64
	MaxDiagnostics    int
	BundleDescription string
	GeneratedAt       time.Time
}

// BundleSources groups the repositories the exporter reads from.
type BundleSources struct {
	Sessions    *persistence.SessionRepository
	Messages    *persistence.MessageRepository
	Tools       *persistence.ToolCallRepository
	Approvals   *persistence.ApprovalRepository
	Diagnostics *persistence.DiagnosticRepository
	Events      *persistence.FrontendEventRepository
	Payloads    *persistence.PayloadStore
}

// BundleExporter writes redacted JSONL/TOML/README artefacts into a
// zip stream so users can attach the result to a support ticket.
type BundleExporter struct {
	policy  RedactionPolicy
	sources BundleSources
}

// NewBundleExporter wires the exporter.
func NewBundleExporter(sources BundleSources, policy RedactionPolicy) *BundleExporter {
	if policy.MaxValueBytes == 0 && len(policy.SecretKeySubstrings) == 0 && len(policy.SecretValuePatterns) == 0 {
		policy = DefaultPolicy()
	}
	return &BundleExporter{policy: policy, sources: sources}
}

// Write streams a zip bundle into w covering the requested session.
func (e *BundleExporter) Write(ctx context.Context, w io.Writer, opts BundleOptions) error {
	if e.sources.Sessions == nil {
		return errors.New("diagnostics: sessions repository is required")
	}
	if strings.TrimSpace(opts.SessionID) == "" {
		return errors.New("diagnostics: session_id is required")
	}
	if opts.MaxDiagnostics <= 0 {
		opts.MaxDiagnostics = 500
	}
	if opts.GeneratedAt.IsZero() {
		opts.GeneratedAt = time.Now().UTC()
	}
	session, err := e.sources.Sessions.Get(ctx, opts.SessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	zw := zip.NewWriter(w)
	defer zw.Close()
	if err := e.writeSessionAndHistory(ctx, zw, session, opts); err != nil {
		return err
	}
	if err := e.writeManifest(zw, session, opts); err != nil {
		return err
	}
	return e.writeReadme(zw, session, opts)
}

func (e *BundleExporter) writeSessionAndHistory(ctx context.Context, zw *zip.Writer, session persistence.Session, opts BundleOptions) error {
	messages, err := e.sources.Messages.ListBySession(ctx, opts.SessionID, 0)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}
	tools, err := e.sources.Tools.ListBySession(ctx, opts.SessionID)
	if err != nil {
		return fmt.Errorf("load tool calls: %w", err)
	}
	approvals, err := e.sources.Approvals.ListBySession(ctx, opts.SessionID)
	if err != nil {
		return fmt.Errorf("load approvals: %w", err)
	}
	diags, err := e.sources.Diagnostics.ListBySession(ctx, opts.SessionID, opts.MaxDiagnostics)
	if err != nil {
		return fmt.Errorf("load diagnostics: %w", err)
	}
	events, err := e.sources.Events.ListBySession(ctx, opts.SessionID, opts.MaxDiagnostics)
	if err != nil {
		return fmt.Errorf("load frontend events: %w", err)
	}
	if err := e.writeJSON(zw, "session.json", session); err != nil {
		return err
	}
	if err := writeNDJSONFile(zw, "messages.jsonl", convertMessages(messages, e.policy)); err != nil {
		return err
	}
	if err := writeNDJSONFile(zw, "tool_calls.jsonl", tools); err != nil {
		return err
	}
	if err := writeNDJSONFile(zw, "approvals.jsonl", approvals); err != nil {
		return err
	}
	if err := writeNDJSONFile(zw, "diagnostics.jsonl", diags); err != nil {
		return err
	}
	if err := writeNDJSONFile(zw, "frontend_events.jsonl", convertEvents(events, e.policy)); err != nil {
		return err
	}
	if opts.IncludePayloads {
		return e.writePayloads(ctx, zw, messages, opts.MaxPayloadBytes)
	}
	return nil
}

func (e *BundleExporter) writeJSON(zw *zip.Writer, name string, value any) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func writeNDJSONFile[T any](zw *zip.Writer, name string, items []T) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	return nil
}
