# Unified Telegram & Bridge Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify the duplicate Telegram notification systems (bridge connector + notification channel) into a single config, add bridge log forwarding to the server, and add bridge lifecycle events — all with improved frontend UX.

**Architecture:** The bridge connector owns the Telegram connection (bot token, chat/group IDs). The server notification system uses it as a delivery target via a `ConnectorResolver` interface. Event delivery moves from bridge polling to server-side event bus dispatch. The bridge becomes a command-only listener for Telegram. A new `BridgeIngestHandler` receives bridge logs and lifecycle events via REST, publishing them through the existing log store and event bus.

**Tech Stack:** Go 1.25, React 19, TypeScript, SQLite, slog, Chi router, TanStack Query, Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-unified-telegram-and-bridge-logging-design.md`

---

## File Map

### New Files (Server)

| File | Responsibility |
|------|---------------|
| `internal/server/handler/bridge_ingest.go` | `BridgeIngestHandler` — `POST /api/v1/ingest/logs` and `POST /api/v1/ingest/events` |
| `internal/server/handler/bridge_ingest_test.go` | Tests for log/event ingestion endpoints |
| `internal/server/notify/connector_resolver.go` | `ConnectorResolver` interface definition |
| `internal/server/notify/renderer.go` | `EventRenderer` type + `TelegramEventRenderer` (ported from bridge `FormatEvent`) |
| `internal/server/notify/renderer_test.go` | Tests for `TelegramEventRenderer` |
| `internal/server/store/connector_resolver.go` | `ConnectorResolver` implementation using store + encryption key |
| `internal/server/store/connector_resolver_test.go` | Tests for connector config resolution |

### New Files (Bridge)

| File | Responsibility |
|------|---------------|
| `internal/bridge/log_forwarder.go` | `slog.Handler` that buffers and ships logs to server via REST |
| `internal/bridge/log_forwarder_test.go` | Tests for log forwarder buffer/flush behavior |
| `internal/bridge/event_emitter.go` | Emits bridge lifecycle events to server via REST |
| `internal/bridge/event_emitter_test.go` | Tests for event emitter |

### New Files (Frontend)

| File | Responsibility |
|------|---------------|
| (none — `ConnectorDetailPage.tsx` already exists, will be modified) | |
| `web/src/hooks/useBridgeStatus.ts` | TanStack Query hook for `GET /api/v1/bridge/status` |

### Modified Files (Server)

| File | Changes |
|------|---------|
| `internal/server/event/event_types.json` | Add 5 bridge event types |
| `internal/server/event/builders.go` | Add `NewBridgeEvent` exported builder for bridge lifecycle events |
| `internal/server/store/migrations.go` | Migration v20: `connector_id` column on `notification_channels` |
| `internal/server/store/notifications.go` | `ConnectorID` field on `NotificationChannel` + `ChannelSubscription`, update all Scan calls, add `GetChannelByConnectorID` |
| `internal/server/notify/dispatcher.go` | Add `ConnectorResolver` + `renderers` map dependencies, resolve connector at dispatch time |
| `internal/server/handler/bridge.go` | Auto-sync notification channel on Telegram connector CRUD, add `NotifStore` interface |
| `cmd/server/main.go` | Wire `BridgeIngestHandler`, `ConnectorResolver`, renderer map, updated `Dispatcher` constructor, register ingest routes |

### Modified Files (Bridge)

| File | Changes |
|------|---------|
| `internal/bridge/connector/telegram/telegram.go` | Remove `pollAndForwardEvents`, add `commands_enabled` config field, remove `event_types` |
| `internal/bridge/connector/telegram/formatter.go` | Remove `ShouldForwardEvent`, `MatchEventType` (keep `FormatEvent` until server port confirmed working) |
| `internal/bridge/client/client.go` | Add `PostLogs()` and `PostEvents()` methods |
| `cmd/bridge/main.go` | Wire log forwarder as slog handler, wire event emitter, emit lifecycle events |

### Modified Files (Frontend)

| File | Changes |
|------|---------|
| `web/src/types/notification.ts` | Add `connector_id?: string` to `NotificationChannel` |
| `web/src/constants/eventTypes.ts` | Add bridge event constants + "Bridge" group in `EVENT_GROUPS` |
| `web/src/components/connectors/TelegramForm.tsx` | Remove `event_types` textbox, add `commands_enabled` toggle |
| `web/src/components/settings/NotificationsTab.tsx` | Connector-backed channels show badge, non-editable; info banner; "Add Channel" email-only |
| `web/src/components/settings/ChannelFormModal.tsx` | Remove Telegram tab entirely |
| `web/src/views/LogsPage.tsx` | Add source filter dropdown |
| `web/src/views/ConnectorDetailPage.tsx` | Extend for Telegram: commands list, notifications link, bridge status |
| `web/src/views/CommandCenter.tsx` | Add bridge status indicator |

---

## Task 1: Database Migration — `connector_id` Column

**Files:**
- Modify: `internal/server/store/migrations.go` (append after line ~545, migration v20)

- [ ] **Step 1: Add migration v20**

In `internal/server/store/migrations.go`, append to the `migrations` slice:

```go
{
    Version:     20,
    Description: "add connector_id to notification_channels",
    SQL:         `ALTER TABLE notification_channels ADD COLUMN connector_id TEXT NULL;`,
},
```

- [ ] **Step 2: Run existing tests to verify migration doesn't break anything**

Run: `go test -race ./internal/server/store/ -run TestMigrations -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: add connector_id column to notification_channels (migration v20)"
```

---

## Task 2: Register Bridge Event Types

**Files:**
- Modify: `internal/server/event/event_types.json`
- Modify: `web/src/constants/eventTypes.ts`

- [ ] **Step 1: Add bridge event types to event_types.json**

Append to the array in `internal/server/event/event_types.json` before the closing `]`:

```json
{
    "name": "TypeBridgeStarted",
    "value": "bridge.started"
},
{
    "name": "TypeBridgeStopped",
    "value": "bridge.stopped"
},
{
    "name": "TypeBridgeConnectorStarted",
    "value": "bridge.connector.started"
},
{
    "name": "TypeBridgeConnectorError",
    "value": "bridge.connector.error"
},
{
    "name": "TypeBridgeConnectorCommand",
    "value": "bridge.connector.command"
}
```

- [ ] **Step 2: Regenerate Go event type constants**

Run: `go generate ./internal/server/event/...`
Expected: `event_types_gen.go` updated with new constants

- [ ] **Step 2b: Add NewBridgeEvent builder to builders.go**

In `internal/server/event/builders.go`, add an exported builder following the existing pattern (`newEvent` is unexported):

```go
// NewBridgeEvent constructs an event for bridge lifecycle changes.
// eventType should be one of the TypeBridge* constants.
func NewBridgeEvent(eventType string, payload map[string]any) Event {
    return newEvent(eventType, "bridge", payload)
}
```

- [ ] **Step 3: Add bridge event types to frontend constants**

In `web/src/constants/eventTypes.ts`, add the new constants to the constants section, add them to `ALL_EVENT_TYPES`, and add a "Bridge" group to `EVENT_GROUPS`:

```typescript
// Bridge events
export const BRIDGE_STARTED = 'bridge.started' as const;
export const BRIDGE_STOPPED = 'bridge.stopped' as const;
export const BRIDGE_CONNECTOR_STARTED = 'bridge.connector.started' as const;
export const BRIDGE_CONNECTOR_ERROR = 'bridge.connector.error' as const;
export const BRIDGE_CONNECTOR_COMMAND = 'bridge.connector.command' as const;
```

Add to `ALL_EVENT_TYPES` array:
```typescript
BRIDGE_STARTED,
BRIDGE_STOPPED,
BRIDGE_CONNECTOR_STARTED,
BRIDGE_CONNECTOR_ERROR,
BRIDGE_CONNECTOR_COMMAND,
```

Add to `EVENT_GROUPS` array:
```typescript
{
    label: 'Bridge',
    events: [
        BRIDGE_STARTED,
        BRIDGE_STOPPED,
        BRIDGE_CONNECTOR_STARTED,
        BRIDGE_CONNECTOR_ERROR,
        BRIDGE_CONNECTOR_COMMAND,
    ],
},
```

- [ ] **Step 4: Verify frontend types compile**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/server/event/event_types.json internal/server/event/event_types_gen.go web/src/constants/eventTypes.ts
git commit -m "feat: register bridge lifecycle event types"
```

