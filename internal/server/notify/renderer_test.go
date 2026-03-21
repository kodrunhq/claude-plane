package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/event"
)

func TestTelegramEventRenderer_SessionStarted(t *testing.T) {
	e := event.Event{
		EventID:   "evt-1",
		Type:      event.TypeSessionStarted,
		Timestamp: time.Now(),
		Source:    "test",
		Payload: map[string]any{
			"session_id":   "sess-abc",
			"session_name": "my-session",
			"machine_name": "worker-1",
			"command":      "echo hello",
		},
	}

	subject, body := TelegramEventRenderer(e)

	if subject != "Session started" {
		t.Errorf("unexpected subject: %s", subject)
	}
	if !strings.Contains(body, "<b>Session started</b>") {
		t.Errorf("body should contain HTML bold tag, got: %s", body)
	}
	if !strings.Contains(body, "<code>worker-1</code>") {
		t.Errorf("body should contain machine name in code tag, got: %s", body)
	}
	if !strings.Contains(body, "<code>sess-abc</code>") {
		t.Errorf("body should contain session id in code tag, got: %s", body)
	}
	if !strings.Contains(body, "<code>my-session</code>") {
		t.Errorf("body should contain session name in code tag, got: %s", body)
	}
	if !strings.Contains(body, "<code>echo hello</code>") {
		t.Errorf("body should contain command in code tag, got: %s", body)
	}
}

func TestTelegramEventRenderer_BridgeStarted(t *testing.T) {
	e := event.Event{
		EventID:   "evt-2",
		Type:      event.TypeBridgeStarted,
		Timestamp: time.Now(),
		Source:    "bridge",
		Payload:   map[string]any{},
	}

	subject, body := TelegramEventRenderer(e)

	if subject != "Bridge started" {
		t.Errorf("unexpected subject: %s", subject)
	}
	if !strings.Contains(body, "<b>Bridge started</b>") {
		t.Errorf("body should contain HTML bold, got: %s", body)
	}
}

func TestTelegramEventRenderer_BridgeConnectorError(t *testing.T) {
	e := event.Event{
		EventID:   "evt-3",
		Type:      event.TypeBridgeConnectorError,
		Timestamp: time.Now(),
		Source:    "bridge",
		Payload: map[string]any{
			"connector_name": "github",
			"error":          "rate limited",
		},
	}

	subject, body := TelegramEventRenderer(e)

	if subject != "Bridge connector error" {
		t.Errorf("unexpected subject: %s", subject)
	}
	if !strings.Contains(body, "<code>github</code>") {
		t.Errorf("body should contain connector name, got: %s", body)
	}
	if !strings.Contains(body, "<code>rate limited</code>") {
		t.Errorf("body should contain error, got: %s", body)
	}
}

func TestTelegramEventRenderer_UnknownEvent(t *testing.T) {
	e := event.Event{
		EventID:   "evt-4",
		Type:      "custom.unknown",
		Timestamp: time.Now(),
		Source:    "test",
		Payload: map[string]any{
			"key": "value",
		},
	}

	subject, body := TelegramEventRenderer(e)

	if subject != "custom.unknown" {
		t.Errorf("unknown events should use type as subject, got: %s", subject)
	}
	if !strings.Contains(body, "<b>custom.unknown</b>") {
		t.Errorf("body should contain event type in bold, got: %s", body)
	}
	if !strings.Contains(body, "value") {
		t.Errorf("body should contain payload, got: %s", body)
	}
}

func TestTelegramEventRenderer_NoMarkdownV2Escapes(t *testing.T) {
	e := event.Event{
		EventID:   "evt-5",
		Type:      event.TypeSessionStarted,
		Timestamp: time.Now(),
		Source:    "test",
		Payload: map[string]any{
			"session_id":   "sess-123",
			"machine_name": "worker[1]",
			"command":      "echo *hello*",
		},
	}

	_, body := TelegramEventRenderer(e)

	mdv2Escapes := []string{"\\*", "\\[", "\\]", "\\(", "\\)", "\\~", "\\`", "\\>", "\\#", "\\+", "\\-", "\\=", "\\|", "\\{", "\\}", "\\.", "\\!"}
	for _, esc := range mdv2Escapes {
		if strings.Contains(body, esc) {
			t.Errorf("body should not contain MarkdownV2 escape %q, got: %s", esc, body)
		}
	}

	// Verify HTML escaping works for special chars
	if !strings.Contains(body, "worker[1]") {
		// [ is not an HTML special char, so it should pass through
		t.Errorf("body should contain literal brackets, got: %s", body)
	}
}

func TestTelegramEventRenderer_HTMLEscaping(t *testing.T) {
	e := event.Event{
		EventID:   "evt-6",
		Type:      event.TypeMachineConnected,
		Timestamp: time.Now(),
		Source:    "test",
		Payload: map[string]any{
			"display_name": "<script>alert('xss')</script>",
		},
	}

	_, body := TelegramEventRenderer(e)

	if strings.Contains(body, "<script>") {
		t.Errorf("body should HTML-escape user values, got: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("body should contain escaped HTML entities, got: %s", body)
	}
}

func TestDefaultEventRenderer_StillWorks(t *testing.T) {
	e := event.Event{
		EventID:   "evt-7",
		Type:      "test.event",
		Timestamp: time.Now(),
		Source:    "test",
		Payload: map[string]any{
			"key1": "value1",
			"key2": "value2",
		},
	}

	subject, body := DefaultEventRenderer(e)

	if subject != "test.event" {
		t.Errorf("DefaultEventRenderer subject should be event type, got: %s", subject)
	}
	if !strings.Contains(body, "key1: value1") {
		t.Errorf("DefaultEventRenderer body should contain payload, got: %s", body)
	}
	if !strings.Contains(body, "key2: value2") {
		t.Errorf("DefaultEventRenderer body should contain payload, got: %s", body)
	}
}

func TestTelegramEventRenderer_EventRendererType(t *testing.T) {
	// Verify TelegramEventRenderer satisfies the EventRenderer type.
	var r EventRenderer = TelegramEventRenderer
	_ = r

	// Also verify DefaultEventRenderer satisfies it.
	var r2 EventRenderer = DefaultEventRenderer
	_ = r2
}
