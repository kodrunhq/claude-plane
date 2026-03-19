// MSW v2 request handlers — shared mock API for tests

import { http, HttpResponse } from 'msw';
import type { Session } from '../types/session.ts';
import type { Machine } from '../lib/types.ts';
import type { Job, Run, Task, RunTask } from '../types/job.ts';
import type { SessionTemplate } from '../types/template.ts';
import type { Event } from '../types/event.ts';
import type { BrowseResponse } from '../api/machines.ts';
import {
  buildSession,
  buildMachine,
  buildJob,
  buildRun,
  buildTemplate,
  buildEvent,
  buildTask,
  buildRunTask,
  buildBrowseResponse,
} from './factories.ts';

// Pre-built mock data arrays for assertions in tests
export const mockSessions: Session[] = [
  buildSession({ session_id: 'sess-100', status: 'running', machine_id: 'machine-100' }),
  buildSession({ session_id: 'sess-101', status: 'completed', machine_id: 'machine-100' }),
];

export const mockMachines: Machine[] = [
  buildMachine({ machine_id: 'machine-100', display_name: 'Worker Alpha', status: 'connected' }),
  buildMachine({ machine_id: 'machine-101', display_name: 'Worker Beta', status: 'disconnected' }),
];

export const mockJobs: Job[] = [
  buildJob({ job_id: 'job-100', name: 'Deploy Frontend' }),
  buildJob({ job_id: 'job-101', name: 'Run Tests' }),
];

export const mockRuns: Run[] = [
  buildRun({ run_id: 'run-100', job_id: 'job-100', status: 'completed' }),
  buildRun({ run_id: 'run-101', job_id: 'job-101', status: 'running' }),
];

export const mockTemplates: SessionTemplate[] = [
  buildTemplate({ template_id: 'tmpl-100', name: 'Default Claude' }),
];

export const mockEvents: Event[] = [
  buildEvent({ event_id: 'evt-100', event_type: 'session.started' }),
  buildEvent({ event_id: 'evt-101', event_type: 'machine.connected' }),
];

export const mockTasks: Task[] = [
  buildTask({ step_id: 'step-100', job_id: 'job-100', name: 'Analyze codebase', task_type: 'claude', prompt: 'Analyze the project structure' }),
  buildTask({ step_id: 'step-101', job_id: 'job-100', name: 'Run linter', task_type: 'shell', command: 'npm', args: 'run lint', prompt: '' }),
];

export const mockRunTasks: RunTask[] = [
  buildRunTask({ run_step_id: 'rstep-100', run_id: 'run-100', step_id: 'step-100', status: 'completed' }),
  buildRunTask({ run_step_id: 'rstep-101', run_id: 'run-100', step_id: 'step-101', status: 'running' }),
];

export const mockBrowseResponse: BrowseResponse = buildBrowseResponse();

