package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// --- Mocks ---

type failedRecord struct {
	InjectionID string
	Reason      string
}

type mockAuditStore struct {
	mu         sync.Mutex
	delivered  []string // injection IDs that were marked delivered
	failed     []failedRecord
	deliverErr error
	failErr    error
}

func (m *mockAuditStore) UpdateInjectionDelivered(_ context.Context, injectionID string, _ time.Time) error {
	if m.deliverErr != nil {
		return m.deliverErr
	}
	m.mu.Lock()
	m.delivered = append(m.delivered, injectionID)
	m.mu.Unlock()
	return nil
}

func (m *mockAuditStore) UpdateInjectionFailed(_ context.Context, injectionID string, reason string) error {
	if m.failErr != nil {
		return m.failErr
	}
	m.mu.Lock()
	m.failed = append(m.failed, failedRecord{InjectionID: injectionID, Reason: reason})
	m.mu.Unlock()
	return nil
}

func (m *mockAuditStore) getDelivered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.delivered))
	copy(result, m.delivered)
	return result
}

func (m *mockAuditStore) getFailed() []failedRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]failedRecord, len(m.failed))
	copy(result, m.failed)
	return result
}

type mockSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*store.Session
}

func (m *mockSessionStore) GetSession(id string) (*store.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s: not found", id)
	}
	// Return a copy to avoid data races on the Status field.
	sessCopy := *sess
	return &sessCopy, nil
}

func (m *mockSessionStore) setStatus(id, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sess, ok := m.sessions[id]; ok {
		sess.Status = status
	}
}

type mockSubscriber struct {
	mu       sync.Mutex
	handlers []event.HandlerFunc
}

func (m *mockSubscriber) Subscribe(_ string, handler event.HandlerFunc, _ event.SubscriberOptions) func() {
	m.mu.Lock()
	m.handlers = append(m.handlers, handler)
	m.mu.Unlock()
	return func() {}
}

func (m *mockSubscriber) fireEvent(evt event.Event) {
	m.mu.Lock()
	handlers := make([]event.HandlerFunc, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	for _, h := range handlers {
		_ = h(context.Background(), evt)
	}
}

// capturedCommand records what was sent via SendCommand.
type capturedCommand struct {
	mu       sync.Mutex
	commands []*pb.ServerCommand
	err      error
}

func (c *capturedCommand) send(cmd *pb.ServerCommand) error {
	if c.err != nil {
		return c.err
	}
	c.mu.Lock()
	c.commands = append(c.commands, cmd)
	c.mu.Unlock()
	return nil
}

func (c *capturedCommand) getCommands() []*pb.ServerCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]*pb.ServerCommand, len(c.commands))
	copy(result, c.commands)
	return result
}

// --- Helpers ---

func newTestConnMgr(t *testing.T, machineID string, captured *capturedCommand) *connmgr.ConnectionManager {
	t.Helper()
	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	agent := &connmgr.ConnectedAgent{
		MachineID:    machineID,
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
		SendCommand:  captured.send,
	}
	if err := cm.Register(machineID, agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	return cm
}

type noopMachineStore struct{}

func (n *noopMachineStore) UpsertMachine(_ string, _ int32) error                        { return nil }
func (n *noopMachineStore) UpdateMachineStatus(_ string, _ string, _ time.Time) error     { return nil }

func newRunningSessionStore(sessionID, machineID string) *mockSessionStore {
	return &mockSessionStore{
		sessions: map[string]*store.Session{
			sessionID: {
				SessionID: sessionID,
				MachineID: machineID,
				Status:    store.StatusRunning,
			},
		},
	}
}

func waitFor(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// --- Tests ---

func TestInjectionQueue_EnqueueToRunningSession(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("hello\n"), 0, "inj-1")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return len(captured.getCommands()) == 1
	})

	cmds := captured.getCommands()
	inputData := cmds[0].GetInputData()
	if inputData == nil {
		t.Fatal("expected InputDataCmd")
	}
	if inputData.SessionId != "sess-1" {
		t.Errorf("SessionId = %q, want %q", inputData.SessionId, "sess-1")
	}
	if string(inputData.Data) != "hello\n" {
		t.Errorf("Data = %q, want %q", string(inputData.Data), "hello\n")
	}
}

func TestInjectionQueue_EnqueueWithDelay(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	start := time.Now()
	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("delayed\n"), 50, "inj-delay")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return len(captured.getCommands()) == 1
	})

	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected at least 40ms delay, got %v", elapsed)
	}
}

