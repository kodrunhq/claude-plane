# Phase 1: Foundation - Research

**Researched:** 2026-03-11
**Domain:** Go project scaffolding, protobuf/gRPC contracts, mTLS CA tooling, SQLite with WAL, TOML config parsing, admin seeding
**Confidence:** HIGH

## Summary

Phase 1 establishes the shared infrastructure that every subsequent phase depends on. It produces two compilable Go binaries (server and agent), a protobuf contract between them, built-in CA tooling for mTLS certificate generation, an initialized SQLite database with correct concurrency settings, TOML config parsing for both binaries, and an admin account seeding mechanism.

All technologies in this phase are mature, well-documented, and have standard patterns. The Go module setup, buf-based protobuf generation, SQLite WAL configuration, TOML parsing, and X.509 certificate generation via Go's standard library are thoroughly understood. The primary risks are not in the technology but in getting foundational patterns right from the start -- particularly the SQLite single-writer connection pattern and the protobuf message design that must accommodate all future phases.

**Primary recommendation:** Invest the most effort in the protobuf contract design and SQLite store layer -- these are the two components most expensive to change later. The CA tooling and config parsing are straightforward.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| INFR-01 | Server is a single Go binary with embedded frontend assets | Go module with `cmd/server/main.go`, `go:embed` placeholder for frontend assets (actual frontend comes in Phase 5) |
| INFR-02 | Agent is a single Go binary with no external dependencies | Go module with `cmd/agent/main.go`, pure-Go dependencies only (`modernc.org/sqlite` not needed in agent) |
| INFR-03 | Server uses SQLite with WAL mode for all persistent storage | `modernc.org/sqlite` via `database/sql`, DSN pragmas for WAL + busy_timeout, single-writer pattern |
| INFR-04 | Server and agent support TOML configuration files | `BurntSushi/toml` for config struct deserialization, validation with clear error messages |
| AGNT-01 | Server provides CA tooling to generate root CA, server certs, and agent certs | `crypto/x509` + `crypto/ecdsa` for cert generation, cobra subcommands `ca init`, `ca issue-server`, `ca issue-agent` |
| AUTH-04 | Admin account can be seeded via server CLI command on first run | `users` table in schema, `golang.org/x/crypto/argon2` for password hashing, cobra subcommand `seed-admin` |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.23+ | Language runtime | Single binary, excellent stdlib for crypto/TLS/SQL |
| `modernc.org/sqlite` | latest | SQLite driver (pure Go) | CGO-free, enables `CGO_ENABLED=0` builds -- hard requirement for single-binary deployment |
| `google.golang.org/grpc` | v1.79+ | gRPC framework | Only needed for proto generation in Phase 1; full gRPC use comes in Phase 2-3 |
| `google.golang.org/protobuf` | v1.36+ | Protobuf runtime | Required for generated Go code from `.proto` files |
| `BurntSushi/toml` | v1.5.0 | TOML config parsing | TOML v1.1 compliant, stable, zero drama |
| `spf13/cobra` | v1.8+ | CLI framework | Subcommands for `serve`, `ca init`, `ca issue-server`, `ca issue-agent`, `seed-admin` |
| `golang.org/x/crypto` | latest | Argon2id password hashing | `argon2.IDKey()` for admin password hashing (AUTH-04) |
| `golang-jwt/jwt` | v5 | JWT token library | Needed for auth infrastructure seeded in Phase 1 schema |
| `log/slog` | stdlib | Structured logging | Zero dependency, JSON for production, text for dev |
| `buf` CLI | latest | Protobuf toolchain | Replaces raw protoc, managed mode for Go package paths |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `protoc-gen-go` | latest | Go protobuf code generation | Via `buf generate` |
| `protoc-gen-go-grpc` | latest | Go gRPC code generation | Via `buf generate` |
| `crypto/x509` | stdlib | X.509 certificate generation | CA init, server cert, agent cert generation |
| `crypto/ecdsa` | stdlib | ECDSA key generation | P-256 keys for CA and leaf certificates |
| `crypto/tls` | stdlib | TLS configuration | Certificate loading and mTLS config structs |
| `database/sql` | stdlib | SQL interface | Standard Go database driver interface for SQLite |
| `embed` | stdlib | Embedded files | Placeholder `go:embed` for frontend assets (actual embedding in Phase 5) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `modernc.org/sqlite` | `mattn/go-sqlite3` | ~2x faster but requires CGO, breaks single-binary cross-compilation |
| `BurntSushi/toml` | `pelletier/go-toml` | go-toml is also good but BurntSushi is simpler for one-shot config load |
| `spf13/cobra` | `urfave/cli` | Both fine; cobra has larger ecosystem, more documentation |
| ECDSA P-256 | RSA 2048/4096 | RSA is larger keys/certs for no benefit in internal PKI; ECDSA is faster and modern |
| Argon2id | bcrypt | Argon2id is OWASP 2025+ recommendation; no 72-byte password limit |

