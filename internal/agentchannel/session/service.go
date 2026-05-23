// Package session owns the chat session lifecycle: project root
// snapshotting, message persistence with payload offloading, and the
// invariants that prevent silent project switches inside a session.
package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diasYuri/agentflow/internal/agentchannel/persistence"
	"github.com/diasYuri/agentflow/internal/app"
)

// ProjectResolver is the slice of app.ProjectRegistry that this
// package needs. The interface keeps tests independent of the JSON
// store while still exercising the production code path.
type ProjectResolver interface {
	Resolve(name string) (app.Project, error)
	List() ([]app.Project, error)
}

// Sessions is the service exposed to the HTTP layer.
type Sessions struct {
	sessions  *persistence.SessionRepository
	messages  *persistence.MessageRepository
	tools     *persistence.ToolCallRepository
	approvals *persistence.ApprovalRepository
	payloads  *persistence.PayloadStore
	projects  ProjectResolver
	now       func() time.Time
}

// Options bundles dependencies for NewSessions.
type Options struct {
	DB       *persistence.DB
	Projects ProjectResolver
	Now      func() time.Time
}

// NewSessions wires the service. Now is optional; time.Now is used by
// default but tests can pass a deterministic clock.
func NewSessions(opts Options) (*Sessions, error) {
	if opts.DB == nil {
		return nil, errors.New("session: DB is required")
	}
	if opts.Projects == nil {
		return nil, errors.New("session: ProjectResolver is required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Sessions{
		sessions:  persistence.NewSessionRepository(opts.DB),
		messages:  persistence.NewMessageRepository(opts.DB),
		tools:     persistence.NewToolCallRepository(opts.DB),
		approvals: persistence.NewApprovalRepository(opts.DB),
		payloads:  persistence.NewPayloadStore(opts.DB),
		projects:  opts.Projects,
		now:       opts.Now,
	}, nil
}

// ErrProjectSwitch is returned when a caller tries to associate a
// session with a project other than the one it was created against.
var ErrProjectSwitch = errors.New("session: project switch is not allowed")

// ErrEmptyContent is returned when a caller appends an empty message.
var ErrEmptyContent = errors.New("session: message content is required")

// CreateInput holds the fields needed to create a session.
type CreateInput struct {
	ProjectName         string
	Title               string
	Provider            string
	Model               string
	Source              string
	ExternalKey         string
	ExternalWorkspaceID string
	ExternalChannelID   string
	ExternalThreadID    string
	ExternalUserID      string
	Metadata            map[string]any
}

// Create snapshots the resolved project root and persists a new
// session.
func (s *Sessions) Create(ctx context.Context, input CreateInput) (persistence.Session, error) {
	project, err := s.projects.Resolve(input.ProjectName)
	if err != nil {
		return persistence.Session{}, fmt.Errorf("resolve project: %w", err)
	}
	now := s.now().UTC()
	session := persistence.Session{
		ID:                  uuid.NewString(),
		ProjectName:         project.Name,
		ProjectPath:         project.Path,
		Title:               strings.TrimSpace(input.Title),
		Status:              persistence.SessionStatusOpen,
		Provider:            strings.TrimSpace(input.Provider),
		Model:               strings.TrimSpace(input.Model),
		Source:              sourceOrDefault(input.Source),
		ExternalKey:         strings.TrimSpace(input.ExternalKey),
		ExternalWorkspaceID: strings.TrimSpace(input.ExternalWorkspaceID),
		ExternalChannelID:   strings.TrimSpace(input.ExternalChannelID),
		ExternalThreadID:    strings.TrimSpace(input.ExternalThreadID),
		ExternalUserID:      strings.TrimSpace(input.ExternalUserID),
		CreatedAt:           now,
		UpdatedAt:           now,
		Metadata:            input.Metadata,
	}
	return s.sessions.Create(ctx, session)
}

// GetByExternalKey returns a session previously associated with a channel
// adapter identity, such as a Slack workspace/channel/thread tuple.
func (s *Sessions) GetByExternalKey(ctx context.Context, externalKey string) (persistence.Session, error) {
	return s.sessions.GetByExternalKey(ctx, externalKey)
}

// ResolveOrCreateByExternalKey reuses a channel-mapped session when it exists
// and creates one otherwise. The external key remains opaque to this package.
func (s *Sessions) ResolveOrCreateByExternalKey(ctx context.Context, input CreateInput) (persistence.Session, error) {
	if strings.TrimSpace(input.ExternalKey) != "" {
		found, err := s.GetByExternalKey(ctx, input.ExternalKey)
		if err == nil {
			return found, nil
		}
		if !errors.Is(err, persistence.ErrSessionNotFound) {
			return persistence.Session{}, err
		}
	}
	return s.Create(ctx, input)
}

// Get returns a session by id.
func (s *Sessions) Get(ctx context.Context, id string) (persistence.Session, error) {
	return s.sessions.Get(ctx, id)
}

// List returns sessions for the named project. An empty project name
// returns every session, sorted by recency.
func (s *Sessions) List(ctx context.Context, projectName string) ([]persistence.Session, error) {
	return s.sessions.ListByProject(ctx, projectName)
}

// Archive marks a session archived.
func (s *Sessions) Archive(ctx context.Context, id string) error {
	return s.sessions.SetStatus(ctx, id, persistence.SessionStatusArchived)
}

// SetTitle updates the human-friendly session title.
func (s *Sessions) SetTitle(ctx context.Context, id, title string) error {
	return s.sessions.SetTitle(ctx, id, title)
}

// AssertProjectMatches confirms that the session was created against
// projectName. Callers use this to refuse cross-project writes that
// would otherwise leak history between unrelated workspaces.
func (s *Sessions) AssertProjectMatches(ctx context.Context, sessionID, projectName string) (persistence.Session, error) {
	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return persistence.Session{}, err
	}
	if strings.TrimSpace(projectName) == "" {
		return session, nil
	}
	if session.ProjectName != projectName {
		return persistence.Session{}, fmt.Errorf("%w: session %q belongs to %q", ErrProjectSwitch, sessionID, session.ProjectName)
	}
	return session, nil
}