---

## Task 3: Update Notification Store — `ConnectorID` Field

**Files:**
- Modify: `internal/server/store/notifications.go`
- Modify: `web/src/types/notification.ts`

- [ ] **Step 1: Add ConnectorID to Go structs**

In `internal/server/store/notifications.go`:

Add `ConnectorID *string` to `NotificationChannel` struct (after `UpdatedAt`):
```go
type NotificationChannel struct {
    ChannelID   string
    ChannelType string
    Name        string
    Config      string
    Enabled     bool
    CreatedBy   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
    ConnectorID *string
}
```

Add `ConnectorID *string` to `ChannelSubscription` struct:
```go
type ChannelSubscription struct {
    ChannelID   string
    ChannelType string
    Config      string
    ConnectorID *string
}
```

- [ ] **Step 2: Update all Scan calls for NotificationChannel**

Find every query that scans into `NotificationChannel` and add `&ch.ConnectorID` to the Scan. There are at least 4: `CreateChannel`, `GetChannel`, `ListChannels`, and `UpdateChannel`. Each SELECT and INSERT/RETURNING must include `connector_id`.

For `ListSubscriptionsForEvent`, update the SQL and Scan:

```go
// SQL change: add nc.connector_id to SELECT
query := `SELECT DISTINCT nc.channel_id, nc.channel_type, nc.config, nc.connector_id
    FROM notification_subscriptions ns
    JOIN notification_channels nc ON ns.channel_id = nc.channel_id
    WHERE ns.event_type = ? AND nc.enabled = 1`

// Scan change: add &sub.ConnectorID
rows.Scan(&sub.ChannelID, &sub.ChannelType, &sub.Config, &sub.ConnectorID)
```

- [ ] **Step 3: Add connector_id to TypeScript type**

In `web/src/types/notification.ts`, add to `NotificationChannel`:
```typescript
connector_id?: string;
```

- [ ] **Step 4: Add GetChannelByConnectorID query**

In `internal/server/store/notifications.go`, add:

```go
func (s *Store) GetChannelByConnectorID(ctx context.Context, connectorID string) (*NotificationChannel, error) {
    row := s.reader.QueryRowContext(ctx,
        `SELECT channel_id, channel_type, name, config, enabled, created_by, created_at, updated_at, connector_id
         FROM notification_channels WHERE connector_id = ?`, connectorID)
    var ch NotificationChannel
    err := row.Scan(&ch.ChannelID, &ch.ChannelType, &ch.Name, &ch.Config, &ch.Enabled,
        &ch.CreatedBy, &ch.CreatedAt, &ch.UpdatedAt, &ch.ConnectorID)
    if err != nil {
        return nil, err
    }
    return &ch, nil
}
```

- [ ] **Step 5: Run Go tests**

Run: `go test -race ./internal/server/store/ -run TestNotification -v`
Expected: PASS (existing tests should still pass — connector_id is nullable)

- [ ] **Step 6: Commit**

```bash
git add internal/server/store/notifications.go web/src/types/notification.ts
git commit -m "feat: add connector_id field to notification channel model"
```

---

## Task 4: ConnectorResolver Interface and Implementation

**Files:**
- Create: `internal/server/notify/connector_resolver.go`
- Create: `internal/server/store/connector_resolver.go`
- Create: `internal/server/store/connector_resolver_test.go`

- [ ] **Step 1: Write the ConnectorResolver interface**

Create `internal/server/notify/connector_resolver.go`:

```go
package notify

import "context"

// ConnectorResolver resolves a bridge connector ID to a merged notification
// config JSON (bot_token, chat_id, topic_id). Handles secret decryption internally.
type ConnectorResolver interface {
    ResolveConnectorConfig(ctx context.Context, connectorID string) (string, error)
}
```

- [ ] **Step 2: Write the failing test for the store implementation**

Create `internal/server/store/connector_resolver_test.go`:

```go
package store

import (
    "context"
    "encoding/json"
    "testing"
)

func TestConnectorConfigResolver_ResolveConnectorConfig(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    // Create a Telegram connector with secret
    connector := &BridgeConnector{
        ConnectorID:   "test-conn-1",
        ConnectorType: "telegram",
        Name:          "test-telegram",
        Enabled:       true,
        Config:        `{"group_id":-1001234,"events_topic_id":5,"commands_topic_id":6,"commands_enabled":true}`,
        CreatedBy:     "admin",
    }
    encKey := make([]byte, 32) // zero key for testing
    secretJSON := []byte(`{"bot_token":"123:ABC"}`)
    if _, err := s.CreateConnector(ctx, connector, secretJSON, encKey); err != nil {
        t.Fatalf("create connector: %v", err)
    }

    resolver := NewConnectorConfigResolver(s, encKey)
    configJSON, err := resolver.ResolveConnectorConfig(ctx, "test-conn-1")
    if err != nil {
        t.Fatalf("resolve: %v", err)
    }

    var cfg struct {
        BotToken string `json:"bot_token"`
        ChatID   string `json:"chat_id"`
        TopicID  int    `json:"topic_id"`
    }
    if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    if cfg.BotToken != "123:ABC" {
        t.Errorf("bot_token = %q, want %q", cfg.BotToken, "123:ABC")
    }
    if cfg.ChatID != "-1001234" {
        t.Errorf("chat_id = %q, want %q", cfg.ChatID, "-1001234")
    }
    if cfg.TopicID != 5 {
        t.Errorf("topic_id = %d, want %d", cfg.TopicID, 5)
    }
}

func TestConnectorConfigResolver_NotFound(t *testing.T) {
    s := newTestStore(t)
    encKey := make([]byte, 32)
    resolver := NewConnectorConfigResolver(s, encKey)

    _, err := resolver.ResolveConnectorConfig(context.Background(), "nonexistent")
    if err == nil {
        t.Fatal("expected error for nonexistent connector")
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -race ./internal/server/store/ -run TestConnectorConfigResolver -v`
Expected: FAIL — `NewConnectorConfigResolver` not defined

- [ ] **Step 4: Write the implementation**

Create `internal/server/store/connector_resolver.go`:

