package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// mockEventStore satisfies handler.EventQueryStore for testing.
type mockEventStore struct {
	events []event.Event
	err    error
}

func (m *mockEventStore) ListEvents(_ context.Context, _ store.EventFilter) ([]event.Event, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

func newEventRouter(t *testing.T, mock handler.EventQueryStore) *httptest.Server {
	t.Helper()
	h := handler.NewEventHandler(mock)
	r := chi.NewRouter()
	r.Get("/api/v1/events", h.ListEvents)
	return httptest.NewServer(r)
}

func makeTestEvent(eventType string) event.Event {
	return event.Event{
		EventID:   uuid.New().String(),
		Type:      eventType,
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Source:    "test",
		Payload:   map[string]any{"key": "val"},
	}
}

func TestEventHandler_ListEvents_Empty(t *testing.T) {
	mock := &mockEventStore{events: nil}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 events, got %d", len(result))
	}
}

func TestEventHandler_ListEvents_Returns(t *testing.T) {
	events := []event.Event{
		makeTestEvent(event.TypeRunCreated),
		makeTestEvent(event.TypeRunStarted),
	}
	mock := &mockEventStore{events: events}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 events, got %d", len(result))
	}
}

func TestEventHandler_ListEvents_InvalidSince(t *testing.T) {
	mock := &mockEventStore{}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?since=not-a-date")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_InvalidLimit(t *testing.T) {
	mock := &mockEventStore{}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?limit=abc")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_NegativeLimit(t *testing.T) {
	mock := &mockEventStore{}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?limit=-1")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_InvalidOffset(t *testing.T) {
	mock := &mockEventStore{}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?offset=-5")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_ValidQueryParams(t *testing.T) {
	events := []event.Event{makeTestEvent(event.TypeRunCreated)}
	mock := &mockEventStore{events: events}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?type=run.*&since=2026-01-01T00:00:00Z&limit=10&offset=0")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_StoreError(t *testing.T) {
	mock := &mockEventStore{err: context.DeadlineExceeded}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestEventHandler_ListEvents_LimitCappedAtMax(t *testing.T) {
	// The handler should cap limit at maxEventsLimit (500) without error.
	mock := &mockEventStore{events: []event.Event{}}
	srv := newEventRouter(t, mock)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/events?limit=9999")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