func TestInjectionQueue_EnqueueToNonRunningSession(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := &mockSessionStore{
		sessions: map[string]*store.Session{
			"sess-done": {
				SessionID: "sess-done",
				MachineID: "machine-1",
				Status:    store.StatusCompleted,
			},
		},
	}
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-done", "machine-1", []byte("hello\n"), 0, "inj-2")
	if err == nil {
		t.Fatal("expected error for terminal session")
	}
}

func TestInjectionQueue_EnqueueQueueFull(t *testing.T) {
	// Use a SendCommand that blocks forever so items stay in the channel.
	blockCh := make(chan struct{})

	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	agent := &connmgr.ConnectedAgent{
		MachineID:    "machine-1",
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
		SendCommand: func(_ *pb.ServerCommand) error {
			<-blockCh
			return nil
		},
	}
	_ = cm.Register("machine-1", agent)

	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	// Close blockCh first (LIFO) so the drainer unblocks before Close() waits.
	defer q.Close()
	defer close(blockCh)

	// The drainer will pick up the first item and block on SendCommand.
	// We fill the channel capacity, then wait for the drainer to consume one
	// (freeing a slot), then fill again until it overflows.
	for i := 0; i < defaultQueueCapacity; i++ {
		err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("x"), 0, fmt.Sprintf("inj-%d", i))
		if err != nil {
			t.Fatalf("Enqueue %d: unexpected error: %v", i, err)
		}
	}

	// Wait for drainer to pick up first item and block on SendCommand,
	// freeing one slot in the channel.
	time.Sleep(50 * time.Millisecond)

	// Fill the freed slot.
	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("x"), 0, "inj-extra")
	if err != nil {
		t.Fatalf("Enqueue extra: unexpected error: %v", err)
	}

	// Now the channel should be full again.
	err = q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("overflow"), 0, "inj-overflow")
	if err == nil {
		t.Fatal("expected queue-full error")
	}
}

func TestInjectionQueue_AgentDisconnected(t *testing.T) {
	// Create a connmgr with no agents registered.
	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("hello\n"), 0, "inj-noagent")
	if err != nil {
		t.Fatalf("Enqueue should succeed (item queued): %v", err)
	}

	// Wait a bit — the drainer should log a warning but not crash.
	time.Sleep(100 * time.Millisecond)

	// Verify no delivery was recorded.
	if len(audit.getDelivered()) != 0 {
		t.Error("expected no deliveries when agent is disconnected")
	}
}

func TestInjectionQueue_SessionExitEvent(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	// Enqueue an item to create the drainer.
	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("hi\n"), 0, "inj-evt")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return len(captured.getCommands()) == 1
	})

	// Fire session exited event.
	sub.fireEvent(event.Event{
		Type:    event.TypeSessionExited,
		Payload: map[string]any{"session_id": "sess-1"},
	})

	// Wait for drainer to exit and queue to be removed.
	waitFor(t, 2*time.Second, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		_, exists := q.queues["sess-1"]
		return !exists
	})
}

func TestInjectionQueue_IdleDrainerExit(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	q.idleTimeout = 50 * time.Millisecond
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("hi\n"), 0, "inj-idle")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return len(captured.getCommands()) == 1
	})

	// Wait for idle timeout to clean up.
	waitFor(t, 2*time.Second, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		_, exists := q.queues["sess-1"]
		return !exists
	})
}

func TestInjectionQueue_StaleItemsDropped(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	// Directly create a queue and inject a stale item.
	sq := q.getOrCreateQueue("sess-1", "machine-1")
	sq.items <- queueItem{
		InjectionID: "inj-stale",
		Data:        []byte("old data\n"),
		QueuedAt:    time.Now().Add(-6 * time.Minute), // 6 minutes ago = stale
	}

	// Give the drainer time to process.
	time.Sleep(100 * time.Millisecond)

	// The stale item should have been dropped — no command sent.
	if len(captured.getCommands()) != 0 {
		t.Error("expected stale item to be dropped, but command was sent")
	}
	if len(audit.getDelivered()) != 0 {
		t.Error("expected no delivery record for stale item")
	}
}

func TestInjectionQueue_AuditRecordUpdated(t *testing.T) {
	captured := &capturedCommand{}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("audit me\n"), 0, "inj-audit")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	waitFor(t, 2*time.Second, func() bool {
		return len(audit.getDelivered()) == 1
	})

	delivered := audit.getDelivered()
	if delivered[0] != "inj-audit" {
		t.Errorf("delivered injection ID = %q, want %q", delivered[0], "inj-audit")
	}
}

