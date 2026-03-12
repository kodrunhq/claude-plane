// Package session provides server-side session management including in-memory
// routing of agent terminal events to WebSocket subscribers.
package session

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

// SubscriberMessage wraps data sent to WebSocket subscribers.
// IsControl distinguishes binary terminal data from JSON control messages.
type SubscriberMessage struct {
	Data      []byte
	IsControl bool
}

// Registry routes terminal output from agents to browser WebSocket subscribers.
// Multiple subscribers per session are supported (e.g., multiple browser tabs).
//
// Design trade-off: Flow control uses buffered channels with a non-blocking drop
// policy for slow consumers. This prevents a single slow subscriber (e.g. a
// browser tab on a congested network) from blocking output delivery to all other
// subscribers and, critically, from back-pressuring the agent PTY read loop.
// The cost is that a slow consumer may miss terminal data, leading to corrupted
// terminal state in that tab. Drops are logged at Warn level with byte counts
// and cumulative drop counters so operators can detect the problem and
// investigate the slow consumer.
type Registry struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan SubscriberMessage]struct{}
	dropped     map[chan SubscriberMessage]*atomic.Int64
	logger      *slog.Logger
}

// NewRegistry creates a new session Registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		subscribers: make(map[string]map[chan SubscriberMessage]struct{}),
		dropped:     make(map[chan SubscriberMessage]*atomic.Int64),
		logger:      logger,
	}
}

// Subscribe creates a buffered channel (cap 256) for receiving session output
// and registers it for the given session ID.
func (r *Registry) Subscribe(sessionID string) chan SubscriberMessage {
	ch := make(chan SubscriberMessage, 256)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subscribers[sessionID] == nil {
		r.subscribers[sessionID] = make(map[chan SubscriberMessage]struct{})
	}
	r.subscribers[sessionID][ch] = struct{}{}
	r.dropped[ch] = &atomic.Int64{}
	return ch
}

// Unsubscribe removes a subscriber channel and closes it.
func (r *Registry) Unsubscribe(sessionID string, ch chan SubscriberMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if subs, ok := r.subscribers[sessionID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(r.subscribers, sessionID)
		}
	}
	delete(r.dropped, ch)
	close(ch)
}

// Publish fans out binary terminal data to all subscribers for a session.
// Non-blocking: if a subscriber channel is full, the message is dropped.
func (r *Registry) Publish(sessionID string, data []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	subs := r.subscribers[sessionID]
	for ch := range subs {
		select {
		case ch <- SubscriberMessage{Data: data, IsControl: false}:
		default:
			counter := r.dropped[ch]
			total := counter.Add(1)
			r.logger.Warn("dropped terminal data for slow subscriber",
				"session_id", sessionID,
				"bytes", len(data),
				"total_dropped", total,
			)
		}
	}
}

// PublishControl fans out a JSON control message to all subscribers for a session.
func (r *Registry) PublishControl(sessionID string, msg []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	subs := r.subscribers[sessionID]
	for ch := range subs {
		select {
		case ch <- SubscriberMessage{Data: msg, IsControl: true}:
		default:
			counter := r.dropped[ch]
			total := counter.Add(1)
			r.logger.Warn("dropped control message for slow subscriber",
				"session_id", sessionID,
				"bytes", len(msg),
				"total_dropped", total,
			)
		}
	}
}

// SubscriberCount returns the number of subscribers for a session.
func (r *Registry) SubscriberCount(sessionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscribers[sessionID])
}

// DroppedCount returns the cumulative number of messages dropped for a
// subscriber channel. Returns 0 if the channel is not registered.
func (r *Registry) DroppedCount(ch chan SubscriberMessage) int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if counter, ok := r.dropped[ch]; ok {
		return counter.Load()
	}
	return 0
}
