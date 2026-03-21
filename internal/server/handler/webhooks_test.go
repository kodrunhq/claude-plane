package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kodrunhq/claude-plane/internal/server/handler"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- mock store ---

type mockWebhookStore struct {
	webhooks   map[string]*store.Webhook
	deliveries map[string][]store.WebhookDelivery
	err        error
}

func newMockWebhookStore() *mockWebhookStore {
	return &mockWebhookStore{
		webhooks:   make(map[string]*store.Webhook),
		deliveries: make(map[string][]store.WebhookDelivery),
	}
}

func (m *mockWebhookStore) CreateWebhook(_ context.Context, w store.Webhook) (*store.Webhook, error) {
	if m.err != nil {
		return nil, m.err
	}
	w.WebhookID = uuid.New().String()
	w.CreatedAt = time.Now().UTC()
	w.UpdatedAt = w.CreatedAt
	cp := w
	m.webhooks[w.WebhookID] = &cp
	return &cp, nil
}

func (m *mockWebhookStore) GetWebhook(_ context.Context, id string) (*store.Webhook, error) {
	if m.err != nil {
		return nil, m.err
	}
	w, ok := m.webhooks[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *w
	return &cp, nil
}

func (m *mockWebhookStore) ListWebhooks(_ context.Context) ([]store.Webhook, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]store.Webhook, 0, len(m.webhooks))
	for _, w := range m.webhooks {
		result = append(result, *w)
	}
	return result, nil
}

func (m *mockWebhookStore) UpdateWebhook(_ context.Context, w store.Webhook) (*store.Webhook, error) {
	if m.err != nil {
		return nil, m.err
	}
	existing, ok := m.webhooks[w.WebhookID]
	if !ok {
		return nil, store.ErrNotFound
	}
	w.CreatedAt = existing.CreatedAt
	w.UpdatedAt = time.Now().UTC()
	cp := w
	m.webhooks[w.WebhookID] = &cp
	return &cp, nil
}

func (m *mockWebhookStore) DeleteWebhook(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.webhooks[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.webhooks, id)
	delete(m.deliveries, id)
	return nil
}

func (m *mockWebhookStore) ListDeliveries(_ context.Context, webhookID string) ([]store.WebhookDelivery, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.deliveries[webhookID], nil
}

// --- router helper ---

func newWebhookRouter(h *handler.WebhookHandler) *httptest.Server {
	r := chi.NewRouter()
	handler.RegisterWebhookRoutes(r, h)
	return httptest.NewServer(r)
}

func toJSON(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

// --- tests ---

func TestWebhookHandler_ListWebhooks_Empty(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/webhooks")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.Webhook
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 webhooks, got %d", len(result))
	}
}

func TestWebhookHandler_CreateWebhook(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name":    "my-hook",
		"url":     "https://example.com/hook",
		"secret":  "s3cr3t",
		"events":  []string{"run.*"},
		"enabled": true,
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created store.Webhook
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.WebhookID == "" {
		t.Error("expected non-empty webhook_id")
	}
	if created.Name != "my-hook" {
		t.Errorf("Name = %q, want %q", created.Name, "my-hook")
	}
}

