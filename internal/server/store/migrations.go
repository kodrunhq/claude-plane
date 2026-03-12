package store

import (
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

// applyMigration runs a single migration inside a transaction and records
// its version in the schema_version table.
func applyMigration(db *sql.DB, m Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit is a no-op

	if _, err := tx.Exec(m.SQL); err != nil {
		return fmt.Errorf("exec: %w", err)
	}

	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO schema_version (version, applied_at) VALUES (?, ?)",
		m.Version, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
