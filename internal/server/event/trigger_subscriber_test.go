package event

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
)

// --- Mock store ---

type mockTriggerStore struct {
	mu       sync.Mutex
	triggers []JobTrigger
	err      error
}

func (m *mockTriggerStore) ListEnabledTriggers(_ context.Context) ([]JobTrigger, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	out := make([]JobTrigger, len(m.triggers))
	copy(out, m.triggers)
	return out, nil
}

// --- Mock orchestrator ---

type mockOrchestrator struct {
	mu      sync.Mutex
	calls   []createRunCall
	err     error
}

type createRunCall struct {
	jobID       string
	triggerType string
}

func (m *mockOrchestrator) CreateRun(_ context.Context, jobID string, triggerType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, createRunCall{jobID: jobID, triggerType: triggerType})
	return m.err
}

func (m *mockOrchestrator) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// --- Helpers ---

func newTestSubscriber(store *mockTriggerStore, orch *mockOrchestrator) *TriggerSubscriber {
	return NewTriggerSubscriber(store, orch, slog.Default())
}

func makeTriggerEvent(eventType string, payload map[string]any) Event {
	if payload == nil {
		payload = map[string]any{}
	}
	return Event{
		EventID: "test-event-id",
		Type:    eventType,
		Payload: payload,
	}
}

// --- Tests ---

func TestTriggerSubscriber_NoTriggers(t *testing.T) {
	store := &mockTriggerStore{}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	err := handler(context.Background(), makeTriggerEvent("run.completed", nil))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if orch.callCount() != 0 {
		t.Errorf("expected 0 CreateRun calls, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_MatchingTrigger(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-1", EventType: "run.completed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	err := handler(context.Background(), makeTriggerEvent("run.completed", nil))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if orch.callCount() != 1 {
		t.Errorf("expected 1 CreateRun call, got %d", orch.callCount())
	}
	call := orch.calls[0]
	if call.jobID != "job-1" {
		t.Errorf("jobID = %q, want %q", call.jobID, "job-1")
	}
	if call.triggerType != "event_trigger" {
		t.Errorf("triggerType = %q, want %q", call.triggerType, "event_trigger")
	}
}

func TestTriggerSubscriber_GlobPattern(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-glob", EventType: "run.*", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()

	// "run.completed" should match "run.*"
	if err := handler(context.Background(), makeTriggerEvent("run.completed", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// "run.failed" should also match "run.*"
	if err := handler(context.Background(), makeTriggerEvent("run.failed", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// "session.started" should NOT match "run.*"
	if err := handler(context.Background(), makeTriggerEvent("session.started", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if orch.callCount() != 2 {
		t.Errorf("expected 2 CreateRun calls, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_WildcardPattern(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-wild", EventType: "*", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	if err := handler(context.Background(), makeTriggerEvent("session.started", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if orch.callCount() != 1 {
		t.Errorf("expected 1 CreateRun call, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_NoMatch(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-1", EventType: "run.completed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	// Different event type — should NOT match.
	if err := handler(context.Background(), makeTriggerEvent("session.started", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if orch.callCount() != 0 {
		t.Errorf("expected 0 CreateRun calls, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_LoopPrevention(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-loop", EventType: "run.completed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()

	// Event payload has job_id matching the trigger's job_id — should be skipped.
	payload := map[string]any{"job_id": "job-loop"}
	if err := handler(context.Background(), makeTriggerEvent("run.completed", payload)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if orch.callCount() != 0 {
		t.Errorf("expected 0 CreateRun calls (loop prevention), got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_LoopPrevention_DifferentJob(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-A", EventType: "run.completed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()

	// job_id is a DIFFERENT job — should NOT be blocked.
	payload := map[string]any{"job_id": "job-B"}
	if err := handler(context.Background(), makeTriggerEvent("run.completed", payload)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if orch.callCount() != 1 {
		t.Errorf("expected 1 CreateRun call, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_MultipleTriggers(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-1", EventType: "run.completed", Enabled: true},
			{TriggerID: "t2", JobID: "job-2", EventType: "run.completed", Enabled: true},
			{TriggerID: "t3", JobID: "job-3", EventType: "run.failed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	if err := handler(context.Background(), makeTriggerEvent("run.completed", nil)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	// t1 and t2 match; t3 does not.
	if orch.callCount() != 2 {
		t.Errorf("expected 2 CreateRun calls, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_StoreError(t *testing.T) {
	store := &mockTriggerStore{err: errors.New("db offline")}
	orch := &mockOrchestrator{}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	err := handler(context.Background(), makeTriggerEvent("run.completed", nil))
	if err == nil {
		t.Fatal("expected error from handler when store fails")
	}
	if orch.callCount() != 0 {
		t.Errorf("expected 0 CreateRun calls on store error, got %d", orch.callCount())
	}
}

func TestTriggerSubscriber_OrchestratorError_ContinuesOtherTriggers(t *testing.T) {
	store := &mockTriggerStore{
		triggers: []JobTrigger{
			{TriggerID: "t1", JobID: "job-1", EventType: "run.completed", Enabled: true},
			{TriggerID: "t2", JobID: "job-2", EventType: "run.completed", Enabled: true},
		},
	}
	orch := &mockOrchestrator{err: errors.New("orchestrator busy")}
	sub := newTestSubscriber(store, orch)

	handler := sub.Handler()
	// Handler should not return an error — orchestrator failures are logged as warnings.
	err := handler(context.Background(), makeTriggerEvent("run.completed", nil))
	if err != nil {
		t.Fatalf("handler should not return error on orchestrator failure, got: %v", err)
	}
	// Both triggers were attempted even though both failed.
	if orch.callCount() != 2 {
		t.Errorf("expected 2 CreateRun attempts, got %d", orch.callCount())
	}
}

func TestPayloadString(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		key     string
		want    string
	}{
		{"nil payload", nil, "k", ""},
		{"missing key", map[string]any{"a": "b"}, "missing", ""},
		{"present string", map[string]any{"k": "value"}, "k", "value"},
		{"non-string value", map[string]any{"k": 42}, "k", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := payloadString(tc.payload, tc.key)
			if got != tc.want {
				t.Errorf("payloadString(%v, %q) = %q, want %q", tc.payload, tc.key, got, tc.want)
			}
		})
	}
}
