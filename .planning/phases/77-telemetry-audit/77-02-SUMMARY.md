---
phase: 77-telemetry-audit
plan: 02
status: complete
shipped_at: 2026-04-27
requirements:
  - OBS-07
---

# Plan 77-02 Summary — Tempo Audit + perRouteSampler (OBS-07)

## What shipped

Audited current Tempo trace state via available data sources (per-route volume from Prometheus, OTEL_BSP confirmation from source, FK-orphan regression cross-referenced from 77-01 Loki audit), then implemented a per-route trace sampler that dispatches on URL-path prefix to per-group sampling ratios. Wrapped in `sdktrace.ParentBased` so cross-service trace continuity is preserved.

Direct TraceQL queries were not available in this session — the `grafana-cloud` MCP server's Tempo proxy tools were not loaded. The Tempo audit appendix in AUDIT.md uses surrogate evidence (Prometheus volume + structural arguments + 77-01 Loki cross-references); the operator can later run TraceQL directly via the Grafana UI to validate empirically post-deploy.

## Pre-merge per-route trace volume distribution (from AUDIT.md)

PromQL: `sum by (http_route) (increase(http_server_request_duration_seconds_count{service_name="peeringdb-plus"}[30m]))`

| Route | Trace count (30m) | Per machine/min | Share |
|-------|-------------------|-----------------|-------|
| `GET /healthz` | ~960 | 4.0 | ~99% |
| `GET /api/{rest...}` | ~5/2h | 0.005 | <1% |
| All other routes | 0 | 0 | 0% |

The audit confirmed the OBS-07 hypothesis precisely: Fly health-check traffic dominates trace volume (`/healthz` = ~99% of HTTP-rooted traces). User-context: production traffic is currently low (tech-demo state without a public traffic source); the proactive sampling sets up the right defaults *before* real traffic arrives.

## Per-route ratios applied (final matrix)

Source of truth: `internal/otel/provider.go` `Setup()` `Routes` map. Mirror in AUDIT.md § Recommended sampling matrix and `docs/ARCHITECTURE.md` § Sampling Matrix.

| Route prefix | Ratio | Rationale |
|--------------|-------|-----------|
| `/healthz` | 0.01 | Liveness traffic; 1% sample for failure-mode debugging without dominating Tempo. |
| `/readyz` | 0.01 | Symmetric with `/healthz`; future-proofed (Fly currently checks only `/healthz`). |
| `/grpc.health.v1.Health/` | 0.01 | gRPC health probes; same rationale. |
| `/api/` | 1.0 | pdbcompat — primary debugging surface. |
| `/rest/v1/` | 1.0 | entrest — primary debugging surface. |
| `/peeringdb.v1.` | 1.0 | ConnectRPC — primary debugging surface. |
| `/graphql` | 1.0 | Mid-volume; full sampling pending v1.19+ reassessment. |
| `/ui/` | 0.5 | Browser traffic; halved per CONTEXT.md plan-hint. |
| `/static/`, `/favicon.ico` | 0.01 | Static assets; rare debugging value. |
| (default) | `PDBPLUS_OTEL_SAMPLE_RATE` (default 1.0) | Sync worker + non-HTTP traces. |

## Test coverage summary

`internal/otel/sampler_test.go` — 11 test functions (committed RED at `aadb953`, GREEN at `48c6148`):

| Test | Locks |
|------|-------|
| `TestPerRouteSampler_HealthzDispatchedToLowRatio` | Basic /healthz → 0.0 dispatch with all-ones TraceID drops deterministically |
| `TestPerRouteSampler_APIDispatchedToFullRatio` | /api/net at 1.0 returns RecordAndSample |
| `TestPerRouteSampler_RestV1PrefixMatch` | /rest/v1/networks matches via prefix |
| `TestPerRouteSampler_ConnectRPCPrefixMatch` | /peeringdb.v1.NetworkService/Get matches dot-terminated prefix |
| `TestPerRouteSampler_LegacyHTTPTargetAttribute` | Sampler reads BOTH `url.path` (semconv v1.21+) AND `http.target` (legacy) |
| `TestPerRouteSampler_UnmatchedPathFallsBackToDefault` | Unrecognised paths use DefaultRatio |
| `TestPerRouteSampler_NoPathAttributeFallsBackToDefault` | Sync-worker spans (no HTTP attrs) use DefaultRatio |
| `TestPerRouteSampler_DescriptionFormat` | Description() includes "PerRouteSampler" + route count |
| `TestPerRouteSampler_RoutesNormalised` | "/api" and "/api/" treated equivalently |
| `TestPerRouteSampler_LongestPrefixWins` | /api/auth/login at /api/=1.0 + /api/auth/=0.0 drops correctly |
| `TestParentBased_InheritsDecisionForSampledIn` | Cross-service trace continuity invariant — sampled-in parent forces all children through RecordAndSample regardless of their own route prefix |

All 11 tests pass under `go test -race`.

