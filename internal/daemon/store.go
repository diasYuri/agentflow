package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
	error TEXT
);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_started_at ON workflow_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON workflow_runs(status);
`)
	return err
}

func (s *SQLiteRunStore) LoadRuns(ctx context.Context) ([]WorkflowRun, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workflow, run_dir, status, started_at, finished_at, error
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
		var finishedAt sql.NullString
		if err := rows.Scan(&run.ID, &run.Workflow, &run.RunDir, &run.Status, &startedAt, &finishedAt, &run.Error); err != nil {
			return nil, err
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
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *SQLiteRunStore) UpsertRun(ctx context.Context, run WorkflowRun) error {
	var finishedAt any
	if !run.FinishedAt.IsZero() {
		finishedAt = run.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO workflow_runs (id, workflow, run_dir, status, started_at, finished_at, error)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	workflow = excluded.workflow,
	run_dir = excluded.run_dir,
	status = excluded.status,
	started_at = excluded.started_at,
	finished_at = excluded.finished_at,
	error = excluded.error`,
		run.ID,
		run.Workflow,
		run.RunDir,
		run.Status,
		run.StartedAt.UTC().Format(time.RFC3339Nano),
		finishedAt,
		run.Error,
	)
	return err
}

func (s *SQLiteRunStore) Close() error {
	return s.db.Close()
}
