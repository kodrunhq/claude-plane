package event

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

const (
	defaultBufferSize  = 256
	defaultConcurrency = 1
)

// Publisher is the narrow write-only interface other packages depend on.
// Bus implements this interface.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// Subscriber is a narrow read-only interface for subscribing to events.
// Complements the write-only Publisher interface.
type Subscriber interface {
	Subscribe(pattern string, handler HandlerFunc, opts SubscriberOptions) (unsubscribe func())
}

// Compile-time checks that Bus satisfies both narrow interfaces.
var (
	_ Publisher  = (*Bus)(nil)
	_ Subscriber = (*Bus)(nil)
)

// HandlerFunc is the callback invoked for each matched event.
// Returning a non-nil error causes a Warn-level log entry; the bus does not retry.
//
// Handlers always receive context.Background() because delivery is asynchronous.
// The context passed to Publish may be cancelled before the handler runs.
type HandlerFunc func(ctx context.Context, event Event) error

// SubscriberOptions configures delivery behaviour for a single subscription.
type SubscriberOptions struct {
	// Concurrency is the number of goroutines draining the subscriber's channel.
	// Defaults to 1 (ordered delivery).
	Concurrency int
	// BufferSize is the capacity of the subscriber's event channel.
	// Defaults to 256. Overflow is dropped (non-blocking fan-out).
	BufferSize int
}

// subscriber holds runtime state for one active subscription.
type subscriber struct {
	pattern string
	handler HandlerFunc
	ch      chan Event
	wg      sync.WaitGroup
	once    sync.Once
}

// Bus is an in-process pub/sub broker with glob-style pattern matching.
//
// Publish fans out synchronously to subscriber channels (non-blocking).
// Each subscriber has an independent goroutine pool draining its channel,
// so slow handlers never stall the publisher.
type Bus struct {
	mu             sync.RWMutex
	subscribers    []*subscriber
	logger         *slog.Logger
	closed         bool
	persistHandler func(context.Context, Event) error
}

// NewBus constructs a Bus. If logger is nil, slog.Default() is used.
func NewBus(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{logger: logger}
}

// SetPersistHandler registers a synchronous handler that is called for every
// published event before fan-out to async subscribers. This ensures persistence
// cannot be dropped due to buffer pressure.
func (b *Bus) SetPersistHandler(fn func(context.Context, Event) error) {
	b.persistHandler = fn
}

// Publish delivers event to every subscriber whose pattern matches event.Type.
// If a persist handler is set, it is called synchronously outside the lock
// before fan-out, so that persistence is guaranteed even under buffer pressure.
// Fan-out is non-blocking: if a subscriber's buffer is full the event is dropped
// and a Warn-level message is emitted. If the bus is already closed, Publish
// silently discards the event and returns nil.
func (b *Bus) Publish(ctx context.Context, event Event) error {
	// Snapshot closed state and persist handler under the lock so we don't
	// persist after Close() has been called.
	b.mu.RLock()
	closed := b.closed
	persistFn := b.persistHandler
	b.mu.RUnlock()

	if closed {
		return nil
	}

	// Synchronous persist — called outside the lock to avoid blocking
	// concurrent publishers on SQLite writes.
	if persistFn != nil {
		if err := persistFn(ctx, event); err != nil {
			b.logger.Error("persist handler failed",
				"event_id", event.EventID,
				"event_type", event.Type,
				"error", err,
			)
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil
	}

	for _, sub := range b.subscribers {
		if !MatchPattern(sub.pattern, event.Type) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			b.logger.Warn("event dropped: subscriber buffer full",
				"event_id", event.EventID,
				"event_type", event.Type,
				"pattern", sub.pattern,
			)
		}
	}
	return nil
}

// Subscribe registers handler for events matching pattern.
// Pattern rules:
//   - "*"        matches every event type
//   - "run.*"    matches any type starting with "run."
//   - exact      matches only that exact type string
//
// The returned function unsubscribes and waits for in-flight handlers to finish.
func (b *Bus) Subscribe(pattern string, handler HandlerFunc, opts SubscriberOptions) (unsubscribe func()) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}

	sub := &subscriber{
		pattern: pattern,
		handler: handler,
		ch:      make(chan Event, opts.BufferSize),
	}

	// Start worker goroutines before registering so they are ready to drain.
	for i := 0; i < opts.Concurrency; i++ {
		sub.wg.Add(1)
		go func() {
			defer sub.wg.Done()
			b.drain(sub)
		}()
	}

	b.mu.Lock()
	if b.closed {
		// Bus was closed between sub creation and registration; clean up and
		// return a no-op unsubscribe so the caller never sees a leaked goroutine.
		b.mu.Unlock()
		sub.once.Do(func() { close(sub.ch) })
		sub.wg.Wait()
		return func() {}
	}
	b.subscribers = append(b.subscribers, sub)
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		// Remove sub from the slice without mutating the original backing array.
		updated := make([]*subscriber, 0, len(b.subscribers))
		for _, s := range b.subscribers {
			if s != sub {
				updated = append(updated, s)
			}
		}
		b.subscribers = updated
		b.mu.Unlock()

		// Close exactly once; drain goroutines will exit on channel close.
		sub.once.Do(func() { close(sub.ch) })
		sub.wg.Wait()
	}
}

// Close stops accepting new events, drains all pending events from every
// subscriber's buffer, and waits for in-flight handlers to complete.
func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	subs := b.subscribers
	b.subscribers = nil
	b.mu.Unlock()

	for _, sub := range subs {
		sub.once.Do(func() { close(sub.ch) })
	}
	for _, sub := range subs {
		sub.wg.Wait()
	}
}

// drain reads events from sub.ch until it is closed, calling the handler for each.
func (b *Bus) drain(sub *subscriber) {
	for event := range sub.ch {
		if err := sub.handler(context.Background(), event); err != nil {
			b.logger.Warn("event handler error",
				"event_id", event.EventID,
				"event_type", event.Type,
				"pattern", sub.pattern,
				"error", err,
			)
		}
	}
}

// MatchPattern reports whether eventType matches pattern.
// Supported forms: "*", "prefix.*", or exact equality.
func MatchPattern(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*") // keep trailing dot: "run."
		return strings.HasPrefix(eventType, prefix)
	}
	return pattern == eventType
}
