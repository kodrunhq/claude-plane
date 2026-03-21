import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { bridgeApi } from '../api/bridge.ts';
import type { CreateConnectorParams, UpdateConnectorParams } from '../types/connector.ts';

export function useConnectors() {
  return useQuery({
    queryKey: ['connectors'],
    queryFn: () => bridgeApi.listConnectors(),
  });
}

export function useConnector(id: string | undefined) {
  return useQuery({
    queryKey: ['connectors', id],
    queryFn: () => bridgeApi.getConnector(id!),
    enabled: !!id,
  });
}

export function useCreateConnector() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: CreateConnectorParams) => bridgeApi.createConnector(params),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connectors'] }),
  });
}

export function useUpdateConnector() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, params }: { id: string; params: UpdateConnectorParams }) =>
      bridgeApi.updateConnector(id, params),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['connectors', id] });
      qc.invalidateQueries({ queryKey: ['connectors'] });
    },
  });
}

export function useDeleteConnector() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => bridgeApi.deleteConnector(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['connectors'] }),
  });
}

export function useRestartBridge() {
  return useMutation({
    mutationFn: () => bridgeApi.restart(),
  });
}

export function useBridgeStatus() {
  return useQuery({
    queryKey: ['bridge-status'],
    queryFn: () => bridgeApi.status(),
    refetchInterval: 15_000,
  });
}
