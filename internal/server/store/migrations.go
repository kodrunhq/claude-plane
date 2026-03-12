package store

import (
	"database/sql"
	"fmt"
)

// schema contains all CREATE TABLE and CREATE INDEX statements for the
// claude-plane database. All statements use IF NOT EXISTS for idempotency.
const schema = `
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
    sort_order     INTEGER NOT NULL,
    timeout_seconds INTEGER,
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
    error_message  TEXT
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
`

// RunMigrations executes all schema creation statements on the provided
// database connection. Uses BEGIN IMMEDIATE to acquire the write lock upfront.
// All statements use IF NOT EXISTS, making this safe to call multiple times.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec("BEGIN IMMEDIATE;" + schema + "COMMIT;")
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
