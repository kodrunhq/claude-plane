# Phase 3: Server Core - Research

**Researched:** 2026-03-11
**Domain:** Go HTTP REST API (chi router), JWT authentication with token revocation, gRPC agent connection manager, SQLite-backed user accounts
**Confidence:** HIGH

## Summary

Phase 3 builds the server's HTTP-facing layer and agent connection tracking. It covers three distinct subsystems: (1) user authentication with registration, login (JWT issuance), and logout (JWT revocation), (2) an HTTP REST API with JWT middleware protecting all endpoints, and (3) a server-side agent connection manager that accepts gRPC streams from agents, tracks their online/offline status, and exposes machine listings through the REST API.

Phase 1 provides the SQLite store with the `users` and `machines` tables, the Argon2id password hashing utility, the TOML config, and the cobra CLI scaffold. Phase 2 provides the gRPC server setup with mTLS and the agent-side client. Phase 3 builds on Phase 1 directly (it depends on Phase 1 per the roadmap). The gRPC server listener and mTLS interceptor patterns from Phase 2 research are relevant -- Phase 3 implements the server-side agent connection manager that accepts and tracks those connections.

The key design tension is that the backend design doc specifies "Basic Auth + TLS" for V1, but the requirements (AUTH-01, AUTH-02, AUTH-03) explicitly call for user accounts with email/password, JWT tokens, and token invalidation. The requirements take precedence. JWT with short-lived access tokens and an in-memory revocation list (backed by SQLite for server restart persistence) is the right approach for a single-server deployment with no Redis dependency.

**Primary recommendation:** Use `go-chi/chi` v5 as the HTTP router with custom JWT middleware built on `golang-jwt/jwt/v5`. Implement token revocation via an in-memory blocklist synced to a `revoked_tokens` SQLite table. Keep the connection manager as a thread-safe in-memory map with DB-backed status updates.

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| AUTH-01 | User can create account with email and password | `POST /api/v1/auth/register` endpoint, `users` table from Phase 1 schema, Argon2id hashing from Phase 1, email uniqueness constraint |
| AUTH-02 | User can log in and receive a JWT session token | `POST /api/v1/auth/login` endpoint, `golang-jwt/jwt/v5` for token signing with HMAC-SHA256, short-lived tokens (15-60 min) |
| AUTH-03 | User can log out and invalidate their session | `POST /api/v1/auth/logout` endpoint, in-memory token blocklist + `revoked_tokens` SQLite table for persistence, check blocklist in JWT middleware |
| AGNT-04 | Server displays list of connected agents with online/offline status | `GET /api/v1/machines` endpoint, `ConnectionManager` tracks connected agent streams in-memory, updates `machines.status` and `machines.last_seen_at` in DB |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go-chi/chi` | v5.2.5 | HTTP router | Lightweight, idiomatic, `net/http` compatible, 17k+ importers, built-in middleware (logger, recoverer, CORS) |
| `golang-jwt/jwt` | v5 | JWT creation and validation | Most popular Go JWT library, supports typed claims, parser options for `alg` validation |
| `golang.org/x/crypto/argon2` | (from Phase 1) | Password hashing | Already established in Phase 1 for admin seeding |
| `modernc.org/sqlite` | (from Phase 1) | SQLite driver | Already established in Phase 1 |
| `google.golang.org/grpc` | v1.79+ (from Phase 1) | gRPC server for agent connections | Already established in Phase 1/2 |
| `log/slog` | stdlib | Structured logging | Consistent with prior phases |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `chi/middleware` | (part of chi) | Request logging, panic recovery, CORS | Apply to all HTTP routes |
| `encoding/json` | stdlib | JSON request/response encoding | REST API handlers |
| `crypto/rand` | stdlib | UUID generation, JWT secret generation | User IDs, token JTI claims |
| `sync` | stdlib | Mutex for connection manager and token blocklist | Thread-safe in-memory state |
| `net/http` | stdlib | HTTP server, handler interface | Chi is a wrapper; handlers are standard `http.Handler` |
| `google/uuid` | v1.6+ | UUID generation | User IDs, machine IDs -- if not already using `crypto/rand` directly |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `go-chi/chi` | `net/http` (Go 1.22+ mux) | Go 1.22 mux works but chi provides middleware chaining, route groups, and URL params out of the box |
| `go-chi/chi` | `gorilla/mux` | Gorilla was unmaintained for a period; chi is more actively maintained and lighter |
| Custom JWT middleware | `go-chi/jwtauth` | jwtauth depends on `lestrrat-go/jwx` (different JWT library); using `golang-jwt/jwt/v5` directly is simpler and avoids extra dependencies |
| In-memory blocklist | Redis | Redis is overkill for single-server; in-memory + SQLite persistence achieves the same for this deployment model |
| HMAC-SHA256 signing | RSA/ECDSA signing | Asymmetric keys needed only for multi-service verification; single server uses symmetric HMAC |

**Installation:**
```bash
# Phase 3 new dependencies (on top of Phase 1)
go get github.com/go-chi/chi/v5@latest
go get github.com/google/uuid@latest

