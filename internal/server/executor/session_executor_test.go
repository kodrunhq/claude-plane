package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/connmgr"
	"github.com/kodrunhq/claude-plane/internal/server/orchestrator"
	"github.com/kodrunhq/claude-plane/internal/server/store"
	"github.com/kodrunhq/claude-plane/internal/shared/cliutil"
	pb "github.com/kodrunhq/claude-plane/internal/shared/proto/claudeplane/v1"
)

// --- Mock store ---

type mockStore struct {
	mu       sync.Mutex
	sessions map[string]*store.Session

	createErr        error
	getErr           error
	updateStatusErr  error
	updateRunStepErr error
}

func newMockStore() *mockStore {
	return &mockStore{sessions: make(map[string]*store.Session)}
}

func (m *mockStore) CreateSession(sess *store.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	m.sessions[sess.SessionID] = &store.Session{
		SessionID:  sess.SessionID,
		MachineID:  sess.MachineID,
		Command:    sess.Command,
		WorkingDir: sess.WorkingDir,
		Status:     sess.Status,
	}
	return nil
}

func (m *mockStore) GetSession(id string) (*store.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	sess, ok := m.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session %s: %w", id, store.ErrNotFound)
	}
	return &store.Session{
		SessionID:  sess.SessionID,
		MachineID:  sess.MachineID,
		Command:    sess.Command,
		WorkingDir: sess.WorkingDir,
		Status:     sess.Status,
	}, nil
}

func (m *mockStore) UpdateSessionStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	sess, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %s: %w", id, store.ErrNotFound)
	}
	sess.Status = status
	return nil
}

func (m *mockStore) UpdateRunStepStatus(_ context.Context, _, _, _ string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateRunStepErr
}

func (m *mockStore) UpdateRunStepErrorMessage(_ context.Context, _, _ string) error {
	return nil
}

// hasSession returns true if at least one session exists in the store.
func (m *mockStore) hasSession() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions) > 0
}

// setAllSessionStatus sets the status of all sessions in the store.
func (m *mockStore) setAllSessionStatus(status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sess := range m.sessions {
		sess.Status = status
	}
}

// deleteAllSessions removes all sessions from the store.
func (m *mockStore) deleteAllSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.sessions {
		delete(m.sessions, id)
	}
}

// --- Helpers ---

// waitForSession polls until the mock store has at least one session or timeout.
func waitForSession(t *testing.T, ms *mockStore, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ms.hasSession() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for session to appear in store")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// registerMockAgent creates a ConnectedAgent in the connmgr with a recording SendCommand.
func registerMockAgent(t *testing.T, cm *connmgr.ConnectionManager, machineID string) *[]*pb.ServerCommand {
	t.Helper()
	var mu sync.Mutex
	var sent []*pb.ServerCommand
	agent := &connmgr.ConnectedAgent{
		MachineID:    machineID,
		RegisteredAt: time.Now(),
		MaxSessions:  4,
		Cancel:       func() {},
		SendCommand: func(cmd *pb.ServerCommand) error {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, cmd)
			return nil
		},
	}
	if err := cm.Register(machineID, agent); err != nil {
		t.Fatalf("register mock agent: %v", err)
	}
	return &sent
}

// mockMachineStore satisfies connmgr.MachineStore for test setup.
type mockMachineStore struct{}

func (mockMachineStore) UpsertMachine(string, int32, string) error            { return nil }
func (mockMachineStore) UpdateMachineStatus(string, string, time.Time) error { return nil }

func newTestConnMgr() *connmgr.ConnectionManager {
	return connmgr.NewConnectionManager(mockMachineStore{}, nil)
}

func makeRunStep(machineID string) store.RunStep {
	return store.RunStep{
		RunStepID:          "rs-1",
		RunID:              "run-1",
		StepID:             "step-1",
		MachineIDSnapshot:  machineID,
		CommandSnapshot:    "claude",
		ArgsSnapshot:       `["--model","opus"]`,
		WorkingDirSnapshot: "/tmp/work",
	}
}

// waitForCompletion blocks until onComplete fires or timeout.
func waitForCompletion(t *testing.T, ch <-chan int, timeout time.Duration) int {
	t.Helper()
	select {
	case code := <-ch:
		return code
	case <-time.After(timeout):
		t.Fatal("timed out waiting for step completion")
		return -1
	}
}

// --- Tests ---

