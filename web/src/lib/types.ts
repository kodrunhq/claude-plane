export interface MachineHealth {
  cpu_cores: number;
  memory_total_mb: number;
  memory_used_mb: number;
  active_sessions: number;
  max_sessions: number;
}

// Machine type -- matches Go machineResponse struct
export interface Machine {
  machine_id: string;
  display_name: string;
  status: 'connected' | 'disconnected';
  max_sessions: number;
  last_health: string;
  last_seen_at: string;
  cert_expires: string;
  created_at: string;
  health?: MachineHealth;
}

// Re-export session types for convenience
export type { Session, CreateSessionRequest, TerminalSize, TerminalStatus } from '../types/session.ts';
