import { create } from 'zustand';
import type { RunStep } from '../types/job.ts';

interface RunStore {
  /** The currently viewed run ID */
  activeRunId: string | null;
  /** Map of stepId -> RunStep for the active run */
  stepStatuses: Map<string, RunStep>;
  selectedStepId: string | null;

  setActiveRunId: (runId: string | null) => void;
  setStepStatuses: (steps: RunStep[]) => void;
  updateStepStatus: (runId: string, stepId: string, status: string, sessionId?: string) => void;
  selectStep: (id: string | null) => void;
  reset: () => void;
}

export const useRunStore = create<RunStore>((set) => ({
  activeRunId: null,
  stepStatuses: new Map(),
  selectedStepId: null,

  setActiveRunId: (runId) =>
    set((state) => {
      if (state.activeRunId === runId) return state;
      return { activeRunId: runId, stepStatuses: new Map(), selectedStepId: null };
    }),

  setStepStatuses: (steps) =>
    set({
      stepStatuses: new Map(steps.map((s) => [s.step_id, s])),
    }),

  updateStepStatus: (runId, stepId, status, sessionId) =>
    set((state) => {
      // Ignore updates for runs other than the currently viewed one
      if (state.activeRunId && state.activeRunId !== runId) return state;
      const updated = new Map(state.stepStatuses);
      const existing = updated.get(stepId);
      if (existing) {
        updated.set(stepId, {
          ...existing,
          status: status as RunStep['status'],
          ...(sessionId ? { session_id: sessionId } : {}),
        });
      } else {
        updated.set(stepId, {
          run_step_id: '',
          run_id: runId,
          step_id: stepId,
          status: status as RunStep['status'],
          ...(sessionId ? { session_id: sessionId } : {}),
        });
      }
      return { stepStatuses: updated };
    }),

  selectStep: (id) => set({ selectedStepId: id }),
  reset: () => set({ activeRunId: null, stepStatuses: new Map(), selectedStepId: null }),
}));
