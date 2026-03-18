// GRPCSinkHandler forwards agent logs to the server via gRPC.
// Wired into the agent's slog in client.go's Run method.
package agent

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

const (
	logFlushSize     = 50
	logFlushInterval = 2 * time.Second
)

// logEntry is an intermediate representation of a log record before it is
// converted to the protobuf LogEntry type for transmission.
type logEntry struct {
	Timestamp time.Time
	Level     string
	Component string
	Message   string
	SessionID string
	Error     string
	Attrs     map[string]string
}

// grpcSinkState holds the shared mutable state for the log buffer and send
// channel. Multiple GRPCSinkHandler instances (created by WithAttrs/WithGroup)
// share the same state so that buffering and flushing happens in one place.
type grpcSinkState struct {
	sendCh    chan<- *pb.AgentEvent
	buf       []logEntry
	mu        sync.Mutex
	sending   atomic.Bool
	done      chan struct{}
	closeOnce sync.Once
}

// GRPCSinkHandler is a slog.Handler that buffers log records and periodically
// flushes them to the server as LogBatch events over the gRPC send channel.
type GRPCSinkHandler struct {
	state *grpcSinkState
	attrs []slog.Attr
	group string
}

// NewGRPCSinkHandler creates a handler that forwards logs via sendCh.
// It starts a background goroutine that flushes the buffer every
// logFlushInterval seconds. Call Close() to stop the goroutine and
// perform a final flush.
func NewGRPCSinkHandler(sendCh chan<- *pb.AgentEvent) *GRPCSinkHandler {
	state := &grpcSinkState{
		sendCh: sendCh,
		done:   make(chan struct{}),
	}
	h := &GRPCSinkHandler{state: state}
	go h.flushLoop()
	return h
}

// Enabled reports whether the handler handles records at the given level.
// All levels are forwarded to the server.
func (h *GRPCSinkHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

// Handle buffers the log record. If the record was generated while a flush
// is in progress (the sending flag is set), it is silently dropped to avoid
// recursive gRPC errors triggering more log sends.
func (h *GRPCSinkHandler) Handle(_ context.Context, r slog.Record) error {
	// Feedback loop guard: drop records generated while sending to avoid
	// recursive gRPC errors triggering more log sends.
	if h.state.sending.Load() {
		return nil
	}

	entry := logEntry{
		Timestamp: r.Time,
		Level:     r.Level.String(),
		Message:   r.Message,
		Attrs:     make(map[string]string),
	}

	// Copy handler-level attrs first.
	for _, a := range h.attrs {
		applyAttr(&entry, a)
	}

	// Then record-level attrs (override handler-level on conflict).
	r.Attrs(func(a slog.Attr) bool {
		applyAttr(&entry, a)
		return true
	})

	h.state.mu.Lock()
	h.state.buf = append(h.state.buf, entry)
	shouldFlush := len(h.state.buf) >= logFlushSize
	h.state.mu.Unlock()

	if shouldFlush {
		h.flush()
	}

	return nil
}

// WithAttrs returns a new handler with the given attrs pre-applied.
// The returned handler shares the same grpcSinkState.
func (h *GRPCSinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)
	return &GRPCSinkHandler{
		state: h.state,
		attrs: combined,
		group: h.group,
	}
}

// WithGroup returns a new handler with the given group name.
// The returned handler shares the same grpcSinkState.
func (h *GRPCSinkHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroup := name
	if h.group != "" {
		newGroup = h.group + "." + name
	}
	return &GRPCSinkHandler{
		state: h.state,
		attrs: h.attrs,
		group: newGroup,
	}
}

// Close stops the flush goroutine and performs a final flush.
func (h *GRPCSinkHandler) Close() {
	h.state.closeOnce.Do(func() {
		close(h.state.done)
		h.flush()
	})
}

// flushLoop runs in a background goroutine and periodically flushes the buffer.
func (h *GRPCSinkHandler) flushLoop() {
	ticker := time.NewTicker(logFlushInterval)
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

// flush drains the buffer and sends a LogBatch event to the gRPC channel.
// It is non-blocking: if the send channel is full, the batch is dropped.
func (h *GRPCSinkHandler) flush() {
	h.state.mu.Lock()
	if len(h.state.buf) == 0 {
		h.state.mu.Unlock()
		return
	}
	entries := h.state.buf
	h.state.buf = nil
	h.state.mu.Unlock()

	pbEntries := make([]*pb.LogEntry, len(entries))
	for i, e := range entries {
		pbEntries[i] = &pb.LogEntry{
			Timestamp: e.Timestamp.UTC().Format(time.RFC3339Nano),
			Level:     e.Level,
			Component: e.Component,
			Message:   e.Message,
			SessionId: e.SessionID,
			Error:     e.Error,
			Attrs:     e.Attrs,
		}
	}

	evt := &pb.AgentEvent{
		Event: &pb.AgentEvent_LogBatch{
			LogBatch: &pb.LogBatch{
				Entries: pbEntries,
			},
		},
	}

	h.state.sending.Store(true)
	defer h.state.sending.Store(false)

	// Non-blocking send: drop if channel is full.
	select {
	case h.state.sendCh <- evt:
	default:
	}
}

// applyAttr extracts well-known attributes (component, session_id, error)
// into dedicated fields and stores the rest in the attrs map.
func applyAttr(entry *logEntry, a slog.Attr) {
	switch a.Key {
	case "component":
		entry.Component = a.Value.String()
	case "session_id":
		entry.SessionID = a.Value.String()
	case "error":
		entry.Error = a.Value.String()
	default:
		entry.Attrs[a.Key] = a.Value.String()
	}
}

// multiHandler fans out slog records to multiple handlers.
type multiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that delegates to all given handlers.
func NewMultiHandler(handlers ...slog.Handler) slog.Handler {
	return &multiHandler{handlers: handlers}
}

// Enabled returns true if any underlying handler is enabled.
func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle delegates the record to all underlying handlers.
func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil {
				return err
			}
		}
	}
	return nil
}

// WithAttrs returns a new multiHandler with attrs applied to all handlers.
func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

// WithGroup returns a new multiHandler with group applied to all handlers.
func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		handlers[i] = h.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}
