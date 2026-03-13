package event

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// nullLogger discards all log output so test output stays clean.
func nullLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(noopWriter{}, nil))
}

type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// makeEvent builds a minimal Event for testing.
func makeEvent(eventType string) Event {
	return Event{
		EventID:   "test-id",
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Source:    "test",
		Payload:   map[string]any{},
	}
}

// receiveWithTimeout reads from ch or times out after d.
func receiveWithTimeout(t *testing.T, ch <-chan Event, d time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(d):
		return Event{}, false
	}
}

// --- Tests ---

func TestPublishSubscribeBasic(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	received := make(chan Event, 1)
	unsub := b.Subscribe("run.created", func(_ context.Context, ev Event) error {
		received <- ev
		return nil
	}, SubscriberOptions{})
	defer unsub()

	ev := makeEvent(TypeRunCreated)
	if err := b.Publish(context.Background(), ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	got, ok := receiveWithTimeout(t, received, time.Second)
	if !ok {
		t.Fatal("timed out waiting for event")
	}
	if got.Type != TypeRunCreated {
		t.Errorf("got type %q, want %q", got.Type, TypeRunCreated)
	}
}

func TestPatternMatchWildcardPrefix(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	var count atomic.Int64
	unsub := b.Subscribe("run.*", func(_ context.Context, _ Event) error {
		count.Add(1)
		return nil
	}, SubscriberOptions{BufferSize: 16})
	defer unsub()

	types := []string{TypeRunCreated, TypeRunStarted, TypeRunCompleted, TypeRunFailed, TypeRunCancelled}
	for _, et := range types {
		_ = b.Publish(context.Background(), makeEvent(et))
	}
	// Publish a non-matching event — should not increment count.
	_ = b.Publish(context.Background(), makeEvent(TypeSessionStarted))

	// Drain by closing; Close waits for all handlers to finish.
	b.Close()

	if got := count.Load(); got != int64(len(types)) {
		t.Errorf("handler called %d times, want %d", got, len(types))
	}
}

func TestPatternMatchGlobalWildcard(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	var count atomic.Int64
	unsub := b.Subscribe("*", func(_ context.Context, _ Event) error {
		count.Add(1)
		return nil
	}, SubscriberOptions{BufferSize: 32})
	defer unsub()

	allTypes := []string{
		TypeRunCreated, TypeRunStarted, TypeRunCompleted, TypeRunFailed, TypeRunCancelled,
		TypeSessionStarted, TypeSessionExited,
		TypeMachineConnected, TypeMachineDisconnected,
		TypeTriggerCron, TypeTriggerWebhook, TypeTriggerJobCompleted,
	}
	for _, et := range allTypes {
		_ = b.Publish(context.Background(), makeEvent(et))
	}

	b.Close()

	if got := count.Load(); got != int64(len(allTypes)) {
		t.Errorf("handler called %d times, want %d", got, len(allTypes))
	}
}

func TestPatternMatchExact(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	var count atomic.Int64
	unsub := b.Subscribe(TypeMachineConnected, func(_ context.Context, _ Event) error {
		count.Add(1)
		return nil
	}, SubscriberOptions{BufferSize: 8})
	defer unsub()

	_ = b.Publish(context.Background(), makeEvent(TypeMachineConnected))
	_ = b.Publish(context.Background(), makeEvent(TypeMachineDisconnected)) // no match

	b.Close()

	if got := count.Load(); got != 1 {
		t.Errorf("handler called %d times, want 1", got)
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	received := make(chan Event, 8)
	unsub := b.Subscribe("*", func(_ context.Context, ev Event) error {
		received <- ev
		return nil
	}, SubscriberOptions{})

	// Publish once before unsubscribing.
	_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
	if _, ok := receiveWithTimeout(t, received, time.Second); !ok {
		t.Fatal("timed out before unsub")
	}

	unsub()

	// Publish again — should not reach handler.
	_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
	if _, ok := receiveWithTimeout(t, received, 100*time.Millisecond); ok {
		t.Fatal("received event after unsubscribe")
	}
}

func TestMultipleSubscribersReceiveSameEvent(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	const n = 5
	channels := make([]chan Event, n)
	unsubs := make([]func(), n)
	for i := range channels {
		ch := make(chan Event, 1)
		channels[i] = ch
		unsubs[i] = b.Subscribe("*", func(_ context.Context, ev Event) error {
			ch <- ev
			return nil
		}, SubscriberOptions{})
	}
	defer func() {
		for _, u := range unsubs {
			u()
		}
	}()

	ev := makeEvent(TypeRunCompleted)
	_ = b.Publish(context.Background(), ev)

	for i, ch := range channels {
		if _, ok := receiveWithTimeout(t, ch, time.Second); !ok {
			t.Errorf("subscriber %d did not receive event", i)
		}
	}
}

func TestConcurrentPublishSafety(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	var count atomic.Int64
	unsub := b.Subscribe("*", func(_ context.Context, _ Event) error {
		count.Add(1)
		return nil
	}, SubscriberOptions{BufferSize: 1024, Concurrency: 4})
	defer unsub()

	const goroutines = 20
	const eventsEach = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsEach; j++ {
				_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
			}
		}()
	}
	wg.Wait()

	b.Close()

	want := int64(goroutines * eventsEach)
	if got := count.Load(); got != want {
		t.Errorf("handled %d events, want %d", got, want)
	}
}

