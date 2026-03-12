# Requirements: claude-plane

**Defined:** 2026-03-11
**Core Value:** A developer can open the browser, connect to a Claude CLI session running on any remote machine, and interact with it as if they were sitting at that terminal -- with sessions that survive disconnection.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Authentication

- [x] **AUTH-01**: User can create account with email and password
- [x] **AUTH-02**: User can log in and receive a JWT session token
- [x] **AUTH-03**: User can log out and invalidate their session
- [x] **AUTH-04**: Admin account can be seeded via server CLI on first run

### Agent Management

- [x] **AGNT-01**: Server provides CA tooling to generate root CA, server certs, and agent certs
- [x] **AGNT-02**: Agent authenticates to server using mTLS with its issued certificate
- [x] **AGNT-03**: Agent registers with server and maintains persistent gRPC bidirectional stream
- [x] **AGNT-04**: Server displays list of connected agents with online/offline status

### Session Management

- [ ] **SESS-01**: User can create a new Claude CLI session on any connected machine
- [ ] **SESS-02**: User can attach to an existing session and receive terminal output
- [ ] **SESS-03**: User can detach from a session without terminating it
- [ ] **SESS-04**: User can list all active sessions across all machines
- [ ] **SESS-05**: User can terminate a session
- [ ] **SESS-06**: Sessions continue running on the agent when the user disconnects from the browser

### Terminal Streaming

- [ ] **TERM-01**: User sees real-time terminal output in the browser via xterm.js
- [ ] **TERM-02**: User can type into the browser terminal and input reaches the remote CLI
- [ ] **TERM-03**: Browser window resize propagates to the remote PTY dimensions
- [ ] **TERM-04**: Flow control prevents fast output from overwhelming the browser

### Job System

- [ ] **JOBS-01**: User can create a job with multiple ordered steps
- [ ] **JOBS-02**: User can execute individual job steps and view their output
- [ ] **JOBS-03**: User can rerun a previously executed step
- [ ] **JOBS-04**: Steps support dependency ordering (step B waits for step A to complete)

### Infrastructure

- [x] **INFR-01**: Server is a single Go binary with embedded frontend assets
- [x] **INFR-02**: Agent is a single Go binary with no external dependencies
- [x] **INFR-03**: Server uses SQLite with WAL mode for all persistent storage
- [x] **INFR-04**: Server and agent support TOML configuration files

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Authentication & Authorization

- **AUTH-05**: User can log in via OIDC/OAuth2 (GitHub, Google, Keycloak)
- **AUTH-06**: Role-based access control (admin, developer, viewer)

### Session Management

- **SESS-07**: Reconnecting to a session replays missed terminal output from scrollback buffer

### Fleet Management

- **FLEET-01**: Machine health dashboard with CPU, memory, disk metrics
- **FLEET-02**: Agent auto-upgrade mechanism

### Workspace Isolation

- **WKSP-01**: Each session gets an isolated git worktree to prevent collisions

### Cost Tracking

- **COST-01**: Token usage tracking per session via API proxy
- **COST-02**: Cost analytics dashboard with per-user and per-model breakdowns

### Job System

- **JOBS-05**: Job templates for reusable multi-step workflows

### Advanced Features

- **ARENA-01**: Arena mode -- multiple Claude sessions compete on the same task

## Out of Scope

| Feature | Reason |
|---------|--------|
| SSH/RDP/VNC support | claude-plane is Claude CLI-specific, not a generic bastion host |
| Cloud machine provisioning | Infrastructure management is out of scope -- machines are pre-provisioned |
| Mobile-optimized UI | Desktop-first, team uses workstations |
| Session recording for compliance | Not a compliance tool, focus on interactive use |
| Real-time collaboration (shared terminal) | Adds significant complexity, defer indefinitely |
| Plugin/extension system | Over-engineering for v1 |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| AUTH-01 | Phase 3 | Complete |
| AUTH-02 | Phase 3 | Complete |
| AUTH-03 | Phase 3 | Complete |
| AUTH-04 | Phase 1 | Complete |
| AGNT-01 | Phase 1 | Complete |
| AGNT-02 | Phase 2 | Complete |
| AGNT-03 | Phase 2 | Complete |
| AGNT-04 | Phase 3 | Complete |
| SESS-01 | Phase 4 | Pending |
| SESS-02 | Phase 4 | Pending |
| SESS-03 | Phase 4 | Pending |
| SESS-04 | Phase 5 | Pending |
| SESS-05 | Phase 4 | Pending |
| SESS-06 | Phase 4 | Pending |
| TERM-01 | Phase 4 | Pending |
| TERM-02 | Phase 4 | Pending |
| TERM-03 | Phase 4 | Pending |
| TERM-04 | Phase 4 | Pending |
| JOBS-01 | Phase 6 | Pending |
| JOBS-02 | Phase 6 | Pending |
| JOBS-03 | Phase 6 | Pending |
| JOBS-04 | Phase 6 | Pending |
| INFR-01 | Phase 1 | Complete |
| INFR-02 | Phase 1 | Complete |
| INFR-03 | Phase 1 | Complete |
| INFR-04 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 26 total
- Mapped to phases: 26
- Unmapped: 0

---
*Requirements defined: 2026-03-11*
*Last updated: 2026-03-11 after roadmap creation*
