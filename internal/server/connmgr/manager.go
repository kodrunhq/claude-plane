// Package connmgr tracks live agent connections in-memory with DB-backed
// status persistence. It provides the real-time view of which agents are
// connected while delegating durable state to the store layer.
package connmgr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// MachineStore is the subset of store operations needed by ConnectionManager.
// This interface allows testing with a mock store.
type MachineStore interface {
	UpsertMachine(machineID string, maxSessions int32) error
	UpdateMachineStatus(machineID, status string, lastSeenAt time.Time) error
}

// ConnectedAgent holds metadata about a connected agent.
type ConnectedAgent struct {
	MachineID    string
	RegisteredAt time.Time
	MaxSessions  int32
	Cancel       context.CancelFunc
	// Stream holds the gRPC stream reference. Typed as interface{} to avoid
	// importing proto package; will be type-asserted when needed.
	Stream interface{}
	// SendCommand sends a server command to the agent via the gRPC stream.
	// Set by the gRPC service when registering the agent. Callers (e.g.,
	// session handlers) use this to dispatch commands without knowing gRPC internals.
	SendCommand func(cmd *pb.ServerCommand) error
}

// AgentInfo is a public DTO for REST API responses.
type AgentInfo struct {
	MachineID   string    `json:"machine_id"`
	Status      string    `json:"status"`
	MaxSessions int32     `json:"max_sessions"`
	ConnectedAt time.Time `json:"connected_at"`
}

// ConnectionManager tracks connected agents in-memory and persists status
// changes to the database via MachineStore.
type ConnectionManager struct {
	mu        sync.RWMutex
	agents    map[string]*ConnectedAgent
	store     MachineStore
	logger    *slog.Logger
	publisher event.Publisher
}

// SetPublisher sets the event publisher used to emit machine connectivity events.
func (cm *ConnectionManager) SetPublisher(p event.Publisher) {
	cm.publisher = p
}

// publishEvent emits an event if a publisher is configured.
func (cm *ConnectionManager) publishEvent(ctx context.Context, e event.Event) {
	if cm.publisher != nil {
		if err := cm.publisher.Publish(ctx, e); err != nil {
			cm.logger.Warn("failed to publish event", "event_type", e.Type, "error", err)
		}
	}
}

// NewConnectionManager creates a new ConnectionManager backed by the given store.
// If logger is nil, slog.Default() is used.
func NewConnectionManager(store MachineStore, logger *slog.Logger) *ConnectionManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &ConnectionManager{
		agents: make(map[string]*ConnectedAgent),
		store:  store,
		logger: logger,
	}
}

// Register adds an agent to the in-memory map and updates the DB to "connected".
// If an agent with the same machineID is already registered, its Cancel function
// is called before replacement.
func (cm *ConnectionManager) Register(machineID string, agent *ConnectedAgent) error {
	// Capture old cancel under lock, but call it after unlock to avoid deadlock.
	var oldCancel context.CancelFunc
	cm.mu.Lock()
	if old, exists := cm.agents[machineID]; exists {
		oldCancel = old.Cancel
		cm.logger.Info("replacing existing agent connection", "machine_id", machineID)
	}
	cm.agents[machineID] = agent
	cm.mu.Unlock()

	if oldCancel != nil {
		oldCancel()
	}

	// Persist to DB outside the lock. On failure, roll back the in-memory state
	// only if it's still our agent (a newer connection may have replaced us).
	if err := cm.store.UpsertMachine(machineID, agent.MaxSessions); err != nil {
		cm.mu.Lock()
		if cm.agents[machineID] == agent {
			delete(cm.agents, machineID)
		}
		cm.mu.Unlock()
		return fmt.Errorf("upsert machine on register: %w", err)
	}
	if err := cm.store.UpdateMachineStatus(machineID, "connected", time.Now()); err != nil {
		cm.mu.Lock()
		if cm.agents[machineID] == agent {
			delete(cm.agents, machineID)
		}
		cm.mu.Unlock()
		return fmt.Errorf("update status on register: %w", err)
	}

	cm.logger.Info("agent registered", "machine_id", machineID, "max_sessions", agent.MaxSessions)
	cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineConnected, machineID))
	return nil
}

// Disconnect removes an agent from the in-memory map and updates the DB to "disconnected".
func (cm *ConnectionManager) Disconnect(machineID string) {
	cm.mu.Lock()
	delete(cm.agents, machineID)
	cm.mu.Unlock()

	// Persist to DB outside the lock
	if err := cm.store.UpdateMachineStatus(machineID, "disconnected", time.Now()); err != nil {
		cm.logger.Error("failed to update status on disconnect", "machine_id", machineID, "error", err)
	}

	cm.logger.Info("agent disconnected", "machine_id", machineID)
	cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineDisconnected, machineID))
}

// GetAgent returns the ConnectedAgent for the given machineID, or nil if not connected.
func (cm *ConnectionManager) GetAgent(machineID string) *ConnectedAgent {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.agents[machineID]
}

// ListAgents returns a snapshot of all currently connected agents as AgentInfo DTOs.
func (cm *ConnectionManager) ListAgents() []AgentInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make([]AgentInfo, 0, len(cm.agents))
	for _, a := range cm.agents {
		result = append(result, AgentInfo{
			MachineID:   a.MachineID,
			Status:      "connected",
			MaxSessions: a.MaxSessions,
			ConnectedAt: a.RegisteredAt,
		})
	}
	return result
}
