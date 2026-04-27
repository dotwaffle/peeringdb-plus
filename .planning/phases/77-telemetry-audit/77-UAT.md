---
status: deploy-pending
phase: 77-telemetry-audit
source:
  - 77-01-SUMMARY.md
  - 77-02-SUMMARY.md
started: 2026-04-27T04:00:16Z
updated: 2026-04-27T04:00:16Z
---

## Current Test

number: 7
name: Loki ingestion volume measurably down post-deploy
expected: |
  After `fly deploy` of the Phase 77 commit chain, repeat the 30-min
  Loki sampling that produced the AUDIT.md baseline. Per the OBS-06
  fix expectation (77-01-SUMMARY.md):
  - DEBUG record count: drops to ~zero (was 52/30min/8 machines).
  - INFO record count on primary: ~85% reduction (per-type per-cycle
    `fetching` / `upserted` / `marked stale deleted` records demoted).
  - WARN volume unchanged at ~1/30min (only the `fk orphans summary`
    aggregate fires).
awaiting: operator deploy + Grafana sampling

## Tests

### 1. AUDIT.md reclassification table is present and complete
status: PASS (auto-verified at commit time)
evidence:
  - Acceptance gate `grep -cE '^\| (internal|cmd)/.+\.go:[0-9]+ \|' .planning/phases/77-telemetry-audit/AUDIT.md` returns 47 (≥8 required).
  - Acceptance gate `grep -E '\| Yes' .planning/phases/77-telemetry-audit/AUDIT.md | grep -cv 'KEEP'` returns 0 (zero security-signal rows demoted).
  - Acceptance gate `grep -F "fk orphan" AUDIT.md` succeeds.
  - Commit: 0d9ad2f
recorded: 2026-04-27

### 2. 9 inline slog level changes applied per AUDIT.md
status: PASS (auto-verified at commit time + locked by RED→GREEN test cycle)
evidence:
  - Acceptance gates from 77-01-PLAN.md Task 2 all pass:
    - `grep -c 'slog\.LevelInfo, "fetching"' internal/sync/worker.go` = 0
    - `grep -c 'slog\.LevelInfo, "upserted"' internal/sync/worker.go` = 0
    - `grep -c 'slog\.LevelWarn, "heap threshold crossed"' internal/sync/worker.go` = 1 (security signal preserved)
    - `grep -c 'slog\.LevelWarn, "fk orphans summary"' internal/sync/worker.go` = 1 (security signal preserved)
    - `grep -c 'slog\.LevelWarn, "incremental sync failed' internal/sync/worker.go` = 1 (operator-actionable signal preserved)
    - `grep -c 'slog\.LevelWarn, "demoted during sync' internal/sync/worker.go` = 1 (LiteFS-demotion signal preserved)
    - `grep -c 'slog\.LevelError, "sync failed after all retries' internal/sync/worker.go` = 1 (final-failure signal preserved)
  - RED tests at 4167213 fail on pre-audit source; GREEN at 5fe9749 turns them green.
  - `go test -race ./internal/sync/... ./internal/health/...` exits 0.
recorded: 2026-04-27

### 3. otelslog handler env-filtered (PDBPLUS_LOG_LEVEL)
status: PASS (auto-verified at commit time)
evidence:
  - `internal/otel/logger.go` constructs `levelFilterHandler{inner: otelHandler, level: otelLevelFromEnv()}` and inserts it into the fanoutHandler in place of the bare otelHandler.
  - 5 unit tests in `internal/otel/logger_levelfilter_test.go` (RED at 4167213, GREEN at 70687a2):
    - `TestLevelFilterHandler_DefaultINFOBlocksDebug`
    - `TestLevelFilterHandler_DEBUGAdmitsDebug`
    - `TestNewDualLogger_DefaultBlocksDebugFromOTelBranch`
    - `TestOTelLevelFromEnv_ParsesValid` (6 sub-cases: unset, debug_lower, DEBUG_upper, info, warn, error)
    - `TestOTelLevelFromEnv_FallsBackOnGarbage`
  - PDBPLUS_LOG_LEVEL documented in `docs/CONFIGURATION.md` and `CLAUDE.md` § Environment Variables (commit c1d86f4).
recorded: 2026-04-27

### 4. perRouteSampler implemented + wired via ParentBased
status: PASS (auto-verified at commit time)
evidence:
  - Acceptance gates from 77-02-PLAN.md Task 2 all pass:
    - `test -f internal/otel/sampler.go` ✓
    - `test -f internal/otel/sampler_test.go` ✓
    - `grep -c 'func.*ShouldSample' internal/otel/sampler.go` ≥ 1
    - `grep -c 'func.*Description' internal/otel/sampler.go` ≥ 1
    - `grep -F "ParentBased" internal/otel/provider.go` ✓
    - `grep -F "NewPerRouteSampler" internal/otel/provider.go` ✓
    - `grep -F "/healthz" internal/otel/provider.go` ✓
    - `grep -F "/api/" internal/otel/provider.go` ✓
    - `grep -F "/peeringdb.v1." internal/otel/provider.go` ✓
    - `grep -c 'sdktrace.TraceIDRatioBased(in.SampleRate)' internal/otel/provider.go` = 0 (old bare sampler removed)
  - 11 unit tests in `internal/otel/sampler_test.go` (RED at aadb953, GREEN at 48c6148) cover per-route dispatch, legacy attribute fallback, longest-prefix-wins, normalisation, and the ParentBased inheritance invariant.
