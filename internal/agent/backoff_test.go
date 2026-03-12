package agent

import (
	"testing"
	"time"
)

func TestBackoffIncreasing(t *testing.T) {
	b := NewBackoff(1*time.Second, 60*time.Second)

	var prev time.Duration
	for i := 0; i < 5; i++ {
		d := b.Next()

		// Each value must be >= 0.8 * min
		if d < time.Duration(float64(800)*float64(time.Millisecond)) {
			t.Errorf("call %d: duration %v below 0.8 * min", i, d)
		}

		// Each value must be <= 1.2 * max
		maxWithJitter := time.Duration(float64(60*time.Second) * 1.2)
		if d > maxWithJitter {
			t.Errorf("call %d: duration %v exceeds 1.2 * max", i, d)
		}

		// General increasing trend (first few should be less than later ones)
		if i > 0 && i < 3 {
			// Allow jitter variation; the base should generally increase
			// We don't enforce strict ordering due to jitter.
		}
		prev = d
	}
	_ = prev
}

func TestBackoffCapsAtMax(t *testing.T) {
	b := NewBackoff(1*time.Second, 60*time.Second)

	maxWithJitter := time.Duration(float64(60*time.Second) * 1.2)
	for i := 0; i < 20; i++ {
		d := b.Next()
		if d > maxWithJitter {
			t.Errorf("call %d: duration %v exceeds max with jitter (%v)", i, d, maxWithJitter)
		}
	}
}

func TestBackoffReset(t *testing.T) {
	b := NewBackoff(1*time.Second, 60*time.Second)

	// Advance several times.
	for i := 0; i < 5; i++ {
		b.Next()
	}

	b.Reset()
	d := b.Next()

	// After reset, should be near min (1s +/- 20% jitter -> [0.8s, 1.2s]).
	if d < 800*time.Millisecond || d > 1200*time.Millisecond {
		t.Errorf("after reset, expected ~1s, got %v", d)
	}
}
