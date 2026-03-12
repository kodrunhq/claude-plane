# CI Pipeline & Project Documentation Design

## Goal

Add GitHub Actions CI pipeline and comprehensive project documentation so that contributors can validate their changes automatically and new users can install and operate claude-plane from scratch.

## Scope

Two independent deliverables:

1. **CI Pipeline** — GitHub Actions workflow for automated testing, linting, and building on every PR and push to main.
2. **Project Documentation** — README rewrite + detailed docs for server install, agent install, quickstart, configuration, and architecture.

---

## 1. CI Pipeline

### Workflow: `.github/workflows/ci.yml`

**Triggers:** `push` to `main`, `pull_request` targeting `main`.

**Two parallel jobs:**

#### Job 1: `backend` (Go)

- **Runner:** `ubuntu-latest`
- **Go version:** `1.25.x` (matches `go.mod`)
- **Steps:**
  1. Checkout code
  2. Set up Go with module caching
  3. `go vet ./...` — static analysis
  4. `go test -race ./...` — tests with race detector
  5. `go build -o /dev/null ./cmd/server` — verify server compiles
  6. `go build -o /dev/null ./cmd/agent` — verify agent compiles

#### Job 2: `frontend` (Node)

- **Runner:** `ubuntu-latest`
- **Node version:** `22.x` (LTS, matches dev environment)
- **Working directory:** `web/`
- **Steps:**
  1. Checkout code
  2. Set up Node with npm cache
  3. `npm ci` — clean install
  4. `npx tsc --noEmit` — typecheck
  5. `npm run lint` — ESLint
  6. `npm run test -- --run` — Vitest single-run (no watch mode)
  7. `npm run build` — verify production build

### Design Decisions

- **No proto job:** Generated proto code is committed. Go build catches proto issues.
- **No Docker/compose:** SQLite is in-process; no external services needed.
- **Race detector on:** Catches concurrency bugs early. Worth the ~2x slowdown.
- **Build to `/dev/null`:** We only care that it compiles, not the artifact.
- **`npm ci` over `npm install`:** Reproducible installs from lockfile.

---

## 2. Project Documentation

### File Structure

```
README.md                   — Project overview, features, quickstart link, build-from-source
docs/
  quickstart.md             — Single-machine all-in-one setup for evaluation
  install-server.md         — Full server deployment guide
  install-agent.md          — Agent deployment on worker machines
  configuration.md          — Config file reference (server.toml, agent.toml)
  architecture.md           — System architecture, data flow, security model
```

### README.md

Sections:

1. **Header** — Project name, one-line description
2. **What is claude-plane?** — 1 paragraph explaining the value proposition
3. **Features** — Bullet list of key capabilities
4. **Architecture Overview** — ASCII diagram showing server/agent/browser topology
5. **Quickstart** — Link to `docs/quickstart.md` with a 3-line teaser
6. **Documentation** — Links to all docs
7. **Build from Source** — Go + Node build commands
8. **Project Structure** — Directory layout explanation
9. **Contributing** — Brief guidance
10. **License** — MIT (or as defined)

### docs/quickstart.md

Single-machine setup for evaluation. Covers:

- Prerequisites (Go 1.25+, Node 22+, Claude CLI)
- Build both binaries
- Initialize CA and issue certs
- Create config files
- Seed admin account
- Start server and agent
- Open browser and verify

### docs/install-server.md

Production server deployment:

- Prerequisites
- Download/build binary
- TLS setup (CA init, server cert)
- Server configuration (`server.toml` reference)
- Database setup (SQLite, path selection)
- Seed admin account
- Running as systemd service
- Reverse proxy (nginx/caddy) considerations
- Firewall rules (ports needed)

### docs/install-agent.md

Worker machine agent deployment:

- Prerequisites (Claude CLI must be installed)
- Download/build binary
- Certificate provisioning (get agent cert from server CA)
- Agent configuration (`agent.toml` reference)
- Running as systemd service
- Verifying connectivity to server
- Multi-agent setup notes

### docs/configuration.md

Reference for all config options:

- `server.toml` — all fields with types, defaults, descriptions
- `agent.toml` — all fields with types, defaults, descriptions
- Environment variable overrides
- Example configs (minimal and full)

### docs/architecture.md

System overview for contributors and operators:

- Component diagram (server, agent, browser, Claude CLI)
- Communication protocols (gRPC, REST, WebSocket)
- Data flow for terminal sessions
- Data flow for job execution
- Security model (mTLS, JWT, auth flow)
- Data model summary (SQLite tables)
- Directory structure explanation

### Design Decisions

- **Quickstart is separate from install guides:** Different audiences. Quickstart is "try it in 5 minutes." Install guides are "run this in production."
- **No auto-generated API docs yet:** REST API is internal to the frontend. Can add OpenAPI later.
- **ASCII diagrams over images:** Editable, diffable, no binary assets to manage.
- **Systemd examples included:** Most Linux servers use systemd. Covers the 90% case.
