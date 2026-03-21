export interface NotificationChannel {
  channel_id: string;
  channel_type: 'email' | 'telegram';
  name: string;
  config: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
  connector_id?: string;
}

export interface NotificationSubscription {
  channel_id: string;
  event_type: string;
}

export interface CreateChannelParams {
  channel_type: 'email' | 'telegram';
  name: string;
  config: string;
}

export interface UpdateChannelParams {
  name: string;
  config: string;
  enabled: boolean;
}

export interface TestChannelResult {
  success: boolean;
  error?: string;
}

export interface SMTPConfig {
  host: string;
  port: number;
  username: string;
  password: string;
  from: string;
  to: string;
  tls: boolean;
}

export interface TelegramConfig {
  bot_token: string;
  chat_id: string;
  topic_id?: number;
}
