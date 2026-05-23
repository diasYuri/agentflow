package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MessageRepository persists Message rows.
type MessageRepository struct {
	db *sql.DB
}

// NewMessageRepository creates a MessageRepository.
func NewMessageRepository(db *DB) *MessageRepository {
	return &MessageRepository{db: db.SQL()}
}

// Append inserts a message into the session history. Sequence is
// assigned automatically as max(sequence)+1 inside a transaction so
// concurrent writers do not collide.
func (r *MessageRepository) Append(ctx context.Context, msg Message) (Message, error) {
	if strings.TrimSpace(msg.SessionID) == "" {
		return Message{}, errors.New("persistence: message session_id is required")
	}
	if msg.Role == "" {
		return Message{}, errors.New("persistence: message role is required")
	}
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	metaJSON, err := encodeMetadata(msg.Metadata)
	if err != nil {
		return Message{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	var nextSeq int64
	row := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM messages WHERE session_id = ?`, msg.SessionID)
	if err := row.Scan(&nextSeq); err != nil {
		return Message{}, fmt.Errorf("compute message sequence: %w", err)
	}
	msg.Sequence = nextSeq
	_, err = tx.ExecContext(ctx, `
INSERT INTO messages (id, session_id, sequence, role, content, payload_ref, correlation_id, created_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.SessionID,
		msg.Sequence,
		string(msg.Role),
		nullableString(msg.Content),
		nullableString(msg.PayloadRef),
		nullableString(msg.CorrelationID),
		msg.CreatedAt.Format(time.RFC3339Nano),
		metaJSON,
	)
	if err != nil {
		return Message{}, fmt.Errorf("insert message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("commit message: %w", err)
	}
	return msg, nil
}

// ListBySession returns the messages of a session in chronological
// order. limit <= 0 returns everything.
func (r *MessageRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("persistence: session_id is required")
	}
	query := `
SELECT id, session_id, sequence, role, content, payload_ref, correlation_id, created_at, metadata
FROM messages WHERE session_id = ?
ORDER BY sequence ASC`
	args := []any{sessionID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

// SinceSequence returns messages newer than the supplied sequence
// number. The result is ordered ascending so SSE replay is correct.
func (r *MessageRepository) SinceSequence(ctx context.Context, sessionID string, sequence int64) ([]Message, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, session_id, sequence, role, content, payload_ref, correlation_id, created_at, metadata
FROM messages WHERE session_id = ? AND sequence > ?
ORDER BY sequence ASC`, sessionID, sequence)
	if err != nil {
		return nil, fmt.Errorf("list messages since: %w", err)
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func scanMessage(scanner rowScanner) (Message, error) {
	var (
		msg                                Message
		content, payloadRef, corr, metaStr sql.NullString
		createdAt                          string
	)
	if err := scanner.Scan(
		&msg.ID,
		&msg.SessionID,
		&msg.Sequence,
		&msg.Role,
		&content,
		&payloadRef,
		&corr,
		&createdAt,
		&metaStr,
	); err != nil {
		return Message{}, err
	}
	if content.Valid {
		msg.Content = content.String
	}
	if payloadRef.Valid {
		msg.PayloadRef = payloadRef.String
	}
	if corr.Valid {
		msg.CorrelationID = corr.String
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Message{}, fmt.Errorf("parse created_at: %w", err)
	}
	msg.CreatedAt = t
	if metaStr.Valid && metaStr.String != "" {
		if err := json.Unmarshal([]byte(metaStr.String), &msg.Metadata); err != nil {
			return Message{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return msg, nil
}
