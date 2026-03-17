import { request } from './client.ts';
import type { User, CreateUserParams, UpdateUserParams } from '../types/user.ts';

export interface ChangePasswordParams {
  current_password: string;
  new_password: string;
}

export interface ResetPasswordParams {
  new_password: string;
}

export interface UpdateProfileParams {
  display_name: string;
}

export const usersApi = {
  list: () => request<User[]>('/users'),

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

  changePassword: (params: ChangePasswordParams) =>
    request<{ status: string }>('/users/me/password', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  resetPassword: (id: string, params: ResetPasswordParams) =>
    request<{ status: string }>(`/users/${encodeURIComponent(id)}/reset-password`, {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  updateProfile: (params: UpdateProfileParams) =>
    request<User>('/users/me', {
      method: 'PUT',
      body: JSON.stringify(params),
    }),
};
