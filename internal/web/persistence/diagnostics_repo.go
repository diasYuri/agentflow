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

// DiagnosticRepository persists Diagnostic rows.
type DiagnosticRepository struct {
	db *sql.DB
}

// NewDiagnosticRepository wires the repo.
func NewDiagnosticRepository(db *DB) *DiagnosticRepository {
	return &DiagnosticRepository{db: db.SQL()}
}

// Insert stores a diagnostic event.
func (r *DiagnosticRepository) Insert(ctx context.Context, diag Diagnostic) (Diagnostic, error) {
	if diag.Level == "" {
		return Diagnostic{}, errors.New("persistence: diagnostic level is required")
	}
	if strings.TrimSpace(diag.Source) == "" {
		return Diagnostic{}, errors.New("persistence: diagnostic source is required")
	}
	if strings.TrimSpace(diag.Message) == "" {
		return Diagnostic{}, errors.New("persistence: diagnostic message is required")
	}
	if diag.ID == "" {
		diag.ID = uuid.NewString()
	}
	if diag.CreatedAt.IsZero() {
		diag.CreatedAt = time.Now().UTC()
	}
	ctxJSON, err := encodeMetadata(diag.Context)
	if err != nil {
		return Diagnostic{}, err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO diagnostics (id, session_id, level, source, code, message, context, correlation_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		diag.ID,
		nullableString(diag.SessionID),
		string(diag.Level),
		diag.Source,
		nullableString(diag.Code),
		diag.Message,
		ctxJSON,
		nullableString(diag.CorrelationID),
		diag.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Diagnostic{}, fmt.Errorf("insert diagnostic: %w", err)
	}
	return diag, nil
}

// ListBySession returns diagnostics ordered by recency.
func (r *DiagnosticRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]Diagnostic, error) {
	query := `
SELECT id, session_id, level, source, code, message, context, correlation_id, created_at
FROM diagnostics
WHERE session_id = ?
ORDER BY created_at DESC`
	args := []any{sessionID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list diagnostics: %w", err)
	}
	defer rows.Close()
	var diags []Diagnostic
	for rows.Next() {
		diag, err := scanDiagnostic(rows)
		if err != nil {
			return nil, err
		}
		diags = append(diags, diag)
	}
	return diags, rows.Err()
}

// ListRecent returns the most recent diagnostics regardless of session.
func (r *DiagnosticRepository) ListRecent(ctx context.Context, limit int) ([]Diagnostic, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id, session_id, level, source, code, message, context, correlation_id, created_at
FROM diagnostics
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list diagnostics: %w", err)
	}
	defer rows.Close()
	var diags []Diagnostic
	for rows.Next() {
		diag, err := scanDiagnostic(rows)
		if err != nil {
			return nil, err
		}
		diags = append(diags, diag)
	}
	return diags, rows.Err()
}

func scanDiagnostic(scanner rowScanner) (Diagnostic, error) {
	var (
		diag                    Diagnostic
		sessionID               sql.NullString
		code, corr, ctxStr      sql.NullString
		createdAt               string
	)
	if err := scanner.Scan(
		&diag.ID,
		&sessionID,
		&diag.Level,
		&diag.Source,
		&code,
		&diag.Message,
		&ctxStr,
		&corr,
		&createdAt,
	); err != nil {
		return Diagnostic{}, err
	}
	if sessionID.Valid {
		diag.SessionID = sessionID.String
	}
	if code.Valid {
		diag.Code = code.String
	}
	if corr.Valid {
		diag.CorrelationID = corr.String
	}
	if ctxStr.Valid && ctxStr.String != "" {
		if err := json.Unmarshal([]byte(ctxStr.String), &diag.Context); err != nil {
			return Diagnostic{}, fmt.Errorf("decode diagnostic context: %w", err)
		}
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Diagnostic{}, fmt.Errorf("parse created_at: %w", err)
	}
	diag.CreatedAt = t
	return diag, nil
}
