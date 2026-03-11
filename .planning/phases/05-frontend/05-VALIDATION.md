---
phase: 5
slug: frontend
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-12
---

# Phase 5 — Validation Strategy

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
| 05-01-01 | 01 | 1 | (scaffold) | build | `cd web && npx tsc --noEmit && npm run build` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | SESS-04 | unit | `cd web && npx vitest run src/__tests__/views/CommandCenter.test.tsx` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | SESS-04 | unit | `cd web && npx vitest run src/__tests__/views/SessionsPage.test.tsx` | ❌ W0 | ⬜ pending |
| 05-02-02 | 02 | 2 | SC-2 | unit | `cd web && npx vitest run src/__tests__/views/MachinesPage.test.tsx` | ❌ W0 | ⬜ pending |
| 05-02-03 | 02 | 2 | SC-4 | smoke | `cd web && npm run build && test -f dist/index.html` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `web/src/__tests__/views/SessionsPage.test.tsx` — session listing tests
- [ ] `web/src/__tests__/views/CommandCenter.test.tsx` — dashboard panel tests
- [ ] `web/src/__tests__/views/MachinesPage.test.tsx` — machine status tests
- [ ] `web/src/__tests__/components/sessions/NewSessionModal.test.tsx` — lifecycle actions
- [ ] `web/src/__tests__/setup.ts` — test setup with React Testing Library
- [ ] Dev dependencies: `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Dashboard renders with real data | SESS-04 | Visual / E2E | Start server+agent, open browser, verify sessions and machines display correctly |
| Session lifecycle from UI | SC-3 | Visual / E2E | Create session from machine list, verify terminal opens, detach, reattach, terminate |
| go:embed serves SPA correctly | SC-4 | Integration | Build server binary, run it, open browser at root URL, verify SPA loads |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
