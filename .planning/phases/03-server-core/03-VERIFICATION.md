---
phase: 03-server-core
verified: 2026-03-12T11:00:00Z
status: passed
score: 14/14 must-haves verified
re_verification: false
---

# Phase 3: Server Core Verification Report

**Phase Goal:** Server accepts agent connections over mTLS, tracks connected machines, and exposes authenticated REST API endpoints for user and session management
**Verified:** 2026-03-12T11:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | User can create account with email/password, log in, receive JWT | VERIFIED | POST /api/v1/auth/register (201), POST /api/v1/auth/login (200 + token). TestRegisterSuccess, TestLoginSuccess pass. |
| 2  | User can log out and JWT is invalidated | VERIFIED | POST /api/v1/auth/logout revokes JTI; TestLogoutRevokesToken confirms revoked token gets 401 on protected endpoint. |
| 3  | Server accepts incoming mTLS agent connections and tracks them as online | VERIFIED | grpc/server.go CommandStream calls agentConnMgr.Register() on connect, Disconnect() on stream end. connmgr tests pass with race detector. |
| 4  | Server displays list of connected agents with online/offline status via REST API | VERIFIED | GET /api/v1/machines overlays live status from ConnectionManager.GetAgent(). TestListMachinesAuthenticated passes (200 with array). |
| 5  | Unauthenticated API requests are rejected with 401 | VERIFIED | JWTAuthMiddleware returns 401 for missing header, invalid format, invalid/expired/revoked tokens. TestProtectedEndpointNoAuth, TestListMachinesUnauthenticated pass. |
| 6  | Valid token accepted; expired/tampered/alg-spoofed token rejected | VERIFIED | jwt.go ValidateToken enforces HS256 via WithValidMethods. TestValidateExpired, TestValidateWrongAlgorithm, TestValidateWrongSignature all pass. |
| 7  | After logout, same token rejected on subsequent requests | VERIFIED | Blocklist.Add() + IsRevoked() wired through JWT service. TestLogoutRevokesToken pass. |
| 8  | Revoked tokens survive server restart (persisted to SQLite) | VERIFIED | tokens.go RevokeToken inserts to DB; blocklist.go NewBlocklist loads active revocations on init. TestNewBlocklistLoadsFromDB pass. |
| 9  | Expired revocation entries cleaned up automatically | VERIFIED | StartCleanup goroutine with ticker. TestBlocklistCleanup pass. |
| 10 | POST /api/v1/auth/register rejects duplicate email with 409 | VERIFIED | IsUniqueViolation(err) check in Register handler. TestRegisterDuplicateEmail pass (409). |
| 11 | POST /api/v1/auth/register rejects invalid input with 400 | VERIFIED | Validates email non-empty, password >= 8 chars. TestRegisterInvalidInput pass (400). |
| 12 | Machine store supports upsert, status update, list, get | VERIFIED | machines.go implements all four. TestUpsertMachine, TestUpdateMachineStatus, TestListMachines, TestGetMachineNotFound pass. |
| 13 | Connection manager thread-safe under concurrent access | VERIFIED | RWMutex with DB ops outside lock. TestConcurrentAccess pass with -race (50 goroutines). |
| 14 | Config supports JWT secret file path and token TTL | VERIFIED | AuthConfig has JWTSecretFile, TokenTTL fields. ParseTokenTTL() returns 60m default. |