```go
package store

import (
    "context"
    "encoding/json"
    "fmt"
    "strconv"
)

// ConnectorConfigResolver implements notify.ConnectorResolver using the store
// layer. It handles secret decryption internally, keeping the encryption key
// out of the notify package.
type ConnectorConfigResolver struct {
    store  *Store
    encKey []byte
}

// NewConnectorConfigResolver creates a resolver that can fetch and decrypt
// bridge connector configs for use as notification channel configs.
func NewConnectorConfigResolver(store *Store, encKey []byte) *ConnectorConfigResolver {
    return &ConnectorConfigResolver{store: store, encKey: encKey}
}

// ResolveConnectorConfig fetches a bridge connector, decrypts its secret, and
// returns a merged JSON config suitable for TelegramNotifier.Send().
// Output format: {"bot_token":"...","chat_id":"...","topic_id":N}
func (r *ConnectorConfigResolver) ResolveConnectorConfig(ctx context.Context, connectorID string) (string, error) {
    connector, secretJSON, err := r.store.GetConnectorWithSecret(ctx, connectorID, r.encKey)
    if err != nil {
        return "", fmt.Errorf("get connector %s: %w", connectorID, err)
    }

    // Parse public config for group_id and events_topic_id
    var connCfg struct {
        GroupID       int64 `json:"group_id"`
        EventsTopicID int  `json:"events_topic_id"`
    }
    if err := json.Unmarshal([]byte(connector.Config), &connCfg); err != nil {
        return "", fmt.Errorf("parse connector config: %w", err)
    }

    // Parse secret for bot_token
    var secret struct {
        BotToken string `json:"bot_token"`
    }
    if secretJSON != nil {
        if err := json.Unmarshal(secretJSON, &secret); err != nil {
            return "", fmt.Errorf("parse connector secret: %w", err)
        }
    }

    // Build merged config matching notify.TelegramConfig format
    merged := map[string]any{
        "bot_token": secret.BotToken,
        "chat_id":   strconv.FormatInt(connCfg.GroupID, 10),
    }
    if connCfg.EventsTopicID > 0 {
        merged["topic_id"] = connCfg.EventsTopicID
    }

    out, err := json.Marshal(merged)
    if err != nil {
        return "", fmt.Errorf("marshal merged config: %w", err)
    }
    return string(out), nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -race ./internal/server/store/ -run TestConnectorConfigResolver -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/notify/connector_resolver.go internal/server/store/connector_resolver.go internal/server/store/connector_resolver_test.go
git commit -m "feat: add ConnectorResolver for Telegram notification delivery"
```

---

## Task 5: EventRenderer Type and TelegramEventRenderer

**Files:**
- Create: `internal/server/notify/renderer.go`
- Create: `internal/server/notify/renderer_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/notify/renderer_test.go`:

```go
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
        Payload: map[string]any{
            "session_id":  "sess-123",
            "session_name": "my-session",
            "machine_id":  "worker-1",
        },
    }
    subject, body := TelegramEventRenderer(e)
    if subject == "" {
        t.Error("subject should not be empty")
    }
    if !strings.Contains(body, "my-session") {
        t.Error("body should contain session name")
    }
    // Must be HTML, not MarkdownV2
    if strings.Contains(body, "\\*") || strings.Contains(body, "\\[") {
        t.Error("body should be HTML, not MarkdownV2")
    }
}

func TestTelegramEventRenderer_UnknownEvent(t *testing.T) {
    e := event.Event{
        EventID:   "evt-2",
        Type:      "some.unknown.event",
        Timestamp: time.Now(),
        Payload:   map[string]any{"key": "value"},
    }
    subject, body := TelegramEventRenderer(e)
    if subject == "" {
        t.Error("subject should not be empty")
    }
    if body == "" {
        t.Error("body should have fallback rendering for unknown events")
    }
}

func TestDefaultEventRenderer_BasicOutput(t *testing.T) {
    e := event.Event{
        EventID:   "evt-3",
        Type:      event.TypeRunCompleted,
        Timestamp: time.Now(),
        Payload:   map[string]any{"run_id": "run-1"},
    }
    subject, body := DefaultEventRenderer(e)
    if subject != e.Type {
        t.Errorf("subject = %q, want %q", subject, e.Type)
    }
    if body == "" {
        t.Error("body should not be empty")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/notify/ -run TestTelegramEventRenderer -v`
Expected: FAIL — `TelegramEventRenderer` not defined

- [ ] **Step 3: Write the implementation**

Create `internal/server/notify/renderer.go`. Port the event formatting logic from `internal/bridge/connector/telegram/formatter.go`, converting MarkdownV2 to HTML:

```go
package notify

import (
    "fmt"
    "html"
    "strings"

    "github.com/kodrunhq/claude-plane/internal/server/event"
)

// EventRenderer converts an event into a subject and body for notification delivery.
type EventRenderer func(e event.Event) (subject string, body string)

// TelegramEventRenderer produces rich HTML-formatted messages for Telegram.
// Ported from bridge FormatEvent(), converted from MarkdownV2 to HTML.
func TelegramEventRenderer(e event.Event) (string, string) {
    str := func(key string) string {
        v, _ := e.Payload[key].(string)
        return v
    }
    nameOrID := func(nameKey, idKey string) string {
        if n := str(nameKey); n != "" {
            return n
        }
        return str(idKey)
    }
    optLine := func(label, value string) string {
        if value == "" {
            return ""
        }
        return fmt.Sprintf("%s: <code>%s</code>\n", html.EscapeString(label), html.EscapeString(value))
    }

    var subject, body string

    switch e.Type {
    // Session events
    case event.TypeSessionStarted:
        name := nameOrID("session_name", "session_id")
        subject = "Session Started"
        body = fmt.Sprintf("▶️ <b>Session Started</b>\n%s%s%s",
            optLine("Session", name),
            optLine("Machine", str("machine_id")),
            optLine("Working Dir", str("working_directory")))

    case event.TypeSessionExited:
        name := nameOrID("session_name", "session_id")
        subject = "Session Exited"
        body = fmt.Sprintf("⏹️ <b>Session Exited</b>\n%s%s%s",
            optLine("Session", name),
            optLine("Exit Code", str("exit_code")),
            optLine("Duration", str("duration")))

    case event.TypeSessionTerminated:
        name := nameOrID("session_name", "session_id")
        subject = "Session Terminated"
        body = fmt.Sprintf("🛑 <b>Session Terminated</b>\n%s%s",
            optLine("Session", name),
            optLine("Reason", str("reason")))

    case event.TypeSessionDispatchFailed:
        name := nameOrID("session_name", "session_id")
        subject = "Session Dispatch Failed"
        body = fmt.Sprintf("❌ <b>Dispatch Failed</b>\n%s%s",
            optLine("Session", name),
            optLine("Error", str("error")))

    case event.TypeSessionWaitingForInput:
        name := nameOrID("session_name", "session_id")
        subject = "Session Waiting for Input"
        body = fmt.Sprintf("⏳ <b>Waiting for Input</b>\n%s%s",
            optLine("Session", name),
            optLine("Machine", str("machine_id")))

    case event.TypeSessionResumed:
        name := nameOrID("session_name", "session_id")
        subject = "Session Resumed"
        body = fmt.Sprintf("▶️ <b>Session Resumed</b>\n%s",
            optLine("Session", name))

    // Run events
    case event.TypeRunCreated:
        subject = "Run Created"
        body = fmt.Sprintf("🆕 <b>Run Created</b>\n%s%s",
            optLine("Run", str("run_id")),
            optLine("Job", nameOrID("job_name", "job_id")))

    case event.TypeRunStarted:
        subject = "Run Started"
        body = fmt.Sprintf("▶️ <b>Run Started</b>\n%s%s",
            optLine("Run", str("run_id")),
            optLine("Job", nameOrID("job_name", "job_id")))

    case event.TypeRunCompleted:
        subject = "Run Completed"
        body = fmt.Sprintf("✅ <b>Run Completed</b>\n%s%s%s",
            optLine("Run", str("run_id")),
            optLine("Job", nameOrID("job_name", "job_id")),
            optLine("Duration", str("duration")))

    case event.TypeRunFailed:
        subject = "Run Failed"
        body = fmt.Sprintf("❌ <b>Run Failed</b>\n%s%s%s",
            optLine("Run", str("run_id")),
            optLine("Job", nameOrID("job_name", "job_id")),
            optLine("Error", str("error")))

    case event.TypeRunCancelled:
        subject = "Run Cancelled"
        body = fmt.Sprintf("🚫 <b>Run Cancelled</b>\n%s%s",
            optLine("Run", str("run_id")),
            optLine("Job", nameOrID("job_name", "job_id")))

    // Machine events
    case event.TypeMachineConnected:
        subject = "Machine Connected"
        body = fmt.Sprintf("🟢 <b>Machine Connected</b>\n%s%s",
            optLine("Machine", str("machine_id")),
            optLine("Hostname", str("hostname")))

    case event.TypeMachineDisconnected:
        subject = "Machine Disconnected"
        body = fmt.Sprintf("🔴 <b>Machine Disconnected</b>\n%s",
            optLine("Machine", str("machine_id")))

    // Bridge events
    case event.TypeBridgeStarted:
        subject = "Bridge Started"
        body = fmt.Sprintf("🌉 <b>Bridge Started</b>\n%s",
            optLine("Version", str("version")))

    case event.TypeBridgeStopped:
        subject = "Bridge Stopped"
        body = fmt.Sprintf("🌉 <b>Bridge Stopped</b>\n%s",
            optLine("Reason", str("reason")))

    case event.TypeBridgeConnectorStarted:
        subject = "Connector Started"
        body = fmt.Sprintf("🔌 <b>Connector Started</b>\n%s%s",
            optLine("Connector", str("name")),
            optLine("Type", str("connector_type")))

    case event.TypeBridgeConnectorError:
        subject = "Connector Error"
        body = fmt.Sprintf("⚠️ <b>Connector Error</b>\n%s%s%s",
            optLine("Connector", str("name")),
            optLine("Type", str("connector_type")),
            optLine("Error", str("error")))

    case event.TypeBridgeConnectorCommand:
        subject = "Telegram Command"
        body = fmt.Sprintf("💬 <b>Command Received</b>\n%s%s%s",
            optLine("Command", str("command")),
            optLine("Sender", str("sender")),
            optLine("Result", str("result")))

    default:
        subject = e.Type
        var lines []string
        lines = append(lines, fmt.Sprintf("📋 <b>%s</b>", html.EscapeString(e.Type)))
        for k, v := range e.Payload {
            lines = append(lines, fmt.Sprintf("%s: <code>%s</code>",
                html.EscapeString(k), html.EscapeString(fmt.Sprint(v))))
        }
        body = strings.Join(lines, "\n")
    }

    return subject, body
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/server/notify/ -run TestTelegramEventRenderer -v`
Expected: PASS