**Installation:**
```bash
# Initialize Go module
go mod init github.com/claudeplane/claude-plane

# Phase 1 dependencies
go get modernc.org/sqlite@latest
go get google.golang.org/grpc@latest
go get google.golang.org/protobuf@latest
go get github.com/BurntSushi/toml@latest
go get github.com/spf13/cobra@latest
go get golang.org/x/crypto@latest
go get github.com/golang-jwt/jwt/v5@latest

# Protobuf code generators
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# buf CLI (system-level install)
# brew install bufbuild/buf/buf  OR
# go install github.com/bufbuild/buf/cmd/buf@latest
```

## Architecture Patterns

### Recommended Project Structure (Phase 1 deliverable)

```
claude-plane/
├── cmd/
│   ├── server/
│   │   └── main.go              # Server binary entrypoint (cobra root cmd)
│   └── agent/
│       └── main.go              # Agent binary entrypoint (cobra root cmd)
├── proto/
│   └── claudeplane/
│       └── v1/
│           └── agent.proto      # gRPC service + all message definitions
├── buf.yaml                     # buf module configuration
├── buf.gen.yaml                 # buf code generation config
├── internal/
│   ├── server/
│   │   ├── store/
│   │   │   ├── db.go            # Database initialization, connection pools
│   │   │   ├── migrations.go    # Schema creation (embedded SQL)
│   │   │   └── users.go         # User CRUD (for admin seeding)
│   │   └── config/
│   │       └── config.go        # Server config struct + TOML loading
│   ├── agent/
│   │   └── config/
│   │       └── config.go        # Agent config struct + TOML loading
│   └── shared/
│       ├── proto/               # Generated protobuf Go code (output of buf)
│       │   └── claudeplane/
│       │       └── v1/
│       │           ├── agent.pb.go
│       │           └── agent_grpc.pb.go
│       └── tlsutil/
│           ├── ca.go            # CA init, cert issuance logic
│           └── loader.go        # Cert/key file loading, tls.Config builders
├── go.mod
├── go.sum
└── Taskfile.yaml                # Task runner (optional, Makefile also fine)
```

### Pattern 1: Single-Writer SQLite with Separate Read Pool

**What:** Two `*sql.DB` instances -- one for writes (MaxOpenConns=1), one for reads (MaxOpenConns=N). All write operations go through the writer; all read operations go through the reader.

**When to use:** Always, for any Go application using SQLite in a concurrent context.

**Example:**
```go
// Source: SQLite concurrent writes analysis + modernc.org/sqlite docs
func NewStore(dbPath string) (*Store, error) {
    // Writer: single connection, IMMEDIATE transactions
    writerDSN := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON", dbPath)
    writer, err := sql.Open("sqlite", writerDSN)
    if err != nil {
        return nil, fmt.Errorf("open writer: %w", err)
    }
    writer.SetMaxOpenConns(1)

    // Reader: multiple connections for concurrent reads
    readerDSN := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON&mode=ro", dbPath)
    reader, err := sql.Open("sqlite", readerDSN)
    if err != nil {
        return nil, fmt.Errorf("open reader: %w", err)
    }
    reader.SetMaxOpenConns(4)

    return &Store{writer: writer, reader: reader}, nil
}
```

### Pattern 2: Cobra CLI with Subcommands

**What:** Use cobra to structure the server binary's CLI with subcommands for different operations: `serve`, `ca init`, `ca issue-server`, `ca issue-agent`, `seed-admin`.

