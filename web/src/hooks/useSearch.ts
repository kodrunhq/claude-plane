import { useQuery } from '@tanstack/react-query';
import { searchApi } from '../api/search.ts';

export function useSessionSearch(query: string, limit = 50) {
  return useQuery({
    queryKey: ['search', 'sessions', query, limit],
    queryFn: () => searchApi.sessions(query, limit),
    enabled: query.length >= 2,
  });
}
