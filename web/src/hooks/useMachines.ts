import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { machinesApi } from '../api/machines.ts';
import type { UpdateMachineParams } from '../api/machines.ts';

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

export function useUpdateMachine() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: UpdateMachineParams }) =>
      machinesApi.update(id, params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['machines'] }),
  });
}
