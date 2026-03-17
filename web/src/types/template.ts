export interface SessionTemplate {
  template_id: string;
  user_id: string;
  name: string;
  description?: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  env_vars?: Record<string, string>;
  initial_prompt?: string;
  terminal_rows: number;
  terminal_cols: number;
  tags?: string[];
  machine_id?: string;
  timeout_seconds: number;
  created_at: string;
  updated_at: string;
}

export interface CreateTemplateParams {
  name: string;
  description?: string;
  command?: string;
  args?: string[];
  working_dir?: string;
  env_vars?: Record<string, string>;
  initial_prompt?: string;
  terminal_rows?: number;
  terminal_cols?: number;
  tags?: string[];
  timeout_seconds?: number;
}
