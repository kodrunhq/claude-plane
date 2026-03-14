import { request } from './client.ts';
import type {
  BridgeConnector,
  CreateConnectorParams,
  UpdateConnectorParams,
  BridgeStatus,
} from '../types/connector.ts';

export const bridgeApi = {
  listConnectors: () =>
    request<BridgeConnector[]>('/bridge/connectors'),

  getConnector: (id: string) =>
    request<BridgeConnector>(`/bridge/connectors/${encodeURIComponent(id)}`),

  createConnector: (params: CreateConnectorParams) =>
    request<BridgeConnector>('/bridge/connectors', {
      method: 'POST',
      body: JSON.stringify(params),
    }),

  updateConnector: (id: string, params: UpdateConnectorParams) =>
    request<BridgeConnector>(`/bridge/connectors/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(params),
    }),

  deleteConnector: (id: string) =>
    request<void>(`/bridge/connectors/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  restart: () =>
    request<{ message: string }>('/bridge/restart', { method: 'POST' }),

  status: () =>
    request<BridgeStatus>('/bridge/status'),
};
