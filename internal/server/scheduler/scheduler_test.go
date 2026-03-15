package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- mock implementations ---

type mockScheduleStore struct {
	mu         sync.Mutex
	schedules  []store.CronSchedule
	err        error
	timestamps []timestampUpdate
}

type timestampUpdate struct {
	scheduleID    string
	lastTriggered time.Time
	nextRun       time.Time
}

func (m *mockScheduleStore) ListEnabledSchedules(_ context.Context) ([]store.CronSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	var out []store.CronSchedule
	for _, sc := range m.schedules {
		if sc.Enabled {
			out = append(out, sc)
		}
	}
	return out, nil
}

func (m *mockScheduleStore) GetSchedule(_ context.Context, scheduleID string) (*store.CronSchedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	for i := range m.schedules {
		if m.schedules[i].ScheduleID == scheduleID {
			sc := m.schedules[i]
			return &sc, nil
		}
	}
	return nil, nil
}

func (m *mockScheduleStore) UpdateScheduleTimestamps(_ context.Context, scheduleID string, lastTriggered, nextRun time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timestamps = append(m.timestamps, timestampUpdate{
		scheduleID:    scheduleID,
		lastTriggered: lastTriggered,
		nextRun:       nextRun,
	})
	return nil
}

func (m *mockScheduleStore) getTimestamps() []timestampUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]timestampUpdate, len(m.timestamps))
	copy(out, m.timestamps)
	return out
}

type mockEventPublisher struct {
	mu     sync.Mutex
	events []event.Event
	err    error
}

func (m *mockEventPublisher) Publish(_ context.Context, ev event.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, ev)
	return nil
}

func (m *mockEventPublisher) getEvents() []event.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]event.Event, len(m.events))
	copy(out, m.events)
	return out
}

// waitForCondition polls fn every 50ms until it returns true or deadline is exceeded.
func waitForCondition(t *testing.T, deadline time.Duration, fn func() bool) bool {
	t.Helper()
	timeout := time.After(deadline)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return false
		case <-ticker.C:
			if fn() {
				return true
			}
		}
	}
}

// --- tests ---

// TestScheduler_StartAndFire verifies that a schedule firing every second
// publishes a trigger.cron event within 3 seconds.
func TestScheduler_StartAndFire(t *testing.T) {
	store := &mockScheduleStore{
		schedules: []store.CronSchedule{
			{
				ScheduleID: "sched-1",
				JobID:      "job-1",
				CronExpr:   "@every 1s",
				Timezone:   "UTC",
				Enabled:    true,
			},
		},
	}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer sched.Stop()

	fired := waitForCondition(t, 3*time.Second, func() bool {
		return len(publisher.getEvents()) > 0
	})
	if !fired {
		t.Fatal("expected at least one event within 3 seconds, got none")
	}

	events := publisher.getEvents()
	ev := events[0]
	if ev.Type != "trigger.cron" {
		t.Errorf("event type = %q, want %q", ev.Type, "trigger.cron")
	}
	if ev.Source != "scheduler" {
		t.Errorf("event source = %q, want %q", ev.Source, "scheduler")
	}
	if ev.Payload["schedule_id"] != "sched-1" {
		t.Errorf("payload schedule_id = %v, want %q", ev.Payload["schedule_id"], "sched-1")
	}
	if ev.Payload["job_id"] != "job-1" {
		t.Errorf("payload job_id = %v, want %q", ev.Payload["job_id"], "job-1")
	}
	if ev.EventID == "" {
		t.Error("event EventID should not be empty")
	}
}

// TestScheduler_ReloadSchedule verifies that a schedule added after Start fires
// correctly when ReloadSchedule is called.
func TestScheduler_ReloadSchedule(t *testing.T) {
	newSched := store.CronSchedule{
		ScheduleID: "sched-reload",
		JobID:      "job-reload",
		CronExpr:   "@every 1s",
		Timezone:   "UTC",
		Enabled:    true,
	}

	store := &mockScheduleStore{}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer sched.Stop()

	// Add schedule to store, then reload.
	store.mu.Lock()
	store.schedules = append(store.schedules, newSched)
	store.mu.Unlock()

	if err := sched.ReloadSchedule(ctx, "sched-reload"); err != nil {
		t.Fatalf("ReloadSchedule() error: %v", err)
	}

	fired := waitForCondition(t, 3*time.Second, func() bool {
		for _, ev := range publisher.getEvents() {
			if ev.Payload["schedule_id"] == "sched-reload" {
				return true
			}
		}
		return false
	})
	if !fired {
		t.Fatal("expected event for sched-reload within 3 seconds")
	}
}

// TestScheduler_RemoveSchedule verifies that a removed schedule stops firing.
func TestScheduler_RemoveSchedule(t *testing.T) {
	store := &mockScheduleStore{
		schedules: []store.CronSchedule{
			{
				ScheduleID: "sched-remove",
				JobID:      "job-remove",
				CronExpr:   "@every 1s",
				Timezone:   "UTC",
				Enabled:    true,
			},
		},
	}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer sched.Stop()

	// Wait until at least one event fires to confirm the schedule is active.
	if !waitForCondition(t, 3*time.Second, func() bool {
		return len(publisher.getEvents()) > 0
	}) {
		t.Fatal("schedule did not fire before removal")
	}

	sched.RemoveSchedule("sched-remove")

	// Snapshot event count after removal.
	countAfterRemoval := len(publisher.getEvents())

	// Wait 2 seconds; no new events should appear.
	time.Sleep(2 * time.Second)

	countFinal := len(publisher.getEvents())
	if countFinal > countAfterRemoval+1 { // allow 1 in-flight event
		t.Errorf("schedule kept firing after removal: events before=%d after=%d", countAfterRemoval, countFinal)
	}
}

// TestScheduler_DisabledSchedule verifies that a disabled schedule is not loaded
// on Start and never fires.
func TestScheduler_DisabledSchedule(t *testing.T) {
	store := &mockScheduleStore{
		schedules: []store.CronSchedule{
			{
				ScheduleID: "sched-disabled",
				JobID:      "job-disabled",
				CronExpr:   "@every 1s",
				Timezone:   "UTC",
				Enabled:    false, // disabled
			},
		},
	}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer sched.Stop()

	// Wait 2 seconds — disabled schedule should never fire.
	time.Sleep(2 * time.Second)

	if events := publisher.getEvents(); len(events) != 0 {
		t.Errorf("expected no events for disabled schedule, got %d", len(events))
	}
}

// TestScheduler_StoreError verifies that Start returns an error when the store
// fails to list enabled schedules.
func TestScheduler_StoreError(t *testing.T) {
	store := &mockScheduleStore{
		err: errStoreFailure,
	}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx := context.Background()
	err := sched.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Start when store fails, got nil")
	}
}

// errStoreFailure is a sentinel error used by TestScheduler_StoreError.
var errStoreFailure = &storeErr{msg: "store unavailable"}

type storeErr struct{ msg string }

func (e *storeErr) Error() string { return e.msg }

// TestScheduler_Stop verifies that Stop() completes without blocking forever.
func TestScheduler_Stop(t *testing.T) {
	store := &mockScheduleStore{}
	publisher := &mockEventPublisher{}

	sched := NewScheduler(store, publisher, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sched.Stop()
	}()

	select {
	case <-done:
		// Stop completed successfully.
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not complete within 5 seconds")
	}
}