# Already available from Phase 1:
# github.com/golang-jwt/jwt/v5
# golang.org/x/crypto
# modernc.org/sqlite
# google.golang.org/grpc
# github.com/spf13/cobra
# github.com/BurntSushi/toml
```

## Architecture Patterns

### Recommended Project Structure (Phase 3 additions)

```
internal/
├── server/
│   ├── api/
│   │   ├── router.go         # Chi router setup, route groups, middleware stack
│   │   ├── auth_handler.go   # POST /auth/register, /auth/login, /auth/logout
│   │   ├── machine_handler.go # GET /machines, GET /machines/:id
│   │   ├── middleware.go      # JWT auth middleware, request ID, logging
│   │   └── response.go       # JSON response helpers (success, error, paginated)
│   ├── auth/
│   │   ├── jwt.go            # JWT service: issue, validate, parse claims
│   │   └── blocklist.go      # Token revocation: in-memory + SQLite persistence
│   ├── connmgr/
│   │   └── manager.go        # Agent connection manager: track streams, online/offline
│   ├── store/
│   │   ├── db.go             # (Phase 1) Database init
│   │   ├── migrations.go     # (Phase 1) Schema
│   │   ├── users.go          # (Phase 1) User CRUD -- expand with login queries
│   │   ├── machines.go       # Machine status queries (update status, list with health)
│   │   └── tokens.go         # Revoked token persistence (store/load blocklist)
│   ├── grpc/
│   │   ├── server.go         # (Phase 2) gRPC server setup
│   │   ├── auth.go           # (Phase 2) mTLS interceptor
│   │   ├── connection_mgr.go # (Phase 2) -- now integrates with connmgr package
│   │   └── service.go        # AgentService implementation: Register, AgentStream
│   └── config/
│       └── config.go         # (Phase 1) -- add auth config section (JWT secret, token TTL)
└── shared/
    ├── proto/                # (Phase 1) Generated protobuf code
    └── tlsutil/              # (Phase 1) Cert loading
