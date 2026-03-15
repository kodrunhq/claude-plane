# Distribution & Agent Join — Design Spec

**Date:** 2026-03-15
**Scope:** Pre-built binary distribution via goreleaser improvements, universal install script, `claude-plane-agent join <CODE>` with short join codes, Docker images, and quickstart deprecation.

---

## Context

claude-plane currently requires Go 1.25+ and Node.js 22+ to install from source. The provisioning flow requires copying a long curl one-liner from the dashboard to the agent machine. This proposal eliminates both friction points: pre-built binaries remove the toolchain requirement, and short join codes simplify agent provisioning to `claude-plane-agent join A3X9K2`.

### What Already Exists

- `.goreleaser.yml` — builds server + agent for linux/darwin x amd64/arm64, checksums, changelog
- `.github/workflows/release.yml` — tag-triggered release via goreleaser
- `.github/workflows/ci.yml` — go vet, go test, tsc, eslint, vitest, vite build
- `scripts/build-agent-binaries.sh` — cross-compiles agent for embedding in server
- `internal/shared/buildinfo/` — Version/Commit/Date injection via ldflags
- `internal/server/agentdl/` — agent binary download endpoints (go:embed)
- `internal/server/provision/` — provisioning tokens, cert generation, install script templates
- `internal/server/handler/provision.go` — REST endpoints for provisioning (create, list, revoke, serve script)
- `internal/server/api/ratelimit.go` — per-IP rate limiter middleware
- `quickstart.sh` — builds from source, generates certs, starts server+agent locally
- Frontend provisioning UI — TokenGenerator (curl command display), TokensList

### What Does Not Exist

- Bridge binary in goreleaser
- Universal install script (`install.sh`) for pre-built binaries
- Short join codes (`short_code` column, generation, redemption)
- `POST /api/v1/provision/join` endpoint
- Agent `join` CLI command
- Dockerfiles
- Docker build/push in release workflow

---

## Part 1: goreleaser + CI Fixes

### Changes to `.goreleaser.yml`

1. **Add bridge build target** — `cmd/bridge`, `CGO_ENABLED=0`, linux/darwin x amd64/arm64, same ldflags pattern as server/agent.
2. **Add bridge archive** — same naming template: `claude-plane-bridge_{{ .Version }}_{{ .Os }}_{{ .Arch }}`.
3. **Explicitly set `CGO_ENABLED=0` on all three builds** — `modernc.org/sqlite` works pure-Go. Eliminates cross-compilation complexity (no gcc cross-compilers, no Docker, no matrix builds).

### No CI Changes Needed

Existing `ci.yml` and `release.yml` are correct. goreleaser picks up new build targets automatically.

### Bridge Not Embedded in Server

The bridge binary is NOT added to `build-agent-binaries.sh` or `agentdl/`. Bridge is a separate component that connects outward to external services — it does not need to be distributed from the server binary. Only agents are served via the server's download endpoint.

---

## Part 2: Universal Install Script

### File: `install.sh` (repo root)

Single shell script that downloads pre-built binaries from GitHub Releases. Dependencies: `curl`, `tar` (Linux) or `unzip` (macOS), `bash`, `uname` — present on every Linux and macOS system.

### Usage

```bash
curl -fsSL .../install.sh | bash              # install server (default)
curl -fsSL .../install.sh | bash -s -- agent  # install agent
curl -fsSL .../install.sh | bash -s -- bridge # install bridge
curl -fsSL .../install.sh | bash -s -- quickstart  # install server + interactive setup
```

### Script Logic

1. **Detect platform** — `uname -s` for OS (Linux/Darwin), `uname -m` for arch (x86_64→amd64, aarch64/arm64→arm64). Fail with clear error on unsupported platforms (Windows, 32-bit).
2. **Determine component** — first argument: `server` (default), `agent`, `bridge`, or `quickstart`.
3. **Resolve version** — if `CLAUDE_PLANE_VERSION` env var is set, use that tag. Otherwise, query `https://api.github.com/repos/kodrunhq/claude-plane/releases/latest`. Parse tag name with `grep`/`sed` (no `jq` dependency).
4. **Download archive** — detect archive format based on OS: `.zip` for Darwin (matches goreleaser `format_overrides`), `.tar.gz` for Linux. Construct URL: `https://github.com/kodrunhq/claude-plane/releases/download/${TAG}/claude-plane-${COMPONENT}_${VERSION}_${OS}_${ARCH}.{tar.gz|zip}`. Download to temp directory.
5. **Verify checksum** — download `checksums.txt` from the same release, validate SHA256 of the archive. Fail with clear error on mismatch.
6. **Extract and install** — use `tar xzf` for `.tar.gz` (Linux), `unzip` for `.zip` (macOS). Install location: if root → `/usr/local/bin/`, if not root → `~/.local/bin/` (create if needed, warn if not in `$PATH`).
7. **Print next steps** — component-specific:
   - Server: "Run `claude-plane-server serve --config server.toml`"
   - Agent: "Run `claude-plane-agent join <CODE> --server <URL>`"
   - Bridge: "Run `claude-plane-bridge --config bridge.toml`"

