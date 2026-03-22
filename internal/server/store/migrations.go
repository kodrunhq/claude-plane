package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Migration represents a single schema migration with a version number and
// the SQL to execute. Migrations are applied in order and each runs inside
// its own transaction.
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// migrations is the ordered list of all schema migrations. New migrations
// should be appended to the end with the next sequential version number.
// Existing migrations must never be modified once released.
var migrations = []Migration{
	{
		Version:     1,
		Description: "initial schema",
		SQL: `
-- Users (needed for AUTH-04 admin seeding, expanded in Phase 3)
CREATE TABLE IF NOT EXISTS users (
    user_id        TEXT PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    display_name   TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    role           TEXT NOT NULL DEFAULT 'user',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Machines
CREATE TABLE IF NOT EXISTS machines (
    machine_id     TEXT PRIMARY KEY,
    display_name   TEXT,
    status         TEXT NOT NULL DEFAULT 'disconnected',
    max_sessions   INTEGER NOT NULL DEFAULT 5,
    last_health    TEXT,
    last_seen_at   DATETIME,
    cert_expires_at DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Sessions
CREATE TABLE IF NOT EXISTS sessions (
    session_id     TEXT PRIMARY KEY,
    machine_id     TEXT NOT NULL REFERENCES machines(machine_id),
    user_id        TEXT REFERENCES users(user_id),
    status         TEXT NOT NULL DEFAULT 'starting',
    command        TEXT NOT NULL DEFAULT 'claude',
    args           TEXT,
    working_dir    TEXT,
    initial_prompt TEXT,
    exit_code      INTEGER,
    started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at       DATETIME,
    scrollback_bytes INTEGER DEFAULT 0,
    run_step_id    TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_machine ON sessions(machine_id, status);

-- Jobs
CREATE TABLE IF NOT EXISTS jobs (
    job_id         TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    description    TEXT,
    user_id        TEXT REFERENCES users(user_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Steps
CREATE TABLE IF NOT EXISTS steps (
    step_id        TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    prompt         TEXT NOT NULL,
    machine_id     TEXT REFERENCES machines(machine_id),
    working_dir    TEXT,
    command        TEXT DEFAULT 'claude',
    args           TEXT,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER DEFAULT 0,
    on_failure     TEXT NOT NULL DEFAULT 'fail_run',
    expected_outputs TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_steps_job ON steps(job_id, sort_order);

-- Step dependencies
CREATE TABLE IF NOT EXISTS step_dependencies (
    step_id        TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    depends_on     TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    PRIMARY KEY (step_id, depends_on),
    CHECK (step_id != depends_on)
);

-- Runs
CREATE TABLE IF NOT EXISTS runs (
    run_id         TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    trigger_type   TEXT NOT NULL,
    trigger_detail TEXT,
    started_at     DATETIME,
    ended_at       DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_runs_job ON runs(job_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

-- Run steps
CREATE TABLE IF NOT EXISTS run_steps (
    run_step_id    TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL REFERENCES steps(step_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    machine_id     TEXT REFERENCES machines(machine_id),
    session_id     TEXT,
    exit_code      INTEGER,
    started_at     DATETIME,
    ended_at       DATETIME,
    error_message  TEXT,
    -- Snapshot fields (immutable copy from step at run creation)
    prompt_snapshot    TEXT,
    machine_id_snapshot TEXT,
    working_dir_snapshot TEXT,
    command_snapshot    TEXT,
    args_snapshot       TEXT
);

CREATE INDEX IF NOT EXISTS idx_run_steps_run ON run_steps(run_id);

-- Credentials
CREATE TABLE IF NOT EXISTS credentials (
    credential_id  TEXT PRIMARY KEY,
    user_id        TEXT REFERENCES users(user_id),
    name           TEXT NOT NULL,
    encrypted_value BLOB NOT NULL,
    nonce          BLOB NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    log_id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id        TEXT,
    action         TEXT NOT NULL,
    resource_type  TEXT,
    resource_id    TEXT,
    detail         TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(timestamp DESC);

-- Revoked tokens
CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti        TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    revoked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_revoked_tokens_expires ON revoked_tokens(expires_at);
`,
	},
	{
		Version:     2,
		Description: "event service layer tables",
		SQL: `
-- Events audit trail
CREATE TABLE IF NOT EXISTS events (
    event_id    TEXT PRIMARY KEY,
    event_type  TEXT NOT NULL,
    timestamp   DATETIME NOT NULL,
    source      TEXT NOT NULL,
    payload     TEXT NOT NULL,  -- JSON
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_events_type_time ON events(event_type, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(timestamp DESC);

-- Outbound webhooks configuration
CREATE TABLE IF NOT EXISTS webhooks (
    webhook_id  TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    secret      BLOB,
    events      TEXT NOT NULL,      -- JSON array of event type patterns
    enabled     BOOLEAN NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Webhook delivery tracking
CREATE TABLE IF NOT EXISTS webhook_deliveries (
    delivery_id    TEXT PRIMARY KEY,
    webhook_id     TEXT NOT NULL REFERENCES webhooks(webhook_id) ON DELETE CASCADE,
    event_id       TEXT NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'pending',
    attempts       INTEGER NOT NULL DEFAULT 0,
    response_code  INTEGER,
    last_error     TEXT,
    next_retry_at  DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status ON webhook_deliveries(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_webhook ON webhook_deliveries(webhook_id);

-- Job triggers configuration
CREATE TABLE IF NOT EXISTS job_triggers (
    trigger_id   TEXT PRIMARY KEY,
    job_id       TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,
    filter       TEXT,
    enabled      BOOLEAN NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_job_triggers_job ON job_triggers(job_id);
CREATE INDEX IF NOT EXISTS idx_job_triggers_event ON job_triggers(event_type, enabled);
`,
	},
	{
		Version:     3,
		Description: "cron schedules",
		SQL: `
CREATE TABLE IF NOT EXISTS cron_schedules (
    schedule_id       TEXT PRIMARY KEY,
    job_id            TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    cron_expr         TEXT NOT NULL,
    timezone          TEXT NOT NULL DEFAULT 'UTC',
    enabled           INTEGER NOT NULL DEFAULT 1,
    next_run_at       DATETIME,
    last_triggered_at DATETIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_cron_schedules_job ON cron_schedules(job_id);
CREATE INDEX IF NOT EXISTS idx_cron_schedules_enabled ON cron_schedules(enabled, next_run_at);
`,
	},
	{
		Version:     4,
		Description: "provisioning tokens",
		SQL: `
CREATE TABLE IF NOT EXISTS provisioning_tokens (
    token           TEXT PRIMARY KEY,
    machine_id      TEXT NOT NULL,
    target_os       TEXT NOT NULL DEFAULT 'linux',
    target_arch     TEXT NOT NULL DEFAULT 'amd64',
    ca_cert_pem     TEXT NOT NULL,
    agent_cert_pem  TEXT NOT NULL,
    agent_key_pem   TEXT NOT NULL,
    server_address  TEXT NOT NULL,
    grpc_address    TEXT NOT NULL,
    created_by      TEXT NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at      DATETIME NOT NULL,
    redeemed_at     DATETIME
);

CREATE INDEX IF NOT EXISTS idx_provisioning_tokens_expires ON provisioning_tokens(expires_at);
`,
	},
	{
		Version:     5,
		Description: "session templates",
		SQL: `
CREATE TABLE IF NOT EXISTS session_templates (
    template_id     TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(user_id),
    name            TEXT NOT NULL,
    description     TEXT,
    command         TEXT,
    args            TEXT,
    working_dir     TEXT,
    env_vars        TEXT,
    initial_prompt  TEXT,
    terminal_rows   INTEGER NOT NULL DEFAULT 24,
    terminal_cols   INTEGER NOT NULL DEFAULT 80,
    tags            TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    deleted_at      DATETIME,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_templates_user ON session_templates(user_id, deleted_at);

ALTER TABLE sessions ADD COLUMN template_id TEXT REFERENCES session_templates(template_id);
`,
	},
	{
		Version:     6,
		Description: "session injections",
		SQL: `
CREATE TABLE IF NOT EXISTS injections (
    injection_id TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(session_id),
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    text_length  INTEGER NOT NULL,
    metadata     TEXT,
    source       TEXT NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_injections_session ON injections(session_id, created_at DESC);
`,
	},
	{
		Version:     7,
		Description: "api keys and bridge connector config",
		SQL: `
CREATE TABLE IF NOT EXISTS api_keys (
    key_id       TEXT PRIMARY KEY,
    key_hmac     TEXT NOT NULL,
    user_id      TEXT NOT NULL REFERENCES users(user_id),
    name         TEXT NOT NULL,
    scopes       TEXT,
    expires_at   DATETIME,
    last_used_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

CREATE TABLE IF NOT EXISTS bridge_connectors (
    connector_id   TEXT PRIMARY KEY,
    connector_type TEXT NOT NULL,
    name           TEXT NOT NULL,
    enabled        INTEGER NOT NULL DEFAULT 1,
    config         TEXT NOT NULL,
    config_secret  BLOB,
    config_nonce   BLOB,
    created_by     TEXT NOT NULL REFERENCES users(user_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bridge_control (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`,
	},
	{
		Version:     8,
		Description: "add machine_id to session templates",
		SQL:         `ALTER TABLE session_templates ADD COLUMN machine_id TEXT NOT NULL DEFAULT '';`,
	},
	{
		Version:     9,
		Description: "add step preferences, run_step snapshots, and user_preferences table",
		SQL: `
ALTER TABLE steps ADD COLUMN skip_permissions INTEGER;
ALTER TABLE steps ADD COLUMN model TEXT DEFAULT '';
ALTER TABLE steps ADD COLUMN delay_seconds INTEGER DEFAULT 0;

ALTER TABLE run_steps ADD COLUMN skip_permissions_snapshot INTEGER;
ALTER TABLE run_steps ADD COLUMN model_snapshot TEXT DEFAULT '';
ALTER TABLE run_steps ADD COLUMN delay_seconds_snapshot INTEGER DEFAULT 0;
ALTER TABLE run_steps ADD COLUMN on_failure_snapshot TEXT NOT NULL DEFAULT 'fail_run';
ALTER TABLE run_steps ADD COLUMN timeout_seconds_snapshot INTEGER DEFAULT 0;

CREATE TABLE IF NOT EXISTS user_preferences (
    user_id    TEXT PRIMARY KEY REFERENCES users(user_id) ON DELETE CASCADE,
    preferences TEXT NOT NULL DEFAULT '{}',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`,
	},
	{
		Version:     10,
		Description: "job system redesign: parameters, task types, shared sessions, retries, task values",
		SQL: `
-- Job-level enhancements
ALTER TABLE jobs ADD COLUMN parameters TEXT;
ALTER TABLE jobs ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN max_concurrent_runs INTEGER NOT NULL DEFAULT 1;

-- Step enhancements
ALTER TABLE steps ADD COLUMN task_type TEXT NOT NULL DEFAULT 'claude_session';
ALTER TABLE steps ADD COLUMN session_key TEXT;
ALTER TABLE steps ADD COLUMN run_if TEXT NOT NULL DEFAULT 'all_success';
ALTER TABLE steps ADD COLUMN max_retries INTEGER NOT NULL DEFAULT 0;
ALTER TABLE steps ADD COLUMN retry_delay_seconds INTEGER NOT NULL DEFAULT 30;
ALTER TABLE steps ADD COLUMN parameters TEXT;

-- Run enhancements
ALTER TABLE runs ADD COLUMN parameters TEXT;

-- Run step snapshots
ALTER TABLE run_steps ADD COLUMN task_type_snapshot TEXT NOT NULL DEFAULT 'claude_session';
ALTER TABLE run_steps ADD COLUMN session_key_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN run_if_snapshot TEXT NOT NULL DEFAULT 'all_success';
ALTER TABLE run_steps ADD COLUMN max_retries_snapshot INTEGER NOT NULL DEFAULT 0;
ALTER TABLE run_steps ADD COLUMN retry_delay_seconds_snapshot INTEGER NOT NULL DEFAULT 30;
ALTER TABLE run_steps ADD COLUMN attempt INTEGER NOT NULL DEFAULT 1;
ALTER TABLE run_steps ADD COLUMN parameters_snapshot TEXT;

-- Task values (inter-step data passing)
CREATE TABLE IF NOT EXISTS run_step_values (
    run_step_id TEXT NOT NULL REFERENCES run_steps(run_step_id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (run_step_id, key)
);
CREATE INDEX IF NOT EXISTS idx_run_step_values_run_step ON run_step_values(run_step_id);
`,
	},
	{
		Version:     11,
		Description: "add run_job task type fields",
		SQL: `
ALTER TABLE steps ADD COLUMN target_job_id TEXT;
ALTER TABLE steps ADD COLUMN job_params TEXT;
ALTER TABLE run_steps ADD COLUMN target_job_id_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN job_params_snapshot TEXT;
`,
	},
	{
		Version:     12,
		Description: "session content search index, server settings, and pending cleanups",
		SQL: `
CREATE TABLE IF NOT EXISTS session_lines (
    rowid       INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT NOT NULL,
    line_number INTEGER NOT NULL,
    content     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_lines_session ON session_lines(session_id, line_number);

CREATE VIRTUAL TABLE IF NOT EXISTS session_content USING fts5(
    content,
    content='session_lines',
    content_rowid='rowid',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS session_lines_ai AFTER INSERT ON session_lines BEGIN
    INSERT INTO session_content(rowid, content) VALUES (new.rowid, new.content);
END;

CREATE TRIGGER IF NOT EXISTS session_lines_ad AFTER DELETE ON session_lines BEGIN
    INSERT INTO session_content(session_content, rowid, content) VALUES('delete', old.rowid, old.content);
END;

CREATE TABLE IF NOT EXISTS session_content_meta (
    session_id  TEXT PRIMARY KEY REFERENCES sessions(session_id) ON DELETE CASCADE,
    line_count  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS server_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pending_cleanups (
    cleanup_id  TEXT PRIMARY KEY,
    session_id  TEXT NOT NULL,
    machine_id  TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pending_cleanups_machine ON pending_cleanups(machine_id);
`,
	},
	{
		Version:     13,
		Description: "short_code column for provisioning tokens",
		SQL: `
ALTER TABLE provisioning_tokens ADD COLUMN short_code TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioning_tokens_short_code ON provisioning_tokens(short_code);
`,
	},
	{
		Version:     14,
		Description: "session metadata columns for model, skip_permissions, env_vars",
		SQL: `ALTER TABLE sessions ADD COLUMN model TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN skip_permissions TEXT DEFAULT '';
ALTER TABLE sessions ADD COLUMN env_vars TEXT DEFAULT '';`,
	},
	{
		Version:     15,
		Description: "machine soft-delete",
		SQL:         `ALTER TABLE machines ADD COLUMN deleted_at DATETIME;`,
	},
	{
		Version:     16,
		Description: "add payload column to webhook_deliveries",
		SQL:         `ALTER TABLE webhook_deliveries ADD COLUMN payload TEXT;`,
	},
	{
		Version:     17,
		Description: "notification channels and subscriptions",
		SQL: `
		CREATE TABLE IF NOT EXISTS notification_channels (
			channel_id   TEXT PRIMARY KEY,
			channel_type TEXT NOT NULL,
			name         TEXT NOT NULL,
			config       TEXT NOT NULL,
			enabled      INTEGER NOT NULL DEFAULT 1,
			created_by   TEXT NOT NULL,
			created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS notification_subscriptions (
			user_id    TEXT NOT NULL,
			channel_id TEXT NOT NULL REFERENCES notification_channels(channel_id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			PRIMARY KEY (user_id, channel_id, event_type)
		);
		`,
	},
	{
		Version:     18,
		Description: "add home_dir to machines",
		SQL:         `ALTER TABLE machines ADD COLUMN home_dir TEXT NOT NULL DEFAULT '';`,
	},
	{
		Version:     19,
		Description: "add updated_at column to sessions",
		SQL: `
ALTER TABLE sessions ADD COLUMN updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP;
UPDATE sessions SET updated_at = ended_at
    WHERE status IN ('completed', 'failed', 'terminated');
UPDATE sessions SET updated_at = CURRENT_TIMESTAMP
    WHERE status NOT IN ('completed', 'failed', 'terminated');
`,
	},
	{
		Version:     20,
		Description: "add connector_id to notification_channels",
		SQL:         `ALTER TABLE notification_channels ADD COLUMN connector_id TEXT NULL;`,
	},
	{
		Version:     21,
		Description: "prevent duplicate active provisioning tokens per machine_id",
		SQL:         `CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioning_tokens_active_machine_id ON provisioning_tokens(machine_id) WHERE redeemed_at IS NULL;`,
	},
}

