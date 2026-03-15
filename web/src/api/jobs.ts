import { request } from './client.ts';
import type {
  Job,
  JobDetail,
  Step,
  StepDependency,
  Run,
  RunDetail,
  CreateJobParams,
  CreateStepParams,
  UpdateStepParams,
  ListRunsParams,
  TriggerRunParams,
} from '../types/job.ts';

export const jobsApi = {
  // Jobs CRUD
  list: () => request<Job[]>('/jobs'),
  get: (id: string) => request<JobDetail>(`/jobs/${encodeURIComponent(id)}`),
  create: (params: CreateJobParams) =>
    request<Job>('/jobs', { method: 'POST', body: JSON.stringify(params) }),
  update: (id: string, params: Partial<CreateJobParams>) =>
    request<Job>(`/jobs/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),
  delete: (id: string) =>
    request<void>(`/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  // Steps
  addStep: (jobId: string, params: CreateStepParams) =>
    request<Step>(`/jobs/${encodeURIComponent(jobId)}/steps`, {
      method: 'POST',
      body: JSON.stringify(params),
    }),
  updateStep: (jobId: string, stepId: string, params: UpdateStepParams) =>
    request<{ status: string }>(
      `/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(stepId)}`,
      { method: 'PUT', body: JSON.stringify(params) },
    ),
  deleteStep: (jobId: string, stepId: string) =>
    request<void>(
      `/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(stepId)}`,
      { method: 'DELETE' },
    ),

  // Dependencies
  addDependency: (jobId: string, stepId: string, dependsOnStepId: string) =>
    request<StepDependency>(
      `/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(stepId)}/deps`,
      { method: 'POST', body: JSON.stringify({ depends_on: dependsOnStepId }) },
    ),
  removeDependency: (jobId: string, stepId: string, depId: string) =>
    request<void>(
      `/jobs/${encodeURIComponent(jobId)}/steps/${encodeURIComponent(stepId)}/deps/${encodeURIComponent(depId)}`,
      { method: 'DELETE' },
    ),

  // Runs
  triggerRun: (jobId: string, params?: TriggerRunParams) =>
    request<Run>(`/jobs/${encodeURIComponent(jobId)}/runs`, {
      method: 'POST',
      body: JSON.stringify(params ?? {}),
    }),
  listRuns: (params?: ListRunsParams) => {
    const searchParams = new URLSearchParams();
    if (params?.job_id) searchParams.set('job_id', params.job_id);
    if (params?.status) searchParams.set('status', params.status);
    if (params?.trigger_type) searchParams.set('trigger_type', params.trigger_type);
    if (params?.limit) searchParams.set('limit', String(params.limit));
    if (params?.offset) searchParams.set('offset', String(params.offset));
    const qs = searchParams.toString();
    return request<Run[]>(`/runs${qs ? `?${qs}` : ''}`);
  },
  getRun: (id: string) => request<RunDetail>(`/runs/${encodeURIComponent(id)}`),
  cancelRun: (id: string) =>
    request<Run>(`/runs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  retryStep: (runId: string, stepId: string) =>
    request<{ status: string }>(
      `/runs/${encodeURIComponent(runId)}/steps/${encodeURIComponent(stepId)}/retry`,
      { method: 'POST' },
    ),
  repairRun: (runId: string, params?: { parameters?: Record<string, string> }) =>
    request<{ status: string }>(`/runs/${encodeURIComponent(runId)}/repair`, {
      method: 'POST',
      body: JSON.stringify(params ?? {}),
    }),
};
