# claude-plane: Distribution & Agent Join — Implementation Plan

**Date:** 2026-03-15
**Scope:** Pre-built binary distribution via goreleaser + GitHub Releases, universal install script, and `claude-plane-agent join <CODE>` command with short join codes.
**Estimated total effort:** ~8–12 hours

---

## Part 1: Pre-Built Binaries via goreleaser + GitHub Actions

### 1.1 Goal

Eliminate the Go toolchain and Node.js as installation prerequisites. A single `curl | tar` command gets a user from zero to a running binary.

### 1.2 goreleaser Configuration

**New file:** `.goreleaser.yaml` at repo root.

The config must build three binaries (server, agent, bridge), each for four platforms. The frontend must be pre-built and embedded into the server binary before goreleaser runs.

```yaml
version: 2

before:
  hooks:
    # Frontend must be built before the server binary so go:embed picks it up.
    - cmd: bash -c "cd web && npm ci && npm run build"
    # Agent binaries must be cross-compiled before the server binary so the
    # agent download handler (agentdl) can embed them.
    - cmd: bash scripts/build-agent-binaries.sh

builds:
  - id: server
    binary: claude-plane-server
    main: ./cmd/server
    env:
      - CGO_ENABLED=1  # Required by modernc.org/sqlite
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Version={{.Version}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Commit={{.ShortCommit}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Date={{.Date}}

  - id: agent
    binary: claude-plane-agent
    main: ./cmd/agent
    env:
      - CGO_ENABLED=0  # Pure Go, no cgo needed
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Version={{.Version}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Commit={{.ShortCommit}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Date={{.Date}}

  - id: bridge
    binary: claude-plane-bridge
    main: ./cmd/bridge
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Version={{.Version}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Commit={{.ShortCommit}}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Date={{.Date}}

archives:
  - id: server
    builds: [server]
    name_template: "claude-plane-server_{{ .Os }}_{{ .Arch }}"
    format: tar.gz
  - id: agent
    builds: [agent]
    name_template: "claude-plane-agent_{{ .Os }}_{{ .Arch }}"
    format: tar.gz
  - id: bridge
    builds: [bridge]
    name_template: "claude-plane-bridge_{{ .Os }}_{{ .Arch }}"
    format: tar.gz

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
```

**CGO note:** The server binary uses `modernc.org/sqlite` which is a pure-Go SQLite implementation — but it uses `cgo` for some optimized code paths. goreleaser supports CGO cross-compilation via `goreleaser-cross` Docker image or by building on native runners per OS. The simplest approach: use GitHub Actions with `matrix` to build on native macOS and Linux runners, then combine the artifacts. Alternatively, if `modernc.org/sqlite` builds without CGO (it does — it has a pure-Go fallback), set `CGO_ENABLED=0` for the server too and accept the minor performance penalty on SQLite operations.

**Decision:** Set `CGO_ENABLED=0` for all three binaries. `modernc.org/sqlite` works without CGO. This dramatically simplifies cross-compilation — no need for `gcc` cross-compilers, no Docker, no matrix builds. One runner produces all 12 binaries.

### 1.3 GitHub Actions Workflow

**New file:** `.github/workflows/release.yml`

Triggers on tag push (`v*`). Runs on `ubuntu-latest`. Installs Go + Node.js, runs goreleaser, which builds the frontend, cross-compiles all binaries, creates the GitHub Release with archives and checksums.

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - uses: actions/setup-node@v4
        with:
          node-version: '22'

      - name: Run goreleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: '~> v2'
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Also add:** A CI workflow (`.github/workflows/ci.yml`) that runs on every push and PR: `go test -race ./...`, `cd web && npm ci && npm run test -- --run`, `go vet ./...`, `golangci-lint run`. This was flagged as missing in V1 and still doesn't exist.

### 1.4 Tagging Convention

Use semantic versioning: `v0.3.0`, `v0.3.1`, etc. The `buildinfo` package already reads the version from ldflags. goreleaser injects `{{.Version}}` from the tag name.

### 1.5 Testing

- Run goreleaser locally with `goreleaser release --snapshot --clean` (no publish) to verify all 12 binaries build.
- Verify the server binary on Linux embeds the frontend (access `http://localhost:8080/` and see the dashboard).
- Verify the agent binary on macOS starts without errors.

