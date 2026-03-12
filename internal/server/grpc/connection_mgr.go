package grpc

import (
	"sync"
	"time"

	pb "github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"
)

// ConnectedAgent holds metadata about a connected agent.
type ConnectedAgent struct {
	MachineID     string
	MaxSessions   int32
	ConnectedAt   time.Time
	SessionStates []*pb.SessionState
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

// Remove unregisters a connected agent.
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
