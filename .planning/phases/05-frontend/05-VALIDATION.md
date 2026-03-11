---
phase: 5
slug: frontend
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-03-12
---

# Phase 5 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Vitest (paired with Vite) |
| **Config file** | `web/vite.config.ts` |
| **Quick run command** | `cd web && npx vitest run --reporter=verbose` |
| **Full suite command** | `cd web && npx vitest run && npm run build` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd web && npx vitest run --reporter=verbose`
- **After every plan wave:** Run `cd web && npx vitest run && npm run build`
- **Before `/gsd:verify-work`:** Full suite must be green + production build succeeds
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-00-01 | 00 | 0 | (infra) | config | `cd web && npx vitest run --reporter=verbose` | N/A | pending |
| 05-00-02 | 00 | 0 | (infra) | stub | `cd web && npx vitest run --reporter=verbose` | Created by task | pending |
| 05-01-01 | 01 | 1 | (scaffold) | build | `cd web && npx tsc --noEmit && npx vitest run --reporter=verbose` | N/A | pending |
| 05-01-02 | 01 | 1 | (scaffold) | build | `cd web && npx tsc --noEmit && npx vite build && npx vitest run --reporter=verbose` | N/A | pending |
| 05-02-01 | 02 | 1 | SESS-04 | build+unit | `cd web && npx tsc --noEmit && npx vitest run --reporter=verbose` | N/A | pending |
| 05-02-02 | 02 | 1 | SESS-04 | build+unit | `cd web && npx tsc --noEmit && npx vite build && npx vitest run --reporter=verbose` | N/A | pending |
| 05-03-01 | 03 | 2 | SESS-04 | unit | `cd web && npx tsc --noEmit && npx vitest run --reporter=verbose` | W0 stubs | pending |
| 05-03-02 | 03 | 2 | SESS-04 | unit+build | `cd web && npx tsc --noEmit && npx vite build && npx vitest run --reporter=verbose` | W0 stubs | pending |
| 05-04-01 | 04 | 3 | SESS-04 | build+go | `cd web && npx vite build && go vet ./internal/server/frontend/...` | N/A | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements (Plan 05-00)

- [x] Plan 05-00 created to install test deps and configure Vitest
- [ ] `web/src/__tests__/setup.ts` -- test setup with React Testing Library
- [ ] `web/src/__tests__/views/CommandCenter.test.tsx` -- stub test
- [ ] `web/src/__tests__/views/SessionsPage.test.tsx` -- stub test
- [ ] `web/src/__tests__/views/MachinesPage.test.tsx` -- stub test
- [ ] `web/src/__tests__/components/sessions/NewSessionModal.test.tsx` -- stub test
- [ ] Dev dependencies: `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Dashboard renders with real data | SESS-04 | Visual / E2E | Start server+agent, open browser, verify sessions and machines display correctly |
| Session lifecycle from UI | SC-3 | Visual / E2E | Create session from machine list, verify terminal opens, detach, reattach, terminate |
| go:embed serves SPA correctly | SC-4 | Integration | Build server binary, run it, open browser at root URL, verify SPA loads |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify with vitest or go vet commands
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references (Plan 05-00)
- [x] No watch-mode flags
- [x] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
