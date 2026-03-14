# Verified Bug Fixes — Design Spec

**Date:** 2026-03-14
**Scope:** 14 confirmed bugs from codebase audit, single branch/PR
**Branch:** `fix/verified-bug-fixes`

---

## Overview

Fix 14 verified bugs across four severity levels. All fixes ship in a single branch. Ordered by priority within each group; groups are worked P0 → P1 → P2 → P3.

---

## P0 — Critical

### B01: Injection queue `Close()` graceful shutdown

**File:** `internal/server/session/injection_queue.go`

**Problem:** `Close()` signals drainers via `close(sq.done)` but never joins on them. In-flight DB writes race with `store.Close()`. Buffered-but-undelivered injections are silently discarded.

**Fix:**
- Add `sync.WaitGroup` to `InjectionQueue` struct.
- Increment in `getOrCreateQueue` when spawning a drainer goroutine.
- `defer wg.Done()` at top of `drainSession`.
- Restructure `drainSession`: after `sq.done` fires, exit the main select loop and enter a non-blocking drain loop over `sq.items`. For each remaining item, do **not** attempt gRPC delivery (the session may already be gone). Instead, mark each as failed via a new `UpdateInjectionFailed(ctx, injectionID, reason string)` method on `InjectionAuditStore`, and log at WARN level with the injection ID and session ID.
- `Close()` calls `wg.Wait()` after signaling all `done` channels, ensuring all drainers have finished their drain loops and DB writes before returning.

**Interaction with B12:** During the shutdown drain loop, items are marked as failed without attempting delivery — consistent with B12's approach of checking session status before sending. B12's `processItem` re-check applies during normal operation; B01's drain loop applies only during shutdown.

### B02: Enforce API key scopes for connector secrets

**Files:** `internal/server/api/middleware.go`, `internal/server/handler/bridge.go`, `internal/server/store/apikeys.go`

**Problem:** `APIKey.Scopes` field exists in the DB but is never enforced. Any admin API key can call `GET /api/v1/bridge/connectors` and receive decrypted Telegram bot tokens and GitHub PATs.

**Fix:**
1. **Propagate scopes to context:** Modify `validateAPIKeyToken` in `middleware.go` to store `APIKey.Scopes` in the request context. Either extend `auth.Claims` to include a `Scopes []string` field, or store scopes separately via a dedicated context key (e.g., `contextKeyScopes`). Parse the comma-separated `Scopes` string from the DB into a `[]string` at this point.
2. **Scope-checking middleware:** Create a `RequireScope(scope string)` middleware that reads scopes from the request context (set in step 1) and rejects with 403 if the required scope is absent. For JWT-authenticated requests (no scopes), this middleware is a no-op — JWT users are already gated by `authorizeAdmin` and the existing `IsAPIKeyAuth` check prevents them from seeing secrets.
3. **Mask secrets by default:** In `toConnectorResponse`, omit `ConfigSecret` unless both `IsAPIKeyAuth(r)` returns true AND the request context has `connectors:read_secret` scope. This layers on top of the existing `IsAPIKeyAuth` guard rather than replacing it — JWT-authenticated admins still cannot see secrets (preserving current behavior).
4. **Apply to bridge handler:** Gate secret inclusion within `ListConnectors`/`GetConnector` based on scope presence in context. Only call `GetConnectorWithSecret` when scope is present.
5. **Bridge provisioning:** Update provisioning/seed tooling so bridge API keys are created with `connectors:read_secret` scope. Document the required scope.

### B03: Dedicated persist delivery path in event bus

**Files:** `internal/server/event/bus.go`, `internal/server/event/persist.go`

**Problem:** `Publish()` uses non-blocking `select/default` for all subscribers including persist. If persist's buffer (1024) fills, events are dropped — causing gaps in the audit trail and missing webhook deliveries.

**Fix:**
- Add a `persistHandler func(Event)` field to `Bus` (set via constructor option).
- In `Publish()`, call `persistHandler(event)` **outside** the `b.mu.RLock()` section — release the read lock first, call persist synchronously, then re-acquire the read lock for the non-blocking fan-out. This avoids blocking concurrent publishers on SQLite writes while still guaranteeing event persistence before fan-out.
- Remove the persist subscriber from the regular subscriber list so it's not double-delivered.
- If `persistHandler` fails, log at ERROR level. The event is still delivered to other subscribers so real-time features (WS fanout) are not blocked.
- Other subscribers (WS fanout, webhook deliverer) keep non-blocking delivery unchanged.

**Positive interaction with B08:** Because persist is now synchronous, the webhook retry loop's `GetEventByID` call is guaranteed to find the event — eliminating the double-failure mode where both persist and webhook drop the same event.

---

## P1 — Bugs

### B05: `/start` fallback to template's default machine_id

