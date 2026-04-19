---
phase: 69
plan: 02
subsystem: ent-schema
tags:
  - ent-schema
  - codegen
  - shadow-columns
  - unicode
dependency-graph:
  requires:
    - 69-01 (internal/unifold package — provides Fold() called by Plan 69-03 sync upserts)
  provides:
    - 16 ent shadow columns (`*_fold`) on 6 entities — internal-only plumbing
    - Per-entity ent builders gain SetNameFold / SetAkaFold / SetNameLongFold / SetCityFold setters
    - ent migrate descriptor knows the new TEXT DEFAULT '' columns (auto-migrate at next deploy)
  affects:
    - Plan 69-03 (sync upserts call SetXxxFold via internal/unifold.Fold)
    - Plan 69-04 (pdbcompat filter routes WHERE clauses to *_fold columns)
tech-stack:
  added: []
  patterns:
    - "entgql.Skip(entgql.SkipAll) + entrest.WithSkip(true) annotation pair to declare server-side-only ent fields that never leak to GraphQL/REST/proto wire surfaces"
    - "Scoped go generate (`./ent/...` + `./internal/web/templates/...` only, NOT `./...`) to preserve hand-edited entgql/entrest annotations from being stripped by cmd/pdb-schema-generate"
key-files:
  created:
    - .planning/phases/69-unicode-operator-in-robustness/69-02-SUMMARY.md
  modified:
    - ent/schema/network.go (3 new fields)
    - ent/schema/facility.go (3 new fields)
    - ent/schema/internetexchange.go (4 new fields)
    - ent/schema/organization.go (3 new fields)
    - ent/schema/campus.go (1 new field)
    - ent/schema/carrier.go (2 new fields)
    - ent/migrate/schema.go (16 new column declarations)
    - ent/mutation.go (16 new mutation accessors)
    - ent/runtime/runtime.go (default value bindings)
    - ent/{network,facility,internetexchange,organization,campus,carrier}.go (struct fields)
    - ent/{network,facility,internetexchange,organization,campus,carrier}/{*}.go (predicate funcs in per-entity packages)
    - ent/{network,facility,internetexchange,organization,campus,carrier}/where.go (where predicates)
    - ent/{network,facility,internetexchange,organization,campus,carrier}_create.go (SetXxxFold setters)
    - ent/{network,facility,internetexchange,organization,campus,carrier}_update.go (SetXxxFold setters)
    - internal/sync/testdata/refactor_parity.golden.json (regenerated to absorb 16 new "" defaults)
decisions:
  - "Honor plan truth #5 (no surface leakage) via explicit entgql.Skip(SkipAll) + entrest.WithSkip(true) annotations rather than the plan's claim that omitting WithFilter/OrderField was sufficient — empirically that does NOT suppress emission; entgql/entrest emit by default"
  - "Scoped codegen instead of `go generate ./...` because the latter recursively runs schema/generate.go which strips hand-edited annotations per CLAUDE.md § Code Generation"
  - "Field-inventory tight count: 16 (D-01 said ~18, see reconciliation note below)"
metrics:
  duration: 8m43s
  completed: 2026-04-19
---

# Phase 69 Plan 02: Add 16 *_fold shadow columns to 6 ent schemas

**One-liner:** Added 16 internal-only `*_fold` shadow columns across 6 ent schemas (network, facility, internetexchange, organization, campus, carrier) with `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` annotations to keep them off GraphQL/REST/proto wire surfaces, then regenerated the ent builder tree via scoped codegen so Plans 69-03 (populate via sync) and 69-04 (route filter queries) have setters to call.

## Field Inventory (16 vs "~18" Reconciliation)

CONTEXT.md D-01 phrased the shadow-column count as "~18 shadow columns across 6 entities." The tight count derived from D-01's per-entity field enumeration is exactly 16. The plan opened with this reconciliation in its `<rationale>` block; this SUMMARY records the resolution as locked-in:

| Entity            | Fields Added                                            | Count |
|-------------------|---------------------------------------------------------|-------|
| network           | name_fold, aka_fold, name_long_fold                     | 3     |
| facility          | name_fold, aka_fold, city_fold                          | 3     |
| internetexchange  | name_fold, aka_fold, name_long_fold, city_fold          | 4     |
| organization      | name_fold, aka_fold, city_fold                          | 3     |
| campus            | name_fold                                               | 1     |
| carrier           | name_fold, aka_fold                                     | 2     |
| **Total**         |                                                         | **16** |

The "~18" tilde in D-01 was approximation; the inventory locked at 16 here per CONTEXT.md § Out of scope bullet 1 (which deferred adding `name_long_fold` to facility/organization/carrier to a future phase if grep-hit-rate audits surface the gap).

## Surface Hygiene

All 16 fields carry both:

```go
.Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true))
```

Verification post-codegen confirmed zero `_fold` field exposure on any wire surface:

