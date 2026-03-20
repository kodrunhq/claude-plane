# Agent Lifecycle Management

**Date:** 2026-03-20
**Status:** Approved
**Scope:** `cmd/agent/`, `internal/agent/`, docs

## Problem

Registering and managing agent nodes requires too many manual steps and has no guard rails. Users must manually kill orphaned processes, have no way to detect duplicate agents, and must piece together multiple commands to re-register a node. There is no `uninstall-service` command, no PID lock, and `join` silently overwrites existing config without stopping a running agent.

Even the project owner has to `ps aux | grep claude` and `kill` PIDs by hand — this is unacceptable for end users.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Single entry point | `join --service` does everything | Common case should be one command |
| Existing agent on join | Auto-stop, no prompt | Intent is clear — user has a new token |
| Uninstall scope | Base = service only, `--purge` = full cleanup | Mirrors `apt remove` vs `apt purge` |
| Duplicate prevention | PID lock + advisory message | Catches accidental double-start |
| Orphan reaping | On `run` startup only | `join` stops agent cleanly; `run` is the safety net for crashes |
| Privilege escalation for `--service` | Re-exec sudo for service part only | Keeps file ownership correct |

## Components

### 1. Shared Lifecycle Package (`internal/agent/lifecycle/`)

All commands that need to find or stop an existing agent use the same detection logic. This avoids duplicating process-scanning code across `join`, `install-service`, `run`, and `uninstall-service`.

**File:** `internal/agent/lifecycle/lifecycle.go`

#### `StopExisting(dataDir string, logger *slog.Logger) error`

> **Note:** All PID/lifecycle files live in `dataDir` (from `cfg.Agent.DataDir`), not the config directory. `dataDir` is guaranteed to exist (created by `run` on startup) and is the natural home for runtime state. The `run` command derives `dataDir` from the loaded config. The `join` command derives it as `{configDir}/data` (matching what `ExecuteJoin` writes to `agent.toml`).

Detection and stop order:

1. **systemd service** — Check if `claude-plane-agent.service` is active via `systemctl is-active`. If active, run `systemctl stop claude-plane-agent`. Log: `"Stopped existing agent (systemd service)"`.
2. **PID file** — Read `{dataDir}/agent.pid`. If PID is alive (signal 0), send SIGTERM. Wait up to 5 seconds for exit. If still alive, send SIGKILL. Remove PID file. Log: `"Stopped existing agent (PID {pid})"`.
3. **Process scan fallback** — Find processes matching `claude-plane-agent run` owned by the current user (via `/proc` on Linux, `pgrep` on macOS). For each, send SIGTERM, wait 3s, SIGKILL. Log: `"Stopped orphaned agent process (PID {pid})"`.

Each step is attempted regardless of whether the previous step found anything — belt and suspenders.

On macOS, step 1 uses `launchctl list | grep claude-plane` and `launchctl bootout system/com.claude-plane.agent` instead of systemctl. Note: we use `bootout`/`bootstrap` (modern launchctl API) rather than the deprecated `load`/`unload`. The existing `installLaunchd` should also be updated from `launchctl load` to `launchctl bootstrap system` as part of this work.

#### `WritePIDFile(dataDir string) (cleanup func(), err error)`

Writes current PID to `{dataDir}/agent.pid`. Returns a cleanup function that removes the file. The cleanup function is safe to call multiple times.

#### `CheckPIDFile(dataDir string) (pid int, alive bool, err error)`

Reads PID file, checks if process is alive. Returns `(0, false, nil)` if no PID file exists. Returns `(pid, false, nil)` if PID file exists but process is dead (stale). Returns `(pid, true, nil)` if process is alive.

#### `ReapOrphanedProcesses(dataDir string, logger *slog.Logger) error`

Finds orphaned `claude` child processes left behind by a previous agent crash. Does **not** rely on scrollback files (which are asciicast `.cast` files keyed by session UUID, not PID). Instead, uses a process scan approach:

1. Scan `/proc/` (Linux) or `ps aux` (macOS) for processes whose command line contains `claude` and whose parent PID is 1 (reparented orphan) or whose parent is the current agent process.
2. Filter to only processes owned by the current user.
3. For each match: send SIGTERM, wait 3 seconds, send SIGKILL if still alive.
4. Log each reaped process: `"Reaped orphaned session process (PID {pid}, cmd: {cmdline})"`.

This is best-effort — if no orphans are found, it does nothing.