### Quickstart Mode

When invoked with `quickstart`, the script additionally:

1. Creates `$HOME/.claude-plane/` as data directory.
2. Generates CA: `claude-plane-server ca init --out-dir $HOME/.claude-plane/ca`.
3. Issues server cert: `claude-plane-server ca issue-server --ca-dir $HOME/.claude-plane/ca --out-dir $HOME/.claude-plane/server-cert`.
4. Generates JWT secret: `head -c 32 /dev/urandom | base64` (no `openssl` dependency).
5. Writes `server.toml` to `$HOME/.claude-plane/server.toml`.
6. Prompts for admin email/password, or reads from `CLAUDE_PLANE_ADMIN_EMAIL` / `CLAUDE_PLANE_ADMIN_PASSWORD` env vars for non-interactive use.
7. Seeds admin: `claude-plane-server seed-admin --db $HOME/.claude-plane/claude-plane.db --email <email> --name Admin` with password read from stdin.
8. Starts server: `claude-plane-server serve --config $HOME/.claude-plane/server.toml`.
9. Prints dashboard URL and next steps.

### Version Pinning

```bash
CLAUDE_PLANE_VERSION=v0.3.0 curl -fsSL .../install.sh | bash
```

### Idempotent Updates

Same script handles updates. If binary exists, it's overwritten. No special update logic.

---

## Part 3: Agent Join with Short Codes

### 3.1 Database Migration (Migration 13)

```sql
ALTER TABLE provisioning_tokens ADD COLUMN short_code TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioning_tokens_short_code ON provisioning_tokens(short_code);
```

Note: SQLite's ALTER TABLE ADD COLUMN does not support inline UNIQUE constraints. Uniqueness is enforced by the UNIQUE INDEX instead.

### 3.2 Short Code Generation

- **Character set:** `ABCDEFGHJKMNPQRSTUVWXYZ23456789` (30 chars, no ambiguous O/0/I/1/L)
- **Length:** 6 characters
- **Entropy:** 30^6 = 729,000,000 possible codes
- **Source:** `crypto/rand`
- **Collision handling:** Check DB for uniqueness on insert, regenerate on collision (negligible probability)
- **Expiry:** Inherits provisioning token TTL (default 1 hour)
- **Not recycled:** Expired codes are never reused

### 3.3 Store Changes

**File:** `internal/server/store/provisioning.go`

**Updated struct** — `ProvisioningToken` gains a `ShortCode` field:
```go
type ProvisioningToken struct {
    // ... existing fields ...
    ShortCode     string     `json:"short_code"`
    // ... existing fields ...
}
```

**Updated `ProvisioningTokenSummary`** — gains `ShortCode` field for the list endpoint:
```go
type ProvisioningTokenSummary struct {
    // ... existing fields ...
    ShortCode     string     `json:"short_code"`
    // ... existing fields ...
}
```

**Updated `CreateProvisioningToken`** — INSERT adds `short_code` column and value.

**Updated `ListProvisioningTokens`** — SELECT adds `short_code`, Scan adds `&t.ShortCode`.

**New method:**
```go
func (s *Store) GetProvisioningTokenByCode(ctx context.Context, code string) (*ProvisioningToken, error)
```

Same validation logic as `GetProvisioningToken` (expiry check, redeemed check), but queries `WHERE short_code = ?` instead of `WHERE token = ?`.

### 3.4 Provision Service Changes

**File:** `internal/server/provision/service.go`

`CreateAgentProvision` updated to:
1. Generate short code using `crypto/rand` + character set
2. Store it alongside the UUID token
3. Return it in `ProvisionResult`

Updated result struct:
```go
type ProvisionResult struct {
    Token       string    `json:"token"`
    ShortCode   string    `json:"short_code"`
    ExpiresAt   time.Time `json:"expires_at"`
    CurlCommand string    `json:"curl_command"`
    JoinCommand string    `json:"join_command"` // "claude-plane-agent join A3X9K2"
}
```

Short code generation function lives in this package (single consumer, no need for a separate utility package).

### 3.5 New Endpoint: Redeem by Short Code

**File:** `internal/server/handler/provision.go`

**`POST /api/v1/provision/join`** — public, no JWT required. Registered in `RegisterProvisionPublicRoutes`.

Request:
```json
{"code": "A3X9K2"}
```

