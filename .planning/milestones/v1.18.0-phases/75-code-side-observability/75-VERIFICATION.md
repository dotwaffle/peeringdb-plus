---
phase: 75-code-side-observability
verified: 2026-04-27T00:15:00Z
status: human_needed
score: 4/4 must-haves verified (codebase contract); 3/4 success criteria require operator deploy-time confirmation
overrides_applied: 0
human_verification:
  - test: "Post-deploy: query `count by(type)(pdbplus_data_type_count{service_name=\"peeringdb-plus\"})` in Grafana Cloud Prometheus within 30s of new machine boot"
    expected: "≥13 distinct `type` labels with non-zero values for entities that have rows in production (some entities like carrier/carrierfac/campus may have small but non-zero counts)"
    why_human: "OTel exporter scrape interval and Grafana Cloud receiver behaviour are external runtime services not exercised by unit/E2E tests — only observable via real production telemetry"
  - test: "Post-deploy: query the 5 zero-rate counters (`pdbplus_sync_type_fallback_total`, `pdbplus_sync_type_fetch_errors_total`, `pdbplus_sync_type_upsert_errors_total`, `pdbplus_sync_type_deleted_total`, `pdbplus_role_transitions_total`) in Grafana Cloud"
    expected: "13 type labels (or 2 direction labels for role_transitions) per metric, all reading 0 cluster-wide; dashboard panels render `0` rather than `No data`"
    why_human: "Same — exporter + Grafana Cloud receiver behaviour required to confirm baseline series are exported and visible"
  - test: "Post-deploy: generate ~5 minutes of varied traffic (curl loop) and run `count by(http_route)(http_server_request_duration_seconds_count{service_name=\"peeringdb-plus\"})`"
    expected: "≥5 distinct http_route labels covering /api/{rest...}, /rest/v1/, /graphql, /ui/{rest...}, and GET /healthz"
    why_human: "Investigation (OBS-04-INVESTIGATION.md) refuted all 3 code-bug hypotheses; root cause is sparse-traffic / Prometheus-staleness artifact. Must verify post-deploy that traffic-driven labels appear; if /healthz remains the only series after curl loop, follow-up investigation in OTLP exporter / Grafana Cloud receiver paths is needed (out of OBS-04 scope per CONTEXT.md)"
  - test: "Post-deploy: open the Grafana 'Request Rate by Route' panel"
    expected: "Multi-line breakdown with at least 5 route families instead of single /healthz line"
    why_human: "Visual dashboard verification — requires real production traffic + render"
---

# Phase 75: Code-side Observability Fixes — Verification Report

**Phase Goal:** Three code-side changes to fix observability gaps surfaced in the 2026-04-26 telemetry audit: cold-start gauge population (OBS-01), zero-rate counter pre-warming (OBS-02), and `http.route` middleware investigation + fix (OBS-04).

