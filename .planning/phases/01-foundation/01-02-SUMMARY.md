---
phase: 01-foundation
plan: 02
subsystem: infra
tags: [mtls, ca, x509, ecdsa, toml, config, tls]

# Dependency graph
requires:
  - phase: 01-foundation-01
    provides: "Go module, protobuf contract, cobra CLI skeleton"
provides:
  - "mTLS CA tooling (GenerateCA, IssueServerCert, IssueAgentCert)"
  - "TLS config builders (ServerTLSConfig, AgentTLSConfig)"
  - "Server TOML config parser with validation"
  - "Agent TOML config parser with defaults and validation"
affects: [01-foundation-03, 02-agent-core, 03-server-core]

# Tech tracking
tech-stack:
  added: [BurntSushi/toml, crypto/x509, crypto/ecdsa, crypto/tls]
  patterns: [TDD red-green, TOML config with validation, PEM cert I/O]

key-files:
  created:
    - internal/shared/tlsutil/ca.go
    - internal/shared/tlsutil/ca_test.go
    - internal/shared/tlsutil/loader.go
    - internal/server/config/config.go
    - internal/server/config/config_test.go
    - internal/agent/config/config.go
    - internal/agent/config/config_test.go
  modified: [go.mod, go.sum]

key-decisions:
  - "ECDSA P-256 for all certificates (compact, fast, modern)"
  - "Random 128-bit serial numbers via crypto/rand"
  - "CA 10yr validity, leaf certs 2yr validity"
  - "MinVersion TLS 1.2 in both server and agent configs"
  - "Agent defaults: max_sessions=5, claude_cli_path=claude"

patterns-established:
  - "TDD workflow: failing test commit, then implementation commit"
  - "Config validation: returns descriptive field-path error (e.g. http.listen is required)"
  - "PEM file I/O pattern: write cert + key as separate PEM files"

requirements-completed: [AGNT-01, INFR-04]

# Metrics
duration: 4min
completed: 2026-03-12
---

# Phase 1 Plan 2: mTLS CA Tooling and Config Parsing Summary

**ECDSA P-256 CA with server/agent cert issuance, mTLS handshake verified, plus TOML config parsers with field-level validation**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-12T07:21:31Z
- **Completed:** 2026-03-12T07:25:16Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- CA tooling generates self-signed CA, server certs with SANs, and agent certs with machine-id CN
- Full mTLS handshake integration test passes (CA trusts both leaf certs)
- Server and agent TOML config parsing with clear validation error messages
- Agent config applies sensible defaults (max_sessions=5, claude_cli_path="claude")

## Task Commits

Each task was committed atomically:

1. **Task 1: CA tooling with mTLS cert generation**
   - `0cc57f3` (test) - Failing tests for CA and mTLS handshake
   - `5629e6a` (feat) - CA tooling and TLS config builders implementation
2. **Task 2: TOML config parsing for server and agent**
   - `4bf6674` (test) - Failing tests for config parsing
   - `3d107bd` (feat) - Config parsing implementation

## Files Created/Modified
- `internal/shared/tlsutil/ca.go` - CA generation and cert issuance (GenerateCA, IssueServerCert, IssueAgentCert)
- `internal/shared/tlsutil/ca_test.go` - 8 tests including full mTLS handshake
- `internal/shared/tlsutil/loader.go` - TLS config builders (ServerTLSConfig, AgentTLSConfig)
- `internal/server/config/config.go` - Server TOML config struct and loader with validation
- `internal/server/config/config_test.go` - 7 tests for server config
- `internal/agent/config/config.go` - Agent TOML config struct and loader with defaults
- `internal/agent/config/config_test.go` - 7 tests for agent config

## Decisions Made
- ECDSA P-256 for all certificates -- compact keys, modern, well-supported in Go stdlib
- Random 128-bit serial numbers for all certs (not sequential or hardcoded)
- CA validity 10 years, leaf cert validity 2 years matching the design doc
- MinVersion TLS 1.2 set on both ServerTLSConfig and AgentTLSConfig
- Agent defaults max_sessions=5 and claude_cli_path="claude" applied before validation

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- CA tooling ready to be wired into cobra `ca init`, `ca issue-server`, `ca issue-agent` commands
- Config parsers ready to be loaded in server and agent entrypoints
- TLS config builders ready for gRPC server/dial options

---
*Phase: 01-foundation*
*Completed: 2026-03-12*

## Self-Check: PASSED

All 7 files verified present. All 4 task commits verified in git history.
