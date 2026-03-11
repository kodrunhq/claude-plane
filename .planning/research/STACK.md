# Technology Stack

**Project:** claude-plane
**Researched:** 2026-03-11
**Overall confidence:** HIGH

## Recommended Stack

### Backend — Go (Server + Agent Binaries)

#### Core Framework & HTTP

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go | 1.23+ | Language runtime | Single binary deployment, excellent concurrency model (goroutines for PTY management, gRPC streaming, WebSocket relay), first-class gRPC ecosystem. Go 1.23 adds `http.Request.Pattern` and improved `net/http` routing. | HIGH |
| go-chi/chi | v5.2.x | HTTP router | Lightweight, idiomatic, composable middleware. Built on `net/http` so it works alongside gRPC on the same server. Chi v5 is actively maintained (Feb 2026 release) and supports Go 1.22+ routing patterns. Standard choice for Go REST APIs. | HIGH |
| go-chi/cors | latest | CORS middleware | Purpose-built for chi. Handles preflight requests correctly at the router level. | HIGH |

#### gRPC — Agent-Server Communication

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| google.golang.org/grpc | v1.79.x | gRPC framework | The canonical Go gRPC implementation. Battle-tested, actively maintained (March 2026 release). Native bidirectional streaming support is critical for the persistent agent connection. Full mTLS support via `credentials.NewTLS()`. | HIGH |
| google.golang.org/protobuf | v1.36.x | Protobuf runtime | Required companion to grpc-go for message serialization. | HIGH |
| buf | latest CLI | Protobuf tooling | Replaces raw `protoc` for code generation. 2x faster compilation, managed mode for Go package paths, reproducible builds via `buf.gen.yaml`. Industry standard for protobuf workflows in 2025+. | HIGH |
| protoc-gen-go + protoc-gen-go-grpc | latest | Code generation | Standard protobuf/gRPC Go code generators. Used via `buf generate`. | HIGH |

**Why grpc-go over ConnectRPC:** ConnectRPC is excellent for browser-facing APIs (simpler HTTP semantics, no proxy needed), but claude-plane's agent-server link requires persistent **bidirectional streaming over mTLS** -- grpc-go's core strength. ConnectRPC supports bidi streaming but only over HTTP/2, and its primary value (browser compat, curl-friendliness) is irrelevant for agent-to-server communication. The frontend uses WebSocket, not gRPC. Use the battle-tested tool for the job.

#### WebSocket — Browser Terminal Streaming

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| coder/websocket | v2.x | WebSocket server | Successor to nhooyr/websocket, now maintained by Coder. Idiomatic Go, builds on `net/http` (works with chi), supports binary messages (essential for terminal I/O), context-aware, proper close handshake. Gorilla WebSocket is still popular but uses its own HTTP/2 implementation that doesn't integrate with `net/http`. | HIGH |

#### PTY Management (Agent)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| creack/pty | v1.1.x | Unix PTY interface | The standard Go library for PTY allocation. Used by every Go terminal project (gotty, ttyd-go, etc.). `pty.Start()` spawns a process with a pseudo-terminal. `pty.InheritSize()` handles SIGWINCH for resize propagation. Simple, stable, battle-tested. | HIGH |

#### Database — Embedded SQLite

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| modernc.org/sqlite | latest | SQLite driver (pure Go) | **Pure Go, no CGO required.** This is critical for single-binary cross-compilation. mattn/go-sqlite3 is faster but requires a C compiler and CGO_ENABLED=1, which breaks the "scp the binary, run it" deployment model. modernc.org/sqlite transpiles the SQLite C code to Go -- slower on benchmarks but fast enough for a control plane workload (metadata, not analytics). Used in production by River (job queue), Gogs, and others. | HIGH |
| database/sql | stdlib | SQL interface | Standard Go database interface. modernc.org/sqlite registers as a `database/sql` driver, so all queries use standard Go patterns. | HIGH |

