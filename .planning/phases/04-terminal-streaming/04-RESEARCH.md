# Phase 4: Terminal Streaming - Research

**Researched:** 2026-03-12
**Domain:** Real-time terminal I/O streaming across Browser-WebSocket-Server-gRPC-Agent-PTY pipeline
**Confidence:** HIGH

## Summary

Phase 4 is the core value delivery phase of claude-plane -- it connects the browser terminal to a remote Claude CLI session through a three-layer relay: WebSocket (browser to server), gRPC bidirectional stream (server to agent), and PTY (agent to CLI process). The architecture is well-specified in the design documents (`backend_v1.md` sections 3, 5, 8; `frontend_v1.md` section 5) and uses standard, battle-tested libraries at every layer.

The highest-risk areas flagged in STATE.md -- scrollback replay offset protocol and three-layer flow control -- are addressable with the patterns documented below. Scrollback replay is handled via the `ScrollbackChunkEvent` protocol (agent reads scrollback file, streams chunks with offset/is_final markers, then transitions to live `SessionOutputEvent`). Flow control leverages gRPC's built-in HTTP/2 flow control on the agent-server leg, and a combination of WebSocket bufferedAmount monitoring and client-side rate limiting on the server-browser leg.

**Primary recommendation:** Use `@xterm/xterm` 5.5.x (not 6.0 -- see rationale below), `creack/pty` v1.1.24 for PTY management, `coder/websocket` for Go WebSocket handling, and the standard `google.golang.org/grpc` for gRPC. The asciicast v2 scrollback format is already specified in the design docs and should be written as simple JSONL -- no external library needed.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| SESS-01 | User can create a new Claude CLI session on any connected machine | REST `POST /api/v1/sessions` + `CreateSessionCmd` gRPC command to agent, agent spawns PTY via `creack/pty.Start()` |
| SESS-02 | User can attach to an existing session and receive terminal output | WebSocket upgrade at `/ws/terminal/:sessionID`, server sends `RequestScrollbackCmd` then `AttachSessionCmd` to agent |
| SESS-03 | User can detach from a session without terminating it | WebSocket close triggers `DetachSessionCmd`, agent continues running PTY and writing scrollback |
| SESS-05 | User can terminate a session | REST `DELETE /api/v1/sessions/:id` sends `KillSessionCmd` (SIGTERM then SIGKILL), agent cleans up PTY |
| SESS-06 | Sessions continue running on the agent when user disconnects | PTY process is independent of gRPC/WebSocket state; scrollback writer goroutine always active |
| TERM-01 | Real-time terminal output in browser via xterm.js | `@xterm/xterm` 5.5.x with WebGL addon, binary WebSocket frames written directly to `term.write()` |
| TERM-02 | User can type and input reaches remote CLI | `term.onData()` sends binary frames over WebSocket, server relays as `InputDataCmd` via gRPC, agent writes to PTY stdin |
| TERM-03 | Browser window resize propagates to remote PTY | `term.onResize()` sends JSON resize message, server sends `ResizeTerminalCmd`, agent calls `pty.Setsize()` |
| TERM-04 | Flow control prevents fast output from overwhelming browser | Three-layer strategy: gRPC HTTP/2 flow control (agent-server), server-side channel buffering with drop policy (server-browser), WebSocket `bufferedAmount` monitoring |
</phase_requirements>

## Standard Stack

### Core (Backend -- Go)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/creack/pty` | v1.1.24 | PTY allocation, process spawning, terminal resize | De facto Go PTY library, stable, Linux/macOS/BSD support |
| `github.com/coder/websocket` | v1.8.14 | WebSocket server (terminal I/O) | Modern, context-aware, zero-alloc reads/writes, binary frame support, maintained by Coder |
| `google.golang.org/grpc` | latest | gRPC bidirectional streaming (agent-server) | Standard Go gRPC implementation, already used by Phase 2 |
| `github.com/go-chi/chi/v5` | latest | HTTP router for REST + WebSocket upgrade | Already used by Phase 3, clean middleware support |

