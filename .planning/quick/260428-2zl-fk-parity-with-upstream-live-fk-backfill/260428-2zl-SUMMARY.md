---
phase: 260428-2zl
plan: 01
subsystem: sync, peeringdb
tags: [fk-parity, rate-limiting, waf-detection, soft-delete, incremental-sync]
requires:
  - 205ee50  # base commit (post pdbcompat default-depth fix)
provides:
  - peeringdb.WithRPS option
  - peeringdb.IsWAFBlocked helper
  - peeringdb.FetchRaw method
  - sync.Worker.fkBackfillParent (live FK backfill)
  - sync.Worker.fkBackfillTried per-cycle dedup cache
  - PDBPLUS_PEERINGDB_RPS env var
  - PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE env var
  - pdbplus.peeringdb.requests counter
  - pdbplus.peeringdb.retries counter
  - pdbplus.peeringdb.rate_limit_wait_ms histogram
  - pdbplus.sync.fk_backfill counter
affects:
  - 575+/day FK orphan drop rate (carrier 277→278 mismatch et al.)
  - sync cycle wall time (FK backfill adds ≤200 HTTP fetches per cycle)
  - dashboards: new gauges/counters require panel additions
tech-stack:
  added:
    - http.RoundTripper wrapper for transport-level concerns
  patterns:
    - sentinel-error chain via errors.Is for WAF detection
    - per-cycle dedup cache + cap pattern for backfill rate-limiting
key-files:
  created:
    - internal/peeringdb/transport.go
    - internal/peeringdb/transport_test.go
    - internal/sync/fk_backfill.go
    - internal/sync/fk_backfill_test.go
    - internal/sync/bootstrap_test.go
    - internal/sync/netixlan_sidefk_test.go
  modified:
    - internal/peeringdb/client.go
    - internal/peeringdb/client_test.go
    - internal/sync/worker.go
    - internal/sync/worker_test.go
    - internal/sync/integration_test.go
    - internal/sync/upsert.go
    - internal/visbaseline/capture_test.go
    - internal/otel/metrics.go
    - internal/config/config.go
    - cmd/peeringdb-plus/main.go
    - docs/CONFIGURATION.md
  removed:
    - internal/sync/delete.go (entire file — markStaleDeleted family + json helper)
decisions:
  - "Bootstrap with time.Unix(1,0) (since=1), not since=0 — upstream rest.py:694-727 treats N>0 as the deleted-rows-included path"
  - "Backfill HTTP per-cycle cap default 200 (operator-tunable) — bounds DoS surface while preserving full recovery for typical churn"
  - "Backfill metrics include both child_type AND parent_type axes for grep symmetry with pdbplus.sync.type.orphans"
  - "errWAFBlocked via errors.Is sentinel rather than typed struct — transport-level concern, no need for structured fields beyond the error message"
  - "isRetryable() drops 429 — transport handles 429 with bounded retry-after, including 429 here would double-bounce"
  - "SetRateLimit rewires the transport's limiter pointer too — without this, tests setting rate.Inf would still see the limiter the transport captured at NewClient"
  - "upsertSingleRaw reuses bulk upsert closures (one-element slice) — keeps validators/fold setters/nullable wiring centralised"
  - "Side-FK SET_NULL handled in nullSideFK helper, not by extending fkCheckParent — cleaner separation between drop-on-miss and null-on-miss semantics"
  - "Removed redundant ix_id check on NetworkIxLan — ix_id is upstream serializer-computed from ixlan.ix_id, not a real FK"
metrics:
  duration: "~40 minutes"
  completed_date: "2026-04-28"
---

# Quick Task 260428-2zl: fk-parity-with-upstream-live-fk-backfill Summary

Bring sync's FK enforcement to parity with upstream PeeringDB by bootstrapping
incremental sync with `?since=1`, live-backfilling missing parent rows from
upstream, removing the inference-by-absence soft-delete path, honoring upstream
SET_NULL on NetworkIxLan side FKs, removing the redundant ix_id check, and
hardening the HTTP client with configurable RPS, robust 429 handling, and WAF
detection.

## Context

Production observation 2026-04-26: `/api/carrier mirror=277 upstream=278`,
plus 575+/day orphan drops across child types. Root cause: PeeringDB's bulk
`/api/<type>` filters to `status='ok'` only (rest.py:694-727), so every
status='deleted' upstream row that a child still references gets dropped
locally as if the parent never existed.

## Commits (in execution order)