func TestBufferOverflowDropsNotBlocks(t *testing.T) {
	b := NewBus(nullLogger())
	defer b.Close()

	// Subscriber with buffer of 1 and a slow handler.
	slowStart := make(chan struct{})
	unsub := b.Subscribe("*", func(_ context.Context, _ Event) error {
		<-slowStart // block until test releases
		return nil
	}, SubscriberOptions{BufferSize: 1, Concurrency: 1})
	defer func() {
		close(slowStart)
		unsub()
	}()

	// Publish more events than the buffer can hold. Must not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
		}
		close(done)
	}()

	select {
	case <-done:
		// Good: all publishes completed without blocking.
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on full subscriber buffer")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	b := NewBus(nullLogger())
	b.Close()
	b.Close() // second call must not panic
}

func TestPublishAfterCloseIsNoop(t *testing.T) {
	b := NewBus(nullLogger())
	b.Close()
	if err := b.Publish(context.Background(), makeEvent(TypeRunCreated)); err != nil {
		t.Errorf("unexpected error after close: %v", err)
	}
}

func TestHandlerErrorLogged(t *testing.T) {
	// We can't inspect slog output easily, but we verify the bus continues
	// delivering subsequent events after a handler returns an error.
	b := NewBus(nullLogger())
	defer b.Close()

	var count atomic.Int64
	unsub := b.Subscribe("*", func(_ context.Context, _ Event) error {
		n := count.Add(1)
		if n == 1 {
			return errors.New("intentional error")
		}
		return nil
	}, SubscriberOptions{})
	defer unsub()

	_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
	_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))

	b.Close()

	if got := count.Load(); got != 2 {
		t.Errorf("handler called %d times, want 2", got)
	}
}

func TestCloseDrainsPendingEvents(t *testing.T) {
	b := NewBus(nullLogger())

	var count atomic.Int64
	_ = b.Subscribe("*", func(_ context.Context, _ Event) error {
		// Small sleep to ensure events are in-flight when Close is called.
		time.Sleep(5 * time.Millisecond)
		count.Add(1)
		return nil
	}, SubscriberOptions{BufferSize: 64})

	const total = 10
	for i := 0; i < total; i++ {
		_ = b.Publish(context.Background(), makeEvent(TypeRunCreated))
	}

	b.Close() // must wait for all pending events to be processed

	if got := count.Load(); got != total {
		t.Errorf("processed %d events, want %d", got, total)
	}
}

// --- Builder function tests ---

func TestNewRunEvent(t *testing.T) {
	ev := NewRunEvent(TypeRunCreated, "run-1", "job-2", "running", "manual")

	if ev.EventID == "" {
		t.Error("EventID must be populated")
	}
	if ev.Type != TypeRunCreated {
		t.Errorf("Type = %q, want %q", ev.Type, TypeRunCreated)
	}
	if ev.Source != "orchestrator" {
		t.Errorf("Source = %q, want %q", ev.Source, "orchestrator")
	}

	wantKeys := []string{"run_id", "job_id", "status", "trigger_type"}
	for _, k := range wantKeys {
		if _, ok := ev.Payload[k]; !ok {
			t.Errorf("Payload missing key %q", k)
		}
	}
	if ev.Payload["run_id"] != "run-1" {
		t.Errorf("run_id = %v, want run-1", ev.Payload["run_id"])
	}
	if ev.Payload["job_id"] != "job-2" {
		t.Errorf("job_id = %v, want job-2", ev.Payload["job_id"])
	}
	if ev.Payload["status"] != "running" {
		t.Errorf("status = %v, want running", ev.Payload["status"])
	}
	if ev.Payload["trigger_type"] != "manual" {
		t.Errorf("trigger_type = %v, want manual", ev.Payload["trigger_type"])
	}
}

