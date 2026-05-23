package diagnostics

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

func (e *BundleExporter) writePayloads(ctx context.Context, zw *zip.Writer, messages []persistence.Message, maxBytes int64) error {
	manifest := make([]payloadManifestEntry, 0)
	for _, msg := range messages {
		if msg.PayloadRef == "" {
			continue
		}
		body, desc, err := e.sources.Payloads.Get(ctx, msg.PayloadRef)
		if err != nil {
			manifest = append(manifest, payloadManifestEntry{Ref: msg.PayloadRef, MessageID: msg.ID, OmittedReason: err.Error()})
			continue
		}
		if maxBytes > 0 && desc.SizeBytes > maxBytes {
			manifest = append(manifest, payloadManifestEntry{
				Ref:           msg.PayloadRef,
				MessageID:     msg.ID,
				SizeBytes:     desc.SizeBytes,
				OmittedReason: fmt.Sprintf("size %d exceeds limit %d", desc.SizeBytes, maxBytes),
			})
			continue
		}
		name := "payloads/" + msg.PayloadRef + payloadExtension(desc.ContentType)
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write(body); err != nil {
			return err
		}
		manifest = append(manifest, payloadManifestEntry{
			Ref:         msg.PayloadRef,
			MessageID:   msg.ID,
			SizeBytes:   desc.SizeBytes,
			ContentType: desc.ContentType,
			ArchivePath: name,
		})
	}
	return e.writeJSON(zw, "payloads/manifest.json", manifest)
}

type payloadManifestEntry struct {
	Ref           string `json:"ref"`
	MessageID     string `json:"message_id,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	ArchivePath   string `json:"archive_path,omitempty"`
	OmittedReason string `json:"omitted_reason,omitempty"`
}

func (e *BundleExporter) writeManifest(zw *zip.Writer, session persistence.Session, opts BundleOptions) error {
	f, err := zw.Create("manifest.toml")
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("[bundle]\n")
	fmt.Fprintf(&b, "session_id = %s\n", strconv.Quote(session.ID))
	fmt.Fprintf(&b, "project_name = %s\n", strconv.Quote(session.ProjectName))
	fmt.Fprintf(&b, "project_path = %s\n", strconv.Quote(session.ProjectPath))
	fmt.Fprintf(&b, "generated_at = %s\n", strconv.Quote(opts.GeneratedAt.UTC().Format(time.RFC3339)))
	fmt.Fprintf(&b, "include_payloads = %t\n", opts.IncludePayloads)
	if opts.MaxPayloadBytes > 0 {
		fmt.Fprintf(&b, "max_payload_bytes = %d\n", opts.MaxPayloadBytes)
	}
	if opts.BundleDescription != "" {
		fmt.Fprintf(&b, "description = %s\n", strconv.Quote(opts.BundleDescription))
	}
	b.WriteString("\n[redaction]\n")
	fmt.Fprintf(&b, "max_value_bytes = %d\n", e.policy.MaxValueBytes)
	fmt.Fprintf(&b, "secret_key_substrings = [%s]\n", quoteList(e.policy.SecretKeySubstrings))
	fmt.Fprintf(&b, "secret_value_patterns = [%s]\n", quoteRegexList(e.policy.SecretValuePatterns))
	_, err = io.WriteString(f, b.String())
	return err
}

func (e *BundleExporter) writeReadme(zw *zip.Writer, session persistence.Session, opts BundleOptions) error {
	f, err := zw.Create("README.md")
	if err != nil {
		return err
	}
	var body strings.Builder
	body.WriteString("# AgentFlow Debug Bundle\n\n")
	fmt.Fprintf(&body, "Session: %s\n", session.ID)
	fmt.Fprintf(&body, "Project: %s (%s)\n", session.ProjectName, session.ProjectPath)
	fmt.Fprintf(&body, "Generated at: %s\n\n", opts.GeneratedAt.UTC().Format(time.RFC3339))
	body.WriteString("## Contents\n\n")
	body.WriteString("- session.json: snapshot of the session record\n")
	body.WriteString("- messages.jsonl: chat history (large bodies stored under payloads/)\n")
	body.WriteString("- tool_calls.jsonl: tool call lifecycle rows\n")
	body.WriteString("- approvals.jsonl: human approval decisions\n")
	body.WriteString("- diagnostics.jsonl: structured diagnostic events (redacted)\n")
	body.WriteString("- frontend_events.jsonl: browser-side events (redacted)\n")
	if opts.IncludePayloads {
		body.WriteString("- payloads/: offloaded message bodies and a manifest.json index\n")
	} else {
		body.WriteString("- payloads/: omitted (set include_payloads=true to attach them)\n")
	}
	body.WriteString("- manifest.toml: bundle metadata and redaction policy summary\n\n")
	body.WriteString("## Redaction\n\n")
	body.WriteString("Secrets matching known token shapes are replaced with [redacted].\n")
	body.WriteString("Long values are truncated and annotated with the original byte count.\n")
	_, err = io.WriteString(f, body.String())
	return err
}

func quoteList(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconv.Quote(item)
	}
	return strings.Join(parts, ", ")
}

func quoteRegexList(items []*regexp.Regexp) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = strconv.Quote(item.String())
	}
	return strings.Join(parts, ", ")
}

func payloadExtension(contentType string) string {
	switch strings.ToLower(contentType) {
	case "application/json":
		return ".json"
	case "text/markdown":
		return ".md"
	case "text/plain":
		return ".txt"
	default:
		return ".bin"
	}
}

func convertMessages(messages []persistence.Message, policy RedactionPolicy) []persistence.Message {
	out := make([]persistence.Message, len(messages))
	for i, msg := range messages {
		cp := msg
		if cp.Content != "" {
			cp.Content = policy.redactString("content", cp.Content)
		}
		if cp.Metadata != nil {
			cp.Metadata = policy.RedactMap(cp.Metadata)
		}
		out[i] = cp
	}
	return out
}

func convertEvents(events []persistence.FrontendEvent, policy RedactionPolicy) []persistence.FrontendEvent {
	out := make([]persistence.FrontendEvent, len(events))
	for i, event := range events {
		cp := event
		if cp.Content != "" {
			cp.Content = policy.redactString("content", cp.Content)
		}
		if cp.Metadata != nil {
			cp.Metadata = policy.RedactMap(cp.Metadata)
		}
		out[i] = cp
	}
	return out
}
