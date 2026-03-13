import { request } from './client.ts';
import type { CronSchedule, CreateScheduleParams, UpdateScheduleParams } from '../types/schedule.ts';

export const schedulesApi = {
  list: (jobId: string) =>
    request<CronSchedule[]>(`/jobs/${encodeURIComponent(jobId)}/schedules`),
  get: (id: string) =>
    request<CronSchedule>(`/schedules/${encodeURIComponent(id)}`),
  create: (jobId: string, params: CreateScheduleParams) =>
    request<CronSchedule>(`/jobs/${encodeURIComponent(jobId)}/schedules`, {
      method: 'POST',
      body: JSON.stringify(params),
    }),
  update: (id: string, params: UpdateScheduleParams) =>
    request<CronSchedule>(`/schedules/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),
  delete: (id: string) =>
    request<void>(`/schedules/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  pause: (id: string) =>
    request<CronSchedule>(`/schedules/${encodeURIComponent(id)}/pause`, {
      method: 'POST',
    }),
  resume: (id: string) =>
    request<CronSchedule>(`/schedules/${encodeURIComponent(id)}/resume`, {
      method: 'POST',
    }),
};
