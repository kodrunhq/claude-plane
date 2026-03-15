# Distribution & Agent Join Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship pre-built binary distribution (goreleaser + install script), short join codes for agent provisioning, Docker images, and quickstart deprecation.

**Architecture:** Extends the existing provisioning system with short codes and a public redemption endpoint. Adds a universal install script that downloads pre-built binaries from GitHub Releases. Adds Docker multi-stage builds for all three binaries.

**Tech Stack:** Go 1.25, Cobra CLI, Chi router, SQLite, crypto/rand, Docker buildx, goreleaser v2, bash

**Spec:** `docs/superpowers/specs/2026-03-15-distribution-and-agent-join-design.md`

---

## Chunk 1: goreleaser + Install Script + Quickstart Deprecation (v0.3.0)

### Task 1: Add bridge build to goreleaser + set CGO_ENABLED=0

**Files:**
- Modify: `.goreleaser.yml`

- [ ] **Step 1: Add env and bridge build to goreleaser config**

Add `env: [CGO_ENABLED=0]` to both existing builds (server, agent) and add the bridge build:

```yaml
# In .goreleaser.yml, add to each existing build:
#   env: [CGO_ENABLED=0]
# Then add after the agent build:

  - id: bridge
    binary: claude-plane-bridge
    main: ./cmd/bridge
    env: [CGO_ENABLED=0]
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Version={{ .Version }}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Commit={{ .ShortCommit }}
      - -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Date={{ .Date }}
```

Add the bridge archive after the agent archive:

```yaml
  - id: bridge
    ids: [bridge]
    formats: [tar.gz]
    format_overrides:
      - goos: darwin
        formats: [zip]
    name_template: "claude-plane-bridge_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
```

- [ ] **Step 2: Validate goreleaser config locally**

Run: `goreleaser check`
Expected: no errors

- [ ] **Step 3: Test snapshot build**

Run: `goreleaser release --snapshot --clean --skip=publish 2>&1 | tail -20`
Expected: builds 12 binaries (3 components × 4 platforms), creates 12 archives + checksums.txt

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yml
git commit -m "feat: add bridge to goreleaser, set CGO_ENABLED=0 for all builds"
```

---

### Task 2: Create universal install script

**Files:**
- Create: `install.sh`

- [ ] **Step 1: Write the install script**

Create `install.sh` at repo root. The script must:
1. Detect OS (`uname -s` → `linux`/`darwin`) and arch (`uname -m` → `amd64`/`arm64`)
2. Parse first arg: `server` (default), `agent`, `bridge`, `quickstart`
3. Resolve version: `CLAUDE_PLANE_VERSION` env var or GitHub API latest release
4. Download archive (`.tar.gz` for Linux, `.zip` for Darwin — matches goreleaser `format_overrides`)
5. Verify SHA256 checksum against `checksums.txt`
6. Extract binary to `/usr/local/bin/` (root) or `~/.local/bin/` (non-root)
7. Print component-specific next steps

```bash
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
  ok "Installed ${binary_name} to ${install_dir}/${binary_name}"
}

# --- Quickstart mode ---
run_quickstart() {
  local data_dir="$HOME/.claude-plane"
  local ca_dir="${data_dir}/ca"
  local server_cert_dir="${data_dir}/server-cert"
  local agent_cert_dir="${data_dir}/agent-cert"
  local config_file="${data_dir}/server.toml"
  local db_path="${data_dir}/claude-plane.db"

  info "Setting up claude-plane quickstart..."
  mkdir -p "$data_dir" "$ca_dir" "$server_cert_dir" "$agent_cert_dir"

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
  for i in $(seq 1 30); do
    if curl -sf http://localhost:8080/api/v1/health &>/dev/null; then
      break
    fi
    if ! kill -0 "$server_pid" 2>/dev/null; then
      fatal "Server process exited unexpectedly"
    fi
    sleep 1
  done

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
      echo "  Or:   install.sh quickstart  (for interactive setup)"
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
```

- [ ] **Step 2: Make the script executable**

Run: `chmod +x install.sh`

- [ ] **Step 3: Verify script syntax**

Run: `bash -n install.sh`
Expected: no output (no syntax errors)

- [ ] **Step 4: Commit**

```bash
git add install.sh
git commit -m "feat: add universal install script for pre-built binaries"
```

---

### Task 3: Deprecate quickstart.sh

**Files:**
- Modify: `quickstart.sh:1-6`

- [ ] **Step 1: Add deprecation notice at the top of quickstart.sh**

Insert after `set -euo pipefail` (line 2), before the comment on line 4:

```bash
echo ""
echo -e "\033[0;33m==> WARNING:\033[0m quickstart.sh is deprecated and will be removed in a future release."
echo -e "    Use instead:  curl -fsSL https://raw.githubusercontent.com/kodrunhq/claude-plane/main/install.sh | bash -s -- quickstart"
echo ""
```

- [ ] **Step 2: Commit**

```bash
git add quickstart.sh
git commit -m "chore: deprecate quickstart.sh in favor of install.sh quickstart"
```

---

## Chunk 2: Short Code Backend — Migration, Store, Service (v0.3.1)

### Task 4: Add migration 13 — short_code column

**Files:**
- Modify: `internal/server/store/migrations.go`

- [ ] **Step 1: Add migration 13 at the end of the `migrations` slice**

Append after migration 12 (the last entry):

```go
	{
		Version:     13,
		Description: "add short_code to provisioning_tokens",
		SQL: `
ALTER TABLE provisioning_tokens ADD COLUMN short_code TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_provisioning_tokens_short_code ON provisioning_tokens(short_code);
`,
	},
```

- [ ] **Step 2: Verify the migration compiles**

Run: `go vet ./internal/server/store/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/server/store/migrations.go
git commit -m "feat: add migration 13 — short_code column on provisioning_tokens"
```

---

### Task 5: Add short code generator

**Files:**
- Create: `internal/server/provision/shortcode.go`
- Create: `internal/server/provision/shortcode_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/server/provision/shortcode_test.go`:

```go
package provision

import (
	"testing"
)

func TestGenerateShortCode_Length(t *testing.T) {
	code, err := GenerateShortCode()
	if err != nil {
		t.Fatalf("GenerateShortCode() error: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("expected length 6, got %d: %q", len(code), code)
	}
}

func TestGenerateShortCode_ValidChars(t *testing.T) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	for i := 0; i < 100; i++ {
		code, err := GenerateShortCode()
		if err != nil {
			t.Fatalf("iteration %d: GenerateShortCode() error: %v", i, err)
		}
		for _, ch := range code {
			found := false
			for _, valid := range alphabet {
				if ch == valid {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("invalid character %q in code %q", string(ch), code)
			}
		}
	}
}

func TestGenerateShortCode_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := GenerateShortCode()
		if err != nil {
			t.Fatalf("iteration %d: GenerateShortCode() error: %v", i, err)
		}
		if seen[code] {
			t.Errorf("duplicate code after %d iterations: %q", i, code)
		}
		seen[code] = true
	}
}

