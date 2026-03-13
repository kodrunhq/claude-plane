import { request } from './client.ts';
import type {
  Webhook,
  WebhookDelivery,
  CreateWebhookParams,
  UpdateWebhookParams,
} from '../types/webhook.ts';

export const webhooksApi = {
  list: () => request<Webhook[]>('/webhooks'),

  get: (id: string) => request<Webhook>(`/webhooks/${encodeURIComponent(id)}`),

  create: (params: CreateWebhookParams) =>
    request<Webhook>('/webhooks', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  update: (id: string, params: UpdateWebhookParams) =>
    request<Webhook>(`/webhooks/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),

  delete: (id: string) =>
    request<void>(`/webhooks/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  listDeliveries: (id: string) =>
    request<WebhookDelivery[]>(`/webhooks/${encodeURIComponent(id)}/deliveries`),
};