**When to use:** Any Go binary that needs multiple operational modes.

**Example:**
```go
// Source: spf13/cobra standard patterns
func main() {
    rootCmd := &cobra.Command{
        Use:   "claude-plane-server",
        Short: "Control plane for Claude CLI sessions",
    }

    rootCmd.AddCommand(
        newServeCmd(),        // Start the server
        newCACmd(),           // CA tooling parent command
        newSeedAdminCmd(),    // Seed admin account
    )

    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

// CA subcommands: ca init, ca issue-server, ca issue-agent
func newCACmd() *cobra.Command {
    caCmd := &cobra.Command{Use: "ca", Short: "Certificate authority operations"}
    caCmd.AddCommand(
        newCAInitCmd(),
        newCAIssueServerCmd(),
        newCAIssueAgentCmd(),
    )
    return caCmd
}
```

### Pattern 3: X.509 Certificate Generation with Go stdlib

**What:** Generate self-signed CA, server, and agent certificates using `crypto/x509`, `crypto/ecdsa`, and `encoding/pem`. No external tooling (openssl, cfssl) needed.

**When to use:** Any project needing built-in PKI for mTLS.

**Example:**
```go
// Source: Go crypto/x509 package documentation
func GenerateCA(outDir string) error {
    // Generate ECDSA P-256 private key
    privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return fmt.Errorf("generate key: %w", err)
    }

    // CA certificate template
    template := &x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject: pkix.Name{
            CommonName:   "claude-plane-ca",
            Organization: []string{"claude-plane"},
        },
        NotBefore:             time.Now(),
        NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
        KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
        BasicConstraintsValid: true,
        IsCA:                  true,
        MaxPathLen:            1,
    }

    // Self-sign
    certDER, err := x509.CreateCertificate(rand.Reader, template, template,
        &privateKey.PublicKey, privateKey)
    if err != nil {
        return fmt.Errorf("create certificate: %w", err)
    }

    // Write PEM files to outDir...
    return writePEMFiles(outDir, certDER, privateKey)
}
```

### Pattern 4: TOML Config with Validation

**What:** Define Go structs matching the TOML structure, unmarshal, then validate required fields with clear error messages.

**When to use:** Both server and agent config loading.

**Example:**
```go
// Source: BurntSushi/toml documentation
type ServerConfig struct {
    HTTP     HTTPConfig     `toml:"http"`
    GRPC     GRPCConfig     `toml:"grpc"`
    TLS      TLSConfig      `toml:"tls"`
    Database DatabaseConfig `toml:"database"`
    Auth     AuthConfig     `toml:"auth"`
}

type HTTPConfig struct {
    Listen  string `toml:"listen"`
    TLSCert string `toml:"tls_cert"`
    TLSKey  string `toml:"tls_key"`
}

func LoadServerConfig(path string) (*ServerConfig, error) {
    var cfg ServerConfig
    if _, err := toml.DecodeFile(path, &cfg); err != nil {
        return nil, fmt.Errorf("parse config %s: %w", path, err)
    }
    if err := cfg.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }
    return &cfg, nil
}

func (c *ServerConfig) Validate() error {
    if c.HTTP.Listen == "" {
        return fmt.Errorf("http.listen is required")
    }
    if c.Database.Path == "" {
        return fmt.Errorf("database.path is required")
    }
    // ... validate each required field with clear message
    return nil
}
```

### Anti-Patterns to Avoid

- **Setting SQLite pragmas only once on first connection:** Pragmas must be set on every connection. Use the DSN string (`?_journal_mode=WAL&_busy_timeout=5000`) so they apply automatically, or use a `ConnInitFunc`.

- **Using `BEGIN` (deferred) for write transactions:** Use `BEGIN IMMEDIATE` so the write lock is acquired upfront. Deferred transactions that read-then-write cause lock upgrade deadlocks under concurrency.

- **Generating RSA keys for internal PKI:** ECDSA P-256 is faster, smaller certs, and sufficient for internal mTLS. No reason to use RSA 2048/4096.

- **Hardcoding cert validity to 1 year:** Use 10 years for the CA and 2 years for leaf certs. Short validity causes silent failures when certs expire months later.

