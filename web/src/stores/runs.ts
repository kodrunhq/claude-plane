import { create } from 'zustand';
import type { RunTask } from '../types/job.ts';

interface RunStore {
  /** The currently viewed run ID */
  activeRunId: string | null;
  /** Map of stepId -> RunTask for the active run */
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
      // Ignore updates for runs other than the currently viewed one
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