func TestProcessItem_TerminalSession(t *testing.T) {
	// Use a blocking SendCommand so we can verify it is never called.
	blockCh := make(chan struct{})
	defer close(blockCh)

	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	agent := &connmgr.ConnectedAgent{
		MachineID:    "machine-1",
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
		SendCommand: func(_ *pb.ServerCommand) error {
			<-blockCh
			return nil
		},
	}
	_ = cm.Register("machine-1", agent)

	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-toctou", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	// Enqueue an item while the session is still running.
	err := q.Enqueue(context.Background(), "sess-toctou", "machine-1", []byte("data\n"), 0, "inj-toctou")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Simulate the TOCTOU window: session transitions to terminal status
	// before processItem runs the re-check.
	sessions.setStatus("sess-toctou", store.StatusTerminated)

	// Wait for processItem to call UpdateInjectionFailed("session terminated").
	waitFor(t, 2*time.Second, func() bool {
		failed := audit.getFailed()
		for _, f := range failed {
			if f.InjectionID == "inj-toctou" && f.Reason == "session terminated" {
				return true
			}
		}
		return false
	})

	// Confirm no gRPC delivery was attempted.
	if len(audit.getDelivered()) != 0 {
		t.Error("expected no delivery when session is in terminal status")
	}
}

func TestInjectionQueue_SendCommandError(t *testing.T) {
	captured := &capturedCommand{err: errors.New("send failed")}
	cm := newTestConnMgr(t, "machine-1", captured)
	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())
	defer q.Close()

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("fail\n"), 0, "inj-fail")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Give drainer time to attempt delivery.
	time.Sleep(100 * time.Millisecond)

	// No delivery should be recorded on send failure.
	if len(audit.getDelivered()) != 0 {
		t.Error("expected no delivery record when SendCommand fails")
	}
}

func TestInjectionQueue_CloseBlocksUntilDrainersExit(t *testing.T) {
	// Use a SendCommand that blocks until we release it, so the drainer
	// is busy when Close() is called.
	blockCh := make(chan struct{})

	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	agent := &connmgr.ConnectedAgent{
		MachineID:    "machine-1",
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
		SendCommand: func(_ *pb.ServerCommand) error {
			<-blockCh
			return nil
		},
	}
	_ = cm.Register("machine-1", agent)

	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())

	err := q.Enqueue(context.Background(), "sess-1", "machine-1", []byte("block\n"), 0, "inj-block")
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Give the drainer time to pick up the item and block on SendCommand.
	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		q.Close()
		close(closeDone)
	}()

	// Close should be blocked because the drainer is stuck on SendCommand.
	select {
	case <-closeDone:
		t.Fatal("Close() returned before drainer exited")
	case <-time.After(100 * time.Millisecond):
		// expected: Close is still waiting
	}

	// Unblock the drainer.
	close(blockCh)

	select {
	case <-closeDone:
		// success: Close returned after drainer finished
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return after drainer was unblocked")
	}
}

func TestInjectionQueue_BufferedItemsMarkedFailedOnShutdown(t *testing.T) {
	// Use a SendCommand that blocks forever so items stay buffered.
	blockCh := make(chan struct{})

	cm := connmgr.NewConnectionManager(&noopMachineStore{}, slog.Default())
	agent := &connmgr.ConnectedAgent{
		MachineID:    "machine-1",
		RegisteredAt: time.Now(),
		MaxSessions:  10,
		Cancel:       func() {},
		SendCommand: func(_ *pb.ServerCommand) error {
			<-blockCh
			return nil
		},
	}
	_ = cm.Register("machine-1", agent)

	audit := &mockAuditStore{}
	sessions := newRunningSessionStore("sess-1", "machine-1")
	sub := &mockSubscriber{}

	q := NewInjectionQueue(cm, audit, sessions, sub, slog.Default())

	// Enqueue several items. The first will be picked up by the drainer
	// and block on SendCommand. The rest stay buffered in the channel.
	for i := 0; i < 5; i++ {
		err := q.Enqueue(context.Background(), "sess-1", "machine-1",
			[]byte(fmt.Sprintf("item-%d\n", i)), 0, fmt.Sprintf("inj-shut-%d", i))
		if err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Give the drainer time to pick up the first item and block.
	time.Sleep(50 * time.Millisecond)

	// Unblock the drainer's current SendCommand so it can proceed to the
	// shutdown drain loop. The done channel closing will cause it to drain
	// remaining items. There's a race between the drainer processing items
	// normally and the shutdown signal, so we check for at least 1 failed.
	close(blockCh)

	q.Close()

	failed := audit.getFailed()
	if len(failed) < 1 {
		t.Fatalf("expected at least 1 buffered item marked failed, got %d", len(failed))
	}

	for _, f := range failed {
		if f.Reason != "queue shutdown" {
			t.Errorf("expected reason %q, got %q", "queue shutdown", f.Reason)
		}
	}
}
