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

// HandlerFunc is the callback invoked for each matched event.
// Returning a non-nil error causes a Warn-level log entry; the bus does not retry.
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
	mu          sync.RWMutex
	subscribers []*subscriber
	logger      *slog.Logger
	closed      bool
}

// NewBus constructs a Bus. If logger is nil, slog.Default() is used.
func NewBus(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{logger: logger}
}

// Publish delivers event to every subscriber whose pattern matches event.Type.
// Fan-out is non-blocking: if a subscriber's buffer is full the event is dropped
// and a Warn-level message is emitted. Publish returns an error only if the bus
// is already closed.
func (b *Bus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil
	}

	for _, sub := range b.subscribers {
		if !matchPattern(sub.pattern, event.Type) {
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

// matchPattern reports whether eventType matches pattern.
// Supported forms: "*", "prefix.*", or exact equality.
func matchPattern(pattern, eventType string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*") // keep trailing dot: "run."
		return strings.HasPrefix(eventType, prefix)
	}
	return pattern == eventType
}
