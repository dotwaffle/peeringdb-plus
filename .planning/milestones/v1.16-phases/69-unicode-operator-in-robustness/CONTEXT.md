---
phase: 69
slug: unicode-operator-in-robustness
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 69 Context: Unicode folding, operator coercion, `__in` robustness

## Goal

pdbcompat filter layer reproduces upstream value-handling: diacritic folding on both sides (query value + DB field) via shadow columns, case-insensitive operator coercion, and arbitrarily-large `__in` lists via `json_each` single-bind rewrite.

## Requirements

- **UNICODE-01** — filter values Unicode-folded before SQL; ASCII queries match non-ASCII DB rows
- **UNICODE-02** — `__contains`/`__startswith` coerced to case-insensitive variants
- **UNICODE-03** — non-ASCII fuzz corpus, zero panics
- **IN-01** — `__in` accepts >999 values
- **IN-02** — empty `__in` returns empty set

## Locked decisions

- **D-01 — Unicode folding strategy: shadow columns**: Searchable text fields get `_fold` sibling columns populated at sync time with NFKD-normalised + ASCII-folded lowercase. Queries hit the folded column via `WHERE <col>_fold LIKE '%' || ? || '%'` with the folded query value. Fields receiving shadow columns (initial scope): `network.{name, aka, name_long}`, `facility.{name, aka, city}`, `ix.{name, aka, name_long, city}`, `organization.{name, aka, city}`, `campus.name`, `carrier.{name, aka}`. Total ~18 shadow columns across 6 entities.
- **D-02 — Unicode library**: `golang.org/x/text/unicode/norm` (NFKD normalisation) + hand-rolled fold map for non-decomposable diacritics (ß→ss, æ→ae, ø→o, ł→l, þ→th, đ→d etc.). Single package `internal/unifold` exposes `Fold(s string) string`. Zero third-party deps. `golang.org/x/text` is already an indirect dep via several packages so no new module is added.
- **D-03 — Backfill strategy**: ent auto-migrate adds `_fold` columns as `TEXT DEFAULT ''`. First post-Phase-69 sync populates them via standard upsert (sync worker calls `unifold.Fold()` when building `Save()`/`Update()` calls). No one-shot backfill script — the next sync cycle (within 1h of deploy) covers all rows. Pre-backfill, queries hit `_fold=''` empty values and return no matches for non-ASCII queries; ASCII queries keep working via the existing non-folded fields. Document the brief divergence window in CHANGELOG.
- **D-04 — Operator coercion scope**: Only `__contains → __icontains` and `__startswith → __istartswith` per `rest.py:638-641`. `__exact`, `__iexact`, `__gt`, `__lt`, etc. keep their existing semantics. `ParseFilters` gets a `coerceToCaseInsensitive(op)` helper.
- **D-05 — `__in` `json_each` rewrite**: `WHERE <field> IN (SELECT value FROM json_each(?))` with the comma-separated input marshalled into a JSON array (`[1,2,3,...]`) and passed as a single parameter. Works for both int and string `__in`. Bypasses SQLite's 999-variable limit. Verified via `EXPLAIN QUERY PLAN` in one test to confirm no fallback to parameter-expansion on modernc.org/sqlite.
- **D-06 — Empty `__in` short-circuit**: In `ParseFilters`, detect `len(values) == 0` after comma-split and early-return an "empty result" sentinel that the handler translates to `{"data":[], "meta":{"count":0}}` without running any SQL. Matches Django ORM `Model.objects.filter(id__in=[])` returning empty QuerySet.
- **D-07 — Fuzz corpus**: `FuzzParseFilters` in `internal/pdbcompat/filter_fuzz_test.go` seeds with: ASCII strings, diacritics (áéíóúñçàèì), CJK (日本語中文한글), combining marks (e\u0301, a\u0308), ZWJ sequences (emoji families), RTL (עברית), right-to-left overrides, null bytes, long strings (>64k). Target: 500k executions without panic/deadlock. Runs in CI on PR as existing fuzz pattern (v1.10 Phase 48).

## Out of scope

- Shadow columns for rarely-searched text fields (notes, policy_url, info_type) — add per-need in a future phase if grep hit-rates surface the gap.
- GraphQL / entrest filter behaviour — both already use ent-level NOCASE / FieldContainsFold. Not in scope for v1.16.
- Shadow-column index strategy — ent auto-migrate adds no index on `_fold` columns by default; Phase 69 plan will decide whether to add `@index(..._fold)` annotations based on benchmark results.

## Dependencies

- **Depends on**: Phase 68 (shared `internal/pdbcompat/filter.go`; serialising order avoids merge pain; status matrix builds a table of pending/ok/deleted which the `__in` tests exercise)
- **Enables**: Phase 70 (cross-entity traversal uses the same operator-coercion and folding helpers)

## Plan hints for executor

- Touchpoints: `internal/pdbcompat/filter.go`, new `internal/unifold/` package, `ent/schema/{network,facility,ix,organization,campus,carrier}.go` (add 18 `field.String("*_fold")` declarations), `internal/sync/upsert.go` (13 upsert funcs call `unifold.Fold()` for affected fields), `internal/pdbcompat/filter_fuzz_test.go`.
- ent codegen regeneration after schema edits via `go generate ./...`.
- Measure: baseline `?name__contains=X` latency at 100 rows and 10k rows both before and after (`internal/pdbcompat/bench_test.go`). Shadow column should be no slower than current NOCASE LIKE.

## References

- ROADMAP.md Phase 69
- REQUIREMENTS.md IN-01..02, UNICODE-01..03
- Upstream: `rest.py:576` (`unidecode.unidecode(v)`), `rest.py:638-641` (operator coercion), `rest.py:644-646` (`__in` split)
- `golang.org/x/text/unicode/norm` docs (NFKD semantics)
- SQLite `json_each` docs (https://sqlite.org/json1.html#jeach)
