# Pitfalls Research

**Domain:** Remote CLI session management control plane (Go backend, React frontend, gRPC + WebSocket streaming, PTY management, mTLS, SQLite)
**Researched:** 2026-03-11
**Confidence:** HIGH (verified against official docs, library issues, and community post-mortems)

## Critical Pitfalls

### Pitfall 1: PTY File Descriptor and Zombie Process Leaks

**What goes wrong:**
Every session spawns a PTY (master/slave fd pair) and a child process. If the agent does not correctly close the master fd when the session ends, or fails to `Wait()` on the child process, you accumulate leaked file descriptors and zombie processes. On a long-running agent managing dozens of sessions over days, this exhausts the fd limit (typically 1024 soft) and fills the process table. The agent appears healthy but silently cannot create new sessions.

**Why it happens:**
Go's `creack/pty` returns an `*os.File` for the master fd. Developers call `pty.Start()` but forget to `defer ptmx.Close()` on every exit path, especially error paths. The child process requires an explicit `cmd.Wait()` call to reap it -- if the PTY read loop exits due to an error (broken pipe, context cancel) before `Wait()` is called, the child becomes a zombie. Go's garbage collector does not call `Wait()` on `exec.Cmd`.

**How to avoid:**
- Always pair `pty.Start()` with a deferred cleanup function that closes the master fd AND calls `cmd.Wait()`.
- Use a session supervisor goroutine per session that owns the lifecycle: create PTY, spawn process, run I/O loop, and on any exit (normal or error), close fd, wait on process, clean up scrollback writer.
- Set `FD_CLOEXEC` on the master fd (Go does this by default for `os.File`, but verify with `creack/pty`).
- Run `goleak` in tests to catch leaked goroutines tied to unclosed fds.
- Monitor `/proc/self/fd` count in the agent's health report.

**Warning signs:**
- Agent health reports show increasing fd count over time.
- `ps aux` on the agent machine shows zombie (`Z`) processes parented to the agent.
- New session creation fails with "too many open files" after days of uptime.

**Phase to address:**
Phase 1 (Agent core) -- this must be correct from the start. A leaky PTY manager cannot be retrofitted; it must be designed with ownership semantics from day one.

---

### Pitfall 2: gRPC Bidirectional Stream Goroutine Leaks on Disconnect

**What goes wrong:**
The agent maintains a persistent bidirectional gRPC stream (`AgentStream`). When the server goes down or the network drops, the `Send()` and `Recv()` goroutines on the agent side can leak if not properly coordinated. Each leaked goroutine holds references to the stream, preventing GC. After several reconnect cycles, the agent accumulates hundreds of leaked goroutines, consuming memory and eventually deadlocking.

