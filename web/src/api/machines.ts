import { request } from './client.ts';
import type { Machine } from '../lib/types.ts';

export interface UpdateMachineParams {
  display_name: string;
}

export const machinesApi = {
  list: () => request<Machine[]>('/machines'),
  get: (id: string) => request<Machine>(`/machines/${encodeURIComponent(id)}`),
  update: (id: string, params: UpdateMachineParams) =>
    request<Machine>(`/machines/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),
  delete: (id: string) =>
    request<void>(`/machines/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
};
