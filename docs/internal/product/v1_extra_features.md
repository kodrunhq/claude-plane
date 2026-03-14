# claude-plane V2: Session Injection, Templates & Bridge

**Version:** 0.2.0-draft
**Author:** José / Claude (Opus)
**Date:** 2026-03-14
**Status:** Proposal
**Depends on:** backend_v1.md, frontend_v1.md

---

## 1. Overview

This document specifies three features that compose into a single automation story: **Templates** define reusable session configurations, **Session Injection** pushes data into running sessions, and the **Polling Bridge** connects claude-plane to external services (Telegram, GitHub) without exposing the server to the internet.

Together they transform claude-plane from a manual session launcher into an event-driven platform: an external event (PR opened, CI failed, Telegram command) triggers a template-based session, and injection feeds context into it mid-flight — all without a single inbound port.

### Design Principles

These features follow the same principles established in V1:

1. **No inbound exposure.** The server stays behind NATs and firewalls. External integrations use outbound-only polling.
2. **Composability over coupling.** Each feature is useful standalone. Templates work without the bridge. Injection works without templates. The bridge ties them together but doesn't require either.
3. **Single-user-first, multi-user-safe.** Defaults are optimized for the homelab operator running 1–5 machines. Authorization rules are ready for teams.
4. **No YAML pipelines.** Configuration is TOML for infra, REST API for workflows, UI for daily use.

---

## 2. Session Templates

### 2.1 Problem

Every session creation today requires specifying command, args, working directory, environment variables, and terminal size. Real usage is repetitive: the same person runs the same setup against the same repo dozens of times a day. The job system has steps with prompts, but a job is heavyweight for "run Claude in this repo with this context."

### 2.2 What a Template Is

A session template is a named, reusable configuration that captures everything needed to launch a session in a single identifier. Templates are first-class resources with their own CRUD lifecycle, not embedded inside jobs.

A template contains:

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique human-readable slug. Used as the identifier in Telegram commands, API calls, and the UI. Example: `review-pr`, `fix-ci`, `refactor-backend`. |
| `description` | No | One-line explanation shown in the UI card and Telegram `/list` response. |
| `command` | No | CLI binary to execute. Defaults to `claude`. Could also be `claude-code`, `aider`, or any PTY-compatible process. |
| `args` | No | Arguments passed to the command. JSON array. Example: `["--model", "opus", "--yes"]`. |
| `working_dir` | No | Absolute path on the agent machine. If empty, the agent uses its own configured default. |
| `env_vars` | No | Additional environment variables injected into the PTY process. JSON object. Credential references (see 2.5) are resolved at session creation time, not stored in the template. |
| `initial_prompt` | No | Text piped into the session's stdin immediately after launch. Supports variable placeholders (see 2.4). |
| `machine_id` | No | Preferred machine for this template. Soft affinity — if the machine is offline or at capacity, the request falls through to manual selection or fails with a clear error. Not auto-routed to a random machine. |
| `terminal_rows` | No | Default: `24`. |
| `terminal_cols` | No | Default: `80`. |
| `tags` | No | JSON array of free-form strings for filtering and grouping in the UI. Example: `["ci", "kodrun"]`. |
| `max_cost_usd` | No | Budget cap for sessions launched from this template. When the session's accumulated cost crosses this threshold, the server sends a `KillSession` command. Requires cost tracking to be wired (see V2 cost tracking). `null` means no cap. |
| `timeout_seconds` | No | Hard timeout. Session is killed after this many seconds regardless of state. `0` means no timeout. |

### 2.3 Template Lifecycle

Templates are owned by the user who created them. Admins can see and manage all templates. Regular users see only their own.

| Operation | Endpoint | Notes |
|-----------|----------|-------|
| Create | `POST /api/v1/templates` | Validates `name` uniqueness per user. |
| List | `GET /api/v1/templates` | Filterable by `?tag=ci` and `?machine_id=nuc-01`. |
| Get | `GET /api/v1/templates/{templateID}` | Also supports `GET /api/v1/templates/by-name/{name}` for ergonomic use from the bridge and CLI. |
| Update | `PUT /api/v1/templates/{templateID}` | Full replacement. No PATCH — templates are small enough that partial updates add complexity without value. |
| Delete | `DELETE /api/v1/templates/{templateID}` | Soft delete (sets `deleted_at`). Templates referenced by active jobs are not hard-deleted. |
| Clone | `POST /api/v1/templates/{templateID}/clone` | Creates a copy with `name` suffixed `-copy`. Useful for quick iteration. |

