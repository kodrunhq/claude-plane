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
	"github.com/kodrunhq/claude-plane/internal/server/notify"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- mock notification store ---

type mockNotificationStore struct {
	channels      map[string]*store.NotificationChannel
	subscriptions map[string][]store.NotificationSubscription // keyed by user_id
	err           error
}

func newMockNotificationStore() *mockNotificationStore {
	return &mockNotificationStore{
		channels:      make(map[string]*store.NotificationChannel),
		subscriptions: make(map[string][]store.NotificationSubscription),
	}
}

func (m *mockNotificationStore) CreateNotificationChannel(_ context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch.ChannelID = uuid.New().String()
	ch.CreatedAt = time.Now().UTC()
	ch.UpdatedAt = ch.CreatedAt
	cp := ch
	m.channels[ch.ChannelID] = &cp
	return &cp, nil
}

func (m *mockNotificationStore) GetNotificationChannel(_ context.Context, id string) (*store.NotificationChannel, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch, ok := m.channels[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *ch
	return &cp, nil
}

func (m *mockNotificationStore) ListNotificationChannels(_ context.Context) ([]store.NotificationChannel, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]store.NotificationChannel, 0, len(m.channels))
	for _, ch := range m.channels {
		result = append(result, *ch)
	}
	return result, nil
}

func (m *mockNotificationStore) UpdateNotificationChannel(_ context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error) {
	if m.err != nil {
		return nil, m.err
	}
	existing, ok := m.channels[ch.ChannelID]
	if !ok {
		return nil, store.ErrNotFound
	}
	ch.CreatedAt = existing.CreatedAt
	ch.CreatedBy = existing.CreatedBy
	ch.ChannelType = existing.ChannelType
	ch.UpdatedAt = time.Now().UTC()
	cp := ch
	m.channels[ch.ChannelID] = &cp
	return &cp, nil
}

func (m *mockNotificationStore) DeleteNotificationChannel(_ context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.channels[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.channels, id)
	return nil
}

func (m *mockNotificationStore) GetSubscriptions(_ context.Context, userID string) ([]store.NotificationSubscription, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.subscriptions[userID], nil
}

func (m *mockNotificationStore) SetSubscriptions(_ context.Context, userID string, subs []store.NotificationSubscription) error {
	if m.err != nil {
		return m.err
	}
	m.subscriptions[userID] = subs
	return nil
}

// --- mock notifier ---

type testNotifier struct {
	sentCount int
	lastErr   error
}

func (n *testNotifier) Type() string { return "email" }
func (n *testNotifier) Send(_ context.Context, _, _, _ string) error {
	n.sentCount++
	return n.lastErr
}

// --- helpers ---

func claimsGetter(userID string) handler.ClaimsGetter {
	return func(_ *http.Request) *handler.UserClaims {
		if userID == "" {
			return nil
		}
		return &handler.UserClaims{UserID: userID, Role: "admin"}
	}
}

func newNotificationRouter(h *handler.NotificationHandler) *httptest.Server {
	r := chi.NewRouter()
	handler.RegisterNotificationRoutes(r, h)
	return httptest.NewServer(r)
}

// --- tests ---

func TestNotificationHandler_ListChannels_Empty(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/notification-channels")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result []store.NotificationChannel
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 channels, got %d", len(result))
	}
}

