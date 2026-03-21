// Entity factories for tests — sensible defaults with override support

import type { Session } from '../types/session.ts';
import type { Machine } from '../lib/types.ts';
import type { Job, Run, Task, RunTask } from '../types/job.ts';
import type { SessionTemplate } from '../types/template.ts';
import type { Event } from '../types/event.ts';
import type { BrowseResponse } from '../api/machines.ts';

const counters: Record<string, number> = {};

function nextId(prefix: string): string {
  counters[prefix] = (counters[prefix] ?? 0) + 1;
  return `${prefix}-${counters[prefix]}`;
}

const NOW = '2026-01-15T10:00:00Z';

export function buildSession(overrides?: Partial<Session>): Session {
  const id = nextId('sess');
  return {
    session_id: id,
    machine_id: 'machine-1',
    user_id: 'user-1',
    command: 'claude',
    working_dir: '/home/user',
    status: 'running',
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  };
}

export function buildMachine(overrides?: Partial<Machine>): Machine {
  const id = nextId('machine');
  return {
    machine_id: id,
    display_name: `Worker ${id}`,
    status: 'connected',
    max_sessions: 5,
    home_dir: '',
    last_seen_at: NOW,
    created_at: NOW,
    ...overrides,
  };
}

export function buildJob(overrides?: Partial<Job>): Job {
  const id = nextId('job');
  return {
    job_id: id,
    name: `Test Job ${id}`,
    description: 'A test job',
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  };
}

export function buildRun(overrides?: Partial<Run>): Run {
  const id = nextId('run');
  return {
    run_id: id,
    job_id: 'job-1',
    status: 'completed',
    created_at: NOW,
    ...overrides,
  };
}

export function buildTemplate(overrides?: Partial<SessionTemplate>): SessionTemplate {
  const id = nextId('tmpl');
  return {
    template_id: id,
    user_id: 'user-1',
    name: `Template ${id}`,
    terminal_rows: 24,
    terminal_cols: 80,
    timeout_seconds: 300,
    created_at: NOW,
    updated_at: NOW,
    ...overrides,
  };
}

export function buildEvent(overrides?: Partial<Event>): Event {
  const id = nextId('evt');
  return {
    event_id: id,
    event_type: 'session.started',
    timestamp: NOW,
    source: 'test',
    payload: {},
    ...overrides,
  };
}

export function buildTask(overrides?: Partial<Task>): Task {
  const id = nextId('step');
  return {
    step_id: id,
    job_id: 'job-1',
    name: `Task ${id}`,
    prompt: 'Do something useful',
    machine_id: 'machine-1',
    working_dir: '/home/user/project',
    command: 'claude',
    args: '',
    sort_order: 0,
    task_type: 'claude',
    ...overrides,
  };
}

export function buildRunTask(overrides?: Partial<RunTask>): RunTask {
  const id = nextId('rstep');
  return {
    run_step_id: id,
    run_id: 'run-1',
    step_id: 'step-1',
    status: 'completed',
    attempt: 1,
    ...overrides,
  };
}

export function buildBrowseResponse(overrides?: Partial<BrowseResponse>): BrowseResponse {
  return {
    path: '/home/user',
    entries: [
      { name: 'Documents', type: 'dir' },
      { name: 'projects', type: 'dir' },
      { name: '.bashrc', type: 'file' },
    ],
    parent: '/',
    ...overrides,
  };
}