```

### Pattern 1: Chi Router with JWT Middleware and Route Groups

**What:** Use chi's route grouping to separate public routes (register, login) from protected routes (everything else). The JWT middleware validates the token and injects the user into the request context.

**When to use:** Every HTTP endpoint except auth.

**Example:**
```go
// Source: go-chi/chi v5 patterns + golang-jwt/jwt/v5
func NewRouter(authSvc *auth.Service, handlers *Handlers) chi.Router {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.RequestID)
    r.Use(middleware.RealIP)
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Public routes (no auth required)
    r.Route("/api/v1/auth", func(r chi.Router) {
        r.Post("/register", handlers.Register)
        r.Post("/login", handlers.Login)
    })

    // Protected routes (JWT required)
    r.Group(func(r chi.Router) {
        r.Use(JWTAuthMiddleware(authSvc))

        r.Post("/api/v1/auth/logout", handlers.Logout)

        r.Route("/api/v1/machines", func(r chi.Router) {
            r.Get("/", handlers.ListMachines)
            r.Get("/{machineID}", handlers.GetMachine)
        })
    })

    return r
}
```

### Pattern 2: JWT Service with Token Issuance and Validation

**What:** A service struct encapsulating JWT signing key, token TTL, and the blocklist. Issues tokens on login, validates on every request, revokes on logout.

**When to use:** All authentication operations.

**Example:**
```go
// Source: golang-jwt/jwt/v5 documentation
type Claims struct {
    jwt.RegisteredClaims
    UserID string `json:"uid"`
    Email  string `json:"email"`
    Role   string `json:"role"`
}

type Service struct {
    signingKey []byte
    tokenTTL   time.Duration
    blocklist  *Blocklist
}

func (s *Service) IssueToken(user *store.User) (string, error) {
    now := time.Now()
    jti := uuid.New().String()

    claims := Claims{
        RegisteredClaims: jwt.RegisteredClaims{
            ID:        jti,
            Subject:   user.UserID,
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(s.tokenTTL)),
            Issuer:    "claude-plane",
        },
        UserID: user.UserID,
        Email:  user.Email,
        Role:   user.Role,
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(s.signingKey)
}

func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{},
        func(t *jwt.Token) (interface{}, error) {
            // Validate signing method
            if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
            }
            return s.signingKey, nil
        },
        jwt.WithValidMethods([]string{"HS256"}),
        jwt.WithIssuer("claude-plane"),
    )
    if err != nil {
        return nil, err
    }

    claims, ok := token.Claims.(*Claims)
    if !ok || !token.Valid {
        return nil, fmt.Errorf("invalid token claims")
    }

    // Check blocklist
    if s.blocklist.IsRevoked(claims.ID) {
        return nil, fmt.Errorf("token has been revoked")
    }

    return claims, nil
}

func (s *Service) RevokeToken(jti string, expiresAt time.Time) error {
    return s.blocklist.Add(jti, expiresAt)
}
```

### Pattern 3: In-Memory Token Blocklist with SQLite Persistence

**What:** A map of revoked token JTIs with expiration times. Checked on every request. Expired entries are periodically cleaned. Backed by SQLite so revocations survive server restarts.

**When to use:** JWT logout/revocation.

**Example:**
```go
type Blocklist struct {
    mu      sync.RWMutex
    entries map[string]time.Time // jti -> expires_at
    store   *store.TokenStore    // SQLite persistence
}

func NewBlocklist(tokenStore *store.TokenStore) (*Blocklist, error) {
    b := &Blocklist{
        entries: make(map[string]time.Time),
        store:   tokenStore,
    }
    // Load persisted revocations on startup
    revoked, err := tokenStore.LoadActiveRevocations()
    if err != nil {
        return nil, err
    }
    for _, r := range revoked {
        b.entries[r.JTI] = r.ExpiresAt
    }
    return b, nil
}

func (b *Blocklist) Add(jti string, expiresAt time.Time) error {
    b.mu.Lock()
    b.entries[jti] = expiresAt
    b.mu.Unlock()
    return b.store.RevokeToken(jti, expiresAt)
}

func (b *Blocklist) IsRevoked(jti string) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    _, exists := b.entries[jti]
    return exists
}

