import { request } from './client.ts';
import type { ProvisioningToken, ProvisionResult, CreateProvisionParams } from '../types/provisioning.ts';

export const provisioningApi = {
  listTokens: () =>
    request<ProvisioningToken[]>('/provision/tokens'),

  createToken: (params: CreateProvisionParams) =>
    request<ProvisionResult>('/provision/agent', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  revokeToken: (tokenId: string) =>
    request<void>(`/provision/tokens/${encodeURIComponent(tokenId)}`, {
      method: 'DELETE',
    }),
};
