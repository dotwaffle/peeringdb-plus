# Phase 3: Production Readiness - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Add comprehensive OpenTelemetry observability (traces, metrics, logs), health/readiness endpoints, and deploy globally on Fly.io with LiteFS for edge-local reads. This phase takes the working application from Phases 1-2 and makes it production-grade.

</domain>

<decisions>
## Implementation Decisions

### OpenTelemetry Setup
- **D-01:** Export telemetry via OTLP protocol — works with any OTel-compatible backend
- **D-02:** Configurable sampling rate via PDBPLUS_OTEL_SAMPLE_RATE env var, default to always sample (1.0)
- **D-03:** slog output flows through OTel log pipeline AND stdout — dual output
- **D-04:** Allow disabling tracing, metrics, and logs via OTel individually — Fly.io may not support all signals
- **D-05:** Track HTTP request metrics (latency/count), sync metrics (duration/status), and Go runtime metrics (goroutines, memory)
- **D-06:** Centralized OTel setup in `internal/otel` package
- **D-07:** Use standard OTel env vars (OTEL_EXPORTER_OTLP_ENDPOINT etc.) — follow OTel convention, no custom env vars
- **D-08:** Service version via `debug.ReadBuildInfo()` at runtime — no ldflags build-time injection
- **D-09:** W3C TraceContext propagation via traceparent/tracestate headers — enables distributed tracing
- **D-10:** OTel resource attributes include Fly.io-specific data: FLY_REGION, FLY_MACHINE_ID, FLY_APP_NAME

### Health & Monitoring
- **D-11:** Health endpoint design at Claude's discretion (separate liveness/readiness or combined)
- **D-12:** Sync freshness threshold configurable via PDBPLUS_SYNC_STALE_THRESHOLD env var, default 24 hours
- **D-13:** Health endpoints return detailed JSON with component status (db, sync, otel) — HTTP status code reflects overall health (200 OK / 503 unhealthy)
- **D-14:** Metrics exported via OTel push only — no Prometheus /metrics endpoint

### Fly.io Deployment
- **D-15:** Start with single Fly.io region — add more regions later
- **D-16:** Machine size: shared-cpu-1x with 512MB RAM
- **D-17:** LiteFS with Consul static leasing for leader election — more resilient primary failover
- **D-18:** Include fly.toml template in the repo with sensible defaults
- **D-19:** No Litestream backup — data can always be re-synced from PeeringDB
- **D-20:** LiteFS as Dockerfile entrypoint / process supervisor — starts LiteFS first, then launches app
- **D-21:** Persistent Fly.io volume mounted at /data for SQLite database
- **D-22:** Manual deploy via `fly deploy` — no CI/CD pipeline initially
- **D-23:** Separate Dockerfile.prod for LiteFS deployment — keep Phase 1 Dockerfile for local dev
- **D-24:** Primary/replica role detection via LiteFS lease file (.primary) — standard LiteFS mechanism
- **D-25:** Fixed machine count per region — no auto-scaling initially
- **D-26:** App handles write-forwarding internally for /sync endpoint — checks if primary, returns redirect if not (does NOT rely on LiteFS proxy for this)
- **D-27:** Use Fly.io's built-in Consul at standard address — no custom Consul configuration

### Claude's Discretion
- Health endpoint path design (separate /healthz + /readyz vs combined /health)
- Exact OTel metric names and histogram buckets
- LiteFS configuration file details (litefs.yml)
- fly.toml region and machine count defaults
- Individual OTel signal disable env var naming

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Prior Phase Context
- `.planning/phases/01-data-foundation/01-CONTEXT.md` — Sync, schema, and infrastructure decisions
- `.planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md` — SQLite + entgo integration (automemlimit for Fly.io)
- `.planning/phases/02-graphql-api/02-CONTEXT.md` — Server setup, middleware, and GraphQL decisions

### Research Artifacts
- `.planning/research/STACK.md` — Technology recommendations (LiteFS version, OTel packages)
- `.planning/research/ARCHITECTURE.md` — System architecture, LiteFS single-writer pattern
- `.planning/research/PITFALLS.md` — LiteFS maintenance mode risks, operational pitfalls

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Basic OTel spans in sync worker (Phase 1, D-35)
- slog structured logging middleware (Phase 2, D-27)
- sync_status metadata table (Phase 1, D-26) — feeds health endpoints
- automemlimit setup (Phase 1 reference) — already handles Fly.io cgroup limits
- Dockerfile from Phase 1 (D-54) — base for Dockerfile.prod
- Environment variable configuration pattern (Phase 1, D-33)

### Established Patterns
- slog for structured logging throughout
- OTel span creation pattern from Phase 1 sync worker
- Env var configuration for all settings

### Integration Points
- OTel middleware wraps the existing HTTP handler from Phase 2
- Health endpoints mount alongside /graphql and /sync on the HTTP server
- LiteFS primary detection gates sync worker startup and auto-migration
- Dockerfile.prod extends Phase 1 Dockerfile with LiteFS layer
- fly.toml references Dockerfile.prod and configures volumes, health checks, regions

</code_context>

<specifics>
## Specific Ideas

- Dual slog output (OTel pipeline + stdout) ensures logs are always visible even if OTel backend is down
- Individual signal disable flags cover the case where Fly.io doesn't support all OTel signals yet
- 24-hour sync freshness default is generous — allows for transient PeeringDB outages without false alerts
- App-level write forwarding for /sync gives more control than LiteFS proxy (can add auth checks, logging)

</specifics>

<deferred>
## Deferred Ideas

- None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-production-readiness*
*Context gathered: 2026-03-22*
