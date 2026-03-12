// Machine type -- matches server REST response
export interface Machine {
  machine_id: string;
  hostname: string;
  os: string;
  arch: string;
  status: 'online' | 'offline';
  last_seen_at: string;
  registered_at: string;
  session_count?: number;
}

// WebSocket event message types
export type EventType = 'session.status' | 'session.exit' | 'machine.status' | 'machine.health' | 'run.step.status';
export interface EventMessage {
  type: EventType;
  payload: Record<string, unknown>;
  timestamp: string;
}

// Re-export session types for convenience
export type { Session, CreateSessionRequest, TerminalSize, TerminalStatus } from '../types/session.ts';
