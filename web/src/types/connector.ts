export interface BridgeConnector {
  connector_id: string;
  connector_type: string;
  name: string;
  enabled: boolean;
  config: string;       // JSON string of non-sensitive config
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface CreateConnectorParams {
  connector_type: string;
  name: string;
  config: string;           // JSON of public config fields
  config_secret?: string;   // JSON of sensitive config fields (e.g. bot_token)
}

export interface UpdateConnectorParams {
  connector_type: string;
  name: string;
  config: string;
  config_secret?: string;
}

export interface BridgeStatus {
  restart_requested_at: string | null;
}
