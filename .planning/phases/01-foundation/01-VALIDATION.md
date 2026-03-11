---
phase: 1
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-11
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — Wave 0 installs |
| **Quick run command** | `go test ./...` |
| **Full suite command** | `go test -v -count=1 ./...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./...`
- **After every plan wave:** Run `go test -v -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 5 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | INFR-01 | build | `go build ./cmd/server && go build ./cmd/agent` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | INFR-02 | build | `go build ./cmd/agent` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | AGNT-01 | unit | `go test ./internal/ca/...` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | INFR-04 | unit | `go test ./internal/config/...` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 1 | INFR-03 | unit | `go test ./internal/db/...` | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 1 | AUTH-04 | unit | `go test ./internal/auth/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` — initialized with module path and Go 1.23+
- [ ] `go.sum` — dependency lockfile
- [ ] Go test infrastructure works with `go test ./...`

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Server binary runs standalone | INFR-01 | Integration | Build binary, run `./claude-plane-server --version` |
| Agent binary runs standalone | INFR-02 | Integration | Build binary, run `./claude-plane-agent --version` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
