import type { CreateSessionRequest, Session } from '../types/session.ts';

function getAuthToken(): string {
  return localStorage.getItem('token') ?? '';
}

function authHeaders(): HeadersInit {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${getAuthToken()}`,
  };
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await response.text().catch(() => '');
    throw new Error(`API error ${response.status}: ${body}`);
  }
  return response.json() as Promise<T>;
}

export async function createSession(req: CreateSessionRequest): Promise<Session> {
  const response = await fetch('/api/v1/sessions', {
    method: 'POST',
    headers: authHeaders(),
    body: JSON.stringify(req),
  });
  return handleResponse<Session>(response);
}

export async function listSessions(): Promise<Session[]> {
  const response = await fetch('/api/v1/sessions', {
    headers: authHeaders(),
  });
  return handleResponse<Session[]>(response);
}

export async function getSession(id: string): Promise<Session> {
  const response = await fetch(`/api/v1/sessions/${encodeURIComponent(id)}`, {
    headers: authHeaders(),
  });
  return handleResponse<Session>(response);
}

export async function terminateSession(id: string): Promise<void> {
  const response = await fetch(`/api/v1/sessions/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authHeaders(),
  });
  if (!response.ok) {
    const body = await response.text().catch(() => '');
    throw new Error(`API error ${response.status}: ${body}`);
  }
}