---

## Part 2: Universal Install Script

### 2.1 Goal

One command installs the right binary for the current platform:

```bash
# Install server
curl -fsSL https://raw.githubusercontent.com/kodrunhq/claude-plane/main/install.sh | bash

# Install agent
curl -fsSL https://raw.githubusercontent.com/kodrunhq/claude-plane/main/install.sh | bash -s -- agent

# Install bridge
curl -fsSL https://raw.githubusercontent.com/kodrunhq/claude-plane/main/install.sh | bash -s -- bridge

# Quickstart (install server + run quickstart setup)
curl -fsSL https://raw.githubusercontent.com/kodrunhq/claude-plane/main/install.sh | bash -s -- quickstart
```

### 2.2 Script Logic

**New file:** `install.sh` at repo root.

The script:

1. **Detects OS and architecture.** `uname -s` for OS (Linux/Darwin), `uname -m` for arch (x86_64→amd64, aarch64/arm64→arm64). Fails with a clear error on unsupported platforms (Windows, 32-bit).

2. **Determines the component to install.** First argument: `server` (default), `agent`, `bridge`, or `quickstart`. Quickstart installs the server, then runs the interactive setup.

3. **Finds the latest release.** Queries the GitHub API: `https://api.github.com/repos/kodrunhq/claude-plane/releases/latest`. Extracts the tag name. No `jq` dependency — uses `grep` + `sed` to parse the JSON (common pattern in install scripts to minimize dependencies).

4. **Downloads the binary.** Constructs the URL: `https://github.com/kodrunhq/claude-plane/releases/download/${TAG}/claude-plane-${COMPONENT}_${OS}_${ARCH}.tar.gz`. Downloads to a temp directory, verifies checksum against `checksums.txt`, extracts the binary.