**Verified:** 2026-04-27T00:15:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (from ROADMAP success_criteria + PLAN frontmatter must_haves)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | (SC1/OBS-01) Within 30s of process startup, `pdbplus_data_type_count` reports correct row counts for all 13 PeeringDB types | VERIFIED (codebase contract) | `internal/sync/initialcounts.go` exports `InitialObjectCounts(ctx, *ent.Client) (map[string]int64, error)` calling `Count(ctx)` on each of the 13 entity tables; `cmd/peeringdb-plus/main.go:199-204` wires it BEFORE `InitObjectCountGauges` at line 210; 3 unit tests pass (all-13-non-zero on seed, all-13-zero on empty DB, key-parity). Deploy-time observation requires HUMAN verification per item 1 in human_verification |
| 2 | (SC2/OBS-02) After a fresh deploy on a healthy fleet, the 5 zero-rate counters render `0` instead of `No data` — every type tuple pre-warmed | VERIFIED (codebase contract) | `internal/otel/prewarm.go` exports `PrewarmCounters(ctx)` emitting `.Add(ctx, 0, ...)` for 4 per-type counters × 13 types + 1 direction counter × 2 directions = 54 baseline series; wired at `cmd/peeringdb-plus/main.go:293` AFTER `InitMetrics()` (line 96) and AFTER syncWorker construction (line 260) but BEFORE `StartScheduler` goroutine (line 297); 3 unit tests pass. Deploy-time observation requires HUMAN verification per item 2 |
| 3 | (SC3/OBS-04) During normal traffic, `count by(http_route)(http_server_request_duration_seconds_count)` returns ≥5 distinct route patterns | VERIFIED (test contract) | `cmd/peeringdb-plus/route_tag_e2e_test.go` `TestRouteTag_E2E_AllRouteFamilies` exercises 5 route families through production-shaped chain (otelhttp → privacyTierLikeMW → routeTagMiddleware → mux); all 5 sub-tests PASS asserting http.route label populates correctly. Per OBS-04-INVESTIGATION.md, all 3 code-bug hypotheses were refuted empirically; root cause was sparse-traffic / Prometheus-staleness artifact. The fix shape was "investigation + doc clarification + E2E test lock" — the contract is locked via test, NOT via reshaping production traffic. Production confirmation requires HUMAN verification per item 3 |
| 4 | (SC4/OBS-04 docs) The root cause for the prior single-route series is documented | VERIFIED | `OBS-04-INVESTIGATION.md` (207 lines) contains all 6 mandatory H2 sections: TL;DR, Empirical Evidence, Root Cause, Hypotheses Ruled Out, Chosen Fix, Acceptance. All 4 hypotheses ruled-out-or-confirmed with specific evidence. Per-family table covers 6 route families. otelhttp v0.68.0 source-line citations present (handler.go:172/178/202, semconv/server.go:367-368, labeler.go:44-46) |
| 5 | (PLAN-75-01) Test asserts new helper populates atomic cache with all 13 types from seeded ent client | VERIFIED | `TestInitialObjectCounts_AllThirteenTypes` in `internal/sync/initialcounts_test.go` uses `seed.Full(t, client)` and asserts `len(counts) == 13` with all values non-zero |
| 6 | (PLAN-75-01) Sync-completion path (OnSyncComplete) is unchanged | VERIFIED | `cmd/peeringdb-plus/main.go:268` still calls `objectCountCache.Store(&m)` after sync completion (verified via grep) |
| 7 | (PLAN-75-02) RoleTransitions special-cased with direction= attribute (not type=) | VERIFIED | `internal/otel/prewarm.go:63-65` loops over `{"promoted", "demoted"}` with `attribute.String("direction", d)` |
| 8 | (PLAN-75-03) E2E test exercises ≥4 route families through production-shaped chain | VERIFIED | `TestRouteTag_E2E_AllRouteFamilies` runs 5 sub-tests (healthz_works, api_family, rest_v1_family, graphql, ui_family); all PASS via captureLabelerMW → privacyTierLikeMW → routeTagMiddleware → mux chain |
| 9 | (PLAN-75-03) Existing TestRouteTagMiddleware + TestMiddlewareChain_Order regression guards stay green | VERIFIED | Both tests pass under `-race` together with new E2E tests |
| 10 | (PLAN-75-03) Fix is minimally-invasive — no new instrumentation library, no body change to routeTagMiddleware | VERIFIED | Per 75-03-SUMMARY: "+23/-4 line delta on routeTagMiddleware doc-comment expansion only — body unchanged"; `git log` shows no go.mod changes adding HTTP instrumentation libraries |

