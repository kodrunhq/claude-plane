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
	UpsertMachine(machineID string, maxSessions int32, homeDir string) error
	UpdateMachineStatus(machineID, status string, lastSeenAt time.Time) error
}

// machineNameLookup is an optional interface for resolving display names.
type machineNameLookup interface {
	GetMachineDisplayName(machineID string) string
}

// HealthInfo holds the latest health metrics reported by an agent.
type HealthInfo struct {
	CPUCores       int32 `json:"cpu_cores"`
	MemoryTotalMB  int64 `json:"memory_total_mb"`
	MemoryUsedMB   int64 `json:"memory_used_mb"`
	ActiveSessions int32 `json:"active_sessions"`
	MaxSessions    int32 `json:"max_sessions"`
}

// ConnectedAgent holds metadata about a connected agent.
type ConnectedAgent struct {
	MachineID    string
	RegisteredAt time.Time
	MaxSessions  int32
	HomeDir      string
	Cancel       context.CancelFunc
	// Ctx is the stream context — checked by the health sweep to detect dead transports.
	Ctx          context.Context
	// Stream holds the gRPC stream reference. Typed as interface{} to avoid
	// importing proto package; will be type-asserted when needed.
	Stream interface{}
	// SendCommand sends a server command to the agent via the gRPC stream.
	// Set by the gRPC service when registering the agent. Callers (e.g.,
	// session handlers) use this to dispatch commands without knowing gRPC internals.
	SendCommand func(cmd *pb.ServerCommand) error
	// health holds the latest health metrics from the agent.
	healthMu sync.RWMutex
	health   *HealthInfo
}

// UpdateHealth stores the latest health metrics for the agent.
func (ca *ConnectedAgent) UpdateHealth(h *HealthInfo) {
	ca.healthMu.Lock()
	ca.health = h
	ca.healthMu.Unlock()
}

// GetHealth returns a copy of the latest health metrics, or nil.
func (ca *ConnectedAgent) GetHealth() *HealthInfo {
	ca.healthMu.RLock()
	defer ca.healthMu.RUnlock()
	if ca.health == nil {
		return nil
	}
	h := *ca.health
	return &h
}

// AgentInfo is a public DTO for REST API responses.
type AgentInfo struct {
	MachineID   string      `json:"machine_id"`
	Status      string      `json:"status"`
	MaxSessions int32       `json:"max_sessions"`
	ConnectedAt time.Time   `json:"connected_at"`
	Health      *HealthInfo `json:"health,omitempty"`
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

// machineDisplayName resolves the display name for a machine if the store supports it.
func (cm *ConnectionManager) machineDisplayName(machineID string) string {
	if lookup, ok := cm.store.(machineNameLookup); ok {
		return lookup.GetMachineDisplayName(machineID)
	}
	return ""
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
	if err := cm.store.UpsertMachine(machineID, agent.MaxSessions, agent.HomeDir); err != nil {
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
	cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineConnected, machineID, cm.machineDisplayName(machineID)))
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
	cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineDisconnected, machineID, cm.machineDisplayName(machineID)))
}

// DisconnectIfMatch removes the agent only if the provided pointer matches
// the currently registered agent. This prevents a closing stream from
// removing a newer replacement connection. Returns true if the agent was
// actually removed.
func (cm *ConnectionManager) DisconnectIfMatch(machineID string, agent *ConnectedAgent) bool {
	cm.mu.Lock()
	current, exists := cm.agents[machineID]
	if !exists || current != agent {
		cm.mu.Unlock()
		return false
	}
	delete(cm.agents, machineID)
	cm.mu.Unlock()

	if err := cm.store.UpdateMachineStatus(machineID, "disconnected", time.Now()); err != nil {
		cm.logger.Error("failed to update status on disconnect", "machine_id", machineID, "error", err)
	}

	cm.logger.Info("agent disconnected", "machine_id", machineID)
	cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineDisconnected, machineID, cm.machineDisplayName(machineID)))
	return true
}

// StartHealthCheck begins a periodic sweep that detects and removes agents
// whose stream context has been cancelled (dead transport).
func (cm *ConnectionManager) StartHealthCheck(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cm.sweepStaleAgents()
			}
		}
	}()
}

// sweepStaleAgents checks all registered agents and removes any whose
// stream context is done (transport died). Publishes TypeMachineStale
// instead of TypeMachineDisconnected to distinguish proactive cleanup.
func (cm *ConnectionManager) sweepStaleAgents() {
	cm.mu.RLock()
	var stale []*ConnectedAgent
	for _, agent := range cm.agents {
		if agent.Ctx != nil && agent.Ctx.Err() != nil {
			stale = append(stale, agent)
		}
	}
	cm.mu.RUnlock()

	for _, agent := range stale {
		cm.mu.Lock()
		current, exists := cm.agents[agent.MachineID]
		if !exists || current != agent {
			cm.mu.Unlock()
			continue
		}
		delete(cm.agents, agent.MachineID)
		cm.mu.Unlock()

		if agent.Cancel != nil {
			agent.Cancel() // Release goroutines blocked on the stream context
		}

		if err := cm.store.UpdateMachineStatus(agent.MachineID, "disconnected", time.Now()); err != nil {
			cm.logger.Error("failed to update stale agent status", "machine_id", agent.MachineID, "error", err)
		}

		cm.logger.Warn("removed stale agent (dead transport)", "machine_id", agent.MachineID)
		cm.publishEvent(context.Background(), event.NewMachineEvent(event.TypeMachineStale, agent.MachineID, cm.machineDisplayName(agent.MachineID)))
	}
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
			Health:      a.GetHealth(),
		})
	}
	return result
}
