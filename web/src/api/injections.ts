import { request } from './client.ts';
import type { Injection } from '../types/injection.ts';

export const injectionsApi = {
  inject: (
    sessionId: string,
    params: { text: string; raw?: boolean; delay_ms?: number; metadata?: Record<string, unknown> },
  ) =>
    request<{ injection_id: string; queued_at: string }>(
      `/sessions/${encodeURIComponent(sessionId)}/inject`,
      { method: 'POST', body: JSON.stringify(params) },
    ),
  list: (sessionId: string) =>
    request<Injection[]>(`/sessions/${encodeURIComponent(sessionId)}/injections`),
};
