import { request } from './client.ts';
import type {
  Job,
  JobDetail,
  Step,
  StepDependency,
  Run,
  RunStep,
  RunDetail,
  CreateJobParams,
  CreateStepParams,
  UpdateStepParams,
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
    request<Step>(
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
  triggerRun: (jobId: string) =>
    request<Run>(`/jobs/${encodeURIComponent(jobId)}/runs`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  listRuns: (jobId: string) => request<Run[]>(`/runs?job_id=${encodeURIComponent(jobId)}`),
  getRun: (id: string) => request<RunDetail>(`/runs/${encodeURIComponent(id)}`),
  cancelRun: (id: string) =>
    request<Run>(`/runs/${encodeURIComponent(id)}/cancel`, { method: 'POST' }),
  retryStep: (runId: string, stepId: string) =>
    request<RunStep>(
      `/runs/${encodeURIComponent(runId)}/steps/${encodeURIComponent(stepId)}/retry`,
      { method: 'POST' },
    ),
};
