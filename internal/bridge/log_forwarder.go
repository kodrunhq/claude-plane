// Package bridge provides the bridge lifecycle and log forwarding.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kodrunhq/claude-plane/internal/bridge/client"
)

const (
	logForwarderFlushSize     = 50
	logForwarderFlushInterval = 2 * time.Second
)

// logForwarderState holds the shared mutable state for the log buffer.
// Multiple LogForwarder instances (created by WithAttrs/WithGroup) share
// the same state so that buffering and flushing happens in one place.
type logForwarderState struct {
	client    *client.Client
	source    string
	buf       []client.LogEntry
	mu        sync.Mutex
	sending   atomic.Bool
	done      chan struct{}
	closeOnce sync.Once

	// Overridable for testing.
	maxBatch      int
	flushInterval time.Duration
}

// LogForwarder is a slog.Handler that buffers log records and periodically
// flushes them to the server via the bridge client's PostLogs method.
type LogForwarder struct {
	state *logForwarderState
	attrs []slog.Attr
	group string
}

// LogForwarderOption configures a LogForwarder.
type LogForwarderOption func(*logForwarderState)

// WithMaxBatch overrides the default flush batch size (50).
func WithMaxBatch(n int) LogForwarderOption {
	return func(s *logForwarderState) {
		if n > 0 {
			s.maxBatch = n
		}
	}
}

// WithFlushInterval overrides the default flush interval (2s).
func WithFlushInterval(d time.Duration) LogForwarderOption {
	return func(s *logForwarderState) {
		if d > 0 {
			s.flushInterval = d
		}
	}
}

// NewLogForwarder creates a handler that forwards logs to the server.
// It starts a background goroutine that flushes the buffer every
// flushInterval. Call Close() to stop the goroutine and drain remaining entries.
func NewLogForwarder(c *client.Client, source string, opts ...LogForwarderOption) *LogForwarder {
	state := &logForwarderState{
		client:        c,
		source:        source,
		done:          make(chan struct{}),
		maxBatch:      logForwarderFlushSize,
		flushInterval: logForwarderFlushInterval,
	}
	for _, opt := range opts {
		opt(state)
	}
	h := &LogForwarder{state: state}
	go h.flushLoop()
	return h
}

// Enabled reports whether the handler handles records at the given level.
// All levels are forwarded to the server.
func (h *LogForwarder) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle buffers the log record. If a flush is in progress (sending flag set),
// the record is silently dropped to prevent recursive HTTP errors from
// generating more log entries.
func (h *LogForwarder) Handle(_ context.Context, r slog.Record) error {
	// Guard against feedback loops: drop records generated while sending.
	if h.state.sending.Load() {
		return nil
	}

	attrs := make(map[string]any)

	// Apply group prefix to attribute keys if set.
	prefix := ""
	if h.group != "" {
		prefix = h.group + "."
	}

	// Copy handler-level attrs first.
	for _, a := range h.attrs {
		attrs[prefix+a.Key] = a.Value.Any()
	}

	// Then record-level attrs (override handler-level on conflict).
	r.Attrs(func(a slog.Attr) bool {
		attrs[prefix+a.Key] = a.Value.Any()
		return true
	})

	entry := client.LogEntry{
		Timestamp:  r.Time.UTC(),
		Level:      r.Level.String(),
		Message:    r.Message,
		Attributes: attrs,
	}

	h.state.mu.Lock()
	h.state.buf = append(h.state.buf, entry)
	shouldFlush := len(h.state.buf) >= h.state.maxBatch
	h.state.mu.Unlock()

	if shouldFlush {
		h.flush()
	}

	return nil
}

// WithAttrs returns a new handler with the given attrs pre-applied.
// The returned handler shares the same logForwarderState.
func (h *LogForwarder) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)
	return &LogForwarder{
		state: h.state,
		attrs: combined,
		group: h.group,
	}
}

// WithGroup returns a new handler with the given group name.
// The returned handler shares the same logForwarderState.
func (h *LogForwarder) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &LogForwarder{
		state: h.state,
		attrs: h.attrs,
		group: newGroup,
	}
}

// Close stops the background flush goroutine and drains any remaining
// buffered entries.
func (h *LogForwarder) Close() {
	h.state.closeOnce.Do(func() {
		close(h.state.done)
		h.flush()
	})
}

// flushLoop runs in a background goroutine and periodically flushes the buffer.
func (h *LogForwarder) flushLoop() {
	ticker := time.NewTicker(h.state.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.flush()
		case <-h.state.done:
			return
		}
	}
}

// flush drains the buffer and sends a log batch to the server.
// Errors are written to stderr (not slog) to prevent infinite recursion.
func (h *LogForwarder) flush() {
	h.state.mu.Lock()
	if len(h.state.buf) == 0 {
		h.state.mu.Unlock()
		return
	}
	entries := h.state.buf
	h.state.buf = nil
	h.state.mu.Unlock()

	h.state.sending.Store(true)
	defer h.state.sending.Store(false)

	req := client.LogIngestionRequest{
		Source:  h.state.source,
		Entries: entries,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.state.client.PostLogs(ctx, req); err != nil {
		// Write to stderr directly — never use slog here to avoid recursion.
		fmt.Fprintf(os.Stderr, "bridge log forwarder: flush failed (%d entries): %v\n", len(entries), err)
	}
}
