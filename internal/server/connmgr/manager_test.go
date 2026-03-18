package connmgr

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// mockStore records calls for assertions.
type mockStore struct {
	mu         sync.Mutex
	upserts    []upsertCall
	statusUpds []statusCall
}

type upsertCall struct {
	MachineID   string
	MaxSessions int32
	HomeDir     string
}

type statusCall struct {
	MachineID  string
	Status     string
	LastSeenAt time.Time
}

func (m *mockStore) UpsertMachine(machineID string, maxSessions int32, homeDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upserts = append(m.upserts, upsertCall{machineID, maxSessions, homeDir})
	return nil
}

func (m *mockStore) UpdateMachineStatus(machineID, status string, lastSeenAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusUpds = append(m.statusUpds, statusCall{machineID, status, lastSeenAt})
	return nil
}

func newTestManager() (*ConnectionManager, *mockStore) {
	ms := &mockStore{}
	cm := NewConnectionManager(ms, nil)
	return cm, ms
}

func TestRegisterAgent(t *testing.T) {
	cm, ms := newTestManager()

	cancel := func() {}
	agent := &ConnectedAgent{
		MachineID:    "m-001",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       cancel,
	}

	if err := cm.Register("m-001", agent); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify GetAgent returns it
	got := cm.GetAgent("m-001")
	if got == nil {
		t.Fatal("GetAgent returned nil")
	}
	if got.MachineID != "m-001" {
		t.Errorf("MachineID = %q, want %q", got.MachineID, "m-001")
	}

	// Verify ListAgents includes it
	agents := cm.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("ListAgents len = %d, want 1", len(agents))
	}
	if agents[0].MachineID != "m-001" {
		t.Errorf("ListAgents[0].MachineID = %q, want %q", agents[0].MachineID, "m-001")
	}

	// Verify store calls
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if len(ms.upserts) != 1 {
		t.Errorf("expected 1 UpsertMachine call, got %d", len(ms.upserts))
	}
	if len(ms.statusUpds) != 1 || ms.statusUpds[0].Status != "connected" {
		t.Errorf("expected UpdateMachineStatus(connected), got %+v", ms.statusUpds)
	}
}

func TestRegisterReplaces(t *testing.T) {
	cm, _ := newTestManager()

	oldCancelled := false
	agentA := &ConnectedAgent{
		MachineID:    "m-001",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       func() { oldCancelled = true },
	}
	agentB := &ConnectedAgent{
		MachineID:    "m-001",
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
	}

	if err := cm.Register("m-001", agentA); err != nil {
		t.Fatalf("Register A: %v", err)
	}
	if err := cm.Register("m-001", agentB); err != nil {
		t.Fatalf("Register B: %v", err)
	}

	if !oldCancelled {
		t.Error("old agent's Cancel was not called")
	}

	got := cm.GetAgent("m-001")
	if got == nil {
		t.Fatal("GetAgent returned nil after replacement")
	}
	if got.MaxSessions != 10 {
		t.Errorf("MaxSessions = %d, want 10 (new agent)", got.MaxSessions)
	}
}

func TestDisconnect(t *testing.T) {
	cm, ms := newTestManager()

	agent := &ConnectedAgent{
		MachineID:    "m-001",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       func() {},
	}
	if err := cm.Register("m-001", agent); err != nil {
		t.Fatalf("Register: %v", err)
	}

	cm.Disconnect("m-001")

	if got := cm.GetAgent("m-001"); got != nil {
		t.Errorf("GetAgent after disconnect should be nil, got %+v", got)
	}

	agents := cm.ListAgents()
	if len(agents) != 0 {
		t.Errorf("ListAgents after disconnect should be empty, got %d", len(agents))
	}

	// Verify store was called with disconnected
	ms.mu.Lock()
	defer ms.mu.Unlock()
	found := false
	for _, c := range ms.statusUpds {
		if c.MachineID == "m-001" && c.Status == "disconnected" {
			found = true
		}
	}
	if !found {
		t.Error("expected UpdateMachineStatus(disconnected) call")
	}
}

func TestListMultipleAgents(t *testing.T) {
	cm, _ := newTestManager()

	for _, id := range []string{"m-001", "m-002", "m-003"} {
		agent := &ConnectedAgent{
			MachineID:    id,
			RegisteredAt: time.Now(),
			MaxSessions:  5,
			Cancel:       func() {},
		}
		if err := cm.Register(id, agent); err != nil {
			t.Fatalf("Register(%q): %v", id, err)
		}
	}

	agents := cm.ListAgents()
	if len(agents) != 3 {
		t.Fatalf("ListAgents len = %d, want 3", len(agents))
	}

	// Verify all IDs present
	ids := map[string]bool{}
	for _, a := range agents {
		ids[a.MachineID] = true
	}
	for _, id := range []string{"m-001", "m-002", "m-003"} {
		if !ids[id] {
			t.Errorf("missing agent %q in ListAgents", id)
		}
	}
}

// failingMockStore fails UpsertMachine after failAfter successful calls.
type failingMockStore struct {
	mu        sync.Mutex
	callCount int
	failAfter int
}

func (m *failingMockStore) UpsertMachine(machineID string, maxSessions int32, homeDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.callCount > m.failAfter {
		return fmt.Errorf("simulated DB failure")
	}
	return nil
}

func (m *failingMockStore) UpdateMachineStatus(machineID, status string, lastSeenAt time.Time) error {
	return nil
}