Run: `go test -race ./internal/server/notify/ -run TestDefaultEventRenderer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/notify/renderer.go internal/server/notify/renderer_test.go
git commit -m "feat: add EventRenderer type and TelegramEventRenderer"
```

---

## Task 6: Update Notification Dispatcher — ConnectorResolver + Renderers

**Files:**
- Modify: `internal/server/notify/dispatcher.go`

- [ ] **Step 1: Read the current dispatcher**

Read `internal/server/notify/dispatcher.go` in full to understand the current `Dispatcher` struct, `NewDispatcher`, and `Handler` method.

- [ ] **Step 2: Update Dispatcher struct and constructor**

Add `resolver ConnectorResolver` and `renderers map[string]EventRenderer` fields to the `Dispatcher` struct. Update `NewDispatcher` to accept them:

```go
type Dispatcher struct {
    store     SubscriptionStore
    notifiers map[string]Notifier
    resolver  ConnectorResolver
    renderers map[string]EventRenderer
    limiter   *RateLimiter
    renderer  func(event.Event) (string, string) // default renderer
    logger    *slog.Logger
}

func NewDispatcher(
    store SubscriptionStore,
    resolver ConnectorResolver,
    notifiers map[string]Notifier,
    renderers map[string]EventRenderer,
    defaultRenderer func(event.Event) (string, string),
    logger *slog.Logger,
) *Dispatcher {
    return &Dispatcher{
        store:     store,
        notifiers: notifiers,
        resolver:  resolver,
        renderers: renderers,
        limiter:   NewRateLimiter(60 * time.Second),
        renderer:  defaultRenderer,
        logger:    logger,
    }
}
```

- [ ] **Step 3: Update Handler method to resolve connector and select renderer**

In the `Handler` method, after getting subscriptions:

```go
// Select renderer based on channel type
render := d.renderer
if r, ok := d.renderers[sub.ChannelType]; ok {
    render = r
}
subject, body := render(e)

// Resolve config — use connector config if connector_id is set
config := sub.Config
if sub.ConnectorID != nil && *sub.ConnectorID != "" {
    resolved, err := d.resolver.ResolveConnectorConfig(ctx, *sub.ConnectorID)
    if err != nil {
        d.logger.Error("resolve connector config", "connector_id", *sub.ConnectorID, "error", err)
        continue
    }
    config = resolved
}

// Send notification
if err := notifier.Send(ctx, config, subject, body); err != nil {
    d.logger.Error("send notification", "channel", sub.ChannelID, "error", err)
}
```

- [ ] **Step 4: Run all notify package tests**

Run: `go test -race ./internal/server/notify/ -v`
Expected: PASS (may need to update existing test constructors for new `NewDispatcher` signature)

- [ ] **Step 5: Fix any test compilation errors due to constructor change**

