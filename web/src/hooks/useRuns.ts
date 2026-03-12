import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/jobs.ts';

export function useRuns() {
  return useQuery({
    queryKey: ['runs'],
    queryFn: () => jobsApi.listRuns(),
  });
}

export function useRun(id: string | undefined) {
  return useQuery({
    queryKey: ['runs', id],
    queryFn: () => jobsApi.getRun(id!),
    enabled: !!id,
    refetchInterval: 5_000, // Fallback polling; WebSocket is primary
  });
}

export function useCancelRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => jobsApi.cancelRun(id),
    onSuccess: (_, id) => qc.invalidateQueries({ queryKey: ['runs', id] }),
  });
}

export function useRetryStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ runId, stepId }: { runId: string; stepId: string }) =>
      jobsApi.retryStep(runId, stepId),
    onSuccess: (_, { runId }) => qc.invalidateQueries({ queryKey: ['runs', runId] }),
  });
}
