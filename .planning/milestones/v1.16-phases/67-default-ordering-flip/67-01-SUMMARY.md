---
phase: 67
plan: 01
subsystem: ent-schemas
tags: [ent-schema, entrest, ordering, index, codegen]
completed_at: 2026-04-19
milestone: v1.16
requires: []
provides: [updated_index_all_13, default_sort_annotation_all_13, generator_template_lockstep]
affects: [ent/schema/*, cmd/pdb-schema-generate, ent/rest/sorting.go, ent/migrate/schema.go, ent/rest/eagerload.go, ent/rest/openapi.json]
tech-stack:
  added: []
  patterns: [generator-template-lockstep, declarative-index-migration]
key-files:
  created: []
  modified:
    - ent/schema/campus.go
    - ent/schema/carrier.go
    - ent/schema/carrierfacility.go
    - ent/schema/facility.go
    - ent/schema/internetexchange.go
    - ent/schema/ixfacility.go
    - ent/schema/ixlan.go
    - ent/schema/ixprefix.go
    - ent/schema/network.go
    - ent/schema/networkfacility.go
    - ent/schema/networkixlan.go
    - ent/schema/organization.go
    - ent/schema/poc.go
    - cmd/pdb-schema-generate/main.go
    - ent/migrate/schema.go
    - ent/rest/sorting.go
    - ent/rest/eagerload.go
    - ent/rest/openapi.json
decisions:
  - D-08 composite updated index implemented via `index.Fields("updated")` on all 13 entities
  - D-02 declarative entrest default sort implemented per-schema (compound ordering deferred to Plan 02 TemplateDir override per D-07)
  - WithSortable(true) on the updated field is mandatory per entrest@v1.0.3 schema_sorting.go:80 validation; without it entrest codegen panics
metrics:
  duration: 15m
  tasks: 2
  files: 18
  commit: 0e4012f
requirements:
  - ORDER-03
---

# Phase 67 Plan 01: Default-ordering declarative foundation Summary

## One-liner

Added `updated` ent index + entrest default-sort annotations + generator-template lock-step edits across all 13 PeeringDB entities, establishing the declarative foundation for Phase 67's default-ordering flip.

## What shipped

Declarative-only changes — no runtime query path was modified by this plan. Three composable edits, applied atomically across 14 files (13 `ent/schema/*.go` + `cmd/pdb-schema-generate/main.go`):

1. **`index.Fields("updated")`** appended to `Indexes()` on all 13 schemas (CONTEXT D-08). Composite `(updated, id)` via PK-backed ordering — enables index-scan `ORDER BY updated DESC, id DESC`. Plan 71 memory-budget sizing depends on this.
2. **`entrest.WithDefaultSort("updated")`** + **`entrest.WithDefaultOrder(entrest.OrderDesc)`** appended to `Annotations()` on all 13 schemas (CONTEXT D-02, updated 2026-04-19). Declares the entrest single-field default for `?sort=` override semantics. Compound tie-break `(-updated, -created, -id)` is delivered by Plan 02's TemplateDir override per D-07.
3. **`entrest.WithSortable(true)`** appended to the `updated` field annotation list on all 13 schemas. Required by `entrest@v1.0.3` `schema_sorting.go:80` validation — without it, entrest codegen panics.

The generator template at `cmd/pdb-schema-generate/main.go` was updated in three emitter sites in lock-step so `go generate ./schema` produces the same output byte-for-byte (verified idempotent via a second regen run):

- `generateIndexes()` now appends `"updated"` unconditionally (new 3-line hunk with D-08 comment).
- Template `Annotations()` block emits `WithDefaultSort` + `WithDefaultOrder` after the existing three entries.
- Template `field.Time("updated")` emits a multi-line annotation chain with `WithSortable(true)` alongside the existing filter annotation.

## Task record

### Task 1 — Edit 13 schemas + generator template (commit 0e4012f)

**Part A — hand-edits.** Applied three edits per file × 13 files = 39 hunks. All files preserved the existing `package schema` imports (entrest + schema + index) — no new imports required. The `updated` field's annotation list was rewritten from single-line form (`Annotations(entrest.WithFilter(...))`) to multi-line form so two annotations could be listed cleanly; gofmt canonicalises the formatting.

**Part B — generator template.** Edited `cmd/pdb-schema-generate/main.go`:

- Line 575 region (`generateIndexes`): appended `indexes = append(indexes, "updated")` with D-08 comment. Sits alongside the existing `status` append so the `slices.Sort(indexes)` + dedup downstream keeps ordering deterministic.
- Line 668 region (schema template `field.Time("updated")`): changed from single-line to multi-line `Annotations(...)` containing both the filter annotation and `entrest.WithSortable(true)`.
- Line 706 region (schema template `Annotations()`): added `entrest.WithDefaultSort("updated"),` and `entrest.WithDefaultOrder(entrest.OrderDesc),` after `WithIncludeOperations`.

**Verification (Task 1 `<verify>` acceptance):**
- `idx=13` `sort=13` `ord=13` `sortable=13` — all 13 schema files carry the three edits.
- `tmpl=3` — generator template has all three emitter changes (grep for `WithDefaultSort|WithSortable|indexes, "updated"`).
- `go build ./...` passes cleanly.
- No new imports required (entrest/schema/index already imported in every schema file).

### Task 2 — Regenerate + verify lock-step (commit 0e4012f)

**Steps executed:**
1. `go generate ./schema` — regenerates all 13 ent/schema/*.go files from the updated generator template. Observed output overwrote the hand-edits, but byte-for-byte equal so no effective diff. **Idempotence verified**: second `go generate ./schema` run produced zero diff against the first run's output (`diff` on `campus.go` and `network.go` both empty).
2. `go generate ./ent` — regenerates ent + entgql + entrest + entproto + buf proto Go types. Output:
   - `ent/rest/sorting.go`: all 13 `*SortConfig` blocks now have `DefaultField: "updated"` and `DefaultOrder: "desc"` (was `"id"` / `"asc"`).
   - `ent/migrate/schema.go`: added 13 `<entity>_updated` index entries (`campus_updated`, `carrier_updated`, `carrierfacility_updated`, `facility_updated`, `internetexchange_updated`, `ixfacility_updated`, `ixlan_updated`, `ixprefix_updated`, `network_updated`, `networkfacility_updated`, `networkixlan_updated`, `organization_updated`, `poc_updated`).
   - `ent/rest/eagerload.go`: nested eager-load helpers now apply `applySorting<Type>(e, "updated", "desc")` for every edge that uses entrest's auto-eagerload. This is the behaviour CONTEXT D-04 clarification anticipates: nested `_set` arrays at `depth>=1` will now sort descending by `updated` in `/rest/v1/<type>?depth=N` responses, matching upstream Django. Plan 06 will assert this with a cross-surface test.
   - `ent/rest/openapi.json`: regenerated OpenAPI spec with new defaults.
3. `go build ./...` — passes.
4. `go vet ./...` — passes (bonus check beyond plan's acceptance criteria).
5. `go test -race ./ent/...` — passes (`ent/schema` tests in 1.481s with race detector; all other ent packages have no tests which is expected for generated code).

**Verification (Task 2 `<verify>` acceptance):**
- `sorting=13` entrest `DefaultField: "updated"` SortConfig blocks.
- `migrate=13` exact `<entity>_updated` index names in `ent/migrate/schema.go` (grep `Name: +"[a-z]+_updated",$`).
- `go build ./...` + `go test -race ./ent/...` pass.

## Files changed

### Ent schemas (13)
`ent/schema/campus.go`, `ent/schema/carrier.go`, `ent/schema/carrierfacility.go`, `ent/schema/facility.go`, `ent/schema/internetexchange.go`, `ent/schema/ixfacility.go`, `ent/schema/ixlan.go`, `ent/schema/ixprefix.go`, `ent/schema/network.go`, `ent/schema/networkfacility.go`, `ent/schema/networkixlan.go`, `ent/schema/organization.go`, `ent/schema/poc.go` — each gained three hunks: `Indexes()` + `Annotations()` + `field.Time("updated")` annotations.

### Generator (1)
`cmd/pdb-schema-generate/main.go` — three emitter sites updated to keep the regenerated schemas lock-step with hand-edits.

### Regen output (4)
`ent/rest/sorting.go`, `ent/migrate/schema.go`, `ent/rest/eagerload.go`, `ent/rest/openapi.json`.

Total: **18 files changed, 423 insertions(+), 143 deletions(-)** — all in commit `0e4012f`.

## Acceptance criteria

| ID | Criterion | Status | Evidence |
|----|-----------|--------|----------|
| T1-1 | All 13 schemas declare `index.Fields("updated")` | PASS | `idx=13` (grep-counted) |
| T1-2 | All 13 schemas declare `WithDefaultSort("updated")` + `WithDefaultOrder(OrderDesc)` | PASS | `sort=13 ord=13` |
| T1-3 | All 13 schemas declare `WithSortable(true)` on `updated` field | PASS | `sortable=13` |
| T1-4 | Generator template emits all three edits (lock-step) | PASS | `tmpl=3` (three distinct emitter lines: `575 indexes, "updated"`; `668 WithSortable(true)`; `706 WithDefaultSort`) |
| T1-5 | `go build ./...` passes | PASS | Build succeeded silently |
| T1-6 | No new imports required | PASS | grep `import` before/after — zero net additions in schema files |
| T2-1 | `go generate ./schema` + idempotence check | PASS | Two consecutive runs produced zero diff (verified by `diff` on `campus.go`/`network.go`) |
| T2-2 | `go generate ./ent` succeeds | PASS | Ran cleanly, produced expected regen output |
| T2-3 | `grep -c 'DefaultField: "updated"' ent/rest/sorting.go` = 13 | PASS | `sorting=13` |
| T2-4 | `ent/migrate/schema.go` has ≥13 `_updated` index entries | PASS | `migrate=13` exact-match on `Name: "<entity>_updated",` |
| T2-5 | `go build ./...` passes | PASS | Build clean |
| T2-6 | `go test -race ./ent/...` passes | PASS | `ok ent/schema 1.481s`, all others no-test-files |

**Success criteria (plan-level):**

| ID | Criterion | Status |
|----|-----------|--------|
| S-1 | 13 schema files + generator template + regen committed; 1 atomic commit | PASS (commit `0e4012f`) |
| S-2 | `/rest/v1/<type>?sort=id&order=asc` still honoured (override semantics) | DEFERRED — asserted by Plan 06 cross-surface test; this plan is declarative only, runtime behaviour not exercised |
| S-3 | Compound `(-updated, -created, -id)` NOT delivered by this plan | PASS — Plan 02 delivers it per D-07 (no over-scope here) |
| S-4 | Runtime: `CREATE INDEX` applied on next `fly deploy` via `migrate.WithDropIndex(true)` | DEFERRED — verified by post-deploy inspection (OBS-04 path); this plan only regenerates `ent/migrate/schema.go` to contain the declarations |

## Deviations from plan

None. All three hand-edit steps were applied as specified and the generator template was updated in lock-step before the first `go generate ./schema`. Idempotence was verified on the first attempt, so no iteration on the template was needed.

Minor-note-only observation: `ent/rest/eagerload.go` picked up 128 lines of regen delta that weren't explicitly enumerated in the plan's expected-artifacts list. This is the direct consequence of CONTEXT.md D-04 clarification ("nested `_set` arrays at `depth>=1` will reorder under the new default") — entrest's eager-load template at `templates/eagerload.tmpl:29` calls `applySorting<Type>` for every auto-eagerloaded edge, so when the sorting defaults flip, eager-load helpers regenerate too. This is expected and desired behaviour per D-04. Flagging here so the verifier doesn't treat it as surprise.

## Signals for the phase verifier

Quick grep commands that prove the work is done:

```bash
# 1. All 13 schemas carry the three edits (expect 13 for each count)
grep -c 'index\.Fields("updated")' ent/schema/{campus,carrier,carrierfacility,facility,internetexchange,ixfacility,ixlan,ixprefix,network,networkfacility,networkixlan,organization,poc}.go | awk -F: '{sum+=$2} END {print sum}'
# → 13

grep -c 'entrest\.WithDefaultSort("updated")' ent/schema/{campus,carrier,carrierfacility,facility,internetexchange,ixfacility,ixlan,ixprefix,network,networkfacility,networkixlan,organization,poc}.go | awk -F: '{sum+=$2} END {print sum}'
# → 13

grep -c 'entrest\.WithDefaultOrder(entrest\.OrderDesc)' ent/schema/{campus,carrier,carrierfacility,facility,internetexchange,ixfacility,ixlan,ixprefix,network,networkfacility,networkixlan,organization,poc}.go | awk -F: '{sum+=$2} END {print sum}'
# → 13

grep -c 'entrest\.WithSortable(true)' ent/schema/{campus,carrier,carrierfacility,facility,internetexchange,ixfacility,ixlan,ixprefix,network,networkfacility,networkixlan,organization,poc}.go | awk -F: '{sum+=$2} END {print sum}'
# → 13

# 2. Generator template has lock-step emitter changes (expect 3)
grep -c 'WithDefaultSort\|WithSortable\|indexes, "updated"' cmd/pdb-schema-generate/main.go
# → 3

# 3. Regenerated entrest sort config (expect 13)
grep -c 'DefaultField: "updated"' ent/rest/sorting.go
# → 13

# 4. Regenerated migrate schema has 13 <entity>_updated index names
grep -cE 'Name: +"[a-z]+_updated",$' ent/migrate/schema.go
# → 13

# 5. Idempotence check — regen must produce zero diff on second run
cp ent/schema/network.go /tmp/a.go && go generate ./schema && diff /tmp/a.go ent/schema/network.go
# → (empty, exit 0)

# 6. Build + vet + ent tests (expect all exit 0)
go build ./...
go vet ./...
go test -race ./ent/... -count=1
```

## Self-Check

Verified files and commit exist:

- `ent/schema/network.go` — FOUND, contains `index.Fields("updated")`, `entrest.WithDefaultSort("updated")`, `entrest.WithDefaultOrder(entrest.OrderDesc)`, `entrest.WithSortable(true)`.
- `cmd/pdb-schema-generate/main.go` — FOUND, contains `WithDefaultSort`, `WithSortable(true)`, and `indexes, "updated"`.
- `ent/migrate/schema.go` — FOUND, contains 13 `<entity>_updated` index names.
- `ent/rest/sorting.go` — FOUND, contains 13 `DefaultField: "updated"` entries.
- Commit `0e4012f` — FOUND in `git log --oneline -1`.

## Self-Check: PASSED