// Cleanup removes expired entries periodically
func (b *Blocklist) StartCleanup(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    go func() {
        for {
            select {
            case <-ctx.Done():
                ticker.Stop()
                return
            case now := <-ticker.C:
                b.mu.Lock()
                for jti, exp := range b.entries {
                    if now.After(exp) {
                        delete(b.entries, jti)
                    }
                }
                b.mu.Unlock()
                b.store.CleanExpired(now)
            }
        }
    }()
}
```

### Pattern 4: Agent Connection Manager

**What:** A thread-safe in-memory registry mapping machine-id to the active gRPC stream and agent metadata. Updates the `machines` table in SQLite when agents connect or disconnect.

**When to use:** Server-side agent tracking (AGNT-04).

**Example:**
```go
type ConnectedAgent struct {
    MachineID    string
    Stream       pb.AgentService_AgentStreamServer
    RegisteredAt time.Time
    MaxSessions  int32
    LastHealth   *pb.HealthEvent
    cancel       context.CancelFunc
}

type ConnectionManager struct {
    mu     sync.RWMutex
    agents map[string]*ConnectedAgent // machine-id -> agent
    store  *store.MachineStore
    logger *slog.Logger
}

func (cm *ConnectionManager) Register(machineID string, agent *ConnectedAgent) error {
    cm.mu.Lock()
    // If already connected, close old stream
    if old, exists := cm.agents[machineID]; exists {
        cm.logger.Warn("replacing existing connection", "machine_id", machineID)
        old.cancel()
    }
    cm.agents[machineID] = agent
    cm.mu.Unlock()

    // Update DB status
    return cm.store.UpdateMachineStatus(machineID, "connected", time.Now())
}

func (cm *ConnectionManager) Disconnect(machineID string) {
    cm.mu.Lock()
    delete(cm.agents, machineID)
    cm.mu.Unlock()

    cm.store.UpdateMachineStatus(machineID, "disconnected", time.Now())
    cm.logger.Info("agent disconnected", "machine_id", machineID)
}

func (cm *ConnectionManager) GetAgent(machineID string) *ConnectedAgent {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    return cm.agents[machineID]
}

func (cm *ConnectionManager) ListAgents() []AgentInfo {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    result := make([]AgentInfo, 0, len(cm.agents))
    for id, agent := range cm.agents {
        result = append(result, AgentInfo{
            MachineID:   id,
            Status:      "connected",
            MaxSessions: agent.MaxSessions,
            LastHealth:  agent.LastHealth,
            ConnectedAt: agent.RegisteredAt,
        })
    }
    return result
}
```

### Pattern 5: JWT Auth Middleware

**What:** An HTTP middleware that extracts the JWT from the `Authorization: Bearer <token>` header, validates it, and injects the claims into the request context. Returns 401 for missing or invalid tokens.

**When to use:** All protected routes.

**Example:**
```go
type contextKey string

const UserClaimsKey contextKey = "user_claims"

