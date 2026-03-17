import { request } from './client.ts';
import type { JobTrigger, CreateTriggerParams, UpdateTriggerParams } from '../types/trigger.ts';

export const triggersApi = {
  listByJob: (jobId: string) =>
    request<JobTrigger[]>(`/jobs/${encodeURIComponent(jobId)}/triggers`),

  create: (jobId: string, params: CreateTriggerParams) =>
    request<JobTrigger>(`/jobs/${encodeURIComponent(jobId)}/triggers`, {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  update: (triggerId: string, params: UpdateTriggerParams) =>
    request<JobTrigger>(`/triggers/${encodeURIComponent(triggerId)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),

  toggle: (triggerId: string) =>
    request<JobTrigger>(`/triggers/${encodeURIComponent(triggerId)}/toggle`, {
      method: 'POST',
    }),

  delete: (triggerId: string) =>
    request<void>(`/triggers/${encodeURIComponent(triggerId)}`, { method: 'DELETE' }),
};
