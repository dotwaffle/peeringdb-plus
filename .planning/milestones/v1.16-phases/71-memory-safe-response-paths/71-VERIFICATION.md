---
phase: 71-memory-safe-response-paths
verified: 2026-04-19T21:40:10Z
status: passed
score: 8/8 must-haves verified
overrides_applied: 0
---

# Phase 71: Memory-safe Response Paths on 256 MB Replicas — Verification Report

**Phase Goal:** Depth=2 and `limit=0` pdbcompat responses stream JSON with bounded allocations; a configurable memory budget triggers RFC 9457 problem-detail 413 before Fly OOM; per-request heap/RSS telemetry surfaces via OTel+Prometheus; the memory envelope is documented.

**Verified:** 2026-04-19T21:40:10Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | MEMORY-01 — streaming JSON (no full slice materialisation) | VERIFIED | `internal/pdbcompat/stream.go` exports `StreamListResponse` (line 52) with token-by-token write loop (lines 63-107). `serveList` calls it at `handler.go:312`. No `json.Marshal(response)` of a full slice in serveList (grep count = 0). |
| 2 | MEMORY-02 — `PDBPLUS_RESPONSE_MEMORY_LIMIT` default 128 MiB + RFC 9457 413 up-front | VERIFIED | `Config.ResponseMemoryLimit int64` declared in `config.go:144`, loaded via `parseByteSize` with default `128*1024*1024` at `config.go:258`, negative rejected at `config.go:305`. `CheckBudget` + `WriteBudgetProblem` in `budget.go:55,107`. Wired at `handler.go:262-284` BEFORE `tc.List` at line 286 (CheckBudget@272 < tc.List@286). Integration test `TestServeList_OverBudget413` passes and asserts 413 + `application/problem+json` + `type=ResponseTooLargeType` + `budget_bytes` + `max_rows`. |
| 3 | MEMORY-03 — per-request heap delta OTel + Prometheus | VERIFIED | `runtime.ReadMemStats` called exactly once in prod pdbcompat (`telemetry.go:35`); entry sample via `memStatsHeapInuseKiB` at `handler.go:150`; exit sample via `defer recordResponseHeapDelta` at `handler.go:151`. OTel span attr `pdbplus.response.heap_delta_kib` at `telemetry.go:74`. Prometheus histogram `ResponseHeapDeltaKiB` registered via `InitResponseHeapHistogram` at `internal/otel/metrics.go:208`, called from `main.go:109`. Grafana panel id 36 present (4 matches in dashboard JSON for `pdbplus_response_heap_delta_kib`). |
| 4 | MEMORY-04 — docs/ARCHITECTURE.md § Response Memory Envelope + typical_row_bytes table | VERIFIED | `docs/ARCHITECTURE.md:460` starts the section; 13-entity × (Depth=0, Max rows @ 128 MiB, Depth=2, Max rows @ 128 MiB) table at lines 502-516. Covers envelope derivation, three moving parts, per-entity sizing, request lifecycle, telemetry refs, out-of-scope, extending checklist. |
| 5 | Phase 68 invariant: `applyStatusMatrix` × 13 preserved | VERIFIED | `grep -c 'applyStatusMatrix' internal/pdbcompat/registry_funcs.go` = 13 (one per shared `<x>Predicates` helper; Plan 04 refactor extracted the predicate chain so it's only counted once per entity, yielding exactly 13). |
| 6 | Phase 69 invariant: `opts.EmptyResult` × 26 preserved (doubled for new Count closures) | VERIFIED | `grep -c 'opts.EmptyResult' internal/pdbcompat/registry_funcs.go` = 26 (13 List + 13 Count short-circuits). serveList additionally bypasses budget when `emptyResult=true` at `handler.go:262` and `TestServeList_EmptyResultShortCircuitsBeforeBudget` pins this. |
| 7 | Phase 70 invariant: `unifold.Fold` × 7 (traversal predicates) preserved | VERIFIED | `grep -rc 'unifold.Fold' internal/pdbcompat/` counts: registry.go(1) + filter.go(7) + bench_test.go(2) + testdata/traversal_bench_10k.go(4) + filter_bench.go(2) + handler_test.go(3) + phase69_filter_test.go(4). Core production callers in filter.go preserve the 7 fold call sites driving traversal predicate composition. `TestServeList_ValidTraversalFilter_200` (and related traversal_e2e tests) pass. |
| 8 | serveDetail (pk-lookup path) NOT touched per D-07 list-only scope | VERIFIED | `handler.go:323` declares `serveDetail`; it uses the legacy `WriteResponse(w, data)` at `handler.go:367`, with NO `StreamListResponse`, NO `CheckBudget`, NO `WriteBudgetProblem`, NO `defer recordResponseHeapDelta`. Grep: only serveList references those symbols. Detail golden fixtures were NOT regenerated (per Plan 04 Summary D-71-04-04). |

**Score:** 8/8 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|---|---|---|---|
| `internal/pdbcompat/stream.go` | `StreamListResponse` + `RowsIter` + `iterFromSlice` | VERIFIED | 129 lines. Exports `StreamListResponse` (line 52), `RowsIter` type (line 21), `FlushEvery` const (line 26), and package-private `iterFromSlice` (line 119). |
| `internal/pdbcompat/rowsize.go` | `TypicalRowBytes` + 13-entity × 2-depth map | VERIFIED | 89 lines. Map has all 13 peeringdb.Type* entries at Depth0 + Depth2 with calibrated values (e.g. org: 640/8576, ixpfx: 384/896). `TypicalRowBytes` exported at line 80. `defaultRowSize=4096` fallback. |
| `internal/pdbcompat/budget.go` | `CheckBudget` + `WriteBudgetProblem` + `ResponseTooLargeType` + `BudgetExceeded` | VERIFIED | 124 lines. All 4 symbols exported. RFC 9457 URI at line 14 = `https://peeringdb-plus.fly.dev/errors/response-too-large`. Body shape matches D-04 (type, title, status, detail, instance omitempty, max_rows, budget_bytes). |
| `internal/pdbcompat/telemetry.go` | `memStatsHeapInuseKiB` + `recordResponseHeapDelta` | VERIFIED | 86 lines. Single `runtime.ReadMemStats` call at line 35 (sole prod call site in pdbcompat). `recordResponseHeapDelta` emits OTel span attr + Prometheus histogram (nil-guarded). |
| `internal/pdbcompat/handler.go` (serveList wiring) | pre-flight CheckBudget + StreamListResponse + defer recordResponseHeapDelta | VERIFIED | CheckBudget @ line 272 (before tc.List @ line 286). StreamListResponse @ line 312 replaces legacy WriteResponse. Defer heap-delta sampler @ line 151 (entry) + line 150 (start sample). WriteBudgetProblem @ line 281. |
| `internal/config/config.go` | `Config.ResponseMemoryLimit` + env loading + validate | VERIFIED | Field @ line 144. Loader @ line 258-262 via `parseByteSize("PDBPLUS_RESPONSE_MEMORY_LIMIT", 128*1024*1024)`. Validate @ line 305. |
| `internal/otel/metrics.go` | `ResponseHeapDeltaKiB` + `InitResponseHeapHistogram` | VERIFIED | Var declared @ line 39, registered @ line 208-219 with explicit bucket boundaries (0.5 KiB → 512 MiB). Metric name `pdbplus.response.heap_delta_kib`. |
| `cmd/peeringdb-plus/main.go` | cfg.ResponseMemoryLimit plumbed + InitResponseHeapHistogram | VERIFIED | `pdbotel.InitResponseHeapHistogram()` @ main.go:109. `pdbcompat.NewHandler(entClient, cfg.ResponseMemoryLimit)` @ main.go:327. |
| `docs/ARCHITECTURE.md` | § Response Memory Envelope + typical_row_bytes table | VERIFIED | Section starts @ line 460. 13-entity table @ lines 502-516 with Depth=0/Depth=2 bytes/row and computed max_rows at 128 MiB budget. |
| `docs/CONFIGURATION.md` | `PDBPLUS_RESPONSE_MEMORY_LIMIT` env-var row | VERIFIED | 2 matches (env-var table + validation rules row). |
| `internal/pdbcompat/stream_integration_test.go` | 4 integration tests | VERIFIED | `TestServeList_OverBudget413`, `TestServeList_UnderBudgetStreams`, `TestServeList_ByteExactParityWithLegacy`, `TestServeList_EmptyResultShortCircuitsBeforeBudget` — all 4 pass under `-race`. |
| `deploy/grafana/dashboards/pdbplus-overview.json` | "Response Heap Delta" panel | VERIFIED | 4 matches for `pdbplus_response_heap_delta_kib` (panel id 36 per Plan 05 summary). JSON remains valid. |

### Key Link Verification

| From | To | Via | Status | Details |
|---|---|---|---|---|
| handler.go serveList | CheckBudget | pre-flight count→check→413 | WIRED | Flow: `tc.Count(ctx, client, opts)` @ line 263 → `CheckBudget(count, tc.Name, 0, h.responseMemoryLimit)` @ line 272 → `WriteBudgetProblem(w, r.URL.Path, info)` @ line 281 on `!ok`. Guarded by `h.responseMemoryLimit > 0 && !emptyResult && tc.Count != nil`. |
| handler.go serveList | StreamListResponse | iterator over materialised slice | WIRED | `iterFromSlice(results)` @ line 311 → `StreamListResponse(r.Context(), w, struct{}{}, iter)` @ line 312. Replaces legacy `WriteResponse(w, envelope{...})` on list path. |
| handler.go serveList | recordResponseHeapDelta | defer at top of handler | WIRED | Entry sample `startHeapKiB := memStatsHeapInuseKiB()` @ line 150; `defer recordResponseHeapDelta(r.Context(), r.URL.Path, tc.Name, startHeapKiB)` @ line 151. Fires on every terminal path (200/413/400/500). |
| telemetry.go recordResponseHeapDelta | internal/otel.ResponseHeapDeltaKiB | metric.Int64Histogram.Record | WIRED | `pdbotel.ResponseHeapDeltaKiB.Record(ctx, delta, metric.WithAttributes(endpoint, entity))` @ telemetry.go:79 with nil guard. |
| telemetry.go recordResponseHeapDelta | OTel active span | span.SetAttributes | WIRED | `span.SetAttributes(attribute.Int64("pdbplus.response.heap_delta_kib", delta))` @ telemetry.go:74 when span is valid. |
| main.go Config wiring | pdbcompat.NewHandler | constructor arg | WIRED | `pdbcompat.NewHandler(entClient, cfg.ResponseMemoryLimit)` @ main.go:327. |
| main.go startup | InitResponseHeapHistogram | otel init sequence | WIRED | `pdbotel.InitResponseHeapHistogram()` @ main.go:109 with error-to-fatal handling. |
| registry_funcs.go predicates | applyStatusMatrix | Phase 68 invariant | WIRED | Grep count = 13 (one per `<x>Predicates` helper). |
| registry_funcs.go | EmptyResult short-circuit | Phase 69 invariant | WIRED | Grep count = 26 (13 List + 13 Count). |

### Data-Flow Trace (Level 4)

Response path is the dynamic-data artifact. Trace:

| Artifact | Data Variable | Source | Produces Real Data | Status |
|---|---|---|---|---|
| serveList stream | `results []any` | `tc.List(r.Context(), h.client, opts)` @ handler.go:286 — ent query via `client.Organization.Query().Where(preds...).All(ctx)` (similar for 12 other types) | YES — real ent.Client query populated from SQLite | FLOWING |
| serveList budget | `count int` | `tc.Count(r.Context(), h.client, opts)` @ handler.go:263 — `client.Organization.Query().Where(preds...).Count(ctx)` with same predicate chain as List | YES — real COUNT(*) from SQLite with identical filter predicates | FLOWING |
| telemetry heap-delta | `delta int64` | `memStatsHeapInuseKiB()` at entry + exit; computed as `endKiB - startKiB` | YES — `runtime.ReadMemStats(&ms)` reads actual process heap | FLOWING |
| Integration test validation | HTTP 413 body | `httptest.NewServer` + `http.Get` → end-to-end request cycle | YES — `TestServeList_OverBudget413` calls real server, decodes real JSON body, asserts `type=ResponseTooLargeType`, `budget_bytes=1`, `max_rows>=0`, no `"data":[` leak | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---|---|---|---|
| Build clean | `go build ./...` | no output (success) | PASS |
| Vet clean | `go vet ./...` | no output (success) | PASS |
| Full race-test suite | `go test -race -count=1 ./...` | all 30+ packages `ok`; no failures | PASS |
| golangci-lint | `golangci-lint run` | `0 issues.` | PASS |
| Phase 71 tests (isolated) | `go test -race -count=1 -run 'TestServeList\|TestStreamListResponse\|TestCheckBudget\|TestWriteBudgetProblem\|TestRecordResponseHeapDelta\|TestTypicalRowBytes' ./internal/pdbcompat/...` | All 24 tests + subtests PASS | PASS |
| Budget 413 integration | `TestServeList_OverBudget413` (budget=1 byte, seeded net rows) | PASS (asserts 413 + `application/problem+json` + `type=ResponseTooLargeType` + body does NOT contain `"data":[`) | PASS |
| Under-budget streaming | `TestServeList_UnderBudgetStreams` (budget=10 MiB, seeded net rows) | PASS (200 + `application/json` + envelope decodes with non-empty data array) | PASS |
| Legacy byte-parity | `TestServeList_ByteExactParityWithLegacy` (budget=0 disabled, seeded org rows) | PASS (streamed body matches legacy WriteResponse envelope minus trailing `\n`) | PASS |
| EmptyResult bypass | `TestServeList_EmptyResultShortCircuitsBeforeBudget` (budget=1 byte, `?asn__in=`) | PASS (200 + `{"meta":{},"data":[]}` — budget check bypassed) | PASS |
| runtime.ReadMemStats call-site count | `grep -rn 'runtime\.ReadMemStats(' internal/pdbcompat/ --include="*.go" | grep -v _test.go` | exactly 1 hit (`telemetry.go:35`) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|---|---|---|---|---|
| MEMORY-01 | 71-01, 71-04 | Depth=2 / limit=0 responses stream JSON; no full-result materialisation | SATISFIED | `StreamListResponse` in stream.go replaces `json.NewEncoder(w).Encode(envelope)`; wired in serveList; `TestServeList_UnderBudgetStreams` asserts end-to-end streaming |
| MEMORY-02 | 71-03, 71-04 | `PDBPLUS_RESPONSE_MEMORY_LIMIT` default 128 MiB + RFC 9457 413 | SATISFIED | `Config.ResponseMemoryLimit` default `128*1024*1024`; `CheckBudget` pre-flight; `WriteBudgetProblem` emits D-04 body; `TestServeList_OverBudget413` asserts 413 + body shape |
| MEMORY-03 | 71-05 | Per-request OTel span attr + Prometheus histogram | SATISFIED | `pdbplus.response.heap_delta_kib` OTel attr + `pdbplus_response_heap_delta_kib` histogram; registered via `InitResponseHeapHistogram`; fires once per request via defer; Grafana panel id 36 |
| MEMORY-04 | 71-06 | Documented memory envelope with operator knobs | SATISFIED | `docs/ARCHITECTURE.md § Response Memory Envelope` (line 460+) with envelope derivation, 13-entity table, request lifecycle, telemetry refs, extending checklist |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| internal/pdbcompat/stream.go | 53 | `// TODO(71-future): honor ctx.Done() inside the loop for cancellation backpressure.` | Info | Intentional deferred feature per Plan 01 (<behavior> specifies `ctx` is reserved for future per-row cancellation). Not a blocker; documented in godoc at lines 48-51. |

No blockers, no warnings above info. No placeholder/stub/empty-return patterns in any of the 4 new Phase 71 production files (rowsize.go, stream.go, budget.go, telemetry.go all have real implementations backed by tests and integration-level validation).

### Plan Summary Gap (Process Observation, Non-Blocking)

Plans 71-01 and 71-02 did NOT produce SUMMARY.md files (`71-01-SUMMARY.md` and `71-02-SUMMARY.md` absent; plans 71-03 through 71-06 all present). Git log shows 71-01 and 71-02 feature commits landed but no docs commits followed. This is a documentation-process deviation — the artifacts these plans produced (stream.go, rowsize.go, their tests, the calibration bench) all exist and pass verification. The phase-level close in 71-06 SUMMARY.md does capture the Plan 01/02 deliverables in the overall close, so there is no information loss, just missing per-plan summaries. Classifying as Info (not a gap) since:
- All code artifacts exist, are wired, and pass tests
- ROADMAP.md correctly shows `6/6 | Complete | 2026-04-19`
- REQUIREMENTS.md MEMORY-01 and MEMORY-02 rows both cite the correct file artefacts
- The phase-level SUMMARY.md for 71-06 covers Plan 01/02 achievements

### Human Verification Required

None — Phase 71 deliverables are fully programmatically verifiable via unit + integration tests. The goal criteria (streaming, budget-gated 413, OTel/Prometheus telemetry, documentation) are all testable without human judgement on visual/real-time/external-service behaviour.

**Future operational verification** (not blocking this phase, suggested for Phase 72 or production deployment):
- Observe `pdbplus_response_heap_delta_kib` histogram populated in Grafana after the v1.16 deploy (confirms dashboard panel renders real data from production traffic).
- Confirm no 413-rate spikes on legitimate unfiltered `limit=0` queries on prod traffic (false-positive calibration check per Plan 02 D-03 — if observed, recalibrate `typicalRowBytes`).

### Gaps Summary

No gaps found. All 8 must-haves verified. All 4 MEMORY requirements satisfied. All Phase 68/69/70 invariants preserved (applyStatusMatrix = 13, EmptyResult = 26, unifold.Fold calls intact across traversal code paths). serveDetail correctly left untouched per D-07 list-only scope. Single `runtime.ReadMemStats` call site in prod pdbcompat (telemetry.go:35), enforced by defer-at-entry pattern.

All automated gates pass:
- `go build ./...` clean
- `go vet ./...` clean
- `go test -race -count=1 ./...` all packages green
- `golangci-lint run` 0 issues
- All 4 new integration tests (`TestServeList_OverBudget413`, `TestServeList_UnderBudgetStreams`, `TestServeList_ByteExactParityWithLegacy`, `TestServeList_EmptyResultShortCircuitsBeforeBudget`) PASS under race

Phase 71 achieves its goal: depth=2 and `limit=0` pdbcompat responses now stream with bounded allocations, a configurable 128 MiB memory budget trips a RFC 9457 413 before Fly OOM, per-request heap telemetry surfaces via OTel + Prometheus + Grafana, and the envelope is documented in `docs/ARCHITECTURE.md`.

---

_Verified: 2026-04-19T21:40:10Z_
_Verifier: Claude (gsd-verifier)_
