package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// SubscriptionStore is the minimal interface the Dispatcher needs.
type SubscriptionStore interface {
	ListSubscriptionsForEvent(ctx context.Context, eventType string) ([]store.ChannelSubscription, error)
}

// Dispatcher subscribes to the event bus and fans out notifications to
// matching channel notifiers, with rate limiting.
type Dispatcher struct {
	store     SubscriptionStore
	resolver  ConnectorResolver
	notifiers map[string]Notifier
	renderers map[string]EventRenderer
	limiter   *RateLimiter
	renderer  EventRenderer
	logger    *slog.Logger
}

// NewDispatcher creates a notification Dispatcher.
func NewDispatcher(
	s SubscriptionStore,
	resolver ConnectorResolver,
	notifiers map[string]Notifier,
	renderers map[string]EventRenderer,
	defaultRenderer EventRenderer,
	logger *slog.Logger,
) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		store:     s,
		resolver:  resolver,
		notifiers: notifiers,
		renderers: renderers,
		limiter:   NewRateLimiter(60 * time.Second),
		renderer:  defaultRenderer,
		logger:    logger,
	}
}

// Handler returns a HandlerFunc suitable for Bus.Subscribe.
func (d *Dispatcher) Handler() event.HandlerFunc {
	return func(ctx context.Context, e event.Event) error {
		subs, err := d.store.ListSubscriptionsForEvent(ctx, e.Type)
		if err != nil {
			d.logger.Warn("notification dispatcher: list subscriptions",
				"event_type", e.Type, "error", err)
			return nil
		}
		if len(subs) == 0 {
			return nil
		}

		for _, sub := range subs {
			if !d.limiter.Allow(sub.ChannelID, e.Type) {
				d.logger.Debug("notification rate-limited",
					"channel_id", sub.ChannelID, "event_type", e.Type)
				continue
			}

			notifier, ok := d.notifiers[sub.ChannelType]
			if !ok {
				d.logger.Warn("notification dispatcher: unknown channel type",
					"type", sub.ChannelType)
				continue
			}

			// Select per-channel-type renderer, falling back to default.
			render := d.renderer
			if d.renderers != nil {
				if r, ok := d.renderers[sub.ChannelType]; ok {
					render = r
				}
			}
			subject, body := render(e)

			// Resolve connector config if ConnectorID is set.
			config := sub.Config
			if sub.ConnectorID != nil && *sub.ConnectorID != "" && d.resolver != nil {
				resolved, err := d.resolver.ResolveConnectorConfig(ctx, *sub.ConnectorID)
				if err != nil {
					d.logger.Warn("notification dispatcher: resolve connector config",
						"channel_id", sub.ChannelID,
						"connector_id", *sub.ConnectorID,
						"error", err)
					continue
				}
				config = resolved
			}

			if err := notifier.Send(ctx, config, subject, body); err != nil {
				d.logger.Warn("notification dispatcher: send failed",
					"channel_id", sub.ChannelID,
					"channel_type", sub.ChannelType,
					"event_type", e.Type,
					"error", err)
			}
		}
		return nil
	}
}

// DefaultEventRenderer converts an event into a subject and body string.
func DefaultEventRenderer(e event.Event) (subject, body string) {
	subject = e.Type

	var lines []string
	for k, v := range e.Payload {
		lines = append(lines, fmt.Sprintf("%s: %v", k, v))
	}
	sort.Strings(lines)
	body = strings.Join(lines, "\n")
	return subject, body
}
