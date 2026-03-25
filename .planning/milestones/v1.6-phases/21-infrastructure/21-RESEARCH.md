# Phase 21: Infrastructure - Research

**Researched:** 2026-03-24
**Domain:** HTTP/2 cleartext (h2c), Fly.io routing, LiteFS proxy removal
**Confidence:** HIGH

## Summary

This phase removes the LiteFS built-in HTTP proxy from the request path, replaces it with application-level fly-replay write forwarding, and enables HTTP/2 cleartext (h2c) support. These changes are prerequisites for serving ConnectRPC/gRPC traffic, which requires HTTP/2 features (trailers, bidirectional streaming framing) that the LiteFS proxy cannot pass through.

The current architecture has Fly.io edge -> LiteFS proxy (:8080) -> application (:8081). The target architecture is Fly.io edge -> application (:8080) directly. LiteFS continues to run as the FUSE filesystem and subprocess supervisor, but its HTTP proxy is removed. The application takes over write forwarding (fly-replay header on POST /sync) and port binding (listen on :8080 directly). Go 1.26's native `http.Protocols` API (introduced in Go 1.24) provides clean h2c support without any external dependencies.

**Primary recommendation:** Remove the LiteFS `proxy:` section from litefs.yml, change the app listen address to `:8080`, configure `http.Server.Protocols` with HTTP1 + UnencryptedHTTP2, fix the fly-replay header to use `region=` syntax, add `h2_backend = true` to fly.toml, and gate fly-replay on Fly.io environment detection.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- all implementation choices are at Claude's discretion for this infrastructure phase.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- infrastructure phase.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| INFRA-01 | App listens directly on Fly.io internal port without LiteFS HTTP proxy intermediary | Remove `proxy:` section from litefs.yml, change `PDBPLUS_LISTEN_ADDR` to `:8080`, update `fly.toml` `internal_port` comment |
| INFRA-02 | Sync requests on replicas are replayed to primary via fly-replay response header, gated on Fly.io environment detection | Fix existing `Fly-Replay: leader` to use correct `fly-replay: region=<PRIMARY_REGION>` syntax, gate on `FLY_REGION` env var presence |
| INFRA-03 | Sync requests are handled directly (no replay) when not running on Fly.io | When `FLY_REGION` is empty, no fly-replay header is emitted; existing `isPrimaryFn()` already defaults to true in local dev |
| INFRA-04 | Server supports HTTP/2 cleartext (h2c) alongside HTTP/1.1 via http.Protocols | Use Go 1.24+ native `http.Protocols` type: `SetHTTP1(true)` + `SetUnencryptedHTTP2(true)` on `http.Server.Protocols` |
| INFRA-05 | fly.toml configured with h2_backend for HTTP/2 to backend | Add `[http_service.http_options]` section with `h2_backend = true` |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| net/http (stdlib) | Go 1.26 | HTTP server with h2c support | `http.Protocols` introduced in Go 1.24, provides native HTTP/1.1 + h2c dual-protocol on same port. No external dependency needed. |
| os (stdlib) | Go 1.26 | Fly.io environment detection | `FLY_REGION` and `PRIMARY_REGION` env vars are set by Fly.io on all machines |

### Supporting
No additional libraries needed. This phase uses only stdlib changes and configuration file edits.

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| http.Protocols (stdlib) | golang.org/x/net/http2/h2c | External dep, wraps handler with h2c.NewHandler(). Stdlib approach is cleaner, no dependency, and is the modern Go 1.24+ way. |
| Region-based fly-replay | Instance-based fly-replay | `region=PRIMARY_REGION` is simpler and aligns with how the app already uses PRIMARY_REGION. Instance-based would require tracking machine IDs. |

## Architecture Patterns

### Current Architecture (Before)
```
Fly.io Edge (HTTPS termination)
  -> LiteFS proxy (:8080, HTTP/1.1 only)
    -> Application (:8081)
       LiteFS handles:
       - Write forwarding (POST/PUT -> fly-replay to primary)
       - Read consistency (waits for replication position via cookie)
```

