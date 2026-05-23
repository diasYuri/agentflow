package persistence

// schemaSQL contains every table and index used by the web store.
// The migrations are idempotent so re-running the same statement on
// existing data is safe; columns added later use ALTER TABLE inside
// applySchemaUpgrades.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project_name TEXT NOT NULL,
    project_path TEXT NOT NULL,
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
);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT,
    payload_ref TEXT,
    correlation_id TEXT,
    created_at TEXT NOT NULL,
    metadata TEXT,
    UNIQUE (session_id, sequence)
);
CREATE INDEX IF NOT EXISTS idx_messages_session_created ON messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_correlation ON messages(correlation_id);

CREATE TABLE IF NOT EXISTS tool_calls (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    message_id TEXT,
    name TEXT NOT NULL,
    status TEXT NOT NULL,
    request_ref TEXT,
    response_ref TEXT,
    error TEXT,
    correlation_id TEXT,
    started_at TEXT NOT NULL,
    finished_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tool_calls_session ON tool_calls(session_id, started_at);
CREATE INDEX IF NOT EXISTS idx_tool_calls_correlation ON tool_calls(correlation_id);

CREATE TABLE IF NOT EXISTS approvals (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    tool_call_id TEXT,
    status TEXT NOT NULL,
    reason TEXT,
    decided_by TEXT,
    created_at TEXT NOT NULL,
    decided_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_approvals_session ON approvals(session_id, created_at);

CREATE TABLE IF NOT EXISTS diagnostics (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    level TEXT NOT NULL,
    source TEXT NOT NULL,
    code TEXT,
    message TEXT NOT NULL,
    context TEXT,
    correlation_id TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_diagnostics_session ON diagnostics(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_diagnostics_correlation ON diagnostics(correlation_id);
CREATE INDEX IF NOT EXISTS idx_diagnostics_level ON diagnostics(level);

CREATE TABLE IF NOT EXISTS frontend_events (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    kind TEXT NOT NULL,
    content TEXT,
    payload_ref TEXT,
    correlation_id TEXT,
    metadata TEXT,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_frontend_events_session ON frontend_events(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_frontend_events_correlation ON frontend_events(correlation_id);

CREATE TABLE IF NOT EXISTS payload_store (
    id TEXT PRIMARY KEY,
    sha256 TEXT NOT NULL,
    content_type TEXT,
    size_bytes INTEGER NOT NULL,
    body BLOB NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_payload_sha ON payload_store(sha256);
`
