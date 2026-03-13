package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- stub EventStore for PersistSubscriber tests ---

type stubEventStore struct {
	calls  []Event
	err    error
}

func (s *stubEventStore) InsertEvent(_ context.Context, e Event) error {
	s.calls = append(s.calls, e)
	return s.err
}

func testEvent() Event {
	return Event{
		EventID:   uuid.New().String(),
		Type:      TypeRunCreated,
		Timestamp: time.Now().UTC(),
		Source:    "test",
		Payload:   map[string]any{"k": "v"},
	}
}

func TestPersistSubscriber_Handler_CallsInsertEvent(t *testing.T) {
	store := &stubEventStore{}
	ps := NewPersistSubscriber(store, nullLogger())
	handler := ps.Handler()

	ev := testEvent()
	if err := handler(context.Background(), ev); err != nil {
		t.Fatalf("Handler returned unexpected error: %v", err)
	}

	if len(store.calls) != 1 {
		t.Fatalf("InsertEvent called %d times, want 1", len(store.calls))
	}
	if store.calls[0].EventID != ev.EventID {
		t.Errorf("InsertEvent called with EventID %q, want %q", store.calls[0].EventID, ev.EventID)
	}
}

func TestPersistSubscriber_Handler_PropagatesError(t *testing.T) {
	wantErr := errors.New("db unavailable")
	store := &stubEventStore{err: wantErr}
	ps := NewPersistSubscriber(store, nullLogger())
	handler := ps.Handler()

	ev := testEvent()
	if err := handler(context.Background(), ev); !errors.Is(err, wantErr) {
		t.Errorf("Handler error = %v, want %v", err, wantErr)
	}
}

func TestPersistSubscriber_Handler_MultipleEvents(t *testing.T) {
	store := &stubEventStore{}
	ps := NewPersistSubscriber(store, nullLogger())
	handler := ps.Handler()

	const n = 5
	for i := 0; i < n; i++ {
		if err := handler(context.Background(), testEvent()); err != nil {
			t.Fatalf("Handler[%d] returned unexpected error: %v", i, err)
		}
	}

	if len(store.calls) != n {
		t.Errorf("InsertEvent called %d times, want %d", len(store.calls), n)
	}
}

func TestPersistSubscriber_NilLogger(t *testing.T) {
	// Passing nil logger must not panic.
	store := &stubEventStore{}
	ps := NewPersistSubscriber(store, nil)
	handler := ps.Handler()

	if err := handler(context.Background(), testEvent()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