### Core (Frontend -- TypeScript)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@xterm/xterm` | ^5.5.0 | Terminal emulator in browser | Industry standard, used by VS Code, production-proven |
| `@xterm/addon-fit` | ^0.10.0 | Auto-fit terminal to container | Required for responsive layout |
| `@xterm/addon-webgl` | ^0.18.0 | WebGL2-accelerated rendering | Performance critical for fast terminal output |
| `reconnecting-websocket` | ^4.4.0 | Auto-reconnecting WebSocket wrapper | Thin, zero-dep, drop-in WebSocket replacement |

### Why xterm.js 5.5 instead of 6.0

xterm.js 6.0.0 (released Dec 2024) has significant breaking changes:
- Canvas renderer addon removed entirely (only WebGL or DOM)
- EventEmitter API replaced with VS Code's internal event system
- Viewport/scrollbar implementation rewritten
- Alt key binding behavior changed

For a new project, 6.0 would normally be preferred. However, the design documents (`frontend_v1.md`) were written targeting the 5.x API surface (including the `xterm-addon-fit` naming convention). The 5.5.x line is stable and will receive security patches. **Recommendation: Start with 5.5.x for V1. Upgrade to 6.x in a future phase when the terminal layer is mature.**

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `coder/websocket` | `gorilla/websocket` | gorilla is more widely used but less idiomatic Go; coder/websocket has context support, zero-alloc, and is actively maintained by Coder |
| `creack/pty` | `os/exec` + manual PTY | No reason to hand-roll PTY management; creack/pty handles platform differences |
| `@xterm/xterm` 5.5 | `@xterm/xterm` 6.0 | 6.0 is newer but has breaking changes vs design docs; 5.5 is stable and sufficient |
| `reconnecting-websocket` | Custom reconnect logic | Not worth building; the library is 4KB, well-tested, and covers edge cases |

### Installation

**Backend (Go):**
```bash
go get github.com/creack/pty@v1.1.24
go get github.com/coder/websocket@v1.8.14
# grpc and chi should already be present from Phase 2/3
```

**Frontend (in `web/` directory):**
```bash
npm install @xterm/xterm@^5.5.0 @xterm/addon-fit@^0.10.0 @xterm/addon-webgl@^0.18.0
npm install reconnecting-websocket@^4.4.0
```

## Architecture Patterns

### Data Flow Architecture

```
Browser                     Server                          Agent
  |                           |                               |
  | xterm.js                  | Session Registry              | Session Manager
  | WebSocket client          | WebSocket handler             | PTY master fd
  |                           | gRPC client to agent          | gRPC server stream
  |                           |                               | Scrollback writer
  |                           |                               |
  |--- binary WS frame ----->|--- InputDataCmd (gRPC) ------>|--- write(ptyFd) -->
  |                           |                               |
  |<-- binary WS frame ------|<-- SessionOutputEvent ---------|<-- read(ptyFd) ---
  |                           |                               |
  |-- JSON {resize} -------->|--- ResizeTerminalCmd --------->|--- pty.Setsize() -
```

### Recommended Project Structure (Phase 4 additions)

```
internal/
├── agent/
│   ├── session/
│   │   ├── manager.go       # Session lifecycle (create, attach, detach, kill)
│   │   ├── pty.go           # PTY allocation, process spawning, resize
│   │   ├── scrollback.go    # Asciicast v2 writer, file rotation
│   │   └── relay.go         # PTY output -> gRPC stream relay goroutine
│   └── ...
├── server/
│   ├── session/
│   │   ├── registry.go      # In-memory session map, subscriber channels
│   │   ├── handler.go       # REST handlers (create, list, get, kill)
│   │   └── ws.go            # WebSocket terminal handler (bridge)
│   └── ...
web/
├── src/
│   ├── components/
│   │   └── terminal/
│   │       ├── TerminalView.tsx      # xterm.js container + chrome
│   │       ├── useTerminalSession.ts # WebSocket lifecycle hook
│   │       └── SessionPlayer.tsx     # Asciicast replay (completed sessions)
│   └── ...
```

### Pattern 1: Agent PTY Session Lifecycle

**What:** Agent manages PTY process independently of gRPC connection state
**When to use:** Every session creation

