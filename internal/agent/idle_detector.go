package agent

import (
	"bytes"
	"log/slog"
	"sync"
	"time"
)

// defaultIdlePromptMarker is the UTF-8 encoding of "❯ " which Claude CLI
// outputs when it returns to its input prompt after completing a response.
// The raw bytes are: E2 9D AF 20 (❯ = U+276F, then a space).
var defaultIdlePromptMarker = []byte{0xE2, 0x9D, 0xAF, 0x20}

// DefaultIdlePromptMarker returns a copy of the default prompt marker bytes.
func DefaultIdlePromptMarker() []byte {
	out := make([]byte, len(defaultIdlePromptMarker))
	copy(out, defaultIdlePromptMarker)
	return out
}

// DefaultStartupTimeout is how long to wait for the startup prompt before
// assuming the CLI is ready. This prevents indefinite hangs if the CLI prompt
// character changes in a future release.
const DefaultStartupTimeout = 10 * time.Second

// IdleDetector watches PTY output for Claude CLI's idle prompt indicator (❯).
// It operates in two phases:
//  1. Waiting for the startup prompt — when detected, calls onReady (e.g., to
//     submit the initial prompt). This replaces a hardcoded sleep.
//  2. Waiting for the completion prompt — when detected, calls onIdle (e.g., to
//     send /exit). Fires only once in normal mode.
//
// In keep-alive mode (WithKeepAlive), onIdle fires on every completion marker
// without setting the triggered flag, allowing the session to remain alive for
// shared session support where multiple prompts are submitted sequentially.
type IdleDetector struct {
	mu              sync.Mutex
	phase           int    // 0 = waiting for startup prompt, 1 = waiting for completion
	triggered       bool   // true after onIdle fired (prevents double-fire in normal mode)
	startupTimedOut bool   // true if startup timeout fired before marker was seen
	keepAlive       bool   // when true, onIdle fires repeatedly without setting triggered
	onReady         func() // called when startup prompt detected (phase 0 → 1)
	onIdle          func() // called when completion prompt detected (phase 1)
	buf             []byte // rolling buffer for cross-chunk detection
	marker          []byte // prompt marker bytes to detect
	startupTimeout  time.Duration
}

// IdleDetectorOption configures optional IdleDetector settings.
type IdleDetectorOption func(*IdleDetector)

// WithPromptMarker overrides the default prompt marker bytes.
func WithPromptMarker(marker []byte) IdleDetectorOption {
	return func(d *IdleDetector) {
		if len(marker) > 0 {
			d.marker = marker
		}
	}
}

// WithKeepAlive enables keep-alive mode. When true, onIdle fires on every
// completion marker without preventing further detections. This is used for
// shared sessions where multiple prompts are submitted to the same CLI instance.
func WithKeepAlive(enabled bool) IdleDetectorOption {
	return func(d *IdleDetector) {
		d.keepAlive = enabled
	}
}

// WithStartupTimeout overrides the default startup timeout.
func WithStartupTimeout(timeout time.Duration) IdleDetectorOption {
	return func(d *IdleDetector) {
		if timeout > 0 {
			d.startupTimeout = timeout
		}
	}
}

// NewIdleDetector creates a detector that watches for Claude CLI's idle prompt.
// onReady is called when the CLI first shows its input prompt (ready for input).
// onIdle is called when the CLI returns to its input prompt after completing a
// response (ready to exit).
func NewIdleDetector(onReady, onIdle func(), opts ...IdleDetectorOption) *IdleDetector {
	d := &IdleDetector{
		onReady:        onReady,
		onIdle:         onIdle,
		marker:         DefaultIdlePromptMarker(),
		startupTimeout: DefaultStartupTimeout,
	}
	for _, opt := range opts {
		opt(d)
	}
	d.buf = make([]byte, 0, len(d.marker))
	return d
}

// Start begins the startup timeout timer. If the startup prompt is not detected
// within the timeout, onReady fires anyway to prevent indefinite hangs.
// If the CLI is simply slow and the real marker appears later, the first marker
// after a timeout is treated as the startup prompt (not as completion).
func (d *IdleDetector) Start() {
	time.AfterFunc(d.startupTimeout, func() {
		d.mu.Lock()
		if d.phase == 0 {
			d.startupTimedOut = true
			d.phase = 1
			d.mu.Unlock()
			slog.Warn("idle detector: startup prompt not detected within timeout, proceeding",
				"timeout", d.startupTimeout)
			d.onReady()
			return
		}
		d.mu.Unlock()
	})
}

// Feed processes a chunk of PTY output, looking for the idle prompt marker.
func (d *IdleDetector) Feed(data []byte) {
	d.mu.Lock()
	if d.triggered {
		d.mu.Unlock()
		return
	}

	// Append data to rolling buffer for cross-chunk boundary detection.
	d.buf = append(d.buf, data...)

	// Keep only the last (markerLen - 1 + dataLen) bytes to handle splits.
	maxKeep := len(d.marker) - 1 + len(data)
	if len(d.buf) > maxKeep {
		d.buf = d.buf[len(d.buf)-maxKeep:]
	}

	if !bytes.Contains(d.buf, d.marker) {
		d.mu.Unlock()
		return
	}

	// Reset buffer after each detection so we start fresh.
	d.buf = d.buf[:0]

	if d.phase == 0 {
		// Phase 0: startup prompt detected — CLI is ready for input.
		d.phase = 1
		d.mu.Unlock()
		d.onReady()
		return
	}

	// If the startup timeout fired but this is the first real marker we've
	// seen, treat it as the (late) startup prompt rather than completion.
	// This prevents premature /exit when the CLI is simply slow to start.
	if d.startupTimedOut {
		d.startupTimedOut = false
		d.mu.Unlock()
		return
	}

	// Phase 1: completion prompt detected — response is done.
	if d.keepAlive {
		// Keep-alive mode: signal idle but allow further detections.
		// Buffer is already reset above (line d.buf = d.buf[:0]) so stale
		// bytes won't cause spurious re-detection.
		d.mu.Unlock()
		d.onIdle()
		return
	}

	d.triggered = true
	d.mu.Unlock()
	d.onIdle()
}
