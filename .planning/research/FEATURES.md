# Feature Research

**Domain:** Remote CLI session management control plane (AI agent orchestration)
**Researched:** 2026-03-11
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume exist. Missing these = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Remote terminal access via browser** | Core premise. Every competitor (Teleport, Guacamole, ttyd, Cockpit) delivers this. Without it there is no product. | HIGH | xterm.js + WebSocket. Must support ANSI, color, cursor movement, resize. All major web terminals use xterm.js. |
| **Session persistence across disconnection** | This is the primary pain point being solved (currently SSH sessions die on disconnect). tmux/screen set the expectation that sessions survive. | HIGH | Agent keeps PTY alive, buffers output. Reconnection replays missed output. This is the single most important technical feature. |
| **Session lifecycle management (create/attach/detach/terminate)** | Teleport, Guacamole, JumpServer all provide this. Users need to see what's running and control it. | MEDIUM | REST API for CRUD, WebSocket for attach/detach. State machine: creating -> running -> detached -> terminated. |
| **Session list / dashboard** | Users need to see all active sessions at a glance. Every remote access tool has this. Cockpit, Guacamole, Teleport all show active sessions. | LOW | Table view with status badges. Filter by machine, user, status. |
| **Per-user authentication** | Even small teams need to know who did what. JumpServer, Teleport, Guacamole all have user management. Shared credentials are a security red flag. | MEDIUM | Username/password for v1. JWT tokens for API. Session association with user. |
| **Agent auto-reconnection** | Network blips happen. If the agent loses connection to server, it must reconnect automatically without losing running sessions. | MEDIUM | Exponential backoff with jitter. gRPC keepalive. Agent-side PTY processes are independent of server connection. |
| **mTLS agent authentication** | Agents connecting over the network must be authenticated. mTLS is industry standard for service-to-service auth (Teleport uses certificate-based auth). No secrets to rotate. | HIGH | Built-in CA tooling. Certificate generation, distribution, validation. Already a project constraint. |
| **Machine/agent registry** | Users need to know which machines are available. Teleport has a node registry, JumpServer has asset management. | LOW | Agent registers on connect. Server tracks agent metadata (hostname, OS, status, last seen). |
| **Terminal resize handling** | Users resize browser windows constantly. Broken terminal rendering is unacceptable. All xterm.js-based tools handle this. | LOW | Propagate terminal dimensions from browser through server to agent PTY via TIOCSWINSZ. |
| **Real-time I/O streaming** | Terminal interaction must feel local. Any perceptible lag and users will SSH instead. ttyd, GoTTY, Guacamole all achieve sub-100ms latency. | HIGH | WebSocket (browser-server) + gRPC streaming (server-agent). Binary frames, no JSON encoding of terminal data. |

### Differentiators (Competitive Advantage)

Features that set the product apart. Not required, but valuable.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Job system (interactive notebooks)** | No competitor in this space combines terminal sessions with structured multi-step jobs. Jupyter notebooks set the UX expectation but for CLI, not Python. Databricks Lakeflow does this for data but not for general CLI. This is claude-plane's unique value. | HIGH | Steps with independent execution, output capture, rerun capability. Jobs are NOT CI pipelines (no YAML). Created interactively through UI. |
| **Multi-machine session visibility** | Unlike SSH (one machine at a time) or tmux (local to one machine), claude-plane shows ALL sessions across ALL machines in one view. Teleport does this for SSH but not for managed AI agent sessions. | MEDIUM | Server aggregates session state from all agents. Dashboard shows cross-fleet view. |
| **Session output buffering with replay** | When a user reconnects, they see what they missed. Most web terminals (ttyd, GoTTY, Wetty) lose output on disconnect. Teleport records sessions but for audit, not for live replay. | HIGH | Ring buffer on agent side. Server-side output log. Replay on reconnect sends buffered output to xterm.js. Must handle large outputs without OOM. |
| **Claude CLI-specific integration** | Purpose-built for Claude CLI sessions, not generic SSH. Can provide Claude-aware features (prompt display, conversation tracking, cost awareness) that generic terminals cannot. | MEDIUM | Initially just a PTY wrapper. Future: parse Claude CLI output for structured data (token counts, model info, conversation boundaries). |
| **Job step rerun and selective execution** | Users can rerun individual steps of a job, skip steps, or modify and re-execute. Jupyter notebooks established this pattern. No terminal-based tool offers this. | MEDIUM | Each step is an independent unit with its own session context. Step dependencies are implicit (sequential by default). |
| **NAT/firewall-friendly architecture** | Agents dial in (server never dials out). Workers behind corporate firewalls, NATs, or cloud VPCs just work. Teleport requires Teleport Proxy; Guacamole needs network access to targets. This is zero-config for network topology. | LOW | Already a design constraint. Agent initiates gRPC connection. Server accepts. No inbound ports needed on worker machines. |
| **Single binary deployment** | No Docker, no package managers, no runtime dependencies. `scp` + `chmod +x` + run. Simpler than Teleport (multi-component), Guacamole (Java + tomcat + guacd), or JumpServer (Docker compose). | LOW | Go compiles to static binaries. SQLite embedded. This is a deployment differentiator, not a feature per se. |

