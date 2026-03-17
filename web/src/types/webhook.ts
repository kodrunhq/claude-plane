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

import { ALL_EVENT_TYPES } from '../constants/eventTypes.ts';

export const WEBHOOK_EVENTS = ALL_EVENT_TYPES;

export type WebhookEvent = (typeof ALL_EVENT_TYPES)[number];
