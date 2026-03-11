# Phase 2: Agent Core - Research

**Researched:** 2026-03-11
**Domain:** Go gRPC mTLS client, bidirectional streaming, reconnection with exponential backoff, PTY process spawning
**Confidence:** HIGH

## Summary

Phase 2 implements the agent binary's core runtime: loading mTLS certificates and dialing the server, calling `Register()`, opening the persistent `AgentStream()` bidirectional stream, handling reconnection with exponential backoff, and spawning a Claude CLI process in a PTY with output relay to the gRPC stream.

All technologies are mature Go ecosystem staples. The gRPC client with mTLS uses Go's standard `crypto/tls` plus `google.golang.org/grpc/credentials`. Bidirectional streaming is a first-class gRPC feature with well-documented patterns. The `creack/pty` library (v1.1.24, used by 25k+ projects) handles PTY allocation. The primary complexity is in the reconnection state machine -- ensuring sessions continue running locally while the agent re-establishes its gRPC connection, and correctly re-registering surviving sessions on reconnect.

Phase 1 provides the protobuf contract, TLS certificate loading utilities (`internal/shared/tlsutil/`), and agent config parsing (`internal/agent/config/`). Phase 2 builds on all of these. The server-side gRPC listener with mTLS verification is also a Phase 1 deliverable that Phase 2 depends on.

**Primary recommendation:** Focus on getting the reconnection state machine right. The mTLS dial and bidirectional stream setup are straightforward. The hard part is cleanly separating the gRPC connection lifecycle from the session (PTY) lifecycle so that sessions survive disconnection.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| AGNT-02 | Agent authenticates to server using mTLS with its issued certificate | Load agent cert+key+CA from config paths, build `tls.Config` with `tls.RequireAndVerifyClientCert` equivalent for client side, use `credentials.NewTLS()` as gRPC dial option |
| AGNT-03 | Agent registers with server and maintains persistent gRPC bidirectional stream | Call `Register()` RPC with machine-id + capabilities, open `AgentStream()` bidi stream, implement keepalive + reconnection with exponential backoff (1s-60s), re-register on reconnect with surviving session states |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `google.golang.org/grpc` | v1.79+ | gRPC client framework | Standard Go gRPC implementation; provides `grpc.Dial`, streaming, keepalive, credentials |
| `google.golang.org/grpc/credentials` | (part of grpc) | TLS credential provider | `credentials.NewTLS()` wraps `tls.Config` for gRPC transport |
| `google.golang.org/grpc/keepalive` | (part of grpc) | HTTP/2 PING-based keepalive | Prevents idle connection closure by firewalls/load balancers; critical for long-lived streams |
| `github.com/creack/pty` | v1.1.24 | PTY allocation and process spawning | The standard Go PTY library; used by 25k+ projects; supports Linux, macOS, BSD |
| `crypto/tls` | stdlib | TLS configuration | Build mTLS `tls.Config` with client cert, CA pool |
| `crypto/x509` | stdlib | Certificate pool management | `x509.NewCertPool()` + `AppendCertsFromPEM()` for CA verification |
| `log/slog` | stdlib | Structured logging | Consistent with Phase 1 choices |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `google.golang.org/grpc/peer` | (part of grpc) | Extract peer info from context | Server-side: extract machine-id from client cert CN |
| `math/rand/v2` | stdlib | Jitter for backoff | Add randomization to exponential backoff to avoid thundering herd |
| `os/exec` | stdlib | Process spawning | Build `exec.Cmd` for Claude CLI before passing to `pty.StartWithSize()` |
| `sync` | stdlib | Concurrency primitives | `sync.Mutex` for session map, `sync.WaitGroup` for graceful shutdown |
| `context` | stdlib | Cancellation propagation | Separate contexts for connection lifecycle vs session lifecycle |
| `io` | stdlib | Stream copying | `io.Copy`-style loops between PTY and gRPC stream |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `creack/pty` | `os/exec` with pipes | Pipes don't support terminal control sequences, ANSI colors, interactive programs -- PTY is required |
| Manual reconnect loop | `grpc-ecosystem/go-grpc-middleware/retry` | Retry middleware works for unary RPCs, NOT for bidirectional streams -- manual reconnect loop is required |
| Custom keepalive | `grpc/keepalive` | Built-in keepalive handles HTTP/2 PINGs correctly; custom heartbeat messages are unnecessary overhead |

