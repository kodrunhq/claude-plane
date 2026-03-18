package logging

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *LogStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test-logs.db")
	ls, err := NewLogStore(dbPath)
	if err != nil {
		t.Fatalf("NewLogStore: %v", err)
	}
	t.Cleanup(func() { ls.Close() })
	return ls
}

func TestInsertBatch_Empty(t *testing.T) {
	ls := newTestStore(t)
	if err := ls.InsertBatch(nil); err != nil {
		t.Fatalf("InsertBatch(nil): %v", err)
	}
}

func TestInsertAndQueryAll(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Component: "server", Message: "started", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "ERROR", Component: "grpc", Message: "connection failed", Error: "timeout", Source: "server"},
		{Timestamp: now.Add(2 * time.Second), Level: "WARN", Component: "auth", Message: "invalid token", MachineID: "m1", SessionID: "s1", Source: "agent"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, total, err := ls.Query(LogFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	// Results should be ordered by timestamp DESC
	if results[0].Level != "WARN" {
		t.Errorf("first result level = %q, want %q", results[0].Level, "WARN")
	}
	if results[2].Level != "INFO" {
		t.Errorf("last result level = %q, want %q", results[2].Level, "INFO")
	}
}

func TestQueryByLevel(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Message: "a", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "ERROR", Message: "b", Source: "server"},
		{Timestamp: now.Add(2 * time.Second), Level: "INFO", Message: "c", Source: "server"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, total, err := ls.Query(LogFilter{Level: "INFO"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Level != "INFO" {
			t.Errorf("unexpected level %q", r.Level)
		}
	}
}

func TestQueryBySource(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Message: "server log", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "INFO", Message: "agent log", Source: "agent"},
		{Timestamp: now.Add(2 * time.Second), Level: "INFO", Message: "bridge log", Source: "bridge"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, total, err := ls.Query(LogFilter{Source: "agent"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if results[0].Source != "agent" {
		t.Errorf("source = %q, want %q", results[0].Source, "agent")
	}
}

func TestQueryBySearch(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Message: "user logged in", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "ERROR", Message: "db query failed", Error: "connection reset", Source: "server"},
		{Timestamp: now.Add(2 * time.Second), Level: "WARN", Message: "cache miss", Source: "server"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Search in message
	results, total, err := ls.Query(LogFilter{Search: "logged"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Message != "user logged in" {
		t.Errorf("unexpected results: %+v", results)
	}

	// Search in error field
	results, total, err = ls.Query(LogFilter{Search: "connection reset"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Level != "ERROR" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestQueryByMachineAndSession(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Message: "a", MachineID: "m1", SessionID: "s1", Source: "agent"},
		{Timestamp: now.Add(time.Second), Level: "INFO", Message: "b", MachineID: "m1", SessionID: "s2", Source: "agent"},
		{Timestamp: now.Add(2 * time.Second), Level: "INFO", Message: "c", MachineID: "m2", SessionID: "s3", Source: "agent"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, total, err := ls.Query(LogFilter{MachineID: "m1"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}

	results, total, err = ls.Query(LogFilter{SessionID: "s2"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Message != "b" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestQueryByTimeRange(t *testing.T) {
	ls := newTestStore(t)

	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	records := []LogRecord{
		{Timestamp: base, Level: "INFO", Message: "first", Source: "server"},
		{Timestamp: base.Add(time.Hour), Level: "INFO", Message: "second", Source: "server"},
		{Timestamp: base.Add(2 * time.Hour), Level: "INFO", Message: "third", Source: "server"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, total, err := ls.Query(LogFilter{
		Since: base.Add(30 * time.Minute),
		Until: base.Add(90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Message != "second" {
		t.Errorf("unexpected results: %+v", results)
	}
}

func TestQueryLimitClamping(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := make([]LogRecord, 5)
	for i := range records {
		records[i] = LogRecord{Timestamp: now.Add(time.Duration(i) * time.Second), Level: "INFO", Message: "msg", Source: "server"}
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Limit 0 defaults to 100
	results, total, err := ls.Query(LogFilter{Limit: 0})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 5 || len(results) != 5 {
		t.Errorf("default limit: total=%d, len=%d", total, len(results))
	}

	// Explicit limit 2
	results, total, err = ls.Query(LogFilter{Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestPurgeBefore(t *testing.T) {
	ls := newTestStore(t)

	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	records := []LogRecord{
		{Timestamp: old, Level: "INFO", Message: "old record", Source: "server"},
		{Timestamp: old.Add(time.Hour), Level: "ERROR", Message: "another old", Source: "server"},
		{Timestamp: recent, Level: "INFO", Message: "recent record", Source: "server"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	deleted, err := ls.PurgeBefore(cutoff)
	if err != nil {
		t.Fatalf("PurgeBefore: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	results, total, err := ls.Query(LogFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(results) != 1 || results[0].Message != "recent record" {
		t.Errorf("unexpected remaining records: %+v", results)
	}
}

func TestStats(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Component: "server", Message: "a", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "INFO", Component: "server", Message: "b", Source: "server"},
		{Timestamp: now.Add(2 * time.Second), Level: "ERROR", Component: "grpc", Message: "c", Source: "server"},
		{Timestamp: now.Add(3 * time.Second), Level: "WARN", Component: "auth", Message: "d", Source: "agent"},
		{Timestamp: now.Add(4 * time.Second), Level: "ERROR", Component: "grpc", Message: "e", Source: "server"},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	stats, err := ls.Stats(now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if stats.Total != 5 {
		t.Errorf("total = %d, want 5", stats.Total)
	}
	if stats.ByLevel["INFO"] != 2 {
		t.Errorf("INFO count = %d, want 2", stats.ByLevel["INFO"])
	}
	if stats.ByLevel["ERROR"] != 2 {
		t.Errorf("ERROR count = %d, want 2", stats.ByLevel["ERROR"])
	}
	if stats.ByLevel["WARN"] != 1 {
		t.Errorf("WARN count = %d, want 1", stats.ByLevel["WARN"])
	}
	if stats.ByComponent["server"] != 2 {
		t.Errorf("server component count = %d, want 2", stats.ByComponent["server"])
	}
	if stats.ByComponent["grpc"] != 2 {
		t.Errorf("grpc component count = %d, want 2", stats.ByComponent["grpc"])
	}
	if stats.ByComponent["auth"] != 1 {
		t.Errorf("auth component count = %d, want 1", stats.ByComponent["auth"])
	}
}

func TestNullableFields(t *testing.T) {
	ls := newTestStore(t)

	now := time.Now()
	records := []LogRecord{
		{Timestamp: now, Level: "INFO", Message: "no optionals", Source: "server"},
		{Timestamp: now.Add(time.Second), Level: "INFO", Message: "with metadata", Source: "server", Metadata: `{"key":"val"}`},
	}
	if err := ls.InsertBatch(records); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	results, _, err := ls.Query(LogFilter{})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len = %d, want 2", len(results))
	}

	// Most recent first (with metadata)
	if results[0].Metadata != `{"key":"val"}` {
		t.Errorf("metadata = %q, want %q", results[0].Metadata, `{"key":"val"}`)
	}
	// Older record has empty optional fields
	if results[1].MachineID != "" || results[1].SessionID != "" || results[1].Error != "" || results[1].Metadata != "" {
		t.Errorf("expected empty optional fields, got: machine=%q session=%q error=%q metadata=%q",
			results[1].MachineID, results[1].SessionID, results[1].Error, results[1].Metadata)
	}
}