// AppendInput captures the request body for AppendMessage.
type AppendInput struct {
	Role          persistence.MessageRole
	Content       string
	CorrelationID string
	Metadata      map[string]any
}

// AppendMessage stores a message inside a session and offloads its
// body to the payload store when it is larger than the inline limit.
// The returned message includes the assigned sequence number.
func (s *Sessions) AppendMessage(ctx context.Context, sessionID string, input AppendInput) (persistence.Message, error) {
	if _, err := s.sessions.Get(ctx, sessionID); err != nil {
		return persistence.Message{}, err
	}
	if input.Role == "" {
		return persistence.Message{}, errors.New("session: role is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return persistence.Message{}, ErrEmptyContent
	}
	msg := persistence.Message{
		ID:            uuid.NewString(),
		SessionID:     sessionID,
		Role:          input.Role,
		CorrelationID: strings.TrimSpace(input.CorrelationID),
		Metadata:      input.Metadata,
	}
	if len(input.Content) > persistence.MaxInlineMessageBytes {
		desc, err := s.payloads.Put(ctx, []byte(input.Content), "text/markdown")
		if err != nil {
			return persistence.Message{}, fmt.Errorf("offload payload: %w", err)
		}
		msg.PayloadRef = desc.ID
	} else {
		msg.Content = input.Content
	}
	stored, err := s.messages.Append(ctx, msg)
	if err != nil {
		return persistence.Message{}, err
	}
	if err := s.sessions.UpdateLastMessageAt(ctx, sessionID, stored.CreatedAt); err != nil {
		return persistence.Message{}, fmt.Errorf("touch session: %w", err)
	}
	return stored, nil
}

// ListMessages returns the messages for a session ordered by sequence.
func (s *Sessions) ListMessages(ctx context.Context, sessionID string, limit int) ([]persistence.Message, error) {
	return s.messages.ListBySession(ctx, sessionID, limit)
}

// SinceSequence returns messages after sequence, useful for SSE
// reconnect replays.
func (s *Sessions) SinceSequence(ctx context.Context, sessionID string, sequence int64) ([]persistence.Message, error) {
	return s.messages.SinceSequence(ctx, sessionID, sequence)
}

// ResolvePayload loads the bytes for a payload reference.
func (s *Sessions) ResolvePayload(ctx context.Context, id string) ([]byte, persistence.PayloadDescriptor, error) {
	return s.payloads.Get(ctx, id)
}

// RecordToolCall inserts a new tool-call lifecycle row.
func (s *Sessions) RecordToolCall(ctx context.Context, call persistence.ToolCall) (persistence.ToolCall, error) {
	if _, err := s.sessions.Get(ctx, call.SessionID); err != nil {
		return persistence.ToolCall{}, err
	}
	return s.tools.Insert(ctx, call)
}

// GetToolCall returns a tool call row by id.
func (s *Sessions) GetToolCall(ctx context.Context, id string) (persistence.ToolCall, error) {
	return s.tools.Get(ctx, id)
}

// UpdateToolCallStatus transitions a tool call.
func (s *Sessions) UpdateToolCallStatus(ctx context.Context, id string, status persistence.ToolCallStatus, responseRef, errMsg string) error {
	return s.tools.UpdateStatus(ctx, id, status, responseRef, errMsg)
}

// ListToolCalls returns the lifecycle rows for a session.
func (s *Sessions) ListToolCalls(ctx context.Context, sessionID string) ([]persistence.ToolCall, error) {
	return s.tools.ListBySession(ctx, sessionID)
}

// CreateApproval inserts a new approval row.
func (s *Sessions) CreateApproval(ctx context.Context, approval persistence.Approval) (persistence.Approval, error) {
	if _, err := s.sessions.Get(ctx, approval.SessionID); err != nil {
		return persistence.Approval{}, err
	}
	return s.approvals.Create(ctx, approval)
}

// DecideApproval records the human decision.
func (s *Sessions) DecideApproval(ctx context.Context, id string, status persistence.ApprovalStatus, reason, decidedBy string) error {
	return s.approvals.Decide(ctx, id, status, reason, decidedBy)
}

// ListApprovals returns the approvals for a session.
func (s *Sessions) ListApprovals(ctx context.Context, sessionID string) ([]persistence.Approval, error) {
	return s.approvals.ListBySession(ctx, sessionID)
}

// SetStatus updates the lifecycle status of a session.
func (s *Sessions) SetStatus(ctx context.Context, id string, status persistence.SessionStatus) error {
	return s.sessions.SetStatus(ctx, id, status)
}

// Delete removes a session and the cascade of dependent rows.
func (s *Sessions) Delete(ctx context.Context, id string) error {
	return s.sessions.Delete(ctx, id)
}

func sourceOrDefault(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "web"
	}
	return source
}
