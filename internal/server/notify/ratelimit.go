package notify

import (
	"sync"
	"time"
)

// RateLimiter enforces max 1 notification per channel per event type per interval.
type RateLimiter struct {
	mu       sync.Mutex
	sent     map[string]time.Time
	interval time.Duration
	nowFn    func() time.Time // overridable for tests
}

// NewRateLimiter creates a rate limiter with the given interval.
func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{
		sent:     make(map[string]time.Time),
		interval: interval,
		nowFn:    time.Now,
	}
}

// Allow returns true if the notification should proceed.
// It also evicts expired entries from the map when it grows beyond 100 entries
// to prevent unbounded memory growth.
func (rl *RateLimiter) Allow(channelID, eventType string) bool {
	key := channelID + ":" + eventType
	now := rl.nowFn()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Periodic cleanup: evict expired entries every time the map exceeds 100 entries.
	if len(rl.sent) > 100 {
		for k, t := range rl.sent {
			if now.Sub(t) >= rl.interval {
				delete(rl.sent, k)
			}
		}
	}

	if last, ok := rl.sent[key]; ok && now.Sub(last) < rl.interval {
		return false
	}
	rl.sent[key] = now
	return true
}
