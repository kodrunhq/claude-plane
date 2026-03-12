// Package session provides server-side session management including in-memory
// routing of agent terminal events to WebSocket subscribers.
package session

import (
	"log/slog"
	"sync"
)

// SubscriberMessage wraps data sent to WebSocket subscribers.
// IsControl distinguishes binary terminal data from JSON control messages.
type SubscriberMessage struct {
	Data      []byte
	IsControl bool
}

// Registry routes terminal output from agents to browser WebSocket subscribers.
// Multiple subscribers per session are supported (e.g., multiple browser tabs).
// Flow control uses buffered channels with a drop policy for slow consumers.
type Registry struct {
	mu          sync.RWMutex
	subscribers map[string]map[chan SubscriberMessage]struct{}
	logger      *slog.Logger
}

// NewRegistry creates a new session Registry.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		subscribers: make(map[string]map[chan SubscriberMessage]struct{}),
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
			r.logger.Debug("dropped message for slow subscriber", "session_id", sessionID)
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
			r.logger.Debug("dropped control message for slow subscriber", "session_id", sessionID)
		}
	}
}

// SubscriberCount returns the number of subscribers for a session.
func (r *Registry) SubscriberCount(sessionID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subscribers[sessionID])
}