### Target Architecture (After)
```
Fly.io Edge (HTTPS termination, h2_backend=true)
  -> Application (:8080, HTTP/1.1 + h2c)
     Application handles:
     - Write forwarding (POST /sync -> fly-replay: region=PRIMARY_REGION)
     - No read consistency tracking (acceptable for read-only mirror)
     LiteFS still provides:
     - FUSE filesystem mount (/litefs)
     - Subprocess supervision (exec section)
     - SQLite replication
     - Consul-based leader election
```

### Pattern 1: Fly.io Environment Detection
**What:** Detect whether the application is running on Fly.io by checking for `FLY_REGION` env var.
**When to use:** Before emitting fly-replay headers that only work on Fly.io.
**Example:**
```go
// Source: https://fly.io/docs/blueprints/multi-region-fly-replay/
// FLY_REGION is set on all Fly.io machines. Its absence means local dev/tests.
func isOnFlyio() bool {
    return os.Getenv("FLY_REGION") != ""
}
```

### Pattern 2: Fly-Replay Region Routing
**What:** Route write requests to the primary region using the fly-replay response header.
**When to use:** When a replica receives a POST /sync request on Fly.io.
**Example:**
```go
// Source: https://fly.io/docs/networking/dynamic-request-routing/
// Source: https://fly.io/docs/blueprints/multi-region-fly-replay/
func syncHandler(w http.ResponseWriter, r *http.Request) {
    if !isPrimaryFn() && isOnFlyio() {
        primaryRegion := os.Getenv("PRIMARY_REGION")
        w.Header().Set("fly-replay", "region="+primaryRegion)
        w.WriteHeader(http.StatusTemporaryRedirect)
        return
    }
    // Handle sync directly
}
```

### Pattern 3: h2c Configuration via http.Protocols
**What:** Enable both HTTP/1.1 and HTTP/2 cleartext on same port using stdlib.
**When to use:** When the server must accept gRPC (HTTP/2) and regular HTTP (HTTP/1.1).
**Example:**
```go
// Source: https://pkg.go.dev/net/http@go1.26.1
var protocols http.Protocols
protocols.SetHTTP1(true)
protocols.SetUnencryptedHTTP2(true)

server := &http.Server{
    Addr:      ":8080",
    Handler:   handler,
    Protocols: &protocols,
}
```

### Anti-Patterns to Avoid
- **Using `Fly-Replay: leader`:** Not a valid fly-replay field. The current code uses this undocumented value. Must be changed to `fly-replay: region=<PRIMARY_REGION>`.
- **Using `golang.org/x/net/http2/h2c` package:** Unnecessary with Go 1.24+ stdlib support. The external package wraps handlers and adds complexity.
- **Removing LiteFS entirely from the container:** LiteFS is still needed for FUSE mount, SQLite replication, and subprocess supervision. Only the proxy section is removed.
- **Hardcoding the primary region in Go code:** Use `os.Getenv("PRIMARY_REGION")` which is already set in fly.toml's `[env]` section.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP/2 cleartext negotiation | Custom h2c upgrade handler | `http.Protocols.SetUnencryptedHTTP2(true)` | Stdlib handles HTTP/2 Prior Knowledge (RFC 9113 section 3.3) correctly; custom handlers miss edge cases |
| Fly.io primary routing | Custom DNS/IP-based routing | `fly-replay: region=PRIMARY_REGION` header | Fly proxy handles replay, retries, and timeout natively |
| Fly.io environment detection | Checking hostname patterns or IP ranges | `FLY_REGION` env var presence | Fly.io guarantees this env var is set on all machines |

**Key insight:** The LiteFS proxy provided two features -- write forwarding and read consistency. This project only needs write forwarding (for POST /sync). Read consistency via cookie tracking is not needed for a read-only mirror where eventual consistency is acceptable.

## Common Pitfalls