func TestNotificationHandler_CreateChannel(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	body := map[string]any{
		"channel_type": "email",
		"name":         "Team Email",
		"config":       `{"host":"smtp.example.com"}`,
	}

	resp, err := http.Post(srv.URL+"/api/v1/notification-channels", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var created store.NotificationChannel
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ChannelID == "" {
		t.Error("expected non-empty channel_id")
	}
	if created.Name != "Team Email" {
		t.Errorf("Name = %q, want %q", created.Name, "Team Email")
	}
}

func TestNotificationHandler_CreateChannel_MissingName(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	body := map[string]any{
		"channel_type": "email",
		"config":       `{"host":"smtp"}`,
	}

	resp, err := http.Post(srv.URL+"/api/v1/notification-channels", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_CreateChannel_InvalidType(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	body := map[string]any{
		"channel_type": "slack",
		"name":         "test",
		"config":       `{}`,
	}

	resp, err := http.Post(srv.URL+"/api/v1/notification-channels", "application/json", toJSON(t, body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_GetChannel(t *testing.T) {
	mock := newMockNotificationStore()
	ch := &store.NotificationChannel{
		ChannelID:   uuid.New().String(),
		ChannelType: "email",
		Name:        "test",
		Config:      "{}",
		Enabled:     true,
		CreatedBy:   "user-1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	mock.channels[ch.ChannelID] = ch

	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/notification-channels/" + ch.ChannelID)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_GetChannel_NotFound(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/notification-channels/nonexistent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_UpdateChannel(t *testing.T) {
	mock := newMockNotificationStore()
	ch := &store.NotificationChannel{
		ChannelID:   uuid.New().String(),
		ChannelType: "email",
		Name:        "old",
		Config:      "{}",
		Enabled:     true,
		CreatedBy:   "user-1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	mock.channels[ch.ChannelID] = ch

	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	enabled := false
	body := map[string]any{
		"name":    "new-name",
		"config":  `{"host":"new-smtp"}`,
		"enabled": enabled,
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/notification-channels/"+ch.ChannelID, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var updated store.NotificationChannel
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.Name != "new-name" {
		t.Errorf("Name = %q, want %q", updated.Name, "new-name")
	}
}

func TestNotificationHandler_DeleteChannel(t *testing.T) {
	mock := newMockNotificationStore()
	ch := &store.NotificationChannel{
		ChannelID:   uuid.New().String(),
		ChannelType: "email",
		Name:        "to-delete",
		Config:      "{}",
		Enabled:     true,
		CreatedBy:   "user-1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	mock.channels[ch.ChannelID] = ch

	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/notification-channels/"+ch.ChannelID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_DeleteChannel_NotFound(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/notification-channels/ghost", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestNotificationHandler_TestChannel(t *testing.T) {
	mock := newMockNotificationStore()
	ch := &store.NotificationChannel{
		ChannelID:   uuid.New().String(),
		ChannelType: "email",
		Name:        "test-ch",
		Config:      `{"host":"smtp"}`,
		Enabled:     true,
		CreatedBy:   "user-1",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	mock.channels[ch.ChannelID] = ch

	tn := &testNotifier{}
	notifiers := map[string]notify.Notifier{"email": tn}

	h := handler.NewNotificationHandler(mock, notifiers, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/notification-channels/"+ch.ChannelID+"/test", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	if tn.sentCount != 1 {
		t.Errorf("expected 1 send call, got %d", tn.sentCount)
	}
}

func TestNotificationHandler_GetSubscriptions(t *testing.T) {
	mock := newMockNotificationStore()
	mock.subscriptions["user-1"] = []store.NotificationSubscription{
		{UserID: "user-1", ChannelID: "ch-1", EventType: "run.completed"},
	}

	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/notifications/subscriptions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var subs []store.NotificationSubscription
	if err := json.NewDecoder(resp.Body).Decode(&subs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(subs) != 1 {
		t.Errorf("expected 1 subscription, got %d", len(subs))
	}
}

func TestNotificationHandler_SetSubscriptions(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	body := map[string]any{
		"subscriptions": []map[string]any{
			{"channel_id": "ch-1", "event_type": "run.completed"},
			{"channel_id": "ch-2", "event_type": "run.failed"},
		},
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/notifications/subscriptions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", resp.StatusCode)
	}

	subs := mock.subscriptions["user-1"]
	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions saved, got %d", len(subs))
	}
}

func TestNotificationHandler_SetSubscriptions_InvalidEntry(t *testing.T) {
	mock := newMockNotificationStore()
	h := handler.NewNotificationHandler(mock, nil, claimsGetter("user-1"))
	srv := newNotificationRouter(h)
	defer srv.Close()

	body := map[string]any{
		"subscriptions": []map[string]any{
			{"channel_id": "", "event_type": "run.completed"},
		},
	}
	b, _ := json.Marshal(body)

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/notifications/subscriptions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}
