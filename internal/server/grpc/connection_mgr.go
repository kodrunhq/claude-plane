package grpc

import (
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

// ConnectedAgent holds metadata about a connected agent.
type ConnectedAgent struct {
	MachineID     string
	StreamToken   uint64 // unique per connection, used to prevent stale removal
	MaxSessions   int32
	ConnectedAt   time.Time
	SessionStates []*pb.SessionState
}

// streamTokenCounter generates unique stream tokens to prevent stale removal.
var streamTokenCounter atomic.Uint64

// NextStreamToken returns a unique stream token for a new connection.
func NextStreamToken() uint64 {
	return streamTokenCounter.Add(1)
}

// ConnectionManager tracks connected agents in a thread-safe map.
type ConnectionManager struct {
	mu     sync.RWMutex
	agents map[string]*ConnectedAgent
}

// NewConnectionManager creates a new ConnectionManager.
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		agents: make(map[string]*ConnectedAgent),
	}
}

// Add registers or updates a connected agent.
func (cm *ConnectionManager) Add(machineID string, info *ConnectedAgent) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.agents[machineID] = info
}

// RemoveIfToken unregisters a connected agent only if the stream token matches,
// preventing a stale stream handler from removing a newer connection.
func (cm *ConnectionManager) RemoveIfToken(machineID string, token uint64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if a, ok := cm.agents[machineID]; ok && a.StreamToken == token {
		delete(cm.agents, machineID)
	}
}

// Remove unregisters a connected agent unconditionally.
func (cm *ConnectionManager) Remove(machineID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.agents, machineID)
}

// Get returns the ConnectedAgent for the given machine-id, if present.
func (cm *ConnectionManager) Get(machineID string) (*ConnectedAgent, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	a, ok := cm.agents[machineID]
	return a, ok
}

// List returns a snapshot of all currently connected agents.
func (cm *ConnectionManager) List() []*ConnectedAgent {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]*ConnectedAgent, 0, len(cm.agents))
	for _, a := range cm.agents {
		result = append(result, a)
	}
	return result
}