**Score:** 10/10 codebase-contract truths verified. SC1, SC2, SC3 deployment-time observations require HUMAN verification.

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/sync/initialcounts.go` | `InitialObjectCounts(ctx, *ent.Client)` runs Count on 13 entity tables | VERIFIED | 86 lines; exports `InitialObjectCounts`; uses closure-table over 13 named entities (org, campus, fac, carrier, carrierfac, ix, ixlan, ixpfx, ixfac, net, poc, netfac, netixlan); errors wrapped with `fmt.Errorf("count %s: %w", q.name, err)` |
| `internal/sync/initialcounts_test.go` | 3 unit tests using seed.Full | VERIFIED | 90 lines; 3 tests: `_AllThirteenTypes`, `_EmptyDB`, `_KeyParityWithSyncSteps`; all `t.Parallel()`-safe |
| `internal/otel/prewarm.go` | `PrewarmCounters(ctx)` + `PeeringDBEntityTypes` | VERIFIED | 66 lines; exports both symbols; calls `.Add(ctx, 0, ...)` on 4 per-type counters (SyncTypeFallback, SyncTypeFetchErrors, SyncTypeUpsertErrors, SyncTypeDeleted) × 13 types + RoleTransitions × 2 directions; deliberately omits SyncTypeObjects/SyncDuration/SyncOperations/SyncTypeOrphans (verified by `grep -cE` returning 0) |
| `internal/otel/prewarm_test.go` | Test PrewarmCounters + cardinality | VERIFIED | 54 lines; 3 tests; cardinality-13 + set-equality + parity-note |
| `cmd/peeringdb-plus/route_tag_e2e_test.go` | E2E test asserting http.route for ≥4 families | VERIFIED | 213 lines; 3 test functions including `TestRouteTag_E2E_AllRouteFamilies` (5 sub-tests), `TestRouteTag_E2E_HealthzStillWorks`, `TestRouteTag_E2E_UnmatchedOmitsLabel`; uses `captureLabelerMW` from existing `route_tag_test.go` |
| `.planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md` | Root-cause + chosen-fix doc | VERIFIED | 207 lines; 6 mandatory H2 sections present; per-family evidence table (6 families); otelhttp source-line citations; 4 hypotheses ruled out with evidence |
| `cmd/peeringdb-plus/main.go` | Wire-ups for InitialObjectCounts + PrewarmCounters | VERIFIED | Line 199: `pdbsync.InitialObjectCounts(ctx, entClient)`; Line 204: `objectCountCache.Store(&seededCounts)`; Line 293: `pdbotel.PrewarmCounters(ctx)`; Line 906-934: expanded `routeTagMiddleware` doc-comment with forward-pointer to OBS-04-INVESTIGATION.md |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `cmd/peeringdb-plus/main.go` | `internal/sync/initialcounts.go` | `pdbsync.InitialObjectCounts(ctx, entClient)` | WIRED | grep returns 1 occurrence at line 199 |
| `cmd/peeringdb-plus/main.go` | `objectCountCache atomic.Pointer` | `objectCountCache.Store(&seededCounts)` | WIRED | line 204; subsequent `InitObjectCountGauges` at line 210 reads via `objectCountCache.Load()` |
| `cmd/peeringdb-plus/main.go` | `internal/otel/prewarm.go` | `pdbotel.PrewarmCounters(ctx)` | WIRED | grep returns 1 occurrence at line 293 |
| `internal/otel/prewarm.go` | `internal/otel/metrics.go` counters | direct package-private `.Add()` calls | WIRED | 5 distinct counter symbols × `.Add(ctx, 0,` invocations verified |
| `production traffic` | `Grafana http_server_request_duration_seconds_count` | `otelhttp → routeTagMiddleware → labeler.Add(http.route)` | WIRED (locked by E2E test) | E2E test asserts label populates for 5 families through production-shaped chain |
| `cmd/peeringdb-plus/main.go buildMiddlewareChain` | `TestMiddlewareChain_Order` regression guard | source-scan ordering assertion | WIRED | `routeTagMiddleware(` literal still scanned by `middleware_chain_test.go`; test passes |

**Ordering invariants verified via line numbers in main.go:**
- Line 96: `InitMetrics()` — counter vars populated
- Line 116: `database.Open` — entClient open
- Line 199: `InitialObjectCounts(ctx, entClient)` — runs AFTER entClient open, BEFORE InitObjectCountGauges
- Line 210: `InitObjectCountGauges` — registers OTel callback reading from now-seeded cache
- Line 260: `syncWorker := pdbsync.NewWorker` — sync worker constructed
- Line 268: `objectCountCache.Store(&m)` — OnSyncComplete callback (unchanged)
- Line 293: `PrewarmCounters(ctx)` — runs AFTER InitMetrics + syncWorker construction, BEFORE StartScheduler
- Line 297: `go syncWorker.StartScheduler` — scheduler goroutine spawned

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|---------------------|--------|
| `pdbplus_data_type_count` gauge | `objectCountCache.Load()` | (1) `InitialObjectCounts(ctx, entClient)` returns from real `Count(ctx)` queries on ent client; (2) `OnSyncComplete` refreshes from sync result | YES (both primers exercised) | FLOWING — startup primer runs `client.X.Query().Count(ctx)` against real LiteFS DB |
| 4 per-type zero-rate counters | OTel SDK metric stream | `PrewarmCounters` emits `.Add(ctx, 0, attr)` per (counter, type) tuple | YES (zero values are real samples that register the series) | FLOWING — first scrape interval after PrewarmCounters runs sees 54 baseline series (4×13 + 1×2) |
| `RoleTransitions` counter | OTel SDK metric stream | `.Add(ctx, 0, direction=promoted|demoted)` | YES | FLOWING — 2 baseline series with `direction` (NOT `type`) attribute matches production emission shape per worker.go:1634/1651 |
| `http_server_request_duration_seconds` `http.route` label | otelhttp `RecordMetrics` reading from `labeler.Get()` | `routeTagMiddleware` post-dispatch `labeler.Add(attribute.String("http.route", r.Pattern))` | YES (per OBS-04-INVESTIGATION § Direct metric-record verification using sdkmetric.NewManualReader) | FLOWING — empirically proven that http.route reaches metric record path through production-shaped chain for all 5 families |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| `go build ./...` clean | `TMPDIR=/tmp/claude-1000 go build ./...` | exit 0, no output | PASS |
| `go vet ./...` clean | `TMPDIR=/tmp/claude-1000 go vet ./...` | exit 0, no output | PASS |
| Targeted phase 75 tests pass under `-race` | `go test -race -run "TestInitialObjectCounts|TestPrewarmCounters|TestPeeringDBEntityTypes" ./internal/sync/... ./internal/otel/...` | both packages PASS | PASS |
| Route tag tests pass | `go test -race -run "TestRouteTagMiddleware|TestRouteTag_E2E_*|TestMiddlewareChain_Order" ./cmd/peeringdb-plus/...` | PASS (5 E2E sub-tests + healthz + unmatched + unit + chain order) | PASS |
| Full `internal/sync` test suite passes under `-race` | `go test -race -count=1 -timeout=180s ./internal/sync/...` | PASS in 9.5s | PASS |
| Full `internal/otel` test suite passes under `-race` | `go test -race -count=1 -timeout=180s ./internal/otel/...` | PASS in 1.1s | PASS |
| Full `cmd/peeringdb-plus` test suite passes under `-race` | `go test -race -count=1 -timeout=120s ./cmd/peeringdb-plus/...` | PASS in 2.6s | PASS |
| `golangci-lint` clean on changed packages | `golangci-lint run ./internal/sync/... ./internal/otel/... ./cmd/peeringdb-plus/...` | `0 issues.` | PASS |
| Probe log statement removed from main.go | `grep -c 'routetag probe' cmd/peeringdb-plus/main.go` | 0 | PASS |
| All 13 entity types appear in `InitialObjectCounts` | `grep -cE 'client\.(Organization|Campus|Facility|Carrier|CarrierFacility|InternetExchange|IxLan|IxPrefix|IxFacility|Network|Poc|NetworkFacility|NetworkIxLan)\.Query\(\)\.Count'` | 13 | PASS |
| `PeeringDBEntityTypes` cardinality is 13 | `TestPeeringDBEntityTypes_Cardinality` passes | PASS | PASS |
| Excluded counters NOT pre-warmed | `grep -cE '(SyncTypeObjects|SyncDuration|SyncOperations|SyncTypeOrphans)' internal/otel/prewarm.go` | 0 | PASS |
| 5 route families exercised in E2E | `go test -v -run TestRouteTag_E2E_AllRouteFamilies` | 5 sub-tests PASS (healthz, api_family, rest_v1_family, graphql, ui_family) | PASS |
| 6 mandatory H2 sections in investigation doc | `grep -cE '^## (TL;DR|Empirical Evidence|Root Cause|Hypotheses Ruled Out|Chosen Fix|Acceptance)'` | 6 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| OBS-01 | 75-01-PLAN | `pdbplus_data_type_count` gauge correct within 30s of startup | SATISFIED (codebase) / NEEDS HUMAN (deploy) | `InitialObjectCounts` helper + main.go wire-up at line 199 BEFORE `InitObjectCountGauges` at line 210; deploy-time PromQL verification deferred to operator |
| OBS-02 | 75-02-PLAN | Zero-rate counter panels render `0` after fresh deploy | SATISFIED (codebase) / NEEDS HUMAN (deploy) | `PrewarmCounters` 54-series baseline; main.go wire at line 293; deploy-time PromQL verification deferred to operator |
| OBS-04 | 75-03-PLAN | `http_route` populates for all routes, not just /healthz | SATISFIED (codebase contract) / NEEDS HUMAN (deploy) | OBS-04-INVESTIGATION.md refuted code-bug hypotheses; E2E test locks contract for 5 families through production-shaped chain; deploy-time PromQL verification + Grafana panel inspection deferred to operator. Per CONTEXT.md D-03 the goal was "≥5 distinct routes" — E2E test exercises exactly 5 |

**Orphaned requirements check:** None. ROADMAP.md maps OBS-01, OBS-02, OBS-04 to Phase 75 — all 3 have a corresponding plan. OBS-03/05 are Phase 76, OBS-06/07 are Phase 77 (out of scope per CONTEXT.md and roadmap mapping).

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | — | — | No anti-patterns found in changed files. No TODO/FIXME/PLACEHOLDER strings; no empty implementations; no console.log-only handlers; no hardcoded empty data flowing to render paths. The probe log line was correctly removed (grep returns 0). |

### Human Verification Required

The codebase contract is fully satisfied (10/10 truths verified). However, three of the four roadmap success criteria reference deployment-time observable behaviour (Grafana panel rendering, Prometheus series presence, multi-route http.route breakdown) that cannot be verified against an in-process unit/E2E test alone. These require operator action post-deploy:

#### 1. OBS-01 Deploy-time gauge population

**Test:** After `fly deploy`, run within 30s of new machine boot:
```promql
count by(type)(pdbplus_data_type_count{service_name="peeringdb-plus"})
```
**Expected:** ≥13 distinct `type` labels with non-zero values for entities that have rows in production.
**Why human:** OTel exporter scrape interval and Grafana Cloud receiver behaviour are external runtime services not exercised by unit/E2E tests.

#### 2. OBS-02 Deploy-time zero-rate panel rendering

**Test:** After `fly deploy`, query each of the 5 zero-rate counters in Grafana Cloud:
```promql
count by(type)(pdbplus_sync_type_fallback_total{service_name="peeringdb-plus"})
count by(type)(pdbplus_sync_type_fetch_errors_total{service_name="peeringdb-plus"})
count by(type)(pdbplus_sync_type_upsert_errors_total{service_name="peeringdb-plus"})
count by(type)(pdbplus_sync_type_deleted_total{service_name="peeringdb-plus"})
count by(direction)(pdbplus_role_transitions_total{service_name="peeringdb-plus"})
```
**Expected:** 13 type labels per per-type counter (or 2 direction labels for role_transitions); dashboard panels show `0` rather than `No data`.
**Why human:** Same — exporter + Grafana Cloud receiver behaviour required to confirm baseline series are exported and visible.

#### 3. OBS-04 Deploy-time http.route multi-family verification

**Test:** After `fly deploy`, generate ~5 minutes of varied traffic and run:
```promql
count by(http_route)(http_server_request_duration_seconds_count{service_name="peeringdb-plus"})
```
Curl loop (from OBS-04-INVESTIGATION.md § Acceptance):
```bash
for i in $(seq 1 60); do
  curl -s 'https://peeringdb-plus.fly.dev/api/networks?limit=1' > /dev/null
  curl -s 'https://peeringdb-plus.fly.dev/rest/v1/networks?limit=1' > /dev/null
  curl -s 'https://peeringdb-plus.fly.dev/graphql' -X POST -H 'Content-Type: application/json' -d '{"query":"{__typename}"}' > /dev/null
  curl -sf -H 'User-Agent: Mozilla/5.0' 'https://peeringdb-plus.fly.dev/ui/asn/13335' > /dev/null
  sleep 5
done
```
**Expected:** ≥5 distinct `http_route` labels covering /api/{rest...}, /rest/v1/, /graphql, /ui/{rest...}, and GET /healthz.
**Why human:** Investigation refuted all 3 code-bug hypotheses. If only `{http_route="GET /healthz"}` remains after curl loop, follow-up investigation in OTLP exporter / Grafana Cloud receiver paths is needed (out of OBS-04 scope per CONTEXT.md). The contract is locked via the E2E test; production reshaping is operator follow-up.

#### 4. OBS-04 Grafana panel verification

**Test:** Open the Grafana "Request Rate by Route" dashboard panel after the curl loop.
**Expected:** Multi-line breakdown showing one line per matched route family (5 families) instead of the single /healthz line currently observed.
**Why human:** Visual dashboard verification — requires real production traffic and Grafana render.

### Gaps Summary

**No codebase gaps.** All 10 must-have truths verify against the codebase. All 4 acceptance criteria from CONTEXT.md decisions D-01, D-02, D-03 are satisfied:
- **D-01 (OBS-01):** Synchronous one-shot Count(ctx) helper at startup, runs before InitObjectCountGauges callback registration. ✓
- **D-02 (OBS-02):** Pre-warm 4 per-type × 13 + 1 direction × 2 = 54 baseline series after InitMetrics, before scheduler goroutine. ✓
- **D-03 (OBS-04):** Investigation refuted code-bug hypotheses; fix shape was investigation + doc clarification + E2E test lock for ≥5 route families. ✓ (per the user's stated approval that "the contract is locked via test, not that production traffic was reshaped")

**Status: human_needed** because three of the four roadmap success criteria reference deployment-time behaviour (Grafana panel rendering, Prometheus series visibility, multi-route http.route breakdown) that require operator confirmation post-deploy. The codebase work is complete; the deployment-time observations are pending.

This matches the pattern documented in 75-01-SUMMARY.md (PromQL verification deferred to operator), 75-02-SUMMARY.md (5 PromQL queries + dashboard visual check deferred), and 75-03-SUMMARY.md (curl loop + PromQL + Grafana panel inspection deferred). The Phase 75 SUMMARYs explicitly call these out as operator follow-ups, not as missing work.

---

*Verified: 2026-04-27T00:15:00Z*
*Verifier: Claude (gsd-verifier)*