func TestWebhookHandler_CreateWebhook_MissingName(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"url":    "https://example.com/hook",
		"events": []string{"run.*"},
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_CreateWebhook_MissingURL(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name":   "hook",
		"events": []string{"run.*"},
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_CreateWebhook_MissingEvents(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name": "hook",
		"url":  "https://example.com/hook",
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_CreateWebhook_StoreError(t *testing.T) {
	mock := newMockWebhookStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name":   "hook",
		"url":    "https://example.com/hook",
		"events": []string{"run.*"},
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_GetWebhook(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	// Pre-insert a webhook directly into the mock.
	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "direct-hook",
		URL:       "https://example.com",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	resp, err := http.Get(srv.URL + "/api/v1/webhooks/" + wh.WebhookID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var got store.Webhook
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.WebhookID != wh.WebhookID {
		t.Errorf("WebhookID = %q, want %q", got.WebhookID, wh.WebhookID)
	}
}

func TestWebhookHandler_GetWebhook_NotFound(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/webhooks/nonexistent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_UpdateWebhook(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "old-name",
		URL:       "https://old.example.com",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	body := map[string]interface{}{
		"name":    "new-name",
		"url":     "https://new.example.com",
		"events":  []string{"session.*"},
		"enabled": false,
	}

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/webhooks/"+wh.WebhookID, toJSON(t, body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated store.Webhook
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Name != "new-name" {
		t.Errorf("Name = %q, want %q", updated.Name, "new-name")
	}
	if updated.Enabled {
		t.Error("expected Enabled = false")
	}
}

func TestWebhookHandler_UpdateWebhook_NotFound(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name":   "x",
		"url":    "https://x.example.com",
		"events": []string{"run.*"},
	}

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/webhooks/ghost", toJSON(t, body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_DeleteWebhook(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "to-delete",
		URL:       "https://example.com",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/webhooks/"+wh.WebhookID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	if _, ok := mock.webhooks[wh.WebhookID]; ok {
		t.Error("webhook still exists after delete")
	}
}

func TestWebhookHandler_DeleteWebhook_NotFound(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/webhooks/ghost", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_ListDeliveries(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "delivery-hook",
		URL:       "https://example.com",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh
	mock.deliveries[wh.WebhookID] = []store.WebhookDelivery{
		{DeliveryID: uuid.New().String(), WebhookID: wh.WebhookID, EventID: uuid.New().String(), Status: "success", CreatedAt: time.Now().UTC()},
		{DeliveryID: uuid.New().String(), WebhookID: wh.WebhookID, EventID: uuid.New().String(), Status: "failed", CreatedAt: time.Now().UTC()},
	}

	resp, err := http.Get(srv.URL + "/api/v1/webhooks/" + wh.WebhookID + "/deliveries")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var deliveries []store.WebhookDelivery
	if err := json.NewDecoder(resp.Body).Decode(&deliveries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(deliveries) != 2 {
		t.Errorf("expected 2 deliveries, got %d", len(deliveries))
	}
}

func TestWebhookHandler_ListDeliveries_WebhookNotFound(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/webhooks/ghost/deliveries")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_ListDeliveries_Empty(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "no-delivery-hook",
		URL:       "https://example.com",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	resp, err := http.Get(srv.URL + "/api/v1/webhooks/" + wh.WebhookID + "/deliveries")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var deliveries []store.WebhookDelivery
	if err := json.NewDecoder(resp.Body).Decode(&deliveries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries, got %d", len(deliveries))
	}
}

func TestWebhookHandler_CreateWebhook_PublishesEvent(t *testing.T) {
	mock := newMockWebhookStore()
	pub := &mockPublisher{}
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	h.SetPublisher(pub)
	srv := newWebhookRouter(h)
	defer srv.Close()

	body := map[string]interface{}{
		"name":    "event-hook",
		"url":     "https://example.com/hook",
		"events":  []string{"run.*"},
		"enabled": true,
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 published event, got %d", len(events))
	}
	evt := events[0]
	if evt.Type != "webhook.created" {
		t.Errorf("event type = %q, want %q", evt.Type, "webhook.created")
	}
	if evt.Payload["webhook_name"] != "event-hook" {
		t.Errorf("payload webhook_name = %v, want %q", evt.Payload["webhook_name"], "event-hook")
	}
}

func TestWebhookHandler_TestDelivery(t *testing.T) {
	mock := newMockWebhookStore()
	pub := &mockPublisher{}
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	h.SetPublisher(pub)
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "test-hook",
		URL:       "https://example.com/hook",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	resp, err := http.Post(srv.URL+"/api/v1/webhooks/"+wh.WebhookID+"/test", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) < 1 {
		t.Fatalf("expected at least 1 published event, got %d", len(events))
	}
	evt := events[0]
	if evt.Type != "webhook.test" {
		t.Errorf("event type = %q, want %q", evt.Type, "webhook.test")
	}
	if evt.Source != "test" {
		t.Errorf("event source = %q, want %q", evt.Source, "test")
	}
	if evt.Payload["webhook_id"] != wh.WebhookID {
		t.Errorf("payload webhook_id = %v, want %q", evt.Payload["webhook_id"], wh.WebhookID)
	}
}

func TestWebhookHandler_TestDelivery_NotFound(t *testing.T) {
	mock := newMockWebhookStore()
	pub := &mockPublisher{}
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	h.SetPublisher(pub)
	srv := newWebhookRouter(h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/webhooks/nonexistent/test", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_TestDelivery_NoPublisher(t *testing.T) {
	mock := newMockWebhookStore()
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin")) // no publisher set
	srv := newWebhookRouter(h)
	defer srv.Close()

	wh := store.Webhook{
		WebhookID: uuid.New().String(),
		Name:      "no-pub-hook",
		URL:       "https://example.com/hook",
		Events:    []string{"run.*"},
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	mock.webhooks[wh.WebhookID] = &wh

	resp, err := http.Post(srv.URL+"/api/v1/webhooks/"+wh.WebhookID+"/test", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_ListWebhooks_StoreError(t *testing.T) {
	mock := newMockWebhookStore()
	mock.err = context.DeadlineExceeded
	h := handler.NewWebhookHandler(mock, testClaimsGetter("admin-1", "admin"))
	srv := newWebhookRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/webhooks")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}