| # | Hash | Message |
|---|------|---------|
| T1 | `5e667f1` | feat(peeringdb): configurable RPS, bounded 429 retry-after, WAF detection, telemetry |
| T2 | `f57a4e2` | fix(sync): bootstrap incremental sync with ?since=1 to capture deleted rows |
| T3 | `8a3780c` | feat(sync): live FK backfill — fetch missing parents from upstream before drop |
| T4 | `94ffea9` | fix(sync): null-on-miss for NetworkIxLan side FKs (mirrors upstream SET_NULL) |
| T5 | `af5bce3` | refactor(sync): drop redundant NetworkIxLan.ix_id FK check (serializer-computed, not real FK) |
| T6 | `bb7e6f7` | refactor(sync): remove inference-by-absence soft-delete (upstream sends explicit deletes via ?since=N) |
| T7 | `1a0a294` | docs(config): document PDBPLUS_PEERINGDB_RPS and PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE |
| —  | `cb4478a` | test(visbaseline): drop log assertion absorbed by transport-level 429 retry (deviation follow-up) |

## Test Count Delta

New test functions added:

- **internal/peeringdb/transport_test.go** (T1): 11 new tests
  - TestTransport_429NumericRetryAfter, TestTransport_429HTTPDateRetryAfter,
    TestTransport_429RetryAfterTooLong_ShortCircuits,
    TestTransport_429MaxRetries_Exhausts, TestTransport_WAF403_NoRetry,
    TestTransport_NormalAuthError403_NoRetry,
    TestTransport_RateLimitSequencing, TestTransport_TelemetryFires,
    TestTransport_BodyRestoredAfterWAFSniff,
    TestTransport_RoundTripContextCancellation, TestClassifyStatus,
    TestParseRetryAfter_Float
- **internal/peeringdb/client_test.go** (T1): 2 new + 1 renamed test
  - TestWithRPS_OverridesDefault, TestWithAPIKey_OverridesWithRPS;
    TestUnauthenticatedRateLimit updated for new default
- **internal/sync/bootstrap_test.go** (T2): 2 new tests
  - TestSync_IncrementalBootstrapUsesSince1, TestSync_FullModeStillBare
- **internal/sync/fk_backfill_test.go** (T3): 5 new tests
  - TestFKCheckParent_BackfillIntegration, TestFKCheckParent_BackfillDedup,
    TestFKCheckParent_BackfillCapZeroDisablesBackfill,
    TestFKCheckParent_BackfillCapHitRecordsRatelimited,
    TestFKCheckParent_BackfillFetchErrorRecordsError,
    TestFetchRaw_PassesQueryParams
- **internal/sync/netixlan_sidefk_test.go** (T4): 1 new test
  - TestFKFilter_NetworkIxLan_NullsSideFKOnMiss
- **internal/sync/worker_test.go** (T2/T6): 1 renamed (TestIncrementalFirstSyncFull → TestIncrementalFirstSyncBootstrap, semantics inverted), 1 renamed (TestSyncSoftDeletesStale → TestSyncPersistsExplicitTombstone, payload shape rewritten)
- **internal/sync/integration_test.go** (T6): 3 renamed + rewritten
  - TestSyncDeletesStaleRecords → TestSyncTombstonesExplicitDeletedRecords
  - TestSync_SoftDeleteMarksRows → TestSync_TombstonePersistedFromExplicitPayload
  - TestSyncDeletesFKIntegrity → TestSyncFKIntegrity_AfterTombstoneCycle

Total: ~22 new test functions; 6 existing tests updated for the new contract.

## Grep Gate Verification

```
no markStaleDeleted/syncDeletePass in non-tests:    PASS (0 matches)
fkBackfillParent in worker.go:                       PASS (≥1 match)
fkBackfillTried decl+reset:                          PASS (≥2 matches)
time.Unix(1,0) bootstrap:                            PASS (≥1 match)
AWS WAF (non-comment) in transport.go:               PASS (≥1 match)
redundant ix_id check removed:                       PASS (0 matches)
side-FK null handling present (NetSideID|IXSideID):  PASS (≥4 matches)
PDBPLUS_PEERINGDB_RPS in docs:                       PASS (≥1 match)
PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE in docs:           PASS (≥1 match)
```

## Build / Test / Lint Status

