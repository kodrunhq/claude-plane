package store

import (
	"context"
	"errors"
	"testing"
)

func TestNotificationChannel_CRUD(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	// Create
	ch, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "email",
		Name:        "Team Email",
		Config:      `{"host":"smtp.example.com","port":587,"from":"noreply@example.com","to":"team@example.com"}`,
		Enabled:     true,
		CreatedBy:   "user-1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ch.ChannelID == "" {
		t.Fatal("expected non-empty channel_id")
	}

	// Get
	got, err := s.GetNotificationChannel(ctx, ch.ChannelID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Team Email" {
		t.Errorf("Name = %q, want %q", got.Name, "Team Email")
	}
	if got.ChannelType != "email" {
		t.Errorf("ChannelType = %q, want %q", got.ChannelType, "email")
	}

	// List
	channels, err := s.ListNotificationChannels(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}

	// Update
	updated, err := s.UpdateNotificationChannel(ctx, NotificationChannel{
		ChannelID: ch.ChannelID,
		Name:      "Updated Email",
		Config:    ch.Config,
		Enabled:   false,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Updated Email" {
		t.Errorf("updated Name = %q, want %q", updated.Name, "Updated Email")
	}
	if updated.Enabled {
		t.Error("expected Enabled = false")
	}

	// Delete
	if err := s.DeleteNotificationChannel(ctx, ch.ChannelID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify deleted
	_, err = s.GetNotificationChannel(ctx, ch.ChannelID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestNotificationChannel_GetNotFound(t *testing.T) {
	s := mustNewStore(t)
	_, err := s.GetNotificationChannel(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNotificationChannel_DeleteNotFound(t *testing.T) {
	s := mustNewStore(t)
	err := s.DeleteNotificationChannel(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNotificationChannel_UpdateNotFound(t *testing.T) {
	s := mustNewStore(t)
	_, err := s.UpdateNotificationChannel(context.Background(), NotificationChannel{
		ChannelID: "nonexistent",
		Name:      "x",
		Config:    "{}",
		Enabled:   true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSubscriptions_SetAndGet(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	// Create two channels
	ch1, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "email", Name: "Email", Config: "{}", Enabled: true, CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("create ch1: %v", err)
	}
	ch2, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "telegram", Name: "Telegram", Config: "{}", Enabled: true, CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("create ch2: %v", err)
	}

	// Set subscriptions
	subs := []NotificationSubscription{
		{ChannelID: ch1.ChannelID, EventType: "run.completed"},
		{ChannelID: ch1.ChannelID, EventType: "run.failed"},
		{ChannelID: ch2.ChannelID, EventType: "run.failed"},
	}
	if err := s.SetSubscriptions(ctx, "user-1", subs); err != nil {
		t.Fatalf("set subscriptions: %v", err)
	}

	// Get subscriptions
	got, err := s.GetSubscriptions(ctx, "user-1")
	if err != nil {
		t.Fatalf("get subscriptions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(got))
	}

	// Replace subscriptions (should clear old ones)
	newSubs := []NotificationSubscription{
		{ChannelID: ch2.ChannelID, EventType: "session.started"},
	}
	if err := s.SetSubscriptions(ctx, "user-1", newSubs); err != nil {
		t.Fatalf("set subscriptions (replace): %v", err)
	}
	got, err = s.GetSubscriptions(ctx, "user-1")
	if err != nil {
		t.Fatalf("get subscriptions after replace: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 subscription after replace, got %d", len(got))
	}
}

func TestListSubscriptionsForEvent(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	// Create enabled and disabled channels
	chEnabled, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "email", Name: "Enabled", Config: `{"host":"smtp"}`, Enabled: true, CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("create enabled channel: %v", err)
	}
	chDisabled, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "telegram", Name: "Disabled", Config: `{"token":"x"}`, Enabled: false, CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("create disabled channel: %v", err)
	}

	// Subscribe to both
	subs := []NotificationSubscription{
		{ChannelID: chEnabled.ChannelID, EventType: "run.completed"},
		{ChannelID: chDisabled.ChannelID, EventType: "run.completed"},
	}
	if err := s.SetSubscriptions(ctx, "user-1", subs); err != nil {
		t.Fatalf("set subscriptions: %v", err)
	}

	// Only enabled channel should be returned
	result, err := s.ListSubscriptionsForEvent(ctx, "run.completed")
	if err != nil {
		t.Fatalf("list subscriptions for event: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result (only enabled), got %d", len(result))
	}
	if result[0].ChannelID != chEnabled.ChannelID {
		t.Errorf("expected channel %s, got %s", chEnabled.ChannelID, result[0].ChannelID)
	}
	if result[0].ChannelType != "email" {
		t.Errorf("expected channel_type email, got %s", result[0].ChannelType)
	}
}

func TestSubscriptions_CascadeOnChannelDelete(t *testing.T) {
	s := mustNewStore(t)
	ctx := context.Background()

	ch, err := s.CreateNotificationChannel(ctx, NotificationChannel{
		ChannelType: "email", Name: "To Delete", Config: "{}", Enabled: true, CreatedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("create channel: %v", err)
	}

	subs := []NotificationSubscription{
		{ChannelID: ch.ChannelID, EventType: "run.completed"},
	}
	if err := s.SetSubscriptions(ctx, "user-1", subs); err != nil {
		t.Fatalf("set subscriptions: %v", err)
	}

	// Delete channel
	if err := s.DeleteNotificationChannel(ctx, ch.ChannelID); err != nil {
		t.Fatalf("delete channel: %v", err)
	}

	// Subscriptions should be gone
	got, err := s.GetSubscriptions(ctx, "user-1")
	if err != nil {
		t.Fatalf("get subscriptions after cascade: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 subscriptions after cascade, got %d", len(got))
	}
}