**Installation:**
```bash
# Phase 2 new dependencies (on top of Phase 1)
go get github.com/creack/pty@latest

# Already available from Phase 1:
# google.golang.org/grpc
# google.golang.org/protobuf
# BurntSushi/toml
# spf13/cobra
```

## Architecture Patterns

### Recommended Project Structure (Phase 2 additions)

```
internal/
├── agent/
│   ├── config/
│   │   └── config.go          # (Phase 1) Agent config struct + TOML loading
│   ├── client.go              # gRPC client: dial, register, stream, reconnect loop
│   ├── session.go             # PTY session: spawn CLI, read/write, lifecycle
│   └── session_manager.go     # Session registry: map[sessionID]*Session, thread-safe
├── server/
│   └── grpc/
│       ├── server.go          # gRPC server: mTLS listener, AgentService impl
│       ├── auth.go            # Interceptor: extract machine-id from cert CN, validate allowlist
│       └── connection_mgr.go  # Track connected agents: machineID -> stream
└── shared/
    ├── proto/                 # (Phase 1) Generated protobuf code
    └── tlsutil/               # (Phase 1) Cert loading, tls.Config builders
```

### Pattern 1: Agent Connection Lifecycle (Reconnect State Machine)

**What:** The agent maintains a persistent connection to the server using a reconnect loop that wraps the gRPC dial + Register + AgentStream sequence. Sessions (PTY processes) are decoupled from the connection -- they continue running during disconnection.

**When to use:** Any agent that must survive server restarts and network interruptions.

**Example:**
```go
// Source: gRPC best practices + backend_v1.md section 3.6
type AgentClient struct {
    cfg        *config.AgentConfig
    tlsCreds   credentials.TransportCredentials
    sessions   *SessionManager
    logger     *slog.Logger

    mu         sync.Mutex
    conn       *grpc.ClientConn
    stream     pb.AgentService_AgentStreamClient
    connected  bool
}

func (a *AgentClient) Run(ctx context.Context) error {
    for {
        err := a.connectAndServe(ctx)
        if ctx.Err() != nil {
            return ctx.Err() // Graceful shutdown
        }
        a.logger.Warn("connection lost, reconnecting", "error", err)
        backoff := a.waitWithBackoff(ctx)
        if backoff != nil {
            return backoff // Context cancelled during backoff
        }
    }
}

func (a *AgentClient) connectAndServe(ctx context.Context) error {
    // 1. Dial with mTLS
    conn, err := grpc.NewClient(
        a.cfg.Server.Address,
        grpc.WithTransportCredentials(a.tlsCreds),
        grpc.WithKeepaliveParams(keepalive.ClientParameters{
            Time:                30 * time.Second,
            Timeout:             10 * time.Second,
            PermitWithoutStream: true,
        }),
    )
    if err != nil {
        return fmt.Errorf("dial: %w", err)
    }
    defer conn.Close()

    client := pb.NewAgentServiceClient(conn)

    // 2. Register
    resp, err := client.Register(ctx, &pb.RegisterRequest{
        MachineId:        a.cfg.Agent.MachineID,
        MaxSessions:      int32(a.cfg.Agent.MaxSessions),
        ExistingSessions: a.sessions.GetStates(), // Report surviving sessions
    })
    if err != nil {
        return fmt.Errorf("register: %w", err)
    }
    if !resp.Accepted {
        return fmt.Errorf("registration rejected: %s", resp.RejectReason)
    }

    // 3. Open bidirectional stream
    stream, err := client.AgentStream(ctx)
    if err != nil {
        return fmt.Errorf("open stream: %w", err)
    }

    a.mu.Lock()
    a.conn = conn
    a.stream = stream
    a.connected = true
    a.mu.Unlock()

    defer func() {
        a.mu.Lock()
        a.connected = false
        a.stream = nil
        a.conn = nil
        a.mu.Unlock()
    }()

    // 4. Event loop: receive commands, dispatch to sessions
    return a.eventLoop(ctx, stream)
}
```