- `go generate ./...`: zero codegen drift
- `go build ./...`: OK
- `go vet ./...`: OK
- `go test -race ./...`: all packages pass (cmd, ent/schema, graph, deploy/grafana, all internal/*)
- `golangci-lint run ./...`: 0 issues
- `govulncheck ./...`: No vulnerabilities found
- `TestWorkerSync_LineBudget`: 96/100 lines (under budget after T6 trimmed 4 lines)

## pdbcompat Invariants Preserved

- `applyStatusMatrix` still appears in 13 closures in `internal/pdbcompat/registry_funcs.go`
- `StatusIn` literals still present at 26 PK-lookup call sites in `internal/pdbcompat/depth.go`
- `internal/privfield.Redact` still called at all 5 surfaces (pdbcompat, ConnectRPC, GraphQL, entrest, web UI — none touched)
- Phase 71 response memory budget — untouched
- ent schema (including `ent/schema/networkixlan.go`) — untouched

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Visbaseline test asserted log line that no longer fires**
- **Found during:** Final test sweep after T6
- **Issue:** `TestCaptureRespectsRateLimit` asserted that `slog` output contains "rate-limited". Pre-2zl, doWithRetry surfaced *RateLimitError on every 429, which the visbaseline outer-retry path caught and logged. Post-T1, the transport absorbs 429 with Retry-After ≤ 60s automatically — the log line never fires for cap-fitting Retry-After values like the test's "1".
- **Fix:** Updated the test to keep the load-bearing two-hit assertion (transport DID retry) but removed the soft log assertion, with a comment explaining why.
- **Files modified:** `internal/visbaseline/capture_test.go`
- **Commit:** `cb4478a`

**2. [Rule 2 - Critical functionality] Telemetry instruments must be nil-guarded for tests**
- **Found during:** First T1 test run
- **Issue:** Transport called `pdbotel.PeeringDBRequests.Add(...)` directly. Tests run without `InitMetrics()`, so the package var is nil → SDK panic.
- **Fix:** Added `recordRequest`/`recordRetry` helper functions in transport.go and `recordBackfill` in fk_backfill.go, all of which nil-guard the instrument before calling. Same pattern was already established for `PeeringDBRateLimitWaitMS` and other counters.
- **Files modified:** `internal/peeringdb/transport.go`, `internal/sync/fk_backfill.go`, `internal/peeringdb/client.go`
- **Commit:** Folded into `5e667f1` (T1) and `8a3780c` (T3)

**3. [Rule 1 - Bug] Worker.Sync line budget regressed 1 line**
- **Found during:** T6 first build
- **Issue:** Added 5-line "Phase B" Task 6 explanatory comment pushed Sync from 100 to 101 lines, violating the REFAC-03 budget enforced by TestWorkerSync_LineBudget.
- **Fix:** Trimmed the comment to one inline annotation.
- **Files modified:** `internal/sync/worker.go`
- **Commit:** Folded into `bb7e6f7` (T6) — final budget 96/100 (under).

### Deferred Items

None — all critical functionality implemented per plan.

## CLAUDE.md Staleness Note

The "Soft-delete tombstones" section in CLAUDE.md describes the
removed markStaleDeleted machinery (cycleStart plumbing,
deleteStaleChunked, etc.). Operator follow-up via
`/claude-md-management:revise-claude-md` to update the docs to
reflect the post-T6 reality:

- markStaleDeleted family is gone
- syncDeletePass is gone
- Tombstones now arrive as explicit `status='deleted'` rows in
  upstream `?since=N` payloads (rest.py:694-727)
- cycleStart is no longer used as a tombstone timestamp (the upstream
  payload's "updated" field is)
- Bootstrap-with-?since=1 is the new incremental cycle-1 default

The "Environment Variables" section also needs PDBPLUS_PEERINGDB_RPS
and PDBPLUS_FK_BACKFILL_MAX_PER_CYCLE rows added.

## Operator Post-Deploy Checklist

After deploy, the operator should verify:

1. **One sync cycle** (~15min after deploy on the primary).

2. **Compare against upstream**:
   ```bash
   ./scripts/compare-upstream-fields.sh /api/carrier
   ```
   Expected: `mirror=278 upstream=278` (the carrier 277→278 mismatch
   that motivated this work should resolve on the first cycle).

3. **Grafana panels to add / watch**:
   - `pdbplus_sync_fk_backfill_total{result="hit"}` — non-zero on
     first cycle, decays to ~0 in steady state.
   - `pdbplus_sync_fk_backfill_total{result="ratelimited"}` — should
     be 0 in steady state. Sustained non-zero means the cap (default
     200) is too tight; investigate before raising.
   - `pdbplus_peeringdb_requests_total{status_class="429"}` — should
     be near-zero with the bumped RPS default (2.0).
   - `pdbplus_peeringdb_rate_limit_wait_ms` p99 — bounded under 1s
     for unauthenticated, under 100ms for authenticated.

4. **Loki**: zero `429-retry exhaustion` warnings (transport's bounded
   retry should absorb everything within the cap; cap-exceeded
   Retry-After is the unauth 1/hr case which sync handles by deferring
   to the next scheduled tick).

5. **CSP / WAF detection**: if any `WAF block detected on 403`
   warnings appear in Loki, investigate immediately — Cloudflare or
   AWS WAF has IP-blocked the egress address.

## Self-Check: PASSED

All 14 files referenced in this summary exist:
- 6 created files: `transport.go`, `transport_test.go`, `fk_backfill.go`, `fk_backfill_test.go`, `bootstrap_test.go`, `netixlan_sidefk_test.go`
- 11 modified files: confirmed via `git log` and `git show`
- 1 removed file: `internal/sync/delete.go` (absent from filesystem post-T6)

All 8 commit hashes verified present in `git log`:
- 5e667f1, f57a4e2, 8a3780c, 94ffea9, af5bce3, bb7e6f7, 1a0a294, cb4478a
