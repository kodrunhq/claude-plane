#!/usr/bin/env bash
set -euo pipefail

# claude-plane Docker quickstart
# Generates certs, config, seeds admin, and starts the server in Docker.
#
# Usage:
#   ./docker-quickstart.sh
#
# Environment variables (optional):
#   CLAUDE_PLANE_ADMIN_EMAIL     Admin email (default: admin@localhost)
#   CLAUDE_PLANE_ADMIN_PASSWORD  Admin password (default: generated)
#   CLAUDE_PLANE_IMAGE           Docker image (default: jurel89/claude-plane:latest)
#   CLAUDE_PLANE_DATA_DIR        Data directory (default: ./claude-plane-data)

IMAGE="${CLAUDE_PLANE_IMAGE:-jurel89/claude-plane:latest}"
DATA_DIR="${CLAUDE_PLANE_DATA_DIR:-./claude-plane-data}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}==>${NC} $*"; }
ok()    { echo -e "${GREEN}==>${NC} $*"; }
err()   { echo -e "${RED}==> ERROR:${NC} $*" >&2; }
fatal() { err "$@"; exit 1; }

# Ensure docker is available
command -v docker &>/dev/null || fatal "Docker is not installed. Install it from https://docs.docker.com/get-docker/"

# Create data directories
CA_DIR="${DATA_DIR}/ca"
CERT_DIR="${DATA_DIR}/server-cert"
CONFIG_FILE="${DATA_DIR}/server.toml"
DB_DIR="${DATA_DIR}/db"

mkdir -p "$CA_DIR" "$CERT_DIR" "$DB_DIR"

# Skip setup if config already exists (re-run just starts the container)
if [ -f "$CONFIG_FILE" ]; then
  info "Config already exists at ${CONFIG_FILE}, starting server..."
else
  info "Pulling ${IMAGE}..."
  docker pull "$IMAGE"

  info "Generating CA..."
  docker run --rm \
    -v "$(cd "$CA_DIR" && pwd):/out" \
    --entrypoint claude-plane-server \
    "$IMAGE" ca init --out-dir /out

  info "Issuing server certificate..."
  docker run --rm \
    -v "$(cd "$CA_DIR" && pwd):/ca:ro" \
    -v "$(cd "$CERT_DIR" && pwd):/out" \
    --entrypoint claude-plane-server \
    "$IMAGE" ca issue-server --ca-dir /ca --out-dir /out

  info "Generating JWT secret..."
  JWT_SECRET=$(head -c 32 /dev/urandom | base64)

  info "Writing server config..."
  cat > "$CONFIG_FILE" <<TOML
[http]
listen = "0.0.0.0:8080"

[grpc]
listen = "0.0.0.0:9090"

[tls]
ca_cert     = "/etc/claude-plane/ca/ca.pem"
server_cert = "/etc/claude-plane/server-cert/server.pem"
server_key  = "/etc/claude-plane/server-cert/server-key.pem"

[database]
path = "/data/claude-plane.db"

[auth]
jwt_secret        = "${JWT_SECRET}"
registration_mode = "closed"
TOML

  # Admin credentials
  ADMIN_EMAIL="${CLAUDE_PLANE_ADMIN_EMAIL:-}"
  ADMIN_PASSWORD="${CLAUDE_PLANE_ADMIN_PASSWORD:-}"

  if [ -z "$ADMIN_EMAIL" ]; then
    printf "${BOLD}Admin email [admin@localhost]:${NC} "
    read -r ADMIN_EMAIL
    ADMIN_EMAIL="${ADMIN_EMAIL:-admin@localhost}"
  fi

  if [ -z "$ADMIN_PASSWORD" ]; then
    printf "${BOLD}Admin password (leave blank to generate):${NC} "
    read -rs ADMIN_PASSWORD
    echo
    if [ -z "$ADMIN_PASSWORD" ]; then
      ADMIN_PASSWORD=$(head -c 16 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 20)
      info "Generated password: ${ADMIN_PASSWORD}"
    fi
  fi

  info "Seeding admin account..."
  echo "$ADMIN_PASSWORD" | docker run --rm -i \
    -v "$(cd "$DB_DIR" && pwd):/data" \
    --entrypoint claude-plane-server \
    "$IMAGE" seed-admin --db /data/claude-plane.db --email "$ADMIN_EMAIL" --name Admin
fi

# Stop existing container if running
docker rm -f claude-plane 2>/dev/null || true

info "Starting claude-plane server..."
docker run -d \
  --name claude-plane \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 9090:9090 \
  -v "$(cd "$DATA_DIR" && pwd)/server.toml:/etc/claude-plane/server.toml:ro" \
  -v "$(cd "$CA_DIR" && pwd):/etc/claude-plane/ca:ro" \
  -v "$(cd "$CERT_DIR" && pwd):/etc/claude-plane/server-cert:ro" \
  -v "$(cd "$DB_DIR" && pwd):/data" \
  "$IMAGE"

# Wait for server to be ready
info "Waiting for server..."
READY=false
for _ in $(seq 1 30); do
  if curl -sf http://localhost:8080/api/v1/health &>/dev/null ||
     curl -sf http://localhost:8080/ &>/dev/null; then
    READY=true
    break
  fi
  sleep 1
done

if [ "$READY" != "true" ]; then
  echo ""
  err "Server did not become ready after 30 seconds. Check logs:"
  echo "  docker logs claude-plane"
  exit 1
fi

echo ""
ok "claude-plane is running!"
echo -e "  Dashboard: ${BOLD}http://localhost:8080${NC}"
echo -e "  gRPC:      ${BOLD}localhost:9090${NC} (for agent connections)"
echo -e "  Admin:     ${BOLD}${ADMIN_EMAIL:-$(grep -oP 'email \K\S+' /dev/null 2>/dev/null || echo 'see initial setup')}${NC}"
echo -e "  Data:      ${BOLD}${DATA_DIR}${NC}"
echo ""
echo "  Stop:      docker stop claude-plane"
echo "  Start:     docker start claude-plane"
echo "  Logs:      docker logs -f claude-plane"
echo "  Remove:    docker rm -f claude-plane"
echo ""