Update existing tests in `internal/server/notify/` that call `NewDispatcher` to pass the new parameters (`nil` for resolver, `nil` for renderers is acceptable in tests that don't exercise connector resolution).

- [ ] **Step 6: Commit**

```bash
git add internal/server/notify/dispatcher.go
git commit -m "feat: dispatcher supports ConnectorResolver and per-channel-type renderers"
```

---

## Task 7: BridgeIngestHandler — Log and Event Ingestion Endpoints

**Files:**
- Create: `internal/server/handler/bridge_ingest.go`
- Create: `internal/server/handler/bridge_ingest_test.go`

- [ ] **Step 1: Write the failing test for log ingestion**

Create `internal/server/handler/bridge_ingest_test.go`:

```go
package handler

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/kodrunhq/claude-plane/internal/server/logging"
)

func TestBridgeIngestHandler_HandleLogs(t *testing.T) {
    logStore := newTestLogStore(t)
    bcast := logging.NewLogBroadcaster()
    h := NewBridgeIngestHandler(logStore, bcast, nil, nil)

    body := map[string]any{
        "source": "bridge",
        "entries": []map[string]any{
            {
                "timestamp": time.Now().UTC().Format(time.RFC3339Nano),
                "level":     "INFO",
                "message":   "test log entry",
                "attributes": map[string]any{
                    "connector_id": "conn-1",
                },
            },
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/logs", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    h.HandleLogs(w, req)

    if w.Code != http.StatusAccepted {
        t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race ./internal/server/handler/ -run TestBridgeIngestHandler_HandleLogs -v`
Expected: FAIL — `NewBridgeIngestHandler` not defined

- [ ] **Step 3: Write the implementation**

Create `internal/server/handler/bridge_ingest.go`:

```go
package handler

import (
    "encoding/json"
    "log/slog"
    "net/http"
    "time"

    "github.com/kodrunhq/claude-plane/internal/server/event"
    "github.com/kodrunhq/claude-plane/internal/server/httputil"
    "github.com/kodrunhq/claude-plane/internal/server/logging"
)

// BridgeIngestHandler receives logs and events from the bridge process.
// Separate from IngestHandler (webhook_ingest.go) which uses HMAC auth.
type BridgeIngestHandler struct {
    logStore *logging.LogStore
    bcast    *logging.LogBroadcaster
    eventBus *event.Bus
    logger   *slog.Logger
}

func NewBridgeIngestHandler(
    logStore *logging.LogStore,
    bcast *logging.LogBroadcaster,
    eventBus *event.Bus,
    logger *slog.Logger,
) *BridgeIngestHandler {
    if logger == nil {
        logger = slog.Default()
    }
    return &BridgeIngestHandler{
        logStore: logStore,
        bcast:    bcast,
        eventBus: eventBus,
        logger:   logger,
    }
}

type logIngestionRequest struct {
    Source  string             `json:"source"`
    Entries []logEntryRequest  `json:"entries"`
}

type logEntryRequest struct {
    Timestamp  time.Time         `json:"timestamp"`
    Level      string            `json:"level"`
    Message    string            `json:"message"`
    Attributes map[string]any    `json:"attributes"`
}

// HandleLogs ingests a batch of structured log entries from the bridge.
func (h *BridgeIngestHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
    var req logIngestionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if len(req.Entries) == 0 {
        httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
        return
    }

    source := req.Source
    if source == "" {
        source = "bridge"
    }

    records := make([]logging.LogRecord, 0, len(req.Entries))
    for _, entry := range req.Entries {
        metadata, _ := json.Marshal(entry.Attributes)
        rec := logging.LogRecord{
            Timestamp: entry.Timestamp,
            Level:     entry.Level,
            Message:   entry.Message,
            Source:    source,
            Metadata:  string(metadata),
        }
        // Extract well-known attributes
        if v, ok := entry.Attributes["component"].(string); ok {
            rec.Component = v
        }
        if v, ok := entry.Attributes["error"].(string); ok {
            rec.Error = v
        }
        records = append(records, rec)
    }

    // Store in database
    if err := h.logStore.InsertBatch(records); err != nil {
        h.logger.Error("bridge log ingestion failed", "error", err)
        httputil.WriteError(w, http.StatusInternalServerError, "failed to store logs")
        return
    }

    // Broadcast to WebSocket subscribers
    for _, rec := range records {
        h.bcast.Broadcast(rec)
    }

    httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
}

type eventIngestionRequest struct {
    Events []eventEntryRequest `json:"events"`
}

type eventEntryRequest struct {
    Type    string         `json:"type"`
    Payload map[string]any `json:"payload"`
}

// HandleEvents ingests lifecycle events from the bridge and publishes them
// to the event bus (triggering notifications, webhooks, WebSocket, and DB persistence).
func (h *BridgeIngestHandler) HandleEvents(w http.ResponseWriter, r *http.Request) {
    var req eventIngestionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        httputil.WriteError(w, http.StatusBadRequest, "invalid request body")
        return
    }
    if len(req.Events) == 0 {
        httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
        return
    }

    for _, entry := range req.Events {
        evt := event.NewBridgeEvent(entry.Type, entry.Payload)
        if err := h.eventBus.Publish(r.Context(), evt); err != nil {
            h.logger.Error("bridge event publish failed", "type", entry.Type, "error", err)
        }
    }

    httputil.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "ok"})
}

// RegisterBridgeIngestRoutes registers the bridge ingest endpoints on a chi.Router.
// In main.go, these are registered in the same API key-authenticated group as bridge routes
// (NOT inside api.NewRouter — bridge routes are registered outside it in main.go).
func RegisterBridgeIngestRoutes(r chi.Router, h *BridgeIngestHandler) {
    r.Post("/api/v1/ingest/logs", h.HandleLogs)
    r.Post("/api/v1/ingest/events", h.HandleEvents)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race ./internal/server/handler/ -run TestBridgeIngestHandler -v`
Expected: PASS

- [ ] **Step 5: Write test for event ingestion**

Add to `bridge_ingest_test.go`:

```go
func TestBridgeIngestHandler_HandleEvents(t *testing.T) {
    bus := event.NewBus(slog.Default())
    defer bus.Close()

    received := make(chan event.Event, 1)
    bus.Subscribe("bridge.*", func(ctx context.Context, e event.Event) error {
        received <- e
        return nil
    }, event.SubscriberOptions{BufferSize: 8})

    h := NewBridgeIngestHandler(nil, nil, bus, nil)

    body := map[string]any{
        "events": []map[string]any{
            {"type": "bridge.started", "payload": map[string]any{"version": "0.8.0"}},
        },
    }
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/events", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    h.HandleEvents(w, req)

    if w.Code != http.StatusAccepted {
        t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
    }

    select {
    case e := <-received:
        if e.Type != "bridge.started" {
            t.Errorf("event type = %q, want bridge.started", e.Type)
        }
        if e.Source != "bridge" {
            t.Errorf("event source = %q, want bridge", e.Source)
        }
    case <-time.After(2 * time.Second):
        t.Fatal("timed out waiting for event")
    }
}
```

- [ ] **Step 6: Run all bridge ingest tests**

Run: `go test -race ./internal/server/handler/ -run TestBridgeIngestHandler -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/server/handler/bridge_ingest.go internal/server/handler/bridge_ingest_test.go
git commit -m "feat: add BridgeIngestHandler for log and event ingestion"
```

---

## Task 8: Auto-Sync Notification Channel on Telegram Connector CRUD

**Files:**
- Modify: `internal/server/handler/bridge.go`

- [ ] **Step 1: Read the current bridge handler**

Read `internal/server/handler/bridge.go` in full to understand `CreateConnector`, `UpdateConnector`, `DeleteConnector`.

- [ ] **Step 2: Add NotificationStore dependency to BridgeHandler**

The `BridgeHandler` needs access to notification channel CRUD. Add a `NotifStore` interface to the struct (method names must match the actual store methods):

```go
type NotifStore interface {
    CreateNotificationChannel(ctx context.Context, ch store.NotificationChannel) (*store.NotificationChannel, error)
    UpdateNotificationChannel(ctx context.Context, channelID string, ch store.NotificationChannel) (*store.NotificationChannel, error)
    DeleteNotificationChannel(ctx context.Context, channelID string) error
    GetChannelByConnectorID(ctx context.Context, connectorID string) (*store.NotificationChannel, error)
}
```

Add to `BridgeHandler` struct and `NewBridgeHandler`.

- [ ] **Step 3: Add auto-sync to CreateConnector**

After the connector is successfully created, if `connector_type == "telegram"`, auto-create a notification channel:

```go
// Auto-create notification channel for Telegram connectors
if connector.ConnectorType == "telegram" {
    var connCfg struct {
        GroupID        int64 `json:"group_id"`
        EventsTopicID int   `json:"events_topic_id"`
    }
    _ = json.Unmarshal([]byte(connector.Config), &connCfg)

    notifConfig := map[string]any{
        "chat_id": strconv.FormatInt(connCfg.GroupID, 10),
    }
    if connCfg.EventsTopicID > 0 {
        notifConfig["topic_id"] = connCfg.EventsTopicID
    }
    cfgJSON, _ := json.Marshal(notifConfig)

    ch := store.NotificationChannel{
        ChannelID:   uuid.NewString(),
        ChannelType: "telegram",
        Name:        connector.Name,
        Config:      string(cfgJSON),
        Enabled:     true,
        CreatedBy:   claims.UserID,
        ConnectorID: &connector.ConnectorID,
    }
    if _, err := h.notifStore.CreateNotificationChannel(r.Context(), ch); err != nil {
        slog.Error("auto-create notification channel failed", "connector_id", connector.ConnectorID, "error", err)
    }
}
```

- [ ] **Step 5: Add auto-sync to UpdateConnector**

After the connector is updated, if `connector_type == "telegram"`, update the linked notification channel:

```go
if connector.ConnectorType == "telegram" {
    existingCh, err := h.notifStore.GetChannelByConnectorID(r.Context(), connectorID)
    if err == nil && existingCh != nil {
        // Update channel config to match connector
        var connCfg struct {
            GroupID        int64 `json:"group_id"`
            EventsTopicID int   `json:"events_topic_id"`
        }
        _ = json.Unmarshal([]byte(connector.Config), &connCfg)

        notifConfig := map[string]any{
            "chat_id": strconv.FormatInt(connCfg.GroupID, 10),
        }
        if connCfg.EventsTopicID > 0 {
            notifConfig["topic_id"] = connCfg.EventsTopicID
        }
        cfgJSON, _ := json.Marshal(notifConfig)

        existingCh.Name = connector.Name
        existingCh.Config = string(cfgJSON)
        if _, err := h.notifStore.UpdateNotificationChannel(r.Context(), existingCh.ChannelID, *existingCh); err != nil {
            slog.Error("auto-update notification channel failed", "connector_id", connectorID, "error", err)
        }
    }
}
```

- [ ] **Step 6: Add auto-sync to DeleteConnector**

Before or after deleting the connector, delete the linked notification channel:

```go
// Delete linked notification channel (subscriptions cascade via FK)
if existingCh, err := h.notifStore.GetChannelByConnectorID(r.Context(), connectorID); err == nil && existingCh != nil {
    if err := h.notifStore.DeleteNotificationChannel(r.Context(), existingCh.ChannelID); err != nil {
        slog.Error("auto-delete notification channel failed", "connector_id", connectorID, "error", err)
    }
}
```

- [ ] **Step 7: Write tests for auto-sync behavior**

Add tests to `internal/server/handler/bridge_test.go` (or a new test file) that verify:
1. Creating a Telegram connector auto-creates a notification channel with matching `connector_id`
2. Updating a Telegram connector updates the linked notification channel's name and config
3. Deleting a Telegram connector deletes the linked notification channel
4. Creating a GitHub connector does NOT create a notification channel

Each test should:
- Create the connector via the handler's HTTP endpoint
- Query the notification store for channels with matching `connector_id`
- Assert the channel exists/doesn't exist and has correct config

- [ ] **Step 8: Run auto-sync tests**

Run: `go test -race ./internal/server/handler/ -run TestBridgeAutoSync -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/server/handler/bridge.go internal/server/store/notifications.go
git commit -m "feat: auto-sync notification channel on Telegram connector CRUD"
```

---

## Task 9: Wire Server — Router, Main, Bridge Status

**Files:**
- Modify: `internal/server/api/router.go`
- Modify: `cmd/server/main.go`
- Modify: `internal/server/handler/bridge.go` (Status endpoint)

- [ ] **Step 1: Update cmd/server/main.go — wire new components**

In `cmd/server/main.go`:

1. Create `ConnectorConfigResolver`:
```go
connectorResolver := store.NewConnectorConfigResolver(s, encryptionKey)
```

2. Create `BridgeIngestHandler`:
```go
bridgeIngestHandler := handler.NewBridgeIngestHandler(logStore, teeHandler.Broadcaster(), eventBus, slog.Default())
```

3. Update `Dispatcher` constructor with new parameters (resolver, renderers):
```go
renderers := map[string]notify.EventRenderer{
    "telegram": notify.TelegramEventRenderer,
}
notifyDispatcher := notify.NewDispatcher(s, connectorResolver, notifiers, renderers, notify.DefaultEventRenderer, slog.Default())
```

4. Register bridge ingest routes in the same API key-authenticated group as bridge routes (at ~line 497, alongside `RegisterBridgeRoutes`):
```go
// Bridge routes: JWT-protected (supports API key auth for bridge binary).
router.Group(func(r chi.Router) {
    r.Use(api.JWTAuthMiddleware(authSvc, apiKeyAuth))
    handler.RegisterBridgeRoutes(r, bridgeHandler)
    handler.RegisterBridgeIngestRoutes(r, bridgeIngestHandler) // NEW
})
```

5. Pass `NotifStore` to `BridgeHandler` (update `NewBridgeHandler` call):
```go
bridgeHandler := handler.NewBridgeHandler(s, handlerClaimsGetter, encryptionKey, s /* notifStore */)
```

- [ ] **Step 3: Extend bridge status endpoint**

In `internal/server/handler/bridge.go`, update the `Status` handler to query recent bridge events and return the extended response format:

```go
type bridgeStatusResponse struct {
    Running    bool                    `json:"running"`
    LastSeen   *time.Time              `json:"last_seen,omitempty"`
    Connectors []connectorStatusEntry  `json:"connectors"`
    RestartRequestedAt *time.Time      `json:"restart_requested_at,omitempty"`
}

type connectorStatusEntry struct {
    ConnectorID string  `json:"connector_id"`
    Name        string  `json:"name"`
    Type        string  `json:"type"`
    Healthy     bool    `json:"healthy"`
    LastError   *string `json:"last_error"`
}
```

Derive `running` and `last_seen` from recent bridge events (query the events table for `bridge.*` types). Derive connector health from `bridge.connector.started` vs `bridge.connector.error` events.

- [ ] **Step 4: Run full backend tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/api/router.go cmd/server/main.go internal/server/handler/bridge.go
git commit -m "feat: wire BridgeIngestHandler, ConnectorResolver, and updated dispatcher"
```

---

## Task 10: Bridge Log Forwarder

**Files:**
- Create: `internal/bridge/log_forwarder.go`
- Create: `internal/bridge/log_forwarder_test.go`
- Modify: `internal/bridge/client/client.go`

- [ ] **Step 1: Add PostLogs method to bridge client**

In `internal/bridge/client/client.go`, add:

```go
type LogEntry struct {
    Timestamp  time.Time      `json:"timestamp"`
    Level      string         `json:"level"`
    Message    string         `json:"message"`
    Attributes map[string]any `json:"attributes,omitempty"`
}

type LogIngestionRequest struct {
    Source  string     `json:"source"`
    Entries []LogEntry `json:"entries"`
}

func (c *Client) PostLogs(ctx context.Context, req LogIngestionRequest) error {
    return c.doVoid(ctx, http.MethodPost, "/api/v1/ingest/logs", req)
}
```

Add a `doVoid` helper similar to `doJSON` but without reading the response body.

- [ ] **Step 2: Write the failing test for log forwarder**

Create `internal/bridge/log_forwarder_test.go`:

```go
package bridge

import (
    "encoding/json"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "sync"
    "testing"
    "time"
)

func TestLogForwarder_FlushOnThreshold(t *testing.T) {
    var mu sync.Mutex
    var received int

    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            Entries []json.RawMessage `json:"entries"`
        }
        json.NewDecoder(r.Body).Decode(&req)
        mu.Lock()
        received += len(req.Entries)
        mu.Unlock()
        w.WriteHeader(http.StatusAccepted)
    }))
    defer srv.Close()

    fwd := NewLogForwarder(srv.URL, "test-key", 5, 10*time.Second)
    defer fwd.Close()

    logger := slog.New(fwd)
    for i := 0; i < 5; i++ {
        logger.Info("test message")
    }

    // Wait for flush
    time.Sleep(500 * time.Millisecond)

    mu.Lock()
    defer mu.Unlock()
    if received != 5 {
        t.Errorf("received %d entries, want 5", received)
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -race ./internal/bridge/ -run TestLogForwarder -v`
Expected: FAIL — `NewLogForwarder` not defined

- [ ] **Step 4: Write the log forwarder implementation**

Create `internal/bridge/log_forwarder.go` — a `slog.Handler` that buffers entries and flushes them via the bridge client. Follow the pattern from `internal/agent/log_sink.go`:

```go
package bridge

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/http"
    "sync"
    "time"
)