**SQLite Configuration (critical):**
```go
// Connection string pragmas for production
dsn := "file:claude-plane.db?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON"
```
- **WAL mode**: Concurrent reads don't block writes. Essential for a server handling multiple API requests.
- **busy_timeout=5000**: Retry on lock instead of failing immediately.
- **synchronous=NORMAL**: Crash-safe with WAL while being faster than FULL.
- **Single write connection**: Use `db.SetMaxOpenConns(1)` for the write pool, separate read pool with higher concurrency.

#### Authentication & Security

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| crypto/tls (stdlib) | Go stdlib | mTLS for gRPC | Go's standard TLS implementation. Configure `tls.Config` with `ClientAuth: tls.RequireAndVerifyClientCert` for the gRPC server, load CA cert pool for mutual verification. No third-party library needed. | HIGH |
| crypto/x509 (stdlib) | Go stdlib | Certificate generation (built-in CA) | Generate self-signed CA, server certs, and agent certs programmatically. The `ca init`, `ca issue-server`, `ca issue-agent` CLI commands use this directly. No external CA tooling needed. | HIGH |
| golang-jwt/jwt | v5.x | JWT tokens for frontend auth | Production-ready JWT library. v5 has a cleaner Claims interface and supports HMAC-SHA, RSA, ECDSA, EdDSA. Used for session cookies / bearer tokens on the HTTP API. | HIGH |
| golang.org/x/crypto/argon2 | latest | Password hashing | Argon2id is the 2025 gold standard for password hashing (OWASP recommended). Use over bcrypt -- no 72-byte password limit, memory-hard against GPU attacks. For v1's simple per-user auth. | HIGH |
| crypto/aes + crypto/cipher (stdlib) | Go stdlib | Credential encryption at rest | AES-256-GCM for encrypting stored API keys and tokens in SQLite. Standard Go crypto primitives, no third-party needed. | HIGH |

#### Configuration & CLI

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| BurntSushi/toml | v1.5.0 | TOML config parsing | TOML v1.1.0 compliant. Human-friendly config format for `agent.toml` and `server.toml`. Stable, zero drama. | HIGH |
| spf13/cobra | v1.8.x | CLI framework | Standard Go CLI framework. Handles subcommands (`serve`, `ca init`, `ca issue-server`, `ca issue-agent`), flags, help text. Used by Docker, Kubernetes, Hugo. | HIGH |

#### Logging & Observability

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| log/slog | Go stdlib (1.21+) | Structured logging | Standard library structured logging. JSON output for production, text for development. No external dependency. 650ns/op is fine for a control plane. Use `slog.With()` for contextual fields (machine_id, session_id). | HIGH |

---

### Frontend — React SPA

#### Core Framework

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| React | 19.2.x | UI framework | Stable since Dec 2024, now at 19.2.1 (Dec 2025). React Compiler for automatic memoization, Actions API, Suspense fully supported. The architecture doc says "React 18+" but 19 is stable and the project is greenfield -- start on the current major. | HIGH |
| TypeScript | 5.7+ | Type safety | Non-negotiable for a project this complex. Strict mode. | HIGH |
| Vite | 7.x | Build tool | Vite 7 is current stable (7.3.1). Fast HMR, clean config, React plugin works out of the box. Vite 6 is still receiving security patches but 7 is the active version. No reason to start on an older major for a new project. | HIGH |

