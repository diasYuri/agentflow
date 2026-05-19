package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ToolCallRepository persists ToolCall lifecycle rows.
type ToolCallRepository struct {
	db *sql.DB
}

// NewToolCallRepository wires the repo.
func NewToolCallRepository(db *DB) *ToolCallRepository {
	return &ToolCallRepository{db: db.SQL()}
}

// ErrToolCallNotFound is returned when the requested tool call does
// not exist in the database.
var ErrToolCallNotFound = errors.New("persistence: tool call not found")

// Insert creates a new tool call row in pending or running state.
func (r *ToolCallRepository) Insert(ctx context.Context, call ToolCall) (ToolCall, error) {
	if strings.TrimSpace(call.SessionID) == "" {
		return ToolCall{}, errors.New("persistence: tool_call session_id is required")
	}
	if strings.TrimSpace(call.Name) == "" {
		return ToolCall{}, errors.New("persistence: tool_call name is required")
	}
	if call.ID == "" {
		call.ID = uuid.NewString()
	}
	if call.Status == "" {
		call.Status = ToolCallStatusPending
	}
	if call.StartedAt.IsZero() {
		call.StartedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO tool_calls (id, session_id, message_id, name, status, request_ref, response_ref, error, correlation_id, started_at, finished_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		call.ID,
		call.SessionID,
		nullableString(call.MessageID),
		call.Name,
		string(call.Status),
		nullableString(call.RequestRef),
		nullableString(call.ResponseRef),
		nullableString(call.Error),
		nullableString(call.CorrelationID),
		call.StartedAt.Format(time.RFC3339Nano),
		nullableTime(call.FinishedAt),
	)
	if err != nil {
		return ToolCall{}, fmt.Errorf("insert tool_call: %w", err)
	}
	return call, nil
}

// UpdateStatus transitions the tool call status. ResponseRef/Error are
// optional and only written when non-empty so partial updates do not
// erase earlier values.
func (r *ToolCallRepository) UpdateStatus(ctx context.Context, id string, status ToolCallStatus, responseRef, errMsg string) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE tool_calls SET
	status = ?,
	response_ref = COALESCE(?, response_ref),
	error = COALESCE(?, error),
	finished_at = CASE WHEN ? IN ("succeeded","failed","cancelled") THEN ? ELSE finished_at END
WHERE id = ?`,
		string(status),
		nullableString(responseRef),
		nullableString(errMsg),
		string(status),
		time.Now().UTC().Format(time.RFC3339Nano),
		id,
	)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrToolCallNotFound
	}
	return nil
}

// Get returns a tool call row by id.
func (r *ToolCallRepository) Get(ctx context.Context, id string) (ToolCall, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, session_id, message_id, name, status, request_ref, response_ref, error, correlation_id, started_at, finished_at
FROM tool_calls WHERE id = ?`, id)
	return scanToolCall(row)
}

// ListBySession returns every tool call linked to sessionID ordered by
// start time.
func (r *ToolCallRepository) ListBySession(ctx context.Context, sessionID string) ([]ToolCall, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, session_id, message_id, name, status, request_ref, response_ref, error, correlation_id, started_at, finished_at
FROM tool_calls WHERE session_id = ? ORDER BY started_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list tool_calls: %w", err)
	}
	defer rows.Close()
	var calls []ToolCall
	for rows.Next() {
		call, err := scanToolCall(rows)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, rows.Err()
}

func scanToolCall(scanner rowScanner) (ToolCall, error) {
	var (
		call                                                       ToolCall
		messageID, requestRef, responseRef, errMsg, corr, finished sql.NullString
		startedAt                                                  string
	)
	if err := scanner.Scan(
		&call.ID,
		&call.SessionID,
		&messageID,
		&call.Name,
		&call.Status,
		&requestRef,
		&responseRef,
		&errMsg,
		&corr,
		&startedAt,
		&finished,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ToolCall{}, ErrToolCallNotFound
		}
		return ToolCall{}, err
	}
	if messageID.Valid {
		call.MessageID = messageID.String
	}
	if requestRef.Valid {
		call.RequestRef = requestRef.String
	}
	if responseRef.Valid {
		call.ResponseRef = responseRef.String
	}
	if errMsg.Valid {
		call.Error = errMsg.String
	}
	if corr.Valid {
		call.CorrelationID = corr.String
	}
	t, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return ToolCall{}, fmt.Errorf("parse started_at: %w", err)
	}
	call.StartedAt = t
	if finished.Valid && finished.String != "" {
		t, err = time.Parse(time.RFC3339Nano, finished.String)
		if err != nil {
			return ToolCall{}, fmt.Errorf("parse finished_at: %w", err)
		}
		call.FinishedAt = t
	}
	return call, nil
}
