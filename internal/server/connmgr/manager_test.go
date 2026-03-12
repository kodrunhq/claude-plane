package connmgr

import (
	"sync"
	"testing"
	"time"
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
}

type statusCall struct {
	MachineID  string
	Status     string
	LastSeenAt time.Time
}

func (m *mockStore) UpsertMachine(machineID string, maxSessions int32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upserts = append(m.upserts, upsertCall{machineID, maxSessions})
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
