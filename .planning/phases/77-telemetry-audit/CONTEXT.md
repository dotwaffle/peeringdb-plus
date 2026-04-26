---
phase: 77
slug: telemetry-audit
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 77 Context: Telemetry Audit & Cleanup

## Goal

Audit and remediate Loki log levels (find chatty INFOs that should be DEBUG, mis-WARNed routine events) and proactively introduce per-endpoint Tempo trace sampling rules to keep trace volume sustainable through the next feature cycle.

## Requirements

- **OBS-06** — Loki log-level audit + reclassifications applied
- **OBS-07** — Tempo trace audit + new proactive sampling rules

## Locked decisions

- **D-01 — OBS-06: audit + fix in same phase.** Produce `.planning/phases/77-telemetry-audit/AUDIT.md` documenting every log-level finding with before/after rationale, AND apply the slog level changes inline. Single phase delivers analysis + cleanup. Approach:
  1. Sample 30 minutes of production Loki logs at INFO level — catalogue what's chatty.
  2. Sample 30 minutes at WARN — catalogue what fires under non-error conditions.
  3. Sample 30 minutes at DEBUG — verify it's actually useful and not noise (DEBUG can sometimes accumulate dead instrumentation).
  4. For each finding, document: source file + line, current level, recommended level, rationale.
  5. Apply changes via `slog` level adjustments or full removal (where the log was DEBUG-grade noise).
  
  Likely candidates flagged in CLAUDE.md and prior phases:
  - Per-step sync DEBUG logs ("upserting type X") — currently DEBUG, may be misclassified
  - FK-orphan summary log — Phase 68 already moved per-row to DEBUG and added a per-cycle WARN summary; verify the WARN is still firing and accurate
  - `_visible` field redaction logs (Phase 64) — verify these are at DEBUG/silent on routine traffic
  - Sync-cycle entry/exit INFO — likely correct as INFO; flag if redundant with the OTel span
  - Health-check 200 responses — should be silent (otelhttp may already log; verify)

- **D-02 — OBS-07: audit + add new sampling rules proactively.** This is the more aggressive of the three options considered. Scope:
  1. Verify current state: max trace size <2 MB, trace sampling at 1.0, batching at PERF-08 settings (`OTEL_BSP_SCHEDULE_DELAY=5s`, `OTEL_BSP_MAX_EXPORT_BATCH_SIZE=512`).
  2. Identify high-volume / low-value endpoints (almost certainly `/healthz` and `/readyz` — Fly health-check traffic dominates).
  3. Add per-endpoint sampling: `/healthz` and `/readyz` at low ratio (e.g., 0.01 — keep 1% for liveness debugging), critical paths (`/api/*`, `/rest/v1/*`, `/peeringdb.v1.*`) at 1.0, mid-traffic paths (`/ui/*`, `/graphql`) at 0.5 (TBD based on actual volume).
  4. Implementation: per-route sampling via `sdktrace.ParentBased(sdktrace.TraceIDRatioBased(r))` with route-specific samplers, OR a custom Sampler that inspects the span's `http.route` attribute (depends on OBS-04's fix landing first — hard dependency).
  5. Document the sampling matrix in `docs/ARCHITECTURE.md § Observability`.
  
  Watch out for: per-endpoint sampling can break cross-service trace continuity. If `/api/foo` calls `/internal/bar`, sampling-out the parent on `/healthz` while sampling-in `/internal/bar` produces orphaned spans. Use `ParentBased` to inherit the sampling decision down the call chain.

## Out of scope

- Migrating away from OTel autoexport — keep the existing pipeline.
- Adding new metric instruments — this phase is logs + traces only.
- Replacing Loki/Tempo with self-hosted alternatives — out of scope.
- Rate-limiting application logs at the application layer — slog levels are the lever; if needed beyond that, defer.
- Loki log structured-attribute audit (e.g., are we logging too many attrs per record?) — narrow to level audit only; structure audit is a separate concern.

## Dependencies

- **Depends on**: Phase 75 (hard) — OBS-04's `http.route` middleware fix is required for D-02's per-endpoint sampling rules to dispatch correctly. Without `http.route` populating, `/api/*` and `/healthz` look identical to a route-based sampler.
- **Enables**: Cleaner production logs and predictable trace volume into v1.19+ feature work.

## Plan hints for executor

- Touchpoints:
  - `internal/otel/provider.go` — `Setup()` `sdktrace.NewTracerProvider(...)` — wrap the existing `TraceIDRatioBased(in.SampleRate)` with a `ParentBased` per-route composite sampler
  - `internal/middleware/` — possibly extend to set sampling decisions before the trace span starts (depends on OTel's sampling-decision lifecycle vs middleware ordering)
  - `internal/sync/worker.go`, `internal/pdbcompat/handler.go`, `cmd/peeringdb-plus/main.go` — log-level adjustments per AUDIT.md
  - `internal/sync/upsert.go` — verify per-step logs are at DEBUG not INFO
  - `docs/ARCHITECTURE.md § Observability` — extend with the sampling matrix
  - `.planning/phases/77-telemetry-audit/AUDIT.md` — produce findings doc (committed alongside code changes)
- Reference docs:
  - CLAUDE.md § Sync observability — existing doc of memory telemetry, fk-orphan summary, etc.
  - PERF-08 baseline (`f7da22d fix(otel): configure batching and reduce trace bloat`) — reference for current trace settings
  - OpenTelemetry Go SDK Sampler interface docs — for D-02 per-endpoint sampler implementation
- Verify on completion:
  - Production Loki query volume measurably down post-merge (qualitative or via the GC usage stats datasource if accessible)
  - Tempo trace inspection: max trace size remains <2 MB; per-endpoint sample ratios match the documented matrix
  - `OTEL_BSP_*` parameters confirmed (or adjusted with justification in AUDIT.md)
  - Cross-service trace continuity preserved (sampling decision inherits via `ParentBased`)
  - `go test -race ./...` clean
  - `golangci-lint run ./...` clean
