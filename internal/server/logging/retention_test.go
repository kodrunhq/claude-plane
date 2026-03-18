package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionCleaner_PurgesOldRecords(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "retention_test.db")
	store, err := NewLogStore(dbPath)
	if err != nil {
		t.Fatalf("NewLogStore: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()

	// Insert old and new records.
	records := []LogRecord{
		{Timestamp: now.Add(-48 * time.Hour), Level: "INFO", Message: "old record", Source: "server"},
		{Timestamp: now.Add(-10 * time.Minute), Level: "INFO", Message: "recent record", Source: "server"},
	}
	if err := store.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Run a purge with 24h max age — should delete the 48h-old record.
	cutoff := now.Add(-24 * time.Hour)
	deleted, err := store.PurgeBefore(cutoff)
	if err != nil {
		t.Fatalf("PurgeBefore: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify only the recent record remains.
	results, total, err := store.Query(LogFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 remaining record, got %d", total)
	}
	if len(results) != 1 || results[0].Message != "recent record" {
		t.Errorf("unexpected remaining record: %+v", results)
	}
}

func TestRetentionCleaner_StartAndCancel(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "retention_cancel_test.db")
	store, err := NewLogStore(dbPath)
	if err != nil {
		t.Fatalf("NewLogStore: %v", err)
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	rc := NewRetentionCleaner(store, 24*time.Hour, logger)

	ctx, cancel := context.WithCancel(context.Background())
	rc.Start(ctx)

	// Cancel immediately — the goroutine should exit without panicking.
	cancel()

	// Give it a moment to exit cleanly.
	time.Sleep(50 * time.Millisecond)
}
