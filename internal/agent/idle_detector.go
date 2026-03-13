package agent

import (
	"bytes"
	"sync"
)

// idlePromptMarker is the UTF-8 encoding of "❯ " which Claude CLI outputs
// when it returns to its input prompt after completing a response.
// The raw bytes are: E2 9D AF 20 (❯ = U+276F, then a space).
var idlePromptMarker = []byte{0xE2, 0x9D, 0xAF, 0x20}

// IdleDetector watches PTY output for Claude CLI's idle prompt indicator (❯).
// It operates in two phases:
//  1. Waiting for the startup prompt — when detected, calls onReady (e.g., to
//     submit the initial prompt). This replaces a hardcoded sleep.
//  2. Waiting for the completion prompt — when detected, calls onIdle (e.g., to
//     send /exit). Fires only once.
type IdleDetector struct {
	mu        sync.Mutex
	phase     int    // 0 = waiting for startup prompt, 1 = waiting for completion
	triggered bool   // true after onIdle fired (prevents double-fire)
	onReady   func() // called when startup prompt detected (phase 0 → 1)
	onIdle    func() // called when completion prompt detected (phase 1)
	buf       []byte // rolling buffer for cross-chunk detection
}

// NewIdleDetector creates a detector that watches for Claude CLI's idle prompt.
// onReady is called when the CLI first shows its input prompt (ready for input).
// onIdle is called when the CLI returns to its input prompt after completing a
// response (ready to exit).
func NewIdleDetector(onReady, onIdle func()) *IdleDetector {
	return &IdleDetector{
		onReady: onReady,
		onIdle:  onIdle,
		buf:     make([]byte, 0, len(idlePromptMarker)),
	}
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
	maxKeep := len(idlePromptMarker) - 1 + len(data)
	if len(d.buf) > maxKeep {
		d.buf = d.buf[len(d.buf)-maxKeep:]
	}

	if !bytes.Contains(d.buf, idlePromptMarker) {
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

	// Phase 1: completion prompt detected — response is done.
	d.triggered = true
	d.mu.Unlock()
	d.onIdle()
}
