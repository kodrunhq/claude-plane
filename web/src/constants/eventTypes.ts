// Event type constants. These MUST match the backend definitions in
// internal/server/event/event.go. If you add or rename a type there,
// update this file to keep them in sync.

// Run lifecycle events.
export const RUN_CREATED = 'run.created';
export const RUN_STARTED = 'run.started';
export const RUN_COMPLETED = 'run.completed';
export const RUN_FAILED = 'run.failed';
export const RUN_CANCELLED = 'run.cancelled';

// Session lifecycle events.
export const SESSION_STARTED = 'session.started';
export const SESSION_EXITED = 'session.exited';
export const SESSION_TERMINATED = 'session.terminated';

// Machine connectivity events.
export const MACHINE_CONNECTED = 'machine.connected';
export const MACHINE_DISCONNECTED = 'machine.disconnected';

// Trigger events.
export const TRIGGER_CRON = 'trigger.cron';
export const TRIGGER_WEBHOOK = 'trigger.webhook';
export const TRIGGER_JOB_COMPLETED = 'trigger.job_completed';

// Template lifecycle events.
export const TEMPLATE_CREATED = 'template.created';
export const TEMPLATE_UPDATED = 'template.updated';
export const TEMPLATE_DELETED = 'template.deleted';

// Run step events.
export const RUN_STEP_COMPLETED = 'run.step.completed';
export const RUN_STEP_FAILED = 'run.step.failed';

// Job lifecycle events.
export const JOB_CREATED = 'job.created';
export const JOB_UPDATED = 'job.updated';
export const JOB_DELETED = 'job.deleted';

// User lifecycle events.
export const USER_CREATED = 'user.created';
export const USER_DELETED = 'user.deleted';

// Schedule lifecycle events.
export const SCHEDULE_CREATED = 'schedule.created';
export const SCHEDULE_PAUSED = 'schedule.paused';
export const SCHEDULE_RESUMED = 'schedule.resumed';
export const SCHEDULE_DELETED = 'schedule.deleted';

// Credential lifecycle events.
export const CREDENTIAL_CREATED = 'credential.created';
export const CREDENTIAL_DELETED = 'credential.deleted';

// Webhook lifecycle events.
export const WEBHOOK_CREATED = 'webhook.created';
export const WEBHOOK_DELETED = 'webhook.deleted';

/** All known event types. Useful for exhaustive matching or filtering. */
export const ALL_EVENT_TYPES = [
  RUN_CREATED,
  RUN_STARTED,
  RUN_COMPLETED,
  RUN_FAILED,
  RUN_CANCELLED,
  SESSION_STARTED,
  SESSION_EXITED,
  SESSION_TERMINATED,
  MACHINE_CONNECTED,
  MACHINE_DISCONNECTED,
  TRIGGER_CRON,
  TRIGGER_WEBHOOK,
  TRIGGER_JOB_COMPLETED,
  TEMPLATE_CREATED,
  TEMPLATE_UPDATED,
  TEMPLATE_DELETED,
  RUN_STEP_COMPLETED,
  RUN_STEP_FAILED,
  JOB_CREATED,
  JOB_UPDATED,
  JOB_DELETED,
  USER_CREATED,
  USER_DELETED,
  SCHEDULE_CREATED,
  SCHEDULE_PAUSED,
  SCHEDULE_RESUMED,
  SCHEDULE_DELETED,
  CREDENTIAL_CREATED,
  CREDENTIAL_DELETED,
  WEBHOOK_CREATED,
  WEBHOOK_DELETED,
] as const;

/** Grouped event types for UI selectors (webhooks, notifications). */
export const EVENT_GROUPS: { label: string; events: string[] }[] = [
  {
    label: 'Runs',
    events: ['run.created', 'run.started', 'run.completed', 'run.failed', 'run.cancelled'],
  },
  {
    label: 'Tasks',
    events: ['run.step.completed', 'run.step.failed'],
  },
  {
    label: 'Sessions',
    events: ['session.started', 'session.exited', 'session.terminated'],
  },
  {
    label: 'Machines',
    events: ['machine.connected', 'machine.disconnected'],
  },
  {
    label: 'Jobs',
    events: ['job.created', 'job.updated', 'job.deleted'],
  },
  {
    label: 'Templates',
    events: ['template.created', 'template.updated', 'template.deleted'],
  },
  {
    label: 'Schedules',
    events: ['schedule.created', 'schedule.paused', 'schedule.resumed', 'schedule.deleted'],
  },
  {
    label: 'Credentials',
    events: ['credential.created', 'credential.deleted'],
  },
  {
    label: 'Webhooks',
    events: ['webhook.created', 'webhook.deleted'],
  },
  {
    label: 'Users',
    events: ['user.created', 'user.deleted'],
  },
  {
    label: 'Triggers',
    events: ['trigger.cron', 'trigger.webhook', 'trigger.job_completed'],
  },
];
