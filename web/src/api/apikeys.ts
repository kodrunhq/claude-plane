import { request } from './client.ts';
import type { APIKey, CreateAPIKeyParams, CreateAPIKeyResponse } from '../types/apikey.ts';

export const apiKeysApi = {
  list: () => request<APIKey[]>('/api-keys'),
  create: (params: CreateAPIKeyParams) =>
    request<CreateAPIKeyResponse>('/api-keys', { method: 'POST', body: JSON.stringify(params) }),
  delete: (keyId: string) =>
    request<void>(`/api-keys/${encodeURIComponent(keyId)}`, { method: 'DELETE' }),
};
