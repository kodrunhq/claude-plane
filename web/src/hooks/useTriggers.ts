import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { triggersApi } from '../api/triggers.ts';
import type { CreateTriggerParams, UpdateTriggerParams } from '../types/trigger.ts';

export function useTriggers(jobId: string | undefined) {
  return useQuery({
    queryKey: ['triggers', jobId],
    queryFn: () => triggersApi.listByJob(jobId!),
    enabled: !!jobId,
  });
}

export function useCreateTrigger() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params: CreateTriggerParams }) =>
      triggersApi.create(jobId, params),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['triggers', data.job_id] });
    },
  });
}

export function useUpdateTrigger() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ triggerId, params }: { triggerId: string; params: UpdateTriggerParams }) =>
      triggersApi.update(triggerId, params),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['triggers', data.job_id] });
    },
  });
}

export function useToggleTrigger() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ triggerId }: { triggerId: string; jobId: string }) =>
      triggersApi.toggle(triggerId),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['triggers', data.job_id] });
    },
  });
}

export function useDeleteTrigger() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ triggerId }: { triggerId: string; jobId: string }) =>
      triggersApi.delete(triggerId),
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: ['triggers', variables.jobId] });
    },
  });
}
