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

	base := time.Now().UTC().Truncate(time.Second)
	eventTypes := []struct {
		typ    string
		source string
	}{
		{event.TypeRunCreated, "orchestrator"},
		{event.TypeRunStarted, "orchestrator"},
		{event.TypeSessionStarted, "session"},
	}
	for i, et := range eventTypes {
		e := makeEvent(et.typ, et.source)
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
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

	base := time.Now().UTC().Truncate(time.Second)
	inserts := []struct {
		typ    string
		source string
	}{
		{event.TypeRunCreated, "orchestrator"},
		{event.TypeRunStarted, "orchestrator"},
		{event.TypeSessionStarted, "session"},
	}
	for i, ins := range inserts {
		e := makeEvent(ins.typ, ins.source)
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
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

	base := time.Now().UTC().Truncate(time.Second)
	runTypes := []string{event.TypeRunCreated, event.TypeRunStarted, event.TypeRunCompleted}
	for i, et := range runTypes {
		e := makeEvent(et, "orchestrator")
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent: %v", err)
		}
	}
	sessionEvt := makeEvent(event.TypeSessionStarted, "session")
	sessionEvt.Timestamp = base.Add(time.Duration(len(runTypes)) * time.Second)
	if err := s.InsertEvent(ctx, sessionEvt); err != nil {
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

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		e := makeEvent(event.TypeRunCreated, "orchestrator")
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
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

// TestListEventsAfter_Cursor inserts 5 events and verifies that querying with
// the cursor pointing to event 2 returns events 3-5.
func TestListEventsAfter_Cursor(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	events := make([]event.Event, 5)
	for i := range events {
		e := makeEvent(event.TypeRunCreated, "orchestrator")
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent %d: %v", i, err)
		}
		events[i] = e
	}

	// Cursor after event index 1 (the second event).
	afterTS := events[1].Timestamp
	afterID := events[1].EventID

	result, err := s.ListEventsAfter(ctx, afterTS, afterID, 100)
	if err != nil {
		t.Fatalf("ListEventsAfter: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("count = %d, want 3", len(result))
	}
	for i, got := range result {
		want := events[i+2]
		if got.EventID != want.EventID {
			t.Errorf("result[%d] EventID = %q, want %q", i, got.EventID, want.EventID)
		}
	}
}

// TestListEventsAfter_ZeroCursor verifies that a zero cursor returns the most
// recent events in ascending order.
func TestListEventsAfter_ZeroCursor(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		e := makeEvent(event.TypeRunCreated, "orchestrator")
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent %d: %v", i, err)
		}
	}

	result, err := s.ListEventsAfter(ctx, time.Time{}, "", 100)
	if err != nil {
		t.Fatalf("ListEventsAfter: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("count = %d, want 5", len(result))
	}
	// Verify ascending order.
	for i := 1; i < len(result); i++ {
		if result[i].Timestamp.Before(result[i-1].Timestamp) {
			t.Errorf("results not in ascending order at index %d", i)
		}
	}
}

// TestListEventsAfter_Limit verifies that the limit parameter is respected.
func TestListEventsAfter_Limit(t *testing.T) {
	s := newTestStoreForEvents(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		e := makeEvent(event.TypeRunCreated, "orchestrator")
		e.Timestamp = base.Add(time.Duration(i) * time.Second)
		if err := s.InsertEvent(ctx, e); err != nil {
			t.Fatalf("InsertEvent %d: %v", i, err)
		}
	}

	result, err := s.ListEventsAfter(ctx, time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("ListEventsAfter: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("count = %d, want 2", len(result))
	}
}
