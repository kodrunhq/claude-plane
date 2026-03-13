package event

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// --- Mock store ---

// mockWebhookStore implements WebhookStore for testing.
type mockWebhookStore struct {
	mu               sync.Mutex
	webhooks         []Webhook
	createdDelivery  *WebhookDelivery
	updatedDelivery  *WebhookDelivery
	pendingList      []WebhookDelivery
	listWebhooksErr  error
	createDeliveryErr error
}

func (m *mockWebhookStore) ListWebhooks(_ context.Context) ([]Webhook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listWebhooksErr != nil {
		return nil, m.listWebhooksErr
	}
	out := make([]Webhook, len(m.webhooks))
	copy(out, m.webhooks)
	return out, nil
}

func (m *mockWebhookStore) CreateDelivery(_ context.Context, d WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createDeliveryErr != nil {
		return m.createDeliveryErr
	}
	cp := d
	m.createdDelivery = &cp
	return nil
}

func (m *mockWebhookStore) UpdateDelivery(_ context.Context, d WebhookDelivery) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := d
	m.updatedDelivery = &cp
	return nil
}

func (m *mockWebhookStore) PendingDeliveries(_ context.Context) ([]WebhookDelivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WebhookDelivery, len(m.pendingList))
	copy(out, m.pendingList)
	return out, nil
}

func (m *mockWebhookStore) lastUpdated() *WebhookDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updatedDelivery
}

// --- Helpers ---

func webhookTestEvent(eventType string) Event {
	return Event{
		EventID:   "evt-abc123",
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Source:    "test",
		Payload:   map[string]any{"key": "value"},
	}
}