- **One giant `.proto` file:** While Phase 1 can start with a single `agent.proto`, structure it with clear sections (registration, commands, events). The design doc already provides the complete message set -- implement it all in Phase 1 even though most messages are not used until later phases.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TOML parsing | Custom parser | `BurntSushi/toml` | Edge cases in TOML spec (multiline strings, datetime, dotted keys) |
| CLI argument parsing | `flag` stdlib | `spf13/cobra` | Subcommand trees, help text, flag inheritance, shell completion |
| Password hashing | `crypto/sha256` + salt | `golang.org/x/crypto/argon2` | Timing attacks, brute force resistance, memory-hardness |
| Protobuf codegen | Hand-written serialization | `buf generate` + `protoc-gen-go` | Binary compatibility, versioning, cross-language support |
| X.509 cert generation | Shelling out to `openssl` | `crypto/x509` + `crypto/ecdsa` | No external dependency, programmatic control, testable |
| SQL migrations | String concatenation | Embedded SQL files or `CREATE TABLE IF NOT EXISTS` | Schema versioning, repeatability |

**Key insight:** Phase 1 is all infrastructure. Every component here has a well-established library. The value is in correct integration and getting configuration right, not in novel code.

## Common Pitfalls

### Pitfall 1: SQLite "database is locked" from Day One

**What goes wrong:** Multiple goroutines open write transactions simultaneously on the same `*sql.DB` pool, causing `SQLITE_BUSY` errors that surface as API 500s.
**Why it happens:** Go's `database/sql` default `MaxOpenConns` is unlimited. Multiple connections each start a write transaction independently.
**How to avoid:** Single writer `*sql.DB` with `MaxOpenConns=1`, separate reader `*sql.DB`. Set `busy_timeout=5000` via DSN. Use `BEGIN IMMEDIATE` for writes.
**Warning signs:** Intermittent "database is locked" errors in server logs under any concurrent load.

### Pitfall 2: Pragmas Not Applied to All Connections

**What goes wrong:** WAL mode or busy_timeout is set on the first connection but not on connections created later by the pool. The pool opens new connections that use SQLite defaults (journal_mode=DELETE, busy_timeout=0).
**Why it happens:** `PRAGMA` statements are per-connection. Developers call `db.Exec("PRAGMA journal_mode=WAL")` once after `sql.Open()` -- this only affects one connection in the pool.
**How to avoid:** Set pragmas via the DSN string: `?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON`. The `modernc.org/sqlite` driver supports these DSN parameters. Alternatively, use a connector's `ConnInitFunc` to run pragmas on each new connection.
**Warning signs:** Foreign key constraints silently not enforced. WAL mode not active on some connections (check with `PRAGMA journal_mode;`).

### Pitfall 3: Certificate Serial Number Collisions

**What goes wrong:** Multiple agent certificates are generated with the same serial number (e.g., hardcoded to 1 or using a simple counter). Some TLS implementations reject certificates with duplicate serial numbers under the same CA.
**Why it happens:** Example code often uses `big.NewInt(1)` as the serial number. Works for demos, breaks in production when issuing multiple certs.
**How to avoid:** Generate cryptographically random serial numbers: `serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))`.
**Warning signs:** Intermittent TLS handshake failures when multiple agents use certs from the same CA.

### Pitfall 4: Missing Users Table for Admin Seeding

**What goes wrong:** The design doc (backend_v1.md) does not define a `users` table -- it references `user_id` columns throughout but defers multi-user to V2. AUTH-04 requires admin seeding, and AUTH-01/02/03 (Phase 3) require user accounts. If Phase 1 does not create the `users` table, Phase 3 must add it and backfill references.
**Why it happens:** The design doc treats V1 as "single user" and puts auth config in the TOML file. But the requirements specify a proper admin account with password hashing.
**How to avoid:** Create a `users` table in Phase 1 schema: `user_id, email, display_name, password_hash, role, created_at, updated_at`. The `seed-admin` command creates a row here.
**Warning signs:** The `seed-admin` command has nowhere to store the account.

### Pitfall 5: Protobuf Package Path Misconfiguration

