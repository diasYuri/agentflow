package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type RunStore interface {
	LoadRuns(ctx context.Context) ([]WorkflowRun, error)
	UpsertRun(ctx context.Context, run WorkflowRun) error
	Close() error
}

type SQLiteRunStore struct {
	db *sql.DB
}

func OpenSQLiteRunStore(ctx context.Context, path string) (*SQLiteRunStore, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteRunStore{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteRunStore) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS workflow_runs (
	id TEXT PRIMARY KEY,
	workflow TEXT NOT NULL,
	run_dir TEXT NOT NULL,
	status TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	error TEXT,
	current_step TEXT,
	completed_steps TEXT,
	pending_steps TEXT,
	total_steps INTEGER,
	terminal_error TEXT,
	failure_reason TEXT,
	recent_events TEXT,
	paused_at TEXT,
	approval_at TEXT,
	pause_reason TEXT,
	approval_node_id TEXT,
	approval_message TEXT,
	resume_count INTEGER,
	request_json TEXT,
	tag TEXT,
	priority INTEGER,
	queued_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_started_at ON workflow_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);
CREATE TABLE IF NOT EXISTS workflow_definitions (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	spec_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_definitions_name ON workflow_definitions(name);
CREATE INDEX IF NOT EXISTS idx_workflow_definitions_updated_at ON workflow_definitions(updated_at DESC);
`)
	if err != nil {
		return err
	}
	cols := []string{
		"current_step TEXT",
		"completed_steps TEXT",
		"pending_steps TEXT",
		"total_steps INTEGER",
		"terminal_error TEXT",
		"failure_reason TEXT",
		"recent_events TEXT",
		"paused_at TEXT",
		"approval_at TEXT",
		"pause_reason TEXT",
		"approval_node_id TEXT",
		"approval_message TEXT",
		"resume_count INTEGER",
		"request_json TEXT",
		"tag TEXT",
		"priority INTEGER",
		"queued_at TEXT",
	}
	for _, col := range cols {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE workflow_runs ADD COLUMN `+col); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return err
			}
		}
	}
	return nil
}

