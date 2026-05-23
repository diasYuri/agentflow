package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
)

// Recorder writes diagnostics through the redaction policy before
// they reach storage. It also issues correlation IDs for callers that
// did not bring their own.
type Recorder struct {
	repo   *persistence.DiagnosticRepository
	events *persistence.FrontendEventRepository
	policy RedactionPolicy
	now    func() time.Time
	pubFn  PublishFn
}

// PublishFn is invoked after a diagnostic has been persisted so
// callers can fan it out through the SSE broker.
type PublishFn func(diag persistence.Diagnostic)

// Options for NewRecorder.
type Options struct {
	DB      *persistence.DB
	Policy  RedactionPolicy
	Now     func() time.Time
	Publish PublishFn
}

// NewRecorder builds a recorder.
func NewRecorder(opts Options) (*Recorder, error) {
	if opts.DB == nil {
		return nil, errors.New("diagnostics: DB is required")
	}
	policy := opts.Policy
	if policy.MaxValueBytes == 0 && len(policy.SecretKeySubstrings) == 0 && len(policy.SecretValuePatterns) == 0 {
		policy = DefaultPolicy()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Recorder{
		repo:   persistence.NewDiagnosticRepository(opts.DB),
		events: persistence.NewFrontendEventRepository(opts.DB),
		policy: policy,
		now:    opts.Now,
		pubFn:  opts.Publish,
	}, nil
}

// NewCorrelationID returns a fresh correlation id callers can stamp
// onto a diagnostic, frontend event, or tool call so the relationship
// survives across packages.
func NewCorrelationID() string { return uuid.NewString() }

// Record persists a diagnostic after applying the redaction policy.
func (r *Recorder) Record(ctx context.Context, diag persistence.Diagnostic) (persistence.Diagnostic, error) {
	if strings.TrimSpace(diag.Source) == "" {
		return persistence.Diagnostic{}, errors.New("diagnostics: source is required")
	}
	if diag.Level == "" {
		diag.Level = persistence.DiagnosticLevelInfo
	}
	if diag.CorrelationID == "" {
		diag.CorrelationID = NewCorrelationID()
	}
	if diag.CreatedAt.IsZero() {
		diag.CreatedAt = r.now().UTC()
	}
	if diag.Context != nil {
		diag.Context = r.policy.RedactMap(diag.Context)
	}
	if r.policy.MaxValueBytes > 0 && len(diag.Message) > r.policy.MaxValueBytes {
		diag.Message = fmt.Sprintf("%s... [truncated %d bytes]", diag.Message[:r.policy.MaxValueBytes], len(diag.Message)-r.policy.MaxValueBytes)
	}
	stored, err := r.repo.Insert(ctx, diag)
	if err != nil {
		return persistence.Diagnostic{}, err
	}
	if r.pubFn != nil {
		r.pubFn(stored)
	}
	return stored, nil
}

// RecordFrontendEvent stores a browser-originated event after running
// the same redaction over its metadata.
func (r *Recorder) RecordFrontendEvent(ctx context.Context, event persistence.FrontendEvent) (persistence.FrontendEvent, error) {
	if strings.TrimSpace(event.Kind) == "" {
		return persistence.FrontendEvent{}, errors.New("diagnostics: event kind is required")
	}
	if event.CorrelationID == "" {
		event.CorrelationID = NewCorrelationID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = r.now().UTC()
	}
	if event.Metadata != nil {
		event.Metadata = r.policy.RedactMap(event.Metadata)
	}
	if r.policy.MaxValueBytes > 0 && len(event.Content) > r.policy.MaxValueBytes {
		event.Content = fmt.Sprintf("%s... [truncated %d bytes]", event.Content[:r.policy.MaxValueBytes], len(event.Content)-r.policy.MaxValueBytes)
	}
	return r.events.Insert(ctx, event)
}

// ListBySession returns diagnostics that have already been redacted.
func (r *Recorder) ListBySession(ctx context.Context, sessionID string, limit int) ([]persistence.Diagnostic, error) {
	return r.repo.ListBySession(ctx, sessionID, limit)
}

// ListRecent returns the most recent diagnostics across all sessions.
func (r *Recorder) ListRecent(ctx context.Context, limit int) ([]persistence.Diagnostic, error) {
	return r.repo.ListRecent(ctx, limit)
}

// ListEvents returns frontend events for a session.
func (r *Recorder) ListEvents(ctx context.Context, sessionID string, limit int) ([]persistence.FrontendEvent, error) {
	return r.events.ListBySession(ctx, sessionID, limit)
}

// Policy exposes the active policy. Callers should treat the result
// as read-only.
func (r *Recorder) Policy() RedactionPolicy { return r.policy }
