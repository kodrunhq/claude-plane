import { request } from './client.ts';
import type { SessionTemplate, CreateTemplateParams } from '../types/template.ts';

export const templatesApi = {
  list: (params?: { tag?: string; name?: string }) => {
    const sp = new URLSearchParams();
    if (params?.tag) sp.set('tag', params.tag);
    if (params?.name) sp.set('name', params.name);
    const qs = sp.toString();
    return request<SessionTemplate[]>(`/templates${qs ? `?${qs}` : ''}`);
  },
  get: (id: string) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}`),
  getByName: (name: string) =>
    templatesApi.list({ name }).then(r => r[0] ?? null),
  create: (params: CreateTemplateParams) =>
    request<SessionTemplate>('/templates', { method: 'POST', body: JSON.stringify(params) }),
  update: (id: string, params: CreateTemplateParams) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}`, {
      method: 'PUT', body: JSON.stringify(params),
    }),
  delete: (id: string) =>
    request<void>(`/templates/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  clone: (id: string) =>
    request<SessionTemplate>(`/templates/${encodeURIComponent(id)}/clone`, { method: 'POST' }),
};
