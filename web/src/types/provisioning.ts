// Provisioning types — matches server REST responses

export interface ProvisioningToken {
  token: string;
  machine_id: string;
  target_os: string;
  target_arch: string;
  server_address: string;
  grpc_address: string;
  created_by: string;
  created_at: string;
  expires_at: string;
  redeemed_at?: string;
}

export interface ProvisionResult {
  token: string;
  expires_at: string;
  curl_command: string;
}

export interface CreateProvisionParams {
  machine_id: string;
  os: string;
  arch: string;
}

export type TokenStatus = 'active' | 'expired' | 'redeemed';

export function getTokenStatus(token: ProvisioningToken): TokenStatus {
  if (token.redeemed_at) return 'redeemed';
  if (new Date(token.expires_at) < new Date()) return 'expired';
  return 'active';
}

export const OS_OPTIONS = ['linux', 'darwin'] as const;
export const ARCH_OPTIONS = ['amd64', 'arm64'] as const;
