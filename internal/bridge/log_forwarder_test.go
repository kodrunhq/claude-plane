package bridge

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

// capturedBatch stores a single log ingestion request received by the test server.
type capturedBatch struct {
	Source  string             `json:"source"`
	Entries []client.LogEntry `json:"entries"`
}

// newTestServer returns an httptest.Server that captures log ingestion POSTs.
func newTestServer(t *testing.T) (*httptest.Server, *[]capturedBatch, *sync.Mutex) {
	t.Helper()
	var batches []capturedBatch
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ingest/logs" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var batch capturedBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		batches = append(batches, batch)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))

	t.Cleanup(srv.Close)
	return srv, &batches, &mu
}

func TestLogForwarder_FlushOnThreshold(t *testing.T) {
	srv, batches, mu := newTestServer(t)
	c := client.New(srv.URL, "test-key")

	fwd := NewLogForwarder(c, "bridge-test",
		WithMaxBatch(5),
		WithFlushInterval(10*time.Second), // long interval so only threshold triggers
	)
	defer fwd.Close()

	logger := slog.New(fwd)

	// Log exactly 5 entries to hit the threshold.
	for i := range 5 {
		logger.Info("test message", "index", i)
	}

	// Give flush a moment to complete the HTTP call.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(*batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(*batches))
	}

	batch := (*batches)[0]
	if batch.Source != "bridge-test" {
		t.Errorf("expected source %q, got %q", "bridge-test", batch.Source)
	}
	if len(batch.Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(batch.Entries))
	}
	if batch.Entries[0].Message != "test message" {
		t.Errorf("expected message %q, got %q", "test message", batch.Entries[0].Message)
	}
	if batch.Entries[0].Level != "INFO" {
		t.Errorf("expected level %q, got %q", "INFO", batch.Entries[0].Level)
	}
}

func TestLogForwarder_FlushOnInterval(t *testing.T) {
	srv, batches, mu := newTestServer(t)
	c := client.New(srv.URL, "test-key")

	fwd := NewLogForwarder(c, "bridge-test",
		WithMaxBatch(100),                    // high threshold
		WithFlushInterval(50*time.Millisecond), // short interval
	)
	defer fwd.Close()

	logger := slog.New(fwd)
	logger.Info("single entry")

	// Wait for the interval flush.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(*batches) == 0 {
		t.Fatal("expected at least 1 batch from interval flush, got 0")
	}

	total := 0
	for _, b := range *batches {
		total += len(b.Entries)
	}
	if total != 1 {
		t.Errorf("expected 1 total entry, got %d", total)
	}
}

func TestLogForwarder_CloseDrainsBuffer(t *testing.T) {
	srv, batches, mu := newTestServer(t)
	c := client.New(srv.URL, "test-key")

	fwd := NewLogForwarder(c, "bridge-test",
		WithMaxBatch(100),                   // high threshold
		WithFlushInterval(10*time.Second),   // long interval
	)

	logger := slog.New(fwd)
	logger.Warn("drain me")

	// Close should drain the buffer.
	fwd.Close()

	// Give the HTTP call a moment.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(*batches) == 0 {
		t.Fatal("expected at least 1 batch after Close, got 0")
	}

	total := 0
	for _, b := range *batches {
		total += len(b.Entries)
	}
	if total != 1 {
		t.Errorf("expected 1 entry after drain, got %d", total)
	}
	if (*batches)[0].Entries[0].Level != "WARN" {
		t.Errorf("expected level WARN, got %q", (*batches)[0].Entries[0].Level)
	}
}

func TestLogForwarder_WithAttrsAndGroup(t *testing.T) {
	srv, batches, mu := newTestServer(t)
	c := client.New(srv.URL, "test-key")

	fwd := NewLogForwarder(c, "bridge-test",
		WithMaxBatch(1),
		WithFlushInterval(10*time.Second),
	)
	defer fwd.Close()

	// Create a logger with pre-set attrs and a group.
	logger := slog.New(fwd.WithAttrs([]slog.Attr{
		slog.String("component", "connector"),
	}).WithGroup("ctx"))

	logger.Info("grouped message", "key", "value")

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(*batches) == 0 {
		t.Fatal("expected at least 1 batch")
	}

	entry := (*batches)[0].Entries[0]
	// Handler-level attr "component" should be under group prefix "ctx."
	if v, ok := entry.Attributes["ctx.component"]; !ok || v != "connector" {
		t.Errorf("expected ctx.component=connector, got %v", entry.Attributes)
	}
	if v, ok := entry.Attributes["ctx.key"]; !ok || v != "value" {
		t.Errorf("expected ctx.key=value, got %v", entry.Attributes)
	}
}

func TestLogForwarder_ServerUnreachable(t *testing.T) {
	// Point to a server that will refuse connections.
	c := client.New("http://127.0.0.1:1", "test-key")

	fwd := NewLogForwarder(c, "bridge-test",
		WithMaxBatch(1),
		WithFlushInterval(10*time.Second),
	)

	logger := slog.New(fwd)
	logger.Info("this will fail to send")

	// Should not panic — errors go to stderr.
	time.Sleep(200 * time.Millisecond)

	// Close should also not panic.
	fwd.Close()
}