const (
    defaultFlushSize     = 50
    defaultFlushInterval = 2 * time.Second
)

type logEntry struct {
    Timestamp  time.Time      `json:"timestamp"`
    Level      string         `json:"level"`
    Message    string         `json:"message"`
    Attributes map[string]any `json:"attributes,omitempty"`
}

type logForwarderState struct {
    baseURL  string
    apiKey   string
    client   *http.Client
    buf      []logEntry
    mu       sync.Mutex
    done     chan struct{}
    wg       sync.WaitGroup
    maxBatch int
    interval time.Duration
}

// LogForwarder is a slog.Handler that buffers log entries and forwards them
// to the server via POST /api/v1/ingest/logs.
type LogForwarder struct {
    state *logForwarderState
    attrs []slog.Attr
    group string
}

func NewLogForwarder(baseURL, apiKey string, maxBatch int, interval time.Duration) *LogForwarder {
    if maxBatch <= 0 {
        maxBatch = defaultFlushSize
    }
    if interval <= 0 {
        interval = defaultFlushInterval
    }
    s := &logForwarderState{
        baseURL:  baseURL,
        apiKey:   apiKey,
        client:   &http.Client{Timeout: 10 * time.Second},
        buf:      make([]logEntry, 0, maxBatch),
        done:     make(chan struct{}),
        maxBatch: maxBatch,
        interval: interval,
    }
    fwd := &LogForwarder{state: s}
    s.wg.Add(1)
    go s.flushLoop()
    return fwd
}