**What goes wrong:** Generated Go code lands in the wrong package path, imports fail, or `buf generate` produces files that do not compile because `go_package` option is mismatched with the project's module path.
**Why it happens:** Protobuf's `option go_package` must align with the Go module path, and `buf.gen.yaml` managed mode can override it. Mismatches between these three (proto file option, buf.gen.yaml, go.mod) cause import errors.
**How to avoid:** Use buf managed mode in `buf.gen.yaml` to set `go_package_prefix` to match the Go module path. Set `option go_package` in the `.proto` file to the generated output path relative to the module root (e.g., `"github.com/claudeplane/claude-plane/internal/shared/proto/claudeplane/v1"`).
**Warning signs:** `go build` fails with import cycle errors or "package not found" after `buf generate`.

## Code Examples

### SQLite Schema (Phase 1 -- all tables)

```sql
-- Source: backend_v1.md design doc, adapted for Phase 1

-- Users (needed for AUTH-04 admin seeding, expanded in Phase 3)
CREATE TABLE IF NOT EXISTS users (
    user_id        TEXT PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    display_name   TEXT NOT NULL,
    password_hash  TEXT NOT NULL,        -- Argon2id hash
    role           TEXT NOT NULL DEFAULT 'user',  -- 'admin', 'user'
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Machines
CREATE TABLE IF NOT EXISTS machines (
    machine_id     TEXT PRIMARY KEY,
    display_name   TEXT,
    status         TEXT NOT NULL DEFAULT 'disconnected',
    max_sessions   INTEGER NOT NULL DEFAULT 5,
    last_health    TEXT,
    last_seen_at   DATETIME,
    cert_expires_at DATETIME,            -- Track cert expiry (Pitfall 6)
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Sessions
CREATE TABLE IF NOT EXISTS sessions (
    session_id     TEXT PRIMARY KEY,
    machine_id     TEXT NOT NULL REFERENCES machines(machine_id),
    user_id        TEXT REFERENCES users(user_id),
    status         TEXT NOT NULL DEFAULT 'starting',
    command        TEXT NOT NULL DEFAULT 'claude',
    args           TEXT,
    working_dir    TEXT,
    initial_prompt TEXT,
    exit_code      INTEGER,
    started_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at       DATETIME,
    scrollback_bytes INTEGER DEFAULT 0,
    run_step_id    TEXT
);

CREATE INDEX IF NOT EXISTS idx_sessions_machine ON sessions(machine_id, status);

-- Jobs
CREATE TABLE IF NOT EXISTS jobs (
    job_id         TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    description    TEXT,
    user_id        TEXT REFERENCES users(user_id),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Steps
CREATE TABLE IF NOT EXISTS steps (
    step_id        TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    prompt         TEXT NOT NULL,
    machine_id     TEXT REFERENCES machines(machine_id),
    working_dir    TEXT,
    command        TEXT DEFAULT 'claude',
    args           TEXT,
    sort_order     INTEGER NOT NULL,
    timeout_seconds INTEGER,
    expected_outputs TEXT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_steps_job ON steps(job_id, sort_order);

-- Step dependencies
CREATE TABLE IF NOT EXISTS step_dependencies (
    step_id        TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    depends_on     TEXT NOT NULL REFERENCES steps(step_id) ON DELETE CASCADE,
    PRIMARY KEY (step_id, depends_on),
    CHECK (step_id != depends_on)
);

-- Runs
CREATE TABLE IF NOT EXISTS runs (
    run_id         TEXT PRIMARY KEY,
    job_id         TEXT NOT NULL REFERENCES jobs(job_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    trigger_type   TEXT NOT NULL,
    trigger_detail TEXT,
    started_at     DATETIME,
    ended_at       DATETIME,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_runs_job ON runs(job_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);

-- Run steps
CREATE TABLE IF NOT EXISTS run_steps (
    run_step_id    TEXT PRIMARY KEY,
    run_id         TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
    step_id        TEXT NOT NULL REFERENCES steps(step_id),
    status         TEXT NOT NULL DEFAULT 'pending',
    machine_id     TEXT REFERENCES machines(machine_id),
    session_id     TEXT,
    exit_code      INTEGER,
    started_at     DATETIME,
    ended_at       DATETIME,
    error_message  TEXT
);

CREATE INDEX IF NOT EXISTS idx_run_steps_run ON run_steps(run_id);

-- Credentials
CREATE TABLE IF NOT EXISTS credentials (
    credential_id  TEXT PRIMARY KEY,
    user_id        TEXT REFERENCES users(user_id),
    name           TEXT NOT NULL,
    encrypted_value BLOB NOT NULL,
    nonce          BLOB NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Audit log
CREATE TABLE IF NOT EXISTS audit_log (
    log_id         INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_id        TEXT,
    action         TEXT NOT NULL,
    resource_type  TEXT,
    resource_id    TEXT,
    detail         TEXT
);

CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_log(timestamp DESC);
```