func JWTAuthMiddleware(authSvc *auth.Service) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" {
                http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
                return
            }

            parts := strings.SplitN(authHeader, " ", 2)
            if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
                http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
                return
            }

            claims, err := authSvc.ValidateToken(parts[1])
            if err != nil {
                http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
                return
            }

            ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func GetClaims(r *http.Request) *auth.Claims {
    claims, _ := r.Context().Value(UserClaimsKey).(*auth.Claims)
    return claims
}
```

### Anti-Patterns to Avoid

- **Storing JWT signing key in source code or config file in plaintext:** Generate a random 256-bit key and store it in a file referenced by config (similar to `master_key_file` pattern). Auto-generate on first run if missing.

- **Long-lived JWT tokens without revocation:** Short TTL (15-60 minutes) combined with the blocklist. Do not issue tokens valid for days/weeks without a refresh mechanism.

- **Checking machine status only from DB:** The `machines` table reflects persistent state. The connection manager holds real-time state. The `GET /machines` endpoint should merge both: DB for display names and metadata, connection manager for live online/offline status.

- **Not validating the JWT signing algorithm:** Always pass `jwt.WithValidMethods([]string{"HS256"})` to the parser. Without this, an attacker can specify `alg: none` and bypass signature verification.

- **One global mutex for the connection manager and all handlers:** Use `sync.RWMutex` -- reads (listing agents, getting agent) use `RLock`, writes (register, disconnect) use `Lock`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP routing | Custom `http.ServeMux` patterns | `go-chi/chi` v5 | Route params, middleware chaining, route groups, built-in recoverer |
| JWT signing/validation | Manual HMAC computation on JSON | `golang-jwt/jwt/v5` | Claims validation, expiry handling, algorithm verification |
| Password hashing | Custom salt + hash | `golang.org/x/crypto/argon2` (Phase 1) | Memory-hardness, timing attack resistance |
| Request logging | `fmt.Println` in handlers | `chi/middleware.Logger` | Structured, includes status code, latency, request ID |
| JSON encoding with error wrapping | Repeated `json.NewEncoder(w).Encode()` | Response helper functions | Consistent Content-Type, error format, HTTP status |
| Token revocation | Custom database polling | In-memory blocklist with periodic DB sync | O(1) check per request, no DB query per request |

**Key insight:** Phase 3's complexity is in correctly wiring the pieces together -- JWT issuance, validation middleware, connection manager events, and REST handlers. Each individual piece is simple. The value is in clean separation and correct concurrency.

## Common Pitfalls

### Pitfall 1: JWT Secret Too Short or Predictable

**What goes wrong:** Using a short or predictable JWT signing key allows brute-force attacks. An attacker can try common secrets and forge tokens.
**Why it happens:** Developers use string literals like "secret" or short config values during development and forget to change them.
**How to avoid:** Generate a 256-bit (32-byte) random key: `crypto/rand.Read(make([]byte, 32))`. Store in a key file. The server startup should refuse to start if the key is missing. Auto-generate on first run.
**Warning signs:** JWT secret visible in config files committed to git.

### Pitfall 2: Token Revocation Not Surviving Server Restart

**What goes wrong:** Tokens revoked via logout are only stored in memory. Server restart clears the blocklist, and previously-revoked tokens become valid again until they expire.
**Why it happens:** In-memory-only blocklist implementation.
**How to avoid:** Persist revocations to the `revoked_tokens` SQLite table. On startup, load all non-expired revocations into the in-memory map.
**Warning signs:** Logout works but tokens become valid again after server restart.

### Pitfall 3: Agent Connection Manager Race with REST API

**What goes wrong:** A REST API request lists machines, reads the connection manager, and the agent disconnects between reading the in-memory map and querying the DB. The response shows inconsistent state.
**Why it happens:** Mixing in-memory and DB state without a consistent snapshot.
**How to avoid:** The machine list endpoint should query the DB (persistent state) and overlay live status from the connection manager. The connection manager always updates the DB first, then the in-memory map. The REST handler reads the DB, then enriches with in-memory data.
**Warning signs:** Machines shown as "connected" in the API but returning errors when commands are sent.

### Pitfall 4: Missing `alg` Validation in JWT Parsing

**What goes wrong:** An attacker crafts a JWT with `"alg": "none"` and no signature. If the parser does not validate the algorithm, it accepts the forged token.
**Why it happens:** `jwt.Parse` without `jwt.WithValidMethods` does not enforce algorithm constraints.
**How to avoid:** Always use `jwt.WithValidMethods([]string{"HS256"})` as a parser option. Additionally, check the signing method in the `Keyfunc` callback.
**Warning signs:** Security audit flags JWT parsing without algorithm validation.

### Pitfall 5: gRPC Service Implementation Not Updating Connection Manager

**What goes wrong:** The `Register()` and `AgentStream()` RPCs are implemented but do not notify the connection manager. The machine list shows all agents as offline even when they are connected.
**Why it happens:** Phase 2 research focused on the agent side. The server-side `AgentService` implementation must wire into the connection manager.
**How to avoid:** The `AgentService.Register()` handler calls `connMgr.Register()`. The `AgentStream()` handler defers `connMgr.Disconnect()`. This is the integration point between the gRPC layer and the REST layer.
**Warning signs:** `GET /api/v1/machines` always returns `"status": "disconnected"`.

### Pitfall 6: Argon2id Verification Slowness on Login

**What goes wrong:** Argon2id is deliberately slow (that's the point for password hashing). With the recommended parameters (memory=64MB, time=1, threads=4), each login request takes ~100-300ms for password verification.
**Why it happens:** Memory-hard hashing is computationally expensive by design.
**How to avoid:** This is expected and acceptable. Do NOT reduce parameters. If concerned about DoS, add rate limiting to the login endpoint (e.g., 5 attempts per minute per IP).
**Warning signs:** High latency on login endpoint only. This is normal.

## Code Examples

### Database Schema Addition (revoked_tokens table)

```sql
-- Add to Phase 1 schema or as Phase 3 migration
CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti         TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    revoked_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  DATETIME NOT NULL  -- When the token would have expired naturally
);

