export interface APIKey {
  key_id: string;
  user_id: string;
  name: string;
  scopes?: string[];
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
}

export interface CreateAPIKeyParams {
  name: string;
  scopes?: string[];
  expires_at?: string;
}

export interface CreateAPIKeyResponse {
  key: string; // plaintext, shown once
  key_id: string;
  name: string;
  scopes?: string[];
  expires_at?: string;
  created_at: string;
}