### Pattern 2: Exponential Backoff with Jitter

**What:** After a disconnection, wait an increasing amount of time before reconnecting. Add jitter to prevent thundering herd when multiple agents reconnect simultaneously after a server restart.

**When to use:** Every reconnect loop.

**Example:**
```go
// Source: gRPC connection backoff spec + backend_v1.md
type Backoff struct {
    current time.Duration
    min     time.Duration  // 1s (from agent.toml reconnect_min_interval)
    max     time.Duration  // 60s (from agent.toml reconnect_max_interval)
}

func NewBackoff(min, max time.Duration) *Backoff {
    return &Backoff{current: min, min: min, max: max}
}

func (b *Backoff) Next() time.Duration {
    d := b.current
    b.current *= 2
    if b.current > b.max {
        b.current = b.max
    }
    // Add 20% jitter: [0.8*d, 1.2*d]
    jitter := d / 5
    d += time.Duration(rand.Int64N(int64(2*jitter))) - jitter
    return d
}

func (b *Backoff) Reset() {
    b.current = b.min
}

func (a *AgentClient) waitWithBackoff(ctx context.Context) error {
    d := a.backoff.Next()
    a.logger.Info("waiting before reconnect", "delay", d)
    select {
    case <-time.After(d):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Pattern 3: PTY Session Management

**What:** Each session wraps a Claude CLI process in a PTY. Output is continuously read and either buffered locally (when detached) or relayed to the gRPC stream (when attached). The session continues running regardless of gRPC connection state.

**When to use:** For every CLI session spawned by the agent.

**Example:**
```go
// Source: creack/pty docs + backend_v1.md section 3.2-3.3
type Session struct {
    ID         string
    Status     string // "starting", "running", "completed", "failed"
    ptyFile    *os.File
    cmd        *exec.Cmd
    mu         sync.Mutex
    attached   bool
    outputCh   chan []byte      // Buffered channel for gRPC relay
    startedAt  time.Time
    exitCode   int
    logger     *slog.Logger
}

