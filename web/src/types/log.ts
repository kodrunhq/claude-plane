export interface LogEntry {
  id: number;
  timestamp: string;
  level: 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';
  component: string;
  message: string;
  machine_id?: string;
  session_id?: string;
  error?: string;
  metadata?: string;
  source: 'server' | 'agent';
}

export interface LogFilter {
  level?: string;
  component?: string;
  source?: string;
  machine_id?: string;
  session_id?: string;
  since?: string;
  until?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

export interface LogsResponse {
  logs: LogEntry[];
  total: number;
}

export interface LogStats {
  by_level: Record<string, number>;
  by_component: Record<string, number>;
  total: number;
}

export interface SessionStats {
  total: number;
  succeeded: number;
  failed: number;
  since: string;
}
