---
phase: 02-agent-core
verified: 2026-03-12T10:05:00Z
status: passed
score: 15/15 must-haves verified
re_verification: false
---

# Phase 2: Agent Core Verification Report

**Phase Goal:** Agent binary can connect to server via mTLS, register, maintain bidirectional stream, spawn PTY sessions, and relay I/O
**Verified:** 2026-03-12T10:05:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | Agent loads mTLS certificates and dials the server successfully | VERIFIED | `NewAgentClient` calls `tlsutil.AgentTLSConfig`, wraps result in `credentials.NewTLS`, passes to `grpc.NewClient` |
| 2  | Server rejects agents with invalid or missing client certificates | VERIFIED | `extractMachineID` returns `codes.Unauthenticated` on missing peer/TLS; `TestAgentMTLSRejection` passes with cross-CA rejection |
| 3  | Server extracts machine-id from agent certificate CN | VERIFIED | `extractMachineID` strips `agent-` prefix from CN; `TestExtractMachineID_ValidCert` verifies "nuc-01" extracted |
| 4  | Agent calls Register() and receives acceptance from server | VERIFIED | `connectAndServe` calls `client.Register`, checks `resp.Accepted`; `TestAgentConnectsAndRegisters` passes |
| 5  | Agent opens a persistent bidirectional AgentStream and receives commands | VERIFIED | `connectAndServe` opens `client.CommandStream`, runs recv loop dispatching to `sessions.HandleCommand`; sender goroutine handles outbound |
| 6  | Agent reconnects with exponential backoff (1s–60s) when connection drops | VERIFIED | `Run` loop calls `waitWithBackoff` on error; `Backoff.Next()` doubles with 20% jitter, capped at 60s; `TestAgentReconnectsOnDrop` passes |
| 7  | Agent re-registers with surviving session states after reconnect | VERIFIED | `connectAndServe` calls `sessions.GetStates()` before `Register`, passes `ExistingSessions` field |
| 8  | Backoff includes jitter to prevent thundering herd | VERIFIED | `Backoff.Next()` multiplies by `[0.8, 1.2)` random factor using `math/rand/v2`; `TestBackoffIncreasing` verifies jitter range |
| 9  | Agent can spawn a process in a PTY and read its output | VERIFIED | `NewSession` calls `pty.StartWithSize`, launches `readLoop` goroutine; `TestSessionSpawnAndRead` passes |
| 10 | Session output is continuously read and sent to a buffered channel | VERIFIED | `readLoop` reads 4096-byte chunks, non-blocking send to `outputCh` (cap 256), drops on full |
| 11 | Session manager tracks multiple sessions by ID with thread safety | VERIFIED | `SessionManager` uses `sync.RWMutex` over `map[string]*Session`; `TestSessionManagerConcurrent` passes with `-race` |
| 12 | Session manager relays output from all sessions to the gRPC send channel | VERIFIED | `StartRelay` and `startSessionRelay` forward `outputCh` data as `pb.AgentEvent_SessionOutput` to `sendCh`; `TestSessionManagerRelay` passes |
| 13 | Agent dispatches incoming server commands (create, input, resize, kill) to the correct session | VERIFIED | `HandleCommand` switches on `pb.ServerCommand` oneof types, routing each to `handleCreate`, `handleInput`, `handleResize`, `handleKill` |
| 14 | Session continues running when gRPC connection drops (output buffered locally) | VERIFIED | `StopRelay` cancels relay goroutines but does not terminate sessions; sessions continue with output buffered in their `outputCh` (cap 256) |
| 15 | Session cleanup closes PTY and signals completion when process exits | VERIFIED | `waitForExit` calls `cmd.Wait`, sets status, closes `ptyFile`, waits for `readDone`, then closes `outputCh` in correct order |

