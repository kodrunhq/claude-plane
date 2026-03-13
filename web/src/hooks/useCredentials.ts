import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { credentialsApi } from '../api/credentials.ts';
import type { CreateCredentialParams } from '../types/credential.ts';

export function useCredentials() {
  return useQuery({
    queryKey: ['credentials'],
    queryFn: () => credentialsApi.list(),
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
