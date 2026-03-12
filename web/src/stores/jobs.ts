import { create } from 'zustand';

interface JobEditorStore {
  selectedStepId: string | null;
  selectStep: (id: string | null) => void;
}

export const useJobEditorStore = create<JobEditorStore>((set) => ({
  selectedStepId: null,
  selectStep: (id) => set({ selectedStepId: id }),
}));