**Files:** `internal/server/store/migrations.go`, `internal/server/store/templates.go`, `internal/server/handler/templates.go`, `internal/bridge/connector/telegram/telegram.go`, `internal/bridge/client/client.go`

**Problem:** `/start` requires `<template_name> <machine_id>`. No fallback to the template's default `machine_id`. The `session_templates` table does not currently have a `machine_id` column.

**Fix:**
- **New migration (version 8):** Add `machine_id TEXT DEFAULT '' NOT NULL` column to `session_templates` using `ALTER TABLE`. Use `IF NOT EXISTS` pattern consistent with other migrations (though `ALTER TABLE ADD COLUMN` doesn't support it in SQLite — use a check: query `PRAGMA table_info(session_templates)` and skip if column exists, or wrap in a migration guard).
- **Update store:** Add `MachineID` field to the `SessionTemplate` struct. Update Create/Update/Get queries to include the column.
- **Update handler:** Accept `machine_id` in template create/update request bodies. Return it in responses.
- **Update bridge client:** Add `MachineID string` field to `Template` struct.
- **Update `handleStart`:** Accept `/start <template> [machine]`. When machine is omitted, look up template and use its `MachineID`. If both are empty, return `"Template has no default machine. Usage: /start <template> <machine>"`.

### B06: Migrations 5, 6, and 7 — defensive SQL

**File:** `internal/server/store/migrations.go`

**Problem:** Migrations 5, 6, and 7 use `CREATE TABLE` and `CREATE INDEX` without `IF NOT EXISTS`, unlike migrations 1–4.

**Fix:** Add `IF NOT EXISTS` to all `CREATE TABLE` and `CREATE INDEX` statements in migrations 5, 6, and 7:
- Migration 5: `session_templates` table + `idx_templates_user` index
- Migration 6: `injections` table + `idx_injections_session` index
- Migration 7: `api_keys` table + `idx_api_keys_user` index + `bridge_connectors` table + `bridge_control` table

### B08: Webhook retry jitter

**File:** `internal/server/event/webhook_delivery.go`

**Problem:** `retryBackoff()` returns fixed durations (1s, 5s, 30s) with no randomization. Simultaneous failures create thundering herd retries.

**Fix:** Add jitter using `rand.Int64N` (available since Go 1.22, project uses Go 1.25): `base + time.Duration(rand.Int64N(int64(base/2)))`.
- Attempt 1: 1s + rand(0–500ms)
- Attempt 2: 5s + rand(0–2500ms)
- Attempt 3+: 30s + rand(0–15s)

---

## P2 — Quality / Maintainability

### B12: Enqueue TOCTOU mitigation

**File:** `internal/server/session/injection_queue.go`

**Problem:** `Enqueue` checks session status then calls `getOrCreateQueue`. Session can exit between these calls, creating an orphaned drainer.

**Fix:**
- Keep the `Enqueue` status check as a fast-path rejection.
- Add a status re-check inside `processItem` before sending the injection via gRPC: call `sessionStore.GetSession(sessionID)` and check `isTerminalStatus`.
- If session is in a terminal state at delivery time, call `UpdateInjectionFailed(ctx, injectionID, "session terminated")` (same new method from B01) and return without sending.

### B13: Configurable event retention

**Files:** `internal/server/event/retention.go`, `internal/server/config/config.go`, `cmd/server/main.go`

**Problem:** Retention period is hardcoded to 7 days. Not configurable via `server.toml`.

**Fix:**
- Add `[events]` section to `ServerConfig`:
  ```go
  type EventsConfig struct {
      RetentionDays int `toml:"retention_days"` // default 7
  }
  ```
- Add validation in `Validate()`: if `RetentionDays <= 0`, set to 7 (prevents accidental purge-all).
- Change `NewRetentionCleaner` to accept a `maxAge time.Duration` parameter.
- Pass `time.Duration(cfg.Events.RetentionDays) * 24 * time.Hour` from `cmd/server/main.go`.
- Log pruned row count at INFO level on each cleanup cycle.

### B14: By-name template endpoint

**Files:** `internal/server/handler/templates.go`, `internal/server/store/templates.go`, `internal/bridge/connector/telegram/telegram.go`, `internal/bridge/client/client.go`

**Problem:** No dedicated by-name lookup. Telegram connector does list + linear scan.

**Fix:**
- Add `GET /api/v1/templates/by-name/{name}` route in `RegisterTemplateRoutes`.
- The existing store method `GetTemplateByName(ctx, userID, name)` requires a `userID`. For API-key-authenticated requests, the `userID` is available from the `auth.Claims` in the request context (API keys are associated with a user). Use this `userID` to maintain existing user-scoping — bridge API keys see templates belonging to their associated user.
- Handler: call `store.GetTemplateByName(ctx, claims.UserID, name)`, return single object or 404.
- Add `GetTemplateByName(ctx, name string) (*Template, error)` to bridge client, calling `GET /api/v1/templates/by-name/{name}`.
- Update Telegram connector `handleStart` to use the new client method instead of list+scan.

### B15: Update PRD variable syntax documentation

**Files:** `docs/internal/product/*.md`

**Problem:** PRD specifies `{{.VarName}}` but code uses `${VAR_NAME}`. Code is correct and internally consistent.

**Fix:** Update PRD documentation to reference `${VAR_NAME}` syntax. No code changes.

---

## P3 — Nice to Have

### B18: Telegram 429 handling

**File:** `internal/bridge/connector/telegram/telegram.go`

**Problem:** No HTTP status code inspection. 429 responses from Telegram Bot API are not handled.

**Fix:**
- After each HTTP call in `sendMessage()` and `getUpdates()`, check `resp.StatusCode`.
- On 429: parse `retry_after` from response JSON body. Use context-aware waiting instead of `time.Sleep`:
  ```go
  select {
  case <-time.After(retryAfter):
  case <-ctx.Done():
      return ctx.Err()
  }
  ```
  This avoids blocking shutdown when the context is cancelled.
- On other non-2xx: return error with status code context.

### B19: Switch Telegram to MarkdownV2

**File:** `internal/bridge/connector/telegram/telegram.go`

**Problem:** Uses deprecated `"parse_mode": "Markdown"` (V1).

**Fix:**
- Change to `"parse_mode": "MarkdownV2"`.
- Add `escapeMarkdownV2(s string) string` helper that escapes required special characters: `_ * [ ] ( ) ~ > # + - = | { } . !`.
- Apply escaper to all dynamic content in these message-building functions: `handleList`, `handleMachines`, `handleKill`, `handleInject`, `handleStart`, `helpText`, and `FormatEvent` (if it exists in the file; verify during implementation). Keep intentional formatting markup (bold `*text*`, code backticks) unescaped — only escape within the dynamic values interpolated into those templates.

### B20: Health endpoint address validation

**File:** `internal/bridge/bridge.go`

**Problem:** Empty or invalid `healthAddr` causes silent failure — bridge continues without health endpoint.

**Fix:**
- Validate `healthAddr` in `Run()`/`RunWithInterval()` before starting the HTTP server.
- If empty, default to `localhost:9091` and log at WARN. (Port 9091 chosen to avoid conflict with gRPC which commonly uses 9090.)
- Start health server and wait briefly for it to bind. If `ListenAndServe` returns an error, propagate it as a startup failure rather than logging and continuing.

---

## Testing Strategy

Each fix includes unit tests covering the changed behavior:
- **B01:** Test that `Close()` blocks until drainers exit. Test that buffered items are marked as failed via `UpdateInjectionFailed` (not silently discarded). Test that `wg.Wait()` completes even when drainers are mid-delay.
- **B02:** Test that `validateAPIKeyToken` propagates scopes to context. Test `RequireScope` middleware (valid scope passes, missing scope returns 403, JWT requests pass through). Test that secrets are masked without `connectors:read_secret` scope. Test that secrets are returned with scope present.
- **B03:** Test that persist handler is called synchronously and completes before fan-out. Test that persist failure (error return) doesn't block other subscribers. Test that other subscribers still use non-blocking delivery.
- **B05:** Test `/start` with explicit machine_id arg. Test `/start` with omitted machine_id falls back to template default. Test error message when both are empty. Test new migration adds column correctly.
- **B06:** Run migrations 5, 6, 7 against a DB that already has the tables — should not error with `IF NOT EXISTS`.
- **B08:** Test that `retryBackoff` returns values within expected jitter range over multiple calls (statistical: min ≥ base, max ≤ base * 1.5).
- **B12:** Test `processItem` rejects delivery to terminal-state session and calls `UpdateInjectionFailed`.
- **B13:** Test retention cleaner uses configured duration. Test default fallback when `RetentionDays <= 0`.
- **B14:** Test by-name endpoint returns single template, 404 for missing. Test user-scoping (can't see other user's templates).
- **B18:** Test 429 handling parses `retry_after` correctly. Test context cancellation during retry wait.
- **B19:** Test `escapeMarkdownV2` escapes all 12 required characters. Test that formatting markup is preserved.
- **B20:** Test empty address defaults to `localhost:9091`. Test invalid address returns startup error.

## Out of Scope

- B04 (not a bug), B07 (already implemented), B09 (idiomatic), B10 (exaggerated), B16 (already iterative), B17 (cookies correct), B21 (partial — separate improvement), B22 (README cosmetic).
- Refactoring `cmd/server/main.go` adapter structs (B10) or `NewRouter` signature (B11) — valid improvement but not a bug fix.