recorded: 2026-04-27

### 5. Sampling Matrix documented in ARCHITECTURE.md
status: PASS (auto-verified at commit time)
evidence:
  - Acceptance gates pass:
    - `grep -F "Sampling Matrix" docs/ARCHITECTURE.md` ✓
    - `grep -F "ParentBased" docs/ARCHITECTURE.md` ✓
  - Commit 75ca1d0 added a new § Sampling Matrix subsection under § OpenTelemetry instrumentation with the per-route ratios table, ParentBased rationale, longest-prefix-wins boundary rule, and OTEL_BSP baseline reference.
recorded: 2026-04-27

### 6. Repo-wide gates clean
status: PASS (auto-verified at commit time)
evidence:
  - `go test -race ./...` — all 31 packages PASS.
  - `golangci-lint run ./internal/sync/... ./internal/health/... ./internal/otel/... ./internal/middleware/... ./cmd/peeringdb-plus/...` — 0 issues.
  - `go generate ./...` — zero drift in `ent/`, `gen/`, `graph/`, `internal/web/templates/`.
recorded: 2026-04-27

### 7. Loki ingestion volume measurably down post-deploy
status: DEPLOY-PENDING
expected: |
  After `fly deploy` of the Phase 77 commit chain (b52edfc..7759323),
  repeat the 30-min Loki sampling that produced the AUDIT.md baseline
  (52 DEBUG / 65 INFO / 1 WARN / 30 min / 8 machines). Expected:
  - DEBUG record count drops to ~zero (otelslog handler now filters at
    PDBPLUS_LOG_LEVEL=INFO default).
  - INFO record count on primary drops ~85% (per-type per-cycle records
    `fetching`, `upserted`, `marked stale deleted` demoted to DEBUG and
    therefore filtered out).
  - WARN unchanged at ~1/30min (the `fk orphans summary` aggregate).
queries:
  - 'sum by (severity_text) (count_over_time({service_name="peeringdb-plus"} | severity_text != "" [30m]))'
  - 'topk(15, sum by (severity_text, line) (count_over_time({service_name="peeringdb-plus"} [30m])))'
recorded_when_run: TBD

### 8. Tempo /healthz trace volume drops to ~1% post-deploy
status: DEPLOY-PENDING
expected: |
  After `fly deploy`, repeat the 30-min per-route trace volume query.
  Expected post-deploy:
  - /healthz traces drop from ~960/30min to ~10/30min (1% sample).
  - /api/* and /rest/v1/* unchanged at full sampling (zero current
    traffic, but ratio is correct).
  - Cross-service trace continuity preserved — when /api/* eventually
    receives traffic that calls into anything else, the parent span's
    sampled-in decision should propagate via traceparent.
queries:
  - 'sum by (http_route) (increase(http_server_request_duration_seconds_count{service_name="peeringdb-plus"}[30m]))'
recorded_when_run: TBD

### 9. Empirical max-per-trace size <2 MB
status: DEPLOY-PENDING
expected: |
  Cannot be queried via the grafana-cloud MCP in the current session
  (Tempo proxy tools not loaded). Operator should validate via
  Grafana UI post-deploy. Expected: max per-trace size <2 MB during
  normal traffic; the FK-orphan WARN-spam regression mode (the only
  known way to breach 7.5 MB) is structurally absent (Phase 68 fix +
  77-01 Loki audit empirical confirmation).
queries:
  - 'TraceQL: { resource.service.name = "peeringdb-plus" } | duration > 1s — top 50 sorted by span count desc'
  - 'TraceQL: { resource.service.name = "peeringdb-plus" && span.name = "sync-incremental" } | spancount > 500 — must return zero traces'
recorded_when_run: TBD

## Summary

| Test | Status |
|------|--------|
| 1. AUDIT.md reclassification table | PASS |
| 2. 9 inline slog level changes | PASS |
| 3. otelslog env-filtered | PASS |
| 4. perRouteSampler + ParentBased | PASS |
| 5. Sampling Matrix in ARCHITECTURE.md | PASS |
| 6. Repo gates (test + lint + drift) | PASS |
| 7. Loki volume reduction | DEPLOY-PENDING |
| 8. Tempo /healthz volume reduction | DEPLOY-PENDING |
| 9. Max per-trace size <2 MB | DEPLOY-PENDING |

6 of 9 success criteria auto-verified at commit time. The remaining 3 require post-deploy empirical observation against the live Grafana stack — they cannot be tested inline. After `fly deploy`, run the queries listed under each test and update this UAT.md with the observed values + recorded timestamp.

## Notes

- Phase 77 is backend observability work. The conversational one-test-at-a-time UAT pattern is a poor fit because there are no user-clickable surfaces to interactively confirm — the deliverables are slog levels, sampler decisions, and OTel pipeline filters that surface only post-deploy via Grafana inspection.
- Documentation drift finding deferred to Phase 78+: `internal/otel/provider.go:54` comment claims `OTEL_BSP_*` env vars are "tuneable" but explicit `WithBatchTimeout` / `WithMaxExportBatchSize` options override env defaults. Non-blocking.
- All commits 0d9ad2f..7759323 are clean of PII (no Grafana stack URL, no email).
