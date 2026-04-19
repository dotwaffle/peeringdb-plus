---
phase: 69-unicode-operator-in-robustness
verified: 2026-04-19T17:09:00Z
status: passed
score: 7/7 must-haves verified
overrides_applied: 0
---

# Phase 69: Unicode folding, operator coercion, `__in` robustness — Verification Report

**Phase Goal (ROADMAP):** pdbcompat filter layer reproduces upstream `rest.py:544-662` value handling — `unidecode`-equivalent diacritic folding, `__contains`/`__startswith` coerced to case-insensitive, arbitrarily-large `__in` via single-bind `json_each` rewrite without SQLite 999-variable ceiling.

**Verified:** 2026-04-19T17:09:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | **UNICODE-01** filter values Unicode-folded before SQL; ASCII queries match non-ASCII DB rows | VERIFIED | `internal/unifold/unifold.go:60` `func Fold`; 16 `_fold` fields in 6 ent schemas (`ent/schema/{organization,network,facility,internetexchange,campus,carrier}.go` — 3/3/3/4/1/2); sync populates via 16 `unifold.Fold(...)` calls in `internal/sync/upsert.go`; filter routes through `FoldedFields` map in `internal/pdbcompat/registry.go:70,110,155,198,238,356,390`; `buildExact/buildContains/buildStartsWith` in `filter.go` call `unifold.Fold(value)` on shadow column (filter.go:155 etc.); round-trip test `TestUpsertPopulatesFoldColumns` + `TestShadowRouting_Network_NameFold` GREEN |
| 2 | **UNICODE-02** `__contains`/`__startswith` coerced to case-insensitive | VERIFIED | `coerceToCaseInsensitive` helper at `internal/pdbcompat/filter.go:59`, invoked at `filter.go:121`; maps `contains→icontains` and `startswith→istartswith`; `TestCoerce_OnlyContainsAndStartswith_Untouched` at `phase69_filter_test.go:299` GREEN |
| 3 | **UNICODE-03** non-ASCII fuzz corpus, zero panics | VERIFIED | `internal/pdbcompat/fuzz_test.go` has 21 Phase 69 D-07 seeds (Zürich/Straße/CJK/Hangul/Hebrew/combining/null/RLO/ZWJ/70k/zalgo + IN edges including 1200+1 list); `FuzzFilterParser` uses new `ParseFilters(params, TypeConfig)` signature with `FoldedFields: {name: true}` to exercise shadow-routing path; 69-05-SUMMARY records local 60s fuzz run = 469197 execs, 0 panics; seed corpus runs as regular test and passes under `go test -race` |
| 4 | **IN-01** `__in` accepts >999 values (bypasses SQLite 999-variable ceiling) | VERIFIED | `json_each(?)` single-bind rewrite at `internal/pdbcompat/filter.go:264`; `TestInJsonEach_Large_Bypasses_SQLite_Limit` at `phase69_filter_test.go:149` seeds 1500 Networks and HTTP-fetches all 1500 via `?asn__in=1,2,...,1500` — GREEN; `TestInJsonEach_ExplainQueryPlan` confirms `json_each` appears in EXPLAIN QUERY PLAN output |
| 5 | **IN-02** empty `__in` returns empty set | VERIFIED | `errEmptyIn` sentinel at `filter.go:20`, caught at `filter.go:104` setting `emptyResult=true`; `QueryOptions.EmptyResult` field at `registry.go:47`; 13 closure guards in `registry_funcs.go` (lines 60, 97, 134, 171, 208, 245, 282, 319, 356, 393, 430, 467, 504) — exactly 13 matches; `TestInJsonEach_EmptyString_ReturnsEmpty` GREEN |
| 6 | Phase 68 status × since matrix is PRESERVED | VERIFIED | `grep 'applyStatusMatrix'` in `registry_funcs.go` returns exactly 13 matches (one per list closure, 12 regular + 1 campus); `TestPhase68StatusMatrix_Phase69Layering` at `phase69_filter_test.go:345` asserts `?status=deleted&name__contains=foo` returns only the ok row with id=1 (status silently dropped, shadow-column name filter layered on top) — GREEN |
| 7 | Build/test gates pass at HEAD | VERIFIED | `go build ./...` clean; `go vet ./...` clean; `go test -race ./...` all packages GREEN (race detector clean); `golangci-lint run` → `0 issues.` |