### Admin Seeding with Argon2id

```go
// Source: golang.org/x/crypto/argon2 package, OWASP recommendations
import "golang.org/x/crypto/argon2"

func HashPassword(password string) (string, error) {
    salt := make([]byte, 16)
    if _, err := rand.Read(salt); err != nil {
        return "", err
    }
    // OWASP recommended: time=1, memory=64MB, threads=4, keyLen=32
    hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

    // Encode as $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
    return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
        argon2.Version, 64*1024, 1, 4,
        base64.RawStdEncoding.EncodeToString(salt),
        base64.RawStdEncoding.EncodeToString(hash),
    ), nil
}
```

### buf.gen.yaml Configuration

```yaml
# Source: buf.build official documentation
version: v2
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/claudeplane/claude-plane/internal/shared/proto
plugins:
  - remote: buf.build/protocolbuffers/go
    out: internal/shared/proto
    opt: paths=source_relative
  - remote: buf.build/grpc/go
    out: internal/shared/proto
    opt: paths=source_relative
```

### buf.yaml Configuration

```yaml
version: v2
modules:
  - path: proto
deps:
  - buf.build/googleapis/googleapis
lint:
  use:
    - DEFAULT
breaking:
  use:
    - FILE
```

### Server Config Example (server.toml)

```toml
# Minimum viable server configuration for Phase 1

[http]
listen = "0.0.0.0:8443"
tls_cert = "/etc/claude-plane/server-cert/server.pem"
tls_key = "/etc/claude-plane/server-cert/server-key.pem"

[grpc]
listen = "0.0.0.0:9443"

[tls]
ca_cert = "/etc/claude-plane/ca/ca.pem"
server_cert = "/etc/claude-plane/server-cert/server.pem"
server_key = "/etc/claude-plane/server-cert/server-key.pem"

[database]
path = "/var/lib/claude-plane/server.db"

[machines]
allowed = ["nuc-01", "nuc-02"]
```

### Agent Config Example (agent.toml)

