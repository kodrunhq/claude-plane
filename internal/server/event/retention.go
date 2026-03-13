package event

import (
	"context"
	"log/slog"
	"time"
)

const (
	defaultRetentionPeriod = time.Hour
	defaultMaxAge          = 7 * 24 * time.Hour
)

// RetentionStore is the persistence interface required by RetentionCleaner.
type RetentionStore interface {
	PurgeEvents(ctx context.Context, before time.Time) (int64, error)
}

// RetentionCleaner periodically deletes events older than maxAge from the store.
// It runs in a background goroutine started by Start.
type RetentionCleaner struct {
	store  RetentionStore
	period time.Duration
	maxAge time.Duration
	logger *slog.Logger
}

// NewRetentionCleaner creates a RetentionCleaner with default period (1 hour)
// and maxAge (7 days). If logger is nil, slog.Default() is used.
func NewRetentionCleaner(store RetentionStore, logger *slog.Logger) *RetentionCleaner {
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionCleaner{
		store:  store,
		period: defaultRetentionPeriod,
		maxAge: defaultMaxAge,
		logger: logger,
	}
}

// Start launches the background cleanup goroutine. It returns immediately.
// The goroutine runs until ctx is cancelled, purging events every period.
func (r *RetentionCleaner) Start(ctx context.Context) {
	go r.run(ctx)
}

// run is the background loop that periodically purges old events.
func (r *RetentionCleaner) run(ctx context.Context) {
	ticker := time.NewTicker(r.period)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.purge(ctx)
		}
	}
}

// purge calls PurgeEvents and logs the result.
func (r *RetentionCleaner) purge(ctx context.Context) {
	before := time.Now().Add(-r.maxAge)
	n, err := r.store.PurgeEvents(ctx, before)
	if err != nil {
		r.logger.Warn("retention cleaner: purge failed", "error", err)
		return
	}
	if n > 0 {
		r.logger.Info("retention cleaner: purged old events",
			"count", n,
			"before", before.Format(time.RFC3339),
		)
	}
}