CREATE INDEX IF NOT EXISTS idx_revoked_expires ON revoked_tokens(expires_at);
```

### Server Config Addition (auth section)

```toml
[auth]
jwt_secret_file = "/etc/claude-plane/jwt.key"  # 256-bit random key
token_ttl = "60m"                               # Access token lifetime
```

### Registration Handler

```go
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
        Name     string `json:"display_name"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Validate
    if req.Email == "" || req.Password == "" {
        writeError(w, http.StatusBadRequest, "email and password are required")
        return
    }
    if len(req.Password) < 8 {
        writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
        return
    }

    // Hash password
    hash, err := auth.HashPassword(req.Password)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    // Create user
    user := &store.User{
        UserID:      uuid.New().String(),
        Email:       req.Email,
        DisplayName: req.Name,
        PasswordHash: hash,
        Role:        "user",
    }
    if err := h.store.CreateUser(user); err != nil {
        if isUniqueViolation(err) {
            writeError(w, http.StatusConflict, "email already registered")
            return
        }
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    writeJSON(w, http.StatusCreated, map[string]string{
        "user_id": user.UserID,
        "email":   user.Email,
    })
}
```

### Login Handler

```go
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    user, err := h.store.GetUserByEmail(req.Email)
    if err != nil {
        writeError(w, http.StatusUnauthorized, "invalid credentials")
        return
    }

    if !auth.VerifyPassword(req.Password, user.PasswordHash) {
        writeError(w, http.StatusUnauthorized, "invalid credentials")
        return
    }

    token, err := h.authSvc.IssueToken(user)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    writeJSON(w, http.StatusOK, map[string]interface{}{
        "token":   token,
        "user_id": user.UserID,
        "email":   user.Email,
        "role":    user.Role,
    })
}
```

### Logout Handler

```go
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
    claims := GetClaims(r)
    if claims == nil {
        writeError(w, http.StatusUnauthorized, "no claims in context")
        return
    }

    if err := h.authSvc.RevokeToken(claims.ID, claims.ExpiresAt.Time); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to revoke token")
        return
    }

    writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}
```

### Machine List Handler

```go
func (h *Handlers) ListMachines(w http.ResponseWriter, r *http.Request) {
    // Get all machines from DB (persistent state)
    machines, err := h.store.ListMachines()
    if err != nil {
        writeError(w, http.StatusInternalServerError, "failed to list machines")
        return
    }

    // Overlay live status from connection manager
    for i := range machines {
        if agent := h.connMgr.GetAgent(machines[i].MachineID); agent != nil {
            machines[i].Status = "connected"
            machines[i].LastSeenAt = time.Now()
            if agent.LastHealth != nil {
                machines[i].ActiveSessions = int(agent.LastHealth.ActiveSessions)
                machines[i].CPUUsage = agent.LastHealth.CpuUsagePercent
            }
        }
    }

    writeJSON(w, http.StatusOK, machines)
}
```

### JSON Response Helpers

```go
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]string{"error": message})
}
```

### AgentService gRPC Integration with Connection Manager

