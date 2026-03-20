export interface MachineOverride {
  working_dir: string;
  model: string;
  env_vars: Record<string, string>;
  max_concurrent_sessions: number;
}

export interface NotificationPrefs {
  events: string[];
}

export interface UIPrefs {
  theme: 'light' | 'dark' | 'system';
  terminal_font_size: number;
  auto_attach_session: boolean;
  command_center_cards: string[];
}

export interface UserPreferences {
  skip_permissions?: boolean;
  default_session_timeout?: number;
  default_step_timeout?: number;
  default_step_delay?: number;
  default_env_vars?: Record<string, string>;
  session_stale_timeout?: number;
  notifications?: NotificationPrefs;
  ui?: UIPrefs;
  machine_overrides?: Record<string, MachineOverride>;
}
