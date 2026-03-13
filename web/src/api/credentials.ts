import { request } from './client.ts';
import type { Credential, CreateCredentialParams } from '../types/credential.ts';

export const credentialsApi = {
  list: () => request<Credential[]>('/credentials'),

  create: (params: CreateCredentialParams) =>
    request<Credential>('/credentials', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  delete: (id: string) =>
    request<void>(`/credentials/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};
