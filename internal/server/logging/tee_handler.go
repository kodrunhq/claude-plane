package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// TeeHandler is an slog.Handler that writes to an inner handler (e.g. stderr)
// synchronously and asynchronously sends records to a LogStore and broadcasts
// them to WebSocket subscribers via a LogBroadcaster.
type TeeHandler struct {
	inner   slog.Handler
	store   *LogStore
	dbSink  chan LogRecord
	bcast   *LogBroadcaster
	done    chan struct{}
	wg      sync.WaitGroup
	attrs   []slog.Attr
	groups  []string
	source  string
	closeMu sync.Once
}

// TeeHandlerOption configures a TeeHandler.
type TeeHandlerOption func(*TeeHandler)

// WithSource sets the source field written to every LogRecord.
func WithSource(source string) TeeHandlerOption {
	return func(h *TeeHandler) {
		h.source = source
	}
}

// NewTeeHandler creates a TeeHandler that fans out to inner, store, and broadcaster.
// bufferSize controls the capacity of the async write channel.
// Call Close() to stop the background writer and flush remaining records.
func NewTeeHandler(inner slog.Handler, store *LogStore, bufferSize int, opts ...TeeHandlerOption) *TeeHandler {
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	h := &TeeHandler{
		inner:  inner,
		store:  store,
		dbSink: make(chan LogRecord, bufferSize),
		bcast:  NewLogBroadcaster(),
		done:   make(chan struct{}),
		source: "server",
	}
	for _, o := range opts {
		o(h)
	}
	h.wg.Add(1)
	go h.backgroundWriter()
	return h
}

// Broadcaster returns the LogBroadcaster for wiring up WebSocket subscribers.
func (h *TeeHandler) Broadcaster() *LogBroadcaster {
	return h.bcast
}

// Enabled delegates to the inner handler.
func (h *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle writes to the inner handler synchronously, converts the slog.Record
// to a LogRecord, enqueues it for async DB write, and broadcasts it.
func (h *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	// Write to inner (stderr) synchronously.
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}

	lr := h.buildLogRecord(r)

	// Non-blocking send to DB writer.
	select {
	case h.dbSink <- lr:
	default:
		// Channel full — drop to avoid blocking the caller.
	}

	// Broadcast to WebSocket subscribers.
	h.bcast.Broadcast(lr)

	return nil
}

// WithAttrs returns a new TeeHandler sharing the same sink, store, broadcaster,
// and done channel, but with additional pre-resolved attributes.
func (h *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)

	return &TeeHandler{
		inner:  h.inner.WithAttrs(attrs),
		store:  h.store,
		dbSink: h.dbSink,
		bcast:  h.bcast,
		done:   h.done,
		attrs:  merged,
		groups: h.groups,
		source: h.source,
	}
}

// WithGroup returns a new TeeHandler with the given group appended.
func (h *TeeHandler) WithGroup(name string) slog.Handler {
	groups := make([]string, 0, len(h.groups)+1)
	groups = append(groups, h.groups...)
	groups = append(groups, name)

	return &TeeHandler{
		inner:  h.inner.WithGroup(name),
		store:  h.store,
		dbSink: h.dbSink,
		bcast:  h.bcast,
		done:   h.done,
		attrs:  h.attrs,
		groups: groups,
		source: h.source,
	}
}

// Close stops the background writer goroutine and flushes remaining records.
func (h *TeeHandler) Close() {
	h.closeMu.Do(func() {
		close(h.done)
	})
	h.wg.Wait()
}

// buildLogRecord converts an slog.Record plus pre-resolved attrs into a LogRecord.
// Well-known attrs (component, machine_id, session_id, error/err) are extracted
// into dedicated fields; the remainder goes into Metadata as JSON.
func (h *TeeHandler) buildLogRecord(r slog.Record) LogRecord {
	lr := LogRecord{
		Timestamp: r.Time.UTC(),
		Level:     r.Level.String(),
		Message:   r.Message,
		Source:    h.source,
	}

	// Use the innermost group as the component fallback.
	if len(h.groups) > 0 {
		lr.Component = h.groups[len(h.groups)-1]
	}

	extra := make(map[string]any)

	// Process pre-resolved attrs first.
	for _, a := range h.attrs {
		h.extractAttr(&lr, extra, a)
	}

	// Process record attrs.
	r.Attrs(func(a slog.Attr) bool {
		h.extractAttr(&lr, extra, a)
		return true
	})

	if len(extra) > 0 {
		if data, err := json.Marshal(extra); err == nil {
			lr.Metadata = string(data)
		}
	}

	return lr
}

