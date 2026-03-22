# Unified Telegram Integration & Bridge Logging

**Date**: 2026-03-21
**Status**: Draft
**Scope**: Bridge logging, Telegram connector/notification unification, frontend UX, bridge lifecycle events

## Problem Statement

Three interconnected issues degrade the claude-plane operator experience:

1. **Bridge logs are invisible.** The bridge process logs to stderr only. No logs or events reach the server, making it impossible to debug connector issues from the UI.

2. **Telegram integration is duplicated.** Two independent systems send events to Telegram:
   - **Bridge Telegram connector** — polls the server's event feed, filters by glob patterns in a text input, forwards to Telegram. Also handles inbound commands (`/sessions`, `/kill`, `/inject`, etc.).
   - **Notification channels** (Settings > Notifications) — subscribes to the server event bus, delivers to Telegram via a separate bot token + chat ID config. Has a checkbox matrix for event selection.

   These systems share no code, no data model, and no configuration. A user setting up Telegram must configure it twice with different UIs.

3. **Telegram connector UX is poor.** Event type filtering is a comma-separated glob textbox (`session.*,run.*`). No visibility into whether filters are working. No post-creation detail page. No documentation of available Telegram commands.

## Design Principles

- **One bot token, one config, one place.** The Connectors page owns the Telegram connection. The Notifications page owns event routing. Neither duplicates the other.
- **Events for automation, logs for debugging.** Bridge lifecycle milestones become server events (triggerable, subscribable). Detailed operational output becomes structured logs (filterable, searchable).
- **Progressive disclosure.** Each page handles its responsibility or points to where it's handled.

---

## 1. Bridge Logging

### Overview

The bridge gets a log forwarder that ships structured log entries to the server via REST API. Reference implementation: the agent's `GRPCSinkHandler` in `internal/agent/log_sink.go` follows the same buffer-and-flush pattern over gRPC; this design mirrors it over REST.

### Server Endpoint

A **new** handler is created for log ingestion, registered in the API key-authenticated route group (not the JWT-authenticated admin routes, since the bridge uses API key auth):

```
POST /api/v1/ingest/logs
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "source": "bridge",
  "entries": [
    {
      "timestamp": "2026-03-21T10:15:30.123Z",
      "level": "INFO",
      "message": "Telegram connector started",
      "attributes": {
        "connector_id": "abc-123",
        "connector_name": "production-telegram",
        "commands_enabled": true
      }
    }
  ]
}
```

**Server-side wiring:** A new `BridgeIngestHandler` struct (separate from the existing `IngestHandler` in `handler/webhook_ingest.go`, which uses HMAC auth for webhooks). The `BridgeIngestHandler.HandleLogs` method:
1. Validates and parses the batch of log entries.
2. Writes entries to the `LogStore` via `LogStore.InsertBatch()` with `source = "bridge"`.
3. Explicitly broadcasts each entry to the WebSocket log stream via `LogBroadcaster.Broadcast()` — this is necessary because the existing `TeeHandler` in `logging/tee_handler.go` only captures server-originated logs. Externally ingested logs must be manually fed to the broadcast path.

### Bridge-Side Implementation

