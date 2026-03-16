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
] as const;
