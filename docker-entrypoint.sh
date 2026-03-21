#!/bin/sh
set -e

DATA_DIR="/data"
CONFIG_FILE="${DATA_DIR}/server.toml"
BRIDGE_CONFIG="${DATA_DIR}/bridge.toml"
CA_DIR="${DATA_DIR}/ca"
CERT_DIR="${DATA_DIR}/server-cert"
DB_FILE="${DATA_DIR}/claude-plane.db"

# First-run initialization: generate certs, config, admin account, and bridge API key.
if [ ! -f "$CONFIG_FILE" ]; then
  echo "==> First run detected. Initializing..."

  mkdir -p "$CA_DIR" "$CERT_DIR"

  # External address for provisioning — agents need a reachable address, not 0.0.0.0.
  # Defaults to hostname if not set via SERVER_URL env var.
  EXTERNAL_HOST="${SERVER_URL:-http://$(hostname):4200}"

  # Extract hostname/IP from SERVER_URL for TLS certificate SANs.
  CERT_HOST=$(echo "$EXTERNAL_HOST" | sed 's|https\?://||;s|:[0-9]*$||')

  echo "==> Generating CA..."
  claude-plane-server ca init --out-dir "$CA_DIR"

  echo "==> Issuing server certificate (SANs: localhost, ${CERT_HOST})..."
  claude-plane-server ca issue-server --ca-dir "$CA_DIR" --out-dir "$CERT_DIR" --hostnames "$CERT_HOST"

  JWT_SECRET=$(head -c 32 /dev/urandom | base64)

  printf '[http]\nlisten = "0.0.0.0:4200"\n\n[grpc]\nlisten = "0.0.0.0:4201"\n\n[tls]\nca_cert = "%s/ca.pem"\nserver_cert = "%s/server.pem"\nserver_key = "%s/server-key.pem"\n\n[database]\npath = "%s"\n\n[auth]\njwt_secret = "%s"\nregistration_mode = "closed"\n\n[ca]\ndir = "%s"\n\n[provision]\nexternal_http_address = "%s"\nexternal_grpc_address = "%s"\n' \
    "$CA_DIR" "$CERT_DIR" "$CERT_DIR" "$DB_FILE" "$JWT_SECRET" "$CA_DIR" \
    "$EXTERNAL_HOST" "$(echo "$EXTERNAL_HOST" | sed 's|https\?://||;s|:[0-9]*$||'):4201" \
    > "$CONFIG_FILE"

  ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
  ADMIN_PASSWORD="${ADMIN_PASSWORD:-changeme123}"

  echo "==> Seeding admin account..."
  echo "$ADMIN_PASSWORD" | claude-plane-server seed-admin \
    --db "$DB_FILE" \
    --email "$ADMIN_EMAIL" \
    --name Admin

  # Generate an API key for the bridge (runs as the admin user).
  echo "==> Creating bridge API key..."
  BRIDGE_API_KEY=$(claude-plane-server create-api-key \
    --db "$DB_FILE" \
    --email "$ADMIN_EMAIL" \
    --name "bridge-internal" \
    --jwt-secret "$JWT_SECRET")

  # Write bridge config pointing to the local server.
  printf '[claude_plane]\napi_url = "http://localhost:4200"\napi_key = "%s"\n\n[state]\npath = "%s/bridge-state.json"\n\n[health]\naddress = ":8081"\n' \
    "$BRIDGE_API_KEY" "$DATA_DIR" \
    > "$BRIDGE_CONFIG"

  echo "==> Initialization complete."
  echo "    Admin email:    $ADMIN_EMAIL"
  echo "    Admin password: $ADMIN_PASSWORD"
  echo "    Dashboard:      http://localhost:4200"
  echo "    Bridge:         auto-configured (internal API key)"
  if [ "$ADMIN_PASSWORD" = "changeme123" ]; then
    echo ""
    echo "=============================================="
    echo "  WARNING: Using default admin password!"
    echo "  This is insecure for production use."
    echo "  Set ADMIN_PASSWORD env var to change it."
    echo "=============================================="
    echo ""
  fi
  echo ""
fi

# Start bridge in the background if config exists.
if [ -f "$BRIDGE_CONFIG" ]; then
  echo "==> Starting bridge..."
  claude-plane-bridge serve --config "$BRIDGE_CONFIG" &
fi

exec claude-plane-server serve --config "$CONFIG_FILE"
