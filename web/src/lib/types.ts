// Machine type -- matches Go store.Machine struct
export interface Machine {
  machine_id: string;
  display_name: string;
  status: 'connected' | 'disconnected';
  max_sessions: number;
  last_health: string;
  last_seen_at: string;
  cert_expires: string;
  created_at: string;
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
