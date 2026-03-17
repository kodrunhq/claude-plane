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
func (rl *RateLimiter) Allow(channelID, eventType string) bool {
	key := channelID + ":" + eventType
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.nowFn()
	if last, ok := rl.sent[key]; ok && now.Sub(last) < rl.interval {
		return false
	}
	rl.sent[key] = now
	return true
}
