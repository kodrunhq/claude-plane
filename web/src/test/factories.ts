// Entity factories for tests — sensible defaults with override support

import type { Session } from '../types/session.ts';
import type { Machine } from '../lib/types.ts';
import type { Job, Run } from '../types/job.ts';
import type { SessionTemplate } from '../types/template.ts';
import type { Event } from '../types/event.ts';

const counters: Record<string, number> = {};

function nextId(prefix: string): string {
  counters[prefix] = (counters[prefix] ?? 0) + 1;
  return `${prefix}-${counters[prefix]}`;
}

/** Reset all ID counters (call in afterEach if deterministic IDs are needed) */
export function resetFactories(): void {
  for (const key of Object.keys(counters)) {
    delete counters[key];
  }
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
    last_health: NOW,
    last_seen_at: NOW,
    cert_expires: '2027-01-15T10:00:00Z',
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
