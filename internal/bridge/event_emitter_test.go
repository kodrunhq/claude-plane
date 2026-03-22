package bridge

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

func TestEventEmitter_Emit(t *testing.T) {
	t.Run("sends event to server", func(t *testing.T) {
		var mu sync.Mutex
		var capturedBody []byte
		var capturedMethod string
		var capturedPath string

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			body, _ := io.ReadAll(r.Body)
			capturedBody = body
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := client.New(srv.URL, "test-key")
		emitter := NewEventEmitter(c, nil)

		emitter.Emit("bridge.started", map[string]any{
			"version": "1.0.0",
		})

		mu.Lock()
		defer mu.Unlock()

		if capturedMethod != http.MethodPost {
			t.Fatalf("expected POST, got %s", capturedMethod)
		}
		if capturedPath != "/api/v1/ingest/events" {
			t.Fatalf("expected /api/v1/ingest/events, got %s", capturedPath)
		}

		var req client.EventIngestionRequest
		if err := json.Unmarshal(capturedBody, &req); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if len(req.Events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(req.Events))
		}
		if req.Events[0].Type != "bridge.started" {
			t.Fatalf("expected type bridge.started, got %s", req.Events[0].Type)
		}
		version, ok := req.Events[0].Payload["version"]
		if !ok {
			t.Fatal("expected version in payload")
		}
		if version != "1.0.0" {
			t.Fatalf("expected version 1.0.0, got %v", version)
		}
	})

	t.Run("sends event with nil payload", func(t *testing.T) {
		var mu sync.Mutex
		var capturedBody []byte

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			body, _ := io.ReadAll(r.Body)
			capturedBody = body
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := client.New(srv.URL, "test-key")
		emitter := NewEventEmitter(c, nil)

		emitter.Emit("bridge.stopped", nil)

		mu.Lock()
		defer mu.Unlock()

		var req client.EventIngestionRequest
		if err := json.Unmarshal(capturedBody, &req); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if len(req.Events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(req.Events))
		}
		if req.Events[0].Type != "bridge.stopped" {
			t.Fatalf("expected type bridge.stopped, got %s", req.Events[0].Type)
		}
	})

	t.Run("logs error on server failure without panicking", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"internal error"}`))
		}))
		defer srv.Close()

		c := client.New(srv.URL, "test-key")
		emitter := NewEventEmitter(c, nil)

		// Should not panic — errors are logged but swallowed.
		emitter.Emit("bridge.connector.error", map[string]any{
			"connector_id": "telegram-1",
			"error":        "connection refused",
		})
	})
}
