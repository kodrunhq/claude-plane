import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/jobs.ts';
import type { ListRunsParams, Run } from '../types/job.ts';

function hasActiveRuns(runs: Run[] | undefined): boolean {
  return runs?.some((r) => r.status === 'pending' || r.status === 'running') ?? false;
}

export function useRuns(params?: ListRunsParams) {
  return useQuery({
    queryKey: ['runs', 'list', params],
    queryFn: () => jobsApi.listRuns(params),
    refetchInterval: (query) => hasActiveRuns(query.state.data) ? 5_000 : false,
  });
}

export function useRun(id: string | undefined) {
  return useQuery({
    queryKey: ['runs', 'detail', id],
    queryFn: () => jobsApi.getRun(id!),
    enabled: !!id,
    refetchInterval: 5_000,
  });
}

export function useCancelRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => jobsApi.cancelRun(id),
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: ['runs', 'detail', id] });
      qc.invalidateQueries({ queryKey: ['runs', 'list'] });
    },
  });
}

export function useRetryStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ runId, stepId }: { runId: string; stepId: string }) =>
      jobsApi.retryStep(runId, stepId),
    onSuccess: (_, { runId }) => {
      qc.invalidateQueries({ queryKey: ['runs', 'detail', runId] });
      qc.invalidateQueries({ queryKey: ['runs', 'list'] });
    },
  });
}
