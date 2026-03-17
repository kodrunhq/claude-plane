import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { schedulesApi } from '../api/schedules.ts';
import type { CreateScheduleParams, UpdateScheduleParams } from '../types/schedule.ts';

export function useAllSchedules() {
  return useQuery({
    queryKey: ['schedules', 'all'],
    queryFn: () => schedulesApi.listAll(),
  });
}

export function useSchedules(jobId: string | undefined) {
  return useQuery({
    queryKey: ['schedules', jobId],
    queryFn: () => schedulesApi.list(jobId!),
    enabled: !!jobId,
  });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, params }: { jobId: string; params: CreateScheduleParams }) =>
      schedulesApi.create(jobId, params),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['schedules', data.job_id] });
    },
  });
}

export function useUpdateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: UpdateScheduleParams }) =>
      schedulesApi.update(id, params),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['schedules', data.job_id] });
    },
  });
}

export function useDeleteSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id }: { id: string; jobId: string }) =>
      schedulesApi.delete(id),
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: ['schedules', variables.jobId] });
    },
  });
}

export function usePauseSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => schedulesApi.pause(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['schedules', data.job_id] });
    },
  });
}

export function useResumeSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => schedulesApi.resume(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ['schedules', data.job_id] });
    },
  });
}