### 2.4 Variable Placeholders in `initial_prompt`

The `initial_prompt` field supports `${VAR_NAME}` placeholders. These are **not** resolved by the server. The server stores the template verbatim — interpolation is the caller's responsibility.

This means:

- The **bridge** substitutes variables before calling the session creation API. A GitHub connector replaces `${PR_URL}`, `${PR_TITLE}`, `${PR_DIFF_URL}`, `${CI_LOG}` etc.
- The **REST API** accepts an optional `variables` map in the session creation request. The server performs simple string replacement on the template's `initial_prompt` before passing it to the agent.
- The **UI** shows a form with text inputs for each detected `${VAR_NAME}` placeholder when launching from a template that contains them.

Unresolved placeholders are left as-is (not stripped, not errored). This is intentional — Claude can often work with the placeholder name as context.

Variable names must match `[A-Z][A-Z0-9_]*`. The server validates this on template creation and rejects names that don't match. This prevents injection via crafted variable names.

### 2.5 Credential References in `env_vars`

Environment variable values that start with `$cred:` are resolved at session creation time from the server's encrypted credential store.

Example template `env_vars`:

```json
{
  "ANTHROPIC_API_KEY": "$cred:anthropic-key",
  "GITHUB_TOKEN": "$cred:github-pat",
  "CUSTOM_FLAG": "literal-value"
}
```

At session creation, the server:

1. Iterates `env_vars` and identifies `$cred:` prefixed values.
2. Looks up each credential by name from the `credentials` table for the requesting user.
3. Decrypts the value (AES-256-GCM, key from server config).
4. Replaces the reference with the decrypted value in the `CreateSessionCmd` sent to the agent.
5. The decrypted value is never stored in the `sessions` table, never logged, and never returned in any API response.

If a referenced credential doesn't exist, session creation fails with a `400` and a clear error naming the missing credential. This is a hard failure, not a warning.

### 2.6 Template-Aware Session Creation

The existing `POST /api/v1/sessions` endpoint gains two new optional fields:

| Field | Type | Description |
|-------|------|-------------|
| `template_id` | string | UUID of the template to use. |
| `template_name` | string | Name of the template (convenience alias). If both are provided, `template_id` wins. |
| `variables` | object | Key-value map for placeholder substitution in `initial_prompt`. |

**Merge semantics:** The template provides defaults. Any field explicitly set in the request body overrides the template value. This allows the bridge to use a template but override `working_dir` for a specific repo, or the UI to let the user tweak args before launch.

Merge order: `template defaults → request body overrides → credential resolution → variable substitution`.

### 2.7 Frontend Integration

