# Phase 6: Job System - Research

**Researched:** 2026-03-12
**Domain:** Job orchestration (Go backend DAG runner + React frontend DAG builder)
**Confidence:** HIGH

## Summary

Phase 6 implements the job system -- the ability to create multi-step jobs, execute them with dependency ordering, view real-time output, and rerun individual steps. The design documents (`backend_v1.md`, `frontend_v1.md`, `suplementary_v1.md`) define this system extensively: REST API endpoints, SQLite schema, DAG runner pseudocode, ReactFlow-based DAG canvas, and edge cases. The implementation follows well-established patterns (Kahn's algorithm for topological sort, snapshot-on-run for immutability, ReactFlow for visual DAG editing).

The backend work centers on a `DAGRunner` that resolves step dependencies using in-degree tracking (Kahn's algorithm), executes ready steps by creating sessions on target agents, and monitors completion to unlock downstream steps. The frontend work centers on a job editor with a ReactFlow DAG canvas (left panel) and step editor form (right panel), plus a run detail view that shows live DAG status with embedded terminals.

**Primary recommendation:** Implement backend job CRUD + DAG runner first, then frontend job editor + run detail view. Use `@xyflow/react` v12 with `@dagrejs/dagre` for automatic layout. Keep V1 scope to the four JOBS requirements -- no cron, no cross-job triggers, no approval gates, no failure policies beyond `fail_run`.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| JOBS-01 | User can create a job with multiple ordered steps | REST API (`POST /api/v1/jobs`, `POST /api/v1/jobs/:id/steps`, dependency edges), SQLite schema (`jobs`, `steps`, `step_dependencies`), DAG cycle validation (Kahn's algorithm), ReactFlow DAG canvas + step editor |
| JOBS-02 | User can execute individual job steps and view their output | `POST /api/v1/jobs/:id/runs` triggers DAGRunner which creates sessions via gRPC, WebSocket streams terminal output to run detail view with embedded xterm.js |
| JOBS-03 | User can rerun a previously executed step | `POST /api/v1/runs/:runId/steps/:stepId/retry` resets run_step to pending, re-evaluates DAG, creates new session with snapshot config |
| JOBS-04 | Steps support dependency ordering (step B waits for step A) | `step_dependencies` table (edges in DAG), DAGRunner tracks in-degree, executes steps when all dependencies are `completed`, cycle detection on save |
</phase_requirements>

## Standard Stack

### Core (Backend)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go `sync` | stdlib | Mutex/WaitGroup for DAGRunner concurrency | No external deps needed for this pattern |
| `github.com/google/uuid` | latest | UUID generation for job/step/run IDs | Already in project stack |
| `modernc.org/sqlite` | (existing) | Persist jobs, steps, dependencies, runs, run_steps | Already chosen in project |

### Core (Frontend)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@xyflow/react` | 12.x | DAG canvas for job editor and run detail view | Project design doc specifies ReactFlow; v12 is current (12.10.1) |
| `@dagrejs/dagre` | 1.x | Automatic layout of DAG nodes | Standard pairing with ReactFlow for tree/DAG layouts |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `@xyflow/react` built-in types | 12.x | TypeScript generics for custom node/edge types | Always -- type safety for StepNode, StepEdge |
| Zustand `jobs` + `runs` stores | (existing) | Client state for job definitions and run state | Design doc already specifies these stores |
| TanStack Query | (existing) | Server state for job/step CRUD | Design doc already specifies |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `@xyflow/react` | D3.js directly | Design doc explicitly rejects: "Too low-level. ReactFlow gives us drag-and-drop, zoom, pan, edge routing for free." |
| `@dagrejs/dagre` | elkjs | elkjs is more powerful but heavier; dagre is sufficient for simple step DAGs |
| Custom DAG runner | External workflow engine | Overkill -- the DAG is simple (< 20 nodes typically), Kahn's algorithm is ~50 lines of Go |

**Installation (frontend additions):**
```bash
npm install @xyflow/react @dagrejs/dagre
npm install -D @types/dagre  # if needed
```

## Architecture Patterns

### Backend: Job System Structure

```
internal/server/
├── handler/
│   ├── jobs.go              # REST handlers for job CRUD
│   ├── steps.go             # REST handlers for step CRUD + dependency edges
│   └── runs.go              # REST handlers for run creation, step retry
├── orchestrator/
│   ├── orchestrator.go      # Job orchestrator (manages active DAGRunners)
│   ├── dag_runner.go        # DAG execution engine (per-run)
│   └── scheduler.go         # Machine selection for steps (V1: explicit only)
├── store/
│   ├── jobs.go              # DB queries for jobs, steps, step_dependencies
│   └── runs.go              # DB queries for runs, run_steps
```

### Frontend: Job System Structure

```
web/src/
├── components/
│   ├── dag/
│   │   ├── DAGCanvas.tsx        # ReactFlow wrapper for job step graph
│   │   ├── StepNode.tsx         # Custom ReactFlow node for a step
│   │   └── StepEdge.tsx         # Custom edge with status coloring
│   ├── jobs/
│   │   ├── StepEditor.tsx       # Form for editing step config
│   │   └── JobMetaForm.tsx      # Job name, description
│   └── runs/
│       └── RunDAGView.tsx       # Read-only DAG with live status + terminals
├── pages/
│   ├── JobsPage.tsx             # Job list
│   ├── JobEditor.tsx            # DAG canvas + step editor (split view)
│   └── RunDetail.tsx            # DAG status + embedded terminals per step
├── stores/
│   ├── jobs.ts                  # Zustand store for job definitions
│   └── runs.ts                  # Zustand store for run state
├── api/
│   └── jobs.ts                  # REST API client for jobs/steps/runs
```

### Pattern 1: DAG Runner (Kahn's Algorithm)

**What:** Server-side engine that executes a job run by tracking step dependencies and launching ready steps as sessions.
**When to use:** Every time a job run is created.
**Key design decisions from design docs:**
- One `DAGRunner` instance per active run
- Uses in-degree map to find root steps (no dependencies)
- Executes ready steps in parallel (each step spawns a session on target machine)
- On step completion, decrements in-degree of dependents; if zero, step becomes runnable
- Thread-safe via mutex (steps complete asynchronously via session exit events)

```go
// Source: backend_v1.md Section 7.2
type DAGRunner struct {
    mu         sync.Mutex
    runID      string
    steps      map[string]*RunStep       // step_id -> run step state
    deps       map[string][]string       // step_id -> step_ids it depends on
    dependents map[string][]string       // step_id -> step_ids that depend on it
    executor   StepExecutor
}

func (d *DAGRunner) Start(ctx context.Context) {
    // Find root steps (no dependencies) and execute them
    for stepID, depList := range d.deps {
        if len(depList) == 0 {
            go d.executeStep(ctx, stepID)
        }
    }
}

func (d *DAGRunner) OnStepCompleted(stepID string, exitCode int) {
    d.mu.Lock()
    defer d.mu.Unlock()
    // Mark step status, then check if dependents are now unblocked
    for _, depID := range d.dependents[stepID] {
        if d.allDependenciesMet(depID) {
            go d.executeStep(context.Background(), depID)
        }
    }
}
```

### Pattern 2: Snapshot-on-Run

**What:** When a run is created, copy step definitions into `run_steps` so edits to the job don't affect in-progress runs.
**When to use:** Every `POST /api/v1/jobs/:id/runs`.
**Source:** suplementary_v1.md Section 8.1

```go
func (o *Orchestrator) createRunSteps(runID string, jobID string) {
    steps := o.db.GetSteps(jobID)
    for _, step := range steps {
        o.db.InsertRunStep(RunStep{
            RunStepID:          uuid.New().String(),
            RunID:              runID,
            StepID:             step.StepID,
            Status:             "pending",
            PromptSnapshot:     step.Prompt,
            MachineIDSnapshot:  step.MachineID,
            WorkingDirSnapshot: step.WorkingDir,
        })
    }
}
```

### Pattern 3: DAG Cycle Detection (Kahn's Algorithm)

**What:** Validate no cycles exist when saving step dependencies.
**When to use:** On every `POST/PUT` that modifies step dependencies.
**Source:** backend_v1.md Section 7.6

```go
func validateDAG(steps []Step, deps []StepDependency) error {
    inDegree := make(map[string]int)
    adj := make(map[string][]string)
    for _, s := range steps {
        inDegree[s.StepID] = 0
    }
    for _, d := range deps {
        adj[d.DependsOn] = append(adj[d.DependsOn], d.StepID)
        inDegree[d.StepID]++
    }
    queue := []string{}
    for id, deg := range inDegree {
        if deg == 0 {
            queue = append(queue, id)
        }
    }
    visited := 0
    for len(queue) > 0 {
        node := queue[0]
        queue = queue[1:]
        visited++
        for _, next := range adj[node] {
            inDegree[next]--
            if inDegree[next] == 0 {
                queue = append(queue, next)
            }
        }
    }
    if visited != len(steps) {
        return fmt.Errorf("cycle detected in job DAG")
    }
    return nil
}
```

### Pattern 4: ReactFlow DAG Canvas

**What:** Interactive visual DAG editor using `@xyflow/react` v12 with custom StepNode components.
**When to use:** Job editor (editable) and run detail (read-only with live status).

```typescript
// Source: reactflow.dev + frontend_v1.md
import { ReactFlow, Node, Edge, useNodesState, useEdgesState } from '@xyflow/react';
import dagre from '@dagrejs/dagre';

// Custom node type for steps
type StepNodeData = {
  label: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  machineId?: string;
};

// Auto-layout using dagre
function getLayoutedElements(nodes: Node[], edges: Edge[]) {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'LR', nodesep: 50, ranksep: 100 });

  nodes.forEach((node) => {
    g.setNode(node.id, { width: 180, height: 60 });
  });
  edges.forEach((edge) => {
    g.setEdge(edge.source, edge.target);
  });

  dagre.layout(g);

  const layoutedNodes = nodes.map((node) => {
    const pos = g.node(node.id);
    return { ...node, position: { x: pos.x - 90, y: pos.y - 30 } };
  });
  return { nodes: layoutedNodes, edges };
}
```

### Anti-Patterns to Avoid

- **Polling for step status:** Use WebSocket events (`run.step.status`) pushed from server, not client polling. The design doc specifies a multiplexed WebSocket.
- **Storing run state only in memory:** The DAGRunner must persist state to `run_steps` table so runs survive server restart. The in-memory DAGRunner is rebuilt from DB on recovery.
- **Executing steps without snapshots:** Never read from `steps` table during execution -- always use `run_steps` snapshot fields. Otherwise edits during a run cause inconsistency.
- **Blocking the DAG runner on step execution:** Steps run as goroutines. The DAGRunner's role is coordination, not execution. It receives completion events asynchronously.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| DAG visualization | Custom SVG/Canvas rendering | `@xyflow/react` | Drag-and-drop, zoom, pan, edge routing, node selection all built-in |
| Auto-layout positioning | Manual x/y calculation | `@dagrejs/dagre` | Handles node sizing, rank assignment, edge routing automatically |
| Cycle detection | Custom graph traversal | Kahn's algorithm (standard impl) | Well-understood, O(V+E), ~30 lines of Go |
| UUID generation | Custom ID schemes | `github.com/google/uuid` | Standard, no collisions |
| Step dependency resolution | Ad-hoc if/else chains | In-degree tracking (Kahn's) | Handles arbitrary DAG shapes, parallel branches, diamond patterns |

**Key insight:** The DAG runner itself is simple enough to implement (~200 lines of Go) because the design doc constrains it: no conditional branching, no loops, no dynamic step creation. It is a static DAG with fixed steps resolved at run creation time.

## Common Pitfalls

### Pitfall 1: Race Conditions in DAGRunner

**What goes wrong:** Multiple steps complete simultaneously, concurrent `OnStepCompleted` calls corrupt shared state or launch a step twice.
**Why it happens:** Steps execute as goroutines; session exit events arrive asynchronously.
**How to avoid:** Mutex around all DAGRunner state mutations. The `OnStepCompleted` method must be serialized.
**Warning signs:** Duplicate session creation for the same step, panics on map access.

### Pitfall 2: Server Restart Loses Active Runs

**What goes wrong:** DAGRunners are in-memory. Server crash kills all active run coordination.
**Why it happens:** Run state only in Go structs, not persisted.
**How to avoid:** Every state transition writes to `run_steps` table first. On startup, scan for `run_steps` with status `running` or `pending` in active runs, rebuild DAGRunners from DB state.
**Warning signs:** Runs stuck in `running` state after restart with no goroutines processing them.

### Pitfall 3: ReactFlow Node Dimensions for Dagre

**What goes wrong:** Dagre needs to know node width/height before layout. Custom nodes with dynamic content produce wrong layout.
**Why it happens:** Dagre calculates positions before React renders; if dimensions are wrong, nodes overlap.
**How to avoid:** Use fixed dimensions for StepNode (e.g., 180x60). If dynamic sizing is needed, measure after render and re-layout.
**Warning signs:** Nodes overlapping or edges routing through nodes.

### Pitfall 4: Editing a Job During Active Run

**What goes wrong:** User edits step config, active run starts using new config mid-execution.
**Why it happens:** Run reads from `steps` table instead of `run_steps` snapshots.
**How to avoid:** Snapshot-on-run pattern (copy step config to `run_steps` at creation). DAGRunner only reads from `run_steps`.
**Warning signs:** Step behavior changes without re-running.

### Pitfall 5: Step Retry Cascading Issues

**What goes wrong:** Retrying a step does not reset downstream `skipped` steps, leaving the run in a broken state.
**Why it happens:** Retry only resets the target step, forgets to cascade status reset.
**How to avoid:** When retrying a step, also reset all downstream steps that were `skipped` or `cancelled` back to `pending`.
**Warning signs:** Retried step completes but downstream steps remain `skipped`.

### Pitfall 6: WebSocket Event Routing for Run Steps

**What goes wrong:** Run step status updates don't reach the right browser tab.
**Why it happens:** WebSocket messages need `run_id` and `step_id` tags; if missing, frontend can't route updates.
**How to avoid:** Include `run_id`, `step_id`, and `session_id` in all run-related WebSocket events.
**Warning signs:** Run detail view shows stale step status.

## Code Examples

### SQLite Schema (from backend_v1.md)

```sql
-- Jobs (the definition -- reusable)
CREATE TABLE jobs (
    job_id         TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    description    TEXT,
    created_by     TEXT NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Steps (belong to a job)
CREATE TABLE steps (
    step_id        TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    prompt         TEXT,
    machine_id     TEXT,
    working_dir    TEXT,
    command        TEXT DEFAULT 'claude',
    args           TEXT,
    timeout_seconds INTEGER DEFAULT 0,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    on_failure     TEXT NOT NULL DEFAULT 'fail_run'
);
CREATE INDEX idx_steps_job ON steps(job_id, sort_order);

-- Step dependencies (edges in the DAG)
CREATE TABLE step_dependencies (
    step_id        TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    depends_on     TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    PRIMARY KEY (step_id, depends_on),
    CHECK (step_id != depends_on)
);

-- Runs (a specific execution of a job)
CREATE TABLE runs (
    run_id         TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    trigger_type   TEXT NOT NULL,
    started_at     DATETIME,
    completed_at   DATETIME,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_runs_job ON runs(job_id, created_at DESC);

-- Run steps (instance of a step within a specific run)
CREATE TABLE run_steps (
    run_step_id    TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL REFERENCES steps(step_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    session_id     TEXT,
    machine_id     TEXT,
    exit_code      INTEGER,
    started_at     DATETIME,
    completed_at   DATETIME,
    -- Snapshot fields (immutable copy from step at run creation)
    prompt_snapshot    TEXT,
    machine_id_snapshot TEXT,
    working_dir_snapshot TEXT,
    command_snapshot    TEXT,
    args_snapshot       TEXT
);
CREATE INDEX idx_run_steps_run ON run_steps(run_id);
CREATE INDEX idx_run_steps_status ON run_steps(status);
```

### REST API Endpoints (V1 scope)

```
# Jobs CRUD
GET    /api/v1/jobs                              # List all jobs
GET    /api/v1/jobs/:id                           # Get job with steps + dependencies
POST   /api/v1/jobs                               # Create a new job
PUT    /api/v1/jobs/:id                           # Update job (validates DAG)
DELETE /api/v1/jobs/:id                           # Delete job (cascade)

# Steps (nested under jobs)
POST   /api/v1/jobs/:id/steps                     # Add a step
PUT    /api/v1/jobs/:jid/steps/:sid               # Update a step
DELETE /api/v1/jobs/:jid/steps/:sid               # Remove a step
POST   /api/v1/jobs/:jid/steps/:sid/deps          # Add dependency edge
DELETE /api/v1/jobs/:jid/steps/:sid/deps/:dep_id  # Remove dependency edge

# Runs
POST   /api/v1/jobs/:id/runs                      # Trigger a run
GET    /api/v1/runs                                # List runs
GET    /api/v1/runs/:id                            # Get run with run_steps
POST   /api/v1/runs/:id/cancel                     # Cancel a run
POST   /api/v1/runs/:rid/steps/:sid/retry          # Retry a failed step
```

### Frontend API Client (from frontend_v1.md)

```typescript
// api/jobs.ts
export const jobsApi = {
  list: () => request<Job[]>('/jobs'),
  get: (id: string) => request<JobDetail>(`/jobs/${id}`),
  create: (params: CreateJobParams) =>
    request<Job>('/jobs', { method: 'POST', body: JSON.stringify(params) }),
  update: (id: string, params: UpdateJobParams) =>
    request<Job>(`/jobs/${id}`, { method: 'PUT', body: JSON.stringify(params) }),
  delete: (id: string) =>
    request<void>(`/jobs/${id}`, { method: 'DELETE' }),
  triggerRun: (id: string) =>
    request<Run>(`/jobs/${id}/runs`, { method: 'POST', body: JSON.stringify({ trigger_type: 'manual' }) }),
  addStep: (jobId: string, params: CreateStepParams) =>
    request<Step>(`/jobs/${jobId}/steps`, { method: 'POST', body: JSON.stringify(params) }),
  updateStep: (jobId: string, stepId: string, params: UpdateStepParams) =>
    request<Step>(`/jobs/${jobId}/steps/${stepId}`, { method: 'PUT', body: JSON.stringify(params) }),
  deleteStep: (jobId: string, stepId: string) =>
    request<void>(`/jobs/${jobId}/steps/${stepId}`, { method: 'DELETE' }),
};
```

### WebSocket Events for Runs

```typescript
// Run-related WebSocket messages (from frontend_v1.md)
// Server pushes these through the multiplexed WebSocket

// Step status change
{ type: 'run.step.status', runId: string, stepId: string, status: string }

// Step output (terminal data from the step's session)
// Reuses existing session terminal data flow -- step's session_id routes output
{ type: 'terminal.output', sessionId: string, data: string }
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `reactflow` (v11, npm `reactflow`) | `@xyflow/react` (v12, npm `@xyflow/react`) | 2024 | New package name, improved TypeScript generics, better API |
| Manual node positioning | `@dagrejs/dagre` auto-layout | Stable | Standard pairing with ReactFlow |

**Deprecated/outdated:**
- `reactflow` npm package (v11): Still works but no longer actively developed on main branch. Use `@xyflow/react` v12.

## V1 Scope Boundaries

The design docs describe many features that are explicitly **out of scope for V1/Phase 6**:

| Feature | Status | Reason |
|---------|--------|--------|
| Cron schedules | V2 | Not in JOBS requirements |
| Cross-job triggers | V2 | Not in JOBS requirements |
| Manual approval gates | V2 | Not in JOBS requirements |
| Job cloning | V2 | Not in JOBS requirements |
| Failure policies (beyond fail_run) | V2 | JOBS requirements only need basic execution |
| Machine auto-scheduling | V2 | V1 requires explicit machine_id per step |
| Job templates | V2 | Explicitly deferred (JOBS-05) |

**V1 focuses on:** Job/step CRUD, dependency edges, DAG validation, manual run trigger, step execution via sessions, real-time output, step retry. The `on_failure` default of `fail_run` is sufficient for V1.

## Open Questions

1. **Session reuse across steps in same run**
   - What we know: Each step creates a new session on the target machine
   - What's unclear: Should steps in the same run on the same machine reuse a session? Design doc says no -- each step gets its own session
   - Recommendation: Follow design doc -- separate sessions per step. Simpler, isolated, matches the "interactive notebook" metaphor

2. **Step output after session ends**
   - What we know: Step output is the terminal output from its session. Completed sessions have scrollback in the agent's buffer
   - What's unclear: How long scrollback is retained, whether it persists across agent restarts
   - Recommendation: This is a Phase 4 concern (session management). Phase 6 can rely on whatever Phase 4 provides for session replay

3. **Concurrent run limits**
   - What we know: Nothing prevents multiple runs of the same job simultaneously
   - What's unclear: Should there be a limit?
   - Recommendation: V1 allows unlimited concurrent runs. Add limits in V2 if needed

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing (backend) + Vitest (frontend) |
| Config file | None yet -- Wave 0 |
| Quick run command | `go test ./internal/server/orchestrator/... -count=1 -v` |
| Full suite command | `go test ./... && cd web && npm run test` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| JOBS-01 | Create job with steps + dependencies, validate DAG | unit | `go test ./internal/server/orchestrator/... -run TestCreateJob -v` | Wave 0 |
| JOBS-01 | DAG cycle detection rejects invalid dependencies | unit | `go test ./internal/server/orchestrator/... -run TestCycleDetection -v` | Wave 0 |
| JOBS-02 | Execute run, steps create sessions, output streams | integration | `go test ./internal/server/orchestrator/... -run TestRunExecution -v` | Wave 0 |
| JOBS-03 | Retry failed step resets status, creates new session | unit | `go test ./internal/server/orchestrator/... -run TestStepRetry -v` | Wave 0 |
| JOBS-04 | Steps wait for dependencies before executing | unit | `go test ./internal/server/orchestrator/... -run TestDependencyOrdering -v` | Wave 0 |
| JOBS-04 | Parallel independent steps execute concurrently | unit | `go test ./internal/server/orchestrator/... -run TestParallelExecution -v` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/server/orchestrator/... -count=1 -v`
- **Per wave merge:** `go test ./... && cd web && npm run test`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps

- [ ] `internal/server/orchestrator/dag_runner_test.go` -- covers JOBS-02, JOBS-04
- [ ] `internal/server/orchestrator/orchestrator_test.go` -- covers JOBS-01, JOBS-03
- [ ] `internal/server/handler/jobs_test.go` -- REST handler tests
- [ ] `internal/server/store/jobs_test.go` -- DB query tests
- [ ] `web/src/components/dag/__tests__/DAGCanvas.test.tsx` -- ReactFlow rendering
- [ ] Test framework setup: `go test` works out of the box; Vitest needs `web/vitest.config.ts`

## Sources

### Primary (HIGH confidence)
- `docs/internal/product/backend_v1.md` -- Section 7 (Job Orchestrator), Section 6 (Data Model), Section 8 (REST API)
- `docs/internal/product/frontend_v1.md` -- Sections 4.3-4.5 (Jobs/Runs views), Section 7 (State), Section 8 (API client), Section 9 (Components)
- `docs/internal/product/suplementary_v1.md` -- Section 8 (Job System Edge Cases)

### Secondary (MEDIUM confidence)
- [ReactFlow official docs](https://reactflow.dev/) -- v12 API, custom nodes/edges, dagre integration
- [@xyflow/react npm](https://www.npmjs.com/package/@xyflow/react) -- v12.10.1 current
- [ReactFlow dagre example](https://reactflow.dev/examples/layout/dagre) -- Auto-layout pattern

### Tertiary (LOW confidence)
- None -- all findings verified against design docs or official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- design docs prescribe exact libraries, verified versions on npm
- Architecture: HIGH -- design docs provide pseudocode, schema, API endpoints, component hierarchy
- Pitfalls: HIGH -- design docs explicitly cover edge cases (Section 8 of supplementary doc)

**Research date:** 2026-03-12
**Valid until:** 2026-04-12 (stable domain, design docs are authoritative)
