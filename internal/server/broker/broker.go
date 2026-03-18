// Package broker provides request-response correlation over gRPC bidirectional streams.
// It allows the server to send a command to an agent and wait for a matching response
// identified by a request ID.
package broker

import (
	"sync"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// pendingEntry stores the response channel alongside the machineID that
// initiated the request, so Resolve can verify the response comes from the
// expected agent.
type pendingEntry struct {
	ch        chan *pb.DirectoryListingEvent
	machineID string
}

// RequestBroker correlates request-response patterns over gRPC bidirectional streams.
// Callers register a request ID before sending a command, then select on the returned
// channel with a timeout. The agent sends back an event with the same request ID,
// which the gRPC receive loop delivers via Resolve.
type RequestBroker struct {
	mu      sync.Mutex
	pending map[string]pendingEntry
}

// New creates a new RequestBroker.
func New() *RequestBroker {
	return &RequestBroker{
		pending: make(map[string]pendingEntry),
	}
}

// Register creates a buffered (size 1) channel for the given request ID and returns it.
// The machineID is stored alongside the channel so Resolve can validate the responding agent.
// The caller should select on the channel with a timeout.
func (b *RequestBroker) Register(requestID, machineID string) <-chan *pb.DirectoryListingEvent {
	ch := make(chan *pb.DirectoryListingEvent, 1)
	b.mu.Lock()
	b.pending[requestID] = pendingEntry{ch: ch, machineID: machineID}
	b.mu.Unlock()
	return ch
}

// Resolve delivers a response for the given request ID. It validates that the
// responding machineID matches the one that registered the request. It is a
// no-op if the request ID is unknown (e.g., already timed out and cancelled)
// or if the machineID does not match.
func (b *RequestBroker) Resolve(requestID, machineID string, evt *pb.DirectoryListingEvent) {
	b.mu.Lock()
	entry, ok := b.pending[requestID]
	if ok && entry.machineID == machineID {
		delete(b.pending, requestID)
	} else {
		ok = false
	}
	b.mu.Unlock()

	if ok {
		entry.ch <- evt
	}
}

// Cancel removes a pending request. Used when the caller times out waiting for a response.
func (b *RequestBroker) Cancel(requestID string) {
	b.mu.Lock()
	delete(b.pending, requestID)
	b.mu.Unlock()
}
