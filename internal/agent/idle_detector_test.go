package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleDetector_SilenceFiresOnIdle(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(100 * time.Millisecond)

	if !idleCalled.Load() {
		t.Fatal("expected onIdle to fire after silence timeout")
	}
}

func TestIdleDetector_ActivityResetsTimer(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(80*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	for i := 0; i < 5; i++ {
		d.Feed(make([]byte, 20))
		time.Sleep(30 * time.Millisecond)
	}

	if idleCalled.Load() {
		t.Fatal("onIdle should not fire while activity continues")
	}

	time.Sleep(120 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle after activity stopped")
	}
}

func TestIdleDetector_MinActivityBytesFilter(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
		WithMinActivityBytes(15),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)

	d.Feed(make([]byte, 5))
	time.Sleep(80 * time.Millisecond)

	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing (small data ignored), got %d", idleCount.Load())
	}
}

func TestIdleDetector_OnActiveFiresOnTransition(t *testing.T) {
	var idleCalled, activeCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		func() { activeCalled.Store(true) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(80 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle")
	}

	d.Feed(make([]byte, 20))
	time.Sleep(30 * time.Millisecond)
	if !activeCalled.Load() {
		t.Fatal("expected onActive on idle→active transition")
	}
}

func TestIdleDetector_OnActiveDoesNotFireInitially(t *testing.T) {
	var activeCalled atomic.Bool
	d := NewIdleDetector(
		func() {},
		func() { activeCalled.Store(true) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)
	if activeCalled.Load() {
		t.Fatal("onActive should not fire on initial feed, only on idle→active transition")
	}
}

func TestIdleDetector_StartupTimeoutFallback(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithStartupTimeout(50*time.Millisecond),
		WithSilenceTimeout(10*time.Second),
	)
	d.Start()
	defer d.Stop()

	time.Sleep(100 * time.Millisecond)
	if !idleCalled.Load() {
		t.Fatal("expected onIdle from startup timeout fallback")
	}
}

func TestIdleDetector_StartupTimeoutCancelledByOutput(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithStartupTimeout(100*time.Millisecond),
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(150 * time.Millisecond)

	if idleCount.Load() != 1 {
		t.Fatalf("expected 1 idle firing, got %d", idleCount.Load())
	}
}

func TestIdleDetector_StopCancelsTimers(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()

	d.Feed(make([]byte, 20))
	d.Stop()
	time.Sleep(100 * time.Millisecond)

	if idleCalled.Load() {
		t.Fatal("onIdle should not fire after Stop()")
	}
}

func TestIdleDetector_StopIsIdempotent(t *testing.T) {
	d := NewIdleDetector(func() {}, nil, WithSilenceTimeout(50*time.Millisecond))
	d.Start()
	d.Stop()
	d.Stop()
}

func TestIdleDetector_RepeatedIdleActiveCycles(t *testing.T) {
	var idleCount, activeCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		func() { activeCount.Add(1) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	for i := 0; i < 3; i++ {
		d.Feed(make([]byte, 20))
		time.Sleep(80 * time.Millisecond) // let idle fire
		d.Feed(make([]byte, 20))
		time.Sleep(30 * time.Millisecond) // let active callback complete
	}

	if idleCount.Load() != 3 {
		t.Fatalf("expected 3 idle firings, got %d", idleCount.Load())
	}
	if activeCount.Load() != 3 {
		t.Fatalf("expected 3 active firings, got %d", activeCount.Load())
	}
}

func TestIdleDetector_NilOnActive(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	d.Feed(make([]byte, 20))
	time.Sleep(80 * time.Millisecond)
	d.Feed(make([]byte, 20))
}

func TestIsRepositioningNoise_CursorSave(t *testing.T) {
	// Ink status bar redraw: save cursor + absolute move + text + restore cursor
	data := []byte("\x1b7\x1b[14;1H\x1b[2KTip: try /help\x1b8")
	if !isRepositioningNoise(data) {
		t.Fatal("expected cursor save/restore to be classified as noise")
	}
}

func TestIsRepositioningNoise_AbsolutePosition(t *testing.T) {
	// CSI cursor position command without save/restore
	data := []byte("\x1b[14;1HSome status text")
	if !isRepositioningNoise(data) {
		t.Fatal("expected absolute cursor position to be classified as noise")
	}
}

func TestIsRepositioningNoise_CursorUp(t *testing.T) {
	data := []byte("\x1b[3AOverwrite previous line")
	if !isRepositioningNoise(data) {
		t.Fatal("expected cursor up to be classified as noise")
	}
}

func TestIsRepositioningNoise_SGROnly(t *testing.T) {
	// Real Claude output: SGR color codes + text (no cursor movement)
	data := []byte("\x1b[32mdef foo():\x1b[0m\n  return 42\n")
	if isRepositioningNoise(data) {
		t.Fatal("SGR-only output should NOT be classified as noise")
	}
}

func TestIsRepositioningNoise_PlainText(t *testing.T) {
	data := []byte("Here is some response text from Claude.\n")
	if isRepositioningNoise(data) {
		t.Fatal("plain text should NOT be classified as noise")
	}
}

func TestIsRepositioningNoise_EraseLineOnly(t *testing.T) {
	// Erase line (K) is a repositioning signal — Ink uses it to clear status lines
	data := []byte("\x1b[2KNew status content")
	if !isRepositioningNoise(data) {
		t.Fatal("erase line should be classified as noise")
	}
}

func TestIdleDetector_IgnoresRepositioningNoise(t *testing.T) {
	var idleCalled atomic.Bool
	d := NewIdleDetector(
		func() { idleCalled.Store(true) },
		nil,
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed real output first to start the silence timer.
	d.Feed(make([]byte, 20))
	time.Sleep(10 * time.Millisecond)

	// Feed Ink status bar noise — should NOT reset the silence timer.
	inkNoise := []byte("\x1b7\x1b[14;1H\x1b[2K\x1b[38;5;243mTip: try /help\x1b8")
	d.Feed(inkNoise)
	time.Sleep(80 * time.Millisecond)

	// Silence timer should have fired because noise was ignored.
	if !idleCalled.Load() {
		t.Fatal("expected onIdle — Ink noise should not reset the silence timer")
	}
}

func TestIdleDetector_NoiseDoesNotTransitionToActive(t *testing.T) {
	var activeCalled atomic.Bool
	d := NewIdleDetector(
		func() {},
		func() { activeCalled.Store(true) },
		WithSilenceTimeout(50*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	// Feed real output, wait for idle.
	d.Feed(make([]byte, 20))
	time.Sleep(80 * time.Millisecond)

	// Feed noise while idle — should NOT trigger onActive.
	inkNoise := []byte("\x1b7\x1b[14;1H\x1b[2KUpdate available\x1b8")
	d.Feed(inkNoise)
	time.Sleep(30 * time.Millisecond)

	if activeCalled.Load() {
		t.Fatal("Ink noise should not trigger idle→active transition")
	}
}

func TestIdleDetector_ConcurrentFeeds(t *testing.T) {
	var idleCount atomic.Int32
	d := NewIdleDetector(
		func() { idleCount.Add(1) },
		nil,
		WithSilenceTimeout(100*time.Millisecond),
	)
	d.Start()
	defer d.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Feed(make([]byte, 20))
		}()
	}
	wg.Wait()
}
