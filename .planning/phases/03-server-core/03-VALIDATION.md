---
phase: 3
slug: server-core
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — Go uses `*_test.go` files |
| **Quick run command** | `go test ./internal/server/api/... ./internal/server/auth/... ./internal/server/connmgr/... -count=1 -short` |
| **Full suite command** | `go test ./... -count=1 -race` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/server/... -count=1 -short`
- **After every plan wave:** Run `go test ./... -count=1 -race`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-01 | 01 | 1 | AUTH-01 | unit | `go test ./internal/server/api/... -run TestRegister -count=1` | ❌ W0 | ⬜ pending |
| 03-01-02 | 01 | 1 | AUTH-02 | unit | `go test ./internal/server/auth/... -run TestIssueToken -count=1` | ❌ W0 | ⬜ pending |
| 03-01-03 | 01 | 1 | AUTH-03 | integration | `go test ./internal/server/api/... -run TestLogout -count=1` | ❌ W0 | ⬜ pending |
| 03-02-01 | 02 | 2 | AGNT-04 | unit | `go test ./internal/server/connmgr/... -run TestConnectionManager -count=1` | ❌ W0 | ⬜ pending |
| 03-02-02 | 02 | 2 | AGNT-04 | integration | `go test ./internal/server/api/... -run TestListMachines -count=1` | ❌ W0 | ⬜ pending |
| 03-02-03 | 02 | 2 | (SC-5) | unit | `go test ./internal/server/api/... -run TestAuthMiddleware -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/server/api/auth_handler_test.go` — register, login, logout handler tests
- [ ] `internal/server/api/machine_handler_test.go` — machine list endpoint tests
- [ ] `internal/server/api/router_test.go` — integration tests for protected/unprotected routes
- [ ] `internal/server/auth/jwt_test.go` — token issuance, validation, algorithm enforcement
- [ ] `internal/server/auth/blocklist_test.go` — revocation, persistence, cleanup
- [ ] `internal/server/connmgr/manager_test.go` — register, disconnect, list, concurrent access
- [ ] Test helpers: in-memory SQLite store, test JWT signing key

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full auth flow end-to-end | AUTH-01/02/03 | Integration | Register via curl, login to get JWT, use JWT on protected endpoint, logout, verify rejected |
| Agent appears in machine list after connecting | AGNT-04 | End-to-end | Start server, connect agent with mTLS, call `GET /api/v1/machines`, verify agent listed as online |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
