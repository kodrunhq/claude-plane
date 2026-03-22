// Re-export useBridgeStatus from the main bridge hooks module.
// This file exists as a convenience import for components that only need bridge status.
export { useBridgeStatus } from './useBridge.ts';
export type { BridgeStatus, ConnectorStatus } from '../types/connector.ts';
