package event

import (
	"context"
	"log/slog"
)

// EventStore is the persistence interface required by PersistSubscriber.
// The store package's Store type implements this interface.
type EventStore interface {
	InsertEvent(ctx context.Context, e Event) error
}

// PersistSubscriber writes every event to SQLite via EventStore.
// It is intended to be registered on the Bus with pattern "*" so that
// all events are durably persisted for the audit trail and query API.
type PersistSubscriber struct {
	store  EventStore
	logger *slog.Logger
}

// NewPersistSubscriber creates a PersistSubscriber backed by store.
// If logger is nil, slog.Default() is used.
func NewPersistSubscriber(store EventStore, logger *slog.Logger) *PersistSubscriber {
	if logger == nil {
		logger = slog.Default()
	}
	return &PersistSubscriber{store: store, logger: logger}
}

// Handler returns the HandlerFunc to pass to Bus.Subscribe.
// The returned handler inserts each event into the store.
// Errors are logged at Warn level; the bus will also log them.
func (p *PersistSubscriber) Handler() HandlerFunc {
	return func(ctx context.Context, e Event) error {
		if err := p.store.InsertEvent(ctx, e); err != nil {
			p.logger.Warn("persist subscriber: failed to insert event",
				"event_id", e.EventID,
				"event_type", e.Type,
				"error", err,
			)
			return err
		}
		return nil
	}
}

// Subscribe registers PersistSubscriber on the given Bus.
// Uses pattern "*", concurrency 1 (serial writes), buffer 1024.
// Returns the unsubscribe function.
func (p *PersistSubscriber) Subscribe(bus *Bus) (unsubscribe func()) {
	return bus.Subscribe("*", p.Handler(), SubscriberOptions{
		Concurrency: 1,
		BufferSize:  1024,
	})
}
