package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/store"
)

// --- RateLimiter tests ---

func TestRateLimiter_AllowFirstDenySecond(t *testing.T) {
	rl := NewRateLimiter(60 * time.Second)
	if !rl.Allow("ch1", "run.completed") {
		t.Error("first call should be allowed")
	}
	if rl.Allow("ch1", "run.completed") {
		t.Error("second call within interval should be denied")
	}
}

func TestRateLimiter_AllowDifferentKeys(t *testing.T) {
	rl := NewRateLimiter(60 * time.Second)
	if !rl.Allow("ch1", "run.completed") {
		t.Error("ch1/run.completed should be allowed")
	}
	if !rl.Allow("ch1", "run.failed") {
		t.Error("ch1/run.failed should be allowed (different event type)")
	}
	if !rl.Allow("ch2", "run.completed") {
		t.Error("ch2/run.completed should be allowed (different channel)")
	}
}

func TestRateLimiter_AllowAfterInterval(t *testing.T) {
	rl := NewRateLimiter(100 * time.Millisecond)

	// Override nowFn to simulate time progression
	now := time.Now()
	rl.nowFn = func() time.Time { return now }

	if !rl.Allow("ch1", "run.completed") {
		t.Error("first call should be allowed")
	}
	if rl.Allow("ch1", "run.completed") {
		t.Error("second call should be denied")
	}

	// Advance time past the interval
	now = now.Add(200 * time.Millisecond)
	if !rl.Allow("ch1", "run.completed") {
		t.Error("should be allowed after interval elapsed")
	}
}

// --- Template tests ---

func TestRenderEmail(t *testing.T) {
	data := EmailData{
		Subject:   "Run Completed",
		Fields:    []KeyValue{{Key: "job_id", Value: "abc123"}, {Key: "status", Value: "success"}},
		Timestamp: "2026-03-17T12:00:00Z",
		AppURL:    "https://example.com/runs/abc123",
	}

	html, err := RenderEmail(data)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}

	if !strings.Contains(html, "Run Completed") {
		t.Error("expected subject in output")
	}
	if !strings.Contains(html, "job_id") {
		t.Error("expected field key in output")
	}
	if !strings.Contains(html, "abc123") {
		t.Error("expected field value in output")
	}
	if !strings.Contains(html, "View in App") {
		t.Error("expected View in App link")
	}
}

func TestRenderEmail_NoAppURL(t *testing.T) {
	data := EmailData{
		Subject:   "Test",
		Fields:    []KeyValue{{Key: "k", Value: "v"}},
		Timestamp: "now",
	}

	html, err := RenderEmail(data)
	if err != nil {
		t.Fatalf("RenderEmail: %v", err)
	}

	if strings.Contains(html, "View in App") {
		t.Error("should not contain View in App when AppURL is empty")
	}
}

// --- Telegram notifier test ---

func TestTelegramNotifier_Send(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n := newTelegramNotifierWithBaseURL(srv.Client(), srv.URL)

	cfg := `{"bot_token":"test-token","chat_id":"12345","topic_id":42}`
	err := n.Send(context.Background(), cfg, "Test Subject", "Test Body")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if received["chat_id"] != "12345" {
		t.Errorf("chat_id = %v, want 12345", received["chat_id"])
	}
	if received["message_thread_id"] != float64(42) {
		t.Errorf("message_thread_id = %v, want 42", received["message_thread_id"])
	}
}

func TestTelegramNotifier_SendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	n := newTelegramNotifierWithBaseURL(srv.Client(), srv.URL)

	cfg := `{"bot_token":"bad-token","chat_id":"12345"}`
	err := n.Send(context.Background(), cfg, "Test", "Body")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestTelegramNotifier_InvalidConfig(t *testing.T) {
	n := NewTelegramNotifier(nil)
	err := n.Send(context.Background(), "not-json", "Test", "Body")
	if err == nil {
		t.Error("expected error for invalid JSON config")
	}
}

// --- Dispatcher tests ---

// mockNotifier records calls for testing.
type mockNotifier struct {
	mu    sync.Mutex
	calls []mockNotifierCall
	err   error
}

type mockNotifierCall struct {
	Config  string
	Subject string
	Body    string
}

func (m *mockNotifier) Type() string { return "mock" }

func (m *mockNotifier) Send(_ context.Context, config, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockNotifierCall{Config: config, Subject: subject, Body: body})
	return m.err
}

func (m *mockNotifier) getCalls() []mockNotifierCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]mockNotifierCall, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// mockSubStore implements SubscriptionStore for testing.
type mockSubStore struct {
	subs []store.ChannelSubscription
	err  error
}

func (m *mockSubStore) ListSubscriptionsForEvent(_ context.Context, _ string) ([]store.ChannelSubscription, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.subs, nil
}