### Anti-Features (Commonly Requested, Often Problematic)

Features that seem good but create problems.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Full session recording/replay (audit)** | Teleport and JumpServer offer this for compliance. Seems valuable for review. | Massive storage requirements. Complex playback UI. claude-plane is a small team tool, not an enterprise PAM product. Adds compliance scope without clear user value for a team of devs. | Log session output to files on the agent. Users can grep logs. Add structured session recording in v2 if compliance needs emerge. |
| **SSH/RDP/VNC protocol support** | Guacamole supports all protocols. Feels like a gap to only support Claude CLI. | Scope explosion. Each protocol needs its own proxy, auth, and terminal handling. claude-plane is not a generic bastion host. | Stay laser-focused on Claude CLI sessions. Users who need generic SSH already have SSH. |
| **RBAC / fine-grained permissions** | Teleport and JumpServer have role-based access. Seems enterprise-ready. | Premature for a small team. RBAC adds complexity to every API endpoint. Permission modeling is hard to get right and expensive to change later. | Simple per-user auth in v1. All authenticated users can access all machines and sessions. Add RBAC in v2 when team size demands it. |
| **Real-time collaboration (shared sessions)** | Guacamole has connection sharing. Pair programming in browser sounds cool. | Complex cursor management, input conflict resolution, presence indicators. High implementation cost for a feature that's rarely used in practice. | Allow multiple users to attach to the same session (read-only viewers + one active controller). Simpler model, covers 90% of use cases. |
| **Mobile-optimized UI** | Remote access from phone seems convenient. | Terminal interaction on mobile is terrible. Small screens, no keyboard, touch-based input. Optimizing for mobile degrades the desktop experience (larger tap targets, simplified layouts). | Desktop-first. Mobile gets a read-only status dashboard at best. Already in project out-of-scope. |
| **Plugin/extension system** | Extensibility sounds future-proof. | Massive API surface to maintain. Plugin compatibility across versions is painful. Premature abstraction when the core product isn't built yet. | Build a good core product. Extract extension points later when real needs emerge. |
| **Built-in file editor / IDE** | "Why not edit files in the browser too?" | VS Code, Cursor, and other IDEs already exist. Building even a basic editor is months of work. Claude CLI sessions can use vim/nano inside the terminal. | The terminal IS the editor interface. Claude CLI can edit files. Don't compete with IDEs. |
| **Auto-scaling / cloud provisioning** | "Spin up machines on demand for jobs." | Cloud provider integration is a massive scope increase. Each provider (AWS, GCP, Azure) needs its own integration. Terraform/Pulumi already solve this. | Users provision machines themselves and install the agent. Provide good agent installation docs. Consider cloud integration in v3+. |

## Feature Dependencies

