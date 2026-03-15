# Frontend UX Improvements Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 7 UX improvements: Command Center layout reorder, session type selector, step-to-task rename, run_job task type, shell command fix, GitHub connector restructure, and credentials/API keys/provisioning clarification (no code change).

**Architecture:** Frontend-first changes (items 1-3, 5) require no backend modifications. The run_job task type (item 4) adds a new task_type with DB migration, validation, and executor changes. GitHub connector restructure (item 6) splits creation from watch management and adds new trigger types to both frontend and backend.

**Tech Stack:** React 19, TypeScript, Zustand, TanStack Query, Go, SQLite, Chi router

**Spec:** `docs/superpowers/specs/2026-03-15-frontend-ux-improvements-design.md`

---

## Chunk 1: Frontend Step-to-Task Rename

This must be done first — all subsequent chunks reference post-rename names.

### Task 1: Rename type definitions

**Files:**
- Modify: `web/src/types/job.ts`

- [ ] **Step 1: Rename types in job.ts**

Rename ONLY the interface/type names. **All field names must stay unchanged** — they match the backend JSON keys (`step_id`, `step_count`, `steps`, `run_steps`, etc.).

Interface renames:
```typescript
// Before: export interface Step { ... }
// After:  export interface Task { ... }

// Before: export interface StepDependency { ... }
// After:  export interface TaskDependency { ... }

// Before: export interface RunStep { ... }
// After:  export interface RunTask { ... }

// Before: export interface CreateStepParams { ... }
// After:  export interface CreateTaskParams { ... }

// Before: export interface UpdateStepParams { ... }
// After:  export interface UpdateTaskParams { ... }
```

Keep all field names (`step_id`, `step_count`) exactly as they are — they must match the backend JSON.

In `JobDetail`, keep the `steps` field name (matches JSON) but change the type:
```typescript
export interface JobDetail {
  job: Job;
  steps: Task[];           // was Step[]
  dependencies: TaskDependency[];  // was StepDependency[]
}
```

In `RunDetail`, keep `run_steps` field name:
```typescript
export interface RunDetail {
  run: Run;
  run_steps: RunTask[];    // was RunStep[]
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd web && npx tsc --noEmit`
Expected: Type errors in files that import the old names (this is expected — we'll fix them in subsequent tasks).

- [ ] **Step 3: Commit**

```bash
git add web/src/types/job.ts
git commit -m "refactor: rename Step/RunStep types to Task/RunTask"
```

### Task 2: Update API client and imports

**Files:**
- Modify: `web/src/api/jobs.ts`

- [ ] **Step 1: Update imports and function names**

Update the import to use new type names:
```typescript
import type {
  Job,
  JobDetail,
  Task,           // was Step
  TaskDependency, // was StepDependency
  Run,
  RunDetail,
  CreateJobParams,
  CreateTaskParams,  // was CreateStepParams
  UpdateTaskParams,  // was UpdateStepParams
  ListRunsParams,
  TriggerRunParams,
} from '../types/job.ts';
```

Rename API functions (keep URL paths unchanged — they still hit `/steps`):
```typescript
// Steps → Tasks (URLs stay the same)
addTask: (jobId: string, params: CreateTaskParams) =>        // was addStep
  request<Task>(`/jobs/${encodeURIComponent(jobId)}/steps`, { ... }),
updateTask: (jobId: string, taskId: string, params: UpdateTaskParams) =>  // was updateStep
  request<{ status: string }>(`/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(taskId)}`, ...),
deleteTask: (jobId: string, taskId: string) =>               // was deleteStep
  request<void>(`/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(taskId)}`, ...),

// Dependencies (param names change, URLs stay)
addDependency: (jobId: string, taskId: string, dependsOnTaskId: string) =>  // was stepId, dependsOnStepId
  request<TaskDependency>(`/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(taskId)}/deps`, ...),
removeDependency: (jobId: string, taskId: string, depId: string) =>         // was stepId
  request<void>(`/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(taskId)}/deps/${encodeURIComponent(depId)}`, ...),

// Retry
retryTask: (runId: string, taskId: string) =>                // was retryStep
  request<{ status: string }>(`/runs/${encodeURIComponent(runId)}/steps/${encodeURIComponent(taskId)}/retry`, ...),
```

Also rename comment `// Steps` to `// Tasks`.

- [ ] **Step 2: Commit**

```bash
git add web/src/api/jobs.ts
git commit -m "refactor: rename step API functions to task"
```

### Task 3: Update hooks

**Files:**
- Modify: `web/src/hooks/useJobs.ts`
- Modify: `web/src/hooks/useRuns.ts`

- [ ] **Step 1: Update useJobs.ts**

Update import to use new type names (`CreateTaskParams`, `UpdateTaskParams`, `TriggerRunParams`).

Rename hooks:
- `useAddStep` → `useAddTask` — calls `jobsApi.addTask`
- `useUpdateStep` → `useUpdateTask` — calls `jobsApi.updateTask`
- `useDeleteStep` → `useDeleteTask` — calls `jobsApi.deleteTask`

Update parameter destructuring to use `taskId` instead of `stepId`:
```typescript
export function useAddTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params: CreateTaskParams }) =>
      jobsApi.addTask(jobId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useUpdateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, params }: { jobId: string; taskId: string; params: UpdateTaskParams }) =>
      jobsApi.updateTask(jobId, taskId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId }: { jobId: string; taskId: string }) =>
      jobsApi.deleteTask(jobId, taskId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}
```