```toml
[server]
address = "controlplane.example.com:9443"
reconnect_min_interval = "1s"
reconnect_max_interval = "60s"

[tls]
ca_cert = "/etc/claude-plane/certs/ca.pem"
agent_cert = "/etc/claude-plane/certs/agent.pem"
agent_key = "/etc/claude-plane/certs/agent-key.pem"

[agent]
machine_id = "nuc-01"
data_dir = "/var/lib/claude-plane"
max_sessions = 5
claude_cli_path = "/usr/local/bin/claude"

[health]
report_interval = "10s"
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `mattn/go-sqlite3` (CGO) | `modernc.org/sqlite` (pure Go) | 2023+ | Enables CGO_ENABLED=0 cross-compilation |
| bcrypt for passwords | Argon2id | OWASP 2024+ | Memory-hard, no 72-byte limit |
| Raw `protoc` invocation | `buf` CLI | 2022+ | Faster builds, managed mode, lint + breaking change detection |
| RSA 2048 for internal PKI | ECDSA P-256 | Industry trend | Smaller certs, faster handshake |
| `PRAGMA` statements after sql.Open | DSN string parameters | modernc.org/sqlite feature | Applies to every connection in pool automatically |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + `go test` |
| Config file | None -- Go's testing uses `*_test.go` files with no config |
| Quick run command | `go test ./internal/... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INFR-01 | `go build` produces server binary | build | `go build -o /dev/null ./cmd/server` | Wave 0 |
| INFR-02 | `go build` produces agent binary | build | `go build -o /dev/null ./cmd/agent` | Wave 0 |
| INFR-03 | SQLite initializes with WAL mode, creates tables | unit | `go test ./internal/server/store/... -run TestInitDB -count=1` | Wave 0 |
| INFR-04 | Config parsing succeeds with valid TOML, fails with clear error on missing fields | unit | `go test ./internal/server/config/... ./internal/agent/config/... -count=1` | Wave 0 |
| AGNT-01 | CA init generates CA cert+key; issue-server/issue-agent generate valid leaf certs; certs pass mTLS handshake | unit | `go test ./internal/shared/tlsutil/... -count=1` | Wave 0 |
| AUTH-04 | seed-admin creates user record with Argon2id hash; duplicate seed fails gracefully | unit | `go test ./internal/server/store/... -run TestSeedAdmin -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps
- [ ] `internal/server/store/db_test.go` -- covers INFR-03, AUTH-04
- [ ] `internal/server/config/config_test.go` -- covers INFR-04 (server)
- [ ] `internal/agent/config/config_test.go` -- covers INFR-04 (agent)
- [ ] `internal/shared/tlsutil/ca_test.go` -- covers AGNT-01

## Open Questions

1. **Go module path**
   - What we know: CLAUDE.md shows `go build -o claude-plane-server ./cmd/server` but does not specify the module path.
   - What's unclear: Whether to use `github.com/claudeplane/claude-plane`, `github.com/joseibanez/claude-plane`, or a shorter path.
   - Recommendation: Use whatever matches the actual GitHub repository path. If undecided, use `github.com/claudeplane/claude-plane`.

2. **Users table scope in Phase 1**
   - What we know: The design doc does not define a users table. AUTH-04 requires admin seeding. AUTH-01/02/03 in Phase 3 require user accounts.
   - What's unclear: Whether to create a minimal users table (just for admin) or the full table now.
   - Recommendation: Create the full users table now (`user_id, email, display_name, password_hash, role, created_at, updated_at`). It is cheap and avoids a migration in Phase 3.

3. **Schema approach: all tables now vs. per-phase?**
   - What we know: The design doc defines all tables. Phase 1 only directly needs `users` and `machines`. Other tables are needed in Phases 3-6.
   - What's unclear: Whether to create all tables in Phase 1 or add them incrementally.
   - Recommendation: Create all tables in Phase 1. The schema is the contract. Having all tables from the start means Phase 2-6 only add code, not schema changes. The tables are empty and cost nothing.

4. **Protobuf: full contract now or incremental?**
   - What we know: The design doc defines the complete protobuf service and all messages. Phase 1 only needs the definitions to compile and be importable.
   - What's unclear: Whether to define all messages now or add them per phase.
   - Recommendation: Define the complete protobuf contract in Phase 1. Messages are the shared API between server and agent. Defining them all now means both binaries can import the generated types from day one. Adding messages later is fine (protobuf is additive), but having them all now avoids churn.

## Sources

### Primary (HIGH confidence)
- `docs/internal/product/backend_v1.md` -- complete protobuf definitions, schema, config structure, CA CLI design
- `modernc.org/sqlite` -- DSN parameter format for pragma settings
- Go stdlib `crypto/x509` -- certificate generation API
- `BurntSushi/toml` v1.5.0 -- TOML parsing API
- `spf13/cobra` -- CLI subcommand patterns
- OWASP Argon2id recommendation -- password hashing parameters

### Secondary (MEDIUM confidence)
- SQLite WAL concurrent writes analysis (tenthousandmeters.com) -- single writer pattern rationale
- buf.build documentation -- `buf.gen.yaml` managed mode configuration

### Tertiary (LOW confidence)
- None -- all Phase 1 technologies are well-documented with official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries are mature, well-documented, and specified in project design docs
- Architecture: HIGH -- project structure follows Go conventions and is defined in CLAUDE.md and design docs
- Pitfalls: HIGH -- SQLite concurrency and cert management pitfalls are well-documented with verified root causes
- Protobuf contract: HIGH -- complete message definitions exist in backend_v1.md

**Research date:** 2026-03-11
**Valid until:** 2026-04-11 (stable technologies, 30-day validity)