### Pitfall 1: Forgetting to Remove the LiteFS Proxy Port Split
**What goes wrong:** If litefs.yml still has the `proxy:` section but fly.toml points to :8080, LiteFS proxy will intercept requests and forward to :8081, but the app is now listening on :8080 instead.
**Why it happens:** The proxy and listen address changes must be synchronized across litefs.yml, fly.toml, and the app's PDBPLUS_LISTEN_ADDR env var.
**How to avoid:** Remove the entire `proxy:` section from litefs.yml, change PDBPLUS_LISTEN_ADDR to `:8080` in fly.toml, and verify the internal_port stays at 8080.
**Warning signs:** 502 errors on deploy, health checks failing, "connection refused" in logs.

### Pitfall 2: Using Invalid fly-replay Header Syntax
**What goes wrong:** The existing code uses `Fly-Replay: leader` which is not documented. While it may have worked historically, it is not a valid field in the current Fly.io documentation.
**Why it happens:** Early Fly.io documentation or examples may have used informal syntax.
**How to avoid:** Use `fly-replay: region=<PRIMARY_REGION>` per current Fly.io documentation. The header name is case-insensitive per HTTP spec, but the value must use `region=` field syntax.
**Warning signs:** Replay silently not working, sync requests on replicas returning 307 but Fly proxy not replaying.

### Pitfall 3: h2c Without fly.toml h2_backend
**What goes wrong:** The Go server supports h2c, but Fly.io's edge proxy still sends HTTP/1.1 to the backend, so gRPC trailers are lost.
**Why it happens:** Fly.io edge defaults to HTTP/1.1 backend connections unless explicitly told to use HTTP/2.
**How to avoid:** Add `[http_service.http_options]` with `h2_backend = true` to fly.toml.
**Warning signs:** gRPC clients get "unexpected content-type" errors, trailers are missing.

### Pitfall 4: Breaking Local Development by Requiring Fly.io Environment
**What goes wrong:** The sync endpoint requires FLY_REGION to be set, or fly-replay logic breaks.
**Why it happens:** Not gating Fly-specific behavior on environment detection.
**How to avoid:** Gate fly-replay emission on `os.Getenv("FLY_REGION") != ""`. When FLY_REGION is empty (local dev), skip replay and handle sync directly using existing isPrimaryFn() logic.
**Warning signs:** POST /sync returns 307 in local dev but nothing replays the request.

### Pitfall 5: Request Size Limit on fly-replay
**What goes wrong:** Requests larger than 1MB cannot be replayed by Fly proxy.
**Why it happens:** Fly proxy buffers the request body for replay and enforces a 1MB limit.
**How to avoid:** POST /sync has no body (fire-and-forget), so this is not a concern for this application. Document the limitation for future reference.
**Warning signs:** N/A for this use case.

## Code Examples

Verified patterns from official sources:

### h2c Server Configuration
```go
// Source: https://pkg.go.dev/net/http@go1.26.1#Protocols
var protocols http.Protocols
protocols.SetHTTP1(true)
protocols.SetUnencryptedHTTP2(true)

server := &http.Server{
    Addr:      cfg.ListenAddr,
    Handler:   handler,
    Protocols: &protocols,
}
```

### fly-replay Write Forwarding
```go
// Source: https://fly.io/docs/blueprints/multi-region-fly-replay/
// Source: https://fly.io/docs/networking/dynamic-request-routing/
mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
    if !isPrimaryFn() {
        // On Fly.io, replay to primary region.
        if flyRegion := os.Getenv("FLY_REGION"); flyRegion != "" {
            primaryRegion := os.Getenv("PRIMARY_REGION")
            w.Header().Set("fly-replay", "region="+primaryRegion)
            w.WriteHeader(http.StatusTemporaryRedirect)
            return
        }
        // Not on Fly.io (local dev) -- non-primary should not handle sync.
        // isPrimaryFn() defaults to true in local dev, so this path
        // is only reached if explicitly set to non-primary.
        http.Error(w, "not primary", http.StatusServiceUnavailable)
        return
    }
    // ... existing sync logic (token check, mode, fire-and-forget)
})
```

