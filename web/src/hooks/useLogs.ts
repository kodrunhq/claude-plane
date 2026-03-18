import { useQuery } from '@tanstack/react-query';
import { logsApi } from '../api/logs.ts';
import type { LogFilter } from '../types/log.ts';

export function useLogs(filter: LogFilter) {
  return useQuery({
    queryKey: ['logs', filter],
    queryFn: () => logsApi.list(filter),
    refetchInterval: 30_000,
  });
}

export function useLogStats(since?: string) {
  return useQuery({
    queryKey: ['log-stats', since],
    queryFn: () => logsApi.stats(since),
    refetchInterval: 30_000,
  });
}

export function useSessionStats(since?: string) {
  return useQuery({
    queryKey: ['session-stats', since],
    queryFn: () => logsApi.sessionStats(since),
    refetchInterval: 30_000,
  });
}