func NewSession(id string, createCmd *pb.CreateSessionCmd, logger *slog.Logger) (*Session, error) {
    cmd := exec.Command(createCmd.Command, createCmd.Args...)
    cmd.Dir = createCmd.WorkingDir

    // Set environment variables (API keys, etc.)
    cmd.Env = os.Environ()
    for k, v := range createCmd.EnvVars {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    // Start in PTY with specified terminal size
    ws := &pty.Winsize{
        Rows: uint16(createCmd.TerminalSize.Rows),
        Cols: uint16(createCmd.TerminalSize.Cols),
    }
    ptmx, err := pty.StartWithSize(cmd, ws)
    if err != nil {
        return nil, fmt.Errorf("start pty: %w", err)
    }

    s := &Session{
        ID:        id,
        Status:    "running",
        ptyFile:   ptmx,
        cmd:       cmd,
        outputCh:  make(chan []byte, 256),
        startedAt: time.Now(),
        logger:    logger.With("session_id", id),
    }

    // Start output reader goroutine
    go s.readLoop()
    // Start process waiter goroutine
    go s.waitForExit()

    return s, nil
}

func (s *Session) readLoop() {
    buf := make([]byte, 4096)
    for {
        n, err := s.ptyFile.Read(buf)
        if n > 0 {
            data := make([]byte, n)
            copy(data, buf[:n])

            // Always buffer output (for scrollback in future phases)
            // If attached and channel not full, send to gRPC relay
            select {
            case s.outputCh <- data:
            default:
                // Channel full -- drop for now (flow control in Phase 4)
            }
        }
        if err != nil {
            return // PTY closed (process exited)
        }
    }
}

func (s *Session) WriteInput(data []byte) error {
    _, err := s.ptyFile.Write(data)
    return err
}

func (s *Session) Resize(rows, cols uint16) error {
    return pty.Setsize(s.ptyFile, &pty.Winsize{Rows: rows, Cols: cols})
}

func (s *Session) waitForExit() {
    err := s.cmd.Wait()
    s.mu.Lock()
    defer s.mu.Unlock()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok {
            s.exitCode = exitErr.ExitCode()
            s.Status = "completed"
        } else {
            s.exitCode = -1
            s.Status = "failed"
        }
    } else {
        s.exitCode = 0
        s.Status = "completed"
    }
    s.ptyFile.Close()
    close(s.outputCh) // Signal output readers that session is done
}
```

### Pattern 4: Server-Side mTLS Interceptor (machine-id extraction)

**What:** A gRPC interceptor on the server extracts the machine-id from the client certificate's CN and validates it against the allowlist. This identity is attached to the context for downstream handlers.

**When to use:** On every gRPC call from agents.

**Example:**
```go
// Source: grpc-go peer package + credentials.TLSInfo
func MachineAuthInterceptor(allowedMachines map[string]bool) grpc.UnaryServerInterceptor {
    return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
        machineID, err := extractMachineID(ctx)
        if err != nil {
            return nil, status.Errorf(codes.Unauthenticated, "cert error: %v", err)
        }
        if !allowedMachines[machineID] {
            return nil, status.Errorf(codes.PermissionDenied, "machine %q not in allowlist", machineID)
        }
        ctx = context.WithValue(ctx, machineIDKey, machineID)
        return handler(ctx, req)
    }
}

