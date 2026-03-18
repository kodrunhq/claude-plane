import { request } from './client.ts';
import type { LogsResponse, LogStats, LogFilter, SessionStats } from '../types/log.ts';

function buildQuery(filter: LogFilter): string {
  const params = new URLSearchParams();
  if (filter.level) params.set('level', filter.level);
  if (filter.component) params.set('component', filter.component);
  if (filter.source) params.set('source', filter.source);
  if (filter.machine_id) params.set('machine_id', filter.machine_id);
  if (filter.session_id) params.set('session_id', filter.session_id);
  if (filter.since) params.set('since', filter.since);
  if (filter.until) params.set('until', filter.until);
  if (filter.search) params.set('search', filter.search);
  if (filter.limit) params.set('limit', String(filter.limit));
  if (filter.offset) params.set('offset', String(filter.offset));
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

export const logsApi = {
  list: (filter: LogFilter = {}) =>
    request<LogsResponse>(`/logs${buildQuery(filter)}`),

  stats: (since?: string) =>
    request<LogStats>(`/logs/stats${since ? `?since=${encodeURIComponent(since)}` : ''}`),

  sessionStats: (since?: string) =>
    request<SessionStats>(`/sessions/stats${since ? `?since=${encodeURIComponent(since)}` : ''}`),
};
