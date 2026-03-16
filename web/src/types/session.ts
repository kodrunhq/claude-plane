export interface TerminalSize {
  rows: number;
  cols: number;
}

export interface Session {
  session_id: string;
  machine_id: string;
  user_id: string;
  command: string;
  working_dir: string;
  status: 'created' | 'running' | 'completed' | 'failed' | 'terminated';
  created_at: string;
  updated_at: string;
  template_id?: string;
  model?: string;
  skip_permissions?: string;
  env_vars?: string;
  args?: string;
  initial_prompt?: string;
}

export interface CreateSessionRequest {
  machine_id: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  terminal_size?: TerminalSize;
  template_id?: string;
  template_name?: string;
  variables?: Record<string, string>;
  model?: string;
  skip_permissions?: boolean;
}

export type TerminalStatus = 'connecting' | 'replaying' | 'live' | 'disconnected';
