export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

const BASE_URL = '/api/v1';

export async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const headers: HeadersInit = {
    ...(options?.body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    ...options?.headers,
  };

  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: 'same-origin',
  });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = await response.json() as { error?: string; message?: string };
      message = body.error ?? body.message ?? message;
    } catch {
      // ignore parse errors, use status-based message
    }
    throw new ApiError(response.status, message);
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return response.json() as Promise<T>;
}
