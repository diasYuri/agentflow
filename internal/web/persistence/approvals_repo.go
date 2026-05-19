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

// ApprovalRepository persists Approval rows.
type ApprovalRepository struct {
	db *sql.DB
}

// NewApprovalRepository returns a repository on top of db.
func NewApprovalRepository(db *DB) *ApprovalRepository {
	return &ApprovalRepository{db: db.SQL()}
}

// ErrApprovalNotFound is returned when no approval row matches.
var ErrApprovalNotFound = errors.New("persistence: approval not found")

// Create stores a new approval in pending state.
func (r *ApprovalRepository) Create(ctx context.Context, approval Approval) (Approval, error) {
	if strings.TrimSpace(approval.SessionID) == "" {
		return Approval{}, errors.New("persistence: approval session_id is required")
	}
	if approval.ID == "" {
		approval.ID = uuid.NewString()
	}
	if approval.Status == "" {
		approval.Status = ApprovalStatusPending
	}
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO approvals (id, session_id, tool_call_id, status, reason, decided_by, created_at, decided_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ID,
		approval.SessionID,
		nullableString(approval.ToolCallID),
		string(approval.Status),
		nullableString(approval.Reason),
		nullableString(approval.DecidedBy),
		approval.CreatedAt.Format(time.RFC3339Nano),
		nullableTime(approval.DecidedAt),
	)
	if err != nil {
		return Approval{}, fmt.Errorf("insert approval: %w", err)
	}
	return approval, nil
}

// Decide records the human decision on an approval row.
func (r *ApprovalRepository) Decide(ctx context.Context, id string, status ApprovalStatus, reason, decidedBy string) error {
	res, err := r.db.ExecContext(ctx, `
UPDATE approvals SET status = ?, reason = COALESCE(?, reason), decided_by = COALESCE(?, decided_by), decided_at = ? WHERE id = ?`,
		string(status),
		nullableString(reason),
		nullableString(decidedBy),
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
		return ErrApprovalNotFound
	}
	return nil
}

// ListBySession returns approvals ordered by creation time.
func (r *ApprovalRepository) ListBySession(ctx context.Context, sessionID string) ([]Approval, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, session_id, tool_call_id, status, reason, decided_by, created_at, decided_at
FROM approvals WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list approvals: %w", err)
	}
	defer rows.Close()
	var approvals []Approval
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func scanApproval(scanner rowScanner) (Approval, error) {
	var (
		approval                            Approval
		toolCall, reason, decidedBy, decided sql.NullString
		createdAt                            string
	)
	if err := scanner.Scan(
		&approval.ID,
		&approval.SessionID,
		&toolCall,
		&approval.Status,
		&reason,
		&decidedBy,
		&createdAt,
		&decided,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Approval{}, ErrApprovalNotFound
		}
		return Approval{}, err
	}
	if toolCall.Valid {
		approval.ToolCallID = toolCall.String
	}
	if reason.Valid {
		approval.Reason = reason.String
	}
	if decidedBy.Valid {
		approval.DecidedBy = decidedBy.String
	}
	t, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Approval{}, fmt.Errorf("parse approval created_at: %w", err)
	}
	approval.CreatedAt = t
	if decided.Valid && decided.String != "" {
		t, err = time.Parse(time.RFC3339Nano, decided.String)
		if err != nil {
			return Approval{}, fmt.Errorf("parse approval decided_at: %w", err)
		}
		approval.DecidedAt = t
	}
	return approval, nil
}
