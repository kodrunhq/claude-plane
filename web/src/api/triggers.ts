import { request } from './client.ts';
import type { JobTrigger, CreateTriggerParams } from '../types/trigger.ts';

export const triggersApi = {
  listByJob: (jobId: string) =>
    request<JobTrigger[]>(`/jobs/${encodeURIComponent(jobId)}/triggers`),

  create: (jobId: string, params: CreateTriggerParams) =>
    request<JobTrigger>(`/jobs/${encodeURIComponent(jobId)}/triggers`, {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  delete: (triggerId: string) =>
    request<void>(`/triggers/${encodeURIComponent(triggerId)}`, { method: 'DELETE' }),
};
