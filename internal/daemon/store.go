package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

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
	pause_reason TEXT,
	resume_count INTEGER,
	request_json TEXT,
	tag TEXT
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_started_at ON workflow_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);
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
		"pause_reason TEXT",
		"resume_count INTEGER",
		"request_json TEXT",
		"tag TEXT",
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

func (s *SQLiteRunStore) LoadRuns(ctx context.Context) ([]WorkflowRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workflow, run_dir, status, started_at, finished_at, error, current_step, completed_steps, pending_steps, total_steps, terminal_error, failure_reason, recent_events, paused_at, pause_reason, resume_count, request_json, tag
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
		var finishedAt, pausedAt sql.NullString
		var completedSteps, pendingSteps, recentEvents sql.NullString
		var runError, currentStep, terminalError, failureReason sql.NullString
		var pauseReason, requestJSON sql.NullString
		var tag sql.NullString
		var totalSteps, resumeCount sql.NullInt64
		if err := rows.Scan(
			&run.ID, &run.Workflow, &run.RunDir, &run.Status,
			&startedAt, &finishedAt, &runError,
			&currentStep, &completedSteps, &pendingSteps,
			&totalSteps, &terminalError, &failureReason, &recentEvents,
			&pausedAt, &pauseReason, &resumeCount, &requestJSON, &tag,
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
		if pauseReason.Valid {
			run.PauseReason = pauseReason.String
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
	var finishedAt, pausedAt any
	if !run.FinishedAt.IsZero() {
		finishedAt = run.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	if !run.PausedAt.IsZero() {
		pausedAt = run.PausedAt.UTC().Format(time.RFC3339Nano)
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
	_, err := s.db.ExecContext(ctx, `
INSERT INTO workflow_runs (id, workflow, run_dir, status, started_at, finished_at, error, current_step, completed_steps, pending_steps, total_steps, terminal_error, failure_reason, recent_events, paused_at, pause_reason, resume_count, request_json, tag)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
	pause_reason = excluded.pause_reason,
	resume_count = excluded.resume_count,
	request_json = COALESCE(excluded.request_json, workflow_runs.request_json),
	tag = COALESCE(excluded.tag, workflow_runs.tag)`,
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
		run.PauseReason,
		run.ResumeCount,
		requestJSON,
		run.Tag,
	)
	return err
}

func (s *SQLiteRunStore) Close() error {
	return s.db.Close()
}
