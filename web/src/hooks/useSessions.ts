import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sessionsApi } from '../api/sessions.ts';
import type { CreateSessionRequest } from '../types/session.ts';

export function useSessions(filters?: Record<string, string>) {
  return useQuery({
    queryKey: ['sessions', filters],
    queryFn: () => sessionsApi.list(filters),
    refetchInterval: 30_000, // Fallback polling; WebSocket is primary
    placeholderData: [],
  });
}

export function useSession(id: string) {
  return useQuery({
    queryKey: ['sessions', id],
    queryFn: () => sessionsApi.get(id),
    enabled: !!id,
  });
}

export function useCreateSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateSessionRequest) => sessionsApi.create(params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });
}

export function useTerminateSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => sessionsApi.kill(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sessions'] });
    },
  });
}