// ensureVersionTable creates the schema_version table if it does not exist.
// This table is outside the migration system itself since it must exist before
// any migrations can be tracked.
func ensureVersionTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version    INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_version table: %w", err)
	}
	return nil
}

// currentVersion returns the highest migration version that has been applied,
// or 0 if no migrations have been run.
func currentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("query current version: %w", err)
	}
	return version, nil
}

// RunMigrations applies all pending schema migrations to the database.
// Each migration runs in its own transaction. The schema_version table is
// used to track which migrations have already been applied, making this
// safe to call on every startup.
func RunMigrations(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("set busy_timeout pragma: %w", err)
	}

	if err := ensureVersionTable(db); err != nil {
		return err
	}

	current, err := currentVersion(db)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Description, err)
		}
	}

	return nil
}

// applyMigration runs a single migration inside a BEGIN IMMEDIATE transaction
// and records its version in the schema_version table. BEGIN IMMEDIATE acquires
// the SQLite write lock upfront, preventing concurrent processes from applying
// the same migration simultaneously. The version is re-checked inside the
// transaction to handle races where two processes both see the same
// currentVersion before either acquires the lock.
func applyMigration(db *sql.DB, m Migration) error {
	// BEGIN IMMEDIATE acquires the write lock immediately, preventing
	// concurrent migration attempts from interleaving.
	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Close()

	// Set busy_timeout on this connection — the pragma is per-connection in SQLite,
	// so the pool-level setting from RunMigrations doesn't carry over.
	if _, err := conn.ExecContext(context.Background(), "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("set busy_timeout on migration conn: %w", err)
	}

	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}

	// Re-check version inside the transaction to handle concurrent startup.
	var applied bool
	err = conn.QueryRowContext(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM schema_version WHERE version = ?)", m.Version,
	).Scan(&applied)
	if err != nil {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		return fmt.Errorf("check version: %w", err)
	}
	if applied {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		return nil // already applied by another process
	}

	if _, err := conn.ExecContext(context.Background(), m.SQL); err != nil {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := conn.ExecContext(context.Background(),
		"INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
		m.Version, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		return fmt.Errorf("record version: %w", err)
	}

	if _, err := conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
