import { create } from 'zustand';

interface JobEditorStore {
  selectedTaskId: string | null;
  selectTask: (id: string | null) => void;
}

export const useJobEditorStore = create<JobEditorStore>((set) => ({
  selectedTaskId: null,
  selectTask: (id) => set({ selectedTaskId: id }),
}));
