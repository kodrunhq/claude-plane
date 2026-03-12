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
