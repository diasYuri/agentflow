package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SessionRepository persists Session records.
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository wires a SessionRepository on top of db.
func NewSessionRepository(db *DB) *SessionRepository {
	return &SessionRepository{db: db.SQL()}
}

// ErrSessionNotFound is returned when no matching session exists.
var ErrSessionNotFound = errors.New("persistence: session not found")

// Create inserts a brand-new session row. CreatedAt/UpdatedAt are
// populated from time.Now if the caller left them zero so the public
// HTTP layer does not have to.
func (r *SessionRepository) Create(ctx context.Context, session Session) (Session, error) {
	if strings.TrimSpace(session.ID) == "" {
		return Session{}, errors.New("persistence: session id is required")
	}
	if strings.TrimSpace(session.ProjectName) == "" {
		return Session{}, errors.New("persistence: project_name is required")
	}
	if strings.TrimSpace(session.ProjectPath) == "" {
		return Session{}, errors.New("persistence: project_path is required")
	}
	now := time.Now().UTC()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
	if session.Status == "" {
		session.Status = SessionStatusOpen
	}
	metaJSON, err := encodeMetadata(session.Metadata)
	if err != nil {
		return Session{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO sessions (id, project_name, project_path, title, status, provider, model, created_at, updated_at, last_message_at, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.ProjectName,
		session.ProjectPath,
		nullableString(session.Title),
		string(session.Status),
		nullableString(session.Provider),
		nullableString(session.Model),
		session.CreatedAt.Format(time.RFC3339Nano),
		session.UpdatedAt.Format(time.RFC3339Nano),
		nullableTime(session.LastMessageAt),
		metaJSON,
	)
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}
	return session, nil
}

// Get returns the session with id.
func (r *SessionRepository) Get(ctx context.Context, id string) (Session, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, project_name, project_path, title, status, provider, model, created_at, updated_at, last_message_at, metadata
FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

// ListByProject returns all sessions for the named project ordered by
// most recently active first. An empty project name returns every
// session in the database.
func (r *SessionRepository) ListByProject(ctx context.Context, projectName string) ([]Session, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(projectName) == "" {
		rows, err = r.db.QueryContext(ctx, `
SELECT id, project_name, project_path, title, status, provider, model, created_at, updated_at, last_message_at, metadata
FROM sessions
ORDER BY COALESCE(last_message_at, updated_at) DESC, created_at DESC`)
	} else {
		rows, err = r.db.QueryContext(ctx, `
SELECT id, project_name, project_path, title, status, provider, model, created_at, updated_at, last_message_at, metadata
FROM sessions WHERE project_name = ?
ORDER BY COALESCE(last_message_at, updated_at) DESC, created_at DESC`, projectName)
	}
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

// UpdateLastMessageAt records when the most recent message landed.
func (r *SessionRepository) UpdateLastMessageAt(ctx context.Context, id string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET last_message_at = ?, updated_at = ? WHERE id = ?`,
		at.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// SetStatus marks a session open or archived.
func (r *SessionRepository) SetStatus(ctx context.Context, id string, status SessionStatus) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// SetTitle updates the human-friendly session title.
func (r *SessionRepository) SetTitle(ctx context.Context, id, title string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`,
		nullableString(title), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// Delete removes a session and all rows that cascade from it.
func (r *SessionRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func scanSession(scanner rowScanner) (Session, error) {
	var (
		session                                         Session
		title, provider, model, lastMessageAt, metaStr sql.NullString
		createdAt, updatedAt                            string
	)
	if err := scanner.Scan(
		&session.ID,
		&session.ProjectName,
		&session.ProjectPath,
		&title,
		&session.Status,
		&provider,
		&model,
		&createdAt,
		&updatedAt,
		&lastMessageAt,
		&metaStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	if title.Valid {
		session.Title = title.String
	}
	if provider.Valid {
		session.Provider = provider.String
	}
	if model.Valid {
		session.Model = model.String
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse created_at: %w", err)
	}
	session.CreatedAt = t
	t, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse updated_at: %w", err)
	}
	session.UpdatedAt = t
	if lastMessageAt.Valid && lastMessageAt.String != "" {
		t, err = time.Parse(time.RFC3339Nano, lastMessageAt.String)
		if err != nil {
			return Session{}, fmt.Errorf("parse last_message_at: %w", err)
		}
		session.LastMessageAt = t
	}
	if metaStr.Valid && metaStr.String != "" {
		if err := json.Unmarshal([]byte(metaStr.String), &session.Metadata); err != nil {
			return Session{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return session, nil
}

// rowScanner is the common subset of sql.Row and sql.Rows we use to
// share scan code between query helpers.
type rowScanner interface {
	Scan(dest ...any) error
}

func encodeMetadata(meta map[string]any) (any, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}
	return string(data), nil
}

func nullableString(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}