Keep `useAddDependency` and `useRemoveDependency` names (they're about dependencies, not steps). Update their parameter names from `stepId` to `taskId`:
```typescript
export function useAddDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, dependsOnTaskId }: { jobId: string; taskId: string; dependsOnTaskId: string }) =>
      jobsApi.addDependency(jobId, taskId, dependsOnTaskId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useRemoveDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, depId }: { jobId: string; taskId: string; depId: string }) =>
      jobsApi.removeDependency(jobId, taskId, depId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}
```

- [ ] **Step 2: Update useRuns.ts**

Import `RunTask` instead of `RunStep`, `ListRunsParams`.

Rename:
- `useRetryStep` → `useRetryTask`
- Update parameter: `stepId` → `taskId`

```typescript
export function useRetryTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ runId, taskId }: { runId: string; taskId: string }) =>
      jobsApi.retryTask(runId, taskId),
    onSuccess: (_, { runId }) => {
      qc.invalidateQueries({ queryKey: ['runs', 'detail', runId] });
      qc.invalidateQueries({ queryKey: ['runs', 'list'] });
    },
  });
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/useJobs.ts web/src/hooks/useRuns.ts
git commit -m "refactor: rename step hooks to task"
```

### Task 4: Update Zustand stores

**Files:**
- Modify: `web/src/stores/jobs.ts`
- Modify: `web/src/stores/runs.ts`

- [ ] **Step 1: Update jobs store**

```typescript
import { create } from 'zustand';

interface JobEditorStore {
  selectedTaskId: string | null;         // was selectedStepId
  selectTask: (id: string | null) => void;  // was selectStep
}

export const useJobEditorStore = create<JobEditorStore>((set) => ({
  selectedTaskId: null,
  selectTask: (id) => set({ selectedTaskId: id }),
}));
```

- [ ] **Step 2: Update runs store**

Import `RunTask` instead of `RunStep`.

Rename all state/methods:
- `stepStatuses` → `taskStatuses`
- `selectedStepId` → `selectedTaskId`
- `setStepStatuses` → `setTaskStatuses`
- `updateStepStatus` → `updateTaskStatus`
- `selectStep` → `selectTask`

Inside `setTaskStatuses`: the map key is still `s.step_id` (matches JSON field).
Inside `updateTaskStatus`: parameters stay `stepId` (matches backend field name), but the method name changes.

```typescript
import { create } from 'zustand';
import type { RunTask } from '../types/job.ts';

interface RunStore {
  activeRunId: string | null;
  taskStatuses: Map<string, RunTask>;
  selectedTaskId: string | null;

  setActiveRunId: (runId: string | null) => void;
  setTaskStatuses: (tasks: RunTask[]) => void;
  updateTaskStatus: (runId: string, stepId: string, status: string, sessionId?: string) => void;
  selectTask: (id: string | null) => void;
  reset: () => void;
}

export const useRunStore = create<RunStore>((set) => ({
  activeRunId: null,
  taskStatuses: new Map(),
  selectedTaskId: null,

  setActiveRunId: (runId) =>
    set((state) => {
      if (state.activeRunId === runId) return state;
      return { activeRunId: runId, taskStatuses: new Map(), selectedTaskId: null };
    }),

  setTaskStatuses: (tasks) =>
    set({
      taskStatuses: new Map(tasks.map((t) => [t.step_id, t])),
    }),

  updateTaskStatus: (runId, stepId, status, sessionId) =>
    set((state) => {
      if (state.activeRunId && state.activeRunId !== runId) return state;
      const updated = new Map(state.taskStatuses);
      const existing = updated.get(stepId);
      if (existing) {
        updated.set(stepId, {
          ...existing,
          status: status as RunTask['status'],
          ...(sessionId ? { session_id: sessionId } : {}),
        });
      } else {
        updated.set(stepId, {
          run_step_id: '',
          run_id: runId,
          step_id: stepId,
          status: status as RunTask['status'],
          ...(sessionId ? { session_id: sessionId } : {}),
        });
      }
      return { taskStatuses: updated };
    }),

  selectTask: (id) => set({ selectedTaskId: id }),
  reset: () => set({ activeRunId: null, taskStatuses: new Map(), selectedTaskId: null }),
}));
```

- [ ] **Step 3: Commit**

```bash
git add web/src/stores/jobs.ts web/src/stores/runs.ts
git commit -m "refactor: rename step store state to task"
```

### Task 5: Rename DAG components

**Files:**
- Rename: `web/src/components/dag/StepNode.tsx` → `web/src/components/dag/TaskNode.tsx`
- Rename: `web/src/components/dag/StepEdge.tsx` → `web/src/components/dag/TaskEdge.tsx`
- Modify: `web/src/components/dag/DAGCanvas.tsx`
- Modify: `web/src/components/runs/RunDAGView.tsx`

- [ ] **Step 1: Rename StepNode.tsx → TaskNode.tsx**

```bash
cd web && git mv src/components/dag/StepNode.tsx src/components/dag/TaskNode.tsx
```

Inside the file:
- Rename `StepNodeData` → `TaskNodeData`
- Rename `StepNodeType` → `TaskNodeType`
- Rename function `StepNode` → `TaskNode`
- Update the export

- [ ] **Step 2: Rename StepEdge.tsx → TaskEdge.tsx**

```bash
cd web && git mv src/components/dag/StepEdge.tsx src/components/dag/TaskEdge.tsx
```

Inside the file:
- Rename `StepEdgeData` → `TaskEdgeData`
- Rename `StepEdgeType` → `TaskEdgeType`
- Rename function `StepEdge` → `TaskEdge`

- [ ] **Step 3: Update DAGCanvas.tsx**

Update imports and registrations:
```typescript
import { TaskNode } from './TaskNode.tsx';
import { TaskEdge } from './TaskEdge.tsx';

const nodeTypes = { step: TaskNode };  // key stays 'step' — it's a React Flow node type identifier
const edgeTypes = { step: TaskEdge };
```

Update type imports: `TaskNodeData`, `TaskEdgeData`, etc.

- [ ] **Step 4: Update RunDAGView.tsx**

Update prop types to use `Task`, `TaskDependency`, `RunTask` instead of `Step`, `StepDependency`, `RunStep`.

Update prop names if they use "step" in the prop name:
- `steps` prop can stay (it's the data, not the concept) — but update its type to `Task[]`
- `runSteps` → `runTasks` (prop name) with type `RunTask[]`
- `selectedStepId` → `selectedTaskId`
- `onStepSelect` → `onTaskSelect`

- [ ] **Step 5: Commit**

```bash
git add -A web/src/components/dag/ web/src/components/runs/RunDAGView.tsx
git commit -m "refactor: rename DAG step components to task"
```

### Task 6: Rename StepEditor component

**Files:**
- Rename: `web/src/components/jobs/StepEditor.tsx` → `web/src/components/jobs/TaskEditor.tsx`

- [ ] **Step 1: Rename file**

```bash
cd web && git mv src/components/jobs/StepEditor.tsx src/components/jobs/TaskEditor.tsx
```

- [ ] **Step 2: Update component internals**

- Rename `StepEditorProps` → `TaskEditorProps`
- Update prop types: `step: Step | null` → `task: Task | null`
- Rename component function: `StepEditor` → `TaskEditor`
- Update import types: `Task`, `UpdateTaskParams`
- Update all internal references from `step` to `task` (prop references)
- Update form field IDs: `step-name` → `task-name`, `step-prompt` → `task-prompt`, etc.
- Update section heading: "Step Configuration" → "Task Configuration"
- Update button text: "Save Step" → "Save Task"
- Keep `TaskType` internal type as-is (it's `'claude' | 'shell'`, not related to step naming)

- [ ] **Step 3: Commit**

```bash
git add -A web/src/components/jobs/
git commit -m "refactor: rename StepEditor to TaskEditor"
```

### Task 7: Update view components

**Files:**
- Modify: `web/src/views/JobEditor.tsx`
- Modify: `web/src/views/RunDetail.tsx`
- Modify: `web/src/views/CommandCenter.tsx`
- Modify: `web/src/views/JobsPage.tsx`

- [ ] **Step 1: Update JobEditor.tsx**

- Update imports: `TaskEditor` from `TaskEditor.tsx`, `useAddTask`, `useUpdateTask`, `useDeleteTask`
- Update store usage: `selectedTaskId`, `selectTask` from `useJobEditorStore`
- Update handler names: `handleStepDirtyChange` → `handleTaskDirtyChange`
- Update toast messages: "step" → "task" in all strings
- The tab labeled "Tasks" already uses the correct label — no change needed
- Update variable names referencing steps throughout

- [ ] **Step 2: Update RunDetail.tsx**

- Update store usage: `selectedTaskId`, `taskStatuses`, `selectTask`, `setTaskStatuses`, `updateTaskStatus`
- Update hook: `useRetryTask` instead of `useRetryStep`
- Update toast messages: "Step retrying" → "Task retrying", "Failed to retry step" → "Failed to retry task"
- Update UI text: "Click a step in the DAG" → "Click a task in the DAG", "Waiting for step to start" → "Waiting for task to start"
- Update `RunDAGView` props: `runTasks`, `selectedTaskId`, `onTaskSelect`

- [ ] **Step 3: Update CommandCenter.tsx**

- Change `{job.step_count} step${...}` to `{job.step_count} task${...}` (user-facing string)

- [ ] **Step 4: Update JobsPage.tsx**

- The job table shows `step_count` in a column. Update the column header/display text from "steps" to "tasks" (e.g., "3 steps" → "3 tasks")
- Search for any other "step" strings in the file and replace with "task"

- [ ] **Step 5: Verify typecheck and lint**

Run: `cd web && npx tsc --noEmit && npm run lint`
Expected: PASS (all references updated)

- [ ] **Step 6: Commit**

```bash
git add web/src/views/JobEditor.tsx web/src/views/RunDetail.tsx web/src/views/CommandCenter.tsx web/src/views/JobsPage.tsx
git commit -m "refactor: update view components for step-to-task rename"
```

### Task 8: Update event stream (keep wire format)

**Files:**
- Modify: `web/src/hooks/useEventStream.ts`
- Audit: `web/src/lib/types.ts` — do NOT change `EventType` string values (they match backend). Check for any `Step`/`RunStep` type re-exports and rename those to `Task`/`RunTask` if present.

- [ ] **Step 1: Update useEventStream.ts**

The event type string `'run.step.status'` must stay the same (matches backend).
Update the store method calls:
- `updateStepStatus` → `updateTaskStatus`

- [ ] **Step 1.5: Audit lib/types.ts**

Check `web/src/lib/types.ts` for any references to `Step`, `RunStep`, `StepDependency` types. If found, rename them to `Task`, `RunTask`, `TaskDependency`. Do NOT change the `EventType` string value `'run.step.status'` or the `Machine` interface.

- [ ] **Step 2: Run tests**

Run: `cd web && npx vitest run`
Expected: PASS

- [ ] **Step 3: Final typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/hooks/useEventStream.ts
git commit -m "refactor: complete step-to-task rename in event stream"
```

---

## Chunk 2: Shell Task Command Fix + Command Center Layout

### Task 9: Fix shell task command defaults

**Files:**
- Modify: `web/src/components/jobs/TaskEditor.tsx`

- [ ] **Step 1: Fix getFormParams command default**

In `getFormParams()`, the claude branch defaults command to `'claude'`:
```typescript
command: (data.get('command') as string) || 'claude',
```

For the shell branch, command should NOT default:
```typescript
// Shell task — command is required, no default
command: data.get('command') as string,
```

This is already correct in the current code (line 54). The issue is that the form field itself shows `'claude'` as a placeholder. Fix:

- [ ] **Step 2: Update command field placeholder and required indicator**

In the command input field (~line 527-540 in old file):
- When `taskType === 'shell'`: show placeholder `"e.g., ./deploy.sh, python script.py"`, mark as required
- When `taskType === 'claude'`: show placeholder `"claude"`, NOT required (backend defaults it)

```tsx
<label htmlFor="task-command" className="...">
  Command
  {taskType === 'shell' && <span className="text-red-400 ml-1">*</span>}
  {taskType === 'claude' && <span className="text-text-secondary/50">(defaults to "claude")</span>}
</label>
<input
  id="task-command"
  name="command"
  type="text"
  defaultValue={task?.command ?? (taskType === 'claude' ? '' : '')}
  placeholder={taskType === 'shell' ? 'e.g., ./deploy.sh, python script.py' : 'claude'}
  required={taskType === 'shell'}
  className="..."
/>
```

- [ ] **Step 3: Verify with lint and typecheck**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 4: Commit**

```bash
git add web/src/components/jobs/TaskEditor.tsx
git commit -m "fix: shell tasks no longer default command to claude"
```

### Task 10: Reorder Command Center layout

**Files:**
- Modify: `web/src/views/CommandCenter.tsx`

- [ ] **Step 1: Restructure the layout sections**

Current order (from research):
1. Header (lines 113-122)
2. Stats Row 1: Sessions & Machines (lines 124-144)
3. Stats Row 2: Jobs & Runs (lines 146-175)
4. Templates (lines 177-226)
5. Recent Jobs & Recent Runs grid (lines 228-291)
6. Active Sessions (lines 293-316)
7. Machines (lines 318-348)

New order:
1. Header (keep)
2. Stats Row — single row with 5 cards: Active Sessions, Online Machines, Total Jobs, Recent Runs, Completion Rate. Remove "Total Sessions" and "Jobs Run" cards.
3. Machines & Active Sessions — side-by-side in a 2-column grid
4. Recent Jobs — full width
5. Recent Runs — full width
6. Templates — at the bottom, conditional

Move the Machines section (currently at bottom) and Active Sessions section up to position 3, placing them in a `grid grid-cols-1 lg:grid-cols-2 gap-4` layout.

Move Templates from position 4 to position 6 (bottom).

- [ ] **Step 2: Remove "Total Sessions" and "Jobs Run" stat cards**

Remove the StatCard for "Total Sessions" (the one showing `sessions.length` with purple accent).
Remove the StatCard for "Jobs Run" (the one showing jobs that have been executed at least once).

Merge the remaining stats into a single `grid-cols-2 sm:grid-cols-5` row.

- [ ] **Step 3: Update task count display**

Change the text that shows step count on job cards:
```tsx
// Before
{job.step_count} step{job.step_count !== 1 ? 's' : ''}

// After
{job.step_count} task{job.step_count !== 1 ? 's' : ''}
```

- [ ] **Step 4: Verify**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 5: Commit**

```bash
git add web/src/views/CommandCenter.tsx
git commit -m "feat: reorder Command Center layout, remove Total Sessions stat"
```

---

## Chunk 3: New Session Type Selector

### Task 11: Add session type toggle to NewSessionModal

**Files:**
- Modify: `web/src/components/sessions/NewSessionModal.tsx`

- [ ] **Step 1: Add session type state**

Add a `sessionType` state variable:
```typescript
const [sessionType, setSessionType] = useState<'claude' | 'terminal'>('claude');
```

- [ ] **Step 2: Add segmented control UI**

Add a segmented control as the FIRST field in the form (before Template):
```tsx
<div className="mb-4">
  <label className="block text-sm font-medium text-text-secondary mb-1">Session Type</label>
  <div className="flex rounded-md overflow-hidden border border-border">
    <button
      type="button"
      onClick={() => setSessionType('claude')}
      className={`flex-1 px-3 py-2 text-sm font-medium transition-colors ${
        sessionType === 'claude'
          ? 'bg-indigo-500 text-white'
          : 'bg-surface-secondary text-text-secondary hover:bg-surface-tertiary'
      }`}
    >
      Claude
    </button>
    <button
      type="button"
      onClick={() => setSessionType('terminal')}
      className={`flex-1 px-3 py-2 text-sm font-medium transition-colors ${
        sessionType === 'terminal'
          ? 'bg-cyan-500 text-white'
          : 'bg-surface-secondary text-text-secondary hover:bg-surface-tertiary'
      }`}
    >
      Terminal
    </button>
  </div>
</div>
```

- [ ] **Step 3: Conditionally render fields**

Wrap Claude-specific fields in `{sessionType === 'claude' && (...)}`:
- Template selector
- Command input
- Model select
- Skip Permissions select
- Template Variables

Keep visible for both types:
- Machine select (required)
- Working Directory input (optional)

- [ ] **Step 4: Update submit handler**

When `sessionType === 'terminal'`, set command to `'bash'`:
```typescript
const handleSubmit = async () => {
  const cmd = sessionType === 'terminal' ? 'bash' : (command || 'claude');
  // ... rest of submit logic using cmd
};
```

- [ ] **Step 5: Reset form state on type switch**

When switching session type, reset Claude-specific fields:
```typescript
useEffect(() => {
  if (sessionType === 'terminal') {
    setCommand('');
    setModel('');
    setSkipPermissions('');
    setSelectedTemplate(null);
    setVariables({});
  }
}, [sessionType]);
```

- [ ] **Step 6: Verify**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 7: Commit**

```bash
git add web/src/components/sessions/NewSessionModal.tsx
git commit -m "feat: add session type selector (Claude vs Terminal)"
```

### Task 12: Add visual differentiation for terminal sessions

**Files:**
- Modify: Session list components that display sessions (CommandCenter active sessions, SessionsPage)

- [ ] **Step 1: Identify terminal sessions**

Terminal sessions are identified by `command === 'bash'` (or any command that isn't `'claude'`). Add a helper:
```typescript
const isTerminalSession = (command: string) => command !== 'claude';
```

- [ ] **Step 2: Add Terminal/Claude badge**

In session list items, add a badge:
```tsx
import { Terminal, Bot } from 'lucide-react';

{isTerminalSession(session.command) ? (
  <span className="flex items-center gap-1 text-xs text-cyan-400">
    <Terminal className="w-3 h-3" /> Terminal
  </span>
) : (
  <span className="flex items-center gap-1 text-xs text-indigo-400">
    <Bot className="w-3 h-3" /> Claude
  </span>
)}
```

- [ ] **Step 3: Update Workbench session display**

Also add the Terminal/Claude badge to the Workbench view (IDE-like terminal view) wherever sessions are listed or tabbed.

- [ ] **Step 4: Commit**

```bash
git add web/src/views/CommandCenter.tsx web/src/views/SessionsPage.tsx web/src/components/
git commit -m "feat: visual badge for terminal vs claude sessions"
```

---

## Chunk 4: run_job Task Type — Backend

### Task 13: Database migration for run_job fields

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add migration 11**

Append to the `migrations` slice (after migration 10, before the closing `}`):

```go
{
    Version:     11,
    Description: "add run_job task type fields",
    SQL: `
ALTER TABLE steps ADD COLUMN target_job_id TEXT;
ALTER TABLE steps ADD COLUMN job_params TEXT;
ALTER TABLE run_steps ADD COLUMN target_job_id_snapshot TEXT;
ALTER TABLE run_steps ADD COLUMN job_params_snapshot TEXT;
`,
},
```

- [ ] **Step 2: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: migration 11 — add run_job fields to steps and run_steps"
```

### Task 14: Update Go store types and queries

**Files:**
- Modify: `internal/server/store/jobs.go`

- [ ] **Step 1: Add fields to Step struct**

Add to `Step` struct (after `Parameters` field at line 183):
```go
TargetJobID string `json:"target_job_id,omitempty"`
JobParams   string `json:"job_params,omitempty"`
```

- [ ] **Step 2: Add fields to RunStep struct**

Add to `RunStep` struct (after `ParametersSnapshot` field):
```go
TargetJobIDSnapshot string `json:"target_job_id_snapshot,omitempty"`
JobParamsSnapshot   string `json:"job_params_snapshot,omitempty"`
```

- [ ] **Step 3: Add fields to CreateStepParams and UpdateStepParams**

Add to both `CreateStepParams` (after `Parameters` field) and `UpdateStepParams`:
```go
TargetJobID string
JobParams   string
```

- [ ] **Step 4: Update CreateStep SQL**

Update the INSERT to include `target_job_id, job_params` columns and values.

- [ ] **Step 5: Update UpdateStep SQL**

Update the UPDATE SET to include `target_job_id = ?, job_params = ?`.

- [ ] **Step 6: Update GetStepsWithDeps SQL**

Add `COALESCE(target_job_id, ''), COALESCE(job_params, '')` to the SELECT and Scan.

- [ ] **Step 7: Update InsertRunSteps SQL**

Add `target_job_id_snapshot, job_params_snapshot` to the INSERT for run_steps, copying from `st.TargetJobID` and `st.JobParams`.

- [ ] **Step 8: Update GetRunWithSteps SQL**

Add `COALESCE(target_job_id_snapshot, ''), COALESCE(job_params_snapshot, '')` to the SELECT and Scan.

- [ ] **Step 9: Run tests**

Run: `go test -race ./internal/server/store/...`

- [ ] **Step 10: Commit**

```bash
git add internal/server/store/jobs.go
git commit -m "feat: store support for run_job target_job_id and job_params"
```

### Task 15: Update validation for run_job

**Files:**
- Modify: `internal/server/orchestrator/dag_runner.go`

- [ ] **Step 1: Update ValidateJobSteps**

Change the task_type validation (line 25) to accept `run_job`:
```go
if s.TaskType != "claude_session" && s.TaskType != "shell" && s.TaskType != "run_job" {
    errs = append(errs, fmt.Errorf("step %q: task_type must be 'claude_session', 'shell', or 'run_job'", s.Name))
}
```

Add run_job-specific validations after the shell validations:
```go
if s.TaskType == "run_job" {
    if s.TargetJobID == "" {
        errs = append(errs, fmt.Errorf("step %q: run_job tasks require a target_job_id", s.Name))
    }
    if s.MachineID != "" {
        errs = append(errs, fmt.Errorf("step %q: run_job tasks must not have a machine_id", s.Name))
    }
    if s.SessionKey != "" {
        errs = append(errs, fmt.Errorf("step %q: run_job tasks cannot share sessions", s.Name))
    }
}
```

The function receives individual steps without job context, so self-reference validation (`target_job_id != current job ID`) cannot be done here. It is handled in the handler layer (Task 16).
```

- [ ] **Step 2: Run tests**

Run: `go test -race ./internal/server/orchestrator/...`

- [ ] **Step 3: Commit**

```bash
git add internal/server/orchestrator/dag_runner.go
git commit -m "feat: validation support for run_job task type"
```

### Task 16: Update handler for run_job

**Files:**
- Modify: `internal/server/handler/jobs.go`

- [ ] **Step 1: Add fields to request structs**

Add to `addStepRequest` (line 332) and `updateStepRequest` (line 451):
```go
TargetJobID string `json:"target_job_id"`
JobParams   string `json:"job_params"`
```

- [ ] **Step 2: Update AddStep handler**

After the shell task handling (line 362), add run_job handling:
```go
// For run_job tasks, clear prompt and command.
if req.TaskType == "run_job" {
    req.Prompt = ""
    req.Command = ""
    req.MachineID = ""
}
```

Update the `store.CreateStepParams` to include `TargetJobID` and `JobParams`.

Add self-reference validation:
```go
if req.TaskType == "run_job" && req.TargetJobID == detail.Job.JobID {
    writeError(w, http.StatusBadRequest, "run_job task cannot target its own job")
    return
}
```

- [ ] **Step 3: Update UpdateStep handler**

Same changes as AddStep: add run_job handling, include new fields in `store.UpdateStepParams`.

- [ ] **Step 4: Update store validation**

In `store/jobs.go` `CreateStep` and `UpdateStep` functions, add `run_job` to the valid task_type check:
```go
if p.TaskType != "claude_session" && p.TaskType != "shell" && p.TaskType != "run_job" {
    return nil, fmt.Errorf("invalid task_type %q: must be claude_session, shell, or run_job", p.TaskType)
}
```

Add run_job-specific handling:
```go
if p.TaskType == "run_job" {
    p.Prompt = ""
    p.Command = ""
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/server/handler/...`

- [ ] **Step 6: Commit**

```bash
git add internal/server/handler/jobs.go internal/server/store/jobs.go
git commit -m "feat: handler support for run_job task type"
```

### Task 17: Add run_job executor

**Files:**
- Modify: `internal/server/executor/session_executor.go`

- [ ] **Step 1: Add RunStarter interface**

The executor needs to be able to start runs. Add an interface:
```go
// RunStarter creates a new run for a job. Used by run_job tasks.
type RunStarter interface {
    StartRun(ctx context.Context, jobID string, params map[string]string) error
}
```

- [ ] **Step 2: Add RunStarter to SessionStepExecutor**

Add a `runStarter` field and update the constructor:
```go
type SessionStepExecutor struct {
    connMgr    *connmgr.ConnectionManager
    store      storeIface
    logger     *slog.Logger
    runStarter RunStarter  // for run_job tasks

    // ... existing fields
}

func NewSessionStepExecutor(
    connMgr *connmgr.ConnectionManager,
    st storeIface,
    logger *slog.Logger,
    runStarter RunStarter,
) *SessionStepExecutor {
    // ... add runStarter to struct init
}
```

- [ ] **Step 3: Add run_job dispatch in ExecuteStep**

Update the switch in `ExecuteStep`:
```go
func (e *SessionStepExecutor) ExecuteStep(
    ctx context.Context,
    runStep store.RunStep,
    resolveCtx *orchestrator.ResolveContext,
    onComplete func(stepID string, exitCode int),
) {
    switch runStep.TaskTypeSnapshot {
    case "shell":
        e.executeShellTask(ctx, runStep, resolveCtx, onComplete)
    case "run_job":
        e.executeRunJob(ctx, runStep, resolveCtx, onComplete)
    default:
        e.executeClaudeSession(ctx, runStep, resolveCtx, onComplete)
    }
}
```

- [ ] **Step 4: Implement executeRunJob**

```go
// executeRunJob triggers a child job run and immediately completes.
func (e *SessionStepExecutor) executeRunJob(
    ctx context.Context,
    runStep store.RunStep,
    resolveCtx *orchestrator.ResolveContext,
    onComplete func(stepID string, exitCode int),
) {
    targetJobID := resolveField(runStep.TargetJobIDSnapshot, resolveCtx)
    if targetJobID == "" {
        e.logger.Error("run_job task has no target_job_id", "step_id", runStep.StepID)
        // Mark as running then failed
        _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "running", "", 0)
        _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "failed", "", failureExitCode)
        onComplete(runStep.StepID, failureExitCode)
        return
    }

    // Parse job params
    params := make(map[string]string)
    if runStep.JobParamsSnapshot != "" {
        if err := json.Unmarshal([]byte(runStep.JobParamsSnapshot), &params); err != nil {
            e.logger.Error("failed to parse job_params", "error", err, "step_id", runStep.StepID)
            _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "running", "", 0)
            _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "failed", "", failureExitCode)
            onComplete(runStep.StepID, failureExitCode)
            return
        }
    }

    // Resolve template expressions in param values
    if resolveCtx != nil {
        for k, v := range params {
            params[k] = resolveField(v, resolveCtx)
        }
    }

    // Mark step as running
    _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "running", "", 0)

    // Fire and forget — start the child run
    if err := e.runStarter.StartRun(ctx, targetJobID, params); err != nil {
        e.logger.Error("failed to start child job run", "error", err, "target_job_id", targetJobID)
        _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "failed", "", failureExitCode)
        onComplete(runStep.StepID, failureExitCode)
        return
    }

    // Immediately mark as completed
    _ = e.store.UpdateRunStepStatus(ctx, runStep.RunStepID, "completed", "", 0)
    onComplete(runStep.StepID, 0)
}
```

- [ ] **Step 5: Update all callers of NewSessionStepExecutor**

Search for `NewSessionStepExecutor(` calls and add the `RunStarter` parameter. This will likely be in the server's main setup code.

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/server/executor/...`

- [ ] **Step 7: Run full backend tests**

Run: `go vet ./... && go test -race ./...`

- [ ] **Step 8: Commit**

```bash
git add internal/server/executor/ internal/server/
git commit -m "feat: run_job executor — fire-and-forget child job execution"
```

---

## Chunk 5: run_job Task Type — Frontend

### Task 18: Add run_job to frontend types

**Files:**
- Modify: `web/src/types/job.ts`

- [ ] **Step 1: Add run_job fields to Task type**

Add optional fields to the `Task` interface:
```typescript
target_job_id?: string;
job_params?: Record<string, string>;
```

Add same fields to `CreateTaskParams` and `UpdateTaskParams`:
```typescript
target_job_id?: string;
job_params?: Record<string, string>;
```

Note: The backend stores `job_params` as a JSON string, but the frontend type uses `Record<string, string>` for ergonomics. Serialization to/from JSON happens at the API boundary (in `getFormParams` and when parsing responses).

- [ ] **Step 2: Commit**

```bash
git add web/src/types/job.ts
git commit -m "feat: add run_job fields to frontend Task type"
```

### Task 19: Add run_job UI to TaskEditor

**Files:**
- Modify: `web/src/components/jobs/TaskEditor.tsx`

- [ ] **Step 1: Add 'run_job' to TaskType**

Update the type:
```typescript
type TaskType = 'claude' | 'shell' | 'run_job';
```

- [ ] **Step 2: Update resolveTaskType**

```typescript
function resolveTaskType(task: Task): TaskType {
  if (task.task_type === 'shell') return 'shell';
  if (task.task_type === 'run_job') return 'run_job';
  return 'claude';
}
```

- [ ] **Step 3: Update getFormParams for run_job**

Add a run_job branch:
```typescript
if (taskType === 'run_job') {
  return {
    ...base,
    prompt: '',
    command: '',
    args: '',
    model: undefined,
    skip_permissions: undefined,
    session_key: undefined,
    machine_id: '',
    target_job_id: data.get('target_job_id') as string,
    job_params: data.get('job_params') as string,
  };
}
```

- [ ] **Step 4: Add task type selector with 3 options**

Update the toggle from 2 buttons to 3:
```tsx
<div className="flex rounded-md overflow-hidden border border-border">
  {(['claude', 'shell', 'run_job'] as const).map((type) => (
    <button key={type} type="button" onClick={() => setTaskType(type)}
      className={`flex-1 px-3 py-2 text-sm font-medium ${taskType === type ? 'bg-accent text-white' : '...'}`}>
      {type === 'claude' ? 'Claude Session' : type === 'shell' ? 'Shell' : 'Run Job'}
    </button>
  ))}
</div>
```

- [ ] **Step 5: Add job selector and param fields**

When `taskType === 'run_job'`, render:
- A job dropdown (needs `useJobs()` hook to fetch job list)
- Dynamic parameter fields based on the selected job's `parameters`

```tsx
{taskType === 'run_job' && (
  <>
    <div>
      <label htmlFor="task-target-job" className="...">
        Target Job <span className="text-red-400">*</span>
      </label>
      <select id="task-target-job" name="target_job_id" required
        value={targetJobId} onChange={(e) => setTargetJobId(e.target.value)}>
        <option value="">Select a job...</option>
        {jobs?.filter(j => j.job_id !== currentJobId).map(j => (
          <option key={j.job_id} value={j.job_id}>{j.name}</option>
        ))}
      </select>
    </div>
    {targetJobParams && Object.entries(targetJobParams).map(([key, defaultVal]) => (
      <div key={key}>
        <label className="...">{key}</label>
        <input name={`job_param_${key}`} defaultValue={...} placeholder={defaultVal || `Value for ${key}`} />
      </div>
    ))}
  </>
)}
```

- [ ] **Step 6: Conditionally hide fields for run_job**

When `taskType === 'run_job'`, hide:
- Prompt, Machine, Model, Skip Permissions, Session Key, Command, Args, Working Directory

Keep visible:
- Name, Delay, Run If, Max Retries, Retry Delay, On Failure

- [ ] **Step 7: Collect job_params on submit**

In `getFormParams`, for `run_job` type, collect all `job_param_*` form fields into a JSON string:
```typescript
const jobParams: Record<string, string> = {};
for (const [key, value] of data.entries()) {
  if (key.startsWith('job_param_') && typeof value === 'string' && value !== '') {
    jobParams[key.replace('job_param_', '')] = value;
  }
}
base.job_params = Object.keys(jobParams).length > 0 ? JSON.stringify(jobParams) : undefined;
```

- [ ] **Step 8: Verify**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 9: Commit**

```bash
git add web/src/components/jobs/TaskEditor.tsx
git commit -m "feat: run_job task type UI with job selector and parameter fields"
```

---

## Chunk 6: GitHub Connector Restructure

### Task 20: Simplify GitHub connector creation form

**Files:**
- Modify: `web/src/components/connectors/GithubForm.tsx`

- [ ] **Step 1: Strip watches from creation mode**

In the `GithubForm` component, when `mode === 'create'` (no existing connector), only render:
- Connector Name input
- GitHub Token input

Remove the watches section and "Add Watch" button from the create form.

For edit mode, keep the token field (optional update) but also remove watches — they move to the detail page.

- [ ] **Step 2: Update submit to navigate to detail page**

On successful creation, navigate to `/connectors/{connector_id}`:
```typescript
const navigate = useNavigate();
// After successful create:
navigate(`/connectors/${result.connector_id}`);
```

- [ ] **Step 3: Set empty watches config on create**

When creating, send `config: JSON.stringify({ watches: [] })`:
```typescript
const config = JSON.stringify({ watches: [] });
```

- [ ] **Step 4: Commit**

```bash
git add web/src/components/connectors/GithubForm.tsx
git commit -m "feat: simplify GitHub connector creation to name + token only"
```

### Task 21: Create connector detail page

**Files:**
- Create: `web/src/views/ConnectorDetailPage.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Add route to App.tsx**

Add below the `/connectors` route:
```tsx
<Route path="/connectors/:connectorId" element={<ProtectedRoute><ConnectorDetailPage /></ProtectedRoute>} />
```

Add the import at the top.

- [ ] **Step 2: Create ConnectorDetailPage.tsx**

Build a page with:
- Header: connector name, type badge (GitHub/Telegram), edit button (opens modal for name/token), delete button
- Watches section: list of watches with add/edit/delete
- Reuse `WatchEditor` and `TriggerConfig` components for editing watches
- "Save Changes" button that PUTs the updated connector config
- "Apply & Restart" banner when changes are saved

Use `useParams()` to get `connectorId`, fetch with `useConnector(connectorId)`.

For watch CRUD, maintain a local `watches` state array. On save, serialize to JSON and call `updateConnector` with the new config.

Key structure:
```tsx
export default function ConnectorDetailPage() {
  const { connectorId } = useParams<{ connectorId: string }>();
  const { data: connector } = useConnector(connectorId!);
  const updateConnector = useUpdateConnector();
  const [watches, setWatches] = useState<WatchData[]>([]);
  const [editingWatchIdx, setEditingWatchIdx] = useState<number | null>(null);

  // Parse watches from connector config on load
  useEffect(() => {
    if (connector?.config) {
      const parsed = JSON.parse(connector.config);
      setWatches(hydrateWatches(parsed.watches || []));
    }
  }, [connector]);

  const handleSave = async () => {
    const config = buildConfigJson(watches);
    await updateConnector.mutateAsync({
      id: connectorId!,
      params: { connector_type: connector!.connector_type, name: connector!.name, config },
    });
  };

  // ... render header, watches list, add/edit/delete controls
}
```

- [ ] **Step 3: Add useConnector hook if missing**

Check `web/src/hooks/useBridge.ts` — research showed `useConnector(id)` already exists (lines 12-18). Verify it works for single connector fetch.

- [ ] **Step 4: Verify**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 5: Commit**

```bash
git add web/src/views/ConnectorDetailPage.tsx web/src/App.tsx
git commit -m "feat: connector detail page for managing watches"
```

### Task 22: Add new GitHub trigger types — Frontend

**Files:**
- Modify: `web/src/components/connectors/TriggerConfig.tsx`
- Modify: `web/src/components/connectors/WatchEditor.tsx`
- Modify: `web/src/components/connectors/watchDefaults.ts`

- [ ] **Step 1: Update watchDefaults.ts**

Add new trigger defaults:
```typescript
export function createDefaultWatch(): WatchData {
  return {
    // ... existing fields ...
    triggers: {
      pull_request_opened: { enabled: false, filters: {} },
      check_run_completed: { enabled: false, filters: {} },
      issue_labeled: { enabled: false, filters: {} },
      issue_comment: { enabled: false, filters: {} },
      pull_request_comment: { enabled: false, filters: {} },
      pull_request_review: { enabled: false, filters: {} },
      release_published: { enabled: false, filters: {} },
    },
  };
}
```

- [ ] **Step 2: Update WatchData interface**

In `WatchEditor.tsx`, add new triggers to the `WatchData.triggers` type:
```typescript
triggers: {
  pull_request_opened: { enabled: boolean; filters: TriggerFilters };
  check_run_completed: { enabled: boolean; filters: TriggerFilters };
  issue_labeled: { enabled: boolean; filters: TriggerFilters };
  issue_comment: { enabled: boolean; filters: TriggerFilters };
  pull_request_comment: { enabled: boolean; filters: TriggerFilters };
  pull_request_review: { enabled: boolean; filters: TriggerFilters };
  release_published: { enabled: boolean; filters: TriggerFilters };
};
```

- [ ] **Step 3: Add TriggerFilters fields for new triggers**

In `TriggerConfig.tsx`, add filter fields:
- `review_states?: string[]` — for pull_request_review (approved, changes_requested, commented)
- `tag_patterns?: string[]` — for release_published

Update `TriggerFilters` interface:
```typescript
export interface TriggerFilters {
  branches?: string[];
  labels?: string[];
  check_names?: string[];
  conclusions?: string[];
  paths?: string[];
  authors_ignore?: string[];
  review_states?: string[];    // NEW
  tag_patterns?: string[];     // NEW
}
```

- [ ] **Step 4: Add UI sections for new triggers in TriggerConfig.tsx**

Add 4 new collapsible trigger sections:

**Issue Comment:**
- Authors Ignore (tag input)

**PR Comment:**
- Authors Ignore (tag input)

**PR Review Submitted:**
- Review State checkboxes: approved, changes_requested, commented
- Authors Ignore (tag input)

**Release Published:**
- Tag Patterns (tag input, e.g., `v1.*`)

- [ ] **Step 5: Update GithubForm buildConfigJson**

Update the serialization function to include new trigger types in the config JSON.

- [ ] **Step 6: Verify**

Run: `cd web && npx tsc --noEmit && npm run lint`

- [ ] **Step 7: Commit**

```bash
git add web/src/components/connectors/
git commit -m "feat: add issue_comment, pr_comment, pr_review, release triggers to frontend"
```

### Task 23: Add new GitHub trigger types — Backend

**Files:**
- Modify: `internal/bridge/connector/github/github.go`
- Modify: `internal/bridge/connector/github/poller.go`
- Modify: `internal/bridge/connector/github/filters.go`
- Modify: `internal/bridge/connector/github/variables.go`

- [ ] **Step 1: Update TriggerConfig struct in github.go**

Add new trigger fields:
```go
type TriggerConfig struct {
    PullRequestOpened  *PRTrigger      `json:"pull_request_opened"`
    CheckRunCompleted  *CheckTrigger   `json:"check_run_completed"`
    IssueLabeled       *IssueTrigger   `json:"issue_labeled"`
    IssueComment       *CommentTrigger `json:"issue_comment"`
    PullRequestComment *CommentTrigger `json:"pull_request_comment"`
    PullRequestReview  *ReviewTrigger  `json:"pull_request_review"`
    ReleasePublished   *ReleaseTrigger `json:"release_published"`
}

type CommentTrigger struct {
    Filters Filters `json:"filters"`
}

type ReviewTrigger struct {
    Filters Filters `json:"filters"`
}

type ReleaseTrigger struct {
    Filters Filters `json:"filters"`
}
```

Note: These trigger structs intentionally omit an `Enabled` field — the trigger is considered enabled when its pointer is non-nil in `TriggerConfig`. This matches the existing pattern: `PullRequestOpened *PRTrigger` is nil when disabled, non-nil when enabled. The frontend sends `null` for disabled triggers and `{ "filters": {...} }` for enabled ones. Verify that existing trigger types (`PRTrigger`, `CheckTrigger`, `IssueTrigger`) follow this same pattern.

- [ ] **Step 2: Update Filters struct**

Add new filter fields:
```go
type Filters struct {
    // ... existing fields
    ReviewStates []string `json:"review_states,omitempty"` // approved, changes_requested, commented
    TagPatterns  []string `json:"tag_patterns,omitempty"`
}
```

- [ ] **Step 3: Add filter matching functions**

In `filters.go`, add:
```go
func matchReviewStates(states []string, state string) bool {
    if len(states) == 0 { return true }
    for _, s := range states { if s == state { return true } }
    return false
}

func matchTagPatterns(patterns []string, tag string) bool {
    if len(patterns) == 0 { return true }
    for _, p := range patterns {
        if matched, _ := filepath.Match(p, tag); matched { return true }
    }
    return false
}
```

- [ ] **Step 4: Add polling for new event types**

In `poller.go`, add polling functions for each new trigger type. Each should:
- Poll the appropriate GitHub API endpoint
- Track state with separate cursors (`comment:{repo}`, `review:{repo}`, `release:{repo}`)
- Match against filters
- Extract variables and create sessions

GitHub API endpoints:
- Issue comments: `GET /repos/{owner}/{repo}/issues/comments?since={cursor}&sort=updated&direction=asc`
- PR comments: `GET /repos/{owner}/{repo}/pulls/comments?since={cursor}&sort=updated&direction=asc`
- PR reviews: `GET /repos/{owner}/{repo}/pulls/{number}/reviews` (for each open PR)
- Releases: `GET /repos/{owner}/{repo}/releases?per_page=10` (track latest release ID)

- [ ] **Step 5: Add variable extraction**

In `variables.go`, add variable extraction for each new event type:
- Issue comment: `COMMENT_BODY`, `COMMENT_AUTHOR`, `ISSUE_NUMBER`, `ISSUE_TITLE`, `ISSUE_URL`
- PR comment: `COMMENT_BODY`, `COMMENT_AUTHOR`, `PR_NUMBER`, `PR_TITLE`, `PR_URL`
- PR review: `REVIEW_STATE`, `REVIEW_AUTHOR`, `REVIEW_BODY`, `PR_NUMBER`, `PR_URL`
- Release: `RELEASE_TAG`, `RELEASE_NAME`, `RELEASE_URL`, `RELEASE_AUTHOR`

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/bridge/...`

- [ ] **Step 7: Run full backend tests**

Run: `go vet ./... && go test -race ./...`

- [ ] **Step 8: Commit**

```bash
git add internal/bridge/connector/github/
git commit -m "feat: add issue_comment, pr_comment, pr_review, release_published triggers to backend"
```

---

## Chunk 7: Final Verification

### Task 24: Full build and test verification

- [ ] **Step 1: Frontend verification**

```bash
cd web && npx tsc --noEmit && npm run lint && npx vitest run
```

- [ ] **Step 2: Backend verification**

```bash
go vet ./... && go test -race ./...
```

- [ ] **Step 3: Build binaries**

```bash
go build -o /dev/null ./cmd/server && go build -o /dev/null ./cmd/agent && go build -o /dev/null ./cmd/bridge
```

- [ ] **Step 4: Frontend build**

```bash
cd web && npm run build
```

- [ ] **Step 5: Commit any final fixes**

If any verification steps fail, fix and commit.
