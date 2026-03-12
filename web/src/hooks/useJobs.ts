import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/jobs.ts';
import type { CreateJobParams, CreateStepParams, UpdateStepParams } from '../types/job.ts';

export function useJobs() {
  return useQuery({
    queryKey: ['jobs'],
    queryFn: () => jobsApi.list(),
  });
}

export function useJob(id: string | undefined) {
  return useQuery({
    queryKey: ['jobs', id],
    queryFn: () => jobsApi.get(id!),
    enabled: !!id,
  });
}

export function useCreateJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateJobParams) => jobsApi.create(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['jobs'] }),
  });
}

export function useUpdateJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: Partial<CreateJobParams> }) =>
      jobsApi.update(id, params),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['jobs', id] });
      qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
}

export function useDeleteJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => jobsApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['jobs'] }),
  });
}

export function useAddStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params: CreateStepParams }) =>
      jobsApi.addStep(jobId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useUpdateStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, stepId, params }: { jobId: string; stepId: string; params: UpdateStepParams }) =>
      jobsApi.updateStep(jobId, stepId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useDeleteStep() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, stepId }: { jobId: string; stepId: string }) =>
      jobsApi.deleteStep(jobId, stepId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useAddDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, stepId, dependsOnStepId }: { jobId: string; stepId: string; dependsOnStepId: string }) =>
      jobsApi.addDependency(jobId, stepId, dependsOnStepId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useRemoveDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, stepId, depId }: { jobId: string; stepId: string; depId: string }) =>
      jobsApi.removeDependency(jobId, stepId, depId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useTriggerRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (jobId: string) => jobsApi.triggerRun(jobId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['runs'] }),
  });
}
