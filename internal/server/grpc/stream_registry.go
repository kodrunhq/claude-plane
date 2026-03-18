package grpc

import (
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// StreamEntry holds metadata about a connected agent's gRPC stream.
// This is the low-level, in-memory counterpart to connmgr.ConnectionManager:
//
//   - StreamRegistry (this package): tracks active gRPC streams and their
//     unique StreamToken values. The token prevents a stale stream handler
//     from removing an entry that belongs to a newer connection from the
//     same agent. Purely in-memory; no database interaction.
//
//   - connmgr.ConnectionManager: manages the full agent lifecycle with
//     DB-backed status persistence (connected/disconnected), cancel
//     functions for replacing old streams, and the SendCommand callback
//     used by session handlers.
type StreamEntry struct {
	MachineID     string
	StreamToken   uint64 // unique per connection, used to prevent stale removal
	MaxSessions   int32
	HomeDir       string
	ConnectedAt   time.Time
	SessionStates []*pb.SessionState
}

// streamTokenCounter generates unique stream tokens to prevent stale removal.
var streamTokenCounter atomic.Uint64

// NextStreamToken returns a unique stream token for a new connection.
func NextStreamToken() uint64 {
	return streamTokenCounter.Add(1)
}

// StreamRegistry tracks active agent gRPC streams in a thread-safe map.
// It is distinct from connmgr.ConnectionManager, which handles the higher-level
// agent lifecycle with DB persistence. See StreamEntry for details.
type StreamRegistry struct {
	mu     sync.RWMutex
	agents map[string]*StreamEntry
}

// NewStreamRegistry creates a new StreamRegistry.
func NewStreamRegistry() *StreamRegistry {
	return &StreamRegistry{
		agents: make(map[string]*StreamEntry),
	}
}

// Add registers or updates a connected agent's stream entry.
func (sr *StreamRegistry) Add(machineID string, info *StreamEntry) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.agents[machineID] = info
}

// RemoveIfToken unregisters a connected agent only if the stream token matches,
// preventing a stale stream handler from removing a newer connection.
func (sr *StreamRegistry) RemoveIfToken(machineID string, token uint64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if a, ok := sr.agents[machineID]; ok && a.StreamToken == token {
		delete(sr.agents, machineID)
	}
}

// Remove unregisters a connected agent unconditionally.
func (sr *StreamRegistry) Remove(machineID string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	delete(sr.agents, machineID)
}

// Get returns the StreamEntry for the given machine-id, if present.
func (sr *StreamRegistry) Get(machineID string) (*StreamEntry, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	a, ok := sr.agents[machineID]
	return a, ok
}

// List returns a snapshot of all currently tracked stream entries.
func (sr *StreamRegistry) List() []*StreamEntry {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	result := make([]*StreamEntry, 0, len(sr.agents))
	for _, a := range sr.agents {
		result = append(result, a)
	}
	return result
}
