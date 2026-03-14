import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiKeysApi } from '../api/apikeys.ts';
import type { CreateAPIKeyParams } from '../types/apikey.ts';

export function useApiKeys() {
  return useQuery({ queryKey: ['api-keys'], queryFn: () => apiKeysApi.list() });
}

export function useCreateApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateAPIKeyParams) => apiKeysApi.create(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-keys'] }),
  });
}

export function useDeleteApiKey() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (keyId: string) => apiKeysApi.delete(keyId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['api-keys'] }),
  });
}
