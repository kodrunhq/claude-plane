---
phase: 2
slug: agent-core
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — Go uses `*_test.go` files |
| **Quick run command** | `go test ./internal/agent/... ./internal/server/grpc/... -count=1 -short` |
| **Full suite command** | `go test ./... -count=1 -race` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/agent/... ./internal/server/grpc/... -count=1 -short`
- **After every plan wave:** Run `go test ./... -count=1 -race`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-01 | 01 | 1 | AGNT-02 | unit | `go test ./internal/server/grpc/... -run TestMachineAuth -count=1` | ❌ W0 | ⬜ pending |
| 02-01-02 | 01 | 1 | AGNT-02 | integration | `go test ./internal/agent/... -run TestMTLSAuth -count=1` | ❌ W0 | ⬜ pending |
| 02-02-01 | 02 | 1 | AGNT-03 | integration | `go test ./internal/agent/... -run TestRegister -count=1` | ❌ W0 | ⬜ pending |
| 02-02-02 | 02 | 1 | AGNT-03 | unit | `go test ./internal/agent/... -run TestReconnectBackoff -count=1` | ❌ W0 | ⬜ pending |
| 02-02-03 | 02 | 1 | AGNT-03 | integration | `go test ./internal/agent/... -run TestStreamKeepalive -count=1` | ❌ W0 | ⬜ pending |
| 02-02-04 | 02 | 1 | AGNT-03 | unit | `go test ./internal/agent/... -run TestPTYSession -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/agent/client_test.go` — mTLS dial, register, stream, reconnect tests
- [ ] `internal/agent/session_test.go` — PTY spawn and output relay tests
- [ ] `internal/agent/backoff_test.go` — exponential backoff with jitter tests
- [ ] `internal/server/grpc/auth_test.go` — machine-id extraction and allowlist validation
- [ ] `internal/server/grpc/server_test.go` — gRPC server setup with mTLS
- [ ] Test helpers: ephemeral CA + certs for integration tests (reuse `tlsutil` from Phase 1)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Agent binary connects to running server | AGNT-02 | End-to-end integration | Start server with certs, run agent with matching certs, verify connection in server logs |
| Stream survives idle period | AGNT-03 | Timing-dependent | Connect agent, wait 2+ minutes with no activity, verify stream is still alive |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
