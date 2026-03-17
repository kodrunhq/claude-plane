import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { credentialsApi } from '../api/credentials.ts';
import type { CreateCredentialParams } from '../types/credential.ts';

export function useCredentials() {
  return useQuery({
    queryKey: ['credentials'],
    queryFn: () => credentialsApi.list(),
  });
}

export function useCredentialStatus() {
  return useQuery({
    queryKey: ['credentials', 'status'],
    queryFn: () => credentialsApi.status(),
    staleTime: 5 * 60 * 1000, // status rarely changes, cache for 5 min
  });
}

export function useCreateCredential() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateCredentialParams) => credentialsApi.create(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['credentials'] }),
  });
}

export function useDeleteCredential() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => credentialsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['credentials'] }),
  });
}
