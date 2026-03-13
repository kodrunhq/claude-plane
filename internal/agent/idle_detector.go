package agent

import (
	"bytes"
	"sync"
)

// idlePromptMarker is the UTF-8 encoding of "❯ " which Claude CLI outputs
// when it returns to its input prompt after completing a response.
// The raw bytes are: E2 9D AF 20 (❯ = U+276F, then a space).
var idlePromptMarker = []byte{0xE2, 0x9D, 0xAF, 0x20}

// IdleDetector watches PTY output for Claude CLI's idle prompt indicator.
// After the initial prompt is submitted, it waits for the CLI to return to
// its input prompt (❯), which signals that the response is complete.
type IdleDetector struct {
	mu        sync.Mutex
	armed     bool     // true after the initial prompt has been submitted
	triggered bool     // true after idle prompt detected (fires only once)
	seenFirst bool     // true after first idle prompt (the startup one) is skipped
	onIdle    func()   // callback when idle detected
	buf       []byte   // rolling buffer for cross-chunk detection
}

// NewIdleDetector creates a detector that calls onIdle when Claude CLI
// returns to its input prompt after completing a response.
func NewIdleDetector(onIdle func()) *IdleDetector {
	return &IdleDetector{
		onIdle: onIdle,
		buf:    make([]byte, 0, len(idlePromptMarker)),
	}
}

// Arm enables detection. Call this after the initial prompt has been submitted
// to the PTY. Before arming, the detector ignores all output.
func (d *IdleDetector) Arm() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.armed = true
}

// Feed processes a chunk of PTY output, looking for the idle prompt marker.
func (d *IdleDetector) Feed(data []byte) {
	d.mu.Lock()
	if !d.armed || d.triggered {
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

	if bytes.Contains(d.buf, idlePromptMarker) {
		if !d.seenFirst {
			// Skip the first idle prompt — it's the startup prompt before
			// the initial input is submitted. The second one signals completion.
			d.seenFirst = true
			// Reset buffer so we start fresh for next detection.
			d.buf = d.buf[:0]
			d.mu.Unlock()
			return
		}
		d.triggered = true
		d.mu.Unlock()
		d.onIdle()
		return
	}

	d.mu.Unlock()
}