**Input validation:** Code must be exactly 6 characters from the set `ABCDEFGHJKMNPQRSTUVWXYZ23456789`. Reject all others with 400.

Response (success):
```json
{
    "machine_id": "nuc-01",
    "grpc_address": "10.0.1.50:9090",
    "ca_cert_pem": "-----BEGIN CERTIFICATE-----\n...",
    "agent_cert_pem": "-----BEGIN CERTIFICATE-----\n...",
    "agent_key_pem": "-----BEGIN RSA PRIVATE KEY-----\n..."
}
```

**Handler flow:**
1. Validate input (6 chars, valid alphabet).
2. Call `store.GetProvisioningTokenByCode(code)` → returns `*ProvisioningToken` with full UUID.
3. Call `store.RedeemProvisioningToken(token.Token)` using the UUID from step 2 — atomic single-use marking.
4. Return cert material and connection info from the token.

Error responses:

| Status | Condition |
|--------|-----------|
| 400 | Missing or malformed code (wrong length, invalid chars) |
| 404 | Code not found, expired, or already redeemed |
| 429 | Rate limited |

**Security note:** All invalid-code scenarios return 404 (not 410 for expired/redeemed) to avoid leaking whether a code was ever valid. This prevents oracle attacks.

**Rate limiting:** 10 requests/minute per IP on this endpoint only. Uses existing `RateLimitMiddleware` from `internal/server/api/ratelimit.go`.

### 3.6 Agent `join` CLI Command

**File:** `cmd/agent/main.go`

```
claude-plane-agent join <CODE> [--server URL] [--config-dir DIR]
```

| Arg/Flag | Required | Default | Description |
|----------|----------|---------|-------------|
| `CODE` | Yes (positional) | — | 6-character join code |
| `--server` | Conditional | — | Server HTTP URL. Falls back to `CLAUDE_PLANE_SERVER` env var. Error if neither. Must be HTTPS unless `--insecure` is set. |
| `--config-dir` | No | `/etc/claude-plane` (root) or `~/.claude-plane` (non-root) | Where to write config and certs |
| `--insecure` | No | `false` | Allow plain HTTP server URL. Prints a warning when used. |

**Flow:**
1. Resolve server URL: `--server` flag → `CLAUDE_PLANE_SERVER` env var → error with message "Server URL required. Use --server or set CLAUDE_PLANE_SERVER."
2. Validate URL scheme: if HTTP (not HTTPS), require `--insecure` flag. If `--insecure` is used, print warning: "WARNING: Using plain HTTP. Certificate material will be transmitted unencrypted. Use HTTPS in production."
3. POST to `{server}/api/v1/provision/join` with `{"code": "CODE"}`.
4. On success, write to `config-dir`:
   - `certs/ca.pem`
   - `certs/agent.pem`
   - `certs/agent-key.pem`
   - `agent.toml` with machine_id, server gRPC address, cert paths
5. Print success:
   ```
   Agent configured for machine "nuc-01"
   Certificates written to /etc/claude-plane/certs/
   Config written to /etc/claude-plane/agent.toml

   Start the agent:
     claude-plane-agent run --config /etc/claude-plane/agent.toml
   ```
6. On failure, print server error message and suggest checking the code.

### 3.7 Frontend Changes

**`TokenGenerator.tsx`** — redesigned layout:
- Short code displayed prominently (large font, monospace, copy button)
- Join command shown below: `claude-plane-agent join A3X9K2` with copy button
- Expiry countdown timer
- Curl command moved to collapsible "Advanced" section (still functional for scripted provisioning)

**`TokensList.tsx`:**
- Add `short_code` column displaying full 6-character code

**`web/src/types/provisioning.ts`:**
- `ProvisioningToken` interface gains `short_code: string` field
- `ProvisionResult` gains `short_code: string` and `join_command: string`

**No new API client function needed** — the `POST /api/v1/provision/join` endpoint is consumed by the agent CLI, not the browser.

### 3.8 Security Analysis

- **Brute force:** 30^6 = 729M codes. At 10 guesses/min (rate limit), exhaustive search: ~139 years. Without rate limit (100 guesses/min): ~14 years.
- **Single-use:** Redeemed atomically, then permanently invalid.
- **Expiry:** 1 hour default, configurable via TTL.
- **Visibility:** Admin sees all machines in dashboard — a stolen code provisions a machine with a fixed, visible identity.
- **MitM on join call:** Response contains cert/key over HTTP. The `join` command defaults to requiring HTTPS; `--insecure` flag is needed for plain HTTP (prints a warning). Production deployments must use TLS. The gRPC connection that follows IS mTLS-secured regardless.

---

## Part 4: Docker Images

