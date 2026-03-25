---
phase: 21-infrastructure
verified: 2026-03-25T12:00:00Z
status: passed
score: 6/6 must-haves verified
re_verification: false
---

# Phase 21: Infrastructure Verification Report

**Phase Goal:** Application serves traffic directly without LiteFS HTTP proxy, supporting HTTP/2 cleartext for native gRPC wire protocol
**Verified:** 2026-03-25
**Status:** PASSED
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| #   | Truth                                                                              | Status     | Evidence                                                                                          |
| --- | ---------------------------------------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------- |
| 1   | LiteFS proxy section is completely removed from litefs.yml                         | VERIFIED   | `litefs.yml` has zero matches for `proxy:`, no `addr:` or `target:` lines. Only exec/fuse/lease remain. |
| 2   | Application listens on port 8080 directly (not 8081 behind proxy)                  | VERIFIED   | `fly.toml` line 17: `PDBPLUS_LISTEN_ADDR = ":8080"`. Zero matches for `:8081` in fly.toml.        |
| 3   | Replica sync requests on Fly.io emit fly-replay: region=PRIMARY_REGION header      | VERIFIED   | `main.go` line 327-329: checks `FLY_REGION`, sets `fly-replay: region=` + `PRIMARY_REGION`. Test `TestSyncReplay_FlyReplica` passes (307 + header `region=lhr`). |
| 4   | Sync requests work directly without fly-replay when not on Fly.io                  | VERIFIED   | `main.go` line 334: returns 503 "not primary" when `FLY_REGION` is empty. Test `TestSyncReplay_LocalNonPrimary` passes. |
| 5   | Server accepts both HTTP/1.1 and HTTP/2 cleartext (h2c) connections                | VERIFIED   | `main.go` lines 239-241: `protocols.SetHTTP1(true)` + `protocols.SetUnencryptedHTTP2(true)`. Test `TestServerProtocols_H2C` makes h2c request and verifies `HTTP/2.0` proto. |
| 6   | fly.toml has h2_backend = true for HTTP/2 backend connections                      | VERIFIED   | `fly.toml` lines 48-49: `[http_service.http_options]` with `h2_backend = true`.                    |

**Score:** 6/6 truths verified

### Required Artifacts

| Artifact                          | Expected                                                        | Status   | Details                                                                                 |
| --------------------------------- | --------------------------------------------------------------- | -------- | --------------------------------------------------------------------------------------- |
| `litefs.yml`                      | LiteFS config without proxy section                             | VERIFIED | 26 lines, `exec:` present, no `proxy:` section                                         |
| `fly.toml`                        | Fly.io config with h2_backend and correct listen address        | VERIFIED | `h2_backend = true` at line 49, `PDBPLUS_LISTEN_ADDR = ":8080"` at line 17             |
| `cmd/peeringdb-plus/main.go`      | Server with h2c, fixed fly-replay, Fly.io environment detection | VERIFIED | `SetUnencryptedHTTP2` at line 241, `fly-replay` at line 329, `FLY_REGION` at line 327  |
| `cmd/peeringdb-plus/main_test.go` | Tests for fly-replay behavior and h2c                           | VERIFIED | `TestSyncReplay` tests at lines 17/43/69, `TestServerProtocols_H2C` at line 157. 213 lines total. |

### Key Link Verification

| From                          | To                             | Via                                       | Status | Details                                                  |
| ----------------------------- | ------------------------------ | ----------------------------------------- | ------ | -------------------------------------------------------- |
| `fly.toml`                    | `cmd/peeringdb-plus/main.go`   | `PDBPLUS_LISTEN_ADDR` env var             | WIRED  | fly.toml sets `:8080`, main.go reads via `config.Load()` |
| `litefs.yml`                  | `cmd/peeringdb-plus/main.go`   | LiteFS exec subprocess                    | WIRED  | litefs.yml exec cmd: `/usr/local/bin/peeringdb-plus`     |
| `cmd/peeringdb-plus/main.go`  | `fly.toml`                     | fly-replay header uses PRIMARY_REGION env | WIRED  | main.go line 328: `os.Getenv("PRIMARY_REGION")`, fly.toml line 21: `PRIMARY_REGION = "lhr"` |

### Data-Flow Trace (Level 4)

Not applicable -- this phase modifies infrastructure configuration and HTTP server setup. No dynamic data rendering artifacts.

### Behavioral Spot-Checks

