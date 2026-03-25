---
phase: 21-infrastructure
plan: 01
subsystem: infra
tags: [litefs, fly-replay, h2c, http2, grpc-prereq, fly.io]

# Dependency graph
requires:
  - phase: 03-production-readiness
    provides: LiteFS proxy config, fly.toml, main.go server setup
provides:
  - LiteFS proxy removed from request path
  - Application serves traffic directly on port 8080
  - fly-replay write forwarding with Fly.io environment detection
  - HTTP/2 cleartext (h2c) support via http.Protocols
  - h2_backend enabled in fly.toml for HTTP/2 backend connections
affects: [22-grpc-api, connectrpc, grpc-setup]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "http.Protocols for h2c (Go 1.24+ stdlib, no external deps)"
    - "SyncHandlerInput struct pattern for testable handler extraction (CS-5)"
    - "FLY_REGION env var presence for Fly.io environment detection"
    - "fly-replay: region=PRIMARY_REGION for documented write forwarding"

key-files:
  created:
    - cmd/peeringdb-plus/main_test.go
  modified:
    - litefs.yml
    - fly.toml
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "Used http.Protocols (Go 1.24+ stdlib) instead of golang.org/x/net/http2/h2c for h2c support"
  - "Gate fly-replay on FLY_REGION presence to avoid broken replays in local dev"
  - "Return 503 not primary for non-Fly.io non-primary nodes instead of silently failing"
  - "Extract sync handler to newSyncHandler with SyncHandlerInput for testability"

patterns-established:
  - "Fly.io detection: os.Getenv(FLY_REGION) != empty means running on Fly.io"
  - "fly-replay header: region=PRIMARY_REGION (not undocumented leader value)"
  - "h2c server: http.Protocols.SetHTTP1(true) + SetUnencryptedHTTP2(true)"

requirements-completed: [INFRA-01, INFRA-02, INFRA-03, INFRA-04, INFRA-05]

# Metrics
duration: 6min
completed: 2026-03-25
---

# Phase 21 Plan 01: Infrastructure Summary

**LiteFS proxy removed, fly-replay fixed to region=PRIMARY_REGION with Fly.io detection, h2c enabled via stdlib http.Protocols**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-25T00:11:24Z
- **Completed:** 2026-03-25T00:17:28Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Removed LiteFS HTTP proxy from request path, application now serves directly on port 8080
- Fixed fly-replay header from undocumented "Fly-Replay: leader" to documented "fly-replay: region=PRIMARY_REGION" syntax
- Gated fly-replay emission on FLY_REGION environment variable presence (no broken replays in local dev)
- Enabled HTTP/2 cleartext (h2c) via Go 1.26 stdlib http.Protocols (no external dependencies)
- Added h2_backend = true to fly.toml for HTTP/2 backend connections from Fly.io edge
- Extracted sync handler to testable newSyncHandler function with comprehensive tests

## Task Commits

Each task was committed atomically:

1. **Task 1: Remove LiteFS proxy and configure fly.toml for h2c** - `8d62e3a` (chore)
2. **Task 2: Fix fly-replay header, add Fly.io detection, enable h2c, and add tests** (TDD)
   - RED: `cfb83b7` (test: failing tests)
   - GREEN: `2ddfe3d` (feat: implementation passing)

## Files Created/Modified
- `litefs.yml` - Removed proxy section, app serves directly on :8080
- `fly.toml` - Changed PDBPLUS_LISTEN_ADDR to :8080, added h2_backend = true
- `cmd/peeringdb-plus/main.go` - Extracted newSyncHandler with SyncHandlerInput, h2c via http.Protocols, fixed fly-replay
- `cmd/peeringdb-plus/main_test.go` - Tests for fly-replay (Fly.io replica, local non-primary, primary auth), h2c verification

## Decisions Made
- Used http.Protocols (Go 1.24+ stdlib) instead of golang.org/x/net/http2/h2c -- no external dependency needed, cleaner API (CS-0, MD-1)
- Gated fly-replay on FLY_REGION presence -- avoids broken replays in local dev where no Fly proxy exists
- Returns 503 "not primary" for non-Fly.io non-primary nodes -- clear error instead of silent failure
- Used SyncHandlerInput struct per CS-5 (>2 args) for testable handler extraction

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- h2c support is live, ready for ConnectRPC/gRPC service implementation
- fly-replay correctly routes write requests to primary on Fly.io
- All existing API surfaces (GraphQL, REST, PeeringDB compat, Web UI) continue working over HTTP/1.1

## Self-Check: PASSED

All 5 files verified present. All 3 commits verified in git log.

---
*Phase: 21-infrastructure*
*Completed: 2026-03-25*
