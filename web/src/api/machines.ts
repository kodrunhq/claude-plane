import { request } from './client.ts';
import type { Machine } from '../lib/types.ts';

export const machinesApi = {
  list: () => request<Machine[]>('/machines'),
  get: (id: string) => request<Machine>(`/machines/${encodeURIComponent(id)}`),
};
