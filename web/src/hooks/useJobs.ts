import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { jobsApi } from '../api/jobs.ts';
import type { CreateJobParams, CreateTaskParams, UpdateTaskParams, TriggerRunParams } from '../types/job.ts';

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

export function useAddTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params: CreateTaskParams }) =>
      jobsApi.addTask(jobId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useUpdateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, params }: { jobId: string; taskId: string; params: UpdateTaskParams }) =>
      jobsApi.updateTask(jobId, taskId, params),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useDeleteTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId }: { jobId: string; taskId: string }) =>
      jobsApi.deleteTask(jobId, taskId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useAddDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, dependsOnTaskId }: { jobId: string; taskId: string; dependsOnTaskId: string }) =>
      jobsApi.addDependency(jobId, taskId, dependsOnTaskId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useRemoveDependency() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, taskId, depId }: { jobId: string; taskId: string; depId: string }) =>
      jobsApi.removeDependency(jobId, taskId, depId),
    onSuccess: (_, { jobId }) => qc.invalidateQueries({ queryKey: ['jobs', jobId] }),
  });
}

export function useTriggerRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params?: TriggerRunParams }) =>
      jobsApi.triggerRun(jobId, params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['runs'] }),
  });
}
