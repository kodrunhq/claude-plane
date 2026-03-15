import { request } from './client.ts';

export type ServerSettings = Record<string, string>;

export const settingsApi = {
  get: () => request<ServerSettings>('/settings'),
  update: (settings: Partial<ServerSettings>) =>
    request<ServerSettings>('/settings', { method: 'PUT', body: JSON.stringify(settings) }),
};
