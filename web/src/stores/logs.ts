import { create } from 'zustand';
import type { LogFilter } from '../types/log.ts';

interface LogsStore {
  filter: LogFilter;
  live: boolean;
  setFilter: (filter: Partial<LogFilter>) => void;
  setLive: (live: boolean) => void;
  resetFilter: () => void;
}

const defaultFilter: LogFilter = {
  limit: 100,
  offset: 0,
};

export const useLogsStore = create<LogsStore>((set) => ({
  filter: { ...defaultFilter },
  live: false,
  setFilter: (partial) =>
    set((state) => ({ filter: { ...state.filter, ...partial, offset: 0 } })),
  setLive: (live) => set({ live }),
  resetFilter: () => set({ filter: { ...defaultFilter } }),
}));
