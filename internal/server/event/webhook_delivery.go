package event

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Webhook is the minimal representation of a webhook configuration needed by
// WebhookDeliverer. It mirrors store.Webhook but lives in this package to
// avoid an import cycle (store imports event; event must not import store).
type Webhook struct {
	WebhookID string
	URL       string
	Secret    []byte
	Events    []string
	Enabled   bool
}

// WebhookDelivery is the minimal delivery record used by WebhookDeliverer.
// It mirrors store.WebhookDelivery.
type WebhookDelivery struct {
	DeliveryID   string
	WebhookID    string
	EventID      string
	Status       string
	Attempts     int
	ResponseCode int
	LastError    string
	NextRetryAt  *time.Time
}

// WebhookStore is the persistence interface required by WebhookDeliverer.
// The store.Store type satisfies this interface via an adapter in the wiring layer.
type WebhookStore interface {
	ListWebhooks(ctx context.Context) ([]Webhook, error)
	CreateDelivery(ctx context.Context, d WebhookDelivery) error
	UpdateDelivery(ctx context.Context, d WebhookDelivery) error
	PendingDeliveries(ctx context.Context) ([]WebhookDelivery, error)
	GetEventByID(ctx context.Context, eventID string) (*Event, error)
}

const (
	maxDeliveryAttempts = 3
	retryPollInterval   = 10 * time.Second
)

// retryBackoff returns the backoff duration for the given attempt number (1-indexed).
// Attempt 1 → 1s, attempt 2 → 5s, attempt 3 → 30s.
func retryBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Second
	case 2:
		return 5 * time.Second
	default:
		return 30 * time.Second
	}
}

// WebhookDeliverer subscribes to the event bus and delivers events to matching
// outbound webhooks via HTTP POST with HMAC-SHA256 signing.
type WebhookDeliverer struct {
	store      WebhookStore
	httpClient *http.Client
	logger     *slog.Logger
}

// NewWebhookDeliverer creates a WebhookDeliverer. httpClient and logger are
// optional; defaults are used when nil.
func NewWebhookDeliverer(store WebhookStore, httpClient *http.Client, logger *slog.Logger) *WebhookDeliverer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookDeliverer{
		store:      store,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Handler returns a HandlerFunc suitable for Bus.Subscribe.
// On each event it queries matching webhooks, creates delivery rows, and
// attempts the HTTP POST.
func (d *WebhookDeliverer) Handler() HandlerFunc {
	return func(ctx context.Context, e Event) error {
		webhooks, err := d.store.ListWebhooks(ctx)
		if err != nil {
			return fmt.Errorf("webhook deliverer: list webhooks: %w", err)
		}

		var wg sync.WaitGroup
		for _, wh := range webhooks {
			if !wh.Enabled {
				continue
			}
			if !webhookMatchesEvent(wh.Events, e.Type) {
				continue
			}

			delivery := WebhookDelivery{
				DeliveryID: uuid.New().String(),
				WebhookID:  wh.WebhookID,
				EventID:    e.EventID,
				Status:     "pending",
				Attempts:   0,
			}

			if createErr := d.store.CreateDelivery(ctx, delivery); createErr != nil {
				d.logger.Warn("webhook deliverer: create delivery failed",
					"webhook_id", wh.WebhookID,
					"event_id", e.EventID,
					"error", createErr,
				)
				continue
			}

			wg.Add(1)
			go func(webhook Webhook, del WebhookDelivery) {
				defer wg.Done()
				d.deliver(context.Background(), webhook, e, &del)
			}(wh, delivery)
		}
		wg.Wait()
		return nil
	}
}

// StartRetryLoop starts a background goroutine that polls for pending deliveries
// and retries them with exponential backoff. It runs until ctx is cancelled.
func (d *WebhookDeliverer) StartRetryLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(retryPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.retryPending(ctx)
			}
		}
	}()
}

