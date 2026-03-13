package handler_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kodrunhq/claude-plane/internal/server/event"
	"github.com/kodrunhq/claude-plane/internal/server/handler"
)

// mockPublisher is a thread-safe in-memory event.Publisher for testing.
type mockPublisher struct {
	mu     sync.Mutex
	events []event.Event
	err    error
}

func (m *mockPublisher) Publish(_ context.Context, evt event.Event) error {
	if m.err != nil {
		return m.err
	}
	m.mu.Lock()
	m.events = append(m.events, evt)
	m.mu.Unlock()
	return nil
}

func (m *mockPublisher) published() []event.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]event.Event, len(m.events))
	copy(out, m.events)
	return out
}

// buildIngestServer creates an httptest.Server wired with the IngestHandler.
func buildIngestServer(t *testing.T, pub event.Publisher, secrets map[string]string) *httptest.Server {
	t.Helper()
	h := handler.NewIngestHandler(pub, secrets, nil)
	r := chi.NewRouter()
	handler.RegisterIngestRoutes(r, h)
	return httptest.NewServer(r)
}

// githubSig computes the X-Hub-Signature-256 header value for body+secret.
func githubSig(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// postJSON sends a POST request to url with body and optional extra headers.
func postJSON(t *testing.T, url string, body []byte, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// TestIngest_ValidRequestWithCorrectSignature verifies that a correctly signed
// request returns 200 and publishes an event with the right type and source.
func TestIngest_ValidRequestWithCorrectSignature(t *testing.T) {
	const secret = "my-github-secret"
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": secret})
	defer srv.Close()

	body := []byte(`{"ref":"refs/heads/main","repository":{"name":"my-repo"}}`)
	sig := githubSig(body, secret)

	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/github", body, map[string]string{
		"X-Hub-Signature-256": sig,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != event.TypeTriggerWebhook {
		t.Errorf("expected event type %q, got %q", event.TypeTriggerWebhook, evt.Type)
	}
	if evt.Source != "webhook:github" {
		t.Errorf("expected source %q, got %q", "webhook:github", evt.Source)
	}
	if evt.Payload["ref"] != "refs/heads/main" {
		t.Errorf("expected payload.ref %q, got %v", "refs/heads/main", evt.Payload["ref"])
	}
}

// TestIngest_InvalidSignature verifies that a tampered signature returns 401
// and no event is published.
func TestIngest_InvalidSignature(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": "correct-secret"})
	defer srv.Close()

	body := []byte(`{"action":"opened"}`)
	badSig := githubSig(body, "wrong-secret")

	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/github", body, map[string]string{
		"X-Hub-Signature-256": badSig,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if len(pub.published()) != 0 {
		t.Error("no event should be published on invalid signature")
	}
}

// TestIngest_MissingSignatureHeader verifies that omitting the signature header
// when a secret is configured returns 401.
func TestIngest_MissingSignatureHeader(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": "some-secret"})
	defer srv.Close()

	body := []byte(`{"action":"closed"}`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/github", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestIngest_UnknownSource verifies that a request for an unconfigured source
// returns 404.
func TestIngest_UnknownSource(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": "secret"})
	defer srv.Close()

	body := []byte(`{"event":"push"}`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/unknown-source", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestIngest_ValidRequestWithoutSecret verifies that a source with an empty secret
// accepts any request without a signature header and publishes the event.
func TestIngest_ValidRequestWithoutSecret(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"generic": ""})
	defer srv.Close()

	body := []byte(`{"key":"value"}`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/generic", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}

	evt := events[0]
	if evt.Type != event.TypeTriggerWebhook {
		t.Errorf("expected event type %q, got %q", event.TypeTriggerWebhook, evt.Type)
	}
	if evt.Source != "webhook:generic" {
		t.Errorf("expected source %q, got %q", "webhook:generic", evt.Source)
	}
}

// TestIngest_EventPublishedWithCorrectPayload verifies that arbitrary JSON payload
// fields are forwarded as-is in the published event.
func TestIngest_EventPublishedWithCorrectPayload(t *testing.T) {
	const secret = "test-secret"
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"myapp": secret})
	defer srv.Close()

	body := []byte(`{"action":"created","id":42,"nested":{"x":true}}`)
	sig := githubSig(body, secret)

	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/myapp", body, map[string]string{
		"X-Hub-Signature-256": sig,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	events := pub.published()
	if len(events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(events))
	}

	payload := events[0].Payload
	if payload["action"] != "created" {
		t.Errorf("payload.action = %v, want %q", payload["action"], "created")
	}
	// JSON numbers decode as float64.
	if payload["id"] != float64(42) {
		t.Errorf("payload.id = %v, want 42", payload["id"])
	}
}

// TestIngest_InvalidJSON verifies that a non-JSON body returns 400.
func TestIngest_InvalidJSON(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"generic": ""})
	defer srv.Close()

	body := []byte(`not json at all`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/generic", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestIngest_MultipleSourcesIsolated verifies that multiple sources with different
// secrets are handled independently.
func TestIngest_MultipleSourcesIsolated(t *testing.T) {
	secrets := map[string]string{
		"github":  "github-secret",
		"gitlab":  "gitlab-secret",
		"generic": "",
	}
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, secrets)
	defer srv.Close()

	tests := []struct {
		source string
		secret string
	}{
		{"github", "github-secret"},
		{"gitlab", "gitlab-secret"},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"source":"%s"}`, tt.source))
			sig := githubSig(body, tt.secret)

			resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/"+tt.source, body, map[string]string{
				"X-Hub-Signature-256": sig,
			})
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200 for source %q, got %d", tt.source, resp.StatusCode)
			}
		})
	}
}

// TestIngest_MalformedSignatureHex verifies that a signature with invalid hex returns 401.
func TestIngest_MalformedSignatureHex(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": "secret"})
	defer srv.Close()

	body := []byte(`{"event":"push"}`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/github", body, map[string]string{
		"X-Hub-Signature-256": "sha256=notvalidhex!!!",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestIngest_MissingPrefixInSignature verifies that a signature without "sha256=" prefix returns 401.
func TestIngest_MissingPrefixInSignature(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"github": "secret"})
	defer srv.Close()

	body := []byte(`{"event":"push"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	rawHex := hex.EncodeToString(mac.Sum(nil))

	// Omit the "sha256=" prefix intentionally.
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/github", body, map[string]string{
		"X-Hub-Signature-256": rawHex,
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestIngest_ResponseBodyIsJSON verifies that the 200 response body is valid JSON.
func TestIngest_ResponseBodyIsJSON(t *testing.T) {
	pub := &mockPublisher{}
	srv := buildIngestServer(t, pub, map[string]string{"generic": ""})
	defer srv.Close()

	body := []byte(`{"hello":"world"}`)
	resp := postJSON(t, srv.URL+"/api/v1/webhooks/ingest/generic", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", result["status"])
	}
}