**Why it happens:**
gRPC bidirectional streams require careful coordination between the send and receive goroutines. The common mistake: the receive goroutine blocks on `stream.Recv()`, the send goroutine blocks on `stream.Send()`, and when the stream breaks, only one gets an error. The other remains blocked. Developers use `select` with a context, but forget that `stream.Send()` is not cancellable via context -- it blocks until the transport notices the connection is dead (which can take minutes without keepalive). Additionally, not calling `CloseSend()` on the client side causes the server to leak two goroutines per stream (per grpc-go issue #6457).

**How to avoid:**
- Use a single "stream manager" goroutine that owns the stream lifecycle. When it detects a broken stream (error from Recv or Send), it cancels a context that all child goroutines select on.
- Configure gRPC keepalive on both client and server: `keepalive.ClientParameters{Time: 10s, Timeout: 5s, PermitWithoutStream: true}` on the agent, matching `keepalive.ServerParameters` and `keepalive.EnforcementPolicy` on the server.
- Always call `CloseSend()` before closing the `ClientConn`.
- Use `goleak` in integration tests that simulate network drops.
- Never use unbuffered channels for passing data between send/recv goroutines -- use buffered channels with drain logic on shutdown.

**Warning signs:**
- Agent memory grows after each reconnect cycle (visible in health reports).
- `runtime.NumGoroutine()` increases monotonically.
- Server-side: goroutine dump shows `transport.(*http2Server).HandleStreams` goroutines piling up.

**Phase to address:**
Phase 1 (Agent-server connectivity) -- the reconnect loop is the first thing built and the hardest to test. Build it with explicit lifecycle management from the start.

---

### Pitfall 3: Terminal I/O Backpressure Failure (Browser Drowning)

**What goes wrong:**
Claude CLI can produce output in bursts -- large code blocks, file contents, or verbose tool output. This data flows: PTY -> agent -> gRPC -> server -> WebSocket -> xterm.js. Without backpressure, the browser's xterm.js instance accumulates data faster than it can render, causing the tab to freeze, scroll position to jump, and eventually the WebSocket write buffer to grow unboundedly on the server side (because the browser stops reading). xterm.js has a hardcoded 50MB input buffer -- data beyond that is silently dropped.

**Why it happens:**
Each hop in the chain (PTY, gRPC, WebSocket, xterm.js) has independent buffering. Developers build each hop in isolation and test with slow interactive typing, never with burst output. The PTY produces data at disk speed, gRPC streams it eagerly, the WebSocket pumps it into the browser, and xterm.js cannot render fast enough. There is no signal from the browser back to the PTY to slow down.

**How to avoid:**
- Implement end-to-end flow control using xterm.js's `write(data, callback)` API. The callback fires when the chunk is processed. Use high/low watermarks (HIGH <= 500KB, LOW = 100KB) to pause/resume the WebSocket.
- On the server side: track pending bytes per WebSocket connection. If pending exceeds a threshold (e.g., 1MB), stop reading from the gRPC stream for that session. This backpressure propagates to the agent, which stops reading from the PTY.
- On the agent side: if the gRPC send buffer is full, pause reading from the PTY master fd. The kernel's PTY buffer (typically 4KB) will fill, and the child process will block on writes -- this is correct POSIX behavior.
- Coalesce writes on the browser side: accumulate chunks within a `requestAnimationFrame` window and flush once per frame (~60fps).

**Warning signs:**
- Browser tab becomes unresponsive during large output (e.g., `cat` of a big file in the CLI).
- Server memory spikes correlated with specific sessions.
- Users report "lost" output or scroll position jumping.

**Phase to address:**
Phase 2 (Terminal streaming) -- after basic streaming works, add flow control before calling it done. Do NOT defer this to "optimization" -- it is a correctness issue.

---

### Pitfall 4: Scrollback Replay Race Condition on Reconnect

**What goes wrong:**
When a user reconnects to a running session, the server must: (1) request the scrollback from the agent, (2) stream it to the browser, (3) seamlessly switch to live output. If the switch from "replaying history" to "live streaming" has a gap, the user misses output. If it has overlap, the user sees duplicate output. Both break the illusion of a persistent terminal.

**Why it happens:**
The scrollback file on the agent is being actively written by the scrollback writer goroutine while simultaneously being read for replay. The naive approach -- "read the file, then start streaming" -- has a race: new output arrives between the file read completing and the live stream starting. The file is append-only (asciicast v2 JSONL), but coordinating the handover requires knowing the exact byte offset where replay ends and live begins.

**How to avoid:**
- Use an offset-based protocol: when the agent receives `RequestScrollbackCmd`, it records the current scrollback file position (byte offset), sends all data up to that offset as scrollback chunks, then begins live streaming from that offset forward. The scrollback writer and live streamer share a mutex-protected offset counter.
- On the browser side: do not write scrollback and live data to xterm.js simultaneously. Write scrollback first (possibly in a single `term.write()` batch for performance), then switch to live.
- Include sequence numbers in `SessionOutputEvent` messages. The browser tracks the last-seen sequence number on disconnect and sends it on reconnect, allowing the server to resume from the exact point.
- Test this with a script that produces continuous output while rapidly connecting/disconnecting.

**Warning signs:**
- Users report missing lines or duplicate lines after reconnection.
- Terminal state looks "garbled" after reconnect (ANSI escape codes split across the boundary).
- Scrollback replay takes unexpectedly long (replaying the entire history instead of just missed output).

**Phase to address:**
Phase 2 (Session persistence) -- this is the hardest part of the "sessions survive disconnection" promise. Build the offset tracking into the scrollback format from day one, not as an afterthought.

---

### Pitfall 5: SQLite "Database is Locked" Under Concurrent gRPC + HTTP Load

**What goes wrong:**
The server handles concurrent HTTP requests (REST API) and gRPC events (agent session updates, health reports) that all write to the same SQLite database. Without proper connection management, `SQLITE_BUSY` errors surface as 500s in the API and dropped agent events. WAL mode helps readers, but writers still serialize -- and Go's `database/sql` connection pool opens multiple connections, all of which may try to write simultaneously.

**Why it happens:**
Go's `database/sql` pool default `MaxOpenConns` is unlimited. With multiple connections, each can start a write transaction independently, and SQLite's write lock causes all but one to get `SQLITE_BUSY`. The `busy_timeout` pragma is per-connection and must be set on every new connection, but developers set it once on the first connection and assume it persists. Additionally, `BEGIN` (deferred) transactions that read then write cause more contention than `BEGIN IMMEDIATE` because the lock upgrade from shared to exclusive can deadlock against other deferred transactions.

**How to avoid:**
- Use a **single writer connection** pattern: one `*sql.DB` with `MaxOpenConns=1` for all writes, a separate `*sql.DB` with higher `MaxOpenConns` for reads. This eliminates write contention entirely.
- Set `PRAGMA busy_timeout = 5000` via the DSN string (`?_busy_timeout=5000`) so it applies to every connection in the pool automatically.
- Set `PRAGMA journal_mode=WAL`, `PRAGMA synchronous=NORMAL`, `PRAGMA foreign_keys=ON` on connection init using a `ConnInitFunc`.
- Keep write transactions short -- one statement per transaction where possible.
- Never use `BEGIN` (deferred) for transactions that will write. Use `BEGIN IMMEDIATE` to take the write lock upfront and fail fast rather than deadlock.

**Warning signs:**
- Intermittent "database is locked" errors in server logs, especially under load.
- Agent health updates silently failing (logged as errors but not surfaced).
- API responses returning 500 sporadically, especially for session creation.

**Phase to address:**
Phase 1 (Server foundation) -- the database layer is foundational. Get the single-writer pattern right before building anything on top of it.

---

### Pitfall 6: mTLS Certificate Expiry Causing Silent Agent Disconnection

**What goes wrong:**
Self-signed certificates generated by the built-in CA have an expiry date. When certificates expire, agents silently fail to reconnect after any connection drop. The server logs show TLS handshake errors, but the agent just retries with exponential backoff forever. There is no user-facing alert. A team discovers their agents have been offline for days only when they try to create a new session.

**Why it happens:**
Developers generate certificates during initial setup and forget about them. The architecture doc mentions "no secrets to rotate" as a benefit of mTLS, which creates a false sense of security -- certificates are not secrets, but they do expire. Self-signed CAs commonly default to 1-year validity. There is no reminder mechanism built into the system.

**How to avoid:**
- Set a long default validity for the CA (10 years) and agent/server certificates (2 years).
- Store certificate `NotAfter` dates in the server database when agents register.
- Add a certificate expiry warning to the server's health check endpoint and UI: "Agent nuc-01 certificate expires in 30 days."
- Log certificate `NotAfter` on every successful agent connection.
- Support hot-reloading of certificates: use `tls.Config.GetCertificate` / `tls.Config.GetConfigForClient` callbacks so new certs can be loaded without restarting the server or agent.
- Document the renewal process in the CLI: `claude-plane-server ca renew-agent --machine-id nuc-01`.

**Warning signs:**
- Agent connections fail after exactly 1 year of operation.
- TLS handshake errors in server logs: "certificate has expired or is not yet valid."
- Agents in perpetual reconnect loop with no sessions possible.

**Phase to address:**
Phase 1 (mTLS setup) -- set long validity defaults and add expiry tracking from the start. Hot-reload can be Phase 2, but the monitoring must exist in Phase 1.

---

### Pitfall 7: WebSocket Connection Lifecycle Mismanagement

**What goes wrong:**
The server maintains WebSocket connections for each attached terminal session. When the browser tab closes, the network drops, or the user navigates away, the WebSocket close event may not fire (especially on mobile networks or laptop lid close). The server keeps the dead WebSocket in memory, continues buffering output for it, and does not send a `DetachSession` command to the agent. Accumulated dead WebSockets leak memory and leave sessions in a phantom "attached" state.

**Why it happens:**
WebSocket close frames are not guaranteed to arrive. TCP keepalive defaults are too long (2 hours on Linux). Developers rely on the `onclose` event, which only fires for clean closes, not for network drops. The server has no mechanism to detect a dead connection until it tries to write and gets an error -- but if no output is being produced, the write never happens.

**How to avoid:**
- Implement application-level ping/pong: server sends a WebSocket ping every 15 seconds, expects a pong within 5 seconds. If no pong, consider the connection dead.
- Set `WriteDeadline` on every WebSocket write. If a write blocks for more than 10 seconds, the connection is dead.
- On detected dead connection: send `DetachSession` to the agent, remove the WebSocket from the session registry, update session state to "detached."
- Use a connection reaper goroutine that periodically scans for stale connections.
- On the frontend: implement reconnection with exponential backoff. Use `visibilitychange` event to detect tab becoming visible and proactively reconnect.

**Warning signs:**
- Server memory grows over time, especially with many users.
- Sessions show as "attached" in the UI but no user is actually viewing them.
- Agent continues streaming live output for sessions no one is watching.

**Phase to address:**
Phase 2 (WebSocket terminal streaming) -- build ping/pong and dead connection detection into the WebSocket handler from the start.

---

### Pitfall 8: Terminal Resize Propagation Delay and Size Mismatch

**What goes wrong:**
When the user resizes their browser window, the new terminal dimensions must propagate: xterm.js -> WebSocket -> server -> gRPC -> agent -> `pty.Setsize()`. If this chain has any delay or dropped messages, the PTY and xterm.js disagree on the terminal size. CLI programs that use `TIOCGWINSZ` (like Claude CLI with its rich TUI) render output for the wrong dimensions, causing wrapped lines, misaligned columns, and garbled full-screen layouts.

**Why it happens:**
Browser resize events fire rapidly (60+ per second during a drag). Each one generates a WebSocket message, a gRPC command, and a `pty.Setsize()` syscall. Without debouncing, the agent is overwhelmed with resize commands, and race conditions between resize and output rendering cause visual corruption. Additionally, on reconnection, the initial terminal size from xterm.js may not match the PTY size that was set during the previous connection.

**How to avoid:**
- Debounce resize events on the frontend: 100-150ms. Only send the final size after the user stops dragging.
- On reconnect, always send the current terminal size as part of the `AttachSession` flow, before any output is rendered.
- On the agent side, serialize resize operations per session -- do not process a new resize while a previous one is in flight.
- Include terminal dimensions in the `RequestScrollbackCmd` so the agent can resize the PTY before replaying scrollback.

**Warning signs:**
- Terminal output looks garbled after resizing, especially with TUI applications.
- Lines wrap incorrectly or columns misalign after browser resize.
- Reconnection shows output formatted for the wrong terminal size.

**Phase to address:**
Phase 2 (Terminal streaming) -- debounce on frontend, size sync on reconnect.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Single gRPC stream for all sessions per agent | Simpler connection model | Multiplexing complexity, one slow session blocks others, hard to add per-session flow control | Acceptable for v1 if session count per agent is low (<10). Revisit if per-session streams needed. |
| Storing scrollback as raw files instead of indexed chunks | Simple append-only writes | Replay of long sessions requires reading entire file; no random access | Acceptable for v1. Add chunk index in v2 when sessions last hours. |
| Plaintext scrollback files on agent disk | No encryption overhead | Scrollback contains all terminal output, possibly secrets typed by user | Acceptable for v1 if agent machines are trusted. Add encryption-at-rest for v2 multi-tenant. |
| `mattn/go-sqlite3` (CGo) over `modernc.org/sqlite` (pure Go) | Battle-tested, widely used | CGo complicates cross-compilation, adds C toolchain dependency | Acceptable. CGo is fine for server binary. Switch only if cross-compilation becomes a blocker. |
| No audit log for v1 | Faster development | Cannot answer "who did what" when something goes wrong | Never acceptable -- even a minimal append-only audit log is trivial and invaluable for debugging. |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| xterm.js + WebSocket | Writing raw bytes directly to `term.write()` without buffering | Batch writes per animation frame, use `write(data, callback)` for flow control |
| gRPC + mTLS | Hardcoding cert paths, not validating cert CN against agent machine-id | Extract machine-id from peer certificate in gRPC interceptor, validate against allowlist |
| Claude CLI + PTY | Assuming Claude CLI uses stdout/stderr normally | Claude CLI uses a rich TUI (alternate screen buffer, cursor positioning). Must pass full terminal size and support alternate screen buffer in xterm.js. |
| asciicast v2 + replay | Replaying at original timing (real-time playback) | On reconnect, replay scrollback instantly (no timing delays), then switch to real-time for live output |
| SQLite + Go `database/sql` | Using default connection pool settings | Set `MaxOpenConns=1` for writer pool, `PRAGMA` settings via DSN string or `ConnInitFunc` |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Per-keystroke WebSocket messages | High message rate, server CPU spikes, head-of-line blocking | Batch input on frontend (5-10ms debounce for paste operations, immediate for single keys) | At 5+ concurrent active sessions |
| Unbounded scrollback file growth | Disk fills on agent machine, large replay payloads | Rotate at 50MB, index chunks, expose disk usage in health reports | Sessions running for days with verbose output |
| Full scrollback replay on every reconnect | Multi-second connection time, browser freezes on large sessions | Track last-seen offset per connection, replay only missed output | Sessions with >10MB of accumulated output |
| Individual `term.write()` per WebSocket message | Renderer thrashes, GPU spikes, dropped frames | Coalesce into one `term.write()` per animation frame | Burst output (>100 chunks/second) |
| gRPC stream per session (if implemented later) | Connection storm when agent has many sessions | Multiplex sessions over single stream, or limit concurrent streams per agent | At 20+ sessions per agent |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Logging terminal output in server logs | Credentials typed into CLI (API keys, passwords) appear in plaintext in logs | Never log terminal I/O data. Log session metadata (id, timestamps, sizes) only. |
| Agent cert private key with permissive file permissions | Any user on the agent machine can impersonate the agent | Set `chmod 600` on agent-key.pem. Verify permissions on agent startup, refuse to start if world-readable. |
| Transmitting credentials (ANTHROPIC_API_KEY) in `CreateSessionCmd` without verifying mTLS is active | Credentials transmitted in plaintext if TLS somehow fails | Verify TLS state on the connection before sending credentials. Use gRPC interceptor to reject non-TLS connections. |
| Certificate CN/SAN not validated against machine-id | A valid agent cert for machine A can impersonate machine B | Extract machine-id from cert CN in gRPC interceptor, compare against the `machine_id` the agent claims in `RegisterRequest`. Reject mismatches. |
| WebSocket endpoint accessible without authentication | Anyone with network access can attach to terminal sessions | Authenticate WebSocket upgrade request (session cookie / JWT in query param or first message). Reject unauthenticated upgrades. |
| No rate limiting on session creation | Malicious or buggy frontend creates hundreds of sessions, exhausting agent resources | Rate limit per user, per agent. Agent enforces `max_sessions` limit. |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| No visual indicator of connection state | User types into a disconnected terminal, input is lost | Show connection status badge on terminal (green/yellow/red). Queue input during reconnect and replay on success. |
| Replaying scrollback with original timing | User waits minutes to see session history | Replay scrollback instantly on reconnect. Only use timing for optional "playback" feature. |
| Terminal not focused after attach | User starts typing, keys go to the page instead of the terminal | Auto-focus xterm.js instance on attach. Use `term.focus()` after scrollback replay completes. |
| No confirmation on session kill | User accidentally kills a long-running session | Confirmation dialog for kill. No confirmation needed for detach (non-destructive). |
| Resize flicker during window resize | Terminal content flashes/reflows visually | Debounce resize, use CSS `resize: none` on terminal container, let xterm.js fit addon handle sizing smoothly |

## "Looks Done But Isn't" Checklist

- [ ] **PTY management:** Often missing child process reaping -- verify no zombie processes after session termination with `ps aux | grep Z`
- [ ] **gRPC reconnect:** Often missing state reconciliation on reconnect -- verify agent reports existing sessions in `RegisterRequest.existing_sessions` and server reconciles its state
- [ ] **Scrollback replay:** Often missing ANSI state continuity -- verify terminal colors and cursor position are correct after replay (not garbled by split escape sequences)
- [ ] **WebSocket auth:** Often missing auth on the upgrade request -- verify that opening a WebSocket to `/ws/terminal/:id` without a valid session cookie returns 401, not a connection
- [ ] **mTLS validation:** Often missing CN validation -- verify that swapping agent certs between machines causes connection rejection
- [ ] **SQLite concurrency:** Often missing `busy_timeout` -- verify that concurrent API requests during agent health updates do not return 500
- [ ] **Terminal resize:** Often missing resize-on-reconnect -- verify that attaching to a session from a different-sized browser window renders correctly
- [ ] **Session cleanup:** Often missing disk cleanup -- verify that terminated session scrollback files are archived/deleted after a retention period
- [ ] **Graceful shutdown:** Often missing drain logic -- verify that `SIGTERM` to the agent waits for active sessions to be properly detached (scrollback flushed) before exiting

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| PTY fd leak | LOW | Restart agent. Sessions survive restart because scrollback files are on disk. On reconnect, agent reports existing sessions. |
| gRPC goroutine leak | LOW | Restart agent. Same recovery as fd leak -- sessions are independent of gRPC state. |
| Scrollback replay corruption | MEDIUM | Clear xterm.js terminal state (`term.reset()`), re-request full scrollback. Add a "refresh" button in UI. |
| SQLite "database is locked" | LOW | Retry the failed operation. If persistent, restart server. Data is not lost -- WAL mode ensures durability. |
| Expired certificates | MEDIUM | Generate new certificates with `ca issue-agent`, copy to agent machine, restart agent. Sessions were already dead (no connection), so no data loss. |
| Dead WebSocket accumulation | LOW | Server restart clears all WebSocket state. Clients auto-reconnect. No data loss. |
| Terminal size mismatch | LOW | User manually resizes browser window, or clicks "refresh" to re-sync. No data loss. |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| PTY fd/zombie leaks | Phase 1 (Agent core) | Integration test: create 50 sessions, terminate all, verify fd count returns to baseline |
| gRPC goroutine leaks | Phase 1 (Agent connectivity) | Integration test: simulate 10 reconnect cycles, verify goroutine count is stable |
| Terminal I/O backpressure | Phase 2 (Terminal streaming) | Load test: pipe 100MB through a session, verify browser remains responsive |
| Scrollback replay race | Phase 2 (Session persistence) | Integration test: produce output while rapidly reconnecting, verify no gaps or duplicates |
| SQLite concurrency | Phase 1 (Server foundation) | Load test: 50 concurrent API requests + agent health updates, zero "database is locked" errors |
| Certificate expiry | Phase 1 (mTLS setup) | Unit test: generate cert with 1-minute validity, verify connection fails after expiry, verify warning is logged 30 days before |
| WebSocket lifecycle | Phase 2 (WebSocket streaming) | Integration test: open WebSocket, kill network (iptables), verify server detects dead connection within 30 seconds |
| Terminal resize propagation | Phase 2 (Terminal streaming) | Manual test: resize browser during active Claude CLI session, verify no rendering corruption |

## Sources

- [creack/pty Go package documentation](https://pkg.go.dev/github.com/creack/pty) -- PTY lifecycle management
- [grpc-go issue #6457: Bidi streaming memory leak when CloseSend not called](https://github.com/grpc/grpc-go/issues/6457) -- gRPC goroutine leak root cause
- [grpc-go issue #1269: Server leaks goroutines](https://github.com/grpc/grpc-go/issues/1269) -- server-side stream cleanup
- [xterm.js Flow Control guide](https://xtermjs.org/docs/guides/flowcontrol/) -- write callback API, watermark strategy
- [xterm.js issue #5447: GPU usage causing lag](https://github.com/xtermjs/xterm.js/issues/5447) -- burst buffering / coalescing
- [mattn/go-sqlite3 issue #1203: "database is locked" in WAL mode](https://github.com/mattn/go-sqlite3/issues/1203) -- WAL concurrency limitations
- [SQLite concurrent writes and "database is locked" errors](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/) -- comprehensive analysis of SQLite write contention
- [High Performance SQLite: Busy Timeout](https://highperformancesqlite.com/watch/busy-timeout) -- busy_timeout configuration
- [GitHub Security Lab: mTLS when certificate authentication is done wrong](https://github.blog/security/vulnerability-research/mtls-when-certificate-authentication-is-done-wrong/) -- certificate validation pitfalls
- [Dapr CLI issue #807: mTLS cert expiry notification](https://github.com/dapr/cli/issues/807) -- real-world cert expiry outage
- [go-reaper: Process reaper library for Go](https://github.com/ramr/go-reaper) -- zombie process cleanup patterns
- [Go 1.26 goroutine leak profile experiment](https://go.dev/doc/go1.26) -- new goroutine leak detection tooling

---
*Pitfalls research for: Remote CLI session management control plane*
*Researched: 2026-03-11*
