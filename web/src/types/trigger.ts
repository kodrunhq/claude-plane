export interface JobTrigger {
  trigger_id: string;
  job_id: string;
  event_type: string;
  filter: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateTriggerParams {
  event_type: string;
  filter: string;
}

export interface UpdateTriggerParams {
  event_type: string;
  filter: string;
}

export const KNOWN_EVENT_TYPES = [
  'run.created',
  'run.started',
  'run.completed',
  'run.failed',
  'run.cancelled',
  'run.step.completed',
  'run.step.failed',
  'session.started',
  'session.exited',
  'session.terminated',
  'machine.connected',
  'machine.disconnected',
  'trigger.cron',
  'trigger.webhook',
  'trigger.job_completed',
  'template.created',
  'template.updated',
  'template.deleted',
  'job.created',
  'job.updated',
  'job.deleted',
  'user.created',
  'user.deleted',
  'schedule.created',
  'schedule.paused',
  'schedule.resumed',
  'schedule.deleted',
  'credential.created',
  'credential.deleted',
  'webhook.created',
  'webhook.deleted',
] as const;