func TestExecuteStep_AgentNotConnected(t *testing.T) {
	cm := newTestConnMgr()
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("no-such-machine"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	code := waitForCompletion(t, ch, 2*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestExecuteStep_CreateSessionStoreFails(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")

	ms := newMockStore()
	ms.createErr = errors.New("db down")
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	code := waitForCompletion(t, ch, 2*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestExecuteStep_SendCommandFails(t *testing.T) {
	cm := newTestConnMgr()
	agent := &connmgr.ConnectedAgent{
		MachineID:    "m1",
		RegisteredAt: time.Now(),
		MaxSessions:  4,
		Cancel:       func() {},
		SendCommand: func(_ *pb.ServerCommand) error {
			return errors.New("stream broken")
		},
	}
	if err := cm.Register("m1", agent); err != nil {
		t.Fatal(err)
	}

	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	code := waitForCompletion(t, ch, 2*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
	// Session should be marked failed in store.
	ms.mu.Lock()
	defer ms.mu.Unlock()
	for _, sess := range ms.sessions {
		if sess.Status != store.StatusFailed {
			t.Errorf("session status = %q, want %q", sess.Status, store.StatusFailed)
		}
	}
}

func TestExecuteStep_SessionCompletes(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(stepID string, exitCode int) {
		if stepID != "step-1" {
			t.Errorf("stepID = %q, want %q", stepID, "step-1")
		}
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestExecuteStep_SessionFails(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusFailed)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestExecuteStep_SessionTerminated(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusTerminated)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestExecuteStep_ContextCancelled(t *testing.T) {
	cm := newTestConnMgr()
	sent := registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan int, 1)
	exec.ExecuteStep(ctx, makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	cancel()

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}

	// Verify a KillSession command was sent to the agent.
	hasKill := false
	for _, cmd := range *sent {
		if cmd.GetKillSession() != nil {
			hasKill = true
			break
		}
	}
	if !hasKill {
		t.Error("expected KillSession command to be sent on context cancellation")
	}
}

func TestExecuteStep_SessionNotFoundExhausted(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.deleteAllSessions()

	code := waitForCompletion(t, ch, 10*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestExecuteStep_TransientDBError(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)

	// Inject a transient (non-ErrNotFound) error for a few polls.
	ms.mu.Lock()
	ms.getErr = errors.New("connection reset")
	ms.mu.Unlock()

	// Let a few polls fail with transient error, then recover.
	time.Sleep(100 * time.Millisecond)
	ms.mu.Lock()
	ms.getErr = nil
	ms.mu.Unlock()

	// Mark completed — should still resolve since transient errors don't count.
	ms.setAllSessionStatus(store.StatusCompleted)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0 after transient errors, got %d", code)
	}
}

func TestExecuteStep_DefaultCommand(t *testing.T) {
	cm := newTestConnMgr()
	sent := registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	rs := makeRunStep("m1")
	rs.CommandSnapshot = "" // empty → should default to "claude"

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), rs, nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)
	waitForCompletion(t, ch, 5*time.Second)

	if len(*sent) == 0 {
		t.Fatal("no commands sent to agent")
	}
	cs := (*sent)[0].GetCreateSession()
	if cs == nil {
		t.Fatal("expected CreateSession command")
	}
	if cs.Command != defaultCommand {
		t.Errorf("command = %q, want %q", cs.Command, defaultCommand)
	}
}

func TestExecuteStep_SendsCreateSessionWithCorrectFields(t *testing.T) {
	cm := newTestConnMgr()
	sent := registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)
	waitForCompletion(t, ch, 5*time.Second)

	if len(*sent) == 0 {
		t.Fatal("no commands sent")
	}
	cs := (*sent)[0].GetCreateSession()
	if cs == nil {
		t.Fatal("expected CreateSession")
	}
	if cs.WorkingDir != "/tmp/work" {
		t.Errorf("working_dir = %q, want %q", cs.WorkingDir, "/tmp/work")
	}
	// Executor prepends --dangerously-skip-permissions by default (nil SkipPermissionsSnapshot).
	wantArgs := []string{"--dangerously-skip-permissions", "--model", "opus"}
	if len(cs.Args) != len(wantArgs) {
		t.Errorf("args = %v, want %v", cs.Args, wantArgs)
	} else {
		for i, a := range wantArgs {
			if cs.Args[i] != a {
				t.Errorf("args[%d] = %q, want %q", i, cs.Args[i], a)
			}
		}
	}
	if cs.TerminalSize == nil {
		t.Fatal("terminal_size is nil")
	}
	if cs.TerminalSize.Rows != defaultTermRows || cs.TerminalSize.Cols != defaultTermCols {
		t.Errorf("terminal size = %dx%d, want %dx%d",
			cs.TerminalSize.Rows, cs.TerminalSize.Cols, defaultTermRows, defaultTermCols)
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty string", "", nil},
		{"whitespace", "  ", nil},
		{"valid array", `["--model","opus"]`, []string{"--model", "opus"}},
		{"single element", `["hello"]`, []string{"hello"}},
		{"invalid json", `not json`, nil},
		{"json object", `{"key":"val"}`, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cliutil.ParseArgs(tc.input)
			if tc.want == nil {
				if len(got) != 0 {
					t.Errorf("cliutil.ParseArgs(%q) = %v, want empty", tc.input, got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Errorf("cliutil.ParseArgs(%q) len = %d, want %d", tc.input, len(got), len(tc.want))
				return
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("cliutil.ParseArgs(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestCompleteStep_UnknownSession(t *testing.T) {
	cm := newTestConnMgr()
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	// Should not panic for unknown session.
	exec.completeStep(context.Background(), "nonexistent-session", 0)
}

func TestNewSessionStepExecutor_NilLogger(t *testing.T) {
	cm := newTestConnMgr()
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)
	if exec.logger == nil {
		t.Error("expected non-nil logger when nil passed")
	}
}

// --- Shell task tests ---

func makeShellRunStep(machineID string) store.RunStep {
	return store.RunStep{
		RunStepID:          "rs-shell-1",
		RunID:              "run-1",
		StepID:             "step-shell-1",
		MachineIDSnapshot:  machineID,
		CommandSnapshot:    "/bin/bash",
		ArgsSnapshot:       `["-c","echo hello"]`,
		WorkingDirSnapshot: "/tmp/work",
		TaskTypeSnapshot:   "shell",
	}
}

func TestShellTask_Success(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeShellRunStep("m1"), nil, func(stepID string, exitCode int) {
		if stepID != "step-shell-1" {
			t.Errorf("stepID = %q, want %q", stepID, "step-shell-1")
		}
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestShellTask_Failure(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeShellRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusFailed)

	code := waitForCompletion(t, ch, 5*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d, got %d", failureExitCode, code)
	}
}

func TestShellTask_NoIdleDetectorFields(t *testing.T) {
	cm := newTestConnMgr()
	sent := registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), makeShellRunStep("m1"), nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)
	waitForCompletion(t, ch, 5*time.Second)

	if len(*sent) == 0 {
		t.Fatal("no commands sent to agent")
	}
	cs := (*sent)[0].GetCreateSession()
	if cs == nil {
		t.Fatal("expected CreateSession command")
	}
	if cs.TaskType != "shell" {
		t.Errorf("task_type = %q, want %q", cs.TaskType, "shell")
	}
	if cs.InitialPrompt != "" {
		t.Errorf("initial_prompt = %q, want empty for shell tasks", cs.InitialPrompt)
	}
	// Shell tasks must NOT have --dangerously-skip-permissions or --model injected.
	for _, a := range cs.Args {
		if a == "--dangerously-skip-permissions" {
			t.Error("shell task should not have --dangerously-skip-permissions")
		}
		if a == "--model" {
			t.Error("shell task should not have --model injected")
		}
	}
}

func TestShellTask_EmptyCommand(t *testing.T) {
	cm := newTestConnMgr()
	registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	rs := makeShellRunStep("m1")
	rs.CommandSnapshot = "" // empty command → should fail

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), rs, nil, func(_ string, exitCode int) {
		ch <- exitCode
	})

	code := waitForCompletion(t, ch, 2*time.Second)
	if code != failureExitCode {
		t.Errorf("expected exit code %d for empty shell command, got %d", failureExitCode, code)
	}
	// Should NOT create a session in the store.
	if ms.hasSession() {
		t.Error("expected no session to be created for empty shell command")
	}
}

func TestShellTask_ResolvedArgs(t *testing.T) {
	cm := newTestConnMgr()
	sent := registerMockAgent(t, cm, "m1")
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	rs := makeShellRunStep("m1")
	// Command uses a template reference — but shell task commands must NOT be
	// resolved (security: prevents parameter-injection into the binary path).
	rs.CommandSnapshot = "/usr/bin/python3"
	rs.ArgsSnapshot = `["-c","${SCRIPT}"]`

	resolveCtx := &orchestrator.ResolveContext{
		RunParams: map[string]string{
			"SCRIPT": "print('hello')",
		},
	}

	ch := make(chan int, 1)
	exec.ExecuteStep(context.Background(), rs, resolveCtx, func(_ string, exitCode int) {
		ch <- exitCode
	})

	waitForSession(t, ms, 2*time.Second)
	ms.setAllSessionStatus(store.StatusCompleted)
	waitForCompletion(t, ch, 5*time.Second)

	if len(*sent) == 0 {
		t.Fatal("no commands sent to agent")
	}
	cs := (*sent)[0].GetCreateSession()
	if cs == nil {
		t.Fatal("expected CreateSession command")
	}
	// Command must be the static string, never resolved from templates.
	if cs.Command != "/usr/bin/python3" {
		t.Errorf("command = %q, want %q", cs.Command, "/usr/bin/python3")
	}
	wantArgs := []string{"-c", "print('hello')"}
	if len(cs.Args) != len(wantArgs) {
		t.Errorf("args = %v, want %v", cs.Args, wantArgs)
	} else {
		for i, a := range wantArgs {
			if cs.Args[i] != a {
				t.Errorf("args[%d] = %q, want %q", i, cs.Args[i], a)
			}
		}
	}
}

func TestRunStepIDForSession(t *testing.T) {
	cm := newTestConnMgr()
	ms := newMockStore()
	exec := NewSessionStepExecutor(cm, ms, nil)

	// Not found
	_, found := exec.RunStepIDForSession("nonexistent")
	if found {
		t.Error("expected not found for nonexistent session")
	}

	// Register a tracking entry manually
	exec.mu.Lock()
	exec.sessionToStep["sess-1"] = &stepTracking{
		runID:     "run-1",
		runStepID: "rs-1",
		stepID:    "step-1",
	}
	exec.mu.Unlock()

	id, found := exec.RunStepIDForSession("sess-1")
	if !found {
		t.Fatal("expected session to be found")
	}
	if id != "rs-1" {
		t.Errorf("runStepID = %q, want %q", id, "rs-1")
	}
}
