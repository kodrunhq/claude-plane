package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/logging"
)

// newBridgeIngestRouter creates a test server with bridge ingest routes.
func newBridgeIngestRouter(t *testing.T, logStore *logging.LogStore, bcast *logging.LogBroadcaster, bus *event.Bus) *httptest.Server {
	t.Helper()
	logger := slog.Default()
	h := handler.NewBridgeIngestHandler(logStore, bcast, bus, logger)
	r := chi.NewRouter()
	handler.RegisterBridgeIngestRoutes(r, h)
	return httptest.NewServer(r)
}

func newTestLogStore(t *testing.T) *logging.LogStore {
	t.Helper()
	ls, err := logging.NewLogStore(t.TempDir() + "/test-logs.db")
	if err != nil {
		t.Fatalf("create log store: %v", err)
	}
	t.Cleanup(func() { ls.Close() })
	return ls
}

func TestBridgeIngestHandler_HandleLogs(t *testing.T) {
	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, nil)
	defer srv.Close()

	body := map[string]any{
		"source": "bridge",
		"entries": []map[string]any{
			{
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
				"level":      "INFO",
				"message":    "bridge started successfully",
				"attributes": map[string]any{"component": "lifecycle", "version": "1.0.0"},
			},
			{
				"timestamp":  time.Now().UTC().Format(time.RFC3339),
				"level":      "ERROR",
				"message":    "connector failed",
				"attributes": map[string]any{"component": "telegram", "error": "timeout"},
			},
		},
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/ingest/logs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Verify records were persisted.
	records, total, err := ls.Query(logging.LogFilter{Source: "bridge", Limit: 10})
	if err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 records, got %d", total)
	}

	// Check first record (ordered by timestamp DESC, so the ERROR one is first if timestamps are equal).
	var foundLifecycle, foundTelegram bool
	for _, rec := range records {
		if rec.Component == "lifecycle" {
			foundLifecycle = true
			if rec.Level != "INFO" {
				t.Errorf("lifecycle record level = %s, want INFO", rec.Level)
			}
			if rec.Source != "bridge" {
				t.Errorf("lifecycle record source = %s, want bridge", rec.Source)
			}
			// "version" should be in metadata, not a well-known field.
			if !strings.Contains(rec.Metadata, "1.0.0") {
				t.Errorf("expected version in metadata, got %s", rec.Metadata)
			}
		}
		if rec.Component == "telegram" {
			foundTelegram = true
			if rec.Error != "timeout" {
				t.Errorf("telegram record error = %s, want timeout", rec.Error)
			}
		}
	}
	if !foundLifecycle {
		t.Error("lifecycle record not found")
	}
	if !foundTelegram {
		t.Error("telegram record not found")
	}
}

func TestBridgeIngestHandler_HandleLogs_Broadcast(t *testing.T) {
	ls := newTestLogStore(t)
	bcast := logging.NewLogBroadcaster()
	srv := newBridgeIngestRouter(t, ls, bcast, nil)
	defer srv.Close()

	sub, err := bcast.Subscribe(logging.LogFilter{Source: "bridge"})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer bcast.Unsubscribe(sub)

	body := map[string]any{
		"source": "bridge",
		"entries": []map[string]any{
			{
				"timestamp": time.Now().UTC().Format(time.RFC3339),
				"level":     "INFO",
				"message":   "test broadcast",
			},
		},
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/ingest/logs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	select {
	case rec := <-sub.Ch:
		if rec.Message != "test broadcast" {
			t.Errorf("broadcast message = %s, want 'test broadcast'", rec.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for broadcast")
	}
}

func TestBridgeIngestHandler_HandleEvents(t *testing.T) {
	bus := event.NewBus(slog.Default())
	defer bus.Close()

	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, bus)
	defer srv.Close()

	var mu sync.Mutex
	var received []event.Event

	unsub := bus.Subscribe("bridge.*", func(_ context.Context, evt event.Event) error {
		mu.Lock()
		received = append(received, evt)
		mu.Unlock()
		return nil
	}, event.SubscriberOptions{})
	defer unsub()

	body := map[string]any{
		"events": []map[string]any{
			{
				"type":    "bridge.started",
				"payload": map[string]any{"version": "1.0.0"},
			},
		},
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/ingest/events", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Give async subscriber time to process.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "bridge.started" {
		t.Errorf("event type = %s, want bridge.started", received[0].Type)
	}
	if received[0].Payload["version"] != "1.0.0" {
		t.Errorf("event payload version = %v, want 1.0.0", received[0].Payload["version"])
	}
}

func TestBridgeIngestHandler_HandleLogs_EmptyEntries(t *testing.T) {
	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, nil)
	defer srv.Close()

	body := map[string]any{
		"source":  "bridge",
		"entries": []map[string]any{},
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/ingest/logs", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestBridgeIngestHandler_HandleEvents_EmptyEvents(t *testing.T) {
	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, nil)
	defer srv.Close()

	body := map[string]any{
		"events": []map[string]any{},
	}

	b, _ := json.Marshal(body)
	resp, err := http.Post(srv.URL+"/api/v1/ingest/events", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestBridgeIngestHandler_HandleLogs_InvalidJSON(t *testing.T) {
	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/ingest/logs", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestBridgeIngestHandler_HandleEvents_InvalidJSON(t *testing.T) {
	ls := newTestLogStore(t)
	srv := newBridgeIngestRouter(t, ls, nil, nil)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/ingest/events", "application/json", strings.NewReader("{invalid"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