func TestValidateShortCode(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		valid bool
	}{
		{"valid code", "A3X9K2", true},
		{"lowercase", "a3x9k2", false},
		{"too short", "A3X9K", false},
		{"too long", "A3X9K2B", false},
		{"empty", "", false},
		{"contains O", "A3XOK2", false},
		{"contains 0", "A3X0K2", false},
		{"contains I", "A3XIK2", false},
		{"contains 1", "A3X1K2", false},
		{"contains L", "A3XLK2", false},
		{"all valid chars", "ABCDEF", true},
		{"all valid digits", "234567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateShortCode(tt.code); got != tt.valid {
				t.Errorf("ValidateShortCode(%q) = %v, want %v", tt.code, got, tt.valid)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests — they should fail**

Run: `go test ./internal/server/provision/ -run TestGenerateShortCode -v`
Expected: FAIL — `GenerateShortCode` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/server/provision/shortcode.go`:

```go
package provision

import (
	"crypto/rand"
	"math/big"
)

// shortCodeAlphabet excludes ambiguous characters: O/0, I/1, L.
const shortCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
const shortCodeLength = 6

// GenerateShortCode produces a cryptographically random 6-character code
// from a 30-character alphabet (no ambiguous chars).
func GenerateShortCode() (string, error) {
	alphabetLen := big.NewInt(int64(len(shortCodeAlphabet)))
	code := make([]byte, shortCodeLength)
	for i := range code {
		idx, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		code[i] = shortCodeAlphabet[idx.Int64()]
	}
	return string(code), nil
}

// ValidateShortCode checks that a code is exactly 6 characters from the
// valid alphabet. Returns false for any invalid input.
func ValidateShortCode(code string) bool {
	if len(code) != shortCodeLength {
		return false
	}
	for _, ch := range code {
		found := false
		for _, valid := range shortCodeAlphabet {
			if ch == valid {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run tests — they should pass**

Run: `go test ./internal/server/provision/ -run "TestGenerateShortCode|TestValidateShortCode" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/provision/shortcode.go internal/server/provision/shortcode_test.go
git commit -m "feat: add short code generator with crypto/rand and validation"
```

---

### Task 6: Update store — ShortCode field + new query method

**Files:**
- Modify: `internal/server/store/provisioning.go`
- Create: `internal/server/store/provisioning_shortcode_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/server/store/provisioning_shortcode_test.go`:

```go
package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kodrunhq/claude-plane/internal/server/store"
)

func newTestStoreForShortCode(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "shortcode_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func makeToken(machineID, shortCode string) store.ProvisioningToken {
	now := time.Now().UTC()
	return store.ProvisioningToken{
		Token:         "tok-" + machineID,
		MachineID:     machineID,
		ShortCode:     shortCode,
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "ca-pem",
		AgentCertPEM:  "cert-pem",
		AgentKeyPEM:   "key-pem",
		ServerAddress: "http://localhost:8080",
		GRPCAddress:   "localhost:9090",
		CreatedBy:     "test-user",
		CreatedAt:     now,
		ExpiresAt:     now.Add(1 * time.Hour),
	}
}

func TestCreateProvisioningToken_WithShortCode(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-01", "A3X9K2")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	got, err := s.GetProvisioningTokenByCode(ctx, "A3X9K2")
	if err != nil {
		t.Fatalf("GetProvisioningTokenByCode: %v", err)
	}
	if got.MachineID != "worker-01" {
		t.Errorf("MachineID = %q, want %q", got.MachineID, "worker-01")
	}
	if got.ShortCode != "A3X9K2" {
		t.Errorf("ShortCode = %q, want %q", got.ShortCode, "A3X9K2")
	}
}

func TestGetProvisioningTokenByCode_NotFound(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	_, err := s.GetProvisioningTokenByCode(ctx, "ZZZZZZ")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetProvisioningTokenByCode_Expired(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-02", "B4Y8J3")
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Hour) // already expired
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningTokenByCode(ctx, "B4Y8J3")
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

func TestGetProvisioningTokenByCode_Redeemed(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-03", "C5Z7H4")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}
	if err := s.RedeemProvisioningToken(ctx, tok.Token); err != nil {
		t.Fatalf("RedeemProvisioningToken: %v", err)
	}

	_, err := s.GetProvisioningTokenByCode(ctx, "C5Z7H4")
	if err == nil {
		t.Fatal("expected error for redeemed token, got nil")
	}
}

func TestListProvisioningTokens_IncludesShortCode(t *testing.T) {
	s := newTestStoreForShortCode(t)
	ctx := context.Background()

	tok := makeToken("worker-04", "D6W8F5")
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("CreateProvisioningToken: %v", err)
	}

	tokens, err := s.ListProvisioningTokens(ctx)
	if err != nil {
		t.Fatalf("ListProvisioningTokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].ShortCode != "D6W8F5" {
		t.Errorf("ShortCode = %q, want %q", tokens[0].ShortCode, "D6W8F5")
	}
}
```

- [ ] **Step 2: Run tests — they should fail**

Run: `go test ./internal/server/store/ -run "TestCreateProvisioningToken_WithShortCode|TestGetProvisioningTokenByCode|TestListProvisioningTokens_IncludesShortCode" -v`
Expected: FAIL — `ShortCode` field undefined, `GetProvisioningTokenByCode` undefined

- [ ] **Step 3: Update ProvisioningToken struct — add ShortCode field**

In `internal/server/store/provisioning.go`, add `ShortCode` field to `ProvisioningToken` struct (after line 14, after `Token`):

```go
	ShortCode     string     `json:"short_code"`
```

- [ ] **Step 4: Update ProvisioningTokenSummary struct — add ShortCode field**

In `internal/server/store/provisioning.go`, add `ShortCode` field to `ProvisioningTokenSummary` struct (after line 139, after `Token`):

```go
	ShortCode     string     `json:"short_code"`
```

- [ ] **Step 5: Update CreateProvisioningToken — add short_code to INSERT**

Replace the INSERT statement (lines 38-46) to include `short_code`:

```go
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO provisioning_tokens
		 (token, short_code, machine_id, target_os, target_arch, ca_cert_pem, agent_cert_pem,
		  agent_key_pem, server_address, grpc_address, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Token, t.ShortCode, t.MachineID, t.TargetOS, t.TargetArch, t.CACertPEM, t.AgentCertPEM,
		t.AgentKeyPEM, t.ServerAddress, t.GRPCAddress, t.CreatedBy,
		t.CreatedAt.UTC(), t.ExpiresAt.UTC(),
	)
```

- [ ] **Step 6: Update GetProvisioningToken — add short_code to SELECT/Scan**

Replace the SELECT/Scan (lines 61-71) to include `short_code`:

```go
	err := s.reader.QueryRowContext(ctx,
		`SELECT token, short_code, machine_id, target_os, target_arch, ca_cert_pem, agent_cert_pem,
		        agent_key_pem, server_address, grpc_address, created_by, created_at,
		        expires_at, redeemed_at
		 FROM provisioning_tokens WHERE token = ?`,
		token,
	).Scan(
		&pt.Token, &pt.ShortCode, &pt.MachineID, &pt.TargetOS, &pt.TargetArch, &pt.CACertPEM, &pt.AgentCertPEM,
		&pt.AgentKeyPEM, &pt.ServerAddress, &pt.GRPCAddress, &pt.CreatedBy, &pt.CreatedAt,
		&pt.ExpiresAt, &redeemedAt,
	)
```

- [ ] **Step 7: Add GetProvisioningTokenByCode method**

Add after `GetProvisioningToken` (after line 89):

```go
// GetProvisioningTokenByCode retrieves a provisioning token by its short code.
// Returns the same errors as GetProvisioningToken.
func (s *Store) GetProvisioningTokenByCode(ctx context.Context, code string) (*ProvisioningToken, error) {
	var pt ProvisioningToken
	var redeemedAt sql.NullTime

	err := s.reader.QueryRowContext(ctx,
		`SELECT token, short_code, machine_id, target_os, target_arch, ca_cert_pem, agent_cert_pem,
		        agent_key_pem, server_address, grpc_address, created_by, created_at,
		        expires_at, redeemed_at
		 FROM provisioning_tokens WHERE short_code = ?`,
		code,
	).Scan(
		&pt.Token, &pt.ShortCode, &pt.MachineID, &pt.TargetOS, &pt.TargetArch, &pt.CACertPEM, &pt.AgentCertPEM,
		&pt.AgentKeyPEM, &pt.ServerAddress, &pt.GRPCAddress, &pt.CreatedBy, &pt.CreatedAt,
		&pt.ExpiresAt, &redeemedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("provisioning token by code: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get provisioning token by code: %w", err)
	}

	if redeemedAt.Valid {
		pt.RedeemedAt = &redeemedAt.Time
		return nil, fmt.Errorf("get provisioning token by code: %w", ErrTokenAlreadyRedeemed)
	}

	if time.Now().UTC().After(pt.ExpiresAt.UTC()) {
		return nil, fmt.Errorf("get provisioning token by code: %w", ErrTokenExpired)
	}

	return &pt, nil
}
```

- [ ] **Step 8: Update ListProvisioningTokens — add short_code to SELECT/Scan**

Replace the SELECT (lines 154-158) to include `short_code`:

```go
	rows, err := s.reader.QueryContext(ctx,
		`SELECT token, short_code, machine_id, target_os, target_arch, server_address, grpc_address,
		        created_by, created_at, expires_at, redeemed_at
		 FROM provisioning_tokens
		 ORDER BY created_at DESC`,
	)
```

And update the Scan (lines 169-172) to include `&t.ShortCode`:

```go
		if err := rows.Scan(
			&t.Token, &t.ShortCode, &t.MachineID, &t.TargetOS, &t.TargetArch,
			&t.ServerAddress, &t.GRPCAddress, &t.CreatedBy,
			&t.CreatedAt, &t.ExpiresAt, &redeemedAt,
		); err != nil {
```

- [ ] **Step 9: Run tests — they should pass**

Run: `go test ./internal/server/store/ -run "TestCreateProvisioningToken_WithShortCode|TestGetProvisioningTokenByCode|TestListProvisioningTokens_IncludesShortCode" -v`
Expected: all PASS

- [ ] **Step 10: Run all store tests to check for regressions**

Run: `go test -race ./internal/server/store/...`
Expected: all PASS. If any existing tests fail because `ProvisioningToken` fixtures don't set `ShortCode` or SELECT/Scan column count changed, update those fixtures to include `ShortCode: ""` (empty string is valid for pre-existing tokens since the column is nullable).

- [ ] **Step 11: Run handler tests and fix regressions from ProvisionResult changes**

Run: `go test -race ./internal/server/handler/ -run TestProvision -v`
Expected: existing tests may fail if they assert on exact `ProvisionResult` JSON (now includes `short_code` and `join_command` fields). Update assertions in `provision_test.go` to expect these new fields.

- [ ] **Step 12: Commit**

```bash
git add internal/server/store/provisioning.go internal/server/store/provisioning_shortcode_test.go
git commit -m "feat: add ShortCode to provisioning store — struct, insert, query by code, list"
```

---

### Task 7: Update provision service — generate short code + return JoinCommand

**Files:**
- Modify: `internal/server/provision/service.go`

- [ ] **Step 1: Update ProvisionResult struct**

In `internal/server/provision/service.go`, replace the `ProvisionResult` struct (lines 25-29):

```go
type ProvisionResult struct {
	Token       string    `json:"token"`
	ShortCode   string    `json:"short_code"`
	ExpiresAt   time.Time `json:"expires_at"`
	CurlCommand string    `json:"curl_command"`
	JoinCommand string    `json:"join_command"`
}
```

- [ ] **Step 2: Update CreateAgentProvision to generate and store short code**

In the `CreateAgentProvision` method, after `tokenID := uuid.New().String()` (line 98), add short code generation:

```go
	shortCode, err := GenerateShortCode()
	if err != nil {
		return nil, fmt.Errorf("generate short code: %w", err)
	}
```

Then add `ShortCode` to the token struct (after line 102):

```go
		ShortCode:     shortCode,
```

And update the return (replace lines 122-126):

```go
	joinCmd := fmt.Sprintf("claude-plane-agent join %s --server %s", shortCode, svc.httpAddress)

	return &ProvisionResult{
		Token:       tokenID,
		ShortCode:   shortCode,
		ExpiresAt:   expiresAt,
		CurlCommand: curlCmd,
		JoinCommand: joinCmd,
	}, nil
```

- [ ] **Step 3: Run all provision tests**

Run: `go test -race ./internal/server/provision/...`
Expected: all PASS

- [ ] **Step 4: Run handler tests to catch regressions from ProvisionResult change**

Run: `go test -race ./internal/server/handler/ -run TestProvision -v`
Expected: PASS (existing handler tests may need updating if they assert on ProvisionResult fields — if they fail, update the expected JSON to include `short_code` and `join_command`)

- [ ] **Step 5: Commit**

```bash
git add internal/server/provision/service.go
git commit -m "feat: generate short codes in provision service, return JoinCommand"
```

---

## Chunk 3: Join Endpoint + Agent CLI (v0.3.1 continued)

### Task 8: Add POST /api/v1/provision/join handler

**Files:**
- Modify: `internal/server/handler/provision.go`
- Modify: `internal/server/handler/provision_test.go`

- [ ] **Step 1: Write failing tests for the join handler**

Add to `internal/server/handler/provision_test.go`:

```go
func TestJoinHandler_ValidCode(t *testing.T) {
	h, s := newProvisionHandlerFixture(t, &handler.UserClaims{UserID: "admin-id", Role: "admin"})
	ctx := context.Background()

	// Create a token with a short code
	tok := store.ProvisioningToken{
		Token:         "test-token-join",
		ShortCode:     "A3X9K2",
		MachineID:     "worker-join",
		TargetOS:      "linux",
		TargetArch:    "amd64",
		CACertPEM:     "ca-data",
		AgentCertPEM:  "cert-data",
		AgentKeyPEM:   "key-data",
		ServerAddress: "http://test.example.com",
		GRPCAddress:   "test.example.com:9090",
		CreatedBy:     "admin",
		CreatedAt:     time.Now().UTC(),
		ExpiresAt:     time.Now().UTC().Add(1 * time.Hour),
	}
	if err := s.CreateProvisioningToken(ctx, tok); err != nil {
		t.Fatalf("setup: %v", err)
	}

	body := `{"code":"A3X9K2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/join", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r := chi.NewRouter()
	r.Post("/api/v1/provision/join", h.JoinByCode)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["machine_id"] != "worker-join" {
		t.Errorf("machine_id = %q, want %q", resp["machine_id"], "worker-join")
	}
	if resp["grpc_address"] != "test.example.com:9090" {
		t.Errorf("grpc_address = %q, want %q", resp["grpc_address"], "test.example.com:9090")
	}
}

func TestJoinHandler_InvalidCode(t *testing.T) {
	h, _ := newProvisionHandlerFixture(t, &handler.UserClaims{UserID: "admin-id", Role: "admin"})

	tests := []struct {
		name string
		body string
		code int
	}{
		{"empty body", `{}`, http.StatusBadRequest},
		{"missing code", `{"code":""}`, http.StatusBadRequest},
		{"too short", `{"code":"A3X9K"}`, http.StatusBadRequest},
		{"too long", `{"code":"A3X9K2B"}`, http.StatusBadRequest},
		{"invalid chars", `{"code":"a3x9k2"}`, http.StatusBadRequest},
		{"ambiguous O", `{"code":"A3XOK2"}`, http.StatusBadRequest},
		{"not found", `{"code":"ZZZZZZ"}`, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/provision/join", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r := chi.NewRouter()
			r.Post("/api/v1/provision/join", h.JoinByCode)
			r.ServeHTTP(w, req)

			if w.Code != tt.code {
				t.Errorf("status = %d, want %d; body = %s", w.Code, tt.code, w.Body.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run tests — they should fail**

Run: `go test ./internal/server/handler/ -run "TestJoinHandler" -v`
Expected: FAIL — `JoinByCode` method undefined

- [ ] **Step 3: Add the JoinByCode handler**

Add to `internal/server/handler/provision.go` after the `ServeScript` method:

```go
type joinRequest struct {
	Code string `json:"code"`
}

type joinResponse struct {
	MachineID    string `json:"machine_id"`
	GRPCAddress  string `json:"grpc_address"`
	CACertPEM    string `json:"ca_cert_pem"`
	AgentCertPEM string `json:"agent_cert_pem"`
	AgentKeyPEM  string `json:"agent_key_pem"`
}

// JoinByCode handles POST /api/v1/provision/join (public, no JWT).
// Redeems a provisioning token by its 6-character short code.
func (h *ProvisionHandler) JoinByCode(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !provision.ValidateShortCode(req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code format: must be 6 characters from ABCDEFGHJKMNPQRSTUVWXYZ23456789")
		return
	}

	token, err := h.store.GetProvisioningTokenByCode(r.Context(), req.Code)
	if err != nil {
		// All error cases (not found, expired, redeemed) return 404
		// to avoid leaking whether a code was ever valid.
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	if err := h.store.RedeemProvisioningToken(r.Context(), token.Token); err != nil {
		writeError(w, http.StatusNotFound, "invalid or expired code")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, joinResponse{
		MachineID:    token.MachineID,
		GRPCAddress:  token.GRPCAddress,
		CACertPEM:    token.CACertPEM,
		AgentCertPEM: token.AgentCertPEM,
		AgentKeyPEM:  token.AgentKeyPEM,
	})
}
```

- [ ] **Step 4: Update RegisterProvisionPublicRoutes — add the join endpoint**

In `RegisterProvisionPublicRoutes` (line 176-178), add the join route:

```go
func RegisterProvisionPublicRoutes(r chi.Router, h *ProvisionHandler) {
	r.Get("/api/v1/provision/{token}/script", h.ServeScript)
	r.Post("/api/v1/provision/join", h.JoinByCode)
}
```

- [ ] **Step 5: Run tests — they should pass**

Run: `go test ./internal/server/handler/ -run "TestJoinHandler" -v`
Expected: all PASS

- [ ] **Step 6: Run all handler tests for regressions**

Run: `go test -race ./internal/server/handler/...`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/server/handler/provision.go internal/server/handler/provision_test.go
git commit -m "feat: add POST /api/v1/provision/join endpoint for short code redemption"
```

---

### Task 9: Add rate limiting to join endpoint

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add rate limiting when registering the join public route**

In `cmd/server/main.go`, find where `RegisterProvisionPublicRoutes` is called (around line 428). Replace with a rate-limited group:

```go
			// Provisioning: public routes (token-authenticated and short-code join).
			router.Group(func(r chi.Router) {
				r.Use(api.RateLimitMiddleware(10.0/60.0, 10)) // 10 req/min per IP
				handler.RegisterProvisionPublicRoutes(r, provisionHandler)
			})
```

Note: `10.0/60.0` = 10 requests per 60 seconds as a `rate.Limit` (events per second). Burst of 10 allows brief spikes.

- [ ] **Step 2: Run go vet**

Run: `go vet ./cmd/server/...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: rate limit provisioning public routes — 10 req/min per IP"
```

---

### Task 10: Add agent join CLI command

**Files:**
- Create: `internal/agent/join.go`
- Create: `internal/agent/join_test.go`
- Modify: `cmd/agent/main.go`

- [ ] **Step 1: Write failing tests for the join logic**

Create `internal/agent/join_test.go`:

```go
package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoin_Success(t *testing.T) {
	// Mock server returning valid join response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/provision/join" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req struct{ Code string `json:"code"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Code != "A3X9K2" {
			t.Errorf("code = %q, want A3X9K2", req.Code)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"machine_id":    "worker-01",
			"grpc_address":  "10.0.1.50:9090",
			"ca_cert_pem":   "-----BEGIN CERTIFICATE-----\nCA\n-----END CERTIFICATE-----",
			"agent_cert_pem": "-----BEGIN CERTIFICATE-----\nAGENT\n-----END CERTIFICATE-----",
			"agent_key_pem":  "-----BEGIN RSA PRIVATE KEY-----\nKEY\n-----END RSA PRIVATE KEY-----",
		})
	}))
	defer server.Close()

	configDir := t.TempDir()

	err := ExecuteJoin(server.URL, "A3X9K2", configDir)
	if err != nil {
		t.Fatalf("ExecuteJoin: %v", err)
	}

	// Verify files written
	assertFileContains(t, filepath.Join(configDir, "certs", "ca.pem"), "-----BEGIN CERTIFICATE-----\nCA")
	assertFileContains(t, filepath.Join(configDir, "certs", "agent.pem"), "-----BEGIN CERTIFICATE-----\nAGENT")
	assertFileContains(t, filepath.Join(configDir, "certs", "agent-key.pem"), "-----BEGIN RSA PRIVATE KEY-----\nKEY")
	assertFileContains(t, filepath.Join(configDir, "agent.toml"), "machine_id")
	assertFileContains(t, filepath.Join(configDir, "agent.toml"), "worker-01")
}

func TestJoin_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired code"})
	}))
	defer server.Close()

	err := ExecuteJoin(server.URL, "ZZZZZZ", t.TempDir())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveServerURL(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		envVar   string
		wantURL  string
		wantErr  bool
	}{
		{"flag provided", "https://example.com", "", "https://example.com", false},
		{"env provided", "", "https://env.example.com", "https://env.example.com", false},
		{"flag takes precedence", "https://flag.com", "https://env.com", "https://flag.com", false},
		{"neither provided", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVar != "" {
				t.Setenv("CLAUDE_PLANE_SERVER", tt.envVar)
			} else {
				os.Unsetenv("CLAUDE_PLANE_SERVER")
			}

			got, err := ResolveServerURL(tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.wantURL {
				t.Errorf("url = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Errorf("%s does not contain %q; contents:\n%s", path, substr, string(data))
	}
}
```

- [ ] **Step 2: Run tests — they should fail**

Run: `go test ./internal/agent/ -run "TestJoin|TestResolveServerURL" -v`
Expected: FAIL — `ExecuteJoin` and `ResolveServerURL` undefined

- [ ] **Step 3: Write the join implementation**

Create `internal/agent/join.go`:

```go
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type joinResponse struct {
	MachineID    string `json:"machine_id"`
	GRPCAddress  string `json:"grpc_address"`
	CACertPEM    string `json:"ca_cert_pem"`
	AgentCertPEM string `json:"agent_cert_pem"`
	AgentKeyPEM  string `json:"agent_key_pem"`
}

type joinErrorResponse struct {
	Error string `json:"error"`
}

// ResolveServerURL determines the server URL from the flag or environment.
func ResolveServerURL(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if env := os.Getenv("CLAUDE_PLANE_SERVER"); env != "" {
		return env, nil
	}
	return "", fmt.Errorf("server URL required. Use --server or set CLAUDE_PLANE_SERVER")
}

// ExecuteJoin calls the server's join endpoint with the short code,
// writes certificates and config to configDir.
func ExecuteJoin(serverURL, code, configDir string) error {
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := http.Post(serverURL+"/api/v1/provision/join", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp joinErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var joinResp joinResponse
	if err := json.Unmarshal(respBody, &joinResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Create directories
	certsDir := filepath.Join(configDir, "certs")
	if err := os.MkdirAll(certsDir, 0o750); err != nil {
		return fmt.Errorf("create certs dir: %w", err)
	}

	// Write certificates
	files := map[string]string{
		filepath.Join(certsDir, "ca.pem"):        joinResp.CACertPEM,
		filepath.Join(certsDir, "agent.pem"):      joinResp.AgentCertPEM,
		filepath.Join(certsDir, "agent-key.pem"):   joinResp.AgentKeyPEM,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", filepath.Base(path), err)
		}
	}

	// Write agent.toml
	configContent := fmt.Sprintf(`[server]
address = %q

[tls]
ca_cert   = %q
agent_cert = %q
agent_key  = %q

[agent]
machine_id = %q
data_dir   = %q
`,
		joinResp.GRPCAddress,
		filepath.Join(certsDir, "ca.pem"),
		filepath.Join(certsDir, "agent.pem"),
		filepath.Join(certsDir, "agent-key.pem"),
		joinResp.MachineID,
		filepath.Join(configDir, "data"),
	)

	configPath := filepath.Join(configDir, "agent.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o640); err != nil {
		return fmt.Errorf("write agent.toml: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Run tests — they should pass**

Run: `go test ./internal/agent/ -run "TestJoin|TestResolveServerURL" -v`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/join.go internal/agent/join_test.go
git commit -m "feat: add agent join logic — server call, cert writing, config generation"
```

---

### Task 11: Wire join command into agent CLI

**Files:**
- Modify: `cmd/agent/main.go`

- [ ] **Step 1: Add newJoinCmd and register it**

In `cmd/agent/main.go`, add the join command registration to `rootCmd.AddCommand` (line 26-28):

```go
	rootCmd.AddCommand(
		newRunCmd(),
		newJoinCmd(),
	)
```

Then add the `newJoinCmd` function after `newRunCmd`:

```go
func newJoinCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "join CODE",
		Short: "Join a server using a 6-character provisioning code",
		Long:  "Redeems a short provisioning code to configure this agent with TLS certificates and server connection details.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := args[0]
			serverFlag, _ := cmd.Flags().GetString("server")
			configDir, _ := cmd.Flags().GetString("config-dir")
			insecure, _ := cmd.Flags().GetBool("insecure")

			serverURL, err := agent.ResolveServerURL(serverFlag)
			if err != nil {
				return err
			}

			// Enforce HTTPS unless --insecure is set
			if !insecure && len(serverURL) >= 7 && serverURL[:7] == "http://" {
				return fmt.Errorf("server URL must use HTTPS. Use --insecure to allow plain HTTP (not recommended for production)")
			}
			if insecure && len(serverURL) >= 7 && serverURL[:7] == "http://" {
				fmt.Fprintln(os.Stderr, "WARNING: Using plain HTTP. Certificate material will be transmitted unencrypted. Use HTTPS in production.")
			}

			if err := agent.ExecuteJoin(serverURL, code, configDir); err != nil {
				return err
			}

			configPath := configDir + "/agent.toml"
			fmt.Printf("\nAgent configured for machine joining\n")
			fmt.Printf("Certificates written to %s/certs/\n", configDir)
			fmt.Printf("Config written to %s\n\n", configPath)
			fmt.Printf("Start the agent:\n")
			fmt.Printf("  claude-plane-agent run --config %s\n\n", configPath)
			return nil
		},
	}

	// Determine default config dir based on user
	defaultConfigDir := os.Getenv("HOME") + "/.claude-plane"
	if os.Getuid() == 0 {
		defaultConfigDir = "/etc/claude-plane"
	}

	cmd.Flags().String("server", "", "Server HTTP URL (falls back to CLAUDE_PLANE_SERVER env var)")
	cmd.Flags().String("config-dir", defaultConfigDir, "Directory for config and certificates")
	cmd.Flags().Bool("insecure", false, "Allow plain HTTP server URL (prints warning)")
	return cmd
}
```

Also add `"os"` to imports if not already present.

- [ ] **Step 2: Verify it compiles**

Run: `go build ./cmd/agent/`
Expected: no errors

- [ ] **Step 3: Verify help output**

Run: `go run ./cmd/agent/ join --help`
Expected: shows usage with CODE arg, --server, --config-dir, --insecure flags

- [ ] **Step 4: Commit**

```bash
git add cmd/agent/main.go
git commit -m "feat: add 'join' subcommand to agent CLI"
```

---

## Chunk 4: Frontend Changes (v0.3.1 continued)

### Task 12: Update frontend types and API

**Files:**
- Modify: `web/src/types/provisioning.ts`

- [ ] **Step 1: Add short_code to ProvisioningToken interface**

In `web/src/types/provisioning.ts`, add `short_code` to `ProvisioningToken` (after `token`):

```typescript
  short_code: string;
```

- [ ] **Step 2: Add short_code and join_command to ProvisionResult interface**

Update `ProvisionResult`:

```typescript
export interface ProvisionResult {
  token: string;
  short_code: string;
  expires_at: string;
  curl_command: string;
  join_command: string;
}
```

- [ ] **Step 3: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: may show errors in components that use these types — that's expected, we fix them in the next tasks

- [ ] **Step 4: Commit**

```bash
git add web/src/types/provisioning.ts
git commit -m "feat: add short_code and join_command to provisioning types"
```

---

### Task 13: Update TokenGenerator — prominent short code display

**Files:**
- Modify: `web/src/components/provisioning/TokenGenerator.tsx`

- [ ] **Step 1: Redesign the result display**

Replace the entire `TokenGenerator` component in `web/src/components/provisioning/TokenGenerator.tsx` with:

```tsx
import { useState, useEffect } from 'react';
import { Copy, Check, Terminal, ChevronDown, ChevronUp } from 'lucide-react';
import { toast } from 'sonner';
import { useCreateProvisioningToken } from '../../hooks/useProvisioning.ts';
import type { CreateProvisionParams, ProvisionResult } from '../../types/provisioning.ts';
import { OS_OPTIONS, ARCH_OPTIONS } from '../../types/provisioning.ts';

const DEFAULT_PARAMS: CreateProvisionParams = {
  machine_id: '',
  os: 'linux',
  arch: 'amd64',
};

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      toast.success('Copied to clipboard');
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Failed to copy to clipboard');
    }
  }

  return (
    <button
      onClick={handleCopy}
      className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-md bg-bg-tertiary text-text-secondary hover:text-text-primary transition-colors"
    >
      {copied ? <Check size={13} className="text-status-success" /> : <Copy size={13} />}
      {copied ? 'Copied' : label}
    </button>
  );
}

function ExpiryCountdown({ expiresAt }: { expiresAt: string }) {
  const [, setTick] = useState(0);

  // Re-render every second for countdown
  useEffect(() => {
    const interval = setInterval(() => setTick((t) => t + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const now = new Date();
  const expires = new Date(expiresAt);
  const diffMs = expires.getTime() - now.getTime();

  if (diffMs <= 0) {
    return <span className="text-xs text-status-error">Expired</span>;
  }

  const minutes = Math.floor(diffMs / 60000);
  const seconds = Math.floor((diffMs % 60000) / 1000);

  return (
    <span className="text-xs text-text-secondary">
      Expires in{' '}
      <span className="text-text-primary font-medium">
        {minutes}m {seconds.toString().padStart(2, '0')}s
      </span>
    </span>
  );
}

export function TokenGenerator() {
  const createToken = useCreateProvisioningToken();
  const [params, setParams] = useState<CreateProvisionParams>(DEFAULT_PARAMS);
  const [result, setResult] = useState<ProvisionResult | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);

  function handleChange(field: keyof CreateProvisionParams, value: string) {
    setParams((prev) => ({ ...prev, [field]: value }));
    if (result) setResult(null);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    try {
      const res = await createToken.mutateAsync(params);
      setResult(res);
      setParams(DEFAULT_PARAMS);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create provisioning token');
    }
  }

  return (
    <div className="rounded-lg border border-border-primary bg-bg-secondary overflow-hidden">
      <div className="px-5 py-4 border-b border-border-primary">
        <h2 className="text-sm font-semibold text-text-primary">Generate Provisioning Token</h2>
        <p className="text-xs text-text-secondary mt-0.5">
          Creates a one-time install token valid for 1 hour
        </p>
      </div>

      <form onSubmit={handleSubmit} className="px-5 py-4 space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="sm:col-span-1">
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Machine ID <span className="text-status-error">*</span>
            </label>
            <input
              type="text"
              value={params.machine_id}
              onChange={(e) => handleChange('machine_id', e.target.value)}
              placeholder="e.g. worker-01"
              required
              pattern="[a-zA-Z0-9][a-zA-Z0-9\-]{0,57}"
              title="Alphanumeric and hyphens, 1-58 characters"
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-accent-primary transition-colors"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">OS</label>
            <select
              value={params.os}
              onChange={(e) => handleChange('os', e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
            >
              {OS_OPTIONS.map((os) => (
                <option key={os} value={os}>{os}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">Architecture</label>
            <select
              value={params.arch}
              onChange={(e) => handleChange('arch', e.target.value)}
              className="w-full px-3 py-2 text-sm rounded-md border border-border-primary bg-bg-primary text-text-primary focus:outline-none focus:border-accent-primary transition-colors"
            >
              {ARCH_OPTIONS.map((arch) => (
                <option key={arch} value={arch}>{arch}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="flex justify-end">
          <button
            type="submit"
            disabled={createToken.isPending || !params.machine_id.trim()}
            className="flex items-center gap-2 px-4 py-2 text-sm rounded-md bg-accent-primary hover:bg-accent-primary/80 text-white transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {createToken.isPending ? 'Generating...' : 'Generate Token'}
          </button>
        </div>
      </form>

      {result && (
        <div className="px-5 pb-5 space-y-3">
          <div className="rounded-md border border-status-success/30 bg-status-success/5 p-4 space-y-4">
            {/* Short code — primary display */}
            <div className="text-center space-y-2">
              <p className="text-xs font-medium text-status-success">Join Code</p>
              <div className="flex items-center justify-center gap-3">
                <code className="text-3xl font-mono font-bold tracking-[0.3em] text-text-primary">
                  {result.short_code}
                </code>
                <CopyButton text={result.short_code} label="Copy" />
              </div>
            </div>

            {/* Join command */}
            <div className="space-y-1.5">
              <p className="text-xs text-text-secondary">Run on the target machine:</p>
              <div className="flex items-center gap-2">
                <Terminal size={14} className="text-text-secondary shrink-0" />
                <code className="flex-1 text-xs font-mono text-text-primary bg-bg-primary rounded px-3 py-2 border border-border-primary">
                  {result.join_command}
                </code>
                <CopyButton text={result.join_command} label="Copy" />
              </div>
            </div>

            {/* Expiry */}
            <div className="flex items-center justify-center">
              <ExpiryCountdown expiresAt={result.expires_at} />
            </div>

            {/* Advanced: curl command */}
            <div className="border-t border-border-primary pt-3">
              <button
                onClick={() => setShowAdvanced(!showAdvanced)}
                className="flex items-center gap-1 text-xs text-text-secondary hover:text-text-primary transition-colors"
              >
                {showAdvanced ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
                Advanced (curl command)
              </button>
              {showAdvanced && (
                <div className="mt-2 space-y-1.5">
                  <p className="text-xs text-text-secondary">For scripted provisioning:</p>
                  <div className="flex items-start gap-2">
                    <code className="flex-1 text-xs font-mono text-text-primary bg-bg-primary rounded px-3 py-2 border border-border-primary break-all">
                      {result.curl_command}
                    </code>
                    <CopyButton text={result.curl_command} label="Copy" />
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add web/src/components/provisioning/TokenGenerator.tsx
git commit -m "feat: redesign TokenGenerator with prominent short code display"
```

---

### Task 14: Update TokensList — add short code column

**Files:**
- Modify: `web/src/components/provisioning/TokensList.tsx`

- [ ] **Step 1: Add short code column to the table header**

In `TokensList.tsx`, add a new `<th>` after the "Machine ID" header (after line 129):

```tsx
              <th className="px-4 py-3 text-xs font-semibold uppercase tracking-wider text-text-secondary">
                Join Code
              </th>
```

- [ ] **Step 2: Add short code cell to TokenRow**

In the `TokenRow` component, add a new `<td>` after the machine_id cell (after line 75):

```tsx
      <td className="px-4 py-3">
        <code className="text-sm font-mono font-medium text-accent-primary tracking-wider">
          {token.short_code}
        </code>
      </td>
```

- [ ] **Step 3: Run typecheck**

Run: `cd web && npx tsc --noEmit`
Expected: no errors

- [ ] **Step 4: Run frontend tests**

Run: `cd web && npx vitest run`
Expected: all pass

- [ ] **Step 5: Commit**

```bash
git add web/src/components/provisioning/TokensList.tsx
git commit -m "feat: add join code column to provisioning tokens list"
```

---

## Chunk 5: Docker Images + Release Workflow (v0.3.2)

### Task 15: Create Dockerfiles

**Files:**
- Create: `Dockerfile.server`
- Create: `Dockerfile.agent`
- Create: `Dockerfile.bridge`

- [ ] **Step 1: Create Dockerfile.server**

```dockerfile
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/internal/server/frontend/dist ./internal/server/frontend/dist
RUN CGO_ENABLED=0 go build -o /claude-plane-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /claude-plane-server /usr/local/bin/
ENTRYPOINT ["claude-plane-server"]
CMD ["serve", "--config", "/etc/claude-plane/server.toml"]
```

- [ ] **Step 2: Create Dockerfile.agent**

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /claude-plane-agent ./cmd/agent

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /claude-plane-agent /usr/local/bin/
ENTRYPOINT ["claude-plane-agent"]
CMD ["run", "--config", "/etc/claude-plane/agent.toml"]
```

- [ ] **Step 3: Create Dockerfile.bridge**

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /claude-plane-bridge ./cmd/bridge

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /claude-plane-bridge /usr/local/bin/
ENTRYPOINT ["claude-plane-bridge"]
CMD ["--config", "/etc/claude-plane/bridge.toml"]
```

- [ ] **Step 4: Verify Dockerfile.server builds locally**

Run: `docker build -f Dockerfile.server -t claude-plane-server:test .`
Expected: builds successfully (may take a few minutes on first run)

- [ ] **Step 5: Verify Dockerfile.agent builds locally**

Run: `docker build -f Dockerfile.agent -t claude-plane-agent:test .`
Expected: builds successfully

- [ ] **Step 6: Verify Dockerfile.bridge builds locally**

Run: `docker build -f Dockerfile.bridge -t claude-plane-bridge:test .`
Expected: builds successfully

- [ ] **Step 7: Commit**

```bash
git add Dockerfile.server Dockerfile.agent Dockerfile.bridge
git commit -m "feat: add multi-stage Dockerfiles for server, agent, and bridge"
```

---

### Task 16: Update release workflow — Docker build and push

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add Docker job to release workflow**

Add the `docker` job after the existing `release` job in `.github/workflows/release.yml`. Also add `packages: write` to permissions:

```yaml
permissions:
  contents: write
  packages: write
```

Then add the docker job:

```yaml
  docker:
    runs-on: ubuntu-latest
    needs: release
    steps:
      - uses: actions/checkout@v4

      - uses: docker/setup-qemu-action@v3

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.server
          push: true
          platforms: linux/amd64,linux/arm64
          tags: |
            ghcr.io/kodrunhq/claude-plane-server:${{ github.ref_name }}
            ghcr.io/kodrunhq/claude-plane-server:latest

      - uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.agent
          push: true
          platforms: linux/amd64,linux/arm64
          tags: |
            ghcr.io/kodrunhq/claude-plane-agent:${{ github.ref_name }}
            ghcr.io/kodrunhq/claude-plane-agent:latest

      - uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.bridge
          push: true
          platforms: linux/amd64,linux/arm64
          tags: |
            ghcr.io/kodrunhq/claude-plane-bridge:${{ github.ref_name }}
            ghcr.io/kodrunhq/claude-plane-bridge:latest
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "feat: add multi-arch Docker build and push to release workflow"
```

---

## Verification

After all tasks are complete:

- [ ] `go vet ./...` — no errors
- [ ] `go test -race ./...` — all pass
- [ ] `cd web && npx tsc --noEmit` — no type errors
- [ ] `cd web && npx vitest run` — all pass
- [ ] `goreleaser check` — config valid
- [ ] `bash -n install.sh` — no syntax errors
