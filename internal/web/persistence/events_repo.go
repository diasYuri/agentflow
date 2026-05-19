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

// FrontendEventRepository persists FrontendEvent rows.
type FrontendEventRepository struct {
	db *sql.DB
}

// NewFrontendEventRepository wires the repo.
func NewFrontendEventRepository(db *DB) *FrontendEventRepository {
	return &FrontendEventRepository{db: db.SQL()}
}

// Insert records a frontend event row.
func (r *FrontendEventRepository) Insert(ctx context.Context, event FrontendEvent) (FrontendEvent, error) {
	if strings.TrimSpace(event.Kind) == "" {
		return FrontendEvent{}, errors.New("persistence: event kind is required")
	}
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	metaJSON, err := encodeMetadata(event.Metadata)
	if err != nil {
		return FrontendEvent{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO frontend_events (id, session_id, kind, content, payload_ref, correlation_id, metadata, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		nullableString(event.SessionID),
		event.Kind,
		nullableString(event.Content),
		nullableString(event.PayloadRef),
		nullableString(event.CorrelationID),
		metaJSON,
		event.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return FrontendEvent{}, fmt.Errorf("insert frontend_event: %w", err)
	}
	return event, nil
}

// ListBySession returns frontend events ordered by creation time.
func (r *FrontendEventRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]FrontendEvent, error) {
	query := `
SELECT id, session_id, kind, content, payload_ref, correlation_id, metadata, created_at
FROM frontend_events WHERE session_id = ? ORDER BY created_at DESC`
	args := []any{sessionID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list frontend_events: %w", err)
	}
	defer rows.Close()
	var events []FrontendEvent
	for rows.Next() {
		event, err := scanFrontendEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func scanFrontendEvent(scanner rowScanner) (FrontendEvent, error) {
	var (
		event                                            FrontendEvent
		sessionID, content, payloadRef, corr, metaStr   sql.NullString
		createdAt                                       string
	)
	if err := scanner.Scan(
		&event.ID,
		&sessionID,
		&event.Kind,
		&content,
		&payloadRef,
		&corr,
		&metaStr,
		&createdAt,
	); err != nil {
		return FrontendEvent{}, err
	}
	if sessionID.Valid {
		event.SessionID = sessionID.String
	}
	if content.Valid {
		event.Content = content.String
	}
	if payloadRef.Valid {
		event.PayloadRef = payloadRef.String
	}
	if corr.Valid {
		event.CorrelationID = corr.String
	}
	if metaStr.Valid && metaStr.String != "" {
		if err := json.Unmarshal([]byte(metaStr.String), &event.Metadata); err != nil {
			return FrontendEvent{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return FrontendEvent{}, fmt.Errorf("parse created_at: %w", err)
	}
	event.CreatedAt = t
	return event, nil
}