### litefs.yml Without Proxy
```yaml
# LiteFS configuration -- proxy section removed for direct app serving
exit-on-error: false

fuse:
  dir: "/litefs"

data:
  dir: "/var/lib/litefs"
  compress: true
  retention: "10m"

exec:
  - cmd: "/usr/local/bin/peeringdb-plus"

lease:
  type: "consul"
  candidate: ${FLY_REGION == PRIMARY_REGION}
  promote: true
  advertise-url: "http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202"
  consul:
    url: "${FLY_CONSUL_URL}"
    key: "litefs/${FLY_APP_NAME}"
```

### fly.toml With h2_backend
```toml
[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "off"
  auto_start_machines = true
  min_machines_running = 1

  [http_service.http_options]
    h2_backend = true

  [http_service.concurrency]
    type = "requests"
    soft_limit = 10
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `golang.org/x/net/http2/h2c` handler wrapper | `http.Protocols.SetUnencryptedHTTP2(true)` | Go 1.24 (Feb 2025) | No external dependency, cleaner API, built into http.Server |
| LiteFS proxy for write forwarding | Application-level fly-replay | Always available; LiteFS proxy was optional convenience | More control, supports HTTP/2 passthrough |
| `Fly-Replay: leader` header | `fly-replay: region=<region>` header | Current docs (2025-2026) | Documented, supported syntax with region field |

**Deprecated/outdated:**
- `golang.org/x/net/http2/h2c`: Still works but unnecessary with Go 1.24+ stdlib
- LiteFS proxy write forwarding: Works but blocks HTTP/2 passthrough needed for gRPC
- `handlers = ["tls"]` in fly.toml: Deprecated by Fly.io; use `h2_backend` instead

## Open Questions

1. **LiteFS proxy provided read-consistency cookie tracking -- is losing this acceptable?**
   - What we know: The proxy set a cookie with the replication position so subsequent reads waited for replicas to catch up. This project is a read-only mirror with hourly sync, so strong read-after-write consistency is not needed.
   - What's unclear: Nothing -- this is acceptable.
   - Recommendation: Accept eventual consistency. Document the tradeoff.

2. **Does `Fly-Replay: leader` actually work today?**
   - What we know: It is not documented in current Fly.io documentation. The documented fields are: region, instance, prefer_instance, app, state, elsewhere, timeout, fallback.
   - What's unclear: Whether Fly proxy silently handles "leader" as an alias for region-based routing.
   - Recommendation: Replace with documented `region=PRIMARY_REGION` syntax regardless. Do not rely on undocumented behavior.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.24+ (http.Protocols) | INFRA-04 (h2c) | Yes | Go 1.26.1 | -- |
| Fly.io (fly-replay) | INFRA-02 | N/A (deploy target) | -- | Direct sync handling when FLY_REGION absent |
| LiteFS | FUSE mount, replication | Yes (in Docker image) | 0.5.x | -- |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None needed (go test convention) |
| Quick run command | `go test ./internal/litefs/... ./internal/config/... -race -count=1` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INFRA-01 | App listens on :8080 directly, LiteFS proxy removed | config + integration | `go test ./internal/config/... -race -count=1 -run TestLoad` | Yes (config_test.go) |
| INFRA-02 | Replica POST /sync returns fly-replay with region= on Fly.io | unit | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestSyncReplay` | No -- Wave 0 |
| INFRA-03 | POST /sync handled directly without replay in local dev | unit | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestSyncLocal` | No -- Wave 0 |
| INFRA-04 | Server supports HTTP/1.1 and h2c on same port | integration | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestH2C` | No -- Wave 0 |
| INFRA-05 | fly.toml has h2_backend | manual (config file inspection) | N/A | N/A |

