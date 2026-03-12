package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Verify WAL mode
	var journalMode string
	if err := s.Reader().QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	// Verify foreign keys enabled
	var fk int
	if err := s.Reader().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestRunMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Open a writer connection to run migrations
	dsn := "file:" + dbPath
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Run migrations twice (must be idempotent)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations (first): %v", err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations (second): %v", err)
	}

	// Verify all tables exist
	tables := []string{
		"users", "machines", "sessions", "jobs", "steps",
		"step_dependencies", "runs", "run_steps", "credentials", "audit_log",
		"revoked_tokens",
	}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestRunMigrations_VersionTracking(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	dsn := "file:" + dbPath
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	// Verify schema_version table exists and has the correct version
	var version int
	if err := db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if version != len(migrations) {
		t.Errorf("schema version = %d, want %d", version, len(migrations))
	}

	// Verify applied_at is recorded
	var appliedAt string
	if err := db.QueryRow("SELECT applied_at FROM schema_version WHERE version = 1").Scan(&appliedAt); err != nil {
		t.Fatalf("query applied_at: %v", err)
	}
	if appliedAt == "" {
		t.Error("applied_at is empty")
	}
}

func TestRunMigrations_SkipsAlreadyApplied(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	dsn := "file:" + dbPath
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Run migrations
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations (first): %v", err)
	}

	// Count rows in schema_version
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count); err != nil {
		t.Fatalf("count schema_version: %v", err)
	}

	// Run again and verify no duplicate rows
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations (second): %v", err)
	}

	var countAfter int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&countAfter); err != nil {
		t.Fatalf("count schema_version after: %v", err)
	}
	if countAfter != count {
		t.Errorf("schema_version row count changed: %d -> %d", count, countAfter)
	}
}

func TestSingleWriter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	// Verify writer has MaxOpenConns=1 by checking Stats
	stats := s.Writer().Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("writer MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}