func extractMachineID(ctx context.Context) (string, error) {
    p, ok := peer.FromContext(ctx)
    if !ok {
        return "", fmt.Errorf("no peer in context")
    }
    tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
    if !ok {
        return "", fmt.Errorf("no TLS info")
    }
    if len(tlsInfo.State.PeerCertificates) == 0 {
        return "", fmt.Errorf("no client certificate")
    }
    cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
    // Expected format: "agent-<machine-id>" per backend_v1.md
    if !strings.HasPrefix(cn, "agent-") {
        return "", fmt.Errorf("unexpected cert CN: %s", cn)
    }
    return strings.TrimPrefix(cn, "agent-"), nil
}
```

### Anti-Patterns to Avoid

- **Sharing a single context between connection lifecycle and session lifecycle:** Sessions must outlive connections. Use a root context for the agent process, derived contexts for connections, and separate derived contexts for sessions. Never cancel session contexts when the connection drops.

- **Using `grpc-ecosystem/go-grpc-middleware/retry` for streaming RPCs:** The retry middleware only works for unary and server-streaming RPCs. Bidirectional streams cannot be retried transparently -- you must implement a manual reconnect loop that creates a new stream.

- **Sending on the gRPC stream without checking connection state:** When the connection drops, `stream.Send()` returns an error. Check `a.connected` before attempting to send, and gracefully handle send failures by buffering output locally.

- **Blocking on `stream.Recv()` without a way to break out:** Use `stream.Context().Done()` or a shared cancellation context so the recv loop can exit when the connection is being torn down for reconnection.

- **Not re-registering surviving sessions on reconnect:** After reconnection, the server has lost state about which sessions are running on this agent. The `Register()` call MUST include `existing_sessions` so the server can reconcile.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PTY allocation | Raw `syscall.Openpty` calls | `creack/pty` | Platform differences (Linux vs macOS vs BSD), session leader setup, controlling terminal |
| HTTP/2 keepalive | Custom heartbeat messages over the stream | `grpc/keepalive.ClientParameters` | HTTP/2 PING is already implemented correctly at the transport layer; custom heartbeats add unnecessary message overhead |
| TLS configuration | Manual `tls.Dial` | `credentials.NewTLS()` + `grpc.WithTransportCredentials()` | gRPC integrates TLS into its connection management and handles renegotiation |
| Exponential backoff formula | Ad-hoc `time.Sleep(n * time.Second)` | Structured `Backoff` type with min/max/jitter | Easy to get wrong (no jitter = thundering herd, no cap = minutes-long waits) |
| Process signal handling | `os.Kill` directly | `cmd.Process.Signal(syscall.SIGTERM)` + timeout + `SIGKILL` | Graceful shutdown needs escalation pattern |

**Key insight:** The complexity in Phase 2 is not in any single component -- it is in correctly composing them. The reconnect loop, session manager, and gRPC stream relay must interact without deadlocks or leaked goroutines.

## Common Pitfalls

### Pitfall 1: Goroutine Leaks on Reconnection

**What goes wrong:** Each connection attempt spawns goroutines (recv loop, send loop, output relay per session). When the connection breaks, these goroutines must be cleanly stopped before starting new ones on the next connection attempt. Without proper cancellation, goroutines accumulate across reconnections.
**Why it happens:** Developers cancel the parent context but don't wait for goroutines to actually exit before creating new ones.
**How to avoid:** Use a `sync.WaitGroup` for all per-connection goroutines. In `connectAndServe()`, defer `wg.Wait()` after cancelling the connection context. Only return (allowing the next connection attempt) after all goroutines have exited.
**Warning signs:** Memory growth over time, especially in agents that frequently reconnect. `runtime.NumGoroutine()` increasing monotonically.

### Pitfall 2: Keepalive Mismatch Between Client and Server

**What goes wrong:** Client sends keepalive pings more frequently than the server's `EnforcementPolicy.MinTime` allows. Server sends GOAWAY and forcibly closes the connection. Agent reconnects, gets GOAWAY again, enters a reconnect-GOAWAY loop.
**Why it happens:** Client `Time` is set below server `MinTime`. Or `PermitWithoutStream` is true on client but false on server's `EnforcementPolicy`.
**How to avoid:** Coordinate settings. Recommended values:
- Client: `Time=30s, Timeout=10s, PermitWithoutStream=true`
- Server: `EnforcementPolicy{MinTime: 15s, PermitWithoutStream: true}`
- Server: `ServerParameters{Time: 30s, Timeout: 10s}`
**Warning signs:** Agent logs show rapid connect/disconnect cycles with "transport is closing" errors.

### Pitfall 3: PTY Read Blocking After Process Exit

**What goes wrong:** `ptyFile.Read()` blocks indefinitely even after the child process has exited, because the PTY master fd is not closed until all references are gone.
**Why it happens:** On Linux, the PTY master read returns `EIO` when the slave side is closed (process exit). But race conditions exist: `cmd.Wait()` may return before the final PTY read completes, or vice versa.
**How to avoid:** Structure the cleanup so `waitForExit()` closes the PTY file after `cmd.Wait()` returns, which causes `readLoop()` to get an error and exit. Use the closed `outputCh` channel as the definitive "session done" signal.
**Warning signs:** Goroutines stuck in `ptyFile.Read()` for exited processes.

### Pitfall 4: Not Using `grpc.NewClient` (grpc-go v1.63+)

**What goes wrong:** Using the deprecated `grpc.Dial` or `grpc.DialContext` which perform eager connection establishment and behave differently with name resolution.
**Why it happens:** Older examples and tutorials use `grpc.Dial`.
**How to avoid:** Use `grpc.NewClient()` which was introduced in grpc-go v1.63 (January 2024). It returns immediately and connects lazily. The first RPC (`Register()`) will trigger the actual connection.
**Warning signs:** Deprecation warnings in build output.

### Pitfall 5: Stream Send/Recv Concurrency

**What goes wrong:** Multiple goroutines call `stream.Send()` concurrently (e.g., output from multiple sessions), causing data corruption or panics.
**Why it happens:** gRPC streams are NOT safe for concurrent Send or concurrent Recv. Each direction (Send, Recv) can be called from one goroutine at a time.
**How to avoid:** Funnel all outgoing events through a single sender goroutine that reads from a channel. Each session writes to the channel; the sender goroutine serializes calls to `stream.Send()`.
**Warning signs:** Panics with "concurrent stream Send" or garbled protobuf messages.

## Code Examples

### mTLS Client Configuration

```go
// Source: crypto/tls docs + grpc/credentials docs
func buildTLSCreds(cfg *config.TLSConfig) (credentials.TransportCredentials, error) {
    // Load agent certificate and key
    cert, err := tls.LoadX509KeyPair(cfg.AgentCert, cfg.AgentKey)
    if err != nil {
        return nil, fmt.Errorf("load agent cert: %w", err)
    }

    // Load CA certificate for verifying the server
    caCert, err := os.ReadFile(cfg.CACert)
    if err != nil {
        return nil, fmt.Errorf("read CA cert: %w", err)
    }
    caPool := x509.NewCertPool()
    if !caPool.AppendCertsFromPEM(caCert) {
        return nil, fmt.Errorf("failed to parse CA cert")
    }

    tlsCfg := &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caPool,
        MinVersion:   tls.VersionTLS13,
    }

    return credentials.NewTLS(tlsCfg), nil
}
```

### Event Loop (Receive Commands, Dispatch)

```go
// Source: gRPC bidirectional streaming patterns
func (a *AgentClient) eventLoop(ctx context.Context, stream pb.AgentService_AgentStreamClient) error {
    // Start sender goroutine (serializes all outgoing events)
    sendCh := make(chan *pb.AgentEvent, 64)
    var wg sync.WaitGroup

    wg.Add(1)
    go func() {
        defer wg.Done()
        for evt := range sendCh {
            if err := stream.Send(evt); err != nil {
                a.logger.Error("stream send failed", "error", err)
                return
            }
        }
    }()

    // Start output relay for all active sessions
    a.sessions.StartRelay(sendCh)

    // Receive loop
    for {
        cmd, err := stream.Recv()
        if err != nil {
            close(sendCh)
            wg.Wait()
            return fmt.Errorf("stream recv: %w", err)
        }
        a.handleCommand(ctx, cmd, sendCh)
    }
}

