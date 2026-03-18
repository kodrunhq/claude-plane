// Package broker provides request-response correlation over gRPC bidirectional streams.
// It allows the server to send a command to an agent and wait for a matching response
// identified by a request ID.
package broker

import (
	"sync"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// RequestBroker correlates request-response patterns over gRPC bidirectional streams.
// Callers register a request ID before sending a command, then select on the returned
// channel with a timeout. The agent sends back an event with the same request ID,
// which the gRPC receive loop delivers via Resolve.
type RequestBroker struct {
	mu      sync.Mutex
	pending map[string]chan *pb.DirectoryListingEvent
}

// New creates a new RequestBroker.
func New() *RequestBroker {
	return &RequestBroker{
		pending: make(map[string]chan *pb.DirectoryListingEvent),
	}
}

// Register creates a buffered (size 1) channel for the given request ID and returns it.
// The caller should select on the channel with a timeout.
func (b *RequestBroker) Register(requestID string) <-chan *pb.DirectoryListingEvent {
	ch := make(chan *pb.DirectoryListingEvent, 1)
	b.mu.Lock()
	b.pending[requestID] = ch
	b.mu.Unlock()
	return ch
}

// Resolve delivers a response for the given request ID. It is a no-op if the
// request ID is unknown (e.g., already timed out and cancelled).
func (b *RequestBroker) Resolve(requestID string, evt *pb.DirectoryListingEvent) {
	b.mu.Lock()
	ch, ok := b.pending[requestID]
	if ok {
		delete(b.pending, requestID)
	}
	b.mu.Unlock()

	if ok {
		ch <- evt
	}
}

// Cancel removes a pending request. Used when the caller times out waiting for a response.
func (b *RequestBroker) Cancel(requestID string) {
	b.mu.Lock()
	delete(b.pending, requestID)
	b.mu.Unlock()
}
