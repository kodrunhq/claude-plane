// MSW v2 request handlers — shared mock API for tests

import { http, HttpResponse } from 'msw';
import type { Session } from '../types/session.ts';
import type { Machine } from '../lib/types.ts';
import type { Job, Run } from '../types/job.ts';
import type { SessionTemplate } from '../types/template.ts';
import type { Event } from '../types/event.ts';
import {
  buildSession,
  buildMachine,
  buildJob,
  buildRun,
  buildTemplate,
  buildEvent,
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
  http.get('/api/v1/sessions/stats', () => HttpResponse.json({ total: 0, succeeded: 0, failed: 0, period: '24h' })),

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
];
