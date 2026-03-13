package event

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// --- stub RetentionStore for RetentionCleaner tests ---

type stubRetentionStore struct {
	calls atomic.Int64
	err   error
	// last records the most recent 'before' argument passed to PurgeEvents.
	last atomic.Value
}

func (s *stubRetentionStore) PurgeEvents(_ context.Context, before time.Time) (int64, error) {
	s.calls.Add(1)
	s.last.Store(before)
	return 0, s.err
}

func TestRetentionCleaner_CallsPurgeEvents(t *testing.T) {
	store := &stubRetentionStore{}
	rc := NewRetentionCleaner(store, nullLogger())

	// Override period to something very short so the test doesn't take long.
	rc.period = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	rc.Start(ctx)

	// Wait for at least one purge call.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()

	if store.calls.Load() < 1 {
		t.Error("PurgeEvents was never called")
	}
}

func TestRetentionCleaner_StopsOnContextCancel(t *testing.T) {
	store := &stubRetentionStore{}
	rc := NewRetentionCleaner(store, nullLogger())
	rc.period = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	rc.Start(ctx)

	// Let it fire a couple of times.
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Snapshot the count shortly after cancellation.
	time.Sleep(30 * time.Millisecond)
	countAfterCancel := store.calls.Load()

	// Wait longer and confirm the count is not still growing.
	time.Sleep(50 * time.Millisecond)
	countLater := store.calls.Load()

	if countLater > countAfterCancel {
		t.Errorf("PurgeEvents continued to be called after context cancellation: %d → %d", countAfterCancel, countLater)
	}
}

func TestRetentionCleaner_NilLogger(t *testing.T) {
	store := &stubRetentionStore{}
	// nil logger must not panic.
	rc := NewRetentionCleaner(store, nil)
	rc.period = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rc.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if store.calls.Load() < 1 {
		t.Error("PurgeEvents was never called")
	}
}

func TestRetentionCleaner_PurgesBeforeMaxAge(t *testing.T) {
	store := &stubRetentionStore{}
	rc := NewRetentionCleaner(store, nullLogger())
	rc.period = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	rc.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.calls.Load() >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()

	if store.calls.Load() == 0 {
		t.Fatal("PurgeEvents was never called")
	}

	before, ok := store.last.Load().(time.Time)
	if !ok || before.IsZero() {
		t.Fatal("last 'before' timestamp not recorded")
	}

	// 'before' should be approximately now minus maxAge (7 days).
	expectedBefore := time.Now().Add(-rc.maxAge)
	diff := expectedBefore.Sub(before)
	if diff < 0 {
		diff = -diff
	}
	// Allow 5 seconds of slack for slow test environments.
	if diff > 5*time.Second {
		t.Errorf("PurgeEvents called with before=%v, expected ~%v (diff %v)", before, expectedBefore, diff)
	}
}
