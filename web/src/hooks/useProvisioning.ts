import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { provisioningApi } from '../api/provisioning.ts';
import type { CreateProvisionParams } from '../types/provisioning.ts';

export function useProvisioningTokens() {
  return useQuery({
    queryKey: ['provisioning', 'tokens'],
    queryFn: () => provisioningApi.listTokens(),
  });
}

export function useCreateProvisioningToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateProvisionParams) => provisioningApi.createToken(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['provisioning', 'tokens'] }),
  });
}

export function useRevokeProvisioningToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (tokenId: string) => provisioningApi.revokeToken(tokenId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['provisioning', 'tokens'] }),
  });
}
