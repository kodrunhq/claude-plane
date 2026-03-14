import { request } from './client.ts';
import type { UserPreferences } from '../types/preferences.ts';

export const preferencesApi = {
  get: () => request<UserPreferences>('/users/me/preferences'),

  update: (prefs: UserPreferences) =>
    request<UserPreferences>('/users/me/preferences', {
      method: 'PUT',
      body: JSON.stringify(prefs),
    }),

  patch: (prefs: Partial<UserPreferences>) =>
    request<UserPreferences>('/users/me/preferences', {
      method: 'PATCH',
      body: JSON.stringify(prefs),
    }),
};
