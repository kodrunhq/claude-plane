import { useQuery } from '@tanstack/react-query';
import { machinesApi } from '../api/machines.ts';

export function useMachines() {
  return useQuery({
    queryKey: ['machines'],
    queryFn: machinesApi.list,
    refetchInterval: 30_000,
  });
}

export function useMachine(id: string) {
  return useQuery({
    queryKey: ['machines', id],
    queryFn: () => machinesApi.get(id),
    enabled: !!id,
  });
}
