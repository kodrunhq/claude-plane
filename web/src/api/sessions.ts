import { request } from './client.ts';
import type { CreateSessionRequest, Session } from '../types/session.ts';

export const sessionsApi = {
  list: (filters?: Record<string, string>) =>
    request<Session[]>(`/sessions${filters ? `?${new URLSearchParams(filters)}` : ''}`),
  get: (id: string) => request<Session>(`/sessions/${encodeURIComponent(id)}`),
  create: (params: CreateSessionRequest) =>
    request<Session>('/sessions', { method: 'POST', body: JSON.stringify(params) }),
  kill: (id: string) => request<void>(`/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};

// Keep named exports for backward compatibility with Phase 4 code
export const listSessions = sessionsApi.list;
export const getSession = sessionsApi.get;
export const createSession = sessionsApi.create;
export const terminateSession = sessionsApi.kill;
