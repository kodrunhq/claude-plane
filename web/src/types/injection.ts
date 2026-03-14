export interface Injection {
  injection_id: string;
  session_id: string;
  user_id: string;
  text_length: number;
  metadata?: string;
  source: string;
  created_at: string;
  delivered_at?: string;
}
