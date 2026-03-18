package logging

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestLogStore creates a LogStore backed by a temp SQLite file.
func newTestLogStore(t *testing.T) *LogStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test_logs.db")
	store, err := NewLogStore(dbPath)
	if err != nil {
		t.Fatalf("NewLogStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestTeeHandler_WritesToBothInnerAndStore(t *testing.T) {
	store := newTestLogStore(t)

	var buf bytes.Buffer
	inner := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})

	h := NewTeeHandler(inner, store, 100)
	defer h.Close()

	logger := slog.New(h)
	logger.Info("hello world", "component", "test-comp")

	// Wait for async flush (500ms tick + margin).
	time.Sleep(700 * time.Millisecond)
	h.Close()

	// Inner handler should have written to buf.
	if buf.Len() == 0 {
		t.Fatal("inner handler received no output")
	}

	// Store should contain the record.
	records, total, err := store.Query(LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 record in store, got %d", total)
	}
	if records[0].Message != "hello world" {
		t.Errorf("message = %q, want %q", records[0].Message, "hello world")
	}
	if records[0].Component != "test-comp" {
		t.Errorf("component = %q, want %q", records[0].Component, "test-comp")
	}
}

func TestLogBroadcaster_SubscribeReceivesMatchingRecords(t *testing.T) {
	b := NewLogBroadcaster()
	sub, err := b.Subscribe(LogFilter{Level: "INFO"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer b.Unsubscribe(sub)

	rec := LogRecord{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "test message",
		Source:    "server",
	}
	b.Broadcast(rec)

	select {
	case got := <-sub.Ch:
		if got.Message != "test message" {
			t.Errorf("message = %q, want %q", got.Message, "test message")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestLogBroadcaster_FiltersOutBelowThreshold(t *testing.T) {
	b := NewLogBroadcaster()
	sub, err := b.Subscribe(LogFilter{Level: "WARN"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer b.Unsubscribe(sub)

	// INFO is below WARN threshold — should be filtered.
	b.Broadcast(LogRecord{Level: "INFO", Message: "should not arrive"})

	// ERROR is above WARN threshold — should arrive.
	b.Broadcast(LogRecord{Level: "ERROR", Message: "should arrive"})

	select {
	case got := <-sub.Ch:
		if got.Message != "should arrive" {
			t.Errorf("message = %q, want %q", got.Message, "should arrive")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ERROR record")
	}

	// Ensure no extra record (the INFO one).
	select {
	case extra := <-sub.Ch:
		t.Fatalf("unexpected extra record: %+v", extra)
	case <-time.After(100 * time.Millisecond):
		// good
	}
}

func TestLogBroadcaster_UnsubscribeStopsDelivery(t *testing.T) {
	b := NewLogBroadcaster()
	sub, err := b.Subscribe(LogFilter{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	b.Unsubscribe(sub)

	// After unsubscribe, channel should be closed.
	_, open := <-sub.Ch
	if open {
		t.Fatal("expected channel to be closed after Unsubscribe")
	}

	// Broadcasting after unsubscribe should not panic.
	b.Broadcast(LogRecord{Level: "INFO", Message: "no-op"})
}

func TestTeeHandler_ExtractsWellKnownAttrs(t *testing.T) {
	store := newTestLogStore(t)

	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewTeeHandler(inner, store, 100)
	defer h.Close()

	logger := slog.New(h)
	logger.Error("operation failed",
		"component", "orchestrator",
		"machine_id", "worker-1",
		"session_id", "sess-abc",
		"error", "connection refused",
		"extra_key", "extra_val",
	)

	time.Sleep(700 * time.Millisecond)
	h.Close()

	records, total, err := store.Query(LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 record, got %d", total)
	}
	r := records[0]
	if r.Component != "orchestrator" {
		t.Errorf("Component = %q, want %q", r.Component, "orchestrator")
	}
	if r.MachineID != "worker-1" {
		t.Errorf("MachineID = %q, want %q", r.MachineID, "worker-1")
	}
	if r.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "sess-abc")
	}
	if r.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", r.Error, "connection refused")
	}
	if r.Metadata == "" {
		t.Fatal("expected non-empty Metadata for extra_key")
	}
}

func TestTeeHandler_WithGroupUsedAsComponentFallback(t *testing.T) {
	store := newTestLogStore(t)

	inner := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewTeeHandler(inner, store, 100)
	defer h.Close()

	grouped := slog.New(h.WithGroup("scheduler").(slog.Handler))
	grouped.Info("tick")

	time.Sleep(700 * time.Millisecond)
	h.Close()

	records, _, err := store.Query(LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least 1 record")
	}
	if records[0].Component != "scheduler" {
		t.Errorf("Component = %q, want %q (group fallback)", records[0].Component, "scheduler")
	}
}

func TestLogBroadcaster_FilterByComponent(t *testing.T) {
	b := NewLogBroadcaster()
	sub, err := b.Subscribe(LogFilter{Component: "auth"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer b.Unsubscribe(sub)

	b.Broadcast(LogRecord{Level: "INFO", Component: "store", Message: "wrong component"})
	b.Broadcast(LogRecord{Level: "INFO", Component: "auth", Message: "right component"})

	select {
	case got := <-sub.Ch:
		if got.Message != "right component" {
			t.Errorf("message = %q, want %q", got.Message, "right component")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	select {
	case extra := <-sub.Ch:
		t.Fatalf("unexpected record: %+v", extra)
	case <-time.After(100 * time.Millisecond):
		// good
	}
}