func (f *LogForwarder) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (f *LogForwarder) Handle(_ context.Context, r slog.Record) error {
    attrs := make(map[string]any)
    for _, a := range f.attrs {
        attrs[a.Key] = a.Value.Any()
    }
    r.Attrs(func(a slog.Attr) bool {
        attrs[a.Key] = a.Value.Any()
        return true
    })

    entry := logEntry{
        Timestamp:  r.Time,
        Level:      r.Level.String(),
        Message:    r.Message,
        Attributes: attrs,
    }

    s := f.state
    s.mu.Lock()
    s.buf = append(s.buf, entry)
    needsFlush := len(s.buf) >= s.maxBatch
    s.mu.Unlock()

    if needsFlush {
        go s.flush()
    }
    return nil
}

func (f *LogForwarder) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &LogForwarder{state: f.state, attrs: append(f.attrs[:len(f.attrs):len(f.attrs)], attrs...), group: f.group}
}

func (f *LogForwarder) WithGroup(name string) slog.Handler {
    return &LogForwarder{state: f.state, attrs: f.attrs, group: name}
}

func (f *LogForwarder) Close() {
    close(f.state.done)
    f.state.wg.Wait()
    f.state.flush() // drain remaining
}

func (s *logForwarderState) flushLoop() {
    defer s.wg.Done()
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.flush()
        case <-s.done:
            return
        }
    }
}

func (s *logForwarderState) flush() {
    s.mu.Lock()
    if len(s.buf) == 0 {
        s.mu.Unlock()
        return
    }
    batch := s.buf
    s.buf = make([]logEntry, 0, s.maxBatch)
    s.mu.Unlock()

    payload := map[string]any{
        "source":  "bridge",
        "entries": batch,
    }
    body, err := json.Marshal(payload)
    if err != nil {
        fmt.Fprintf(os.Stderr, "log forwarder marshal error: %v\n", err)
        return
    }

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/v1/ingest/logs", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+s.apiKey)

    resp, err := s.client.Do(req)
    if err != nil {
        // Server unreachable — log to stderr, don't recurse
        return
    }
    resp.Body.Close()
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/bridge/ -run TestLogForwarder -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/log_forwarder.go internal/bridge/log_forwarder_test.go internal/bridge/client/client.go
git commit -m "feat: add bridge log forwarder slog handler"
```

---

## Task 11: Bridge Event Emitter

**Files:**
- Create: `internal/bridge/event_emitter.go`
- Create: `internal/bridge/event_emitter_test.go`
- Modify: `internal/bridge/client/client.go`

- [ ] **Step 1: Add PostEvents method to bridge client**

In `internal/bridge/client/client.go`, add:

```go
type EventEntry struct {
    Type    string         `json:"type"`
    Payload map[string]any `json:"payload,omitempty"`
}

type EventIngestionRequest struct {
    Events []EventEntry `json:"events"`
}

func (c *Client) PostEvents(ctx context.Context, req EventIngestionRequest) error {
    return c.doVoid(ctx, http.MethodPost, "/api/v1/ingest/events", req)
}
```

- [ ] **Step 2: Write the event emitter**

Create `internal/bridge/event_emitter.go`:

```go
package bridge

import (
    "context"
    "log/slog"
    "time"

    "github.com/kodrunhq/claude-plane/internal/bridge/client"
)

// EventEmitter sends bridge lifecycle events to the server.
type EventEmitter struct {
    client *client.Client
    logger *slog.Logger
}

func NewEventEmitter(c *client.Client, logger *slog.Logger) *EventEmitter {
    return &EventEmitter{client: c, logger: logger}
}

func (e *EventEmitter) Emit(eventType string, payload map[string]any) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    req := client.EventIngestionRequest{
        Events: []client.EventEntry{{Type: eventType, Payload: payload}},
    }
    if err := e.client.PostEvents(ctx, req); err != nil {
        e.logger.Error("failed to emit event", "type", eventType, "error", err)
    }
}
```

- [ ] **Step 3: Write a basic test**

Create `internal/bridge/event_emitter_test.go` that verifies the emitter calls the correct endpoint.

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/bridge/ -run TestEventEmitter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/event_emitter.go internal/bridge/event_emitter_test.go internal/bridge/client/client.go
git commit -m "feat: add bridge event emitter for lifecycle events"
```

---

## Task 12: Strip Event Forwarding from Bridge Telegram Connector

**Files:**
- Modify: `internal/bridge/connector/telegram/telegram.go`
- Modify: `internal/bridge/connector/telegram/formatter.go`

- [ ] **Step 1: Read the current Telegram connector**

Read `internal/bridge/connector/telegram/telegram.go` and `formatter.go` in full.

- [ ] **Step 2: Add `commands_enabled` to Config, remove `event_types`**

In `telegram.go`, update the `Config` struct:

```go
type Config struct {
    BotToken        string
    GroupID         int64
    EventsTopicID   int
    CommandsTopicID int
    PollTimeout     int
    CommandsEnabled bool
    // EventTypes removed — event routing handled by server notification subscriptions
}
```

- [ ] **Step 3: Remove pollAndForwardEvents from Start**

In the `Start` method, remove the goroutine that runs `pollAndForwardEvents`. Keep only the command polling goroutine. If `CommandsEnabled` is false, skip command polling too.

- [ ] **Step 4: Remove pollAndForwardEvents method**

Delete the `pollAndForwardEvents` method entirely from `telegram.go`.

- [ ] **Step 5: Remove ShouldForwardEvent and MatchEventType from formatter.go**

