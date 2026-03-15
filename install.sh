#!/usr/bin/env bash
set -euo pipefail

# claude-plane universal installer
# Usage:
#   curl -fsSL .../install.sh | bash              # install server
#   curl -fsSL .../install.sh | bash -s -- agent  # install agent
#   curl -fsSL .../install.sh | bash -s -- bridge # install bridge
#   curl -fsSL .../install.sh | bash -s -- quickstart  # install + setup

REPO="kodrunhq/claude-plane"
COMPONENT="${1:-server}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${CYAN}==>${NC} $*"; }
ok()    { echo -e "${GREEN}==>${NC} $*"; }
err()   { echo -e "${RED}==> ERROR:${NC} $*" >&2; }
fatal() { err "$@"; exit 1; }

# --- Platform detection ---
detect_platform() {
  local os arch

  case "$(uname -s)" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *)      fatal "Unsupported OS: $(uname -s). Only Linux and macOS are supported." ;;
  esac

  case "$(uname -m)" in
    x86_64)       arch="amd64" ;;
    amd64)        arch="amd64" ;;
    aarch64)      arch="arm64" ;;
    arm64)        arch="arm64" ;;
    *)            fatal "Unsupported architecture: $(uname -m). Only amd64 and arm64 are supported." ;;
  esac

  OS="$os"
  ARCH="$arch"
}

# --- Version resolution ---
resolve_version() {
  if [ -n "${CLAUDE_PLANE_VERSION:-}" ]; then
    VERSION="$CLAUDE_PLANE_VERSION"
    info "Using pinned version: $VERSION"
    return
  fi

  info "Fetching latest release..."
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  local response
  response=$(curl -fsSL "$api_url" 2>/dev/null) || fatal "Failed to fetch latest release from GitHub API"

  VERSION=$(echo "$response" | grep -o '"tag_name": *"[^"]*"' | head -1 | sed 's/.*"tag_name": *"//;s/"//')
  [ -n "$VERSION" ] || fatal "Could not parse version from GitHub API response"

  info "Latest version: $VERSION"
}

# --- Download and verify ---
download_and_verify() {
  local component="$1"
  local binary_name="claude-plane-${component}"
  local version_no_v="${VERSION#v}"

  # Determine archive format: .zip for macOS, .tar.gz for Linux
  local ext="tar.gz"
  if [ "$OS" = "darwin" ]; then
    ext="zip"
  fi

  local archive_name="${binary_name}_${version_no_v}_${OS}_${ARCH}.${ext}"
  local download_url="https://github.com/${REPO}/releases/download/${VERSION}/${archive_name}"
  local checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

  local tmp_dir
  tmp_dir=$(mktemp -d)
  trap "rm -rf '$tmp_dir'" EXIT

  info "Downloading ${archive_name}..."
  curl -fsSL -o "${tmp_dir}/${archive_name}" "$download_url" || fatal "Download failed: ${download_url}"

  info "Downloading checksums..."
  curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksums_url" || fatal "Checksums download failed"

  info "Verifying checksum..."
  local expected_checksum
  expected_checksum=$(grep "${archive_name}" "${tmp_dir}/checksums.txt" | awk '{print $1}')
  [ -n "$expected_checksum" ] || fatal "Archive not found in checksums.txt"

  local actual_checksum
  if command -v sha256sum &>/dev/null; then
    actual_checksum=$(sha256sum "${tmp_dir}/${archive_name}" | awk '{print $1}')
  elif command -v shasum &>/dev/null; then
    actual_checksum=$(shasum -a 256 "${tmp_dir}/${archive_name}" | awk '{print $1}')
  else
    fatal "No sha256sum or shasum found — cannot verify checksum"
  fi

  if [ "$expected_checksum" != "$actual_checksum" ]; then
    fatal "Checksum mismatch!\n  Expected: ${expected_checksum}\n  Got:      ${actual_checksum}"
  fi
  ok "Checksum verified"

  info "Extracting..."
  if [ "$ext" = "zip" ]; then
    command -v unzip &>/dev/null || fatal "unzip is required on macOS but not found"
    unzip -o -q "${tmp_dir}/${archive_name}" -d "${tmp_dir}/extracted"
  else
    mkdir -p "${tmp_dir}/extracted"
    tar xzf "${tmp_dir}/${archive_name}" -C "${tmp_dir}/extracted"
  fi

  # Find the binary (goreleaser may place it in a subdirectory or at root)
  BINARY_PATH=$(find "${tmp_dir}/extracted" -name "${binary_name}" -type f | head -1)
  [ -n "$BINARY_PATH" ] || fatal "Binary '${binary_name}' not found in archive"
  chmod +x "$BINARY_PATH"
}