**Score:** 14/14 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/server/auth/jwt.go` | JWT issuance, validation, revocation service | VERIFIED | Claims, Service, NewService, IssueToken, ValidateToken, RevokeToken all present and substantive. HS256 enforcement confirmed. |
| `internal/server/auth/blocklist.go` | In-memory blocklist with SQLite persistence | VERIFIED | Blocklist, NewBlocklist, Add, IsRevoked, StartCleanup all present. RWMutex concurrency confirmed. |
| `internal/server/store/tokens.go` | SQLite persistence for revoked tokens | VERIFIED | RevokeToken, LoadActiveRevocations, CleanExpired against real SQLite. |
| `internal/server/auth/jwt_test.go` | Tests for JWT issuance, validation, algorithm enforcement | VERIFIED | 7 tests: TestIssueToken, TestValidateToken, TestValidateExpired, TestValidateWrongAlgorithm, TestValidateRevoked, TestIssueTokenUniqueness, TestValidateWrongSignature. All pass. |
| `internal/server/auth/blocklist_test.go` | Tests for blocklist | VERIFIED | 5 tests including concurrency. All pass with -race. |
| `internal/server/store/tokens_test.go` | Tests for token store persistence | VERIFIED | TestRevokeAndLoad, TestCleanExpired. Both pass. |
| `internal/server/store/machines.go` | Machine CRUD operations on SQLite | VERIFIED | UpsertMachine, UpdateMachineStatus, ListMachines, GetMachine all substantive. |
| `internal/server/store/machines_test.go` | Tests for machine store | VERIFIED | 5 tests covering upsert, update, list, not-found. All pass. |
| `internal/server/connmgr/manager.go` | In-memory agent connection tracking with DB-backed status | VERIFIED | ConnectionManager, MachineStore interface, Register, Disconnect, GetAgent, ListAgents all present. |
| `internal/server/connmgr/manager_test.go` | Tests for connection manager | VERIFIED | 5 tests including concurrent access. All pass with -race. |
| `internal/server/grpc/server.go` | gRPC AgentStream wired to connection manager | VERIFIED | agentConnMgr field, Register/Disconnect calls in CommandStream, deferred Disconnect on stream end. |
| `internal/server/api/router.go` | Chi router with public and protected route groups | VERIFIED | NewHandlers, NewRouter, public group (register, login), protected group with JWTAuthMiddleware. |
| `internal/server/api/middleware.go` | JWT auth middleware and context helpers | VERIFIED | JWTAuthMiddleware, GetClaims, UserClaimsKey all present and wired. |
| `internal/server/api/response.go` | JSON response helpers | VERIFIED | writeJSON, writeError — minimal but complete. |
| `internal/server/api/auth_handler.go` | Register, Login, Logout HTTP handlers | VERIFIED | All three handlers with full validation, DB calls, and JWT operations. |
| `internal/server/api/machine_handler.go` | Machine list and get handlers with live status overlay | VERIFIED | ListMachines and GetMachine both overlay live status via connMgr.GetAgent(). |
| `internal/server/api/auth_handler_test.go` | Integration tests for auth endpoints | VERIFIED | 8 tests covering full auth lifecycle. All pass. |
| `internal/server/api/machine_handler_test.go` | Integration tests for machine endpoints | VERIFIED | 2 tests (authenticated, unauthenticated). Both pass. |
| `internal/server/config/config.go` | Auth section in server config | VERIFIED | AuthConfig with JWTSecret, JWTSecretFile, TokenTTL, ParseTokenTTL helper. |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/server/auth/jwt.go` | `github.com/golang-jwt/jwt/v5` | JWT signing and parsing | VERIFIED | jwt.NewWithClaims and jwt.ParseWithClaims both present in jwt.go |
| `internal/server/auth/blocklist.go` | `internal/server/store/tokens.go` | Persist and load revocations | VERIFIED | NewBlocklist calls tokenStore.LoadActiveRevocations(); Add calls store.RevokeToken(); cleanExpired calls store.CleanExpired() |
| `internal/server/auth/jwt.go` | `internal/server/auth/blocklist.go` | Check blocklist during validation | VERIFIED | ValidateToken calls s.blocklist.IsRevoked(claims.ID); RevokeToken calls s.blocklist.Add() |
| `internal/server/connmgr/manager.go` | `internal/server/store/machines.go` | Update DB on register/disconnect | VERIFIED | Register calls store.UpsertMachine + store.UpdateMachineStatus("connected"); Disconnect calls store.UpdateMachineStatus("disconnected") |
| `internal/server/connmgr/manager.go` | `sync.RWMutex` | Thread-safe agent map access | VERIFIED | sync.RWMutex present; Lock for writes, RLock for reads, DB ops outside lock |
| `internal/server/grpc/server.go` | `internal/server/connmgr/manager.go` | Register agent on stream start, disconnect on stream end | VERIFIED | CommandStream calls agentConnMgr.Register() then defers agentConnMgr.Disconnect() |
| `internal/server/api/middleware.go` | `internal/server/auth/jwt.go` | ValidateToken in middleware | VERIFIED | JWTAuthMiddleware calls authSvc.ValidateToken(parts[1]) |
| `internal/server/api/auth_handler.go` | `internal/server/store/users.go` | GetUserByEmail, CreateUser for login/register | VERIFIED | Register calls h.store.CreateUser(user); Login calls h.store.GetUserByEmail(req.Email) |
| `internal/server/api/auth_handler.go` | `internal/server/auth/jwt.go` | IssueToken on login, RevokeToken on logout | VERIFIED | Login calls h.authSvc.IssueToken(user); Logout calls h.authSvc.RevokeToken(claims.ID, claims.ExpiresAt.Time) |
| `internal/server/api/machine_handler.go` | `internal/server/connmgr/manager.go` | Overlay live status on DB data | VERIFIED | Both ListMachines and GetMachine call h.connMgr.GetAgent(machineID) to overlay "connected" status |
| `internal/server/api/router.go` | `github.com/go-chi/chi/v5` | HTTP router and middleware chaining | VERIFIED | chi.NewRouter used; chi.Group for protected routes; chi/v5/middleware for global middleware |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| AUTH-01 | 03-03-PLAN | User can create account with email and password | SATISFIED | POST /api/v1/auth/register handler with validation, password hashing, unique email enforcement. Integration tests pass (201/409/400). |
| AUTH-02 | 03-01-PLAN, 03-03-PLAN | User can log in and receive a JWT session token | SATISFIED | JWT Service issues HS256 tokens with UUID JTI, correct claims. POST /api/v1/auth/login returns token. TestLoginSuccess passes. |
| AUTH-03 | 03-01-PLAN, 03-03-PLAN | User can log out and invalidate their session | SATISFIED | Blocklist persisted to SQLite; POST /api/v1/auth/logout revokes JTI. Revoked token rejected on next request. TestLogoutRevokesToken passes. |
| AGNT-04 | 03-02-PLAN, 03-03-PLAN | Server displays list of connected agents with online/offline status | SATISFIED | ConnectionManager tracks live agents; GET /api/v1/machines overlays live status. DB stores persistent status. TestListMachinesAuthenticated passes. |

