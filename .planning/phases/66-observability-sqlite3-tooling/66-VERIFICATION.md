---
phase: 66-observability-sqlite3-tooling
verified: 2026-04-18T12:00:00Z
status: human_needed
score: 10/10 must-haves verified
overrides_applied: 0
human_verification:
  - test: "sqlite3 available on prod primary via fly ssh console"
    expected: "fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db \".tables\"' returns the PeeringDB table list"
    why_human: "Requires live Fly.io SSH access and prod deployment; verifier cannot exec into a running Fly machine"
  - test: "Grafana 'Sync Memory (SEED-001 watch)' row renders on imported dashboard"
    expected: "Panels 'Peak Heap (MiB)' (threshold line at 400) and 'Peak RSS (MiB)' (threshold line at 384) render non-empty timeseries from Prometheus, with the heap threshold line visible in the panel"
    why_human: "Requires live Prometheus datasource and Grafana UI; JSON validity and metric-name wiring verified programmatically, but panel rendering is visual"
  - test: "slog.Warn('heap threshold crossed', ...) fires in prod under real sync load when threshold tuned low"
    expected: "Setting PDBPLUS_HEAP_WARN_MIB=50 (below observed baseline ~84 MiB) produces a WARN log line with the heap_over=true attr on the next sync cycle"
    why_human: "Threshold-crossing behaviour at real sync workload can only be observed against prod or a staging machine with real data"
  - test: "Panel 35 (Peak Heap by Process Group) shows primary vs replica breakdown once fly.process_group resource attr is wired"
    expected: "Two (or more) series appear — one per fly.process_group value"
    why_human: "Known limitation flagged in 66-02 SUMMARY: fly.process_group OTel resource attribute wiring is not in scope for Phase 66. Deferred follow-up; panel JSON is correct but will render a single aggregate series until that wiring lands"
---

# Phase 66: Observability + sqlite3 tooling — Verification Report

**Phase Goal:** Make SEED-001's "peak heap >threshold" trigger observable via OTel span attrs + slog.Warn + Prometheus gauges; document the SEED-001 escalation path; reference the sqlite3 debug shell landed pre-phase via quick task 260418-1cn.

**Verified:** 2026-04-18
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Operators can set `PDBPLUS_HEAP_WARN_MIB` (default 400) and `PDBPLUS_RSS_WARN_MIB` (default 384) | VERIFIED | `internal/config/config.go:143,151,233,239` — fields + parseMiB + Load + defaults |
| 2 | `parseMiB` rejects unit suffixes, negatives, non-numeric, floats | VERIFIED | `internal/config/config.go:451-…`; `TestLoad_HeapWarnMiB_Parse` + `TestLoad_RSSWarnMiB_Parse` cover 8 cases each, race-clean |
| 3 | `emitMemoryTelemetry` called from `recordSuccess`, `rollbackAndRecord`, `recordFailure` | VERIFIED | 3 call sites at worker.go lines 540, 561, 1267 (grep-verified) |
| 4 | `readLinuxVMHWM` parses `/proc/self/status` VmHWM; returns `(n, true)` on Linux, `(0, false)` elsewhere | VERIFIED | worker.go:509; `TestReadLinuxVmHWM` asserts positive bytes on Linux |
| 5 | OTel span attrs `pdbplus.sync.peak_heap_mib` (always) + `pdbplus.sync.peak_rss_mib` (Linux only) | VERIFIED | worker.go:459,464; `TestEmitMemoryTelemetry_Attrs` asserts heap always present + RSS on Linux |
| 6 | `slog.Warn("heap threshold crossed", ...)` with typed attrs including `heap_over` / `rss_over` bools | VERIFIED | worker.go:489-493; test asserts warn fires for heap_over, not for below/both-disabled |
| 7 | Prometheus gauges `pdbplus.sync.peak_heap_mib` + `pdbplus.sync.peak_rss_mib` registered via `InitMemoryGauges`, wired from `main.go`, suppress zero values | VERIFIED | `internal/otel/metrics.go:147-182`; `cmd/peeringdb-plus/main.go:101` calls init; `SyncPeakHeapMiB.Store` at worker.go:469 |
| 8 | Grafana dashboard has `Sync Memory (SEED-001 watch)` row with 3 panels: Peak Heap (threshold 400), Peak RSS (threshold 384), Peak Heap by Process Group | VERIFIED | `deploy/grafana/dashboards/pdbplus-overview.json` row ID 32, panels 33/34/35; `jq` confirms thresholds 400/384 present and JSON valid |
| 9 | CLAUDE.md has `### Sync observability` section + env var rows + SEED-001 + slog key + attr names; docs/DEPLOYMENT.md has `Sync memory watch (SEED-001)` subsection with sqlite3 one-liner; PROJECT.md has Phase 66 Key Decisions row | VERIFIED | CLAUDE.md:129-130,185-201; docs/DEPLOYMENT.md:293-309; .planning/PROJECT.md:221 |
| 10 | Phase 66 does NOT re-add sqlite3 to Dockerfile.prod (landed pre-phase in quick task 260418-1cn / commit 4dfc52a) | VERIFIED | `git diff --name-only 7858fa1..f543d44` does not include `Dockerfile.prod`; Dockerfile.prod line 17 already has `apk add --no-cache fuse3 sqlite` from 4dfc52a |

