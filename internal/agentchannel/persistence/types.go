// Package persistence owns the SQLite store that backs the
// `agentflow web` server. It captures sessions, messages, tool-call
// lifecycle rows, approvals, diagnostics, frontend events, and a
// payload offload table used for content that would otherwise bloat
// the row tables.
//
// The store is intentionally separate from the daemon's run store so
// chat history and workflow execution state never share rows.
package persistence

import "time"

// SessionStatus describes the lifecycle stage of a chat session.
type SessionStatus string

const (
	SessionStatusOpen     SessionStatus = "open"
	SessionStatusArchived SessionStatus = "archived"
)

// MessageRole identifies the author of a message row.
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleTool      MessageRole = "tool"
)

// ToolCallStatus is the lifecycle state of a tool call.
type ToolCallStatus string

const (
	ToolCallStatusPending   ToolCallStatus = "pending"
	ToolCallStatusRunning   ToolCallStatus = "running"
	ToolCallStatusSucceeded ToolCallStatus = "succeeded"
	ToolCallStatusFailed    ToolCallStatus = "failed"
	ToolCallStatusCancelled ToolCallStatus = "cancelled"
)

// ApprovalStatus tracks human approval decisions.
type ApprovalStatus string

const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
)

// DiagnosticLevel mirrors slog levels so existing tooling can read it.
type DiagnosticLevel string

const (
	DiagnosticLevelDebug   DiagnosticLevel = "debug"
	DiagnosticLevelInfo    DiagnosticLevel = "info"
	DiagnosticLevelWarning DiagnosticLevel = "warning"
	DiagnosticLevelError   DiagnosticLevel = "error"
)

// Session is the durable record of a chat conversation. ProjectPath
// captures the project root at session creation time so the chat
// history stays valid even if the registry entry is renamed later.
type Session struct {
	ID                  string         `json:"id"`
	ProjectName         string         `json:"project_name"`
	ProjectPath         string         `json:"project_path"`
	Title               string         `json:"title,omitempty"`
	Status              SessionStatus  `json:"status"`
	Provider            string         `json:"provider,omitempty"`
	Model               string         `json:"model,omitempty"`
	Source              string         `json:"source,omitempty"`
	ExternalKey         string         `json:"external_key,omitempty"`
	ExternalWorkspaceID string         `json:"external_workspace_id,omitempty"`
	ExternalChannelID   string         `json:"external_channel_id,omitempty"`
	ExternalThreadID    string         `json:"external_thread_id,omitempty"`
	ExternalUserID      string         `json:"external_user_id,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	LastMessageAt       time.Time      `json:"last_message_at,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// Message is an immutable history row. PayloadRef points at the
// payload_store table when Content was too large to inline.
type Message struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	Sequence      int64          `json:"sequence"`
	Role          MessageRole    `json:"role"`
	Content       string         `json:"content,omitempty"`
	PayloadRef    string         `json:"payload_ref,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ToolCall is the lifecycle row for a single tool invocation.
type ToolCall struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id"`
	MessageID     string         `json:"message_id,omitempty"`
	Name          string         `json:"name"`
	Status        ToolCallStatus `json:"status"`
	RequestRef    string         `json:"request_ref,omitempty"`
	ResponseRef   string         `json:"response_ref,omitempty"`
	Error         string         `json:"error,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	FinishedAt    time.Time      `json:"finished_at,omitempty"`
}

// Approval represents a human approval gate for a tool call.
type Approval struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"session_id"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Status     ApprovalStatus `json:"status"`
	Reason     string         `json:"reason,omitempty"`
	DecidedBy  string         `json:"decided_by,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	DecidedAt  time.Time      `json:"decided_at,omitempty"`
}

// Diagnostic represents a structured event with redacted context.
type Diagnostic struct {
	ID            string          `json:"id"`
	SessionID     string          `json:"session_id,omitempty"`
	Level         DiagnosticLevel `json:"level"`
	Source        string          `json:"source"`
	Code          string          `json:"code,omitempty"`
	Message       string          `json:"message"`
	Context       map[string]any  `json:"context,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// FrontendEvent records browser-side interactions tied back to a
// session for correlation with diagnostics.
type FrontendEvent struct {
	ID            string         `json:"id"`
	SessionID     string         `json:"session_id,omitempty"`
	Kind          string         `json:"kind"`
	Content       string         `json:"content,omitempty"`
	PayloadRef    string         `json:"payload_ref,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// PayloadDescriptor describes a stored payload.
type PayloadDescriptor struct {
	ID          string    `json:"id"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type,omitempty"`
	Sha256      string    `json:"sha256"`
	CreatedAt   time.Time `json:"created_at"`
}
