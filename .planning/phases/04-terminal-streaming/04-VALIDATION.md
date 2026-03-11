---
phase: 4
slug: terminal-streaming
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-12
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go: `testing` + Vitest (frontend) |
| **Config file** | Go: none; Frontend: `web/vitest.config.ts` |
| **Quick run command** | `go test ./internal/agent/session/... ./internal/server/session/... -count=1 -short` |
| **Full suite command** | `go test ./... -count=1 && cd web && npm test` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/agent/session/... ./internal/server/session/... -count=1 -short`
- **After every plan wave:** Run `go test ./... -count=1 && cd web && npm test`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | SESS-01, SESS-05 | unit | `go test ./internal/agent/session/... -run TestCreateSession -count=1` | ❌ W0 | ⬜ pending |
| 04-01-02 | 01 | 1 | SESS-03, SESS-06 | unit | `go test ./internal/agent/session/... -run TestDetachSession -count=1` | ❌ W0 | ⬜ pending |
| 04-01-03 | 01 | 1 | TERM-03 | unit | `go test ./internal/agent/session/... -run TestResize -count=1` | ❌ W0 | ⬜ pending |
| 04-02-01 | 02 | 2 | SESS-02, TERM-01 | integration | `go test ./internal/server/session/... -run TestAttachSession -count=1` | ❌ W0 | ⬜ pending |
| 04-02-02 | 02 | 2 | TERM-02 | integration | `go test ./internal/server/session/... -run TestInputRelay -count=1` | ❌ W0 | ⬜ pending |
| 04-02-03 | 02 | 2 | TERM-04 | unit | `go test ./internal/server/session/... -run TestFlowControl -count=1` | ❌ W0 | ⬜ pending |
| 04-03-01 | 03 | 3 | TERM-01, TERM-02 | unit | `cd web && npx vitest run --reporter=verbose src/components/terminal/` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/agent/session/manager_test.go` — covers SESS-01, SESS-03, SESS-05
- [ ] `internal/agent/session/pty_test.go` — covers TERM-03
- [ ] `internal/agent/session/scrollback_test.go` — covers scrollback writing
- [ ] `internal/server/session/registry_test.go` — covers SESS-02, SESS-06
- [ ] `internal/server/session/ws_test.go` — covers TERM-01, TERM-02, TERM-04
- [ ] `web/src/components/terminal/__tests__/useTerminalSession.test.ts` — covers frontend hook

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end terminal session in browser | TERM-01, TERM-02 | Full stack integration | Start server+agent, open browser, create session, type commands, verify output renders in xterm.js |
| Session survives browser disconnect | SESS-06 | Browser behavior | Create session, close browser tab, reopen, reattach — verify session still running |
| Resize propagates visually | TERM-03 | Visual verification | Resize browser window, run `tput cols; tput lines` in remote terminal, verify values match |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