### Sampling Rate
- **Per task commit:** `go test ./internal/litefs/... ./internal/config/... -race -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] Sync handler tests for fly-replay behavior (Fly.io vs local) -- currently inline in main.go, may need to extract to testable function
- [ ] h2c integration test -- start server, make HTTP/2 prior-knowledge request, verify response

## Detailed Change Inventory

### Files to Modify
| File | Change | Rationale |
|------|--------|-----------|
| `litefs.yml` | Remove entire `proxy:` section (lines 17-24) | INFRA-01: No more proxy in request path |
| `fly.toml` | Change `PDBPLUS_LISTEN_ADDR` from `:8081` to `:8080`, add `[http_service.http_options]` with `h2_backend = true`, update comment | INFRA-01, INFRA-05 |
| `cmd/peeringdb-plus/main.go` | Configure `http.Server.Protocols` for h2c, fix fly-replay header syntax, gate on Fly.io detection | INFRA-02, INFRA-03, INFRA-04 |

### Files Unchanged
| File | Why |
|------|-----|
| `Dockerfile.prod` | LiteFS is still the ENTRYPOINT (`litefs mount`). FUSE mount and subprocess supervision remain. |
| `internal/litefs/primary.go` | Primary detection logic unchanged. Still checks lease file with env fallback. |
| `internal/config/config.go` | Listen address default remains `:8080`. The env var in fly.toml changes from `:8081` to `:8080`. |
| `internal/health/handler.go` | Health endpoints unchanged. |

## Project Constraints (from CLAUDE.md)

Directives affecting this phase:

- **CS-0 (MUST):** Modern Go code guidelines -- use http.Protocols (Go 1.24+ API), not x/net/http2/h2c
- **ERR-1 (MUST):** Wrap errors with %w and context
- **CFG-1 (MUST):** Config via env/flags; validate on startup; fail fast -- listen address and Fly.io detection via env vars
- **CFG-2 (MUST):** Config immutable after init; pass explicitly -- no runtime config changes
- **SEC-1 (MUST):** Validate inputs; set explicit I/O timeouts -- existing sync token validation preserved
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic
- **T-2 (MUST):** Run -race in CI; add t.Cleanup for teardown
- **OBS-1 (MUST):** Structured logging (slog) with levels and consistent fields
- **TL-4 (CAN):** APIs: buf for Protobuf -- relevant for later phases, not this one
- **MD-1 (SHOULD):** Prefer stdlib -- h2c via stdlib, no external deps needed
- **CS-5 (MUST):** Use input structs for functions with >2 args

## Sources

### Primary (HIGH confidence)
- [Go 1.26 net/http package docs](https://pkg.go.dev/net/http@go1.26.1) - Protocols type API, Server.Protocols field, SetUnencryptedHTTP2
- [Go 1.24 release notes](https://go.dev/doc/go1.24) - Introduction of http.Protocols with h2c support
- `go doc net/http.Protocols` - Local Go 1.26.1 runtime verification of API
- [Fly.io fly-replay docs](https://fly.io/docs/networking/dynamic-request-routing/) - Valid fly-replay fields: region, instance, prefer_instance, app, state, elsewhere, timeout, fallback
- [Fly.io multi-region fly-replay blueprint](https://fly.io/docs/blueprints/multi-region-fly-replay/) - `fly-replay: region=PRIMARY_REGION` pattern
- [Fly.io fly.toml configuration](https://fly.io/docs/reference/configuration/) - h2_backend in http_service.http_options
- [Fly.io gRPC deployment guide](https://fly.io/docs/app-guides/grpc-and-grpc-web-services/) - h2_backend=true + alpn=["h2"] for gRPC
- [Fly.io LiteFS proxy docs](https://fly.io/docs/litefs/proxy/) - Proxy is optional, handles write forwarding + read consistency
- [Fly.io LiteFS config reference](https://fly.io/docs/litefs/config/) - proxy section disabled by default, exec subprocess supervision

### Secondary (MEDIUM confidence)
- [Fly.io community h2_backend thread](https://community.fly.io/t/h2-backend-no-host-header/27100) - Confirmed h2_backend Host header behavior

### Tertiary (LOW confidence)
- None -- all findings verified with official documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Go 1.26 stdlib, verified via `go doc`, no external deps
- Architecture: HIGH - Current codebase fully inspected, all change points identified
- Pitfalls: HIGH - Verified fly-replay syntax against official docs, identified existing bug (Fly-Replay: leader)

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable infrastructure, unlikely to change)
