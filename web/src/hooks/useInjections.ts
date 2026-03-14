import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { injectionsApi } from '../api/injections.ts';

export function useInjections(sessionId: string | undefined) {
  return useQuery({
    queryKey: ['injections', sessionId],
    queryFn: () => injectionsApi.list(sessionId!),
    enabled: !!sessionId,
    refetchInterval: 5000,
  });
}

export function useInjectSession() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      sessionId,
      params,
    }: {
      sessionId: string;
      params: { text: string; raw?: boolean; delay_ms?: number };
    }) => injectionsApi.inject(sessionId, params),
    onSuccess: (_, { sessionId }) => {
      qc.invalidateQueries({ queryKey: ['injections', sessionId] });
    },
  });
}