**Score:** 10/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|---------|---------|--------|---------|
| `internal/config/config.go` | HeapWarnBytes + RSSWarnBytes fields, parseMiB helper, validate() bounds | VERIFIED | lines 132-151 (fields), 233-243 (Load wiring), 274-279 (validate), 440-…(parseMiB) |
| `internal/config/config_test.go` | Table-driven tests for parseMiB | VERIFIED | `TestLoad_HeapWarnMiB_Parse` + `TestLoad_RSSWarnMiB_Parse` — 8 cases each including default, zero-disable, negative, unit-suffix, float rejects |
| `internal/sync/worker.go` | emitMemoryTelemetry + readLinuxVMHWM + 3 call sites | VERIFIED | helper at 445-494; VmHWM parser at 509-…; 3 call sites at 540/561/1267 |
| `internal/sync/worker_test.go` | Telemetry + VmHWM unit tests | VERIFIED | TestEmitMemoryTelemetry_Attrs (3 cases: below/heap_over/both_disabled) + TestReadLinuxVmHWM (Linux-only, skipped elsewhere) |
| `cmd/peeringdb-plus/main.go` | Wires HeapWarnBytes/RSSWarnBytes + InitMemoryGauges | VERIFIED | lines 101 (InitMemoryGauges), 248-249 (WorkerConfig plumbing) |
| `internal/otel/metrics.go` | SyncPeakHeapMiB/RSSMiB atomics + InitMemoryGauges() | VERIFIED | atomics at 19,24; Int64ObservableGauge registration at 153-182 with zero-suppression callback |
| `deploy/grafana/dashboards/pdbplus-overview.json` | Sync Memory row + 3 panels, thresholds, unique IDs, valid JSON | VERIFIED | `jq empty` passes; 16 panels, 16 unique IDs; threshold 400 on heap, 384 on RSS |
| `CLAUDE.md` | Env var table rows + Sync observability section | VERIFIED | env var rows at 129-130; section at 185-201; references SEED-001, emitMemoryTelemetry, heap threshold crossed, sqlite3 |
| `docs/DEPLOYMENT.md` | SEED-001 watch subsection + dashboard panel names + sqlite3 one-liner | VERIFIED | section at 293-309; 5 paragraphs covering measurement, thresholds, dashboard, escalation, incident-response shell |
| `.planning/PROJECT.md` | Phase 66 Key Decisions row | VERIFIED | line 221 — single row capturing hybrid OTel/slog/gauge design, defaults, VmHWM source, non-flip posture |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/config/config.go` | `internal/sync/worker.go` | WorkerConfig.HeapWarnBytes / RSSWarnBytes via main.go | WIRED | main.go:248-249 passes cfg fields; worker.go uses `w.config.HeapWarnBytes` at call sites |
| `worker.go emitMemoryTelemetry` | OTel span | `span.SetAttributes(attribute.Int64("pdbplus.sync.peak_heap_mib", ...))` | WIRED | worker.go:458-466; test captures via tracetest.NewSpanRecorder |
| `worker.go emitMemoryTelemetry` | Prometheus | `pdbotel.SyncPeakHeapMiB.Store(...)` + InitMemoryGauges callback | WIRED | worker.go:469 Store; metrics.go:159 Load in gauge callback |
| `main.go` | `pdbotel.InitMemoryGauges` | Called during startup before DB open | WIRED | main.go:101 returns startup error if registration fails |
| `Grafana panels` | OTel gauges | PromQL `pdbplus_sync_peak_heap_mib` (OTel dot→underscore mapping) | WIRED | panels 33/34/35 reference both `pdbplus_sync_peak_heap_mib` + `pdbplus_sync_peak_rss_mib` |
| `docs/DEPLOYMENT.md` Monitoring | `.planning/seeds/SEED-001-incremental-sync-evaluation.md` | Markdown link + threshold var names | WIRED | explicit filesystem path reference in SEED-001 escalation paragraph |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|---------|---------------|--------|--------------------|--------|
| `emitMemoryTelemetry` span attrs | heapMiB, rssMiB | `runtime.MemStats.HeapInuse` + `/proc/self/status` VmHWM | Yes (stdlib runtime sampling + kernel-backed /proc read) | FLOWING |
| Prometheus gauges | `SyncPeakHeapMiB.Load()` | `SyncPeakHeapMiB.Store(heapMiB)` at end of each sync cycle | Yes (store happens on every recordSuccess/rollback/recordFailure path) | FLOWING |
| Grafana panels 33/34 | `pdbplus_sync_peak_heap_mib` / `pdbplus_sync_peak_rss_mib` time series | Prometheus scrape of the OTel autoexport | Yes (gauges registered; zero-suppressed until first sync) | FLOWING (pending live panel render — human verification) |
| Grafana panel 35 | `avg by (fly_process_group) (pdbplus_sync_peak_heap_mib)` | Same metric, grouped label | Partially — label not yet exported as OTel resource attribute | HOLLOW_PROP (documented known limitation in 66-02 SUMMARY; flagged as follow-up, not a Phase 66 gate per plan must-haves) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|---------|---------|--------|--------|
| Config tests pass | `go test -race -count=1 -run 'TestLoad_HeapWarnMiB_Parse\|TestLoad_RSSWarnMiB_Parse' ./internal/config/...` | `ok` | PASS |
| Worker tests pass | `go test -race -count=1 -run 'TestEmitMemoryTelemetry\|TestReadLinuxVmHWM' ./internal/sync/...` | `ok` | PASS |
| Build clean | `go build ./...` | no output (clean) | PASS |
| Vet clean on touched packages | `go vet ./internal/sync/... ./internal/config/... ./internal/otel/... ./cmd/peeringdb-plus/...` | no output | PASS |
| Lint clean on touched packages | `golangci-lint run ./internal/sync/... ./internal/config/... ./internal/otel/... ./cmd/peeringdb-plus/...` | `0 issues` | PASS |
| Dashboard JSON valid | `jq empty deploy/grafana/dashboards/pdbplus-overview.json` | exit 0 | PASS |
| Panel IDs unique | `jq '[.panels[].id] \| (length == (unique \| length))'` | `true` (16 == 16) | PASS |
| Thresholds present | `jq '.panels[] \| select(.title == "Peak Heap (MiB)") \| .fieldConfig.defaults.thresholds.steps[].value'` | includes 400; RSS panel includes 384 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| OBS-04 | 66-03 | sqlite3 binary available in prod image for operator debugging | SATISFIED | Dockerfile.prod:17 (landed in quick task 260418-1cn / 4dfc52a); documented in docs/DEPLOYMENT.md:307 and CLAUDE.md Sync observability section |
| OBS-05 | 66-01 + 66-02 | Heap-threshold monitoring + Grafana heap panel | SATISFIED | Span attrs + slog.Warn + Prometheus gauges (Plan 66-01); 3 Grafana panels under Sync Memory row (Plan 66-02) |
| DOC-04 | 66-03 | Peak-heap watching documented in CLAUDE.md + DEPLOYMENT.md | SATISFIED | CLAUDE.md:185-201 (Sync observability section); docs/DEPLOYMENT.md:293-309 (Sync memory watch subsection); PROJECT.md:221 (Key Decisions row) |

### Anti-Patterns Found

None. `grep -E "TODO|FIXME|XXX|HACK"` returns zero matches across `internal/sync/worker.go` and `internal/otel/metrics.go`.

### Human Verification Required

Four items require human verification — automated checks passed for all programmatic dimensions, but the following require live prod/Grafana access:

### 1. sqlite3 available on prod primary

**Test:** `fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db ".tables"'`
**Expected:** Returns the PeeringDB table list (network, facility, ix, etc.)
**Why human:** Requires live Fly.io SSH access to the prod machine.