func (s *SQLiteRunStore) LoadWorkflowDefinitions(ctx context.Context) ([]WorkflowDefinitionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, spec_json, created_at, updated_at
FROM workflow_definitions
ORDER BY updated_at DESC, created_at DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []WorkflowDefinitionRecord
	for rows.Next() {
		var record WorkflowDefinitionRecord
		var createdAt, updatedAt string
		if err := rows.Scan(&record.ID, &record.Name, &record.SpecJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at for workflow definition %q: %w", record.ID, err)
		}
		parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at for workflow definition %q: %w", record.ID, err)
		}
		record.CreatedAt = parsedCreatedAt
		record.UpdatedAt = parsedUpdatedAt
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteRunStore) GetWorkflowDefinition(ctx context.Context, id string) (WorkflowDefinitionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, spec_json, created_at, updated_at
FROM workflow_definitions
WHERE id = ?`, strings.TrimSpace(id))
	return scanWorkflowDefinitionRecord(row)
}

func (s *SQLiteRunStore) GetWorkflowDefinitionByName(ctx context.Context, name string) (WorkflowDefinitionRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, spec_json, created_at, updated_at
FROM workflow_definitions
WHERE name = ?
ORDER BY updated_at DESC, created_at DESC
LIMIT 2`, strings.TrimSpace(name))
	if err != nil {
		return WorkflowDefinitionRecord{}, err
	}
	defer rows.Close()

	var records []WorkflowDefinitionRecord
	for rows.Next() {
		record, err := scanWorkflowDefinitionRows(rows)
		if err != nil {
			return WorkflowDefinitionRecord{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return WorkflowDefinitionRecord{}, err
	}
	switch len(records) {
	case 0:
		return WorkflowDefinitionRecord{}, os.ErrNotExist
	case 1:
		return records[0], nil
	default:
		return WorkflowDefinitionRecord{}, fmt.Errorf("duplicate workflow definition name %q", name)
	}
}

func scanWorkflowDefinitionRecord(row rowScanner) (WorkflowDefinitionRecord, error) {
	var record WorkflowDefinitionRecord
	var createdAt, updatedAt string
	if err := row.Scan(&record.ID, &record.Name, &record.SpecJSON, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkflowDefinitionRecord{}, os.ErrNotExist
		}
		return WorkflowDefinitionRecord{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("parse created_at for workflow definition %q: %w", record.ID, err)
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("parse updated_at for workflow definition %q: %w", record.ID, err)
	}
	record.CreatedAt = parsedCreatedAt
	record.UpdatedAt = parsedUpdatedAt
	return record, nil
}

func scanWorkflowDefinitionRows(rows *sql.Rows) (WorkflowDefinitionRecord, error) {
	var record WorkflowDefinitionRecord
	var createdAt, updatedAt string
	if err := rows.Scan(&record.ID, &record.Name, &record.SpecJSON, &createdAt, &updatedAt); err != nil {
		return WorkflowDefinitionRecord{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("parse created_at for workflow definition %q: %w", record.ID, err)
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return WorkflowDefinitionRecord{}, fmt.Errorf("parse updated_at for workflow definition %q: %w", record.ID, err)
	}
	record.CreatedAt = parsedCreatedAt
	record.UpdatedAt = parsedUpdatedAt
	return record, nil
}

func (s *SQLiteRunStore) LoadRuns(ctx context.Context) ([]WorkflowRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workflow, run_dir, status, started_at, finished_at, error, current_step, completed_steps, pending_steps, total_steps, terminal_error, failure_reason, recent_events, paused_at, approval_at, pause_reason, approval_node_id, approval_message, resume_count, request_json, tag, priority, queued_at
FROM workflow_runs
ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []WorkflowRun
	for rows.Next() {
		var run WorkflowRun
		var startedAt string
		var finishedAt, pausedAt, approvalAt sql.NullString
		var completedSteps, pendingSteps, recentEvents sql.NullString
		var runError, currentStep, terminalError, failureReason sql.NullString
		var pauseReason, approvalNodeID, approvalMessage, requestJSON sql.NullString
		var tag sql.NullString
		var totalSteps, resumeCount sql.NullInt64
		var priority sql.NullInt64
		var queuedAt sql.NullString
		if err := rows.Scan(
			&run.ID, &run.Workflow, &run.RunDir, &run.Status,
			&startedAt, &finishedAt, &runError,
			&currentStep, &completedSteps, &pendingSteps,
			&totalSteps, &terminalError, &failureReason, &recentEvents,
			&pausedAt, &approvalAt, &pauseReason, &approvalNodeID, &approvalMessage, &resumeCount, &requestJSON, &tag,
			&priority, &queuedAt,
		); err != nil {
			return nil, err
		}
		if runError.Valid {
			run.Error = runError.String
		}
		if currentStep.Valid {
			run.CurrentStep = currentStep.String
		}
		if terminalError.Valid {
			run.TerminalError = terminalError.String
		}
		if failureReason.Valid {
			run.FailureReason = failureReason.String
		}
		if tag.Valid {
			run.Tag = tag.String
		}
		if priority.Valid {
			run.Priority = int(priority.Int64)
		}
		if queuedAt.Valid && queuedAt.String != "" {
			parsedQueuedAt, err := time.Parse(time.RFC3339Nano, queuedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse queued_at for run %q: %w", run.ID, err)
			}
			run.QueuedAt = parsedQueuedAt
		}
		parsedStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return nil, fmt.Errorf("parse started_at for run %q: %w", run.ID, err)
		}
		run.StartedAt = parsedStartedAt
		if finishedAt.Valid && finishedAt.String != "" {
			parsedFinishedAt, err := time.Parse(time.RFC3339Nano, finishedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse finished_at for run %q: %w", run.ID, err)
			}
			run.FinishedAt = parsedFinishedAt
		}
		if pausedAt.Valid && pausedAt.String != "" {
			parsedPausedAt, err := time.Parse(time.RFC3339Nano, pausedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse paused_at for run %q: %w", run.ID, err)
			}
			run.PausedAt = parsedPausedAt
		}
		if approvalAt.Valid && approvalAt.String != "" {
			parsedApprovalAt, err := time.Parse(time.RFC3339Nano, approvalAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse approval_at for run %q: %w", run.ID, err)
			}
			run.ApprovalAt = parsedApprovalAt
		}
		if pauseReason.Valid {
			run.PauseReason = pauseReason.String
		}
		if approvalNodeID.Valid {
			run.ApprovalNodeID = approvalNodeID.String
		}
		if approvalMessage.Valid {
			run.ApprovalMessage = approvalMessage.String
		}
		if resumeCount.Valid {
			run.ResumeCount = int(resumeCount.Int64)
		}
		if completedSteps.Valid && completedSteps.String != "" {
			_ = json.Unmarshal([]byte(completedSteps.String), &run.CompletedSteps)
		}
		if pendingSteps.Valid && pendingSteps.String != "" {
			_ = json.Unmarshal([]byte(pendingSteps.String), &run.PendingSteps)
		}
		if totalSteps.Valid {
			run.TotalSteps = int(totalSteps.Int64)
		}
		if recentEvents.Valid && recentEvents.String != "" {
			_ = json.Unmarshal([]byte(recentEvents.String), &run.RecentEvents)
		}
		if requestJSON.Valid && requestJSON.String != "" {
			req := &RunWorkflowRequest{}
			if err := json.Unmarshal([]byte(requestJSON.String), req); err == nil {
				run.Request = req
			}
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *SQLiteRunStore) UpsertRun(ctx context.Context, run WorkflowRun) error {
	var finishedAt, pausedAt, approvalAt any
	if !run.FinishedAt.IsZero() {
		finishedAt = run.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.PausedAt.IsZero() {
		pausedAt = run.PausedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.ApprovalAt.IsZero() {
		approvalAt = run.ApprovalAt.UTC().Format(time.RFC3339Nano)
	}
	completedSteps, _ := json.Marshal(run.CompletedSteps)
	pendingSteps, _ := json.Marshal(run.PendingSteps)
	recentEvents, _ := json.Marshal(run.RecentEvents)
	var requestJSON any
	if run.Request != nil {
		if data, err := json.Marshal(run.Request); err == nil {
			requestJSON = string(data)
		}
	}
	var queuedAt any
	if !run.QueuedAt.IsZero() {
		queuedAt = run.QueuedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO workflow_runs (id, workflow, run_dir, status, started_at, finished_at, error, current_step, completed_steps, pending_steps, total_steps, terminal_error, failure_reason, recent_events, paused_at, approval_at, pause_reason, approval_node_id, approval_message, resume_count, request_json, tag, priority, queued_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	workflow = excluded.workflow,
	run_dir = excluded.run_dir,
	status = excluded.status,
	started_at = excluded.started_at,
	finished_at = excluded.finished_at,
	error = excluded.error,
	current_step = excluded.current_step,
	completed_steps = excluded.completed_steps,
	pending_steps = excluded.pending_steps,
	total_steps = excluded.total_steps,
	terminal_error = excluded.terminal_error,
	failure_reason = excluded.failure_reason,
	recent_events = excluded.recent_events,
	paused_at = excluded.paused_at,
	approval_at = excluded.approval_at,
	pause_reason = excluded.pause_reason,
	approval_node_id = excluded.approval_node_id,
	approval_message = excluded.approval_message,
	resume_count = excluded.resume_count,
	request_json = COALESCE(excluded.request_json, workflow_runs.request_json),
	tag = COALESCE(excluded.tag, workflow_runs.tag),
	priority = excluded.priority,
	queued_at = excluded.queued_at`,
		run.ID,
		run.Workflow,
		run.RunDir,
		run.Status,
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		finishedAt,
		run.Error,
		run.CurrentStep,
		string(completedSteps),
		string(pendingSteps),
		run.TotalSteps,
		run.TerminalError,
		run.FailureReason,
		string(recentEvents),
		pausedAt,
		approvalAt,
		run.PauseReason,
		run.ApprovalNodeID,
		run.ApprovalMessage,
		run.ResumeCount,
		requestJSON,
		run.Tag,
		run.Priority,
		queuedAt,
	)
	return err
}

func (s *SQLiteRunStore) Close() error {
	return s.db.Close()
}