// retryPending fetches all due pending deliveries and attempts to re-deliver them.
func (d *WebhookDeliverer) retryPending(ctx context.Context) {
	pending, err := d.store.PendingDeliveries(ctx)
	if err != nil {
		d.logger.Warn("webhook deliverer: poll pending deliveries", "error", err)
		return
	}

	webhooks, err := d.store.ListWebhooks(ctx)
	if err != nil {
		d.logger.Warn("webhook deliverer: list webhooks for retry", "error", err)
		return
	}

	whByID := make(map[string]Webhook, len(webhooks))
	for _, wh := range webhooks {
		whByID[wh.WebhookID] = wh
	}

	for _, delivery := range pending {
		if delivery.Attempts >= maxDeliveryAttempts {
			// Exceeded max attempts — mark failed.
			delivery.Status = "failed"
			if err := d.store.UpdateDelivery(ctx, delivery); err != nil {
				d.logger.Warn("webhook deliverer: mark delivery failed", "delivery_id", delivery.DeliveryID, "error", err)
			}
			continue
		}

		wh, ok := whByID[delivery.WebhookID]
		if !ok {
			// Webhook was deleted; leave delivery as-is or it will be cleaned by cascade.
			continue
		}

		// Look up the full event so the retry payload is complete.
		fullEvent, err := d.store.GetEventByID(ctx, delivery.EventID)
		if err != nil {
			d.logger.Warn("webhook deliverer: get event for retry",
				"delivery_id", delivery.DeliveryID,
				"event_id", delivery.EventID,
				"error", err,
			)
			continue
		}
		d.deliver(ctx, wh, *fullEvent, &delivery)
	}
}

// deliver posts event to webhook and updates the delivery record.
func (d *WebhookDeliverer) deliver(ctx context.Context, wh Webhook, e Event, delivery *WebhookDelivery) {
	body, err := json.Marshal(e)
	if err != nil {
		d.logger.Warn("webhook deliverer: marshal event", "event_id", e.EventID, "error", err)
		return
	}

	sig := computeHMAC(body, wh.Secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		d.recordFailure(ctx, delivery, 0, fmt.Sprintf("build request: %s", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if sig != "" {
		req.Header.Set("X-Signature-256", "sha256="+sig)
	}

	resp, err := d.httpClient.Do(req)
	delivery.Attempts++

	if err != nil {
		next := nextRetryTime(delivery.Attempts)
		delivery.NextRetryAt = &next
		d.recordFailure(ctx, delivery, 0, err.Error())
		return
	}
	defer func() {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}()

	delivery.ResponseCode = resp.StatusCode

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.Status = "success"
		delivery.NextRetryAt = nil
	} else {
		next := nextRetryTime(delivery.Attempts)
		delivery.NextRetryAt = &next
		delivery.Status = "pending"
		delivery.LastError = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	if err := d.store.UpdateDelivery(ctx, *delivery); err != nil {
		d.logger.Warn("webhook deliverer: update delivery after send",
			"delivery_id", delivery.DeliveryID,
			"error", err,
		)
	}
}

// recordFailure marks a delivery as pending with an error and next retry time.
func (d *WebhookDeliverer) recordFailure(ctx context.Context, delivery *WebhookDelivery, code int, errMsg string) {
	delivery.Status = "pending"
	delivery.ResponseCode = code
	delivery.LastError = errMsg
	next := nextRetryTime(delivery.Attempts)
	delivery.NextRetryAt = &next

	if err := d.store.UpdateDelivery(ctx, *delivery); err != nil {
		d.logger.Warn("webhook deliverer: record failure",
			"delivery_id", delivery.DeliveryID,
			"error", err,
		)
	}
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of body using secret.
// If secret is empty, an empty string is returned (no signing).
func computeHMAC(body, secret []byte) string {
	if len(secret) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// nextRetryTime returns the absolute time for the next retry based on attempt count.
func nextRetryTime(attempts int) time.Time {
	return time.Now().UTC().Add(retryBackoff(attempts))
}

// webhookMatchesEvent reports whether any pattern in the webhook's event list
// matches the given event type using the bus MatchPattern rules.
func webhookMatchesEvent(patterns []string, eventType string) bool {
	for _, p := range patterns {
		if MatchPattern(p, eventType) {
			return true
		}
	}
	return false
}
