---
phase: 75
slug: code-side-observability
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 75 Context: Code-side Observability Fixes

## Goal

Three code-side changes to fix observability gaps surfaced in the 2026-04-26 telemetry audit: cold-start gauge population, zero-rate counter pre-warming for dashboard panels, and `http.route` middleware investigation + fix.

## Requirements

- **OBS-01** — `pdbplus_data_type_count` gauge correct within 30s of process startup (no false zeros)
- **OBS-02** — Zero-rate counter panels render `0` not `No data` after fresh deploy
- **OBS-04** — `http_route` label populates for all routes, not just `GET /healthz`

## Locked decisions

- **D-01 — OBS-01: synchronous one-shot COUNT(*) at process init.** Run `SELECT COUNT(*) FROM <table> WHERE status IN (ok, pending) GROUP BY type` for each of the 13 entity tables at startup, before the OTel ObservableGauge callback registers. Synchronous (~1-2s startup cost). Replicas already hydrate the LiteFS DB in 5-45s; +1-2s is noise on top. Implementation: a new `internal/sync/initialcounts.go` (or add to existing `internal/otel/metrics.go` `InitObjectCountGauges`) that runs the count queries once and seeds the same cache currently primed by sync-completion. Sync-completion path is unchanged — just no longer the SOLE path that primes the cache.

- **D-02 — OBS-02: pre-warm 13 types only (no status-cardinality pre-warm).** Each of the 5 zero-rate counters (`pdbplus_sync_type_fallback_total`, `pdbplus_role_transitions_total`, `pdbplus_sync_type_upsert_errors_total`, `pdbplus_sync_type_fetch_errors_total`, `pdbplus_sync_type_deleted_total`) gets `Counter.Add(ctx, 0, attribute.String("type", t))` called once per type at startup, where `t` ranges over the 13 entity types. Total baseline: 13 types × 5 metrics = 65 baseline series. Status dimension self-populates on first real event — accept some "No data" on multi-attr panels until the first real event fires. `pdbplus_role_transitions_total` is special: pre-warm with `direction={promoted,demoted}` instead of `type=` (the metric is per-direction, not per-entity). Place the pre-warm in `cmd/peeringdb-plus/main.go` after `InitMetrics()` but before any sync goroutine starts.

- **D-03 — OBS-04: investigate root cause + fix the middleware.** Likely root causes for `http.route` only populating on `/healthz`:
  1. `r.Pattern` is empty for routes registered without a method prefix (Go 1.22+ `ServeMux` only populates `Pattern` for `METHOD /path` registrations); `/healthz` is registered as `GET /healthz` while other routes may use bare `/api/` etc.
  2. Middleware ordering: `routeTagMiddleware` runs BEFORE the mux dispatch, so `r.Pattern` isn't populated yet at the time the OTel labeler reads it.
  3. The OTel label is set via `LabelerFromContext(r.Context()).Add(...)` but the labeler context is replaced by a downstream middleware before the request finishes.
  
  Investigation order: (1) read `cmd/peeringdb-plus/main.go` middleware chain order, (2) verify `r.Pattern` is populated post-mux for non-healthz routes via a temporary `slog.DebugContext` log, (3) check otelhttp v0.68.0 source for LabelerFromContext lifecycle. The fix is whichever root cause is actually in play. Deliverable: `count by(http_route)(http_server_request_duration_seconds_count)` returns ≥5 distinct routes during normal traffic.

## Out of scope

- Adding new HTTP metrics — this phase fixes the existing `http.route` label, doesn't introduce new instruments.
- Per-endpoint HTTP histograms (e.g., separate buckets for `/api/*` vs `/rest/v1/*`) — possible future work but not required by OBS-04.
- Replacing otelhttp with a different instrumentation library — out of scope.
- Pre-warming the per-type sync metrics that DO populate naturally on every cycle (`pdbplus_sync_type_objects_total`, `pdbplus_sync_duration_seconds`) — they're not in the OBS-02 list.

## Dependencies

- **Depends on**: None.
- **Enables**: Phase 76 (dashboard hardening) has a soft dependency — visual confirmation of OBS-01/02 panels rendering correctly is the natural QA for OBS-03's filter sweep. Phase 77 (telemetry audit) has a hard dependency — OBS-04's `http.route` fix changes log/trace shape that 77 audits.

## Plan hints for executor

- Touchpoints:
  - `internal/otel/metrics.go` — extend `InitObjectCountGauges` to take an initial-counts callback OR add a new `InitInitialObjectCounts(ctx, dbClient)` helper
  - `cmd/peeringdb-plus/main.go` — call the new initial-counts helper after DB open, before the HTTP server starts; also add the OBS-02 pre-warm `.Add(0, ...)` calls for the 5 zero-rate metrics
  - `internal/middleware/route_tag.go` (or wherever `routeTagMiddleware` lives, likely added by 260426-lod) — investigate + fix
  - `cmd/peeringdb-plus/main_test.go` — add an HTTP test asserting that `http.route` label is set for non-healthz routes (use httptest server + otelhttp + a labeler inspection)
- Reference docs:
  - `docs/ARCHITECTURE.md § Response Memory Envelope` (existing OBS docs to extend)
  - CLAUDE.md § OTel resource attributes (post-260426-lod) — explains the `cloud_region` / `service_namespace` allowlist context
  - `internal/otel/metrics.go` `InitMetrics()` — existing pre-warm-friendly registration site
- Verify on completion:
  - Post-deploy: `pdbplus_data_type_count` shows correct values within 30s (verify via Grafana instant query immediately after deploy)
  - Post-deploy: dashboard panels for fallback/role-transitions/errors/deletes show `0` not `No data`
  - Post-deploy: `count by(http_route)(http_server_request_duration_seconds_count)` returns ≥5 distinct routes within ~5 min of normal traffic
  - `go test -race ./...` clean
  - `golangci-lint run ./...` clean