### 2. Grafana panels render with real data

**Test:** Import `deploy/grafana/dashboards/pdbplus-overview.json` into the Grafana instance backed by prod Prometheus; navigate to the "Sync Memory (SEED-001 watch)" row.
**Expected:** Panels 33 "Peak Heap (MiB)" and 34 "Peak RSS (MiB)" render non-empty timeseries; heap panel shows a threshold line at 400 MiB; RSS panel at 384 MiB.
**Why human:** Requires live Prometheus datasource and Grafana UI. Dashboard JSON is programmatically validated (valid JSON, unique IDs, correct queries and thresholds), but visual rendering and real data presence require a human eye.

### 3. slog.Warn fires under real sync pressure

**Test:** Set `PDBPLUS_HEAP_WARN_MIB=50` on a staging machine (below observed baseline ~84 MiB); run one sync cycle; inspect logs.
**Expected:** A structured log line `"heap threshold crossed"` with `heap_over=true` attr appears at WARN level after the next `recordSuccess`.
**Why human:** Requires realistic sync workload to observe threshold-crossing behaviour end-to-end; unit tests validate the code path but not prod log-pipeline consumption.

### 4. Panel 35 (process-group breakdown) — deferred follow-up

**Test:** After `fly.process_group` is wired as an OTel resource attribute (not in Phase 66 scope), verify Panel 35 shows one series per process group (primary + replicas).
**Expected:** Two (or more) series labeled by `fly_process_group`.
**Why human:** Known limitation flagged in 66-02 SUMMARY. Panel JSON is correct; the label will appear once the resource attribute wiring lands (deferred to a future small task). Does not block Phase 66 closure — panel renders a single aggregate series in the meantime.

### Gaps Summary

No gaps. All 10 observable truths verified, all artifacts present and substantive, all key links wired, data flows correctly through OTel span → Prometheus gauge → Grafana panel pipeline (with the documented single-label limitation on Panel 35). `go test -race`, `go vet`, `golangci-lint`, and `jq empty` all pass.

Four live-system checks need human verification before the phase can be signed off against prod.

---

_Verified: 2026-04-18_
_Verifier: Claude (gsd-verifier)_
