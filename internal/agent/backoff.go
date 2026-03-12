// Package agent provides the agent-side gRPC client and reconnection logic
// for the claude-plane agent binary.
package agent

import (
	"math/rand/v2"
	"time"
)

// Backoff implements exponential backoff with jitter for reconnection.
type Backoff struct {
	current time.Duration
	min     time.Duration
	max     time.Duration
}

// NewBackoff creates a Backoff with the given min and max durations.
// The first call to Next() returns a value near min.
func NewBackoff(min, max time.Duration) *Backoff {
	return &Backoff{
		current: min,
		min:     min,
		max:     max,
	}
}

// Next returns the next backoff duration and doubles the base for next time.
// A 20% jitter is applied: the returned value is in [0.8*base, 1.2*base].
// The base is capped at max before jitter is applied.
func (b *Backoff) Next() time.Duration {
	base := b.current

	// Double for next call, capped at max.
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}

	// Apply 20% jitter: multiply by a factor in [0.8, 1.2).
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(base) * jitter)
}

// Reset returns the backoff to its minimum value.
func (b *Backoff) Reset() {
	b.current = b.min
}