**Score:** 15/15 truths verified

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/server/grpc/server.go` | gRPC server with mTLS listener and AgentService implementation | VERIFIED | Exports `NewGRPCServer`, `Serve`, `ConnectionManager`; implements `Register` and `CommandStream` |
| `internal/server/grpc/auth.go` | mTLS interceptors for machine-id extraction and validation | VERIFIED | Exports `MachineAuthUnaryInterceptor`, `MachineAuthStreamInterceptor`, `MachineIDFromContext`; `wrappedServerStream` for stream context enrichment |
| `internal/server/grpc/connection_mgr.go` | Connected agent tracking | VERIFIED | Exports `ConnectionManager`, `NewConnectionManager`; `Add`, `Remove`, `Get`, `List` with `sync.RWMutex` |
| `internal/agent/client.go` | Agent gRPC client with reconnect loop, register, and bidi stream | VERIFIED | Exports `AgentClient`, `NewAgentClient`, `Run`; defines `SessionProvider` interface; uses `grpc.NewClient` (not deprecated `Dial`) |
| `internal/agent/backoff.go` | Exponential backoff with jitter | VERIFIED | Exports `Backoff`, `NewBackoff`; `Next()` doubles with 20% jitter via `math/rand/v2`; `Reset()` returns to min |
| `internal/agent/session.go` | PTY session lifecycle: spawn, read loop, write input, resize, wait for exit | VERIFIED | Exports `Session`, `NewSession`; uses `pty.StartWithSize`; `readDone` channel coordinates goroutine shutdown |
| `internal/agent/session_manager.go` | Thread-safe session registry implementing SessionProvider interface | VERIFIED | Exports `SessionManager`, `NewSessionManager`; compile-time interface check `var _ SessionProvider = (*SessionManager)(nil)` |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/agent/client.go` | `internal/shared/tlsutil/loader.go` | `tlsutil.AgentTLSConfig` | WIRED | Line 53: `tlsutil.AgentTLSConfig(cfg.TLS.CACert, cfg.TLS.AgentCert, cfg.TLS.AgentKey)` |
| `internal/agent/client.go` | `internal/shared/proto/claudeplane/v1` | `pb.NewAgentServiceClient` | WIRED | Line 108: `client := pb.NewAgentServiceClient(conn)` |
| `internal/server/grpc/auth.go` | `google.golang.org/grpc/peer` | `peer.FromContext` | WIRED | Line 36: `p, ok := peer.FromContext(ctx)` |
| `internal/server/grpc/server.go` | `internal/shared/tlsutil/loader.go` | `tlsutil.ServerTLSConfig` | WIRED (indirect) | `NewGRPCServer` accepts `*tls.Config` parameter and applies it via `credentials.NewTLS(tlsCfg)`; callers supply config built from `tlsutil.ServerTLSConfig` (verified in `auth_test.go` line 53) |
| `internal/agent/session.go` | `github.com/creack/pty` | `pty.StartWithSize` | WIRED | Line 45: `ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{...})` |
| `internal/agent/session_manager.go` | `internal/agent/client.go` | Implements `SessionProvider` | WIRED | Compile-time check line 266: `var _ SessionProvider = (*SessionManager)(nil)` |
| `internal/agent/session_manager.go` | `internal/shared/proto/claudeplane/v1` | `pb.ServerCommand` dispatch | WIRED | `HandleCommand` switches on `*pb.ServerCommand_CreateSession`, `_InputData`, `_ResizeTerminal`, `_KillSession` |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|---------|
| AGNT-02 | 02-01-PLAN | Agent authenticates to server using mTLS with its issued certificate | SATISFIED | `extractMachineID` rejects invalid/missing certs; `TestAgentMTLSRejection` proves cross-CA rejection; `TestExtractMachineID_InvalidCN` proves prefix validation |
| AGNT-03 | 02-01-PLAN, 02-02-PLAN | Agent registers with server and maintains persistent gRPC bidirectional stream | SATISFIED | `connectAndServe` registers then opens `CommandStream` bidi stream; PTY sessions relay output through `StartRelay`; reconnect loop with backoff restores stream on drop |

No orphaned requirements — only AGNT-02 and AGNT-03 are mapped to Phase 2 in REQUIREMENTS.md traceability table.

---

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/server/grpc/server.go` | 110 | Comment: "Command dispatch will be wired in Plan 02" | INFO | Misleading comment — server-side command dispatch to sessions is scoped to Phase 3 (server core), not Plan 02 of this phase. The comment is stale. No functional impact; stream holds open and logs events as designed for this phase. |
| `internal/agent/session_manager.go` | 74–76 | `AttachSession`/`DetachSession` are log-and-skip | INFO | Explicitly deferred to Phase 4 per PLAN spec. Not a blocker for phase goal. |

No blockers. No stubs in critical paths.

---

## Human Verification Required

None. All automated checks are sufficient for this phase's goals. The phase delivers internal infrastructure (gRPC connection lifecycle and PTY management) with no user-facing behavior to visually verify.

---

## Test Results

All tests passed with race detector enabled:

- `go test ./internal/server/grpc/... -v -count=1`: 4/4 tests PASS (0.022s)
  - `TestExtractMachineID_ValidCert` PASS
  - `TestExtractMachineID_NoPeerInfo` PASS
  - `TestExtractMachineID_InvalidCN` PASS
  - `TestConnectionManager_AddRemoveList` PASS
- `go test ./internal/agent/... -v -count=1 -race -timeout=60s`: 14/14 tests PASS (4.482s)
  - Backoff: `TestBackoffIncreasing`, `TestBackoffCapsAtMax`, `TestBackoffReset` PASS
  - Session: `TestSessionSpawnAndRead`, `TestSessionWriteInput`, `TestSessionExitStatus`, `TestSessionResize` PASS
  - SessionManager: `TestSessionManagerCreate`, `TestSessionManagerInput`, `TestSessionManagerRelay`, `TestSessionManagerConcurrent`, `TestSessionManagerGetStates` PASS
  - AgentClient integration: `TestAgentConnectsAndRegisters`, `TestAgentMTLSRejection`, `TestAgentReconnectsOnDrop` PASS

---

## Summary

Phase 2 goal is fully achieved. The agent binary infrastructure is complete:

- mTLS mutual authentication works end-to-end: agents with valid CA-issued certs are accepted; agents with mismatched CA certs are rejected at the TLS handshake
- Machine-id extraction from certificate CN is implemented in both unary and streaming interceptors
- The agent's reconnect state machine (`Run` -> `connectAndServe` -> `waitWithBackoff`) handles connection drops with exponential backoff (1s–60s, 20% jitter)
- Re-registration on reconnect carries surviving session states from `SessionManager.GetStates()`
- PTY session lifecycle is fully implemented with the `readDone` channel pattern preventing the close-on-wrong-goroutine race
- `SessionManager` implements the `SessionProvider` interface, completing the wiring between `AgentClient` and PTY sessions
- The sole stale comment in `server.go` (line 110) is an INFO-level observation with no functional impact

---

_Verified: 2026-03-12T10:05:00Z_
_Verifier: Claude (gsd-verifier)_
