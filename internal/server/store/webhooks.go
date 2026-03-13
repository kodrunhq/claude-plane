package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Webhook represents a configured outbound webhook destination.
// The Secret field is never serialized to JSON; it is stored as a BLOB.
type Webhook struct {
	WebhookID string    `json:"webhook_id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    []byte    `json:"-"`
	Events    []string  `json:"events"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WebhookDelivery tracks a single delivery attempt for a webhook.
type WebhookDelivery struct {
	DeliveryID   string     `json:"delivery_id"`
	WebhookID    string     `json:"webhook_id"`
	EventID      string     `json:"event_id"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	ResponseCode int        `json:"response_code,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateWebhook inserts a new webhook and returns it with a generated ID.
func (s *Store) CreateWebhook(ctx context.Context, w Webhook) (*Webhook, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	eventsJSON, err := json.Marshal(w.Events)
	if err != nil {
		return nil, fmt.Errorf("marshal webhook events: %w", err)
	}

	_, err = s.writer.ExecContext(ctx,
		`INSERT INTO webhooks (webhook_id, name, url, secret, events, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, w.Name, w.URL, nullBytesIfEmpty(w.Secret), string(eventsJSON), w.Enabled, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}

	result := Webhook{
		WebhookID: id,
		Name:      w.Name,
		URL:       w.URL,
		Secret:    w.Secret,
		Events:    w.Events,
		Enabled:   w.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return &result, nil
}

// GetWebhook retrieves a webhook by ID.
func (s *Store) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	var w Webhook
	var eventsJSON string
	var secret []byte

	err := s.reader.QueryRowContext(ctx,
		`SELECT webhook_id, name, url, secret, events, enabled, created_at, updated_at
		 FROM webhooks WHERE webhook_id = ?`, id,
	).Scan(&w.WebhookID, &w.Name, &w.URL, &secret, &eventsJSON, &w.Enabled, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("webhook %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook: %w", err)
	}

	w.Secret = secret

	if err := json.Unmarshal([]byte(eventsJSON), &w.Events); err != nil {
		return nil, fmt.Errorf("unmarshal webhook events: %w", err)
	}

	return &w, nil
}

// ListWebhooks returns all webhooks ordered by created_at DESC.
func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT webhook_id, name, url, secret, events, enabled, created_at, updated_at
		 FROM webhooks ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []Webhook
	for rows.Next() {
		var w Webhook
		var eventsJSON string
		var secret []byte

		if err := rows.Scan(&w.WebhookID, &w.Name, &w.URL, &secret, &eventsJSON, &w.Enabled, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook: %w", err)
		}

		w.Secret = secret

		if err := json.Unmarshal([]byte(eventsJSON), &w.Events); err != nil {
			return nil, fmt.Errorf("unmarshal webhook events: %w", err)
		}

		webhooks = append(webhooks, w)
	}
	return webhooks, rows.Err()
}

// UpdateWebhook replaces a webhook's mutable fields and returns the updated record.
func (s *Store) UpdateWebhook(ctx context.Context, w Webhook) (*Webhook, error) {
	eventsJSON, err := json.Marshal(w.Events)
	if err != nil {
		return nil, fmt.Errorf("marshal webhook events: %w", err)
	}

	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`UPDATE webhooks SET name = ?, url = ?, secret = ?, events = ?, enabled = ?, updated_at = ?
		 WHERE webhook_id = ?`,
		w.Name, w.URL, nullBytesIfEmpty(w.Secret), string(eventsJSON), w.Enabled, now, w.WebhookID,
	)
	if err != nil {
		return nil, fmt.Errorf("update webhook: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("webhook %s: %w", w.WebhookID, ErrNotFound)
	}

	updated, err := s.GetWebhook(ctx, w.WebhookID)
	if err != nil {
		return nil, fmt.Errorf("read updated webhook: %w", err)
	}
	return updated, nil
}

// DeleteWebhook removes a webhook by ID. Deliveries cascade via FK ON DELETE CASCADE.
func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	result, err := s.writer.ExecContext(ctx, `DELETE FROM webhooks WHERE webhook_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("webhook %s: %w", id, ErrNotFound)
	}
	return nil
}

// CreateDelivery inserts a new delivery record.
func (s *Store) CreateDelivery(ctx context.Context, d WebhookDelivery) error {
	now := time.Now().UTC()
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO webhook_deliveries
		 (delivery_id, webhook_id, event_id, status, attempts, response_code, last_error, next_retry_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.DeliveryID, d.WebhookID, d.EventID, d.Status, d.Attempts,
		nullIntIfZero(d.ResponseCode), nullStringIfEmpty(d.LastError),
		nullTimeIfNil(d.NextRetryAt), now, now,
	)
	if err != nil {
		return fmt.Errorf("create delivery: %w", err)
	}
	return nil
}

// UpdateDelivery updates a delivery record's mutable fields.
func (s *Store) UpdateDelivery(ctx context.Context, d WebhookDelivery) error {
	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`UPDATE webhook_deliveries
		 SET status = ?, attempts = ?, response_code = ?, last_error = ?, next_retry_at = ?, updated_at = ?
		 WHERE delivery_id = ?`,
		d.Status, d.Attempts,
		nullIntIfZero(d.ResponseCode), nullStringIfEmpty(d.LastError),
		nullTimeIfNil(d.NextRetryAt), now, d.DeliveryID,
	)
	if err != nil {
		return fmt.Errorf("update delivery: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("delivery %s: %w", d.DeliveryID, ErrNotFound)
	}
	return nil
}

// ListDeliveries returns all deliveries for a webhook, ordered by created_at DESC.
func (s *Store) ListDeliveries(ctx context.Context, webhookID string) ([]WebhookDelivery, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT delivery_id, webhook_id, event_id, status, attempts,
		        COALESCE(response_code, 0), COALESCE(last_error, ''), next_retry_at, created_at, updated_at
		 FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_at DESC`, webhookID,
	)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	return scanDeliveries(rows)
}

// PendingDeliveries returns all deliveries with status "pending" that are due for retry.
func (s *Store) PendingDeliveries(ctx context.Context) ([]WebhookDelivery, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT delivery_id, webhook_id, event_id, status, attempts,
		        COALESCE(response_code, 0), COALESCE(last_error, ''), next_retry_at, created_at, updated_at
		 FROM webhook_deliveries
		 WHERE status = 'pending'
		   AND (next_retry_at IS NULL OR next_retry_at <= ?)
		 ORDER BY created_at ASC`, time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("pending deliveries: %w", err)
	}
	defer rows.Close()

	return scanDeliveries(rows)
}

// scanDeliveries scans rows into a slice of WebhookDelivery.
func scanDeliveries(rows *sql.Rows) ([]WebhookDelivery, error) {
	var deliveries []WebhookDelivery
	for rows.Next() {
		var d WebhookDelivery
		var nextRetryAt sql.NullTime

		if err := rows.Scan(
			&d.DeliveryID, &d.WebhookID, &d.EventID, &d.Status,
			&d.Attempts, &d.ResponseCode, &d.LastError,
			&nextRetryAt, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}

		if nextRetryAt.Valid {
			t := nextRetryAt.Time
			d.NextRetryAt = &t
		}

		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// nullBytesIfEmpty returns nil for empty byte slices.
func nullBytesIfEmpty(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

// nullIntIfZero returns nil for zero integers (used for response_code).
func nullIntIfZero(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}

// nullStringIfEmpty returns nil for empty strings (SQL-friendly).
func nullStringIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullTimeIfNil returns nil for nil time pointers.
func nullTimeIfNil(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC()
}