```go
func (s *AgentServiceServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
    machineID := getMachineIDFromContext(ctx) // From mTLS interceptor

    // Verify machine-id in request matches cert
    if req.MachineId != machineID {
        return &pb.RegisterResponse{
            Accepted:     false,
            RejectReason: "machine-id mismatch with certificate",
        }, nil
    }

    // Ensure machine exists in DB (upsert)
    s.store.UpsertMachine(machineID, req.MaxSessions)

    s.logger.Info("agent registered", "machine_id", machineID, "max_sessions", req.MaxSessions)

    return &pb.RegisterResponse{
        Accepted:      true,
        ServerVersion: version.String(),
    }, nil
}

func (s *AgentServiceServer) AgentStream(stream pb.AgentService_AgentStreamServer) error {
    machineID := getMachineIDFromContext(stream.Context())

    ctx, cancel := context.WithCancel(stream.Context())
    agent := &connmgr.ConnectedAgent{
        MachineID:    machineID,
        Stream:       stream,
        RegisteredAt: time.Now(),
        cancel:       cancel,
    }

    if err := s.connMgr.Register(machineID, agent); err != nil {
        cancel()
        return err
    }
    defer s.connMgr.Disconnect(machineID)

    // Event loop: receive agent events
    for {
        event, err := stream.Recv()
        if err != nil {
            s.logger.Info("agent stream ended", "machine_id", machineID, "error", err)
            return err
        }
        s.handleAgentEvent(ctx, machineID, event)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `gorilla/mux` | `go-chi/chi` v5 | 2023+ (gorilla maintenance gap) | Chi is lighter, better maintained, `net/http` compatible |
| `dgrijalva/jwt-go` | `golang-jwt/jwt/v5` | 2021 (fork), v5 in 2023 | Active maintenance, typed claims, better validation |
| HTTP Basic Auth for APIs | JWT Bearer tokens | Industry standard | Stateless (mostly), frontend-friendly, supports claims |
| Single `*sql.DB` for reads and writes | Separate reader/writer pools | SQLite best practice | Prevents lock contention in concurrent server |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + `go test` |
| Config file | None -- Go uses `*_test.go` files |
| Quick run command | `go test ./internal/server/api/... ./internal/server/auth/... ./internal/server/connmgr/... -count=1 -short` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| AUTH-01 | Register creates user with hashed password; rejects duplicate email | unit | `go test ./internal/server/api/... -run TestRegister -count=1` | Wave 0 |
| AUTH-01 | Rejects invalid input (empty email, short password) | unit | `go test ./internal/server/api/... -run TestRegisterValidation -count=1` | Wave 0 |
| AUTH-02 | Login returns JWT for valid credentials; rejects invalid | unit | `go test ./internal/server/api/... -run TestLogin -count=1` | Wave 0 |
| AUTH-02 | JWT contains correct claims (user_id, email, role, exp) | unit | `go test ./internal/server/auth/... -run TestIssueToken -count=1` | Wave 0 |
| AUTH-03 | Logout revokes token; subsequent requests with revoked token get 401 | integration | `go test ./internal/server/api/... -run TestLogout -count=1` | Wave 0 |
| AUTH-03 | Revoked tokens survive server restart (blocklist loaded from DB) | unit | `go test ./internal/server/auth/... -run TestBlocklistPersistence -count=1` | Wave 0 |
| AGNT-04 | Machine list endpoint returns connected agents with status | integration | `go test ./internal/server/api/... -run TestListMachines -count=1` | Wave 0 |
| AGNT-04 | Connection manager tracks register/disconnect correctly | unit | `go test ./internal/server/connmgr/... -run TestConnectionManager -count=1` | Wave 0 |
| (SC-5) | Unauthenticated requests to protected endpoints return 401 | unit | `go test ./internal/server/api/... -run TestAuthMiddleware -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/server/... -count=1 -short`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps
- [ ] `internal/server/api/router_test.go` -- integration tests for protected/unprotected routes
- [ ] `internal/server/api/auth_handler_test.go` -- register, login, logout handler tests
- [ ] `internal/server/api/machine_handler_test.go` -- machine list endpoint tests
- [ ] `internal/server/auth/jwt_test.go` -- token issuance, validation, algorithm enforcement
- [ ] `internal/server/auth/blocklist_test.go` -- revocation, persistence, cleanup
- [ ] `internal/server/connmgr/manager_test.go` -- register, disconnect, list, concurrent access
- [ ] `internal/server/store/machines_test.go` -- machine status CRUD
- [ ] `internal/server/store/tokens_test.go` -- revoked token persistence
- [ ] Test helpers: in-memory SQLite store, test JWT signing key

## Open Questions

1. **Token refresh mechanism**
   - What we know: AUTH-02 requires JWT issuance. AUTH-03 requires logout. The requirements do not mention refresh tokens.
   - What's unclear: Whether to implement refresh tokens (long-lived token that issues new access tokens) or just short-lived access tokens that require re-login on expiry.
   - Recommendation: Start with access-only tokens with a 60-minute TTL. This is sufficient for V1. If users complain about frequent re-login, add refresh tokens in a follow-up. Keep it simple.

2. **Machine allowlist enforcement timing**
   - What we know: Phase 1 config includes `[machines] allowed = [...]`. Phase 2 research shows mTLS interceptor validates against this allowlist.
   - What's unclear: Whether the allowlist should be DB-backed (so machines can be added at runtime) or config-only (requires restart to add machines).
   - Recommendation: Config-driven for V1 (matches the design doc). If a machine connects that is not in the allowlist, it is rejected. Adding a machine requires config change + restart. DB-backed dynamic allowlist is a V2 enhancement.

3. **HTTP server TLS**
   - What we know: The design doc shows the HTTP server on port 8443 with TLS. The gRPC server on port 9443 uses mTLS for agents.
   - What's unclear: Whether Phase 3 should set up TLS on the HTTP server or defer to a reverse proxy.
   - Recommendation: Implement TLS on the HTTP server using the server cert from Phase 1's CA tooling. The config already has `http.tls_cert` and `http.tls_key` fields. For development, allow an optional `--insecure` flag that runs plain HTTP on port 8080.

## Sources

### Primary (HIGH confidence)
- `docs/internal/product/backend_v1.md` -- Server architecture (sections 4.1-4.3), REST API summary (section 9), auth model (section 2.2), connection manager pattern (section 4.2)
- [golang-jwt/jwt v5 package docs](https://pkg.go.dev/github.com/golang-jwt/jwt/v5) -- Claims, parser options, signing methods
- [go-chi/chi v5 package docs](https://pkg.go.dev/github.com/go-chi/chi/v5) -- Router, middleware, route groups
- Phase 1 research (01-RESEARCH.md) -- SQLite store patterns, Argon2id hashing, users table schema, server config
- Phase 2 research (02-RESEARCH.md) -- gRPC server setup, mTLS interceptor, keepalive settings

### Secondary (MEDIUM confidence)
- [go-chi/jwtauth](https://github.com/go-chi/jwtauth) -- Referenced for JWT middleware patterns (not using directly, building custom with golang-jwt)
- [JWT token revocation patterns](https://supertokens.com/blog/revoking-access-with-a-jwt-blacklist) -- Blocklist approach validation

### Tertiary (LOW confidence)
- None -- all Phase 3 technologies are well-documented with official sources

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- chi and golang-jwt are the most popular Go libraries for their respective domains, well-documented
- Architecture: HIGH -- REST API routes, auth middleware, and connection manager are standard patterns fully specified in the backend design doc
- Pitfalls: HIGH -- JWT algorithm validation, token revocation persistence, and connection manager concurrency are well-documented with known solutions
- Auth flow: HIGH -- Registration, login, logout with JWT is a thoroughly understood pattern

**Research date:** 2026-03-11
**Valid until:** 2026-04-11 (stable technologies, 30-day validity)