5. **Installs to the right location.** If running as root: `/usr/local/bin/`. If not root: `~/.local/bin/` (creates it if needed, warns if it's not in `$PATH`).

6. **Prints next steps.** For server: "Run `claude-plane-server serve --config server.toml`". For agent: "Run `claude-plane-agent join <CODE>`". For quickstart: runs the setup inline.

### 2.3 Quickstart Mode

When invoked with `quickstart`, the script:

1. Installs the server binary (steps 1–5 above).
2. Creates `$HOME/.claude-plane/` as the data directory.
3. Generates the CA: `claude-plane-server ca init --out-dir $HOME/.claude-plane/ca`.
4. Issues the server cert: `claude-plane-server ca issue-server --ca-dir $HOME/.claude-plane/ca --out-dir $HOME/.claude-plane/server-cert`.
5. Generates a random 32-byte JWT secret.
6. Writes a `server.toml` to `$HOME/.claude-plane/server.toml` with all paths filled in.
7. Prompts for admin email and password (or accepts them as env vars `CLAUDE_PLANE_ADMIN_EMAIL` and `CLAUDE_PLANE_ADMIN_PASSWORD` for non-interactive use).
8. Seeds the admin: `claude-plane-server seed-admin --db $HOME/.claude-plane/claude-plane.db --email <email>` with password piped to stdin.
9. Starts the server: `claude-plane-server serve --config $HOME/.claude-plane/server.toml`.
10. Prints the dashboard URL and the next step: "Open http://localhost:8080 and click 'Add Machine' to connect an agent."

### 2.4 Prerequisites Check

The script only requires `curl`, `tar`, `uname`, and `bash` — all present on every Linux and macOS system. No Go, no Node.js, no npm. The quickstart mode additionally requires `openssl` for JWT secret generation (fallback: `head -c 32 /dev/urandom | base64`).

### 2.5 Version Pinning

Support an optional `CLAUDE_PLANE_VERSION` environment variable:

```bash
CLAUDE_PLANE_VERSION=v0.3.0 curl -fsSL .../install.sh | bash
```

If set, the script downloads that specific version instead of `latest`. Useful for reproducible deployments.

### 2.6 Updating

The same script handles updates. If the binary already exists, the script overwrites it. No special update logic — download, replace, done. A future `claude-plane-server update` command could wrap this, but it's not needed for V1.

---

## Part 3: Agent Join Command with Short Codes

### 3.1 Goal

Replace the current flow:

```
[server admin] → run CLI command → copy long curl one-liner → [agent machine] → paste and run
```

With:

```
[server admin] → click "Add Machine" in dashboard → see code A3X9K2
[agent machine] → claude-plane-agent join A3X9K2
```

### 3.2 Schema Changes

**Migration (add to existing provisioning_tokens table):**

```sql
ALTER TABLE provisioning_tokens ADD COLUMN short_code TEXT UNIQUE;
CREATE INDEX IF NOT EXISTS idx_provisioning_tokens_short_code ON provisioning_tokens(short_code);
```

### 3.3 Short Code Generation

**Format:** 6 alphanumeric characters, uppercase, no ambiguous characters (no O/0, I/1, L). Character set: `ABCDEFGHJKMNPQRSTUVWXYZ23456789` (30 chars). This gives 30^6 = 729 million possible codes — more than enough, and every code is easy to read aloud or type on a phone.

**Generation:** Use `crypto/rand` to pick 6 characters from the set. Check for uniqueness in the DB (collision probability is negligible but check anyway). If collision, regenerate.

**Expiry:** The short code inherits the provisioning token's expiry (default 1 hour). After expiry, the code is invalid. Expired codes are not recycled — new provisions always generate new codes.

### 3.4 Server-Side Changes

#### 3.4.1 Provision Service

`CreateAgentProvision` gains a `short_code` field in its return:

```go
type ProvisionResult struct {
    Token       string    `json:"token"`
    ShortCode   string    `json:"short_code"`
    ExpiresAt   time.Time `json:"expires_at"`
    CurlCommand string    `json:"curl_command"`
    JoinCommand string    `json:"join_command"`
}
```

`JoinCommand` is formatted as: `claude-plane-agent join A3X9K2`

The service generates the short code and stores it alongside the token.

#### 3.4.2 New Endpoint: Redeem by Short Code

**`POST /api/v1/provision/join`** — public, no JWT required.

Request body:

```json
{
    "code": "A3X9K2"
}
```

Response (on success):

```json
{
    "machine_id": "nuc-01",
    "grpc_address": "10.0.1.50:9090",
    "ca_cert_pem": "-----BEGIN CERTIFICATE-----\n...",
    "agent_cert_pem": "-----BEGIN CERTIFICATE-----\n...",
    "agent_key_pem": "-----BEGIN RSA PRIVATE KEY-----\n..."
}
```

The endpoint:

1. Looks up the token by `short_code`.
2. Validates it's not expired and not redeemed.
3. Marks it as redeemed.
4. Returns the certificates and connection info.
5. Does NOT return the binary — the agent is already installed.

Error responses:

| Status | Condition |
|--------|-----------|
| 404 | Code not found |
| 410 | Code expired |
| 410 | Code already redeemed |
| 400 | Missing or malformed code |

**Rate limiting:** 10 attempts per minute per IP. Prevents brute-forcing the 6-character code space. With 30^6 possibilities and 10 guesses per minute, brute force would take ~139 years. But rate limiting is defense in depth.

#### 3.4.3 Store Changes

New methods on `store.Store`:

```go
func (s *Store) GetProvisioningTokenByCode(ctx context.Context, code string) (*ProvisioningToken, error)
```

Same validation logic as `GetProvisioningToken` (expired check, redeemed check) but looks up by `short_code` instead of `token`.

#### 3.4.4 Frontend: Dashboard "Add Machine" Flow

The current provisioning UI shows the full curl command. Update it to prominently display the short code with a "copy" button, and show the join command:

```
┌───────────────────────────────────────┐
│        Join Code: A3X9K2              │
│        [Copy]                         │
│                                       │
│  Run on the target machine:           │
│  claude-plane-agent join A3X9K2       │
│  [Copy Command]                       │
│                                       │
│  Expires in 58 minutes                │
│                                       │
│  ─── Advanced ────────────────────    │
│  Full curl command (for scripted       │
│  provisioning):                       │
│  curl -sfL http://... | sudo bash     │
│  [Copy]                               │
└───────────────────────────────────────┘
```

The curl command remains available for automated/scripted provisioning. The join code is the primary path for interactive use.

### 3.5 Agent-Side Changes

#### 3.5.1 New `join` Command

**File:** `cmd/agent/main.go` — add `newJoinCmd()`.

```
claude-plane-agent join <CODE> [--server URL] [--config-dir DIR]
```

Arguments:

| Arg/Flag | Required | Default | Description |
|----------|----------|---------|-------------|
| `CODE` | Yes (positional) | — | 6-character join code |
| `--server` | Conditional | — | Server HTTP URL. Required if the agent doesn't know where the server is. See discovery below. |
| `--config-dir` | No | `/etc/claude-plane` (root) or `~/.claude-plane` (non-root) | Where to write config and certs |

#### 3.5.2 Join Flow

1. **Determine server URL.** If `--server` is provided, use it. Otherwise, check environment variable `CLAUDE_PLANE_SERVER`. If neither exists, print an error: "Server URL required. Use --server or set CLAUDE_PLANE_SERVER."

2. **Call the redeem endpoint.** `POST <server>/api/v1/provision/join` with `{"code": "A3X9K2"}`.

3. **On success:** Write certificates and config to `--config-dir`:
   - `certs/ca.pem`
   - `certs/agent.pem`
   - `certs/agent-key.pem`
   - `agent.toml` (with `machine_id`, `server.address` from the response, cert paths)

4. **Print next step:**
   ```
   ✓ Agent configured for machine "nuc-01"
   ✓ Certificates written to /etc/claude-plane/certs/
   ✓ Config written to /etc/claude-plane/agent.toml

   Start the agent:
     claude-plane-agent run --config /etc/claude-plane/agent.toml

   Or install as a service:
     sudo claude-plane-agent install
   ```

5. **On failure:** Print the error from the server (expired, redeemed, not found) and suggest checking the code.

#### 3.5.3 Optional: `install` Subcommand

A convenience command that creates the systemd/launchd service file and enables it. This reuses the logic from the existing provisioning script but runs locally:

```
claude-plane-agent install [--config /etc/claude-plane/agent.toml]
```

Detects the OS, writes the appropriate service file, runs `systemctl daemon-reload && systemctl enable --now claude-plane-agent` (Linux) or `launchctl load` (macOS).

This makes the full agent setup:

```bash
# Install the binary
curl -fsSL .../install.sh | bash -s -- agent

# Join the server
claude-plane-agent join A3X9K2 --server http://10.0.1.50:8080

# Install as a service and start
sudo claude-plane-agent install
```

Three commands, no manual config editing, no certificate management.

#### 3.5.4 Server URL Discovery

For the quickstart/homelab case, requiring `--server` is friction. Two progressive enhancements (implement if time permits, not blockers):

**Environment variable:** `CLAUDE_PLANE_SERVER=http://10.0.1.50:8080` in `.bashrc` or set by the install script. The join command checks this automatically.

**Embed server URL in the install script:** When the install script is served from the claude-plane server itself (via `curl -fsSL http://10.0.1.50:8080/install.sh | bash -s -- agent`), the server can dynamically inject its own URL into the script. The agent install then writes `CLAUDE_PLANE_SERVER` to a well-known location. This way, `join A3X9K2` works without `--server` because the agent already knows where the server is from the install step.

### 3.6 Security Considerations

**Short code brute force:** 30^6 ≈ 729M possibilities. At 10 guesses/minute (rate limit), exhaustive search takes 139 years. At 100 guesses/minute (without rate limit), 14 years. Acceptable.

**Code exposure:** The code is valid for 1 hour by default. If someone sees it, they have 1 hour to redeem it. Once redeemed, it's dead. The machine ID is fixed at creation time, so a stolen code provisions a machine with a known identity (visible in the dashboard — the admin would see an unexpected machine).

**Man-in-the-middle on the join call:** The `POST /api/v1/provision/join` call is over plain HTTP in the quickstart case (no TLS on the HTTP listener by default). The response contains the CA cert and agent cert/key. If an attacker intercepts this call, they get valid credentials. Mitigation: document that production deployments should run the HTTP listener behind TLS (nginx/caddy) or on a trusted network. The gRPC connection that follows IS mTLS-secured regardless.

**Cert material in memory:** The join response contains PEM-encoded certificates in the HTTP response body. The agent writes them to disk immediately and the HTTP response is garbage collected. No different from the current curl-script flow, which also transmits certs over HTTP.

---

## Part 4: Update Existing Quickstart Script

### 4.1 Changes

The existing `quickstart.sh` currently builds from source (requires Go + Node.js). Update it to:

1. Check if pre-built binaries are available (`claude-plane-server` in PATH or current directory).
2. If not, download them via the install script logic.
3. Remove the `go build` and `npm install/build` steps.
4. Keep the CA init, cert generation, config generation, admin seeding, and server start logic.

Alternatively, **deprecate `quickstart.sh` entirely** in favor of `install.sh quickstart`. The install script handles binary download, and the quickstart mode handles the interactive setup. One script instead of two.

**Decision:** Deprecate `quickstart.sh`. Add a note at the top redirecting to `install.sh`. Keep the file for one release cycle, then remove.

---

## Part 5: Docker Images

### 5.1 Dockerfiles

**`Dockerfile.server`:**

```dockerfile
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /claude-plane-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /claude-plane-server /usr/local/bin/
ENTRYPOINT ["claude-plane-server"]
CMD ["serve", "--config", "/etc/claude-plane/server.toml"]
```

**`Dockerfile.agent`:**

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /claude-plane-agent ./cmd/agent

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /claude-plane-agent /usr/local/bin/
ENTRYPOINT ["claude-plane-agent"]
CMD ["run", "--config", "/etc/claude-plane/agent.toml"]
```

### 5.2 GitHub Actions: Build and Push

Add to the release workflow:

```yaml
  docker:
    runs-on: ubuntu-latest
    needs: release
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: Dockerfile.server
          push: true
          tags: ghcr.io/kodrunhq/claude-plane-server:${{ github.ref_name }},ghcr.io/kodrunhq/claude-plane-server:latest
      - uses: docker/build-push-action@v5
        with:
          context: .
          file: Dockerfile.agent
          push: true
          tags: ghcr.io/kodrunhq/claude-plane-agent:${{ github.ref_name }},ghcr.io/kodrunhq/claude-plane-agent:latest
```

### 5.3 Scope

Docker is a secondary distribution path. If it takes more than 2 hours to get working, ship the release without it and add Docker in a follow-up. Pre-built binaries and the install script are the priority.

---

## Implementation Order

| Step | Component | Effort | Notes |
|------|-----------|--------|-------|
| 1 | `.goreleaser.yaml` | 1 hr | Config + local snapshot test |
| 2 | `.github/workflows/release.yml` | 30 min | Tag-triggered release workflow |
| 3 | `.github/workflows/ci.yml` | 30 min | Test + lint on push/PR |
| 4 | `install.sh` | 2 hr | Platform detection, download, checksum verify, quickstart mode |
| 5 | Short code generation + schema migration | 1 hr | `short_code` column, crypto/rand generator, store methods |
| 6 | `POST /api/v1/provision/join` endpoint | 1 hr | Handler, rate limiting, store lookup by code |
| 7 | Provision service: generate short codes | 30 min | Wire into `CreateAgentProvision`, return in result |
| 8 | Frontend: join code display in provisioning UI | 1 hr | Prominent code display, copy button, expiry countdown |
| 9 | Agent `join` command | 1.5 hr | Cobra command, HTTP call, cert/config writing, next-steps output |
| 10 | Agent `install` command | 1 hr | systemd/launchd service file generation |
| 11 | Deprecate `quickstart.sh` | 15 min | Add redirect notice |
| 12 | Dockerfiles + workflow | 1.5 hr | If time permits |
| 13 | README update | 30 min | New install instructions, remove "Build from Source" as primary path |
| **Total** | | **~12 hr** | Steps 1–4 can ship as one release, steps 5–11 as the next |

### Suggested Release Plan

**v0.3.0** — Pre-built binaries + install script (steps 1–4, 11, 13). This unblocks everything else and gives immediate value.

**v0.3.1** — Agent join command + short codes (steps 5–10). This builds on v0.3.0's install script.

**v0.3.2** — Docker images (step 12). Secondary path, lowest priority.