- **Command Center:** Templates appear as a card grid below the machine list. Each card shows name, description, tags, preferred machine, and a "Launch" button. If the template has placeholders, clicking Launch opens a modal with input fields for each variable.
- **Template Editor:** A dedicated page (`/templates/{id}/edit`) with a form for all template fields. The `initial_prompt` field uses a textarea with syntax highlighting for `${VAR_NAME}` placeholders. A "Test Launch" button creates a session with `dry_run=true` (validates everything, returns what would be sent to the agent, but doesn't actually create the session).
- **Session creation modal:** Gains a "From Template" dropdown at the top. Selecting a template pre-fills all fields. The user can still override before confirming.

---

## 3. Session Injection

### 3.1 Problem

Today, the only way to give a running Claude session new information is to type it into the terminal via the WebSocket connection. There's no programmatic way to push context into an active session. This means:

- If CI fails after a session is already running, you have to manually copy-paste the log.
- If a second event occurs that's relevant to the current session, you either interrupt Claude or start a new session.
- Automation (bridge, webhooks, cron) can't interact with sessions that already exist.

### 3.2 What Injection Is

Session injection writes arbitrary text into the stdin of a running PTY session, exactly as if the user had typed it. The text is queued and delivered in order. The session's scrollback captures both the injected input and Claude's response, maintaining a complete audit trail.

Injection is **not** special — it's the same `InputDataCmd` that keystroke relay uses. The difference is the entry point: a REST endpoint instead of a WebSocket frame.

### 3.3 API

```
POST /api/v1/sessions/{sessionID}/inject
```

**Request body:**

| Field | Required | Description |
|-------|----------|-------------|
| `text` | Yes | The text to inject. Delivered as-is to the PTY stdin. A trailing newline is appended automatically unless `raw` is `true`. |
| `raw` | No | Default: `false`. When `true`, no newline is appended. Useful for sending control sequences or partial input. |
| `delay_ms` | No | Default: `0`. Delay in milliseconds before injection. Useful when chaining multiple injections to give Claude time to process. |
| `metadata` | No | Free-form JSON object stored in the audit log but not sent to the session. Example: `{"source": "github", "pr": 42}`. |

**Response:**

- `202 Accepted` — Injection queued. Body contains `{"injection_id": "...", "queued_at": "..."}`.
- `404 Not Found` — Session doesn't exist or caller doesn't have access.
- `409 Conflict` — Session is not in `running` state (exited, terminated, etc.).
- `503 Service Unavailable` — Agent for this session is disconnected.

**Why 202 and not 200:** The server forwards the input to the agent via gRPC, but doesn't wait for Claude to process it. The injection is fire-and-deliver — confirmation means the bytes were sent to the agent, not that Claude has responded.

### 3.4 Injection Queue

When multiple injections arrive in quick succession (e.g., bridge sends PR diff then CI log), they must be delivered in order and without interleaving.

The server maintains a per-session injection queue in memory (not persisted — if the server restarts, pending injections are lost, which is acceptable since the PTY session itself survives on the agent and the user can re-inject).

Queue behavior:

- Injections are appended with their `delay_ms` value.
- A single goroutine per session drains the queue, sending each injection to the agent with the specified delay between items.
- If the agent is temporarily disconnected, the queue pauses and resumes on reconnection.
- Queue depth is capped at 32 items. If the queue is full, new injections return `429 Too Many Requests`.
- Items older than 5 minutes in the queue are silently dropped (the context is likely stale).

### 3.5 Authorization

Injection follows the same rules as session access: the caller must be the session owner or an admin. The JWT token is required.

Additionally, the audit log records every injection with: `user_id`, `session_id`, `injection_id`, `text_length` (not the text itself — it may contain secrets), `metadata`, and `timestamp`.

### 3.6 Data Flow

```
Caller (bridge / UI / curl)
  │
  ▼
REST API: POST /sessions/{id}/inject
  │
  ├── Validate JWT, session ownership, session status
  ├── Append to in-memory injection queue
  ├── Return 202
  │
  ▼
Injection drainer goroutine (per session)
  │
  ├── Wait delay_ms
  ├── Append \n if raw=false
  │
  ▼
gRPC: ServerCommand.InputData
  │
  ▼
Agent: session.WriteInput(data)
  │
  ▼
PTY stdin → Claude reads it as user input
  │
  ▼
Claude responds → PTY stdout → scrollback + live relay
```

### 3.7 Frontend Integration

- **Session detail view:** A collapsible "Inject" panel below the terminal. Contains a textarea and a "Send" button. The textarea supports Ctrl+Enter to send. Injected text appears in the terminal as normal input (because it is — the PTY echoes it).
- **Injection history:** A small log below the inject panel showing recent injections with timestamp, text preview (first 80 chars), and source (manual / bridge / API). Sourced from the audit log.

### 3.8 Interaction with Templates

When a session is created from a template with an `initial_prompt`, the prompt is delivered as the first injection. This reuses the injection queue rather than being a special code path. The only difference is that the initial prompt injection is created server-side during session creation, while subsequent injections come from the REST endpoint.

This means the `initial_prompt` goes through the same queue, respects the same ordering guarantees, and appears in the same audit log.

---

## 4. Polling Bridge (`claude-plane-bridge`)

### 4.1 Problem

claude-plane is designed to run behind firewalls with no public IP. This makes it impossible to receive traditional webhooks from GitHub, GitLab, Slack, or any external service that pushes events via HTTP POST. Telegram bots support long-polling natively, but the server has no Telegram integration. Users need external event sources to trigger sessions and external channels to receive notifications and send commands — all without inbound exposure.

### 4.2 What the Bridge Is

`claude-plane-bridge` is a standalone Go binary that runs alongside the server (or anywhere on the same network). It connects to external services using outbound-only protocols (HTTP polling, Telegram Bot long-polling) and translates events into claude-plane API calls over localhost.

The bridge is:

- **Separate binary.** Not compiled into the server or agent. Optional to run.
- **Stateless (almost).** The only persistent state is a high-water mark file per connector that tracks the last seen event ID/timestamp to avoid reprocessing after restart.
- **Multi-connector.** A single bridge instance can run Telegram, GitHub, GitLab, and future connectors simultaneously.
- **Authenticated.** The bridge authenticates to claude-plane's REST API using a regular JWT token (or a long-lived API key, see 4.8).

### 4.3 Architecture

```
                          ┌──────────────┐
                          │   Telegram   │
                          │   Bot API    │
                          └──────┬───────┘
                                 │ long-poll (outbound HTTPS)
                                 │
┌─────────────┐    outbound     ┌▼──────────────────┐    localhost    ┌──────────────────┐
│   GitHub    │◄────poll────────┤                    ├───────REST────►│                  │
│   REST API  │    (HTTPS)      │  claude-plane      │                │  claude-plane    │
└─────────────┘                 │  bridge             │                │  server          │
                                │                    │◄──────REST─────┤                  │
┌─────────────┐    outbound     │  (single binary)   │   (event feed) │                  │
│   GitLab    │◄────poll────────┤                    │                └──────────────────┘
│   REST API  │    (HTTPS)      └────────────────────┘
└─────────────┘

All arrows are outbound from the bridge. Nothing dials in.
```

### 4.4 Connector: Telegram

The Telegram connector provides two-way interaction with claude-plane through a Telegram supergroup with topics enabled (forum mode).

#### 4.4.1 Setup

1. Create a bot via [@BotFather](https://t.me/BotFather).
2. Create a supergroup with topics enabled.
3. Create two topics: "Events" (receives notifications from claude-plane) and "Commands" (users send commands to claude-plane).
4. Add the bot to the group as admin.
5. Configure the bridge with bot token, group ID, and topic IDs.

#### 4.4.2 Events Topic (claude-plane → Telegram)

The bridge polls the server's event feed endpoint (see 4.7) and formats events into Telegram messages:

| Event | Message Format |
|-------|----------------|
| Session started | `🟢 Session **{name}** started on **{machine}**` with template name if applicable. |
| Session completed | `✅ Session **{id}** completed (exit 0) — {duration}, {cost}` |
| Session failed | `🔴 Session **{id}** failed (exit {code}) — {duration}, {cost}` |
| Session killed (budget) | `💰 Session **{id}** killed: budget cap ${cap} reached` |
| Session killed (timeout) | `⏱ Session **{id}** killed: timeout {seconds}s reached` |
| Agent connected | `🖥 Agent **{machine}** connected` |
| Agent disconnected | `🖥 Agent **{machine}** disconnected` |
| Job run completed | `📋 Job **{name}** run #{n} completed — {passed}/{total} steps passed` |
| Job run failed | `📋 Job **{name}** run #{n} failed at step **{step}**` |

Messages include inline buttons where actionable: "View Session" (deep link to UI), "Kill" (with confirmation), "Rerun" (for jobs).

#### 4.4.3 Commands Topic (Telegram → claude-plane)

The bridge uses Telegram's `getUpdates` long-polling to receive messages from the Commands topic. Messages are parsed as slash commands:

| Command | Action | Maps to |
|---------|--------|---------|
| `/start <template> [machine]` | Launch a session from a template. Machine defaults to the template's `machine_id` or the first available. | `POST /api/v1/sessions` with `template_name` |
| `/start <template> [machine] \| <variables>` | Launch with variables. Example: `/start review-pr nuc-01 \| PR_URL=https://...` | `POST /api/v1/sessions` with `template_name` + `variables` |
| `/list` | List available templates. | `GET /api/v1/templates` |
| `/status` | Show active sessions with machine, duration, cost. | `GET /api/v1/sessions?status=running` |
| `/status <session-id>` | Show detail for one session. | `GET /api/v1/sessions/{id}` |
| `/kill <session-id>` | Kill a session. Requires confirmation (inline button). | `DELETE /api/v1/sessions/{id}` |
| `/inject <session-id> <text>` | Inject text into a running session. | `POST /api/v1/sessions/{id}/inject` |
| `/machines` | List connected machines with health. | `GET /api/v1/machines` |
| `/cost [today\|week\|month]` | Cost summary for the given period. | `GET /api/v1/cost/summary` (future) |
| `/help` | Show available commands. | Local — no API call. |

**Authorization:** The bridge authenticates to claude-plane with a single service account token. All Telegram commands execute as this service account. There is no per-Telegram-user mapping in V1. This is acceptable for personal/small-team use. Per-user mapping (Telegram user ID → claude-plane user ID) is a V2 extension.

**Confirmation for destructive actions:** `/kill` sends an inline keyboard with "Confirm Kill" and "Cancel" buttons. The bridge processes the callback query before making the API call.

#### 4.4.4 Error Handling

- If the claude-plane API is unreachable, the bridge logs the error and replies to the Telegram user with "⚠️ Control plane unreachable. Retrying..."
- If a command fails (bad template name, session not found), the bridge replies with the API error message formatted for Telegram.
- If the Telegram API returns rate limit errors (429), the bridge backs off using the `retry_after` field from the response.

### 4.5 Connector: GitHub

The GitHub connector polls repository events and creates sessions in response to configurable triggers.

#### 4.5.1 Polling Mechanism

For each configured repository, the bridge polls the GitHub REST API using a Personal Access Token (PAT) or GitHub App installation token.

**Endpoints polled:**

| Trigger | GitHub API Endpoint | Poll Interval |
|---------|-------------------|---------------|
| PR opened/updated | `GET /repos/{owner}/{repo}/pulls?state=open&sort=updated&direction=desc` | Configurable, default 60s |
| CI check completed | `GET /repos/{owner}/{repo}/check-runs?filter=latest` | Configurable, default 60s |
| Issue labeled | `GET /repos/{owner}/{repo}/issues?labels={label}&sort=updated&direction=desc` | Configurable, default 120s |

The bridge tracks the last seen `updated_at` timestamp per repo+trigger combination in a local JSON file (`~/.claude-plane-bridge/state.json`). On each poll, only events newer than the high-water mark are processed.

**Rate budget:** GitHub allows 5,000 requests/hour with a PAT. Polling 5 repos with 2 triggers each at 60-second intervals consumes ~600 requests/hour — well within budget. The bridge tracks remaining rate limit from response headers (`X-RateLimit-Remaining`) and automatically backs off when below 10% capacity.

#### 4.5.2 Event Processing

When a new event is detected, the bridge:

1. Matches it against the configured trigger rules (repo + event type + optional filters).
2. Loads the associated template name from the connector config.
3. Fetches additional context from GitHub as needed (PR diff, CI log, issue body).
4. Builds the `variables` map for template placeholder substitution.
5. Calls `POST /api/v1/sessions` with `template_name` and `variables`.
6. Optionally posts a status comment on the PR/issue: "🤖 claude-plane session `{id}` started from template `{template}` on `{machine}`."

**Available variables per trigger:**

| Trigger | Variables |
|---------|-----------|
| `pull_request.opened` | `PR_URL`, `PR_TITLE`, `PR_BODY`, `PR_AUTHOR`, `PR_BRANCH`, `PR_BASE`, `PR_NUMBER`, `PR_DIFF_URL`, `REPO_FULL_NAME` |
| `pull_request.updated` | Same as above, plus `PR_UPDATED_AT` |
| `check_run.completed` | `CHECK_NAME`, `CHECK_STATUS`, `CHECK_CONCLUSION`, `CHECK_URL`, `CHECK_OUTPUT` (truncated to 4KB), `PR_URL` (if associated), `REPO_FULL_NAME` |
| `issue.labeled` | `ISSUE_URL`, `ISSUE_TITLE`, `ISSUE_BODY`, `ISSUE_AUTHOR`, `ISSUE_LABELS`, `ISSUE_NUMBER`, `REPO_FULL_NAME` |

#### 4.5.3 Deduplication

The bridge must not create duplicate sessions for the same event. Each processed event is recorded in the state file as `{repo}:{trigger}:{event_id}`. Before creating a session, the bridge checks whether this event ID has already been processed.

The state file is pruned on each write: entries older than 7 days are removed.

#### 4.5.4 Filters

Not every PR or CI failure warrants a session. The connector config supports filters:

| Filter | Description |
|--------|-------------|
| `branches` | Only trigger for PRs targeting these base branches. Example: `["main", "develop"]`. |
| `labels` | Only trigger for PRs/issues with these labels. Example: `["claude-review"]`. |
| `check_names` | Only trigger for these check run names. Example: `["CI / test", "CI / lint"]`. |
| `conclusions` | Only trigger for these check conclusions. Example: `["failure", "timed_out"]`. Ignores `success` by default. |
| `paths` | Only trigger if changed files match these glob patterns. Example: `["src/**", "*.go"]`. Requires fetching the PR's file list. |
| `authors_ignore` | Skip events from these authors. Example: `["dependabot[bot]"]`. |

All filters are AND-combined. An event must pass all configured filters to trigger a session.

### 4.6 Connector: GitLab (Future, V2+)

Same pattern as GitHub: poll the GitLab REST API for merge request and pipeline events. Use a Project Access Token. Variable mappings follow the same naming conventions with `MR_` prefix instead of `PR_`.

Not specified in detail here — the architecture is identical to GitHub with different API endpoints and field names.

### 4.7 Server-Side Event Feed

The bridge needs to receive events from the server for the Telegram Events topic. Two options:

**Option A: Polling (simpler, chosen for V1).**
New endpoint: `GET /api/v1/events?after={cursor}`. Returns a paginated list of recent events (session lifecycle, agent status, job runs) with a cursor for efficient polling. The bridge polls this every 2–5 seconds. Events are stored in the existing `audit_log` table with structured `detail` JSON.

**Option B: Server-Sent Events (SSE) or WebSocket (lower latency, V2).**
A persistent connection that pushes events as they happen. Better UX (instant Telegram notifications) but adds a long-lived connection to manage. The existing events WebSocket (`/ws/events`) could be reused, but it currently requires browser-style auth.

For V1, Option A is sufficient. Telegram notification latency of 2–5 seconds is acceptable.

### 4.8 Bridge Authentication

The bridge needs a stable authentication credential that doesn't expire every 60 minutes (the default JWT TTL).

**Proposed: API keys.** A new resource type — long-lived tokens with configurable expiry (or no expiry) and scoped permissions. Stored hashed in the DB. Created via `POST /api/v1/api-keys` or the `seed-admin` CLI.

| Field | Description |
|-------|-------------|
| `key_id` | Public identifier (prefix of the key, e.g., `cpk_abc123...`). |
| `key_hash` | bcrypt hash of the full key. |
| `user_id` | Owner. Determines what the key can access. |
| `name` | Human label. Example: "bridge-homelab". |
| `scopes` | JSON array of permitted scopes. Example: `["sessions:write", "templates:read", "machines:read"]`. Empty array means full access (admin keys). |
| `expires_at` | Optional. `null` means no expiry. |
| `last_used_at` | Updated on each use. Helps identify stale keys. |

The bridge sends the API key as `Authorization: Bearer cpk_...` — the auth middleware detects the `cpk_` prefix and validates against the API keys table instead of parsing as a JWT.

### 4.9 Bridge Configuration

Full TOML config reference:

```toml
# Connection to the claude-plane server (localhost or LAN)
[claude_plane]
api_url = "http://localhost:8080"
api_key = "cpk_..."               # or api_key_file for file-based loading

# High-water mark persistence
[state]
path = "~/.claude-plane-bridge/state.json"

# ── Telegram connector ──
[telegram]
enabled = true
bot_token_file = "/run/secrets/telegram-bot-token"
group_id = -1001234567890
events_topic_id = 2
commands_topic_id = 3
poll_timeout_seconds = 30           # Telegram long-poll timeout

# ── GitHub connector ──
[github]
enabled = true
token_file = "/run/secrets/github-pat"

[[github.watches]]
repo = "kodrunhq/kodrun"
template = "review-pr"
machine_id = "nuc-01"
poll_interval = "60s"

[github.watches.triggers.pull_request_opened]
enabled = true
branches = ["main"]
labels = []                         # empty = no label filter
authors_ignore = ["dependabot[bot]"]

[github.watches.triggers.check_run_completed]
enabled = true
check_names = ["CI / test", "CI / lint"]
conclusions = ["failure", "timed_out"]

[[github.watches]]
repo = "kodrunhq/spark-lens"
template = "review-pr"
machine_id = "nuc-01"
poll_interval = "120s"

[github.watches.triggers.pull_request_opened]
enabled = true
branches = ["main"]

# ── GitLab connector (future) ──
# [gitlab]
# enabled = false
```

### 4.10 Bridge Lifecycle

- **Startup:** Load config, validate all connectors can authenticate (Telegram `getMe`, GitHub `GET /user`, claude-plane `GET /api/v1/machines`). Log failures per connector but don't exit — partial operation is better than total failure.
- **Runtime:** Each connector runs as an independent goroutine. One connector failing doesn't stop the others.
- **Shutdown:** SIGINT/SIGTERM triggers graceful shutdown. The bridge flushes the state file and closes Telegram/HTTP connections. Graceful shutdown timeout: 10 seconds.
- **Health:** The bridge exposes a minimal health endpoint on a configurable local port (default `localhost:9090/healthz`) for monitoring. Reports per-connector status.

---

## 5. Interaction Patterns

### 5.1 PR Review Automation (GitHub + Template + Injection)

1. Developer opens a PR on `kodrunhq/kodrun` targeting `main`.
2. Bridge detects the PR via polling (60s interval).
3. Bridge fetches PR metadata and diff URL from GitHub.
4. Bridge calls `POST /api/v1/sessions` with `template_name=review-pr` and `variables={"PR_URL": "...", "PR_TITLE": "..."}`.
5. Server resolves the template, substitutes variables into `initial_prompt`, resolves `$cred:github-pat` in env vars, sends `CreateSessionCmd` to agent `nuc-01`.
6. Agent spawns Claude in the repo working directory with the initial prompt injected.
7. Claude reviews the PR.
8. Session completes → server emits event → bridge polls event feed → bridge posts result to Telegram Events topic.

### 5.2 CI Failure Fix (GitHub + Injection into Existing Session)

1. Developer already has a session running on `nuc-01` for the same repo (session `abc-123`).
2. CI fails on their latest push.
3. Bridge detects the `check_run.completed` event with conclusion `failure`.
4. Bridge config for this trigger has `inject_into_existing = true` and `session_match = "machine_id:nuc-01,working_dir:/home/jose/kodrun"` (instead of creating a new session, find a running one).
5. Bridge calls `POST /api/v1/sessions/abc-123/inject` with the CI log as text.
6. Claude receives the failure context mid-conversation and adjusts.

### 5.3 Mobile Session Management (Telegram)

1. José is away from his desk, monitoring via phone.
2. Types `/status` in the Commands topic. Bridge replies with active sessions list.
3. Types `/start fix-ci nuc-01 | CI_LOG=https://github.com/...`. Bridge creates a session.
4. Events topic shows "🟢 Session started."
5. 10 minutes later, Events topic shows "✅ Session completed (exit 0) — 9m 42s, $1.23."
6. Types `/kill abc-123` for a stuck session. Bridge replies with confirmation button. Taps confirm. Session killed.

---

## 6. Data Model Changes

### 6.1 New Tables

**`session_templates`** — Template definitions. See section 2.2 for fields.

**`api_keys`** — Long-lived API keys for bridge and automation. See section 4.8 for fields.

**`injections`** — Audit trail for session injections. Lightweight; stored alongside the existing `audit_log` or as a dedicated table for queryability.

| Column | Type | Description |
|--------|------|-------------|
| `injection_id` | TEXT PK | UUID |
| `session_id` | TEXT FK | Target session |
| `user_id` | TEXT FK | Who injected (or API key owner) |
| `text_length` | INTEGER | Length of injected text (not the text itself) |
| `metadata` | TEXT | JSON, caller-provided context |
| `source` | TEXT | `manual`, `api`, `bridge-telegram`, `bridge-github` |
| `created_at` | DATETIME | When the injection was queued |
| `delivered_at` | DATETIME | When the injection was sent to the agent (null if pending/failed) |

### 6.2 Modified Tables

**`sessions`** — Add `template_id TEXT REFERENCES session_templates(template_id)` to track which template spawned the session. Nullable for sessions created without a template.

---

## 7. Implementation Phases

### Phase 1: Templates (Foundation)

- `session_templates` table + migration
- Template CRUD endpoints
- Template-aware session creation (merge logic, variable substitution)
- Credential reference resolution in env vars
- UI: template cards on Command Center, template editor page, "From Template" in session creation modal
- Tests: template CRUD, merge semantics, variable substitution, credential resolution, authorization

**Estimated scope:** ~2,500 lines of Go + ~1,500 lines of TypeScript.

### Phase 2: Session Injection

- Injection REST endpoint
- Per-session injection queue (in-memory)
- Injection drainer goroutine
- Audit logging for injections
- `initial_prompt` delivered via injection queue
- UI: inject panel in session detail view
- Tests: injection delivery, queue ordering, delay, authorization, rate limiting

**Estimated scope:** ~1,200 lines of Go + ~500 lines of TypeScript.

### Phase 3: Polling Bridge — Core + Telegram

- `claude-plane-bridge` binary scaffold (Cobra CLI, TOML config, graceful shutdown)
- Server-side event feed endpoint (`GET /api/v1/events`)
- API keys table + auth middleware support
- Telegram connector: events topic (outbound notifications)
- Telegram connector: commands topic (inbound commands with slash parsing)
- State file for high-water marks
- Tests: Telegram message formatting, command parsing, event feed pagination

**Estimated scope:** ~3,000 lines of Go (new binary).

### Phase 4: Bridge — GitHub

- GitHub polling connector
- Event processing and variable extraction
- Filter evaluation
- Deduplication via state file
- PR/issue comment posting (optional status updates)
- Inject-into-existing session matching
- Tests: GitHub event parsing, filter logic, deduplication, rate limit handling

**Estimated scope:** ~2,000 lines of Go.

---

## 8. Open Questions

| # | Question | Leaning | Decision |
|---|----------|---------|----------|
| 1 | Should templates be global or per-user? | Per-user with admin visibility. A future "shared templates" or "org templates" feature can come later. | TBD |
| 2 | Should the bridge support multiple claude-plane servers? | No. One bridge per server. Run multiple bridge instances if needed. Keeps config simple. | TBD |
| 3 | Should injection support binary data (control sequences like Ctrl+C)? | Yes, via `raw=true`. The `text` field accepts base64-encoded data when `encoding: base64` is set. | TBD |
| 4 | Should the bridge post GitHub PR comments automatically? | Configurable per watch. Default off — some teams find bot comments noisy. | TBD |
| 5 | Should API keys support IP allowlisting? | Not in V1. The bridge runs on localhost or LAN — network-level restriction is sufficient. | TBD |
| 6 | Should the event feed be a separate table or reuse `audit_log`? | Reuse `audit_log` with structured JSON in `detail`. Add an `event_type` column for efficient filtering. Avoids dual-write. | TBD |
| 7 | Should template `name` be globally unique or per-user unique? | Per-user unique. Two users can both have a template called `review-pr`. The bridge uses the service account's namespace. | TBD |