```
[mTLS / CA Tooling]
    └──requires──> [Agent Binary]
                       └──requires──> [gRPC Protocol Definition]

[Session Persistence]
    └──requires──> [Agent PTY Management]
                       └──requires──> [Output Ring Buffer]

[Browser Terminal]
    └──requires──> [WebSocket Server]
                       └──requires──> [Session Routing (server knows which agent owns which session)]

[Job System]
    └──requires──> [Session Lifecycle Management]
                       └──requires──> [Agent PTY Management]

[Session Dashboard]
    └──requires──> [Machine/Agent Registry]
                       └──requires──> [Agent Connection Management]

[Per-User Auth]
    └──requires──> [User Store (SQLite)]

[Job Step Rerun]
    └──requires──> [Job System]
                       └──requires──> [Step Output Capture]

[Session Reconnect Replay]
    └──requires──> [Output Ring Buffer]
                       └──requires──> [Session Persistence]
```

### Dependency Notes

- **Browser Terminal requires Session Routing:** The server must know which agent owns a given session to proxy terminal I/O to the right place. This means agent registry and session tracking must exist before the terminal UI can work.
- **Job System requires Session Lifecycle:** Jobs are composed of steps, each step runs in a session. Without reliable session create/run/terminate, jobs cannot function.
- **Session Reconnect Replay requires Output Ring Buffer:** The agent must buffer output while the user is disconnected. Without the buffer, reconnection shows a blank terminal.
- **mTLS requires CA Tooling:** Certificates must be generated before agents can connect. The `claude-plane-server ca` subcommand must exist before any agent communication works.

## MVP Definition

### Launch With (v1)

Minimum viable product -- what's needed to replace "SSH into machines and run Claude CLI manually."

- [ ] **Agent binary with PTY management** -- foundation for everything; manages Claude CLI processes
- [ ] **Server binary with gRPC agent handling** -- accepts agent connections, routes commands
- [ ] **mTLS CA tooling and certificate auth** -- agents must authenticate securely; no shortcuts
- [ ] **Session lifecycle (create/attach/detach/terminate)** -- core interaction model
- [ ] **Session persistence with output buffering** -- sessions survive disconnection; this is the killer feature
- [ ] **WebSocket terminal streaming to browser** -- users interact via xterm.js in browser
- [ ] **Per-user authentication** -- know who is doing what
- [ ] **Session dashboard** -- see all sessions across all machines
- [ ] **Machine/agent registry** -- see which machines are connected
- [ ] **Job system with multi-step notebooks** -- structured task execution, not just raw terminals
- [ ] **Terminal resize handling** -- basic UX requirement

### Add After Validation (v1.x)

Features to add once core is working.

- [ ] **Session search and filtering** -- when session count grows, users need to find specific ones
- [ ] **Job templates** -- save and reuse common job definitions
- [ ] **Multi-user session viewing** -- let others watch (read-only) an active session
- [ ] **Notifications** -- alert when a job step completes or fails
- [ ] **Session output export** -- download session transcript as text
- [ ] **Agent health monitoring** -- basic heartbeat, disk space, CPU indicators

### Future Consideration (v2+)

Features to defer until product-market fit is established.

