import { useQuery } from '@tanstack/react-query';
import { eventsApi } from '../api/events.ts';
import type { ListEventsParams } from '../types/event.ts';

export function useEvents(params?: ListEventsParams) {
  return useQuery({
    queryKey: ['events', params],
    queryFn: () => eventsApi.list(params),
  });
}