Delete `ShouldForwardEvent` and `MatchEventType` from `formatter.go`. Keep `FormatEvent` for now (it will be deprecated once the server-side renderer is confirmed working, but doesn't hurt to keep as reference).

- [ ] **Step 6: Update cmd/bridge/main.go**

In `cmd/bridge/main.go`, update the Telegram connector config parsing:
- Add `CommandsEnabled` field parsing from connector config JSON
- Remove `EventTypes` field parsing
- Wire the log forwarder as the slog handler
- Wire the event emitter and emit `bridge.started` / `bridge.connector.started` / `bridge.stopped` events

```go
// At startup
logFwd := bridge.NewLogForwarder(cfg.ClaudePlane.APIURL, cfg.ClaudePlane.APIKey, 50, 2*time.Second)
slog.SetDefault(slog.New(logFwd))
defer logFwd.Close()

emitter := bridge.NewEventEmitter(apiClient, slog.Default())
emitter.Emit("bridge.started", map[string]any{"version": buildinfo.Version})

// ... for each connector start:
emitter.Emit("bridge.connector.started", map[string]any{
    "connector_id":   cc.ConnectorID,
    "connector_type": cc.ConnectorType,
    "name":           cc.Name,
})

// On shutdown:
emitter.Emit("bridge.stopped", map[string]any{"reason": "shutdown"})
```

- [ ] **Step 7: Run bridge tests**

Run: `go test -race ./internal/bridge/... -v`
Expected: PASS (some Telegram connector tests may need updating)

- [ ] **Step 8: Update Telegram connector tests**

Update tests that reference `EventTypes`, `ShouldForwardEvent`, or `pollAndForwardEvents`. Remove test cases for event forwarding. Add test for `CommandsEnabled` config.

- [ ] **Step 9: Run all tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/bridge/connector/telegram/ cmd/bridge/main.go
git commit -m "feat: strip event forwarding from bridge Telegram connector, add lifecycle events"
```

---

## Task 13: Frontend — Simplify TelegramForm, Remove Telegram from ChannelFormModal

**Files:**
- Modify: `web/src/components/connectors/TelegramForm.tsx`
- Modify: `web/src/components/settings/ChannelFormModal.tsx`

- [ ] **Step 1: Simplify TelegramForm**

In `TelegramForm.tsx`:
- Remove the `eventTypes` state variable and text input
- Remove `event_types` from the config object in `handleSubmit`
- Add a `commandsEnabled` toggle (boolean state, default `true`)
- Add `commands_enabled` to the config object
- Make Events Topic ID and Commands Topic ID optional (remove `required` attribute)

- [ ] **Step 2: Remove Telegram from ChannelFormModal**

In `ChannelFormModal.tsx`:
- Remove the `telegram` tab from the tab buttons (keep only `email`)
- Remove the `telegramConfig` state, `parseTelegramConfig`, `defaultTelegramConfig`
- Remove the Telegram config section (bot token, chat ID, topic ID inputs)
- Remove the `TelegramConfig` import from types
- The `activeTab` state can be removed entirely (always `email`)

- [ ] **Step 3: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Run frontend tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/connectors/TelegramForm.tsx web/src/components/settings/ChannelFormModal.tsx
git commit -m "feat: simplify Telegram connector form, remove Telegram from notification channel modal"
```

---

## Task 14: Frontend — NotificationsTab Connector Integration

**Files:**
- Modify: `web/src/components/settings/NotificationsTab.tsx`

- [ ] **Step 1: Read the current NotificationsTab**

Read `web/src/components/settings/NotificationsTab.tsx` in full.

- [ ] **Step 2: Add connector awareness to channel list**

Modify the channel list rendering:
- For channels with `connector_id`, show a "Connector" badge and a link icon
- Disable edit/delete buttons for connector-backed channels with tooltip: "Managed from Connectors page"
- If no channels exist AND no Telegram connectors exist, show info banner: "Want Telegram notifications? Set up a Telegram connector first." with link to `/connectors`

- [ ] **Step 3: Filter "Add Channel" to email only**

Change the "Add Channel" button behavior: when clicked, open the modal directly in email mode (no channel type selection needed since Telegram is gone from the modal).

- [ ] **Step 4: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/src/components/settings/NotificationsTab.tsx
git commit -m "feat: show connector-backed Telegram channels as non-editable in notifications"
```

---

## Task 15: Frontend — Logs Page Source Filter

**Files:**
- Modify: `web/src/views/LogsPage.tsx`

- [ ] **Step 1: Read the current LogsPage**

Read `web/src/views/LogsPage.tsx` in full.

- [ ] **Step 2: Add source filter dropdown**

Add a dropdown/select next to existing filters with options: All, Server, Bridge. When selected, update the `source` query parameter (already supported by the backend `GET /api/v1/logs` endpoint).

- [ ] **Step 3: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/src/views/LogsPage.tsx
git commit -m "feat: add source filter dropdown to Logs page"
```

---

## Task 16: Frontend — Connector Detail Page for Telegram

**Files:**
- Modify: `web/src/views/ConnectorDetailPage.tsx` (already exists — currently handles GitHub only)
- Create: `web/src/hooks/useBridgeStatus.ts`

- [ ] **Step 1: Read the existing ConnectorDetailPage**

Read `web/src/views/ConnectorDetailPage.tsx` to understand the current GitHub-specific rendering.

- [ ] **Step 2: Create bridge status hook**

Create `web/src/hooks/useBridgeStatus.ts`:

```typescript
import { useQuery } from '@tanstack/react-query';
import { request } from '../api/client';

interface ConnectorStatus {
  connector_id: string;
  name: string;
  type: string;
  healthy: boolean;
  last_error: string | null;
}

interface BridgeStatus {
  running: boolean;
  last_seen: string | null;
  connectors: ConnectorStatus[];
}

export function useBridgeStatus() {
  return useQuery<BridgeStatus>({
    queryKey: ['bridge', 'status'],
    queryFn: () => request<BridgeStatus>('/bridge/status'),
    refetchInterval: 15000, // poll every 15s
  });
}
```

- [ ] **Step 3: Add Telegram section to ConnectorDetailPage**

When the connector type is `telegram`, render:
- Connection status indicator (using `useBridgeStatus` to find this connector's health)
- Last seen / uptime
- Edit button to modify config (reuse `TelegramForm` in edit mode)
- If `commands_enabled`: show a "Available Commands" section listing `/sessions`, `/machines`, `/kill`, `/inject`, `/status`, `/start`, `/list`, `/help` with brief descriptions
- Link to Settings > Notifications: "Configure which events are sent to this connector"

- [ ] **Step 4: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add web/src/views/ConnectorDetailPage.tsx web/src/hooks/useBridgeStatus.ts
git commit -m "feat: add Telegram connector detail view with commands and status"
```

---

## Task 17: Frontend — Command Center Bridge Status Indicator

**Files:**
- Modify: `web/src/views/CommandCenter.tsx`

- [ ] **Step 1: Read the current CommandCenter**

Read `web/src/views/CommandCenter.tsx` to understand the dashboard layout.

- [ ] **Step 2: Add bridge status indicator**

Add a small bridge status card/indicator to the dashboard using `useBridgeStatus()`:
- If bridge is running: green dot + "Bridge" + connector count
- If bridge is disconnected: red dot + "Bridge offline"
- If no bridge data: gray dot + "Bridge" (no status)

Keep it minimal — a small status pill in the dashboard header area or alongside machine status.

- [ ] **Step 3: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/src/views/CommandCenter.tsx
git commit -m "feat: add bridge status indicator to Command Center dashboard"
```

---

## Task 18: Frontend — Connector Status Dots

**Files:**
- Modify: connector list component (identify correct file by reading the connectors page)

`useBridgeStatus` hook was already created in Task 16.

- [ ] **Step 1: Add status dots to connector list**

In the connector list component, use `useBridgeStatus()` to show colored dots:
- Green: healthy
- Red: error or disconnected
- Gray: unknown/no data

- [ ] **Step 3: Verify frontend compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/src/hooks/useBridgeStatus.ts web/src/components/connectors/
git commit -m "feat: add bridge status hook and connector status indicators"
```

---

## Task 19: Full Integration Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test -race ./...`
Expected: PASS

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 3: Run frontend typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Run frontend lint**

Run: `cd web && npm run lint`
Expected: No errors

- [ ] **Step 5: Run frontend tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [ ] **Step 6: Build frontend**

Run: `cd web && npm run build`
Expected: Build succeeds (this runs `tsc -b` which is stricter than `tsc --noEmit`)

- [ ] **Step 7: Build Go binaries**

Run: `go build -o claude-plane-server ./cmd/server && go build -o claude-plane-bridge ./cmd/bridge`
Expected: Both binaries build successfully

- [ ] **Step 8: Verify event types are in sync**

Run: `go generate ./internal/server/event/... && git diff --exit-code internal/server/event/`
Expected: No diff (event types already generated)

- [ ] **Step 9: Commit any remaining fixes**

```bash
git add -A
git commit -m "chore: integration verification fixes"
```
