import { request } from './client.ts';
import type { Event, ListEventsParams } from '../types/event.ts';

export const eventsApi = {
  list: (params?: ListEventsParams): Promise<Event[]> => {
    const sp = new URLSearchParams();
    if (params?.type) sp.set('type', params.type);
    if (params?.since) sp.set('since', params.since);
    if (params?.limit !== undefined) sp.set('limit', String(params.limit));
    if (params?.offset !== undefined) sp.set('offset', String(params.offset));
    const qs = sp.toString();
    return request<Event[]>(`/events${qs ? `?${qs}` : ''}`);
  },
};