- `proto/peeringdb/v1/v1.proto` — unchanged (entproto.SkipGenFile in ent/entc.go is active per v1.6 D-01)
- `gen/peeringdb/v1/*.go` — unchanged (no proto regen → no Go proto regen)
- `ent/rest/openapi.json` — no `_fold` keys in any object schema
- `graph/schema.graphqls` — no `nameFold`/`akaFold`/`nameLongFold`/`cityFold` types or filters
- `ent/gql_collection.go` — no `*Fold` collection paths
- `ent/gql_where_input.go` — no `*Fold` field predicates (the pre-existing `EqualFold`/`ContainsFold` operator predicates on existing fields are unrelated and untouched)

The proof: `git status --short` after codegen showed ZERO modifications to `proto/`, `gen/`, `graph/`, `ent/rest/`, `ent/gql_*` — only the per-entity `ent/` builder/predicate tree + `migrate/schema.go` + `mutation.go` + `runtime/runtime.go` plus the 6 hand-edited schemas + the regenerated sync golden.

## Verification Gates

| Gate                              | Result | Notes                                                              |
|-----------------------------------|--------|--------------------------------------------------------------------|
| `go build ./ent/schema/...`       | PASS   | After hand-edits, before codegen                                   |
| `go generate ./ent/...`           | PASS   | entc + buf clean                                                   |
| `go generate ./internal/web/templates/...` | PASS   | templ clean (updates=0)                                       |
| `go build ./...`                  | PASS   | Full tree rebuild after regen                                      |
| `go vet ./...`                    | PASS   | Zero issues                                                        |
| `go test -race -count=1 ./...`    | PASS   | All 27 packages green after sync golden refresh                    |
| `golangci-lint run`               | PASS   | 0 issues                                                           |
| Proto/gen drift check             | PASS   | `git diff proto/ gen/` empty                                       |
| Surface leakage check             | PASS   | Zero `_fold`/`*Fold` matches in REST/GraphQL output files          |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking issue] Plan recipe `go generate ./...` is destructive in this repo**

- **Found during:** Task 2 (codegen)
- **Issue:** The plan's `<verify><automated>` line and `<action>` block both prescribe `go generate ./...`. But `./...` recursively walks subdirectories alphabetically, including `./schema`, whose `schema/generate.go` invokes `cmd/pdb-schema-generate` to regenerate ent/schema/*.go from peeringdb.json — and the generator strips ALL hand-edited annotations (entgql/entrest/entproto) per the explicit warning in CLAUDE.md § Code Generation: "Do NOT run `go generate ./schema` after entproto annotations are added — the schema generator strips entproto annotations." The plan's `<codegen_rules>` block correctly warned about this, but the verify recipe contradicts it. Empirically confirmed when the first `go generate ./...` invocation completed and re-stripped my freshly added 16 `_fold` declarations, leaving zero hits in `ent/schema/*.go` despite the codegen having run successfully on the tree before the strip.
- **Fix:** Switched to scoped codegen — `go generate ./ent/... && go generate ./internal/web/templates/...`. This covers the two pipeline steps documented in CLAUDE.md (1: ent+buf via ent/generate.go, 2: templ via internal/web/templates/generate.go) and deliberately omits step 3 (schema regen) because hand-edits would be destroyed.
- **Files modified:** none (procedural fix, applied during execution)
- **Commit:** 9e408de (commit body documents the choice)

**2. [Rule 2 — Missing critical functionality] Plan annotation guidance under-specified surface suppression**

- **Found during:** Task 2 verification (after first codegen attempt)
- **Issue:** The plan's Task 1 `<action>` block explicitly states: "DO NOT attach any of these to `_fold` fields: `entgql.Skip(...)` / `entproto.Field(...)` — NOT needed because entgql emits all non-annotated fields by default AND proto emission is frozen via `entproto.SkipGenFile`." The first half of that claim is empirically wrong: entgql DOES emit all non-annotated fields by default, which means without `entgql.Skip` the `_fold` columns become BOTH readable GraphQL fields AND filter predicates (`nameFold`, `nameFoldIn`, `nameFoldContains`, etc.) — directly contradicting the plan's truth #5 ("No `_fold` field carries entrest/entgql/entproto filter or sort annotations — they are internal plumbing, not surface fields"). Same problem for entrest: without `entrest.WithSkip(true)`, the `_fold` column appears as a readable property in every REST response payload schema.
- **Fix:** Added `Annotations(entgql.Skip(entgql.SkipAll), entrest.WithSkip(true))` to all 16 declarations. The plan's INTENT (no leakage) is preserved; the IMPLEMENTATION needed an annotation pair the plan didn't account for. Verified post-codegen that zero `_fold` references appear in `ent/rest/openapi.json`, `graph/schema.graphqls`, `ent/gql_where_input.go`, or `ent/gql_collection.go`.
- **Files modified:** All 6 `ent/schema/*.go` files (16 annotations added, one per `_fold` declaration)
- **Commit:** 9e408de

