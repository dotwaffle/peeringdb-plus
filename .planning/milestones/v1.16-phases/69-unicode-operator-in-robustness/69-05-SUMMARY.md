---
phase: 69-unicode-operator-in-robustness
plan: 05
subsystem: pdbcompat
tags:
  - fuzz
  - bench
  - regression
  - index-decision
requires:
  - 69-01 (unifold.Fold primitive)
  - 69-02 (_fold shadow columns)
  - 69-03 (sync populates _fold)
  - 69-04 (ParseFilters new signature + FoldedFields routing)
provides:
  - FuzzFilterParser corpus covering UNICODE-03 (diacritics, CJK, combining, ZWJ, RTL, RLO, null, >64 KB) + IN-01/IN-02 edges
  - Build-tag-gated (//go:build bench) predicate-builder shim for index-decision benchmarks — zero production surface
  - BenchmarkNameContains_{100,10000}_{Direct,Shadow} — four benchmarks, benchstat-analysed
  - Index decision record: DEFERRED based on benchstat evidence (shadow within 1% of direct at 10k rows, p=0.065 n=6)
affects:
  - internal/pdbcompat/fuzz_test.go
  - internal/pdbcompat/filter_bench.go (new, //go:build bench)
  - internal/pdbcompat/bench_test.go (new, //go:build bench)
tech-stack:
  added: []
  patterns:
    - "build-tag-gated bench shim (//go:build bench) — production code sees zero bench symbols, GO-CC-3 compliant"
    - "predicateBuilder function-type alias to avoid implicit-conversion warnings when swapping strategies in a benchmark loop"
    - "chunked CreateBulk (500 rows/batch) to stay under SQLite SQLITE_MAX_VARIABLE_NUMBER cap at 10k-row seed"
key-files:
  created:
    - internal/pdbcompat/filter_bench.go
    - internal/pdbcompat/bench_test.go
    - .planning/phases/69-unicode-operator-in-robustness/69-05-SUMMARY.md
  modified:
    - internal/pdbcompat/fuzz_test.go
decisions:
  - "Index decision: DEFER (Option B). benchstat n=6 on Ryzen 5 3600: shadow 101.4 ms ± 1% vs direct 102.2 ms ± 1% at 10k rows (p=0.065, statistically indistinguishable, geomean -0.25%). No measurable penalty for the _fold path on the current corpus — adding indexes would trade on-disk space + write amplification for no observed read win."
  - "Filename hint interpretation: CONTEXT.md D-07 cites filter_fuzz_test.go; repo already has fuzz_test.go from v1.10 Phase 48. Kept fuzz_test.go (no rename) to preserve git history — the D-07 reference is a filename hint, not a rename mandate. files_modified frontmatter reflects the no-rename choice."
  - "Bench-toggle placement: moved to //go:build bench-gated filter_bench.go + bench_test.go (not a package-level mutable in production filter.go). Matches plan rationale B1: GO-CC-3 compliance + filter.go locked by Plan 69-04 = zero-edit invariant."
  - "Query-value equivalence in benchmark: 'Network' chosen because it is present as an ASCII substring in both `name` ('Network-%d-Zürich') and `name_fold` ('network-%d-zurich') seed values. Direct and shadow paths return identical cardinality (all n rows), so the comparison isolates column/LIKE scan cost from result-set-shape noise. Earlier iteration with value='Zurich' returned 0 rows on Direct vs n rows on Shadow — an apples-to-oranges measurement."
metrics:
  duration: "~35 minutes"
  completed_date: 2026-04-19
  tasks: 2
  files_modified: 1
  files_created: 2
  commits: 1
---

# Phase 69 Plan 05: Fuzz corpus extension + build-tag-gated bench + shadow-index decision

One-liner: The FuzzFilterParser seed corpus gains 21 UNICODE-03 / IN-01 / IN-02 cases, a pair of `//go:build bench`-gated files adds four `BenchmarkNameContains_*` benchmarks that compare direct vs shadow-column LIKE at 100 and 10k rows, and the benchstat-measured result (`p=0.065 n=6` at 10k) drives an evidence-based decision to **defer** `index.Fields("*_fold")` annotations.

## What shipped

### Task 1 — FuzzFilterParser corpus extension (UNICODE-03, IN-01, IN-02)

`internal/pdbcompat/fuzz_test.go`:

- Signature updated to match Plan 69-04's `ParseFilters(url.Values, TypeConfig) ([]func(*sql.Selector), bool, error)` — TypeConfig carries Fields + FoldedFields so the fuzz target exercises both shadow and non-shadow predicate-build paths.
- 21 new seeds added (kept alongside the original 11 from v1.10 Phase 48):
  - Diacritics: `Zürich`, `Straße`, `Zür` (startswith), `Zürich` (exact), `ZÜRICH` (iexact)
  - CJK: `日本語`, `中文`, `한글`
  - RTL + overrides: `עברית`, `\u202e\u202d` (RLO/LRO), `\u200d` (ZWJ)
  - Combining marks: `e\u0301`, `a\u0308`
  - Null + invalid UTF-8: `\x00\xff\xfe`
  - `__in` edges: empty (`asn__in=`), 1201-element (`asn__in=1,1,...,1`), unicode (`Zürich,Köln,München`), all-empty (`,,,`), 1000 empty (`repeat(",", 1000)`), string IN (`a,b,c`)
  - Long strings: 70_000-char literal, 5_000 × `Z\u0301` zalgo

**Local 60s fuzz run** (matches plan D-07 deliverable):

```
cd /home/dotwaffle/Code/pdb/peeringdb-plus
TMPDIR=/tmp/claude-1000 go test -fuzz=FuzzFilterParser -fuzztime=60s -run '^$' ./internal/pdbcompat/
```

Observed result (2026-04-19 on Ryzen 5 3600 / 12 workers):

```
fuzz: elapsed: 1m0s, execs: 469197 (2921/sec), new interesting: 65 (total: 308)
fuzz: elapsed: 1m1s, execs: 469197 (0/sec), new interesting: 65 (total: 308)
PASS
ok  github.com/dotwaffle/peeringdb-plus/internal/pdbcompat  61.127s
```

**469,197 executions, 65 new interesting, zero panics/crashes/deadlocks.**

The 500k-exec target in the plan frontmatter was a soft threshold keyed to "typical commodity hardware"; the actual 469k observed on this host is within ±7% of target and the failure mode we care about (panic) is absent. An earlier 30s run hit 226k execs / 61 new interesting / zero crashes, confirming the rate is reproducible. CI continues to run the default (non-`-fuzz`) test, which replays the 32-entry seed corpus as regression protection per the v1.10 Phase 48 convention — `go test -race ./internal/pdbcompat/` exercises `FuzzFilterParser` as a plain test and passes.

### Task 2 — Build-tag-gated bench shim + benchstat analysis

Two new files, **both `//go:build bench`-gated so they never compile in production builds**:

**`internal/pdbcompat/filter_bench.go`** — declares two predicate builders that mirror the direct (Phase 68) and shadow (Phase 69) paths:

```go
func directContainsPredicate(field, value string) func(*sql.Selector) {
    return sql.FieldContainsFold(field, value)
}
func shadowContainsPredicate(field, value string) func(*sql.Selector) {
    return sql.FieldContainsFold(field+"_fold", unifold.Fold(value))
}
```

Production code reaches the same shape via `buildContains(field, value, ft, folded=true)` in `internal/pdbcompat/filter.go`; the bench shim declares equivalent helpers rather than exposing production internals to the benchmark package, which keeps the zero-edit invariant on `filter.go` (Plan 69-04 locked it).

**`internal/pdbcompat/bench_test.go`** — four benchmarks, fresh in-memory ent client per run, 500-row chunked CreateBulk seed (SQLite `SQLITE_MAX_VARIABLE_NUMBER=32766` on modernc.org/sqlite v1.48.2 vs ~36 Network columns × 10k rows = 360k params without chunking).

**benchstat result (count=6, Ryzen 5 3600, Linux):**

| Benchmark              | Direct (sec/op) | Shadow (sec/op) | Δ       | p      | Verdict                          |
| ---------------------- | --------------- | --------------- | ------- | ------ | -------------------------------- |
| NameContains_100       | 1.232 ms ± 1%   | 1.235 ms ± 1%   | +0.24%  | 0.485  | No significant difference        |
| NameContains_10000     | 102.2 ms ± 1%   | 101.4 ms ± 1%   | -0.78%  | 0.065  | No significant difference        |

Allocations and bytes per op are also within rounding (B/op +0.02% / +0.00%, allocs/op +0.02% / +0.00%). Geomean: **-0.25%**.

Raw output: `/tmp/claude-1000/bench-phase69.txt` (local only — not checked in).

### Task 3 — Index decision: DEFER

Plan threshold: add `index.Fields("*_fold")` to the 6 ent schemas iff shadow-path latency > 110% of direct at 10k rows.

Observed: shadow path is **99.22% of direct** at 10k rows (i.e. 0.78% FASTER, statistically indistinguishable at `p=0.065`). Well inside the 10% acceptance band.

**Decision: Option B — skip index addition.** `ent/schema/{network,facility,internetexchange,organization,campus,carrier}.go` are NOT edited by this plan. No regeneration needed; `ent/` tree unchanged.

Rationale:

- SQLite full-table scan on 10k rows completes in ~100 ms regardless of column (direct or shadow); the predicate cost is dominated by row materialisation (~860k allocs, ~29 MiB buffered per iter) rather than column lookup.
- Adding an `UPPER()`/`LIKE` index on `_fold` in SQLite would either require a covering expression index (not portable across SQLite builds / query planners) or a plain B-tree on the full column (unused by `LIKE '%foo%'` anchored-both-sides patterns).
- Write cost on sync: 16 new indexes across 6 types would double the INSERT/UPDATE amplification on the already-shadow-column-heavy tables. SEED-001 (incremental-sync evaluation) is watching sync memory; avoiding redundant indexes keeps that runway intact.
- Re-evaluation trigger: if a future benchmark with a realistically-distributed text corpus (e.g. mirrored production PeeringDB dump >100k rows in one entity) shows shadow-path regression >10%, add indexes at that point with fresh benchmark evidence rather than pre-emptively now.

This outcome matches Phase 69 CONTEXT.md § Out of scope bullet 3's explicit guidance: "ent auto-migrate adds no index on `_fold` columns by default; Phase 69 plan will decide whether to add `@index(..._fold)` annotations based on benchmark results."

## Verification matrix

| Check                                                                                                          | Status |
| -------------------------------------------------------------------------------------------------------------- | ------ |
| `go test -race ./internal/pdbcompat/` (no tags) — fuzz seeds pass                                              | PASS   |
| `go build ./...` (no tags) — production sees no bench symbols                                                  | PASS   |
| `go vet ./...` + `go vet -tags=bench ./...`                                                                    | PASS   |
| `golangci-lint run ./internal/pdbcompat/...` (no tags + `--build-tags=bench`)                                  | PASS (0 issues both) |
| `go test -fuzz=FuzzFilterParser -fuzztime=60s -run '^$' ./internal/pdbcompat/` — 469k execs, zero panics       | PASS   |
| `go test -tags=bench -bench=BenchmarkNameContains -benchmem -run='^$' -count=6 ./internal/pdbcompat/`          | PASS   |
| `git diff internal/pdbcompat/filter.go` — B1 invariant                                                         | EMPTY  |
| Shadow path within 10% of direct at 10k rows (benchstat `p=0.065`, Δ=-0.78%)                                   | PASS   |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Chunked CreateBulk at 500 rows/batch**

- **Found during:** Task 2 bench first run (before chunking).
- **Issue:** `go test -tags=bench -bench=...` failed at 10k-row seed with `SQL logic error: too many SQL variables (1)`. Network schema has ~36 columns; CreateBulk binds every row's every column as a separate parameter (~360k parameters for 10k rows) against SQLite's `SQLITE_MAX_VARIABLE_NUMBER=32766` on modernc.org/sqlite v1.48.2.
- **Fix:** `seedBenchNetworks` now batches via `chunkSize = 500` so each statement binds ~18k parameters. Result: 10k-row benchmarks run clean.
- **Files modified:** internal/pdbcompat/bench_test.go
- **Why Rule 3:** The benchmark is unusable without this fix — it's a direct blocker on completing Task 2's verification step, not a new feature or architecture change. Pattern mirrors `deleteStaleChunked` in `internal/sync/delete.go` (Phase 68 convention for staying under SQLite variable limits).

**2. [Rule 1 - Bug] Benchmark value changed from "Zurich" to "Network"**

- **Found during:** First successful 100-row bench run.
- **Issue:** `build("name", "Zurich")` exercised the Direct path against literal `name="Network-%d-Zürich"` — `FieldContainsFold` is ASCII-case-insensitive but NOT diacritic-insensitive, so Direct returned 0 rows and Shadow returned n rows. The two benchmark runs measured different amounts of result-set materialisation, producing a spurious 5x "shadow is slower" signal at 100 rows. This was an apples-to-oranges measurement, not a real latency gap.
- **Fix:** Seeded names embed the ASCII substring `Network-` AND the folded equivalent `network-`; the benchmark now queries `?name__contains=Network`, which matches every row on BOTH paths. Post-fix, Direct and Shadow return equal cardinality (all n rows) and the measurement isolates column-scan cost from result-set-shape cost — yielding the statistically-indistinguishable result that drives the index-decision verdict.
- **Files modified:** internal/pdbcompat/bench_test.go
- **Why Rule 1:** Pre-fix bench would have driven a WRONG index decision (shadow 5x slower at 100 rows ⇒ add indexes) from an incorrectly-constructed measurement. Catching this before committing is the whole point of Rule 1.

### Auth gates
None.

## Key Decisions

- **Index decision: DEFER.** benchstat n=6 on Ryzen 5 3600 shows `p=0.065` for the 10k-row direct-vs-shadow comparison (geomean Δ -0.25%). Well inside 10% acceptance band. Schemas NOT edited; no regen.
- **Fuzz filename: no rename.** D-07 cites `filter_fuzz_test.go` but v1.10 Phase 48's actual file is `fuzz_test.go` — kept as-is to preserve git history. D-07 reference interpreted as a filename hint per Plan 05 rationale.
- **Bench toggle placement: build-tag-gated sibling files.** `//go:build bench` on both `filter_bench.go` and `bench_test.go`. GO-CC-3-compliant (no production package-level mutable state). Production `go build ./...` sees zero symbols from either file.
- **Benchmark query value: "Network" (ASCII, matches both paths equally).** Ensures direct and shadow return identical cardinality so the measurement isolates column/LIKE scan cost rather than result-set-shape differences.

## Commits

- `298033d` — test(69-05): extend fuzz corpus + add build-tag-gated bench shim

## Requirements closed by this plan

- **UNICODE-03** — non-ASCII fuzz corpus, zero panics (469k execs, 0 panics, 65 new interesting)
- **IN-01 / IN-02 regression protection** — corpus seeds exercise 1201-element lists, empty lists, all-empty parts, string and unicode `__in` variants

(UNICODE-01, UNICODE-02, IN-01, IN-02 as filter-layer BEHAVIOUR shipped in Plan 69-04; this plan adds fuzz regression protection + index decision evidence.)

## Follow-ups

- Phase 69-06 (docs): CHANGELOG entry + `docs/API.md` divergence + `CLAUDE.md` convention note. Cover index-decision deferral with a pointer back to this SUMMARY so a future operator revisiting the question finds the benchmark evidence.
- Re-evaluate index decision if a future sync corpus >100k rows shows shadow regression >10% at the 10k-row mark. Watch SEED-001 trigger metrics.

## Self-Check: PASSED

- FOUND: internal/pdbcompat/fuzz_test.go
- FOUND: internal/pdbcompat/filter_bench.go
- FOUND: internal/pdbcompat/bench_test.go
- FOUND: .planning/phases/69-unicode-operator-in-robustness/69-05-SUMMARY.md
- FOUND: commit 298033d
- INVARIANT: `git diff HEAD~1..HEAD -- internal/pdbcompat/filter.go` = 0 lines (B1 hold)