A custom `slog.Handler` (`internal/bridge/log_forwarder.go`) that:
- Buffers log entries in memory (matching agent's `log_sink.go` pattern)
- Flushes to the server every 2-3 seconds or when buffer reaches 50 entries
- Falls back to stderr-only if the server is unreachable
- Tags all entries with `source=bridge`
- Is configured as the bridge's default slog handler at startup

### Log Coverage

**Startup/shutdown:**
- Bridge process starting with config summary (server URL, number of connectors)
- Each connector initializing — name, type, config summary (secrets redacted)
- Startup failures with full error context

**Telegram connector:**
- Command received: who sent it, which command, arguments, result
- Command execution errors with full context
- Telegram API errors (rate limits, auth failures, network)
- Bot poll cycle health

**GitHub connector:**
- Poll cycle: repo, triggers checked, matches found
- Session creation attempts: template, machine, variables, result
- GitHub API errors (rate limits, auth, 404s)
- Filter evaluation: which triggers matched, which didn't and why

**General:**
- Config reload / restart signal received
- API client errors (server unreachable, auth failures)
- Health check status changes

### UI Changes

The Logs page (`web/src/views/LogsPage.tsx`) gets a source filter dropdown: **All** / **Server** / **Bridge**. The existing `GET /api/v1/logs` handler already accepts a `source` query parameter (line 73 of `handler/logs.go`) and the `LogFilter` struct already has a `Source` field — no backend query changes needed. Bridge logs appear in real-time alongside server logs via the existing WebSocket log stream.

---

## 2. Unified Telegram Architecture

### Overview

The bridge connector owns the Telegram connection (bot token, chat IDs, topic IDs). The server notification system uses it as a delivery target. Event forwarding moves from the bridge to the server. The bridge Telegram connector becomes a command-only listener.

### Data Model Changes

**`bridge_connectors` table** — unchanged. Remains the single source of truth for bot token, chat/group ID, topic IDs.

**`notification_channels` table** — add column:

```sql
ALTER TABLE notification_channels ADD COLUMN connector_id TEXT NULL;
```

**Go struct updates:**
- `store/notifications.go`: Add `ConnectorID *string` field to `NotificationChannel` struct. Update all `Scan` calls (at least 4 queries) to include the new column.
- `store/notifications.go`: Add `ConnectorID *string` field to `ChannelSubscription` struct returned by `ListSubscriptionsForEvent()` — the notification dispatcher needs this to resolve the bot token.

**TypeScript type updates:**
- `web/src/types/notification.ts`: Add `connector_id?: string` to `NotificationChannel` type.

When a Telegram connector is created on the Connectors page, the server auto-creates a corresponding `notification_channel` record with `connector_id` set and `channel_type = 'telegram'`. This channel appears in the notification subscription matrix. Updating or deleting the connector cascades to the notification channel.

### Telegram Connector Config

**Before** (bridge owns everything):
```json
{
  "group_id": -1001234567890,
  "events_topic_id": 1,
  "commands_topic_id": 2,
  "poll_timeout": 30,
  "event_types": ["session.*", "run.*"]
}
```

**After** (bridge owns connection + commands):
```json
{
  "group_id": -1001234567890,
  "events_topic_id": 1,
  "commands_topic_id": 2,
  "commands_enabled": true,
  "poll_timeout": 30
}
```

`event_types` is removed. Event routing is controlled entirely by the notification subscription matrix in Settings.

### Event Delivery Flow (outbound)

```
Server Event Bus
  -> Notification Dispatcher
     -> checks notification_subscriptions for matching channels
     -> resolves channel config:
        -> Email channel? -> SMTP notifier (unchanged)
        -> Telegram channel (connector_id set)?
           -> dispatcher resolves bot_token + chat_id from bridge_connectors
           -> builds merged config JSON (bot_token, chat_id, topic_id)
           -> passes merged config to TelegramNotifier.Send()
```

**Connector resolution approach:** The `Dispatcher` receives a narrow `ConnectorResolver` interface (defined in the `notify` package, not importing from `handler`):

```go
// notify/connector_resolver.go
type ConnectorResolver interface {
    // ResolveConnectorConfig returns the merged config (bot_token, chat_id, topic_id)
    // for a connector-backed notification channel. Handles secret decryption internally.
    ResolveConnectorConfig(ctx context.Context, connectorID string) (string, error)
}
```

The implementation lives in `store/` or a thin adapter, and receives the encryption key at construction — keeping the key out of the `notify` package. When processing a `ChannelSubscription` with a non-nil `ConnectorID`, the dispatcher:
1. Calls `ConnectorResolver.ResolveConnectorConfig(ctx, connectorID)` which internally fetches the connector, decrypts `config_secret`, reads `bot_token`, reads `group_id` (as `chat_id`) and `events_topic_id` (as `topic_id`) from config, and returns a merged JSON string.
2. Passes the merged config JSON to `TelegramNotifier.Send()`.

This keeps the `Notifier.Send(ctx, channelConfig, subject, body)` interface unchanged. The dispatcher handles the resolution, not the notifier. The notifier remains stateless. The encryption key stays in the store layer.

**Telegram message formatting:** The bridge's `FormatEvent()` in `formatter.go` produces rich MarkdownV2 messages with emojis and structured fields per event type. The server's `DefaultEventRenderer` produces plain `subject + body` text, and the existing `TelegramNotifier.Send()` sends with `parse_mode: "HTML"`.

To preserve notification quality, `FormatEvent()` logic is ported to the server side as a `TelegramEventRenderer`. During the port, the output format is converted from **MarkdownV2 to HTML** to match the existing notifier's `parse_mode: "HTML"`. This means replacing MarkdownV2 escaping (`escapeMarkdownV2()`) with `html.EscapeString()`, `*bold*` with `<b>bold</b>`, etc. The rich structure (emojis, per-event-type layouts) is preserved — only the markup syntax changes.

The `EventRenderer` type signature:

```go
// notify/renderer.go
type EventRenderer func(e event.Event) (subject string, body string)
```

The dispatcher constructor accepts `renderers map[string]EventRenderer` keyed by channel type, falling back to `DefaultEventRenderer`. For Telegram channels, the rendered `body` contains the full formatted HTML message; the `subject` is used only for logging/debugging.

The notification dispatcher handles ALL outbound Telegram delivery. The bridge no longer polls the events feed or forwards events. The following bridge code paths are removed:
- `pollAndForwardEvents()` in `telegram.go`
- `ShouldForwardEvent()` / `MatchEventType()` in `formatter.go`
- Telegram-specific event cursor tracking in state store. **Do NOT remove** `GetCursor`/`SetCursor`/`IsProcessed`/`MarkProcessed` from `state/state.go` — the GitHub connector depends on these methods for its polling state.

### Command Listening Flow (inbound)

```
Telegram API <-(long-poll)- Bridge Telegram Connector
  -> parse /command
  -> dispatch to handler (/sessions, /kill, /inject, /status, etc.)
  -> call Server REST API
```

The bridge Telegram connector becomes a command-only listener. It reads the bot token from the server (as it already does via connector config), long-polls Telegram for messages in the `commands_topic_id` thread, and dispatches commands.

### Auto-Sync Logic

Server-side, in the bridge connector handler (`handler/bridge.go`):

- **On connector create** (type=telegram): Within a **single database transaction**, create the `bridge_connector` row and auto-create a `notification_channel` with `connector_id` pointing to the new connector. The notification channel config stores `topic_id` (mapped from the connector's `events_topic_id`) for delivery routing. If either insert fails, the transaction rolls back — no orphan records.
- **On connector update**: Within a transaction, update both the connector and the linked notification channel (syncing `topic_id` from `events_topic_id`, channel name from connector name).
- **On connector delete**: Delete the linked notification channel. Subscription cleanup relies on the existing `ON DELETE CASCADE` foreign key constraint on `notification_subscriptions.channel_id` (migration version 17).

**Field mapping between connector config and notification channel config:**

| Connector config field | Notification channel config field |
|---|---|
| `group_id` | `chat_id` |
| `events_topic_id` | `topic_id` |
| (bot_token in config_secret) | (resolved at dispatch time via connector_id, not duplicated) |

### Server Startup Wiring

In `cmd/server/main.go`, the notifier map and dispatcher construction change:

```go
// Before
notifiers := map[string]notify.Notifier{
    "email":    &notify.SMTPNotifier{},
    "telegram": notify.NewTelegramNotifier(nil),
}
notifyDispatcher := notify.NewDispatcher(s, notifiers, notify.DefaultEventRenderer, slog.Default())

// After
notifiers := map[string]notify.Notifier{
    "email":    &notify.SMTPNotifier{},
    "telegram": notify.NewTelegramNotifier(nil), // unchanged — still stateless
}
renderers := map[string]notify.EventRenderer{
    "telegram": notify.TelegramEventRenderer, // ported from bridge FormatEvent()
}
notifyDispatcher := notify.NewDispatcher(s, bridgeStore, notifiers, renderers, slog.Default())
//                                          ^^^^^^^^^^^          ^^^^^^^^^
//                                          new dependency       per-channel-type renderers
```

---

## 3. Frontend UX

### Connectors Page

**Telegram connector creation form** (`TelegramForm.tsx`) — simplified fields:
- Name (required)
- Bot Token (required, password field)
- Group/Chat ID (required)
- Events Topic ID (optional — for forum-style groups)
- Commands Topic ID (optional — for forum-style groups)
- Commands Enabled toggle (default: on)
- Poll Timeout (collapsed into "Advanced" section)

No event_types textbox. Event routing is handled in Settings > Notifications.

**Connector detail page** — after creation, clicking a connector opens a detail view showing:
- Connection status (healthy/error from bridge health data)
- Last seen / uptime
- Edit button to modify config
- If commands are enabled: list of available commands with descriptions
- Link to Settings > Notifications: "Configure which events are sent to this connector"

**Connector list status indicators:**
- Green dot + "Active" — bridge running, connector healthy
- Yellow dot + "Commands only" — commands enabled but no event subscriptions configured
- Gray dot + "No subscriptions" — connector exists but no events routed to it
- Red dot + "Disconnected" — bridge not reporting health

### Settings > Notifications

**Channel list section** (`NotificationsTab.tsx`):
- Telegram connectors auto-appear as channels with a "Connector" badge and link icon
- These channels cannot be edited or deleted from this page — tooltip: "Managed from Connectors page" with link
- "Add Channel" button only offers Email (SMTP)
- If no Telegram connector exists: info banner — "Want Telegram notifications? Set up a Telegram connector first." with link to Connectors page

**Subscription matrix** — unchanged. The auto-created Telegram channel appears as a column alongside any email channels. Users tick checkboxes per event per channel.

**Channel form modal** (`ChannelFormModal.tsx`):
- Remove the Telegram tab entirely (lines 171-185 tab buttons, lines 296-346 Telegram config section)
- The `TelegramConfig` type, state, and parser are removed from this component
- Only Email (SMTP) configuration remains

### Event Types Frontend

New bridge event types must be added to:
- `web/src/constants/eventTypes.ts` — add constants and include in `ALL_EVENT_TYPES`
- Add a new **"Bridge"** group in `EVENT_GROUPS` array so bridge events appear as a section in the notification subscription matrix
- CI validates sync between `event_types.json` and frontend constants via `go generate`

---

## 4. Bridge Lifecycle Events

### New Event Types

Added to `internal/server/event/event_types.json` and emitted by the bridge via a new `POST /api/v1/ingest/events` endpoint:

| Event | Payload | When |
|-------|---------|------|
| `bridge.started` | `{version}` | Bridge process starts |
| `bridge.stopped` | `{reason}` | Bridge process shuts down |
| `bridge.connector.started` | `{connector_id, connector_type, name}` | A connector initializes successfully |
| `bridge.connector.error` | `{connector_id, connector_type, name, error}` | A connector fails to start or hits a runtime error |
| `bridge.connector.command` | `{connector_id, command, sender, result}` | A Telegram command was received and executed |

### Event Ingestion Endpoint

A **new** handler registered in the API key-authenticated route group:

```
POST /api/v1/ingest/events
Authorization: Bearer <api-key>
Content-Type: application/json

{
  "events": [
    {
      "type": "bridge.started",
      "payload": {"version": "0.8.0"}
    }
  ]
}
```

**Critical implementation detail:** This endpoint must **publish events to the `event.Bus`** (not just insert into the `events` table). Publishing to the bus triggers the full event pipeline:
- Notification dispatcher → Telegram/email delivery
- Webhook delivery with retry
- WebSocket event stream → frontend cache invalidation
- Events table persistence (handled by the bus's store subscriber)

The handler calls `eventBus.Publish(event)` for each ingested event, which fans out to all subscribers. This is the same path server-originated events take.

### Bridge Status Endpoint

`GET /api/v1/bridge/status` extended to return:

```json
{
  "running": true,
  "last_seen": "2026-03-21T10:15:30Z",
  "connectors": [
    {
      "connector_id": "abc-123",
      "name": "production-telegram",
      "type": "telegram",
      "healthy": true,
      "last_error": null
    }
  ]
}
```

**Derivation logic:**
- `running`: `true` if a `bridge.started` event exists with no subsequent `bridge.stopped`, AND `last_seen` is within 60 seconds.
- `last_seen`: timestamp of the most recent bridge event or log entry.
- `connectors[].healthy`: `true` if the most recent event for this connector is `bridge.connector.started` (not `bridge.connector.error`).
- `connectors[].last_error`: error message from the most recent `bridge.connector.error` event, or `null`.
- If the retention policy has purged old events, `running` falls back to `false` (safe default). The bridge re-emits `bridge.started` on reconnection, so this self-heals.

The Command Center dashboard shows a bridge status indicator using this data.

---

## Migration Path

### Database

1. Add `connector_id TEXT NULL` column to `notification_channels`
2. Register new event types in `event_types.json`: `bridge.started`, `bridge.stopped`, `bridge.connector.started`, `bridge.connector.error`, `bridge.connector.command`
3. Run `go generate ./internal/server/event/...` to regenerate event type constants

### Bridge Binary

1. Add `slog.Handler` log forwarder (`internal/bridge/log_forwarder.go`) — buffer + flush to `POST /api/v1/ingest/logs`
2. Add event emitter (`internal/bridge/event_emitter.go`) — startup/shutdown/connector lifecycle to `POST /api/v1/ingest/events`
3. Strip event polling/forwarding from Telegram connector — remove `pollAndForwardEvents()`, keep command listener only
4. Add `commands_enabled` config field, remove `event_types` field
5. Remove only Telegram-specific cursor usage — do NOT remove cursor/processed methods from `state.Store` (GitHub connector depends on them)

### Server Binary

1. Add `BridgeIngestHandler` struct (separate from existing `IngestHandler`) with `HandleLogs` and `HandleEvents` methods — API key auth, log entries via `LogStore.InsertBatch()` + `LogBroadcaster.Broadcast()`, events via `eventBus.Publish()`
2. Modify bridge connector handler (`handler/bridge.go`): auto-create/update/delete linked notification channel on Telegram connector CRUD, wrapped in database transactions
3. Add `ConnectorResolver` interface in `notify/` package — implemented by store layer, handles secret decryption internally (encryption key stays in store)
4. Modify notification dispatcher (`notify/dispatcher.go`): accept `ConnectorResolver` dependency, resolve `connector_id` to merged config at dispatch time
5. Port `FormatEvent()` from bridge to server as `TelegramEventRenderer` in `notify/telegram.go` — convert MarkdownV2 output to HTML to match existing `parse_mode: "HTML"`
6. Add `EventRenderer` type and per-channel-type renderer map to `Dispatcher` constructor
7. Update `NotificationChannel` and `ChannelSubscription` Go structs with `ConnectorID` field
8. Update all `Scan` queries in `store/notifications.go` to include `connector_id`
9. Extend `GET /api/v1/bridge/status` with connector health data derived from bridge events
10. Update notifier wiring in `cmd/server/main.go` — new `BridgeIngestHandler` registration, `ConnectorResolver` impl, renderer map, updated `Dispatcher` constructor

### Frontend

1. Simplify `TelegramForm.tsx` — remove `event_types` textbox, add `commands_enabled` toggle
2. Add connector detail page with status, commands list, link to notifications
3. Modify `NotificationsTab.tsx` — connector-backed channels show "Connector" badge, non-editable; "Add Channel" only offers email; info banner when no Telegram connector exists
4. Modify `ChannelFormModal.tsx` — remove Telegram tab entirely
5. Add source filter dropdown to Logs page (`LogsPage.tsx`)
6. Add bridge status indicator to Command Center dashboard
7. Add connector status dots to connector list
8. Add "Bridge" group to `EVENT_GROUPS` in `web/src/constants/eventTypes.ts`
9. Update `NotificationChannel` TypeScript type with `connector_id` field

### Backward Compatibility

- Existing Telegram connectors with `event_types` in config continue to work during migration — the bridge ignores the field once event forwarding is removed
- Existing notification channels of type `telegram` (without `connector_id`) remain functional but are marked as "Legacy" in the UI with a prompt to migrate to a connector-backed channel
- Legacy Telegram notification channels retain their own `bot_token` in config and continue to work via the existing `TelegramNotifier.Send()` path (no connector resolution needed when `connector_id` is nil)