func TestRegister_DBFailureDoesNotDeleteNewerConnection(t *testing.T) {
	failStore := &failingMockStore{failAfter: 1}
	cm := NewConnectionManager(failStore, nil)

	// First registration succeeds
	agent1 := &ConnectedAgent{
		MachineID:   "m1",
		Cancel:      func() {},
		SendCommand: func(cmd *pb.ServerCommand) error { return nil },
	}
	if err := cm.Register("m1", agent1); err != nil {
		t.Fatalf("Register agent1: %v", err)
	}

	// Second registration will fail on DB upsert
	agent2 := &ConnectedAgent{
		MachineID:   "m1",
		Cancel:      func() {},
		SendCommand: func(cmd *pb.ServerCommand) error { return fmt.Errorf("agent2") },
	}
	if err := cm.Register("m1", agent2); err == nil {
		t.Fatal("expected error from Register agent2")
	}

	// The identity check ensures rollback only removes our own failed agent,
	// not a different agent that may have registered concurrently.
	got := cm.GetAgent("m1")
	if got != nil {
		// If something is there, verify it's not the failed agent2
		if err := got.SendCommand(nil); err != nil {
			t.Error("agent in map should not be the failed agent2")
		}
	}
}

func TestDisconnectIfMatch_MatchingAgent(t *testing.T) {
	cm, ms := newTestManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agent := &ConnectedAgent{
		MachineID:    "worker-1",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       cancel,
		Ctx:          ctx,
	}
	if err := cm.Register("worker-1", agent); err != nil {
		t.Fatalf("register: %v", err)
	}

	disconnected := cm.DisconnectIfMatch("worker-1", agent)
	if !disconnected {
		t.Fatal("expected DisconnectIfMatch to return true for matching agent")
	}
	if got := cm.GetAgent("worker-1"); got != nil {
		t.Fatal("expected agent to be removed after DisconnectIfMatch")
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()
	found := false
	for _, c := range ms.statusUpds {
		if c.MachineID == "worker-1" && c.Status == "disconnected" {
			found = true
		}
	}
	if !found {
		t.Error("expected UpdateMachineStatus(disconnected) call")
	}
}

func TestDisconnectIfMatch_DifferentAgent(t *testing.T) {
	cm, _ := newTestManager()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	oldAgent := &ConnectedAgent{
		MachineID:    "worker-1",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       cancel1,
		Ctx:          ctx1,
	}
	newAgent := &ConnectedAgent{
		MachineID:    "worker-1",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       cancel2,
		Ctx:          ctx2,
	}

	if err := cm.Register("worker-1", oldAgent); err != nil {
		t.Fatalf("register old: %v", err)
	}
	if err := cm.Register("worker-1", newAgent); err != nil {
		t.Fatalf("register new: %v", err)
	}

	disconnected := cm.DisconnectIfMatch("worker-1", oldAgent)
	if disconnected {
		t.Fatal("expected false for non-matching agent")
	}
	if got := cm.GetAgent("worker-1"); got != newAgent {
		t.Fatal("new agent should still be registered")
	}
}

func TestDisconnectIfMatch_NotRegistered(t *testing.T) {
	cm, _ := newTestManager()
	agent := &ConnectedAgent{MachineID: "worker-1"}
	disconnected := cm.DisconnectIfMatch("worker-1", agent)
	if disconnected {
		t.Fatal("expected false when machine not registered")
	}
}

func TestStartHealthCheck_RemovesStaleAgent(t *testing.T) {
	cm, _ := newTestManager()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately — simulates dead transport

	agent := &ConnectedAgent{
		MachineID:    "stale-worker",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       func() {},
		Ctx:          ctx,
	}
	cm.mu.Lock()
	cm.agents["stale-worker"] = agent
	cm.mu.Unlock()

	healthCtx, healthCancel := context.WithCancel(context.Background())
	defer healthCancel()
	cm.StartHealthCheck(healthCtx, 50*time.Millisecond)

	time.Sleep(200 * time.Millisecond)

	if got := cm.GetAgent("stale-worker"); got != nil {
		t.Fatal("expected stale agent to be removed by health check")
	}
}

func TestSweepStaleAgents_MixedHealthyAndStale(t *testing.T) {
	cm, _ := newTestManager()

	// Live agent: context is NOT cancelled.
	liveCtx, liveCancel := context.WithCancel(context.Background())
	defer liveCancel()
	liveAgent := &ConnectedAgent{
		MachineID:    "healthy-worker",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       liveCancel,
		Ctx:          liveCtx,
	}

	// Stale agent: context is already cancelled.
	staleCtx, staleCancel := context.WithCancel(context.Background())
	staleCancel() // cancel immediately to simulate dead transport
	staleAgent := &ConnectedAgent{
		MachineID:    "stale-worker",
		RegisteredAt: time.Now(),
		MaxSessions:  5,
		Cancel:       func() {},
		Ctx:          staleCtx,
	}

	// Insert both directly into the agents map (same package access).
	cm.mu.Lock()
	cm.agents["healthy-worker"] = liveAgent
	cm.agents["stale-worker"] = staleAgent
	cm.mu.Unlock()

	cm.sweepStaleAgents()

	// Live agent should remain.
	if got := cm.GetAgent("healthy-worker"); got == nil {
		t.Error("expected healthy agent to remain after sweep")
	}

	// Stale agent should be removed.
	if got := cm.GetAgent("stale-worker"); got != nil {
		t.Error("expected stale agent to be removed after sweep")
	}

	// Verify total agents count.
	agents := cm.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent after sweep, got %d", len(agents))
	}
}

func TestConcurrentAccess(t *testing.T) {
	cm, _ := newTestManager()

	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "m-concurrent"
			agent := &ConnectedAgent{
				MachineID:    id,
				RegisteredAt: time.Now(),
				MaxSessions:  int32(n),
				Cancel:       func() {},
			}
			_ = cm.Register(id, agent)
			cm.GetAgent(id)
			cm.ListAgents()
			cm.Disconnect(id)
		}(i)
	}
	wg.Wait()
}