### Dockerfiles

**`Dockerfile.server`** — three-stage build:
1. `node:22-alpine` — `npm ci` + `npm run build` → frontend dist
2. `golang:1.25-alpine` — copies frontend dist, `CGO_ENABLED=0 go build -o /claude-plane-server ./cmd/server`
3. `alpine:3.20` — `ca-certificates` + binary only. ~15-20MB final image.

Entrypoint: `claude-plane-server`, CMD: `serve --config /etc/claude-plane/server.toml`

**`Dockerfile.agent`** — two-stage:
1. `golang:1.25-alpine` — `CGO_ENABLED=0 go build -o /claude-plane-agent ./cmd/agent`
2. `alpine:3.20` — `ca-certificates` + binary.

Entrypoint: `claude-plane-agent`, CMD: `run --config /etc/claude-plane/agent.toml`

**`Dockerfile.bridge`** — two-stage (same pattern as agent):
1. `golang:1.25-alpine` — builds `./cmd/bridge`
2. `alpine:3.20` — `ca-certificates` + binary.

Entrypoint: `claude-plane-bridge`, CMD: `--config /etc/claude-plane/bridge.toml`

### Release Workflow Addition

Add `docker` job to `.github/workflows/release.yml`, runs after goreleaser:
- Login to `ghcr.io` with `GITHUB_TOKEN`
- Build and push all three images
- Tags: `ghcr.io/kodrunhq/claude-plane-{server|agent|bridge}:{tag}` and `:latest`

### Multi-Platform Images

Use `docker buildx build --platform linux/amd64,linux/arm64` for multi-architecture images. This matches the binary distribution matrix for Linux. Darwin Docker images do not exist (Docker containers are Linux), so the Docker matrix is inherently narrower than the binary matrix.

### Design Decisions

- **No docker-compose.yml** — users running Docker have their own orchestration preferences
- **Alpine, not distroless** — `ca-certificates` needed for TLS, `sh` available for debugging
- **No bridge embedding in server image** — separate component, separate image
- **Multi-arch** — `linux/amd64` + `linux/arm64` via buildx, matching goreleaser targets

---

## Part 5: Quickstart Deprecation

- Add deprecation notice at top of `quickstart.sh` that prints a warning and suggests `install.sh quickstart`
- Keep the script functional for one release cycle
- Optionally delegate to `install.sh quickstart` after printing the notice

---

## Testing Requirements

All new code requires unit tests (80%+ coverage). Key test areas:

| Component | Tests |
|-----------|-------|
| Short code generation | Correct length, valid charset, uniqueness across 1000 generations, crypto/rand usage |
| `GetProvisioningTokenByCode` | Found, not found, expired, redeemed, SQL error |
| `CreateProvisioningToken` with short_code | Insert succeeds, short_code stored and retrievable |
| `ListProvisioningTokens` with short_code | Short code returned in summary |
| Join handler (`POST /api/v1/provision/join`) | Valid code, invalid format (wrong length, bad chars), expired, redeemed, rate limited, missing body |
| Agent `join` CLI command | Server URL resolution (flag → env → error), HTTPS enforcement, --insecure bypass, successful join (mock HTTP), failure cases |
| `install.sh` | Tested via goreleaser `--snapshot` local build + manual verification on Linux and macOS |

---

## Release Plan

| Release | Contents | Steps |
|---------|----------|-------|
| **v0.3.0** | goreleaser bridge build + install script + quickstart deprecation | 1, 2, 16 |
| **v0.3.1** | Short codes + join command + frontend updates | 3–11 |
| **v0.3.2** | Docker images + workflow | 12–15 |

## Implementation Order

| # | Component | Type |
|---|---|---|
| 1 | `.goreleaser.yml` — add bridge build + explicit CGO_ENABLED=0 | Change |
| 2 | `install.sh` — universal install script | New |
| 3 | Migration 13 — short_code column + index | New |
| 4 | Short code generator — crypto/rand, 30-char alphabet | New |
| 5 | Store: `GetProvisioningTokenByCode` | New |
| 6 | Provision service — generate short code, update ProvisionResult | Change |
| 7 | `POST /api/v1/provision/join` — public endpoint, rate limited | New |
| 8 | Agent `join` CLI command | New |
| 9 | Frontend: TokenGenerator — prominent code display, collapsible curl | Change |
| 10 | Frontend: TokensList — add short_code column | Change |
| 11 | Frontend: types + API interfaces | Change |
| 12 | `Dockerfile.server` | New |
| 13 | `Dockerfile.agent` | New |
| 14 | `Dockerfile.bridge` | New |
| 15 | `release.yml` — Docker build+push job | Change |
| 16 | `quickstart.sh` — deprecation notice | Change |