func TestNewSessionEvent(t *testing.T) {
	ev := NewSessionEvent(TypeSessionStarted, "sess-1", "machine-1")

	if ev.EventID == "" {
		t.Error("EventID must be populated")
	}
	if ev.Type != TypeSessionStarted {
		t.Errorf("Type = %q, want %q", ev.Type, TypeSessionStarted)
	}
	if ev.Source != "session" {
		t.Errorf("Source = %q, want %q", ev.Source, "session")
	}

	wantKeys := []string{"session_id", "machine_id"}
	for _, k := range wantKeys {
		if _, ok := ev.Payload[k]; !ok {
			t.Errorf("Payload missing key %q", k)
		}
	}
	if ev.Payload["session_id"] != "sess-1" {
		t.Errorf("session_id = %v, want sess-1", ev.Payload["session_id"])
	}
	if ev.Payload["machine_id"] != "machine-1" {
		t.Errorf("machine_id = %v, want machine-1", ev.Payload["machine_id"])
	}
}

func TestNewMachineEvent(t *testing.T) {
	ev := NewMachineEvent(TypeMachineConnected, "machine-42")

	if ev.EventID == "" {
		t.Error("EventID must be populated")
	}
	if ev.Type != TypeMachineConnected {
		t.Errorf("Type = %q, want %q", ev.Type, TypeMachineConnected)
	}
	if ev.Source != "connmgr" {
		t.Errorf("Source = %q, want %q", ev.Source, "connmgr")
	}
	if _, ok := ev.Payload["machine_id"]; !ok {
		t.Error("Payload missing key machine_id")
	}
	if ev.Payload["machine_id"] != "machine-42" {
		t.Errorf("machine_id = %v, want machine-42", ev.Payload["machine_id"])
	}
}

func TestNewTriggerEvent(t *testing.T) {
	payload := map[string]any{"repo": "acme/app", "ref": "main"}
	ev := NewTriggerEvent(TypeTriggerWebhook, "webhook:github", payload)

	if ev.EventID == "" {
		t.Error("EventID must be populated")
	}
	if ev.Type != TypeTriggerWebhook {
		t.Errorf("Type = %q, want %q", ev.Type, TypeTriggerWebhook)
	}
	if ev.Source != "webhook:github" {
		t.Errorf("Source = %q, want %q", ev.Source, "webhook:github")
	}
	if ev.Payload["repo"] != "acme/app" {
		t.Errorf("repo = %v, want acme/app", ev.Payload["repo"])
	}
	if ev.Payload["ref"] != "main" {
		t.Errorf("ref = %v, want main", ev.Payload["ref"])
	}
}

func TestNewTriggerEventNilPayload(t *testing.T) {
	ev := NewTriggerEvent(TypeTriggerCron, "cron", nil)

	if ev.EventID == "" {
		t.Error("EventID must be populated")
	}
	if ev.Payload == nil {
		t.Error("Payload must not be nil when nil is passed")
	}
}

func TestBuilderEventIDsAreUnique(t *testing.T) {
	ev1 := NewRunEvent(TypeRunCreated, "r1", "j1", "running", "manual")
	ev2 := NewRunEvent(TypeRunCreated, "r1", "j1", "running", "manual")
	if ev1.EventID == ev2.EventID {
		t.Error("successive builder calls must produce distinct EventIDs")
	}
}

// --- MatchPattern unit tests ---

func TestMatchPattern(t *testing.T) {
	cases := []struct {
		pattern   string
		eventType string
		want      bool
	}{
		{"*", TypeRunCreated, true},
		{"*", TypeSessionStarted, true},
		{"run.*", TypeRunCreated, true},
		{"run.*", TypeRunCompleted, true},
		{"run.*", TypeSessionStarted, false},
		{"session.*", TypeSessionStarted, true},
		{"session.*", TypeSessionExited, true},
		{"session.*", TypeRunCreated, false},
		{TypeRunCreated, TypeRunCreated, true},
		{TypeRunCreated, TypeRunCompleted, false},
		{"machine.*", TypeMachineConnected, true},
		{"machine.*", TypeMachineDisconnected, true},
		{"trigger.*", TypeTriggerCron, true},
		{"trigger.*", TypeRunCreated, false},
	}
	for _, tc := range cases {
		got := MatchPattern(tc.pattern, tc.eventType)
		if got != tc.want {
			t.Errorf("MatchPattern(%q, %q) = %v, want %v", tc.pattern, tc.eventType, got, tc.want)
		}
	}
}
