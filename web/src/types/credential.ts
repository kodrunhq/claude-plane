// Credential system types -- matches server REST responses.
// Note: the secret value is NEVER returned by the API.

export interface Credential {
  credential_id: string;
  user_id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface CreateCredentialParams {
  name: string;
  value: string;
}
