package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DefaultDatabaseName is the file inside the web data directory that
// holds the chat history database.
const DefaultDatabaseName = "web-sessions.sqlite"

// DefaultPath returns the canonical location of the web SQLite file
// inside an AgentFlow root directory.
func DefaultPath(root string) string {
	return filepath.Join(root, "web", DefaultDatabaseName)
}

// DB wraps the underlying *sql.DB so callers depend on a small, well
// scoped surface even when the implementation grows.
type DB struct {
	sql  *sql.DB
	path string
}

// Open creates or opens the SQLite database at path and applies all
// pending schema migrations. The parent directory is created lazily.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("persistence: path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	pragmas := "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	conn, err := sql.Open("sqlite", path+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)
	db := &DB{sql: conn, path: path}
	if err := db.migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return db, nil
}

// Path returns the SQLite file path on disk.
func (db *DB) Path() string { return db.path }

// SQL exposes the underlying *sql.DB for repositories within this
// package. The method is intentionally not exported through a wider
// interface so other packages cannot bypass the repository helpers.
func (db *DB) SQL() *sql.DB { return db.sql }

// Close releases the underlying connection.
func (db *DB) Close() error { return db.sql.Close() }

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if err := db.applySchemaUpgrades(ctx); err != nil {
		return err
	}
	return nil
}

func (db *DB) applySchemaUpgrades(ctx context.Context) error {
	columns := map[string]string{
		"source":                "TEXT NOT NULL DEFAULT 'web'",
		"external_key":          "TEXT",
		"external_workspace_id": "TEXT",
		"external_channel_id":   "TEXT",
		"external_thread_id":    "TEXT",
		"external_user_id":      "TEXT",
	}
	for name, spec := range columns {
		exists, err := db.columnExists(ctx, "sessions", name)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := db.sql.ExecContext(ctx, fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s %s", name, spec)); err != nil {
			return fmt.Errorf("add sessions.%s: %w", name, err)
		}
	}
	if err := db.ensureNullableSessionProject(ctx); err != nil {
		return err
	}
	if _, err := db.sql.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_external_key ON sessions(external_key) WHERE external_key IS NOT NULL`); err != nil {
		return fmt.Errorf("create sessions external key index: %w", err)
	}
	return nil
}

func (db *DB) ensureNullableSessionProject(ctx context.Context) error {
	needsRebuild, err := db.sessionProjectColumnsNeedRebuild(ctx)
	if err != nil {
		return err
	}
	if !needsRebuild {
		return nil
	}
	if _, err := db.sql.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys for sessions rebuild: %w", err)
	}
	defer db.sql.ExecContext(ctx, `PRAGMA foreign_keys = ON`)
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE sessions_new (
    id TEXT PRIMARY KEY,
    project_name TEXT,
    project_path TEXT,
    title TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    provider TEXT,
    model TEXT,
    source TEXT NOT NULL DEFAULT 'web',
    external_key TEXT,
    external_workspace_id TEXT,
    external_channel_id TEXT,
    external_thread_id TEXT,
    external_user_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_message_at TEXT,
    metadata TEXT
)`); err != nil {
		return fmt.Errorf("create nullable sessions table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions_new (
    id, project_name, project_path, title, status, provider, model,
    source, external_key, external_workspace_id, external_channel_id, external_thread_id, external_user_id,
    created_at, updated_at, last_message_at, metadata
)
SELECT
    id, project_name, project_path, title, status, provider, model,
    source, external_key, external_workspace_id, external_channel_id, external_thread_id, external_user_id,
    created_at, updated_at, last_message_at, metadata
FROM sessions`); err != nil {
		return fmt.Errorf("copy sessions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE sessions`); err != nil {
		return fmt.Errorf("drop old sessions table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE sessions_new RENAME TO sessions`); err != nil {
		return fmt.Errorf("rename sessions table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_name, created_at DESC)`); err != nil {
		return fmt.Errorf("recreate sessions project index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status)`); err != nil {
		return fmt.Errorf("recreate sessions status index: %w", err)
	}
	return tx.Commit()
}

func (db *DB) sessionProjectColumnsNeedRebuild(ctx context.Context) (bool, error) {
	rows, err := db.sql.QueryContext(ctx, "PRAGMA table_info(sessions)")
	if err != nil {
		return false, fmt.Errorf("inspect sessions schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if (name == "project_name" || name == "project_path") && notNull != 0 {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (db *DB) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := db.sql.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
