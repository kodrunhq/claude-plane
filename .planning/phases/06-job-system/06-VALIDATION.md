---
phase: 6
slug: job-system
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-12
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go: `testing` + Vitest (frontend) |
| **Config file** | Go: none; Frontend: `web/vitest.config.ts` |
| **Quick run command** | `go test ./internal/server/orchestrator/... ./internal/server/handler/... ./internal/server/store/... -count=1 -short` |
| **Full suite command** | `go test ./... -count=1 && cd web && npx vitest run` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/server/orchestrator/... ./internal/server/handler/... ./internal/server/store/... -count=1 -short`
- **After every plan wave:** Run `go test ./... -count=1 && cd web && npx vitest run`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | JOBS-01, JOBS-04 | unit | `go test ./internal/server/store/... -run TestJobStore -count=1` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | JOBS-01, JOBS-04 | unit | `go test ./internal/server/orchestrator/... -run TestCycleDetection -count=1` | ❌ W0 | ⬜ pending |
| 06-01-03 | 01 | 1 | JOBS-02, JOBS-04 | unit | `go test ./internal/server/orchestrator/... -run TestDAGRunner -count=1` | ❌ W0 | ⬜ pending |
| 06-02-01 | 02 | 2 | JOBS-01, JOBS-02, JOBS-03 | integration | `go test ./internal/server/handler/... -run TestJobHandler -count=1` | ❌ W0 | ⬜ pending |
| 06-02-02 | 02 | 2 | JOBS-03 | unit | `go test ./internal/server/orchestrator/... -run TestStepRetry -count=1` | ❌ W0 | ⬜ pending |
| 06-03-01 | 03 | 3 | JOBS-01 | unit | `cd web && npx vitest run --reporter=verbose src/components/dag/` | ❌ W0 | ⬜ pending |
| 06-03-02 | 03 | 3 | JOBS-02 | unit | `cd web && npx vitest run --reporter=verbose src/pages/RunDetail` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/server/store/jobs_test.go` — job/step/dependency CRUD tests
- [ ] `internal/server/orchestrator/dag_runner_test.go` — DAG execution, dependency ordering, parallel steps
- [ ] `internal/server/orchestrator/orchestrator_test.go` — job creation, run creation with snapshots, step retry
- [ ] `internal/server/handler/jobs_test.go` — REST handler tests for jobs/steps/runs endpoints
- [ ] `web/src/components/dag/__tests__/DAGCanvas.test.tsx` — ReactFlow rendering stub
- [ ] `web/src/pages/__tests__/RunDetail.test.tsx` — Run detail view stub

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end job run in browser | JOBS-02 | Full stack integration | Create job with 3 steps (A→B→C), trigger run, verify steps execute in order with real-time terminal output |
| DAG canvas drag-and-drop | JOBS-01 | Visual / interaction | Open job editor, add steps, drag to connect, verify edges render correctly |
| Step retry from run detail | JOBS-03 | Full stack integration | Run job, fail a step manually, click retry, verify new session created and downstream steps re-execute |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
