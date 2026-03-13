import { request } from './client.ts';
import type { User, CreateUserParams, UpdateUserParams } from '../types/user.ts';

export const usersApi = {
  list: () => request<User[]>('/users'),

  get: (id: string) => request<User>(`/users/${encodeURIComponent(id)}`),

  create: (params: CreateUserParams) =>
    request<User>('/users', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  update: (id: string, params: UpdateUserParams) =>
    request<User>(`/users/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),

  delete: (id: string) =>
    request<void>(`/users/${encodeURIComponent(id)}`, { method: 'DELETE' }),
};
