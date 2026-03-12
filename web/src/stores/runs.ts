import { create } from 'zustand';
import type { RunStep } from '../types/job.ts';

interface RunStore {
  /** Map of stepId -> RunStep for the active run */
  stepStatuses: Map<string, RunStep>;
  selectedStepId: string | null;

  setStepStatuses: (steps: RunStep[]) => void;
  updateStepStatus: (stepId: string, status: string, sessionId?: string) => void;
  selectStep: (id: string | null) => void;
  reset: () => void;
}

export const useRunStore = create<RunStore>((set) => ({
  stepStatuses: new Map(),
  selectedStepId: null,

  setStepStatuses: (steps) =>
    set({
      stepStatuses: new Map(steps.map((s) => [s.step_id, s])),
    }),

  updateStepStatus: (stepId, status, sessionId) =>
    set((state) => {
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
          id: '',
          run_id: '',
          step_id: stepId,
          status: status as RunStep['status'],
          ...(sessionId ? { session_id: sessionId } : {}),
        });
      }
      return { stepStatuses: updated };
    }),

  selectStep: (id) => set({ selectedStepId: id }),
  reset: () => set({ stepStatuses: new Map(), selectedStepId: null }),
}));