func TestDispatcher_CallsCorrectNotifier(t *testing.T) {
	emailNotifier := &mockNotifier{}
	telegramNotifier := &mockNotifier{}

	subStore := &mockSubStore{
		subs: []store.ChannelSubscription{
			{ChannelID: "ch1", ChannelType: "email", Config: `{"host":"smtp"}`},
			{ChannelID: "ch2", ChannelType: "telegram", Config: `{"bot_token":"tok"}`},
		},
	}

	d := NewDispatcher(subStore, nil, map[string]Notifier{
		"email":    emailNotifier,
		"telegram": telegramNotifier,
	}, nil, DefaultEventRenderer, nil)

	handler := d.Handler()
	evt := event.Event{
		EventID: "evt-1",
		Type:    "run.completed",
		Payload: map[string]any{"run_id": "r1"},
	}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}

	emailCalls := emailNotifier.getCalls()
	if len(emailCalls) != 1 {
		t.Fatalf("expected 1 email call, got %d", len(emailCalls))
	}
	if emailCalls[0].Config != `{"host":"smtp"}` {
		t.Errorf("email config = %q", emailCalls[0].Config)
	}

	telegramCalls := telegramNotifier.getCalls()
	if len(telegramCalls) != 1 {
		t.Fatalf("expected 1 telegram call, got %d", len(telegramCalls))
	}
}

func TestDispatcher_RateLimiting(t *testing.T) {
	notifier := &mockNotifier{}

	subStore := &mockSubStore{
		subs: []store.ChannelSubscription{
			{ChannelID: "ch1", ChannelType: "mock", Config: "{}"},
		},
	}

	d := NewDispatcher(subStore, nil, map[string]Notifier{
		"mock": notifier,
	}, nil, DefaultEventRenderer, nil)

	handler := d.Handler()
	evt := event.Event{EventID: "evt-1", Type: "run.completed", Payload: map[string]any{}}

	// First call should go through
	_ = handler(context.Background(), evt)
	// Second call should be rate-limited
	_ = handler(context.Background(), evt)

	calls := notifier.getCalls()
	if len(calls) != 1 {
		t.Errorf("expected 1 call (rate-limited), got %d", len(calls))
	}
}

func TestDispatcher_UnknownChannelType(t *testing.T) {
	subStore := &mockSubStore{
		subs: []store.ChannelSubscription{
			{ChannelID: "ch1", ChannelType: "unknown", Config: "{}"},
		},
	}

	d := NewDispatcher(subStore, nil, map[string]Notifier{}, nil, DefaultEventRenderer, nil)

	handler := d.Handler()
	evt := event.Event{EventID: "evt-1", Type: "run.completed", Payload: map[string]any{}}

	// Should not panic or error
	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestDispatcher_NoSubscriptions(t *testing.T) {
	subStore := &mockSubStore{subs: nil}

	d := NewDispatcher(subStore, nil, map[string]Notifier{}, nil, DefaultEventRenderer, nil)

	handler := d.Handler()
	evt := event.Event{EventID: "evt-1", Type: "run.completed", Payload: map[string]any{}}

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestDefaultEventRenderer(t *testing.T) {
	evt := event.Event{
		Type: "run.completed",
		Payload: map[string]any{
			"run_id": "r1",
			"status": "success",
		},
	}

	subject, body := DefaultEventRenderer(evt)
	if subject != "run.completed" {
		t.Errorf("subject = %q, want %q", subject, "run.completed")
	}
	if !strings.Contains(body, "run_id: r1") {
		t.Errorf("body missing run_id, got: %s", body)
	}
	if !strings.Contains(body, "status: success") {
		t.Errorf("body missing status, got: %s", body)
	}
}

// --- SMTPNotifier validation tests ---

func TestSMTPNotifier_Send_InvalidConfig(t *testing.T) {
	n := &SMTPNotifier{}
	err := n.Send(context.Background(), "not-json", "subj", "body")
	if err == nil {
		t.Fatal("expected error for invalid JSON config")
	}
}

func TestSMTPNotifier_Send_MissingHost(t *testing.T) {
	n := &SMTPNotifier{}
	cfg := `{"port":587,"from":"a@b.com","to":"c@d.com"}`
	err := n.Send(context.Background(), cfg, "subj", "body")
	if err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected host error, got: %v", err)
	}
}

func TestSMTPNotifier_Send_InvalidPort(t *testing.T) {
	n := &SMTPNotifier{}
	cfg := `{"host":"smtp.example.com","port":0,"from":"a@b.com","to":"c@d.com"}`
	err := n.Send(context.Background(), cfg, "subj", "body")
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected port error, got: %v", err)
	}
}

func TestSMTPNotifier_Send_CRLFInjection(t *testing.T) {
	n := &SMTPNotifier{}
	cfg := `{"host":"smtp.example.com","port":587,"from":"a@b.com\r\nBcc: evil@bad.com","to":"c@d.com"}`
	err := n.Send(context.Background(), cfg, "subj", "body")
	if err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected CRLF injection error, got: %v", err)
	}
}
