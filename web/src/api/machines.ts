import { request } from './client.ts';
import type { Machine } from '../lib/types.ts';

export interface UpdateMachineParams {
  display_name: string;
}

export interface BrowseEntry {
  name: string;
  type: 'dir' | 'file';
}

export interface BrowseResponse {
  path: string;
  entries: BrowseEntry[];
  parent: string;
}

export const browseMachineDirectory = (machineId: string, path: string) =>
  request<BrowseResponse>(`/machines/${encodeURIComponent(machineId)}/browse?path=${encodeURIComponent(path)}`);

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
