package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NotificationChannel represents a configured notification delivery target.
type NotificationChannel struct {
	ChannelID   string    `json:"channel_id"`
	ChannelType string    `json:"channel_type"` // "email" or "telegram"
	Name        string    `json:"name"`
	Config      string    `json:"config"` // JSON blob (SMTP settings, bot token, etc.)
	Enabled     bool      `json:"enabled"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NotificationSubscription links a user + channel + event type.
type NotificationSubscription struct {
	UserID    string `json:"user_id,omitempty"`
	ChannelID string `json:"channel_id"`
	EventType string `json:"event_type"`
}

// ChannelSubscription pairs a channel config with its type for delivery.
// Used by the notification dispatcher.
type ChannelSubscription struct {
	ChannelID   string
	ChannelType string
	Config      string
}

// CreateNotificationChannel inserts a new notification channel.
func (s *Store) CreateNotificationChannel(ctx context.Context, ch NotificationChannel) (*NotificationChannel, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO notification_channels (channel_id, channel_type, name, config, enabled, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, ch.ChannelType, ch.Name, ch.Config, ch.Enabled, ch.CreatedBy, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create notification channel: %w", err)
	}

	result := NotificationChannel{
		ChannelID:   id,
		ChannelType: ch.ChannelType,
		Name:        ch.Name,
		Config:      ch.Config,
		Enabled:     ch.Enabled,
		CreatedBy:   ch.CreatedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return &result, nil
}

// GetNotificationChannel retrieves a notification channel by ID.
func (s *Store) GetNotificationChannel(ctx context.Context, channelID string) (*NotificationChannel, error) {
	var ch NotificationChannel
	err := s.reader.QueryRowContext(ctx,
		`SELECT channel_id, channel_type, name, config, enabled, created_by, created_at, updated_at
		 FROM notification_channels WHERE channel_id = ?`, channelID,
	).Scan(&ch.ChannelID, &ch.ChannelType, &ch.Name, &ch.Config, &ch.Enabled, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("notification channel %s: %w", channelID, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get notification channel: %w", err)
	}
	return &ch, nil
}

// ListNotificationChannels returns all notification channels ordered by created_at DESC.
func (s *Store) ListNotificationChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT channel_id, channel_type, name, config, enabled, created_by, created_at, updated_at
		 FROM notification_channels ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list notification channels: %w", err)
	}
	defer rows.Close()

	var channels []NotificationChannel
	for rows.Next() {
		var ch NotificationChannel
		if err := rows.Scan(&ch.ChannelID, &ch.ChannelType, &ch.Name, &ch.Config, &ch.Enabled, &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification channel: %w", err)
		}
		channels = append(channels, ch)
	}
	return channels, rows.Err()
}

// UpdateNotificationChannel updates a notification channel's mutable fields.
func (s *Store) UpdateNotificationChannel(ctx context.Context, ch NotificationChannel) (*NotificationChannel, error) {
	now := time.Now().UTC()
	result, err := s.writer.ExecContext(ctx,
		`UPDATE notification_channels SET name = ?, config = ?, enabled = ?, updated_at = ?
		 WHERE channel_id = ?`,
		ch.Name, ch.Config, ch.Enabled, now, ch.ChannelID,
	)
	if err != nil {
		return nil, fmt.Errorf("update notification channel: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, fmt.Errorf("notification channel %s: %w", ch.ChannelID, ErrNotFound)
	}

	updated, err := s.GetNotificationChannel(ctx, ch.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("read updated notification channel: %w", err)
	}
	return updated, nil
}

// DeleteNotificationChannel removes a notification channel by ID.
// Subscriptions cascade via FK ON DELETE CASCADE.
func (s *Store) DeleteNotificationChannel(ctx context.Context, channelID string) error {
	result, err := s.writer.ExecContext(ctx, `DELETE FROM notification_channels WHERE channel_id = ?`, channelID)
	if err != nil {
		return fmt.Errorf("delete notification channel: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("notification channel %s: %w", channelID, ErrNotFound)
	}
	return nil
}

// GetSubscriptions returns all notification subscriptions for a user.
func (s *Store) GetSubscriptions(ctx context.Context, userID string) ([]NotificationSubscription, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT user_id, channel_id, event_type FROM notification_subscriptions WHERE user_id = ?`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("get subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []NotificationSubscription
	for rows.Next() {
		var sub NotificationSubscription
		if err := rows.Scan(&sub.UserID, &sub.ChannelID, &sub.EventType); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

// SetSubscriptions replaces all notification subscriptions for a user.
func (s *Store) SetSubscriptions(ctx context.Context, userID string, subs []NotificationSubscription) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_subscriptions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("delete existing subscriptions: %w", err)
	}

	for _, sub := range subs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notification_subscriptions (user_id, channel_id, event_type) VALUES (?, ?, ?)`,
			userID, sub.ChannelID, sub.EventType,
		); err != nil {
			return fmt.Errorf("insert subscription: %w", err)
		}
	}

	return tx.Commit()
}

// ListSubscriptionsForEvent returns all channel subscriptions matching an event type.
// Only enabled channels are included. This is the method used by the notification dispatcher.
func (s *Store) ListSubscriptionsForEvent(ctx context.Context, eventType string) ([]ChannelSubscription, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT DISTINCT nc.channel_id, nc.channel_type, nc.config
		 FROM notification_subscriptions ns
		 JOIN notification_channels nc ON ns.channel_id = nc.channel_id
		 WHERE ns.event_type = ? AND nc.enabled = 1`, eventType,
	)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for event: %w", err)
	}
	defer rows.Close()

	var subs []ChannelSubscription
	for rows.Next() {
		var sub ChannelSubscription
		if err := rows.Scan(&sub.ChannelID, &sub.ChannelType, &sub.Config); err != nil {
			return nil, fmt.Errorf("scan channel subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}