#### Routing & State

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| react-router | v7.13.x | Client-side routing | v7 is stable (March 2026). Non-breaking upgrade from v6. Simplified package -- everything from `react-router`, no separate `react-router-dom`. Nested layouts for the shell/workbench structure. | HIGH |
| Zustand | 5.x | Client state management | Minimal boilerplate, excellent TypeScript support, middleware for devtools/persistence. v5.0.11 is current. Handles UI state: sidebar collapse, active terminal tab, theme. NOT for server data (that's TanStack Query). | HIGH |
| @tanstack/react-query | v5.90.x | Server state / data fetching | Caching, background refetch, optimistic updates for REST API calls (sessions list, machines list, job CRUD). v5 is the React version (v6 is Svelte-only). Suspense-compatible. | HIGH |

#### Terminal Emulation

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| @xterm/xterm | 6.0.0 | Terminal emulator | v6 released Dec 2025. 30% smaller bundle (265kb from 379kb), shadow DOM support, improved ligatures. The industry standard -- VS Code, Theia, Hyper all use xterm.js. No alternative comes close. | HIGH |
| @xterm/addon-fit | 6.x | Terminal resize | Auto-fits terminal to container dimensions. Sends resize events back through WebSocket to agent (SIGWINCH to PTY). | HIGH |
| @xterm/addon-webgl | 6.x | GPU-accelerated rendering | WebGL2 renderer for smooth scrolling on long outputs. Falls back to canvas/DOM renderer if WebGL2 unavailable. Essential for Claude CLI output which can be verbose. | HIGH |
| @xterm/addon-canvas | 6.x | Fallback renderer | 2D canvas fallback when WebGL2 isn't available. Better perf than DOM renderer. | MEDIUM |

**Important:** Use the scoped `@xterm/*` packages, NOT the old `xterm` + `xterm-addon-*` packages (deprecated, no longer maintained).

#### Styling & UI

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Tailwind CSS | v4.x | Utility-first CSS | v4 is a ground-up rewrite (Jan 2025). 5x faster builds, CSS-native config via `@import "tailwindcss"`, OKLCH colors, built-in container queries. No `tailwind.config.js` needed for most cases -- use CSS `@theme` blocks. | HIGH |
| Lucide React | 0.577.x | Icons | Clean, consistent, tree-shakeable SVG icons. Active development (weekly releases). Fork of Feather Icons with 10K+ dependents. | HIGH |
| Sonner | latest | Toast notifications | Minimal config, beautiful defaults. For session events, connection status, errors. | MEDIUM |

#### Data Fetching & Real-time

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Native WebSocket API | browser built-in | Terminal I/O streaming | Raw WebSocket for binary terminal data. No abstraction layer -- terminal I/O needs direct byte-level control. `ArrayBuffer` messages, not text framing. | HIGH |
| reconnecting-websocket | 4.4.0 | WebSocket auto-reconnect | Thin wrapper for auto-reconnect with exponential backoff. Last published 2019 but stable and dependency-free. For the non-terminal WebSocket connections (activity feed, status updates). For terminal WebSocket, implement reconnect manually since you need scrollback replay logic anyway. | MEDIUM |
| ky | latest | HTTP client | Lightweight fetch wrapper with retry, timeout, JSON parsing. For REST API calls via TanStack Query. Cleaner than raw fetch, lighter than axios. | MEDIUM |

#### Dev Dependencies

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| ESLint | 9.x | Linting | Flat config format (eslint.config.js). Use with typescript-eslint. | HIGH |
| Prettier | 3.x | Code formatting | Standard. | HIGH |
| Vitest | 3.x | Unit testing | Vite-native test runner. Same config, same transforms. Fast. | HIGH |

---

### Development & Build Tooling

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Go toolchain | 1.23+ | Backend build | `go build` produces static binaries. `CGO_ENABLED=0` with modernc.org/sqlite (pure Go). Cross-compile with `GOOS`/`GOARCH`. | HIGH |
| Task (go-task) | latest | Task runner / Makefile replacement | YAML-based, cross-platform, better than Make for Go projects. Define tasks for `buf generate`, `go build`, `vite build`, `embed frontend`. | MEDIUM |
| buf CLI | latest | Protobuf management | Lint, generate, breaking change detection for .proto files. | HIGH |

### Embedding Frontend in Go Binary

The Go server embeds the built frontend using `embed.FS`:

```go
//go:embed frontend/dist/*
var frontendFS embed.FS
```

This is how you get "single binary per role" -- the server binary contains the React SPA. No separate static file server needed.

---

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| gRPC framework | grpc-go | ConnectRPC | ConnectRPC's value is browser compatibility. Agent-server uses mTLS + bidi streaming, not browsers. grpc-go is battle-tested for exactly this pattern. |
| WebSocket (Go) | coder/websocket | gorilla/websocket | Gorilla uses its own HTTP/2 implementation, doesn't integrate with net/http ecosystem. coder/websocket is idiomatic, context-aware, maintained by Coder (who build terminal-over-web products). |
| SQLite driver | modernc.org/sqlite | mattn/go-sqlite3 | mattn is faster but requires CGO. Single-binary cross-compilation without a C toolchain is a hard requirement. Performance difference is negligible for metadata workloads. |
| SQLite driver | modernc.org/sqlite | zombiezen/go-sqlite | zombiezen wraps modernc internally. Adds another layer of abstraction without clear benefit. Go with modernc directly via database/sql. |
| State management | Zustand | Redux / MobX | Zustand does everything needed with 1/10th the boilerplate. Redux is overkill for a control plane UI. |
| CSS framework | Tailwind CSS v4 | Styled-components / Emotion | CSS-in-JS adds runtime overhead. Tailwind is zero-runtime, faster to iterate, and v4 is a significant improvement. |
| HTTP client (frontend) | ky | axios | axios is 14KB gzipped, ky is 3KB. Both do the same thing. Lighter is better. |
| Build tool | Vite 7 | Webpack | Webpack is slower, more config, more pain. No reason to use it in 2026. |
| Router | react-router v7 | TanStack Router | TanStack Router has excellent type safety but react-router v7 is mature, stable, and the team already knows it. Lower risk for v1. |
| Terminal emulator | xterm.js | None | There is no alternative. xterm.js is the only production-grade terminal emulator for the web. |
| Protobuf tooling | buf | raw protoc | buf is faster, easier to configure, handles imports cleanly. No reason to use protoc directly anymore. |
| Config format | TOML | YAML / JSON | TOML is human-friendly for config files. YAML has footguns (implicit type coercion). JSON lacks comments. |
| CLI framework | cobra | urfave/cli | cobra is the Go standard. More documentation, more ecosystem support. |
| Logging | slog (stdlib) | zerolog / zap | slog is in the standard library since Go 1.21. Zero dependency, good enough performance. Only reach for zerolog/zap if sub-microsecond logging matters (it doesn't here). |
| SSR framework | None (SPA) | Next.js / Remix | This is a SPA served by the Go binary. No SEO, no server-side rendering needed. Adding an SSR framework would mean a separate Node.js process, breaking the single-binary constraint. |
| DAG visualization | ReactFlow | D3.js | D3 is too low-level. ReactFlow gives drag-and-drop, zoom, pan, edge routing for free. Purpose-built for node graphs. |
| Password hashing | Argon2id | bcrypt | Argon2id is OWASP-recommended (2025). Memory-hard, no 72-byte limit. bcrypt is showing its age. |
| WebSocket reconnect | reconnecting-websocket | Custom implementation | Stable, zero-dependency, API-compatible with native WebSocket. For non-terminal connections only. |

---

## Installation

### Backend

```bash
# Initialize Go module
go mod init github.com/your-org/claude-plane

# Core dependencies
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get github.com/go-chi/chi/v5@latest
go get github.com/go-chi/cors@latest
go get github.com/coder/websocket@latest
go get github.com/creack/pty@latest
go get modernc.org/sqlite@latest
go get github.com/golang-jwt/jwt/v5@latest
go get golang.org/x/crypto@latest
go get github.com/BurntSushi/toml@latest
go get github.com/spf13/cobra@latest

# Protobuf tooling (install globally or via buf)
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Task runner
go install github.com/go-task/task/v3/cmd/task@latest

# buf CLI (via brew or direct download)
# brew install bufbuild/buf/buf
```

### Frontend

```bash
# Create Vite project
npm create vite@latest frontend -- --template react-ts
cd frontend

# Core dependencies
npm install react@latest react-dom@latest
npm install react-router@latest
npm install zustand@latest
npm install @tanstack/react-query@latest
npm install @xterm/xterm@latest @xterm/addon-fit@latest @xterm/addon-webgl@latest @xterm/addon-canvas@latest
npm install lucide-react@latest
npm install sonner@latest
npm install ky@latest
npm install reconnecting-websocket@latest

# Dev dependencies
npm install -D tailwindcss@latest
npm install -D eslint@latest prettier@latest
npm install -D typescript-eslint@latest
npm install -D vitest@latest @testing-library/react@latest
npm install -D @types/react@latest @types/react-dom@latest
```

---

## Key Architecture Decisions Locked by Stack

1. **Pure Go SQLite (modernc.org) means CGO_ENABLED=0 builds.** Cross-compilation works everywhere. Trade-off: ~2x slower than C SQLite on heavy queries. Acceptable for metadata.

2. **grpc-go for agent comms, WebSocket for browser.** Two protocol boundaries. The server is the bridge: it speaks gRPC to agents and WebSocket to browsers. Terminal I/O flows: browser WebSocket --> server --> gRPC bidi stream --> agent --> PTY.

3. **Embedded frontend via `embed.FS`.** Single binary. No nginx, no static file server. `go build` produces the server binary with the React SPA baked in.

4. **slog for logging, not zerolog/zap.** Zero external dependencies for logging. Good enough for a control plane. Structured JSON in production, text in development.

5. **Argon2id for passwords, JWT for sessions.** Modern auth stack for v1. When OIDC comes in v2, JWT infrastructure is already in place.

---

## Sources

- [grpc-go releases](https://github.com/grpc/grpc-go/releases) -- v1.79.x (March 2026)
- [go-chi/chi v5](https://pkg.go.dev/github.com/go-chi/chi/v5) -- v5.2.x (Feb 2026)
- [coder/websocket](https://github.com/coder/websocket) -- successor to nhooyr/websocket
- [creack/pty](https://github.com/creack/pty) -- v1.1.x, standard PTY library
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) -- pure Go SQLite driver
- [golang-jwt/jwt v5](https://github.com/golang-jwt/jwt) -- production-ready JWT
- [BurntSushi/toml v1.5.0](https://github.com/BurntSushi/toml/releases) -- June 2025
- [Go slog](https://go.dev/blog/slog) -- stdlib structured logging
- [React 19.2](https://react.dev/blog/2025/10/01/react-19-2) -- Oct 2025 stable
- [Vite 7](https://vite.dev/blog/announcing-vite7) -- current stable
- [React Router v7.13](https://reactrouter.com/changelog) -- March 2026
- [Zustand 5.x](https://github.com/pmndrs/zustand/releases) -- v5.0.11
- [TanStack Query v5.90](https://github.com/tanstack/query/releases) -- latest for React
- [xterm.js 6.0](https://github.com/xtermjs/xterm.js/releases) -- Dec 2025, 30% smaller bundle
- [Tailwind CSS v4](https://tailwindcss.com/blog/tailwindcss-v4) -- Jan 2025 rewrite
- [Lucide React](https://lucide.dev/) -- 0.577.x, weekly releases
- [buf CLI](https://buf.build/) -- protobuf tooling
- [ConnectRPC streaming docs](https://connectrpc.com/docs/go/streaming/) -- bidi requires HTTP/2
- [Argon2 OWASP recommendation](https://guptadeepak.com/the-complete-guide-to-password-hashing-argon2-vs-bcrypt-vs-scrypt-vs-pbkdf2-2026/)
- [Go SQLite bench](https://github.com/cvilsmeier/go-sqlite-bench) -- CGO vs pure Go comparison
- [reconnecting-websocket](https://github.com/pladaria/reconnecting-websocket) -- v4.4.0, stable