```go
// Agent session manager - creates and manages PTY processes
func (m *SessionManager) CreateSession(cmd *proto.CreateSessionCmd) error {
    // 1. Build the command
    c := exec.Command(cmd.Command, cmd.Args...)
    c.Dir = cmd.WorkingDir
    c.Env = buildEnv(cmd.EnvVars)

    // 2. Start in PTY
    ptmx, err := pty.StartWithSize(c, &pty.Winsize{
        Rows: uint16(cmd.TerminalSize.Rows),
        Cols: uint16(cmd.TerminalSize.Cols),
    })
    if err != nil {
        return fmt.Errorf("pty start: %w", err)
    }

    // 3. Create session state
    sess := &Session{
        ID:        cmd.SessionId,
        PTY:       ptmx,
        Process:   c.Process,
        Status:    StatusRunning,
        StartedAt: time.Now(),
    }

    // 4. Start scrollback writer (ALWAYS running)
    go sess.writeScrollback()

    // 5. Store session
    m.sessions[cmd.SessionId] = sess

    return nil
}
```

### Pattern 2: Server WebSocket-gRPC Bridge

**What:** Server relays terminal data between browser WebSocket and agent gRPC stream
**When to use:** Every terminal attachment

```go
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // 1. Authenticate and find session
    session, agent := s.resolveSession(sessionID)

    // 2. Upgrade to WebSocket (binary mode)
    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
    if err != nil {
        return
    }
    defer conn.CloseNow()

    // 3. Request scrollback replay first
    agent.SendCommand(&proto.RequestScrollbackCmd{SessionId: sessionID})

    // 4. Subscribe to session output channel
    outputCh := s.sessionRegistry.Subscribe(sessionID)
    defer s.sessionRegistry.Unsubscribe(sessionID, outputCh)

    ctx, cancel := context.WithCancel(r.Context())
    defer cancel()

    // 5. Agent output -> WebSocket (goroutine)
    go func() {
        for {
            select {
            case <-ctx.Done():
                return
            case data := <-outputCh:
                conn.Write(ctx, websocket.MessageBinary, data)
            }
        }
    }()

    // 6. WebSocket -> Agent input (main loop)
    for {
        _, msg, err := conn.Read(ctx)
        if err != nil {
            agent.SendCommand(&proto.DetachSessionCmd{SessionId: sessionID})
            return
        }
        // Distinguish binary (keystrokes) from text (control messages like resize)
        agent.SendCommand(&proto.InputDataCmd{
            SessionId: sessionID,
            Data:      msg,
        })
    }
}
```

### Pattern 3: Scrollback Replay on Reconnect

**What:** When user attaches to an existing session, replay buffered output then switch to live
**When to use:** Attaching to a running session that has prior output

```
Agent receives RequestScrollbackCmd:
1. Open scrollback.cast file
2. Read in chunks (e.g., 32KB)
3. Send ScrollbackChunkEvent for each chunk (with offset, total_bytes)
4. Send final ScrollbackChunkEvent (is_final=true)
5. Wait for AttachSessionCmd
6. Switch to live streaming via SessionOutputEvent
```

The server forwards scrollback chunks as binary WebSocket frames. The frontend writes them directly to `term.write()`. When `scrollback_end` control message arrives, the frontend enables keyboard input (`isReplayComplete = true`).

### Pattern 4: Frontend Terminal Hook

**What:** React hook managing xterm.js instance + WebSocket lifecycle
**When to use:** Every terminal view component