**Orphaned requirements check:** REQUIREMENTS.md maps AUTH-01, AUTH-02, AUTH-03, AGNT-04 to Phase 3. All four are claimed by plans and verified above. No orphaned requirements.

---

### Anti-Patterns Found

No anti-patterns detected in any phase 3 files:
- No TODO/FIXME/HACK/PLACEHOLDER comments
- No empty implementations or stub return values
- No unconnected state or fire-and-forget fetch calls (Go, not JS)
- `go vet` reports no issues across auth, store, connmgr, api packages

---

### Human Verification Required

None. All phase 3 behaviors are backend HTTP endpoints verifiable programmatically. Integration tests cover the complete token lifecycle and machine status overlay. No UI, visual, or real-time browser behavior involved in this phase.

---

### Gaps Summary

No gaps found. All 14 truths verified, all 19 artifacts confirmed substantive and wired, all 11 key links confirmed active in the code. All four requirements (AUTH-01, AUTH-02, AUTH-03, AGNT-04) are satisfied with passing test coverage. The full server package suite passes with the race detector enabled.

One notable design observation (not a gap): the `blocklist.go` passes an empty string for `userID` when persisting revocations since the JWT service context does not carry the userID at revocation time. The `NOT NULL` constraint is satisfied and the field is not used for any query, so this is a non-issue.

---

_Verified: 2026-03-12T11:00:00Z_
_Verifier: Claude (gsd-verifier)_
