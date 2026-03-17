// Webhook system types -- matches server REST responses

export interface Webhook {
  webhook_id: string;
  name: string;
  url: string;
  events: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  delivery_id: string;
  webhook_id: string;
  event_id: string;
  status: string;
  attempts: number;
  response_code?: number;
  last_error?: string;
  next_retry_at?: string;
  payload?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateWebhookParams {
  name: string;
  url: string;
  secret?: string;
  events: string[];
  enabled: boolean;
}

export interface UpdateWebhookParams {
  name: string;
  url: string;
  secret?: string;
  events: string[];
  enabled: boolean;
}

export const WEBHOOK_EVENTS = [
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

export type WebhookEvent = (typeof WEBHOOK_EVENTS)[number];
