package auth

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// Blocklist maintains an in-memory set of revoked JWT token IDs backed by
// SQLite persistence via the token store. Thread-safe for concurrent access.
type Blocklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time // jti -> expiresAt
	store   *store.Store
}

// NewBlocklist creates a new Blocklist, loading all active (non-expired)
// revocations from the database into the in-memory map.
func NewBlocklist(tokenStore *store.Store) (*Blocklist, error) {
	bl := &Blocklist{
		entries: make(map[string]time.Time),
		store:   tokenStore,
	}

	revoked, err := tokenStore.LoadActiveRevocations()
	if err != nil {
		return nil, fmt.Errorf("load active revocations: %w", err)
	}

	for _, r := range revoked {
		bl.entries[r.JTI] = r.ExpiresAt
	}

	return bl, nil
}

// Add adds a token JTI to the blocklist with the given expiry time.
// It updates both the in-memory map and persists to the database.
func (bl *Blocklist) Add(jti, userID string, expiresAt time.Time) error {
	// Persist to DB first; only update in-memory on success.
	if err := bl.store.RevokeToken(jti, userID, expiresAt); err != nil {
		return fmt.Errorf("persist revocation: %w", err)
	}

	bl.mu.Lock()
	bl.entries[jti] = expiresAt
	bl.mu.Unlock()
	return nil
}

// IsRevoked returns true if the given JTI is in the blocklist.
func (bl *Blocklist) IsRevoked(jti string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.entries[jti]
	return ok
}

// StartCleanup runs a background goroutine that periodically removes expired
// entries from both the in-memory map and the database.
func (bl *Blocklist) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bl.cleanExpired()
			}
		}
	}()
}

// cleanExpired removes expired entries from the in-memory map and database.
func (bl *Blocklist) cleanExpired() {
	now := time.Now()

	bl.mu.Lock()
	for jti, exp := range bl.entries {
		if exp.Before(now) {
			delete(bl.entries, jti)
		}
	}
	bl.mu.Unlock()

	// Best-effort DB cleanup; errors are not critical
	if err := bl.store.CleanExpired(now); err != nil {
		slog.Warn("failed to clean expired blocklist entries from database", "error", err)
	}
}
