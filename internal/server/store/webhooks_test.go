package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestStoreForWebhooks(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "webhooks_test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// insertTestEvent inserts a minimal event row so webhook_deliveries FK is satisfied.
func insertTestEvent(t *testing.T, s *Store, eventID string) {
	t.Helper()
	_, err := s.writer.ExecContext(context.Background(),
		`INSERT INTO events (event_id, event_type, timestamp, source, payload, created_at)
		 VALUES (?, 'run.created', ?, 'test', '{}', ?)`,
		eventID, time.Now().UTC(), time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("insertTestEvent: %v", err)
	}
}

func makeWebhook(name string) Webhook {
	return Webhook{
		Name:    name,
		URL:     "https://example.com/hook",
		Secret:  []byte("super-secret"),
		Events:  []string{"run.*", "session.started"},
		Enabled: true,
	}
}

// --- Webhook CRUD ---

func TestCreateWebhook(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	w, err := s.CreateWebhook(ctx, makeWebhook("test-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	if w.WebhookID == "" {
		t.Error("expected non-empty WebhookID")
	}
	if w.Name != "test-hook" {
		t.Errorf("Name = %q, want %q", w.Name, "test-hook")
	}
	if len(w.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(w.Events))
	}
	if w.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestCreateWebhook_NoSecret(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook := makeWebhook("no-secret")
	hook.Secret = nil

	w, err := s.CreateWebhook(ctx, hook)
	if err != nil {
		t.Fatalf("CreateWebhook (no secret): %v", err)
	}
	if w.Secret != nil {
		t.Errorf("expected nil Secret, got %v", w.Secret)
	}
}

func TestGetWebhook(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	created, err := s.CreateWebhook(ctx, makeWebhook("get-test"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	got, err := s.GetWebhook(ctx, created.WebhookID)
	if err != nil {
		t.Fatalf("GetWebhook: %v", err)
	}

	if got.WebhookID != created.WebhookID {
		t.Errorf("WebhookID = %q, want %q", got.WebhookID, created.WebhookID)
	}
	if got.Name != "get-test" {
		t.Errorf("Name = %q, want %q", got.Name, "get-test")
	}
	if string(got.Secret) != "super-secret" {
		t.Errorf("Secret = %q, want %q", string(got.Secret), "super-secret")
	}
}

func TestGetWebhook_NotFound(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	_, err := s.GetWebhook(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListWebhooks(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	// Empty list
	webhooks, err := s.ListWebhooks(ctx)
	if err != nil {
		t.Fatalf("ListWebhooks (empty): %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("expected 0 webhooks, got %d", len(webhooks))
	}

	// Create two webhooks
	if _, err := s.CreateWebhook(ctx, makeWebhook("hook-a")); err != nil {
		t.Fatalf("CreateWebhook a: %v", err)
	}
	if _, err := s.CreateWebhook(ctx, makeWebhook("hook-b")); err != nil {
		t.Fatalf("CreateWebhook b: %v", err)
	}

	webhooks, err = s.ListWebhooks(ctx)
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(webhooks) != 2 {
		t.Errorf("expected 2 webhooks, got %d", len(webhooks))
	}
}

func TestUpdateWebhook(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	created, err := s.CreateWebhook(ctx, makeWebhook("original"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	updated := Webhook{
		WebhookID: created.WebhookID,
		Name:      "updated-name",
		URL:       "https://new.example.com/hook",
		Secret:    []byte("new-secret"),
		Events:    []string{"run.completed"},
		Enabled:   false,
	}

	got, err := s.UpdateWebhook(ctx, updated)
	if err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}

	if got.Name != "updated-name" {
		t.Errorf("Name = %q, want %q", got.Name, "updated-name")
	}
	if got.URL != "https://new.example.com/hook" {
		t.Errorf("URL = %q, want %q", got.URL, "https://new.example.com/hook")
	}
	if got.Enabled {
		t.Error("expected Enabled = false")
	}
	if len(got.Events) != 1 || got.Events[0] != "run.completed" {
		t.Errorf("Events = %v, want [run.completed]", got.Events)
	}
}

func TestUpdateWebhook_NotFound(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	_, err := s.UpdateWebhook(ctx, Webhook{WebhookID: "does-not-exist", Events: []string{}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDeleteWebhook(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	created, err := s.CreateWebhook(ctx, makeWebhook("to-delete"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	if err := s.DeleteWebhook(ctx, created.WebhookID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}

	_, err = s.GetWebhook(ctx, created.WebhookID)
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	if err := s.DeleteWebhook(ctx, "ghost-id"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestWebhook_EventsRoundTrip(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	events := []string{"run.*", "session.*", "machine.connected"}
	hook := makeWebhook("roundtrip")
	hook.Events = events

	created, err := s.CreateWebhook(ctx, hook)
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	got, err := s.GetWebhook(ctx, created.WebhookID)
	if err != nil {
		t.Fatalf("GetWebhook: %v", err)
	}

	if len(got.Events) != len(events) {
		t.Fatalf("Events len = %d, want %d", len(got.Events), len(events))
	}
	for i, e := range events {
		if got.Events[i] != e {
			t.Errorf("Events[%d] = %q, want %q", i, got.Events[i], e)
		}
	}
}

// --- Delivery CRUD ---

func TestCreateDelivery(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("delivery-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	eventID := uuid.New().String()
	insertTestEvent(t, s, eventID)

	d := WebhookDelivery{
		DeliveryID: uuid.New().String(),
		WebhookID:  hook.WebhookID,
		EventID:    eventID,
		Status:     "pending",
		Attempts:   0,
	}

	if err := s.CreateDelivery(ctx, d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
}

func TestUpdateDelivery(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("update-delivery-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	eventID := uuid.New().String()
	insertTestEvent(t, s, eventID)

	deliveryID := uuid.New().String()
	d := WebhookDelivery{
		DeliveryID: deliveryID,
		WebhookID:  hook.WebhookID,
		EventID:    eventID,
		Status:     "pending",
		Attempts:   0,
	}
	if err := s.CreateDelivery(ctx, d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	d.Status = "success"
	d.Attempts = 1
	d.ResponseCode = 200
	if err := s.UpdateDelivery(ctx, d); err != nil {
		t.Fatalf("UpdateDelivery: %v", err)
	}

	deliveries, err := s.ListDeliveries(ctx, hook.WebhookID)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].Status != "success" {
		t.Errorf("Status = %q, want %q", deliveries[0].Status, "success")
	}
	if deliveries[0].ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", deliveries[0].ResponseCode)
	}
}

func TestUpdateDelivery_NotFound(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	err := s.UpdateDelivery(ctx, WebhookDelivery{DeliveryID: "ghost-delivery"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListDeliveries(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("list-deliveries-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	for i := 0; i < 3; i++ {
		eventID := uuid.New().String()
		insertTestEvent(t, s, eventID)

		d := WebhookDelivery{
			DeliveryID: uuid.New().String(),
			WebhookID:  hook.WebhookID,
			EventID:    eventID,
			Status:     "pending",
		}
		if err := s.CreateDelivery(ctx, d); err != nil {
			t.Fatalf("CreateDelivery %d: %v", i, err)
		}
	}

	deliveries, err := s.ListDeliveries(ctx, hook.WebhookID)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(deliveries) != 3 {
		t.Errorf("expected 3 deliveries, got %d", len(deliveries))
	}
}

func TestPendingDeliveries(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("pending-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	// Create a pending delivery (no next_retry_at, should be immediately due)
	eventID1 := uuid.New().String()
	insertTestEvent(t, s, eventID1)
	pendingD := WebhookDelivery{
		DeliveryID: uuid.New().String(),
		WebhookID:  hook.WebhookID,
		EventID:    eventID1,
		Status:     "pending",
	}
	if err := s.CreateDelivery(ctx, pendingD); err != nil {
		t.Fatalf("CreateDelivery pending: %v", err)
	}

	// Create a delivery scheduled far in the future (should NOT appear)
	eventID2 := uuid.New().String()
	insertTestEvent(t, s, eventID2)
	futureTime := time.Now().UTC().Add(1 * time.Hour)
	futureD := WebhookDelivery{
		DeliveryID:  uuid.New().String(),
		WebhookID:   hook.WebhookID,
		EventID:     eventID2,
		Status:      "pending",
		NextRetryAt: &futureTime,
	}
	if err := s.CreateDelivery(ctx, futureD); err != nil {
		t.Fatalf("CreateDelivery future: %v", err)
	}

	// Create a succeeded delivery (should NOT appear)
	eventID3 := uuid.New().String()
	insertTestEvent(t, s, eventID3)
	successD := WebhookDelivery{
		DeliveryID: uuid.New().String(),
		WebhookID:  hook.WebhookID,
		EventID:    eventID3,
		Status:     "success",
	}
	if err := s.CreateDelivery(ctx, successD); err != nil {
		t.Fatalf("CreateDelivery success: %v", err)
	}

	pending, err := s.PendingDeliveries(ctx)
	if err != nil {
		t.Fatalf("PendingDeliveries: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 pending delivery, got %d", len(pending))
	}
	if pending[0].DeliveryID != pendingD.DeliveryID {
		t.Errorf("DeliveryID = %q, want %q", pending[0].DeliveryID, pendingD.DeliveryID)
	}
}

func TestDeleteWebhook_CascadesDeliveries(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("cascade-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	eventID := uuid.New().String()
	insertTestEvent(t, s, eventID)

	if err := s.CreateDelivery(ctx, WebhookDelivery{
		DeliveryID: uuid.New().String(),
		WebhookID:  hook.WebhookID,
		EventID:    eventID,
		Status:     "pending",
	}); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	if err := s.DeleteWebhook(ctx, hook.WebhookID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}

	// Deliveries should be gone via CASCADE
	deliveries, err := s.ListDeliveries(ctx, hook.WebhookID)
	if err != nil {
		t.Fatalf("ListDeliveries after delete: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries after cascade delete, got %d", len(deliveries))
	}
}

func TestDelivery_NextRetryAt_RoundTrip(t *testing.T) {
	s := newTestStoreForWebhooks(t)
	ctx := context.Background()

	hook, err := s.CreateWebhook(ctx, makeWebhook("retry-hook"))
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	eventID := uuid.New().String()
	insertTestEvent(t, s, eventID)

	retryAt := time.Now().UTC().Add(5 * time.Second).Truncate(time.Second)
	d := WebhookDelivery{
		DeliveryID:  uuid.New().String(),
		WebhookID:   hook.WebhookID,
		EventID:     eventID,
		Status:      "pending",
		NextRetryAt: &retryAt,
	}
	if err := s.CreateDelivery(ctx, d); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	deliveries, err := s.ListDeliveries(ctx, hook.WebhookID)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	if deliveries[0].NextRetryAt == nil {
		t.Fatal("NextRetryAt is nil, want non-nil")
	}
	// SQLite stores datetime to second precision
	if !deliveries[0].NextRetryAt.Truncate(time.Second).Equal(retryAt) {
		t.Errorf("NextRetryAt = %v, want %v", deliveries[0].NextRetryAt, retryAt)
	}
}