## Files modified

| File | Change |
|------|--------|
| `internal/otel/sampler.go` | NEW — `NewPerRouteSampler`, `PerRouteSamplerInput`, `perRouteSampler` (with `ShouldSample` + `Description`), helper functions `normalisePrefix`, `matchesPrefix`, `pathFromAttributes`, `isAlnum`. ~150 LOC. |
| `internal/otel/sampler_test.go` | NEW — 11 test functions covering sampler dispatch, attribute fallback, normalisation, longest-prefix-wins, ParentBased inheritance. ~250 LOC. |
| `internal/otel/provider.go` | Replaced `sdktrace.WithSampler(sdktrace.TraceIDRatioBased(in.SampleRate))` with `sdktrace.WithSampler(sdktrace.ParentBased(NewPerRouteSampler(...)))` containing the verbatim AUDIT.md sampling matrix. Anchor comment links AUDIT.md and provider.go. |
| `docs/ARCHITECTURE.md` | Added § Sampling Matrix subsection under § OpenTelemetry instrumentation. Updated TracerProvider bullet to reference ParentBased + perRouteSampler chain. |
| `.planning/phases/77-telemetry-audit/AUDIT.md` | Appended § Tempo Trace Audit (OBS-07) section with per-route volume, OTEL_BSP confirmation, FK-orphan regression cross-reference, and the finalised sampling matrix Task 2 implements verbatim. |

## Deviations from the AUDIT.md matrix

None. The `provider.go` ratio map matches AUDIT.md § Recommended sampling matrix verbatim. The anchor comment in `provider.go` documents the cross-reference for future maintainers.

## Documentation drift finding (non-blocking)

`internal/otel/provider.go:54` comment states `OTEL_BSP_SCHEDULE_DELAY` and `OTEL_BSP_MAX_EXPORT_BATCH_SIZE` are "tuneable via" env vars, but the explicit `WithBatchTimeout(5*time.Second)` and `WithMaxExportBatchSize(512)` options override env defaults — the values are effectively hardcoded. Recommended Phase 78+ follow-up: either drop the env-var comment or implement env-var override. The values are correct as-is, so this is a documentation bug, not an operational concern.

## Post-merge expected behaviour

| Signal | Pre-merge | Post-merge expected |
|--------|-----------|---------------------|
| `/healthz` Tempo traces | 100% (~960/30m fleet) | 1% (~10/30m fleet) |
| `/readyz` Tempo traces | 100% of (currently zero) traffic | 1% (future-proof) |
| `/api/` + `/rest/v1/` + `/peeringdb.v1.` | 100% | 100% (unchanged) |
| `/ui/` (when traffic arrives) | 100% | 50% |
| `/static/` (when traffic arrives) | 100% | 1% |
| Sync-worker + internal traces | 100% (DefaultRatio=1.0) | 100% (unchanged) |
| Cross-service trace continuity | n/a (no cross-service paths today) | Preserved by ParentBased — sampled-in parent forces all children through |

## Verification gates

| Gate | Result |
|------|--------|
| `go test -race ./...` | All 31 packages PASS |
| `go test -race ./internal/otel/...` | 11/11 sampler tests + existing logger tests PASS |
| `golangci-lint run ./internal/otel/...` | 0 issues |
| `go generate ./...` | Zero drift in `ent/`, `gen/`, `graph/`, `internal/web/templates/` |
| `grep -F "ParentBased" internal/otel/provider.go` | ✓ |
| `grep -F "NewPerRouteSampler" internal/otel/provider.go` | ✓ |
| `grep -F "/healthz" internal/otel/provider.go` | ✓ |
| `grep -F "/api/" internal/otel/provider.go` | ✓ |
| `grep -F "/peeringdb.v1." internal/otel/provider.go` | ✓ |
| `grep -c 'sdktrace.TraceIDRatioBased(in.SampleRate)' internal/otel/provider.go` | 0 (old bare sampler removed) |
| `grep -F "Sampling Matrix" docs/ARCHITECTURE.md` | ✓ |
| `grep -F "ParentBased" docs/ARCHITECTURE.md` | ✓ |

## Empirical post-deploy validation (deferred to operator)

The plan's "Empirical Tempo inspection of normal-traffic traces shows max per-trace size <2 MB" success criterion requires direct TraceQL access. The Grafana MCP server in this session did not expose Tempo query tools; the audit appendix uses structural argument (Phase 68 fixed the only known regression mode; 77-01 confirmed it's still off) instead. Operator can validate empirically via the Grafana UI post-deploy:

- TraceQL: `{ resource.service.name = "peeringdb-plus" } | duration > 1s` — top-50 sorted by span count desc; expect max <2 MB.
- TraceQL: `{ resource.service.name = "peeringdb-plus" && span.name = "sync-incremental" } | spancount > 500` — must return zero traces (FK-orphan regression mode absent).

Record findings in 77-VERIFICATION.md when verified.