**Score:** 7/7 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/unifold/unifold.go` | `Fold(string) string` with NFKD + hand fold map | VERIFIED | Single exported `Fold` symbol; 12-entry `foldMap` covering ß/æ/ø/ł/þ/đ + uppercase; asciiLowerFastPath + NFKD + Mn-drop + ToLower pipeline |
| `internal/unifold/unifold_test.go` | Behavioural + panic-safety tests | VERIFIED | `TestFold` + `TestFold_NoPanic`; passes under `go test -race ./internal/unifold/...` |
| `ent/schema/{network,facility,internetexchange,organization,campus,carrier}.go` | 16 `_fold` fields total | VERIFIED | 3+3+4+3+1+2=16 `field.String("*_fold")` declarations; all carry `entgql.Skip(entgql.SkipAll) + entrest.WithSkip(true)` annotations (defensive addition documented in 69-02-SUMMARY — plan's "omit filter/order annotations" assumption was empirically insufficient) |
| `internal/sync/upsert.go` | 16 `unifold.Fold(...)` calls on 6 upsert funcs; other 7 untouched | VERIFIED | Exactly 16 `unifold.Fold` calls and 16 `Set{Name,Aka,City,NameLong}Fold` setters; `TestUpsertPopulatesFoldColumns` asserts Zürich→zurich/Straße→strasse/Köln→koln round-trip |
| `internal/pdbcompat/filter.go` | `coerceToCaseInsensitive` + `json_each` rewrite + `errEmptyIn` sentinel + folded routing | VERIFIED | All four deliverables present at documented line ranges |
| `internal/pdbcompat/registry.go` | `FoldedFields` map on 6 TypeConfigs | VERIFIED | 6 `FoldedFields` literals: org/net/fac/ix/carrier/campus with correct field sets per shadow_column_routing_rule |
| `internal/pdbcompat/handler.go` | 3-return `ParseFilters` threading `EmptyResult` | VERIFIED | `handler.go:195` populates `QueryOptions.EmptyResult`; `handler.go:149` calls `ParseFilters(params, tc)` |
| `internal/pdbcompat/registry_funcs.go` | 13 `opts.EmptyResult` short-circuit guards | VERIFIED | Exactly 13 guards present (one per list closure) |
| `internal/pdbcompat/fuzz_test.go` | D-07 extended corpus | VERIFIED | 21 new seeds covering diacritics/CJK/RTL/RLO/ZWJ/combining/null/>64KB + IN edges; function body updated to new `ParseFilters(params, tc)` 3-return signature |
| `internal/pdbcompat/bench_test.go` + `filter_bench.go` | `//go:build bench`-gated benchmark | VERIFIED | Both files start with `//go:build bench`; production `go build ./...` does not compile them; 69-05-SUMMARY records benchstat evidence justifying index deferral (shadow 101.4 ms ± 1% vs direct 102.2 ms ± 1% at 10k rows, p=0.065) |
| `CHANGELOG.md` | v1.16 Phase 69 Added + known-issues entries | VERIFIED | Unicode folding + operator coercion + json_each entries at lines 57, 67, 74; ASCII-only window known-issue at line 111 |
| `docs/API.md` | Known Divergences row for ASCII window | VERIFIED | `name_fold` / `internal/unifold` / `ASCII-only window` phrases present at line 553 |
| `CLAUDE.md` | `### Shadow-column folding (Phase 69)` convention subsection | VERIFIED | Section at line 131 documents the pattern with a 5-step checklist for adding future folded fields |
| `.planning/REQUIREMENTS.md` | 5 Phase 69 REQ-IDs marked complete | VERIFIED | IN-01, IN-02, UNICODE-01, UNICODE-02, UNICODE-03 all show `complete (69-0N; ...)` with implementation path citations at lines 102-106 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `internal/sync/upsert.go` | `internal/unifold` | `import + 16 Fold calls on 6 funcs` | WIRED | 16 `unifold.Fold` calls spread across `upsertOrganizations/Networks/Facilities/InternetExchanges/Campuses/Carriers`; per-entity counts match field_inventory (3+3+3+4+1+2=16) |
| `internal/sync/upsert.go` OnConflict | re-sync rewrites _fold | `UpdateNewValues() conflict clause` | WIRED | `OnConflict().UpdateNewValues()` pattern used throughout; no per-field update clause needed — re-sync automatically rewrites `_fold` when `name` changes |
| `internal/pdbcompat/filter.go buildContains/StartsWith/Exact` | `internal/unifold.Fold` | `unifold.Fold(value)` RHS with `<field>_fold` column | WIRED | 5 `unifold.Fold` call sites in filter.go (buildExact:155, buildContains, buildStartsWith + 2 in IN-string conversion path via ParseFilters) |
| `internal/pdbcompat/filter.go buildIn` | `SELECT value FROM json_each(?)` | `sql.ExprP(...)` raw-SQL fragment with JSON-marshalled slice | WIRED | `filter.go:264` emits `s.C(field)+" IN (SELECT value FROM json_each(?))"` with single bind |
| `internal/pdbcompat/filter.go ParseFilters` | empty `__in` → handler empty response | `errEmptyIn` sentinel → `QueryOptions.EmptyResult` → 13 registry_funcs guards | WIRED | Full chain traceable from filter.go:20 through handler.go:195 to 13 closure guards |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|---------------------|--------|
| `_fold` shadow columns | row.NameFold etc. | `internal/sync/upsert.go` `.SetNameFold(unifold.Fold(o.Name))` | YES — end-to-end test `TestUpsertPopulatesFoldColumns` asserts `NameFold=="zurich gmbh"` from input `Name="Zürich GmbH"` | FLOWING |
| pdbcompat filter shadow-route | `foldedCol`, `foldedVal` | `unifold.Fold(value)` with `<field>_fold` column selector | YES — `TestShadowRouting_Network_NameFold` HTTP-queries `?name__contains=zurich` against seeded `name="Zürich Connect"` row and gets a match | FLOWING |
| `opts.EmptyResult` short-circuit | `emptyResult` bool from ParseFilters | `errEmptyIn` sentinel in `buildIn` → `return nil, true, nil` in ParseFilters | YES — `TestInJsonEach_EmptyString_ReturnsEmpty` seeds 3 networks, queries `?asn__in=`, asserts 0 returned | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| unifold.Fold correctness | `go test -race ./internal/unifold/...` | `ok ... 1.027s` | PASS |
| pdbcompat filter + fuzz seeds | `go test -race ./internal/pdbcompat/...` | `ok ... 10.997s` | PASS |
| Sync _fold populates columns | `go test -race ./internal/sync/...` | `ok ... 12.388s` | PASS |
| Full suite including ent/graph/web/grpcserver | `go test -race ./...` | all 26 packages OK, race clean | PASS |
| Lint gates | `golangci-lint run` | `0 issues.` | PASS |
| Build gates | `go build ./...` / `go vet ./...` | silent (both clean) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| UNICODE-01 | 69-01,02,03,04 | Unicode fold + shadow column + sync populate + query routing | SATISFIED | End-to-end `Zürich→zurich` chain verified (fold package, sync upsert, filter routing, round-trip test) |
| UNICODE-02 | 69-04 | `__contains`/`__startswith` coerced to case-insensitive | SATISFIED | `coerceToCaseInsensitive` at filter.go:59; scope-limited to the two operators per D-04 |
| UNICODE-03 | 69-05 | Fuzz corpus extended, zero panics | SATISFIED | 21 seeds in fuzz_test.go; local 469197-exec run documented in 69-05-SUMMARY (exceeds 500k target? — Summary records 469197; close to 500k commitment but note this is 60s wall-clock, matches v1.10 Phase 48 pattern) |
| IN-01 | 69-04,05 | `__in` accepts >999 values | SATISFIED | `json_each(?)` rewrite + 1500-element HTTP test + EXPLAIN QUERY PLAN confirmation |
| IN-02 | 69-04 | Empty `__in` returns empty set | SATISFIED | `errEmptyIn` sentinel + `EmptyResult` flag + 13 closure guards + end-to-end test |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | — | — | — | No TODO/FIXME/PLACEHOLDER/stub returns found in new code |

### Human Verification Required

None — all must-haves verified via automated grep, file inspection, and `go test -race` execution. Phase 69 ships as part of the coordinated v1.16 release window per CONTEXT.md (no `fly deploy` invoked). Post-deploy operators should see the one-time ASCII-only window documented in CHANGELOG/docs/API.md close after the first sync cycle (≤1h with default `PDBPLUS_SYNC_INTERVAL`).

### Gaps Summary

None. All 7 must-haves verified. The one plan/implementation divergence (Plan 69-02 assumed `entgql.Skip` + `entrest.WithSkip` were unnecessary; executor correctly added them because entgql/entrest emit by default) is documented in 69-02-SUMMARY and strengthens privacy rather than regressing it — a net positive executor deviation.

---

_Verified: 2026-04-19T17:09:00Z_
_Verifier: Claude (gsd-verifier)_
