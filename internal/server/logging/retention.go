package logging

import (
	"context"
	"log/slog"
	"time"
)

// RetentionCleaner periodically purges old log records.
type RetentionCleaner struct {
	store  *LogStore
	maxAge time.Duration
	logger *slog.Logger
}

// NewRetentionCleaner creates a RetentionCleaner that purges records older than maxAge.
func NewRetentionCleaner(store *LogStore, maxAge time.Duration, logger *slog.Logger) *RetentionCleaner {
	return &RetentionCleaner{store: store, maxAge: maxAge, logger: logger}
}

// Start begins the periodic purge loop. It runs every hour and deletes records
// older than maxAge. The loop exits when ctx is cancelled.
func (rc *RetentionCleaner) Start(ctx context.Context) {
	// Initial purge on startup
	go func() {
		cutoff := time.Now().UTC().Add(-rc.maxAge)
		if deleted, err := rc.store.PurgeBefore(cutoff); err != nil {
			rc.logger.Warn("initial log purge failed", "error", err)
		} else if deleted > 0 {
			rc.logger.Info("initial purge of old logs", "deleted", deleted)
		}
	}()
	// Periodic purge
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-rc.maxAge)
				deleted, err := rc.store.PurgeBefore(cutoff)
				if err != nil {
					rc.logger.Warn("log retention purge failed", "error", err)
				} else if deleted > 0 {
					rc.logger.Info("purged old logs", "deleted", deleted, "cutoff", cutoff)
				}
			}
		}
	}()
}
