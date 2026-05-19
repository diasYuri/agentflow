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
	return nil
}