**3. [Rule 3 — Blocking issue] Sync parity golden needed regeneration after schema delta**

- **Found during:** Task 2 verification (`go test -race ./internal/sync/...`)
- **Issue:** `TestSync_RefactorParity` snapshots the JSON shape of a full sync round-trip. The 16 new `_fold` columns each default to `""`, which adds bytes to the snapshot (15171 → 15940 bytes, +769 bytes). The test framework already provides `-update` to refresh the golden — this is the established v1.15 Phase 63 schema-hygiene pattern documented in CLAUDE.md § Schema hygiene drops.
- **Fix:** `cd internal/sync && go test -update -run TestSync_RefactorParity` — golden regenerated, test passes.
- **Files modified:** `internal/sync/testdata/refactor_parity.golden.json`
- **Commit:** 9e408de

### Process Hazard Note (no behavioural impact)

During the deviation-1 detection cycle I ran `git checkout -- ent/ graph/` to revert the destructive `./...` regen output. That command also reverted my hand-edits in `ent/schema/*.go` (because `ent/schema` is under `ent/`). I had to re-apply the 16 declarations a second time. **This was a near-violation of the destructive-git-prohibition rule** ("Never use blanket reset or clean operations that affect the entire working tree"). The correct alternative would have been per-file checkouts of only the regen tree (`ent/network.go`, `ent/network/`, `ent/network_create.go`, etc.) leaving `ent/schema/*.go` untouched, or to reapply the schema edits THEN do the scoped codegen which would naturally overwrite the destructive regen output. Recording here so a future audit can grep the SUMMARY and the orchestrator/user can flag the pattern in CLAUDE.md or executor rules if desired.

## Authentication Gates

None. This plan operated entirely on local files and codegen — no external services, no auth flows.

## Files Touched (commit 9e408de)

40 files changed, 5181 insertions(+), 24 deletions(-):

- 6 hand-edited schemas (`ent/schema/{network,facility,internetexchange,organization,campus,carrier}.go`)
- 33 regenerated ent files:
  - 6 entity main files (`ent/{network,facility,internetexchange,organization,campus,carrier}.go`)
  - 6 entity package main files (`ent/{network,...}/{network,...}.go`)
  - 6 entity package where files (`ent/{network,...}/where.go`)
  - 6 entity create builders (`ent/{network,...}_create.go`)
  - 6 entity update builders (`ent/{network,...}_update.go`)
  - `ent/migrate/schema.go` — new column declarations
  - `ent/mutation.go` — mutation accessors
  - `ent/runtime/runtime.go` — runtime defaults
- 1 regenerated sync golden (`internal/sync/testdata/refactor_parity.golden.json`)

## Self-Check: PASSED

- ent/schema/network.go: FOUND (3 _fold declarations confirmed via grep)
- ent/schema/facility.go: FOUND (3)
- ent/schema/internetexchange.go: FOUND (4)
- ent/schema/organization.go: FOUND (3)
- ent/schema/campus.go: FOUND (1)
- ent/schema/carrier.go: FOUND (2)
- Sum: 16/16
- Annotation symmetry: 16 `entgql.Skip(entgql.SkipAll), entrest.WithSkip(true)` pairs found
- ent/migrate/schema.go: FOUND (16 _fold column references)
- Setters: SetNameFold/SetAkaFold/SetCityFold/SetNameLongFold present in all 6 *_create.go builders (39+39+52+39+13+26 = 208 hits across the create builders alone)
- Commit hash 9e408de: FOUND in `git log --oneline -3`
- Surface leakage: 0 `_fold` references in `ent/rest/openapi.json`, `graph/schema.graphqls`, `ent/gql_collection.go`, `ent/gql_where_input.go`
- Proto drift: `git diff proto/ gen/` empty (SkipGenFile holds)

## Forward Pointers

- **Plan 69-03** consumes the regenerated setters: `internal/sync/upsert.go` (6 of 13 upsertX functions) calls `SetNameFold(unifold.Fold(name))` etc.
- **Plan 69-04** consumes the column NAMES: `internal/pdbcompat/filter.go` rewrites `WHERE name LIKE '%X%'` → `WHERE name_fold LIKE '%fold(X)%'`.
- **Plan 69-05** decides whether to add `@index(name_fold)` based on benchmark results (CONTEXT.md § Out of scope bullet 3 explicitly defers the index decision).
- **Plan 69-06** documents the brief ASCII-only divergence window in CHANGELOG and `docs/API.md` (D-03).

ent auto-migrate handles the schema delta on next prod deploy: 16 `ALTER TABLE ... ADD COLUMN <name>_fold TEXT DEFAULT ''` statements execute on primary startup; LiteFS replicates the schema change to all replica regions transparently.