// Handlers array — add to or override per-test with server.use(...)
export const handlers = [
  http.get('/api/v1/sessions', () => HttpResponse.json(mockSessions)),
  http.get('/api/v1/sessions/:id', ({ params }) => {
    const session = mockSessions.find((s) => s.session_id === params.id);
    return session
      ? HttpResponse.json(session)
      : HttpResponse.json({ error: 'not found' }, { status: 404 });
  }),

  http.get('/api/v1/machines', () => HttpResponse.json(mockMachines)),
  http.get('/api/v1/machines/:id', ({ params }) => {
    const machine = mockMachines.find((m) => m.machine_id === params.id);
    return machine
      ? HttpResponse.json(machine)
      : HttpResponse.json({ error: 'not found' }, { status: 404 });
  }),

  http.get('/api/v1/jobs', () => HttpResponse.json(mockJobs)),
  http.get('/api/v1/jobs/:id', ({ params }) => {
    const job = mockJobs.find((j) => j.job_id === params.id);
    return job
      ? HttpResponse.json({ job, steps: [], dependencies: [] })
      : HttpResponse.json({ error: 'not found' }, { status: 404 });
  }),

  http.get('/api/v1/runs', () => HttpResponse.json(mockRuns)),
  http.get('/api/v1/runs/:id', ({ params }) => {
    const run = mockRuns.find((r) => r.run_id === params.id);
    return run
      ? HttpResponse.json({ run, run_steps: [] })
      : HttpResponse.json({ error: 'not found' }, { status: 404 });
  }),

  http.get('/api/v1/templates', () => HttpResponse.json(mockTemplates)),

  http.get('/api/v1/events', () => HttpResponse.json(mockEvents)),

  http.get('/api/v1/logs', () => HttpResponse.json({ logs: [], total: 0 })),
  http.get('/api/v1/logs/stats', () => HttpResponse.json({ by_level: {}, by_component: {}, total: 0 })),
  http.get('/api/v1/sessions/stats', () => HttpResponse.json({ total: 0, succeeded: 0, failed: 0, since: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString() })),

  http.get('/api/v1/users/me/preferences', () =>
    HttpResponse.json({
      ui: {
        theme: 'system',
        terminal_font_size: 14,
        auto_attach_session: false,
        command_center_cards: ['sessions', 'machines', 'jobs', 'runs', 'templates'],
      },
    }),
  ),

  // --- Session mutations ---
  http.post('/api/v1/sessions', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>;
    const session = buildSession({
      machine_id: (body.machine_id as string) ?? 'machine-100',
      status: 'running',
    });
    return HttpResponse.json(session, { status: 201 });
  }),
  http.delete('/api/v1/sessions/:id', () => new HttpResponse(null, { status: 204 })),

  // --- Job mutations ---
  http.post('/api/v1/jobs', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>;
    const job = buildJob({ name: (body.name as string) ?? 'New Job' });
    return HttpResponse.json(job, { status: 201 });
  }),
  http.put('/api/v1/jobs/:id', async ({ request, params }) => {
    const body = await request.json() as Record<string, unknown>;
    const existing = mockJobs.find((j) => j.job_id === params.id);
    const job = buildJob({ ...existing, ...body as Partial<Job> });
    return HttpResponse.json(job);
  }),
  http.delete('/api/v1/jobs/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/jobs/:id/clone', ({ params }) => {
    const job = buildJob({ name: `Clone of ${params.id}` });
    return HttpResponse.json({ job, steps: [], dependencies: [] }, { status: 201 });
  }),

  // --- Task (step) mutations ---
  http.post('/api/v1/jobs/:jobId/steps', async ({ request, params }) => {
    const body = await request.json() as Record<string, unknown>;
    const task = buildTask({
      job_id: params.jobId as string,
      name: (body.name as string) ?? 'New Task',
      machine_id: (body.machine_id as string) ?? 'machine-100',
    });
    return HttpResponse.json(task, { status: 201 });
  }),
  http.put('/api/v1/jobs/:jobId/steps/:stepId', () =>
    HttpResponse.json({ status: 'ok' }),
  ),
  http.delete('/api/v1/jobs/:jobId/steps/:stepId', () =>
    new HttpResponse(null, { status: 204 }),
  ),

  // --- Dependency mutations ---
  http.post('/api/v1/jobs/:jobId/steps/:stepId/deps', async ({ request, params }) => {
    const body = await request.json() as Record<string, unknown>;
    return HttpResponse.json(
      { step_id: params.stepId, depends_on: body.depends_on },
      { status: 201 },
    );
  }),
  http.delete('/api/v1/jobs/:jobId/steps/:stepId/deps/:depId', () =>
    new HttpResponse(null, { status: 204 }),
  ),

  // --- Run mutations ---
  http.post('/api/v1/jobs/:jobId/runs', ({ params }) => {
    const run = buildRun({ job_id: params.jobId as string, status: 'pending' });
    return HttpResponse.json(run, { status: 201 });
  }),
  http.post('/api/v1/runs/:id/cancel', ({ params }) => {
    const run = buildRun({ run_id: params.id as string, status: 'cancelled' });
    return HttpResponse.json(run);
  }),
  http.post('/api/v1/runs/:runId/steps/:stepId/retry', () =>
    HttpResponse.json({ status: 'ok' }),
  ),
  http.post('/api/v1/runs/:id/repair', () =>
    HttpResponse.json({ status: 'ok' }),
  ),

  // --- Machine mutations ---
  http.put('/api/v1/machines/:id', async ({ request, params }) => {
    const body = await request.json() as Record<string, unknown>;
    const existing = mockMachines.find((m) => m.machine_id === params.id);
    const machine = buildMachine({ ...existing, display_name: (body.display_name as string) ?? 'Updated' });
    return HttpResponse.json(machine);
  }),
  http.delete('/api/v1/machines/:id', () => new HttpResponse(null, { status: 204 })),
  http.get('/api/v1/machines/:id/browse', () =>
    HttpResponse.json(mockBrowseResponse),
  ),

  // --- Template mutations ---
  http.post('/api/v1/templates', async ({ request }) => {
    const body = await request.json() as Record<string, unknown>;
    const template = buildTemplate({ name: (body.name as string) ?? 'New Template' });
    return HttpResponse.json(template, { status: 201 });
  }),
  http.put('/api/v1/templates/:id', async ({ request, params }) => {
    const body = await request.json() as Record<string, unknown>;
    const existing = mockTemplates.find((t) => t.template_id === params.id);
    const template = buildTemplate({ ...existing, ...body as Partial<SessionTemplate> });
    return HttpResponse.json(template);
  }),
  http.delete('/api/v1/templates/:id', () => new HttpResponse(null, { status: 204 })),
  http.post('/api/v1/templates/:id/clone', ({ params }) => {
    const existing = mockTemplates.find((t) => t.template_id === params.id);
    const template = buildTemplate({ name: `Copy of ${existing?.name ?? params.id}` });
    return HttpResponse.json(template, { status: 201 });
  }),
];