**Why this is needed:** When the agent receives SIGTERM, the signal handler in `run` cancels the context and `client.Run()` returns, but `SessionManager` does not currently have an explicit `Shutdown()` that terminates active PTY sessions. The child `claude` processes get reparented to PID 1 and keep running. The orphan reaper on the next `run` startup is the safety net for this gap. A future improvement could add graceful session teardown to the agent's shutdown path, but that is out of scope for this spec.

This is called by `run` on startup, not by `join`.

### 2. PID Lock on `run` Startup

**File:** `cmd/agent/main.go` (modify `newRunCmd`)

Before entering the main loop, `run` now:

1. Calls `lifecycle.CheckPIDFile(dataDir)`.
2. If alive: prints `"Agent already running (PID {pid}). Stop it first or use 'join' to re-register."` and exits with code 1.
3. If stale: removes the stale PID file, logs warning.
4. Calls `lifecycle.WritePIDFile(dataDir)` — defers the cleanup function.
5. Calls `lifecycle.ReapOrphanedProcesses(dataDir, logger)`.
6. Proceeds with normal startup.

The PID file is also removed on signal handler (SIGINT/SIGTERM) before shutdown completes. The defer + signal handler double-coverage ensures cleanup in both clean and forced exit paths.

### 3. Enhanced `join` Command

**File:** `cmd/agent/main.go` (modify `newJoinCmd`), `internal/agent/join.go`

New flag:
- `--service` — After join, install and start the systemd/launchd service (requires sudo).

**Important:** `join` must NOT be run as root / via `sudo`. It writes config and certs that must be owned by the target user. If `join` detects it is running as root (and `--service` is set), it prints an error: `"Do not run 'join' as root. Run as your normal user — only the service installation needs sudo."` and exits. The `--service` flag handles the sudo escalation internally for just the `install-service` step.

Flow:

1. Resolve server URL and validate (unchanged).
2. **Call `lifecycle.StopExisting(filepath.Join(configDir, "data"), logger)`** — stops any running agent.
3. Execute join (call server API, write certs + config) — unchanged.
4. Print success message (unchanged).
5. **If `--service` flag is set:**
   a. Resolve absolute binary path and config path.
   b. Print `"Installing systemd service (requires sudo)..."`.
   c. Execute: `sudo {binPath} install-service --config {configPath}`.
   d. The sudo call inherits stdin/stdout/stderr so the user sees the sudo prompt and output.
   e. If sudo fails (user cancels, not authorized), print error but don't fail the join — config is already written.
6. **If `--service` flag is NOT set:** print the manual `install-service` command as today.

### 4. Enhanced `install-service` Command

**File:** `cmd/agent/service.go`

Before installing, `installSystemd` and `installLaunchd` now:

1. Check if the service already exists and is active.
2. If active: stop it, disable it. Log: `"Stopped existing claude-plane-agent service"`.
3. Check for running agent processes via `lifecycle.CheckPIDFile` and process scan.
4. If found: stop them. Log accordingly.
5. Proceed with install as today (write unit file, daemon-reload, enable, start).

This makes `install-service` fully idempotent — safe to run multiple times.

### 5. New `uninstall-service` Command

**File:** `cmd/agent/main.go` (new `newUninstallServiceCmd`), `cmd/agent/service.go` (new functions)

Subcommand: `claude-plane-agent uninstall-service [--purge]`

Requires root/sudo (same as `install-service`).

#### Base behavior (no `--purge`):

**Linux:**
1. `systemctl stop claude-plane-agent`
2. `systemctl disable claude-plane-agent`
3. Remove `/etc/systemd/system/claude-plane-agent.service`
4. `systemctl daemon-reload`
5. Print summary: `"Service stopped and removed."`

**macOS:**
1. `launchctl bootout system/com.claude-plane.agent` (gracefully stops and unregisters).
2. Remove `/Library/LaunchDaemons/com.claude-plane.agent.plist`.
3. Print summary.

If the service doesn't exist, print `"No claude-plane-agent service found."` and exit cleanly (not an error).

#### With `--purge`:

After removing the service, also:
1. Kill any remaining agent processes (via `lifecycle.StopExisting`).
2. Determine config directory:
   - If `--config-dir` flag provided, use that.
   - Else if `$SUDO_USER` is set, resolve their home directory via `os/user.Lookup($SUDO_USER)` and use `{homeDir}/.claude-plane`. This correctly handles both Linux (`/home/user`) and macOS (`/Users/user`).
   - Else use `/etc/claude-plane`.
