import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/jobs.ts';

export function useRuns(jobId: string | undefined) {
  return useQuery({
    queryKey: ['runs', 'list', jobId],
    queryFn: () => jobsApi.listRuns({ job_id: jobId! }),
    enabled: !!jobId,
  });
}

export function useRun(id: string | undefined) {
  return useQuery({
    queryKey: ['runs', 'detail', id],
    queryFn: () => jobsApi.getRun(id!),
    enabled: !!id,
    refetchInterval: 5_000, // Fallback polling; WebSocket is primary
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
