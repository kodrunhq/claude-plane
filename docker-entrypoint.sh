#!/bin/sh
set -e

DATA_DIR="/data"
CONFIG_FILE="${DATA_DIR}/server.toml"
CA_DIR="${DATA_DIR}/ca"
CERT_DIR="${DATA_DIR}/server-cert"
DB_FILE="${DATA_DIR}/claude-plane.db"

# First-run initialization: generate certs, config, and admin account.
if [ ! -f "$CONFIG_FILE" ]; then
  echo "==> First run detected. Initializing..."

  mkdir -p "$CA_DIR" "$CERT_DIR"

  echo "==> Generating CA..."
  claude-plane-server ca init --out-dir "$CA_DIR"

  echo "==> Issuing server certificate..."
  claude-plane-server ca issue-server --ca-dir "$CA_DIR" --out-dir "$CERT_DIR"

  JWT_SECRET=$(head -c 32 /dev/urandom | base64)

  printf '[http]\nlisten = "0.0.0.0:8080"\n\n[grpc]\nlisten = "0.0.0.0:9090"\n\n[tls]\nca_cert = "%s/ca.pem"\nserver_cert = "%s/server.pem"\nserver_key = "%s/server-key.pem"\n\n[database]\npath = "%s"\n\n[auth]\njwt_secret = "%s"\nregistration_mode = "closed"\n' \
    "$CA_DIR" "$CERT_DIR" "$CERT_DIR" "$DB_FILE" "$JWT_SECRET" \
    > "$CONFIG_FILE"

  ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
  ADMIN_PASSWORD="${ADMIN_PASSWORD:-changeme123}"

  echo "==> Seeding admin account..."
  echo "$ADMIN_PASSWORD" | claude-plane-server seed-admin \
    --db "$DB_FILE" \
    --email "$ADMIN_EMAIL" \
    --name Admin

  echo "==> Initialization complete."
  echo "    Admin email:    $ADMIN_EMAIL"
  echo "    Admin password: $ADMIN_PASSWORD"
  echo "    Dashboard:      http://localhost:8080"
  echo ""
fi

exec claude-plane-server serve --config "$CONFIG_FILE"
