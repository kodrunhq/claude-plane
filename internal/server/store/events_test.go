package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kodrunhq/claude-plane/internal/server/event"
)

func newTestStoreForEvents(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "events_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeEvent(eventType, source string) event.Event {
	return event.Event{
		EventID:   uuid.New().String(),
		Type:      eventType,
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Source:    source,
		Payload: map[string]any{
			"key": "value",
		},
	}
}

func TestInsertEvent(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	e := makeEvent(event.TypeRunCreated, "orchestrator")
	if err := s.InsertEvent(ctx, e); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
}

func TestInsertEvent_DuplicateID(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	e := makeEvent(event.TypeRunCreated, "orchestrator")
	if err := s.InsertEvent(ctx, e); err != nil {
		t.Fatalf("first InsertEvent: %v", err)
	}
	// Insert same event again — should fail due to PRIMARY KEY constraint.
	if err := s.InsertEvent(ctx, e); err == nil {
		t.Fatal("expected error on duplicate event_id, got nil")
	}
}

func TestListEvents_All(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	events := []event.Event{
		makeEvent(event.TypeRunCreated, "orchestrator"),
		makeEvent(event.TypeRunStarted, "orchestrator"),
		makeEvent(event.TypeSessionStarted, "session"),
	}
	for _, e := range events {
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}

	result, err := s.ListEvents(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("ListEvents count = %d, want 3", len(result))
	}
}

func TestListEvents_TypePattern_Exact(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	if err := s.InsertEvent(ctx, makeEvent(event.TypeRunCreated, "orchestrator")); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if err := s.InsertEvent(ctx, makeEvent(event.TypeRunStarted, "orchestrator")); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if err := s.InsertEvent(ctx, makeEvent(event.TypeSessionStarted, "session")); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	result, err := s.ListEvents(ctx, EventFilter{TypePattern: event.TypeRunCreated})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("ListEvents exact count = %d, want 1", len(result))
	}
	if result[0].Type != event.TypeRunCreated {
		t.Errorf("Type = %q, want %q", result[0].Type, event.TypeRunCreated)
	}
}

func TestListEvents_TypePattern_Glob(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	runEvents := []event.Event{
		makeEvent(event.TypeRunCreated, "orchestrator"),
		makeEvent(event.TypeRunStarted, "orchestrator"),
		makeEvent(event.TypeRunCompleted, "orchestrator"),
	}
	for _, e := range runEvents {
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
	if err := s.InsertEvent(ctx, makeEvent(event.TypeSessionStarted, "session")); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	result, err := s.ListEvents(ctx, EventFilter{TypePattern: "run.*"})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("ListEvents glob count = %d, want 3", len(result))
	}
}

func TestListEvents_Since(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	past := makeEvent(event.TypeRunCreated, "orchestrator")
	past.Timestamp = time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if err := s.InsertEvent(ctx, past); err != nil {
		t.Fatalf("InsertEvent past: %v", err)
	}

	recent := makeEvent(event.TypeRunStarted, "orchestrator")
	recent.Timestamp = time.Now().UTC().Truncate(time.Second)
	if err := s.InsertEvent(ctx, recent); err != nil {
		t.Fatalf("InsertEvent recent: %v", err)
	}

	since := time.Now().UTC().Add(-1 * time.Hour)
	result, err := s.ListEvents(ctx, EventFilter{Since: since})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("ListEvents since count = %d, want 1", len(result))
	}
	if result[0].EventID != recent.EventID {
		t.Errorf("EventID = %q, want %q", result[0].EventID, recent.EventID)
	}
}

func TestListEvents_PayloadRoundTrip(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	e := event.Event{
		EventID:   uuid.New().String(),
		Type:      event.TypeRunCreated,
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Source:    "orchestrator",
		Payload: map[string]any{
			"run_id": "run-123",
			"job_id": "job-456",
			"count":  float64(42),
		},
	}
	if err := s.InsertEvent(ctx, e); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	result, err := s.ListEvents(ctx, EventFilter{TypePattern: event.TypeRunCreated})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result))
	}

	got := result[0]
	if got.Payload["run_id"] != "run-123" {
		t.Errorf("Payload run_id = %v, want run-123", got.Payload["run_id"])
	}
	if got.Payload["job_id"] != "job-456" {
		t.Errorf("Payload job_id = %v, want job-456", got.Payload["job_id"])
	}
	if got.Payload["count"] != float64(42) {
		t.Errorf("Payload count = %v, want 42", got.Payload["count"])
	}
}

func TestListEvents_Pagination(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := s.InsertEvent(ctx, makeEvent(event.TypeRunCreated, "orchestrator")); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}

	page1, err := s.ListEvents(ctx, EventFilter{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatalf("ListEvents page1: %v", err)
	}
	if len(page1) != 3 {
		t.Errorf("page1 count = %d, want 3", len(page1))
	}

	page2, err := s.ListEvents(ctx, EventFilter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatalf("ListEvents page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 count = %d, want 2", len(page2))
	}
}

func TestPurgeEvents(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	old := makeEvent(event.TypeRunCreated, "orchestrator")
	old.Timestamp = time.Now().UTC().Add(-8 * 24 * time.Hour).Truncate(time.Second)
	if err := s.InsertEvent(ctx, old); err != nil {
		t.Fatalf("InsertEvent old: %v", err)
	}

	recent := makeEvent(event.TypeRunStarted, "orchestrator")
	recent.Timestamp = time.Now().UTC().Truncate(time.Second)
	if err := s.InsertEvent(ctx, recent); err != nil {
		t.Fatalf("InsertEvent recent: %v", err)
	}

	before := time.Now().UTC().Add(-7 * 24 * time.Hour)
	n, err := s.PurgeEvents(ctx, before)
	if err != nil {
		t.Fatalf("PurgeEvents: %v", err)
	}
	if n != 1 {
		t.Errorf("PurgeEvents deleted = %d, want 1", n)
	}

	remaining, err := s.ListEvents(ctx, EventFilter{})
	if err != nil {
		t.Fatalf("ListEvents after purge: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("remaining count = %d, want 1", len(remaining))
	}
	if remaining[0].EventID != recent.EventID {
		t.Errorf("remaining EventID = %q, want %q", remaining[0].EventID, recent.EventID)
	}
}

func TestPurgeEvents_NoneToDelete(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	if err := s.InsertEvent(ctx, makeEvent(event.TypeRunCreated, "orchestrator")); err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}

	before := time.Now().UTC().Add(-30 * 24 * time.Hour)
	n, err := s.PurgeEvents(ctx, before)
	if err != nil {
		t.Fatalf("PurgeEvents: %v", err)
	}
	if n != 0 {
		t.Errorf("PurgeEvents deleted = %d, want 0", n)
	}
}