func (h *TeeHandler) extractAttr(lr *LogRecord, extra map[string]any, a slog.Attr) {
	switch a.Key {
	case "component":
		lr.Component = a.Value.String()
	case "machine_id":
		lr.MachineID = a.Value.String()
	case "session_id":
		lr.SessionID = a.Value.String()
	case "error", "err":
		lr.Error = a.Value.String()
	default:
		extra[a.Key] = a.Value.Any()
	}
}

// backgroundWriter drains dbSink and batch-writes to LogStore every 500ms or 100 records.
func (h *TeeHandler) backgroundWriter() {
	defer h.wg.Done()
	const (
		maxBatch  = 100
		flushTick = 500 * time.Millisecond
	)

	ticker := time.NewTicker(flushTick)
	defer ticker.Stop()

	buf := make([]LogRecord, 0, maxBatch)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		batch := make([]LogRecord, len(buf))
		copy(batch, buf)
		buf = buf[:0]
		// Best-effort write; log errors would recurse.
		_ = h.store.InsertBatch(batch)
	}

	for {
		select {
		case rec := <-h.dbSink:
			buf = append(buf, rec)
			if len(buf) >= maxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-h.done:
			// Drain remaining records.
		drain:
			for {
				select {
				case rec := <-h.dbSink:
					buf = append(buf, rec)
				default:
					break drain
				}
			}
			flush()
			return
		}
	}
}

// ---------------------------------------------------------------------------
// LogBroadcaster — fan-out to WebSocket subscribers
// ---------------------------------------------------------------------------

// LogBroadcaster manages a set of WebSocket subscribers and fans out
// LogRecords to those whose filters match.
type LogBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[*LogSubscriber]struct{}
}

// NewLogBroadcaster creates a ready-to-use broadcaster.
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		subscribers: make(map[*LogSubscriber]struct{}),
	}
}

const maxLogSubscribers = 100

// Subscribe creates a new subscriber with the given filter and returns it.
// The caller must eventually call Unsubscribe to release resources.
func (b *LogBroadcaster) Subscribe(filter LogFilter) (*LogSubscriber, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subscribers) >= maxLogSubscribers {
		return nil, fmt.Errorf("maximum subscriber limit reached")
	}
	sub := &LogSubscriber{
		Ch:     make(chan LogRecord, 256),
		filter: filter,
	}
	b.subscribers[sub] = struct{}{}
	return sub, nil
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *LogBroadcaster) Unsubscribe(sub *LogSubscriber) {
	b.mu.Lock()
	if _, ok := b.subscribers[sub]; ok {
		delete(b.subscribers, sub)
		close(sub.Ch)
	}
	b.mu.Unlock()
}

// Broadcast sends a LogRecord to all matching subscribers. Non-blocking per
// subscriber — records are dropped if a subscriber's channel is full.
func (b *LogBroadcaster) Broadcast(rec LogRecord) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sub := range b.subscribers {
		if sub.matches(rec) {
			select {
			case sub.Ch <- rec:
			default:
			}
		}
	}
}

// LogSubscriber receives LogRecords on Ch that match its filter.
type LogSubscriber struct {
	Ch     chan LogRecord
	mu     sync.RWMutex
	filter LogFilter
}

// UpdateFilter atomically replaces the subscriber's filter.
func (s *LogSubscriber) UpdateFilter(f LogFilter) {
	s.mu.Lock()
	s.filter = f
	s.mu.Unlock()
}

// levelOrder maps level strings to numeric order for threshold comparison.
var levelOrder = map[string]int{
	"DEBUG": 0,
	"INFO":  1,
	"WARN":  2,
	"ERROR": 3,
}

// matches returns true if the record passes the subscriber's filter.
// Level filtering is threshold-based: a filter for WARN matches WARN and ERROR.
// Other fields use exact match when set.
func (s *LogSubscriber) matches(rec LogRecord) bool {
	s.mu.RLock()
	f := s.filter
	s.mu.RUnlock()

	// Threshold-based level filtering.
	if f.Level != "" {
		threshold, ok := levelOrder[strings.ToUpper(f.Level)]
		if !ok {
			threshold = 1 // default to INFO
		}
		recLevel, ok := levelOrder[strings.ToUpper(rec.Level)]
		if !ok {
			recLevel = 1
		}
		if recLevel < threshold {
			return false
		}
	}

	if f.Component != "" && f.Component != rec.Component {
		return false
	}
	if f.Source != "" && f.Source != rec.Source {
		return false
	}
	if f.MachineID != "" && f.MachineID != rec.MachineID {
		return false
	}

	return true
}
