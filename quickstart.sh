#!/usr/bin/env bash
set -euo pipefail

# claude-plane quickstart — single-machine setup in one command.
# Usage: ./quickstart.sh [admin-email] [admin-password]

ADMIN_EMAIL="${1:-admin@localhost}"
ADMIN_PASSWORD="${2:-}"
DATA_DIR="${CLAUDE_PLANE_DATA:-./data}"
HTTP_PORT="${CLAUDE_PLANE_HTTP_PORT:-8080}"
GRPC_PORT="${CLAUDE_PLANE_GRPC_PORT:-9090}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}==>${NC} $*"; }
ok()    { echo -e "${GREEN}==>${NC} $*"; }
err()   { echo -e "${RED}ERROR:${NC} $*" >&2; exit 1; }

check_port_in_use() {
  local port="$1"

  if command -v lsof >/dev/null 2>&1; then
    lsof -i :"$port" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi

  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :$port" >/dev/null 2>&1
    return $?
  fi

  if command -v netstat >/dev/null 2>&1; then
    netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$port\$"
    return $?
  fi

  info "No suitable tool (lsof/ss/netstat) found to check if port $port is in use; skipping port availability check."
  return 1
}

# ── Pre-flight checks ──────────────────────────────────────────────

command -v go >/dev/null 2>&1 || err "Go is not installed. Install it from https://go.dev/dl/"

# Build frontend (must happen before server so it gets embedded via go:embed)
if [ -d "web" ]; then
  info "Building frontend..."
  (cd web && npm install --silent && npm run build --silent) || err "Frontend build failed"
  ok "Frontend built"
fi

# Build binaries
info "Building server..."
go build -o ./claude-plane-server ./cmd/server || err "Server build failed"
ok "Server built"

info "Building agent..."
go build -o ./claude-plane-agent ./cmd/agent || err "Agent build failed"
ok "Agent built"

# Check if ports are already in use
if check_port_in_use "$HTTP_PORT"; then
  err "Port $HTTP_PORT is already in use. Set CLAUDE_PLANE_HTTP_PORT to use a different port."
fi
if check_port_in_use "$GRPC_PORT"; then
  err "Port $GRPC_PORT is already in use. Set CLAUDE_PLANE_GRPC_PORT to use a different port."
fi

# ── Setup data directory ────────────────────────────────────────────

mkdir -p "$DATA_DIR"/{ca,server-cert,agent-cert}

# ── Generate certificates (skip if CA already exists) ───────────────

if [ ! -f "$DATA_DIR/ca/ca.pem" ]; then
  info "Initializing certificate authority..."
  ./claude-plane-server ca init --out-dir "$DATA_DIR/ca"
  ok "CA initialized"
else
  info "CA already exists, skipping"
fi

if [ ! -f "$DATA_DIR/server-cert/server.pem" ]; then
  info "Issuing server certificate..."
  ./claude-plane-server ca issue-server \
    --ca-dir "$DATA_DIR/ca" \
    --out-dir "$DATA_DIR/server-cert"
  ok "Server certificate issued"
fi

if [ ! -f "$DATA_DIR/agent-cert/agent.pem" ]; then
  info "Issuing agent certificate..."
  ./claude-plane-server ca issue-agent \
    --ca-dir "$DATA_DIR/ca" \
    --out-dir "$DATA_DIR/agent-cert" \
    --machine-id agent-local
  ok "Agent certificate issued"
fi

# ── Generate configs (skip if they exist) ───────────────────────────

JWT_SECRET_FILE="$DATA_DIR/.jwt_secret"
if [ ! -f "$JWT_SECRET_FILE" ]; then
  openssl rand -hex 32 > "$JWT_SECRET_FILE"
  chmod 600 "$JWT_SECRET_FILE"
fi
JWT_SECRET=$(cat "$JWT_SECRET_FILE")

SERVER_TOML="$DATA_DIR/server.toml"
if [ ! -f "$SERVER_TOML" ]; then
  cat > "$SERVER_TOML" <<EOF
[http]
listen = "127.0.0.1:${HTTP_PORT}"

[grpc]
listen = "127.0.0.1:${GRPC_PORT}"

[tls]
ca_cert     = "${DATA_DIR}/ca/ca.pem"
server_cert = "${DATA_DIR}/server-cert/server.pem"
server_key  = "${DATA_DIR}/server-cert/server-key.pem"

[database]
path = "${DATA_DIR}/claude-plane.db"

[auth]
jwt_secret        = "${JWT_SECRET}"
registration_mode = "open"

[shutdown]
timeout = "30s"
EOF
  ok "Server config written"
fi

AGENT_TOML="$DATA_DIR/agent.toml"
if [ ! -f "$AGENT_TOML" ]; then
  cat > "$AGENT_TOML" <<EOF
[server]
address = "127.0.0.1:${GRPC_PORT}"

[tls]
ca_cert    = "${DATA_DIR}/ca/ca.pem"
agent_cert = "${DATA_DIR}/agent-cert/agent.pem"
agent_key  = "${DATA_DIR}/agent-cert/agent-key.pem"

[agent]
machine_id   = "local"
data_dir     = "${DATA_DIR}/agent-data"
max_sessions = 5

[shutdown]
timeout = "15s"
EOF
  ok "Agent config written"
fi

mkdir -p "$DATA_DIR/agent-data"

# ── Seed admin account (skip if DB exists) ──────────────────────────

if [ ! -f "$DATA_DIR/claude-plane.db" ]; then
  if [ -z "$ADMIN_PASSWORD" ]; then
    ADMIN_PASSWORD="claude-plane-$(openssl rand -hex 4)"
  fi

  echo "$ADMIN_PASSWORD" | ./claude-plane-server seed-admin \
    --db "$DATA_DIR/claude-plane.db" \
    --email "$ADMIN_EMAIL" \
    --name Admin

  ok "Admin account created"
  echo ""
  echo -e "  ${BOLD}Email:${NC}    $ADMIN_EMAIL"
  echo -e "  ${BOLD}Password:${NC} $ADMIN_PASSWORD"
  echo ""
fi

# ── Start server and agent ──────────────────────────────────────────

info "Starting server..."
./claude-plane-server serve --config "$SERVER_TOML" &
SERVER_PID=$!

info "Starting agent..."
./claude-plane-agent run --config "$AGENT_TOML" &
AGENT_PID=$!

# Install trap immediately after starting processes so early failures
# (e.g. health check) still clean up backgrounded processes.
cleanup() {
  echo ""
  info "Shutting down..."
  kill "$SERVER_PID" "$AGENT_PID" 2>/dev/null || true
  wait "$SERVER_PID" "$AGENT_PID" 2>/dev/null || true
  ok "Stopped"
}
trap cleanup INT TERM EXIT

sleep 2

# Verify server is responding
if curl -sf http://127.0.0.1:"$HTTP_PORT"/ >/dev/null 2>&1; then
  ok "Server is running"
else
  err "Server failed to start. Check logs above."
fi

# ── Done ────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}${BOLD}claude-plane is running!${NC}"
echo ""
echo -e "  ${BOLD}Dashboard:${NC}  http://127.0.0.1:${HTTP_PORT}"
echo -e "  ${BOLD}Server PID:${NC} ${SERVER_PID}"
echo -e "  ${BOLD}Agent PID:${NC}  ${AGENT_PID}"
echo ""
echo "Press Ctrl+C to stop."
echo ""

# Disable EXIT trap for normal wait — cleanup runs on signal only.
trap - EXIT
wait "$SERVER_PID" "$AGENT_PID"