func expectedHMAC(body, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// waitForCondition polls pred every 5 ms until it returns true or timeout elapses.
func waitForCondition(t *testing.T, timeout time.Duration, pred func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// --- Tests ---

// TestHandlerDeliversToMatchingWebhook verifies that a matching, enabled webhook
// receives an HTTP POST with the serialised event body.
func TestHandlerDeliversToMatchingWebhook(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-1",
				URL:       srv.URL,
				Secret:    []byte("secret"),
				Events:    []string{"run.*"},
				Enabled:   true,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	ev := webhookTestEvent(TypeRunCreated)
	if err := handler(context.Background(), ev); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	select {
	case body := <-received:
		var got Event
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("could not unmarshal received body: %v", err)
		}
		if got.EventID != ev.EventID {
			t.Errorf("EventID = %q, want %q", got.EventID, ev.EventID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP request to reach test server")
	}
}

// TestHandlerHMACSignatureIsCorrect verifies that the X-Signature-256 header
// contains the correct sha256=<hex> value.
func TestHandlerHMACSignatureIsCorrect(t *testing.T) {
	secret := []byte("super-secret")
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Signature-256")
		body, _ := io.ReadAll(r.Body)
		// Verify signature against body inline.
		want := "sha256=" + expectedHMAC(body, secret)
		if gotSig != want {
			t.Errorf("X-Signature-256 = %q, want %q", gotSig, want)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-sig",
				URL:       srv.URL,
				Secret:    secret,
				Events:    []string{"*"},
				Enabled:   true,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	if err := handler(context.Background(), webhookTestEvent(TypeRunCompleted)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if gotSig == "" {
		t.Error("X-Signature-256 header was empty or not received")
	}
}

// TestHandlerNonMatchingPatternSkipsDelivery ensures webhooks whose event
// patterns do not match the published event type are never called.
func TestHandlerNonMatchingPatternSkipsDelivery(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-nomatch",
				URL:       srv.URL,
				Secret:    nil,
				Events:    []string{"session.*"}, // does not match run.* events
				Enabled:   true,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	// Publish a run event — must NOT reach the session.* webhook.
	if err := handler(context.Background(), webhookTestEvent(TypeRunCreated)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Give any inadvertent async delivery a moment to arrive.
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("non-matching webhook was called unexpectedly")
	}
	if store.createdDelivery != nil {
		t.Error("delivery row was created for non-matching webhook")
	}
}

// TestHandlerHTTPFailureCreatesRetryDelivery verifies that when the target
// server returns a non-2xx status, the delivery is updated with status "pending"
// and a non-nil NextRetryAt.
func TestHandlerHTTPFailureCreatesRetryDelivery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-fail",
				URL:       srv.URL,
				Secret:    nil,
				Events:    []string{"*"},
				Enabled:   true,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	if err := handler(context.Background(), webhookTestEvent(TypeRunFailed)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	ok := waitForCondition(t, 2*time.Second, func() bool {
		return store.lastUpdated() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for UpdateDelivery to be called")
	}

	updated := store.lastUpdated()
	if updated.Status != "pending" {
		t.Errorf("status = %q, want %q", updated.Status, "pending")
	}
	if updated.NextRetryAt == nil {
		t.Error("NextRetryAt must be set on failure")
	}
	if updated.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", updated.Attempts)
	}
}

// TestComputeHMACProducesCorrectHex validates computeHMAC against a known value.
func TestComputeHMACProducesCorrectHex(t *testing.T) {
	body := []byte(`{"event_type":"run.created"}`)
	secret := []byte("mysecret")

	got := computeHMAC(body, secret)
	want := expectedHMAC(body, secret)

	if got != want {
		t.Errorf("computeHMAC = %q, want %q", got, want)
	}
}

// TestComputeHMACEmptySecretReturnsEmpty verifies that an empty secret yields
// an empty signature (no signing performed).
func TestComputeHMACEmptySecretReturnsEmpty(t *testing.T) {
	result := computeHMAC([]byte("body"), nil)
	if result != "" {
		t.Errorf("expected empty string for nil secret, got %q", result)
	}

	result = computeHMAC([]byte("body"), []byte{})
	if result != "" {
		t.Errorf("expected empty string for empty secret, got %q", result)
	}
}

// TestRetryBackoffValues confirms the three documented backoff tiers.
func TestRetryBackoffValues(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 1 * time.Second},
		{2, 5 * time.Second},
		{3, 30 * time.Second},
		{4, 30 * time.Second}, // beyond known attempts → default
		{10, 30 * time.Second},
	}

	for _, tc := range cases {
		got := retryBackoff(tc.attempt)
		if got != tc.want {
			t.Errorf("retryBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

// TestHandlerDisabledWebhookSkipsDelivery ensures that a webhook with
// Enabled=false is never called even when its pattern matches.
func TestHandlerDisabledWebhookSkipsDelivery(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-disabled",
				URL:       srv.URL,
				Secret:    nil,
				Events:    []string{"*"},
				Enabled:   false,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	if err := handler(context.Background(), webhookTestEvent(TypeRunCreated)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("disabled webhook was called unexpectedly")
	}
	if store.createdDelivery != nil {
		t.Error("delivery row was created for disabled webhook")
	}
}

// TestHandlerSuccessMarksDeliverySuccess verifies that a 2xx response causes
// the delivery to be updated with status "success" and no retry time.
func TestHandlerSuccessMarksDeliverySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &mockWebhookStore{
		webhooks: []Webhook{
			{
				WebhookID: "wh-ok",
				URL:       srv.URL,
				Secret:    nil,
				Events:    []string{"*"},
				Enabled:   true,
			},
		},
	}

	deliverer := NewWebhookDeliverer(store, nil, nullLogger())
	handler := deliverer.Handler()

	if err := handler(context.Background(), webhookTestEvent(TypeRunCompleted)); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	ok := waitForCondition(t, 2*time.Second, func() bool {
		return store.lastUpdated() != nil
	})
	if !ok {
		t.Fatal("timed out waiting for UpdateDelivery to be called")
	}

	updated := store.lastUpdated()
	if updated.Status != "success" {
		t.Errorf("status = %q, want %q", updated.Status, "success")
	}
	if updated.NextRetryAt != nil {
		t.Errorf("NextRetryAt must be nil on success, got %v", updated.NextRetryAt)
	}
}
