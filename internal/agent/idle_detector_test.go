package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleDetector_OnReadyThenOnIdle(t *testing.T) {
	var readyCalled, idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { readyCalled.Store(true) },
		func() { idleCalled.Store(true) },
	)
	d.Start()

	// Phase 0: feed startup marker.
	d.Feed(DefaultIdlePromptMarker())
	if !readyCalled.Load() {
		t.Fatal("expected onReady to fire after startup marker")
	}
	if idleCalled.Load() {
		t.Fatal("onIdle should not fire after startup marker")
	}

	// Phase 1: feed completion marker.
	d.Feed(DefaultIdlePromptMarker())
	if !idleCalled.Load() {
		t.Fatal("expected onIdle to fire after completion marker")
	}
}

func TestIdleDetector_TriggeredOnce(t *testing.T) {
	var count atomic.Int32
	d := NewIdleDetector(
		func() {},
		func() { count.Add(1) },
	)
	d.Start()

	d.Feed(DefaultIdlePromptMarker()) // startup
	d.Feed(DefaultIdlePromptMarker()) // completion — fires
	d.Feed(DefaultIdlePromptMarker()) // should NOT fire again

	time.Sleep(10 * time.Millisecond)
	if got := count.Load(); got != 1 {
		t.Fatalf("expected onIdle to fire exactly once, got %d", got)
	}
}

func TestIdleDetector_KeepAlive_SignalsWithoutExit(t *testing.T) {
	var count atomic.Int32
	d := NewIdleDetector(
		func() {},
		func() { count.Add(1) },
		WithKeepAlive(true),
	)
	d.Start()

	d.Feed(DefaultIdlePromptMarker()) // startup
	d.Feed(DefaultIdlePromptMarker()) // idle #1
	d.Feed(DefaultIdlePromptMarker()) // idle #2
	d.Feed(DefaultIdlePromptMarker()) // idle #3

	time.Sleep(10 * time.Millisecond)
	if got := count.Load(); got != 3 {
		t.Fatalf("expected onIdle to fire 3 times in keep-alive mode, got %d", got)
	}
}

func TestIdleDetector_KeepAlive_PreservesStartupGuard(t *testing.T) {
	var readyCalled atomic.Bool
	var idleCount atomic.Int32

	d := NewIdleDetector(
		func() { readyCalled.Store(true) },
		func() { idleCount.Add(1) },
		WithKeepAlive(true),
		WithStartupTimeout(10*time.Millisecond),
	)
	d.Start()

	// Wait for timeout to fire onReady.
	time.Sleep(50 * time.Millisecond)
	if !readyCalled.Load() {
		t.Fatal("expected onReady from timeout")
	}

	// First real marker after timeout should be consumed by startup guard, not idle.
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if idleCount.Load() != 0 {
		t.Fatal("first marker after startup timeout should be consumed by guard, not idle")
	}

	// Second marker should fire idle.
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing, got %d", idleCount.Load())
	}
}

func TestIdleDetector_KeepAlive_BufferResetOnDetection(t *testing.T) {
	// Verify buffer is reset after detection so partial marker in buffer
	// doesn't cause spurious re-detection on next Feed.
	var idleCount atomic.Int32
	marker := DefaultIdlePromptMarker()

	d := NewIdleDetector(
		func() {},
		func() { idleCount.Add(1) },
		WithKeepAlive(true),
	)
	d.Start()

	d.Feed(marker) // startup

	// Feed marker followed by first byte of marker in same chunk.
	chunk := make([]byte, 0, len(marker)+1)
	chunk = append(chunk, marker...)
	chunk = append(chunk, marker[0])
	d.Feed(chunk)

	time.Sleep(10 * time.Millisecond)
	if idleCount.Load() != 1 {
		t.Fatalf("expected exactly 1 idle firing, got %d", idleCount.Load())
	}

	// Feed remaining bytes of marker — should NOT trigger again because
	// buffer was reset and we only have a partial marker.
	d.Feed(marker[1:])
	time.Sleep(10 * time.Millisecond)

	// This SHOULD trigger because the buffer has marker[0] + marker[1:] = full marker.
	// Actually after reset, buf was cleared. Then we fed marker+marker[0], detected marker,
	// reset buf. Then we fed marker[1:] — buf is marker[1:] which is NOT a full marker.
	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing after partial feed, got %d", idleCount.Load())
	}
}

func TestIdleDetector_ResetToPhase1(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() {},
		func() { idleCount.Add(1) },
		WithKeepAlive(true),
	)
	d.Start()

	// Phase 0 → 1: startup marker.
	d.Feed(DefaultIdlePromptMarker())
	// Phase 1: first idle fires.
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if got := idleCount.Load(); got != 1 {
		t.Fatalf("expected 1 idle firing before reset, got %d", got)
	}

	// Reset and verify next marker fires idle again.
	d.ResetToPhase1()
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if got := idleCount.Load(); got != 2 {
		t.Fatalf("expected 2 idle firings after reset, got %d", got)
	}
}

func TestIdleDetector_ResetToPhase1_ClearsTriggered(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() {},
		func() { idleCount.Add(1) },
		// Normal mode (not keep-alive): triggered flag prevents re-fire.
	)
	d.Start()

	// Phase 0 → 1: startup marker.
	d.Feed(DefaultIdlePromptMarker())
	// Phase 1: idle fires and sets triggered=true.
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if got := idleCount.Load(); got != 1 {
		t.Fatalf("expected 1 idle firing, got %d", got)
	}

	// Further markers should NOT fire (triggered=true).
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if got := idleCount.Load(); got != 1 {
		t.Fatalf("expected still 1 idle firing (triggered=true blocks), got %d", got)
	}

	// Reset clears triggered, so next marker should fire again.
	d.ResetToPhase1()
	d.Feed(DefaultIdlePromptMarker())
	time.Sleep(10 * time.Millisecond)
	if got := idleCount.Load(); got != 2 {
		t.Fatalf("expected 2 idle firings after reset, got %d", got)
	}
}

func TestIdleDetector_KeepAlive_ConcurrentFeeds(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() {},
		func() { idleCount.Add(1) },
		WithKeepAlive(true),
	)
	d.Start()

	d.Feed(DefaultIdlePromptMarker()) // startup

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Feed(DefaultIdlePromptMarker())
		}()
	}
	wg.Wait()

	time.Sleep(10 * time.Millisecond)
	got := idleCount.Load()
	if got < 1 {
		t.Fatalf("expected at least 1 idle firing from concurrent feeds, got %d", got)
	}
}