| Behavior                                    | Command                                                                                      | Result              | Status |
| ------------------------------------------- | -------------------------------------------------------------------------------------------- | ------------------- | ------ |
| Tests pass with race detector               | `go test ./cmd/peeringdb-plus/... -race -count=1 -run "TestSync\|TestServer"`                | ok, 1.108s          | PASS   |
| Full suite passes (no regressions)          | `go test ./... -race -count=1`                                                                | All packages pass   | PASS   |
| go vet clean                                | `go vet ./cmd/peeringdb-plus/...`                                                            | No output (clean)   | PASS   |
| golangci-lint clean                         | `golangci-lint run ./cmd/peeringdb-plus/...`                                                  | 0 issues            | PASS   |
| Compilation succeeds                        | `go build ./cmd/peeringdb-plus`                                                               | Exit 0              | PASS   |
| h2c integration test verifies HTTP/2 proto  | `TestServerProtocols_H2C` (makes h2c request, checks `resp.Proto == "HTTP/2.0"`)              | PASS                | PASS   |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                                     | Status    | Evidence                                                                 |
| ----------- | ---------- | ----------------------------------------------------------------------------------------------- | --------- | ------------------------------------------------------------------------ |
| INFRA-01    | 21-01      | App listens directly on Fly.io internal port without LiteFS HTTP proxy intermediary             | SATISFIED | litefs.yml proxy removed, fly.toml PDBPLUS_LISTEN_ADDR=:8080             |
| INFRA-02    | 21-01      | Sync requests on replicas replayed to primary via fly-replay, gated on Fly.io env detection     | SATISFIED | main.go checks FLY_REGION, sets fly-replay: region=PRIMARY_REGION        |
| INFRA-03    | 21-01      | Sync requests handled directly (no replay) when not on Fly.io                                  | SATISFIED | main.go returns 503 "not primary" when FLY_REGION empty                  |
| INFRA-04    | 21-01      | Server supports HTTP/2 cleartext (h2c) alongside HTTP/1.1 via http.Protocols                   | SATISFIED | main.go SetHTTP1(true) + SetUnencryptedHTTP2(true), verified by h2c test |
| INFRA-05    | 21-01      | fly.toml configured with h2_backend for HTTP/2 to backend                                      | SATISFIED | fly.toml [http_service.http_options] h2_backend = true                   |

No orphaned requirements found.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | - | - | - | No anti-patterns detected in any modified files |

No TODO/FIXME/PLACEHOLDER comments, no stub implementations, no empty returns, no hardcoded empty data, no console.log patterns found in any of the four modified files.

### Human Verification Required

### 1. Fly.io Deployment Behavior

**Test:** Deploy to Fly.io and verify that POST /sync from a replica returns a `fly-replay: region=lhr` header and the Fly proxy actually replays the request to the primary node.
**Expected:** The Fly proxy intercepts the 307 with `fly-replay` header and transparently replays the request to the primary region. The sync completes on primary.
**Why human:** Requires actual Fly.io infrastructure with multi-region deployment. The fly-replay behavior depends on the Fly proxy, which cannot be simulated in tests.

### 2. h2c Over Fly.io Edge

**Test:** After deployment, verify that a gRPC/ConnectRPC client can establish an HTTP/2 connection through Fly.io's edge proxy to the application.
**Expected:** The `h2_backend = true` setting causes Fly.io edge to use HTTP/2 when connecting to the backend. An h2c-aware client successfully communicates with the server.
**Why human:** Requires verifying Fly.io edge proxy behavior with h2_backend configuration. Cannot test Fly.io edge behavior locally.

### 3. Existing API Surfaces Still Work

**Test:** After deployment, verify that GraphQL (/graphql), REST (/rest/v1/), PeeringDB compat (/api/), and Web UI (/ui/) continue to work correctly over HTTP/1.1.
**Expected:** All existing endpoints return correct responses. No regressions from removing the LiteFS proxy.
**Why human:** While tests pass locally, production environment with LiteFS, Consul, and Fly.io edge may behave differently.

### Gaps Summary

No gaps found. All 6 observable truths are verified. All 5 requirements are satisfied. All artifacts exist, are substantive, and are wired correctly. All tests pass with race detector. No anti-patterns detected. Three items flagged for human verification are deployment-environment behaviors that cannot be tested programmatically.

---

_Verified: 2026-03-25_
_Verifier: Claude (gsd-verifier)_