```typescript
function useTerminalSession(sessionId: string, containerRef: RefObject<HTMLDivElement>) {
  const termRef = useRef<Terminal | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<'connecting' | 'replaying' | 'live' | 'disconnected'>('connecting');

  useEffect(() => {
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, Menlo, monospace',
      fontSize: 14,
      theme: { background: '#1a1b26' },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    try {
      term.loadAddon(new WebglAddon());
    } catch {
      // Falls back to canvas/DOM renderer
    }

    term.open(containerRef.current!);
    fitAddon.fit();

    // WebSocket connection
    const ws = new WebSocket(`wss://${location.host}/ws/terminal/${sessionId}`);
    ws.binaryType = 'arraybuffer';

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      } else {
        const msg = JSON.parse(event.data);
        if (msg.type === 'scrollback_end') {
          setStatus('live');
        }
      }
    };

    // Keystrokes -> server (only when live)
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN && status === 'live') {
        ws.send(new TextEncoder().encode(data));
      }
    });

    // Resize -> server
    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }));
      }
    });

    // Handle window resize
    const observer = new ResizeObserver(() => fitAddon.fit());
    observer.observe(containerRef.current!);

    termRef.current = term;
    wsRef.current = ws;

    return () => {
      observer.disconnect();
      ws.close();
      term.dispose();
    };
  }, [sessionId]);

  return { status, term: termRef, ws: wsRef };
}
```

### Anti-Patterns to Avoid

- **Polling for terminal output:** Never poll REST endpoints for terminal data. Always use WebSocket push.
- **Base64-encoding terminal data:** Send binary WebSocket frames directly. Base64 adds ~33% overhead and is unnecessary.
- **Killing PTY on WebSocket close:** WebSocket close means "detach," not "kill." The session must survive browser disconnection.
- **Blocking gRPC stream on scrollback replay:** Use separate goroutines for scrollback replay and live streaming. Never block the main gRPC event loop.
- **Single goroutine for read+write on PTY:** Always use separate goroutines for reading from PTY (output) and writing to PTY (input) to prevent deadlocks.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PTY allocation + process management | Custom syscall wrappers for openpty/forkpty | `creack/pty` | Platform differences (Linux vs macOS vs BSD), signal handling, fd management |
| Terminal emulation in browser | Custom ANSI parser + canvas rendering | `@xterm/xterm` + WebGL addon | ANSI/VT100/xterm escape sequences are enormously complex; xterm.js handles thousands of edge cases |
| WebSocket auto-reconnection | Custom retry logic with timers | `reconnecting-websocket` | Exponential backoff, message buffering during reconnect, connection state management |
| Scrollback file format | Custom binary format | Asciicast v2 (JSONL) | Human-readable, debuggable, asciinema ecosystem compatibility, timestamps for replay |
| gRPC flow control | Custom windowing protocol | gRPC built-in HTTP/2 flow control | Automatic BDP-based window sizing, battle-tested, transparent to application code |

**Key insight:** Every layer of the terminal streaming pipeline has a well-established solution. The value of this phase is in the integration (correct relay between layers, proper lifecycle management) not in reimplementing any individual layer.

## Common Pitfalls

### Pitfall 1: PTY Fd Leak on Session Cleanup

**What goes wrong:** PTY file descriptors are not closed when a session terminates, leading to fd exhaustion on the agent machine.
**Why it happens:** Multiple code paths can end a session (user kill, CLI exit, agent shutdown) and it's easy to miss cleanup in one of them.
**How to avoid:** Use a single `Session.Close()` method called from a `defer` in the session goroutine. Close the PTY master fd, wait for the process, close the scrollback file, and remove from the session map -- in that order.
**Warning signs:** `lsof` shows growing number of `/dev/ptmx` file descriptors on the agent.

### Pitfall 2: Scrollback-to-Live Transition Gap

**What goes wrong:** After scrollback replay finishes and before live streaming starts, output produced during this gap is lost.
**Why it happens:** There's a window between the agent finishing scrollback replay (`is_final=true`) and the server sending `AttachSessionCmd` where the PTY may produce output that goes only to the scrollback file.
**How to avoid:** The agent should buffer PTY output during the transition. When `AttachSessionCmd` arrives, flush any buffered output since the last scrollback byte offset, then switch to live relay. The `ScrollbackChunkEvent.offset` field enables this -- the agent tracks the byte offset where scrollback replay ended and replays any new data from that offset before going live.
**Warning signs:** Users see a "jump" in terminal content when attaching to a session that was actively producing output.

### Pitfall 3: WebSocket Message Ordering Under Load

**What goes wrong:** Terminal output arrives at the browser out of order, causing garbled display.
**Why it happens:** If using multiple goroutines to write to the WebSocket without serialization.
**How to avoid:** Use a single writer goroutine for each WebSocket connection. Fan in all output (scrollback chunks, live output, control messages) through a single channel that the writer goroutine consumes sequentially.
**Warning signs:** Intermittent garbled terminal output, especially during scrollback replay.

### Pitfall 4: Resize Race Condition

**What goes wrong:** Terminal output is rendered with wrong dimensions immediately after a resize.
**Why it happens:** The resize command (sent via WebSocket -> gRPC -> agent -> `pty.Setsize()`) takes time to propagate, but the PTY may produce output with the old dimensions in the meantime.
**How to avoid:** This is inherent in any remote terminal and is generally acceptable. The terminal reflows on the next full-screen redraw. Do not try to buffer or rewrite output during resize propagation -- it creates worse problems.
**Warning signs:** Brief visual artifacts after resizing, which self-correct. This is normal behavior.

### Pitfall 5: Browser Tab Accumulation Memory Leak

**What goes wrong:** Multiple terminal sessions open in tabs consume growing amounts of memory.
**Why it happens:** xterm.js maintains a scrollback buffer in memory. With WebGL renderer, GPU memory can also accumulate.
**How to avoid:** Set `scrollback` option on xterm.js (e.g., 10000 lines). When a tab is hidden, consider pausing the WebGL renderer. Dispose xterm.js instances when tabs are closed. The design doc notes hidden tabs keep WebSocket connections open -- this is correct behavior, but monitor memory.
**Warning signs:** Browser memory growing over time with multiple sessions open.

### Pitfall 6: gRPC Stream Multiplexing Conflicts

**What goes wrong:** Multiple sessions on the same agent share a single gRPC bidirectional stream, and a slow session blocks fast ones.
**Why it happens:** If session output events are sent synchronously on the shared stream.
**How to avoid:** Each session should have its own output buffer (Go channel). A single sender goroutine consumes from all session channels (using `select` or a multiplexer) and writes to the gRPC stream. This way, one slow session's buffered data does not block another session's output.
**Warning signs:** Terminal responsiveness degrades when multiple sessions are active on the same agent.

## Code Examples

### Asciicast v2 Scrollback Writer (Go)

```go
// Source: Design doc backend_v1.md Section 3.4
type ScrollbackWriter struct {
    file    *os.File
    mu      sync.Mutex
    offset  int64
    started time.Time
}