func (a *AgentClient) handleCommand(ctx context.Context, cmd *pb.ServerCommand, sendCh chan<- *pb.AgentEvent) {
    switch c := cmd.Command.(type) {
    case *pb.ServerCommand_CreateSession:
        a.handleCreateSession(ctx, c.CreateSession, sendCh)
    case *pb.ServerCommand_InputData:
        a.sessions.WriteInput(c.InputData.SessionId, c.InputData.Data)
    case *pb.ServerCommand_ResizeTerminal:
        a.sessions.Resize(c.ResizeTerminal.SessionId,
            uint16(c.ResizeTerminal.Size.Rows),
            uint16(c.ResizeTerminal.Size.Cols))
    case *pb.ServerCommand_KillSession:
        a.sessions.Kill(c.KillSession.SessionId, c.KillSession.Signal)
    case *pb.ServerCommand_AttachSession:
        a.sessions.Attach(c.AttachSession.SessionId)
    case *pb.ServerCommand_DetachSession:
        a.sessions.Detach(c.DetachSession.SessionId)
    }
}
```

### Server-Side gRPC Setup with mTLS

```go
// Source: grpc-go mTLS examples
func newGRPCServer(cfg *config.ServerConfig, allowedMachines map[string]bool) (*grpc.Server, error) {
    // Load server cert
    cert, err := tls.LoadX509KeyPair(cfg.TLS.ServerCert, cfg.TLS.ServerKey)
    if err != nil {
        return nil, err
    }

    // Load CA for verifying agent certs
    caCert, err := os.ReadFile(cfg.TLS.CACert)
    if err != nil {
        return nil, err
    }
    caPool := x509.NewCertPool()
    caPool.AppendCertsFromPEM(caCert)

    tlsCfg := &tls.Config{
        Certificates: []tls.Certificate{cert},
        ClientCAs:    caPool,
        ClientAuth:   tls.RequireAndVerifyClientCert,
        MinVersion:   tls.VersionTLS13,
    }

    srv := grpc.NewServer(
        grpc.Creds(credentials.NewTLS(tlsCfg)),
        grpc.KeepaliveParams(keepalive.ServerParameters{
            Time:    30 * time.Second,
            Timeout: 10 * time.Second,
        }),
        grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
            MinTime:             15 * time.Second,
            PermitWithoutStream: true,
        }),
        grpc.UnaryInterceptor(MachineAuthInterceptor(allowedMachines)),
        grpc.StreamInterceptor(MachineAuthStreamInterceptor(allowedMachines)),
    )

    return srv, nil
}
```

### Graceful Shutdown

```go
// Source: Go signal handling patterns
func (a *AgentClient) RunWithSignals() error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

    errCh := make(chan error, 1)
    go func() {
        errCh <- a.Run(ctx)
    }()

    select {
    case sig := <-sigCh:
        a.logger.Info("received signal, shutting down", "signal", sig)
        cancel() // Triggers reconnect loop exit
        // Sessions continue running -- agent restart will re-register them
        return nil
    case err := <-errCh:
        return err
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `grpc.Dial()` | `grpc.NewClient()` | grpc-go v1.63 (Jan 2024) | Lazy connection, proper name resolution, non-blocking |
| TLS 1.2 default | TLS 1.3 (`MinVersion: tls.VersionTLS13`) | Go 1.22+ ecosystem shift | Stronger ciphers, faster handshake (1-RTT) |
| Custom heartbeat messages | `keepalive.ClientParameters` | Always available, newly emphasized | HTTP/2 native PING, no application-level overhead |
| `creack/pty` v1 only | Still v1.1.24 (no v2 released) | Current as of 2024 | v1 is stable and actively maintained |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + `go test` |
| Config file | None -- Go uses `*_test.go` files |
| Quick run command | `go test ./internal/agent/... ./internal/server/grpc/... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AGNT-02 | Agent loads mTLS certs and server rejects invalid/missing certs | integration | `go test ./internal/agent/... -run TestMTLSAuth -count=1` | Wave 0 |
| AGNT-02 | Server extracts machine-id from cert CN and validates allowlist | unit | `go test ./internal/server/grpc/... -run TestMachineAuth -count=1` | Wave 0 |
| AGNT-03 | Agent calls Register() and receives acceptance | integration | `go test ./internal/agent/... -run TestRegister -count=1` | Wave 0 |
| AGNT-03 | AgentStream bidi stream stays alive across idle periods (keepalive) | integration | `go test ./internal/agent/... -run TestStreamKeepalive -count=1 -short` | Wave 0 |
| AGNT-03 | Agent reconnects with exponential backoff after connection drop | unit | `go test ./internal/agent/... -run TestReconnectBackoff -count=1` | Wave 0 |
| AGNT-03 | Agent re-registers with surviving session states after reconnect | integration | `go test ./internal/agent/... -run TestReconnectReregister -count=1` | Wave 0 |
| (SC-4) | Agent spawns Claude CLI in PTY and relays output | unit | `go test ./internal/agent/... -run TestPTYSession -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/agent/... ./internal/server/grpc/... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps
- [ ] `internal/agent/client_test.go` -- covers AGNT-02 (mTLS dial), AGNT-03 (register, stream, reconnect)
- [ ] `internal/agent/session_test.go` -- covers PTY spawn and output relay
- [ ] `internal/agent/backoff_test.go` -- covers exponential backoff with jitter
- [ ] `internal/server/grpc/auth_test.go` -- covers machine-id extraction and allowlist validation
- [ ] `internal/server/grpc/server_test.go` -- covers gRPC server setup with mTLS
- [ ] Test helpers: generate ephemeral CA + certs for integration tests (reuse `tlsutil` from Phase 1)

## Open Questions

1. **Stream interceptor for bidirectional streams**
   - What we know: Unary interceptors do not apply to streaming RPCs. grpc-go has `grpc.StreamInterceptor` but extracting peer info works the same way via `peer.FromContext`.
   - What's unclear: Whether we need a separate `StreamServerInterceptor` or if the `WrappedServerStream` pattern from grpc-middleware is needed.
   - Recommendation: Implement a simple `StreamServerInterceptor` that extracts machine-id and attaches it to context. The pattern is identical to unary except the function signature differs.

2. **Agent binary entrypoint integration with cobra**
   - What we know: Phase 1 sets up cobra for the agent binary with a root command.
   - What's unclear: Whether Phase 2 adds a `run` or `connect` subcommand, or if the default command starts the agent.
   - Recommendation: Make the default behavior (no subcommand) start the agent. This follows the "scp and run" principle. A `run` subcommand is acceptable but unnecessary for a single-purpose binary.

3. **Session output during disconnection**
   - What we know: Sessions continue running during disconnection. Output should eventually be written to scrollback files (Phase 4). In Phase 2, we need output buffering but scrollback persistence is not yet required.
   - What's unclear: How much output to buffer in memory during disconnection.
   - Recommendation: Use a bounded channel (256 messages). Drop oldest when full. Phase 4 adds scrollback file persistence. For Phase 2, losing some output during disconnection is acceptable -- the reconnection replay mechanism is Phase 4 scope.

## Sources

### Primary (HIGH confidence)
- `docs/internal/product/backend_v1.md` -- gRPC protocol definition (sections 3.6, 5.1-5.5), agent process model, session lifecycle, config format
- [grpc-go keepalive package docs](https://pkg.go.dev/google.golang.org/grpc/keepalive) -- ClientParameters, ServerParameters, EnforcementPolicy struct definitions
- [creack/pty package docs](https://pkg.go.dev/github.com/creack/pty) -- Start, StartWithSize, Setsize API (v1.1.24)
- [grpc-go credentials package](https://pkg.go.dev/google.golang.org/grpc/credentials) -- NewTLS, TLSInfo for mTLS
- [gRPC connection backoff spec](https://github.com/grpc/grpc/blob/master/doc/connection-backoff.md) -- exponential backoff algorithm

### Secondary (MEDIUM confidence)
- [gRPC authentication guide](https://grpc.io/docs/guides/auth/) -- mTLS setup patterns
- [gRPC keepalive guide](https://grpc.io/docs/guides/keepalive/) -- coordination between client and server settings
- [grpc-go peer package](https://github.com/grpc/grpc-go/issues/111) -- extracting client cert CN from context

### Tertiary (LOW confidence)
- None -- all Phase 2 technologies are well-documented with official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries are mature, well-documented, specified in project design docs
- Architecture: HIGH -- agent process model and gRPC protocol are fully defined in backend_v1.md
- Pitfalls: HIGH -- gRPC streaming concurrency, keepalive coordination, and PTY lifecycle issues are well-documented with known solutions
- Reconnection pattern: HIGH -- follows gRPC official backoff spec and community best practices

**Research date:** 2026-03-11
**Valid until:** 2026-04-11 (stable technologies, 30-day validity)