# --- Install binary ---
install_binary() {
  local binary_name="claude-plane-${1}"
  local install_dir

  if [ "$(id -u)" -eq 0 ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
    mkdir -p "$install_dir"
    case ":${PATH}:" in
      *":${install_dir}:"*) ;;
      *) echo -e "\n${CYAN}NOTE:${NC} ${install_dir} is not in your PATH. Add it:\n  export PATH=\"${install_dir}:\$PATH\"" ;;
    esac
  fi

  cp "$BINARY_PATH" "${install_dir}/${binary_name}"
  # Ensure binary is findable by subsequent commands in this script
  export PATH="${install_dir}:${PATH}"
  ok "Installed ${binary_name} to ${install_dir}/${binary_name}"
}

# --- Quickstart mode ---
run_quickstart() {
  local data_dir="$HOME/.claude-plane"
  local ca_dir="${data_dir}/ca"
  local server_cert_dir="${data_dir}/server-cert"
  local config_file="${data_dir}/server.toml"
  local db_path="${data_dir}/claude-plane.db"

  info "Setting up claude-plane quickstart..."
  mkdir -p "$data_dir" "$ca_dir" "$server_cert_dir"

  info "Generating CA..."
  claude-plane-server ca init --out-dir "$ca_dir"

  info "Issuing server certificate..."
  claude-plane-server ca issue-server --ca-dir "$ca_dir" --out-dir "$server_cert_dir"

  info "Generating JWT secret..."
  local jwt_secret
  jwt_secret=$(head -c 32 /dev/urandom | base64)

  info "Writing server config..."
  cat > "$config_file" <<TOML
[http]
address = "0.0.0.0:8080"

[grpc]
address = "0.0.0.0:9090"

[tls]
ca_cert     = "${ca_dir}/ca.pem"
server_cert = "${server_cert_dir}/server.pem"
server_key  = "${server_cert_dir}/server-key.pem"

[auth]
jwt_secret = "${jwt_secret}"

[store]
db_path = "${db_path}"
TOML

  # Admin credentials
  local admin_email="${CLAUDE_PLANE_ADMIN_EMAIL:-}"
  local admin_password="${CLAUDE_PLANE_ADMIN_PASSWORD:-}"

  if [ -z "$admin_email" ]; then
    printf "${BOLD}Admin email [admin@localhost]:${NC} "
    read -r admin_email
    admin_email="${admin_email:-admin@localhost}"
  fi

  if [ -z "$admin_password" ]; then
    printf "${BOLD}Admin password:${NC} "
    read -rs admin_password
    echo
    if [ -z "$admin_password" ]; then
      admin_password=$(head -c 16 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 20)
      info "Generated random password: ${admin_password}"
    fi
  fi

  info "Seeding admin account..."
  echo "$admin_password" | claude-plane-server seed-admin --db "$db_path" --email "$admin_email" --name Admin

  info "Starting server..."
  claude-plane-server serve --config "$config_file" &
  local server_pid=$!

  # Wait for server to be ready
  local ready=false
  for i in $(seq 1 30); do
    if curl -sf http://localhost:8080/api/v1/health &>/dev/null; then
      ready=true
      break
    fi
    if ! kill -0 "$server_pid" 2>/dev/null; then
      fatal "Server process exited unexpectedly"
    fi
    sleep 1
  done

  if [ "$ready" != "true" ]; then
    fatal "Server did not become ready after 30 seconds"
  fi

  echo ""
  ok "claude-plane is running!"
  echo -e "  Dashboard: ${BOLD}http://localhost:8080${NC}"
  echo -e "  Admin:     ${BOLD}${admin_email}${NC}"
  echo ""
  echo "Press Ctrl+C to stop."
  wait "$server_pid"
}

# --- Print next steps ---
print_next_steps() {
  echo ""
  case "$1" in
    server)
      ok "claude-plane-server installed successfully!"
      echo "  Next: claude-plane-server serve --config server.toml"
      echo "  Or:   curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash -s -- quickstart"
      ;;
    agent)
      ok "claude-plane-agent installed successfully!"
      echo "  Next: claude-plane-agent join <CODE> --server <URL>"
      ;;
    bridge)
      ok "claude-plane-bridge installed successfully!"
      echo "  Next: claude-plane-bridge --config bridge.toml"
      ;;
  esac
  echo ""
}

# --- Main ---
case "$COMPONENT" in
  server|agent|bridge)
    detect_platform
    resolve_version
    download_and_verify "$COMPONENT"
    install_binary "$COMPONENT"
    print_next_steps "$COMPONENT"
    ;;
  quickstart)
    detect_platform
    resolve_version
    download_and_verify "server"
    install_binary "server"
    run_quickstart
    ;;
  *)
    fatal "Unknown component: ${COMPONENT}. Use: server, agent, bridge, or quickstart"
    ;;
esac