3. Remove the entire config directory (`{configDir}/` — includes certs, config, data, PID file).
4. Print summary listing everything removed:
   ```
   Removed:
     Service:  /etc/systemd/system/claude-plane-agent.service
     Config:   /home/user/.claude-plane/
   ```

### 6. Orphan Reaping on `run` Startup

**File:** `internal/agent/lifecycle/lifecycle.go` (`ReapOrphanedProcesses`)

When `run` starts (after PID lock is acquired), before entering the gRPC reconnection loop, it calls `lifecycle.ReapOrphanedProcesses(dataDir, logger)` which uses the process scan approach described in Component 1 above (scan `/proc/` or `ps aux` for orphaned `claude` processes owned by the current user with PPID 1).

### 7. Documentation Updates

#### `docs/install-agent.md`

- **Rewrite "Alternative: Quick Join" section** as the primary recommended flow (rename to "Quick Start: Join with Provisioning Code")
- Document the `--service` flag on `join`
- Add "Re-registering an Agent" section explaining that `join` auto-stops existing agents
- Add "Uninstalling" section documenting `uninstall-service` and `--purge`
- Update troubleshooting with new scenarios (duplicate agent, orphaned processes)

#### `docs/quickstart.md`

- Update step 7 (Start the Agent) to show the `join --service` single-command flow
- Simplify the agent setup to: download binary → `join CODE --server URL --service`

#### `docs/configuration.md`

- Document `agent.pid` file location and behavior
- Document `--service` flag on `join`
- Document `uninstall-service` subcommand with `--purge`

#### `CLAUDE.md`

- Add `uninstall-service` to the **agent** subcommands (not server — this is an agent command)
- Add lifecycle package to the Agent Architecture table
- Update the agent CLI commands reference

#### `cmd/agent/main.go` — In-app Help Text

- Update `join` long description to mention auto-stop behavior
- Add `uninstall-service` command with clear help text
- Update `install-service` long description to mention idempotency

#### `README.md` (if exists)

- Update any agent setup instructions to reflect the simplified flow

## File Changes Summary

| File | Change |
|------|--------|
| `internal/agent/lifecycle/lifecycle.go` | **New** — shared lifecycle utilities |
| `internal/agent/lifecycle/lifecycle_test.go` | **New** — tests for lifecycle package |
| `cmd/agent/main.go` | **Modify** — PID lock in `run`, `--service` flag on `join`, new `uninstall-service` command |
| `cmd/agent/service.go` | **Modify** — guards on `install-service`, new `uninstallService` functions |
| `internal/agent/join.go` | **Modify** — call `lifecycle.StopExisting` before join |
| `docs/install-agent.md` | **Modify** — rewrite with new flow, add uninstall section |
| `docs/quickstart.md` | **Modify** — simplify agent setup |
| `docs/configuration.md` | **Modify** — document new commands and files |
| `CLAUDE.md` | **Modify** — update architecture tables and commands |

## Testing Strategy

### Unit Tests (`internal/agent/lifecycle/`)

- `TestCheckPIDFile` — no file, stale PID, alive PID
- `TestWritePIDFile` — write + cleanup removes file
- `TestStopExisting` — mock systemctl/process checks (use build tags or interfaces)
- `TestReapOrphanedProcesses` — mock process scan with fake orphaned claude processes

### Integration Tests

- `TestJoinStopsExistingAgent` — start a mock agent process, run join, verify it was stopped
- `TestRunPIDLock` — start `run`, attempt second `run`, verify error message
- `TestInstallServiceIdempotent` — run install-service twice, verify clean state
- `TestUninstallServicePurge` — install then uninstall with --purge, verify cleanup

### Manual Testing Checklist

- [ ] Fresh join + run on clean machine
- [ ] Re-join with running service (auto-stop works)
- [ ] `join --service` installs service after join
- [ ] `run` with existing PID lock shows helpful error
- [ ] `run` after crash reaps orphaned processes
- [ ] `uninstall-service` removes service cleanly
- [ ] `uninstall-service --purge` removes everything
- [ ] `install-service` on already-installed service (idempotent)
- [ ] macOS launchd equivalents for all above

## Non-Goals

- **Windows support** — not a target platform for the agent.
- **Multi-agent on same machine** — supported via separate config dirs, but lifecycle management assumes one agent per config dir.
- **Automatic binary updates** — out of scope; agent download is a separate concern.