- [ ] **Workspace isolation (git worktrees)** -- per-session isolated directories; already designed, defer to v2
- [ ] **OIDC/OAuth2 authentication** -- enterprise SSO; overkill for small team v1
- [ ] **RBAC / permissions** -- when team grows beyond trust-everyone model
- [ ] **Session recording and replay (audit)** -- compliance-driven, not user-driven
- [ ] **Cost tracking / token analytics** -- parse Claude CLI output for usage data
- [ ] **Arena mode** -- parallel Claude sessions competing on same task; v3+ feature
- [ ] **Command Center dashboard** -- machine health, system-wide metrics view
- [ ] **Webhook integrations** -- trigger jobs from external events

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Session persistence + reconnect replay | HIGH | HIGH | P1 |
| Browser terminal (xterm.js + WebSocket) | HIGH | HIGH | P1 |
| Session lifecycle management | HIGH | MEDIUM | P1 |
| mTLS agent authentication | HIGH | HIGH | P1 |
| Agent PTY management | HIGH | MEDIUM | P1 |
| Per-user authentication | HIGH | MEDIUM | P1 |
| Machine/agent registry | MEDIUM | LOW | P1 |
| Session dashboard | MEDIUM | LOW | P1 |
| Job system (interactive notebooks) | HIGH | HIGH | P1 |
| Terminal resize | MEDIUM | LOW | P1 |
| Job templates | MEDIUM | LOW | P2 |
| Multi-user session viewing | MEDIUM | MEDIUM | P2 |
| Session search/filtering | LOW | LOW | P2 |
| Notifications | MEDIUM | MEDIUM | P2 |
| Agent health monitoring | MEDIUM | LOW | P2 |
| Session output export | LOW | LOW | P2 |
| Workspace isolation | HIGH | MEDIUM | P3 |
| OIDC/OAuth2 | MEDIUM | HIGH | P3 |
| RBAC | MEDIUM | HIGH | P3 |
| Cost tracking | MEDIUM | MEDIUM | P3 |
| Arena mode | MEDIUM | HIGH | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Competitor Feature Analysis

| Feature | Teleport | Guacamole | ttyd/GoTTY | Cockpit | JumpServer | claude-plane |
|---------|----------|-----------|------------|---------|------------|--------------|
| Web terminal | Yes (via Connect) | Yes (HTML5) | Yes (xterm.js) | Yes (built-in) | Yes (web) | Yes (xterm.js) |
| Session persistence | No (SSH sessions die) | No | No | No | No | **Yes -- core differentiator** |
| Session recording | Yes (audit replay) | Yes (guacenc) | No | No | Yes (replay) | No (v2) |
| Multi-machine view | Yes (node list) | Yes (connection list) | No (single process) | Yes (multi-host) | Yes (asset list) | Yes |
| Job orchestration | No | No | No | No | No | **Yes -- unique** |
| Interactive notebooks | No | No | No | No | No | **Yes -- unique** |
| Protocol support | SSH, K8s, DB, RDP | SSH, VNC, RDP | Single command | SSH (local) | SSH, RDP, K8s, DB | Claude CLI (focused) |
| Auth model | SSO, MFA, RBAC | LDAP, TOTP, OpenID | None | PAM/SSO | AD, LDAP, SAML, OAuth | Per-user (v1), OIDC (v2) |
| Deployment | Multi-component | Docker (Java+guacd) | Single binary | System package | Docker compose | **Single binary** |
| NAT-friendly | Proxy mode | Needs network access | N/A (local) | Needs network access | Needs network access | **Yes -- agents dial in** |
| Connection sharing | No | Yes (sharing profiles) | Read-only option | No | No | v1.x (read-only viewers) |
| Self-hosted | Yes | Yes | Yes | Yes | Yes | Yes |

## Sources

- [Teleport Features](https://goteleport.com/features/) -- session recording, RBAC, SSO, audit
- [Teleport Connect blog](https://goteleport.com/blog/teleport-connect/) -- terminal + web hybrid UX
- [Apache Guacamole admin interface](https://guacamole.apache.org/doc/gug/administration.html) -- session management, connection sharing
- [Apache Guacamole user interface](https://guacamole.apache.org/doc/gug/using-guacamole.html) -- connection sharing profiles
- [ttyd GitHub](https://github.com/tsl0922/ttyd) -- xterm.js-based web terminal, single binary
- [GoTTY GitHub](https://github.com/yudai/gotty) -- Go web terminal, hterm/xterm.js
- [Cockpit Project](https://cockpit-project.org/) -- web-based server management, multi-host
- [JumpServer](https://www.jumpserver.com/) -- open-source PAM, session recording, approval workflows
- [xterm.js](https://xtermjs.org/) -- terminal emulation library, GPU-accelerated renderer
- [JumpServer session tracking (DeepWiki)](https://deepwiki.com/jumpserver/jumpserver/10.2-session-tracking-and-replay) -- session replay architecture

---
*Feature research for: Remote CLI session management control plane*
*Researched: 2026-03-11*