func NewScrollbackWriter(path string, cols, rows uint32) (*ScrollbackWriter, error) {
    f, err := os.Create(path)
    if err != nil {
        return nil, err
    }

    // Write asciicast v2 header
    header := fmt.Sprintf(`{"version":2,"width":%d,"height":%d,"timestamp":%d}`,
        cols, rows, time.Now().Unix())
    n, err := fmt.Fprintln(f, header)
    if err != nil {
        f.Close()
        return nil, err
    }

    return &ScrollbackWriter{
        file:    f,
        offset:  int64(n),
        started: time.Now(),
    }, nil
}

func (w *ScrollbackWriter) WriteOutput(data []byte) error {
    w.mu.Lock()
    defer w.mu.Unlock()

    elapsed := time.Since(w.started).Seconds()
    // Escape the data for JSON string
    escaped := strings.ReplaceAll(string(data), `\`, `\\`)
    escaped = strings.ReplaceAll(escaped, `"`, `\"`)

    line := fmt.Sprintf(`[%f,"o","%s"]`, elapsed, escaped)
    n, err := fmt.Fprintln(w.file, line)
    w.offset += int64(n)
    return err
}

func (w *ScrollbackWriter) CurrentOffset() int64 {
    w.mu.Lock()
    defer w.mu.Unlock()
    return w.offset
}
```

### PTY Resize Handler (Go)

```go
// Source: creack/pty docs + design doc
func (s *Session) Resize(cols, rows uint32) error {
    return pty.Setsize(s.PTY, &pty.Winsize{
        Rows: uint16(rows),
        Cols: uint16(cols),
    })
}
```

### WebSocket Binary Frame Handling (Go, using coder/websocket)

```go
// Source: coder/websocket docs
import "github.com/coder/websocket"

// Reading binary frames (keystrokes from browser)
msgType, data, err := conn.Read(ctx)
if err != nil {
    return err // connection closed
}
if msgType == websocket.MessageBinary {
    // Raw keystroke data -> send to agent
    agent.SendInput(sessionID, data)
} else {
    // JSON control message (resize, etc.)
    var msg ControlMessage
    json.Unmarshal(data, &msg)
    handleControl(msg)
}

// Writing binary frames (terminal output to browser)
err = conn.Write(ctx, websocket.MessageBinary, outputData)
```

### xterm.js Initialization (TypeScript)

```typescript
// Source: xterm.js docs + design doc frontend_v1.md Section 5.1
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import '@xterm/xterm/css/xterm.css';

function createTerminal(container: HTMLElement): Terminal {
    const term = new Terminal({
        cursorBlink: true,
        fontFamily: 'JetBrains Mono, Menlo, Monaco, Consolas, monospace',
        fontSize: 14,
        scrollback: 10000,
        theme: {
            background: '#1a1b26',
            foreground: '#c0caf5',
        },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);

    // WebGL renderer (falls back silently)
    try {
        term.loadAddon(new WebglAddon());
    } catch {
        console.warn('WebGL not available, using default renderer');
    }

    term.open(container);
    fitAddon.fit();

    return term;
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `xterm` npm package | `@xterm/xterm` scoped package | xterm.js 5.4.0 (March 2024) | Must use `@xterm/` prefix for all imports |
| `xterm-addon-webgl` | `@xterm/addon-webgl` | xterm.js 5.4.0 (March 2024) | Same functionality, new package name |
| `nhooyr.io/websocket` | `github.com/coder/websocket` | 2024 | nhooyr.io import path still works but deprecated |
| `gorilla/websocket` (unmaintained period) | `gorilla/websocket` (resumed) or `coder/websocket` | 2023-2024 | gorilla resumed maintenance; coder/websocket is the modern alternative |
| Canvas renderer (xterm.js) | WebGL renderer | Ongoing | Canvas addon removed entirely in 6.0; WebGL is the performance path |

**Deprecated/outdated:**
- `xterm` (unscoped npm package): Use `@xterm/xterm` instead
- `nhooyr.io/websocket` import path: Use `github.com/coder/websocket`
- xterm.js canvas renderer addon: Removed in 6.0, prefer WebGL even in 5.5

## Flow Control Strategy (TERM-04)

This is the highest-risk requirement. Terminal output can be extremely fast (e.g., `cat` of a large file) and will overwhelm the browser if not managed.

### Three-Layer Flow Control

**Layer 1: Agent -> Server (gRPC)**
- gRPC uses HTTP/2 with automatic flow control based on Bandwidth Delay Product (BDP)
- When the server cannot consume fast enough, `stream.Send()` on the agent blocks automatically
- This backpressure propagates to the PTY read goroutine, which slows PTY reads
- **No custom code needed.** gRPC handles this transparently.

**Layer 2: Server -> Browser (WebSocket)**
- The server's output channel for each session should be a buffered Go channel (e.g., capacity 256)
- If the channel is full (browser is slow), the server drops the oldest messages or coalesces them
- The server can monitor WebSocket write latency and adapt
- **Implementation: Use a ring buffer channel. Log when drops occur.**

**Layer 3: Browser (xterm.js)**
- xterm.js `term.write()` is synchronous and fast (especially with WebGL)
- For extreme bursts, batch incoming WebSocket messages and write in animation frames
- Monitor `WebSocket.bufferedAmount` on the send side (not relevant for output direction)
- **Implementation: Write directly to `term.write()`. Only batch if performance testing shows issues.**

### Practical Approach for V1

For V1, a simple approach works:
1. Let gRPC handle agent-server backpressure (free).
2. Use a buffered channel (256 slots of max 32KB) on the server per session. If full, drop oldest.
3. Write directly to xterm.js. The WebGL renderer handles 60fps+ of terminal output.
4. Add metrics (dropped messages, channel depth) to monitor in production.

Sophisticated flow control (e.g., WebSocketStream API for true end-to-end backpressure) can be added in V2 if V1 monitoring shows issues.

## Open Questions

1. **Scrollback file encoding for binary data**
   - What we know: Asciicast v2 uses JSON strings for terminal data. Binary/non-UTF8 output needs escaping.
   - What's unclear: The exact escaping strategy for raw bytes (base64 within the JSON string? Hex escapes?)
   - Recommendation: Use Go's `json.Marshal` for the data field -- it handles binary data via unicode escaping automatically. Match what asciinema does.

2. **Maximum scrollback replay size**
   - What we know: Design doc mentions 50MB rotation threshold per scrollback file.
   - What's unclear: How long scrollback replay takes for a 50MB file over a typical connection.
   - Recommendation: Cap initial scrollback replay at 1MB or last N lines. Offer "load more" for full history. This is a UX decision, not a technical limitation.

3. **Multiple browser tabs attached to same session**
   - What we know: Design doc shows "Attached (1 viewer)" in session list.
   - What's unclear: Whether multiple browsers can attach simultaneously, and if so, how input conflicts are handled.
   - Recommendation: For V1, allow multiple viewers but only the first attachment gets input rights. Additional attachments are read-only. This avoids input conflicts without complexity.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go: `testing` + Vitest (frontend) |
| Config file | Go: none needed; Frontend: `web/vitest.config.ts` (from Phase 3) |
| Quick run command | `go test ./internal/agent/session/... ./internal/server/session/... -count=1 -short` |
| Full suite command | `go test ./... -count=1 && cd web && npm test` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SESS-01 | Create session spawns PTY | unit | `go test ./internal/agent/session/ -run TestCreateSession -count=1` | Wave 0 |
| SESS-02 | Attach returns scrollback then live | integration | `go test ./internal/server/session/ -run TestAttachSession -count=1` | Wave 0 |
| SESS-03 | Detach keeps PTY running | unit | `go test ./internal/agent/session/ -run TestDetachSession -count=1` | Wave 0 |
| SESS-05 | Kill terminates PTY process | unit | `go test ./internal/agent/session/ -run TestKillSession -count=1` | Wave 0 |
| SESS-06 | Session survives WS disconnect | integration | `go test ./internal/server/session/ -run TestSessionSurvivesDisconnect -count=1` | Wave 0 |
| TERM-01 | Output reaches xterm.js | integration | `go test ./internal/server/session/ -run TestOutputRelay -count=1` | Wave 0 |
| TERM-02 | Input reaches PTY | integration | `go test ./internal/server/session/ -run TestInputRelay -count=1` | Wave 0 |
| TERM-03 | Resize propagates to PTY | unit | `go test ./internal/agent/session/ -run TestResize -count=1` | Wave 0 |
| TERM-04 | Flow control under load | unit | `go test ./internal/server/session/ -run TestFlowControl -count=1` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/agent/session/... ./internal/server/session/... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 && cd web && npm test`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/agent/session/manager_test.go` -- covers SESS-01, SESS-03, SESS-05
- [ ] `internal/agent/session/pty_test.go` -- covers TERM-03
- [ ] `internal/agent/session/scrollback_test.go` -- covers scrollback writing
- [ ] `internal/server/session/registry_test.go` -- covers SESS-02, SESS-06
- [ ] `internal/server/session/ws_test.go` -- covers TERM-01, TERM-02, TERM-04
- [ ] `web/src/components/terminal/__tests__/useTerminalSession.test.ts` -- covers frontend hook

## Sources

### Primary (HIGH confidence)
- [creack/pty](https://github.com/creack/pty) -- v1.1.24, PTY management API (Start, Setsize, Winsize)
- [coder/websocket](https://github.com/coder/websocket) -- v1.8.14, Go WebSocket library
- [xterm.js releases](https://github.com/xtermjs/xterm.js/releases) -- v5.5.0 and v6.0.0 release notes
- [xterm.js official site](https://xtermjs.org/) -- addon documentation, API reference
- Design documents: `docs/internal/product/backend_v1.md` sections 3, 5, 8
- Design documents: `docs/internal/product/frontend_v1.md` section 5

### Secondary (MEDIUM confidence)
- [gRPC flow control](https://pkg.go.dev/google.golang.org/grpc/examples/features/flow_control) -- Official gRPC Go examples
- [Go Forum: WebSocket in 2025](https://forum.golangbridge.org/t/websocket-in-2025/38671) -- Gorilla vs coder/websocket comparison
- [reconnecting-websocket](https://www.npmjs.com/package/reconnecting-websocket) -- v4.4.0, auto-reconnect wrapper

### Tertiary (LOW confidence)
- [WebSocket backpressure](https://skylinecodes.substack.com/p/backpressure-in-websocket-streams) -- Backpressure patterns (general, not Go-specific)
- [WebSocketStream API](https://web.dev/websocketstream/) -- Chrome-only, not ready for production use

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- All libraries are well-established, versions verified against official sources
- Architecture: HIGH -- Design docs provide detailed protocol specifications; patterns match industry standard (VS Code remote terminals, Coder, Wetty)
- Pitfalls: HIGH -- Common issues in remote terminal systems are well-documented; specific mitigations identified
- Flow control: MEDIUM -- V1 strategy is sound (buffered channel + drop) but may need tuning in production

**Research date:** 2026-03-12
**Valid until:** 2026-04-12 (stable domain, libraries mature)
