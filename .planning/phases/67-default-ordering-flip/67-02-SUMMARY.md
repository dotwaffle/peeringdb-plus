---
phase: 67
plan: 02
subsystem: ent-codegen
tags: [ent-codegen, entrest, template-override, ordering, compound-tie-break]
completed_at: 2026-04-19
milestone: v1.16
requires: [67-01]
provides: [entrest_compound_default_orderby, entrest_template_override_wiring]
affects: [ent/templates/entrest-sorting/*, ent/entc.go, ent/rest/sorting.go]
tech-stack:
  added: []
  patterns: [project-local-template-override, entrest-funcmap-wiring]
key-files:
  created:
    - ent/templates/entrest-sorting/sorting.tmpl
  modified:
    - ent/entc.go
    - ent/rest/sorting.go
decisions:
  - D-07 template override implemented via custom entc.Option instead of entc.TemplateDir because the upstream sorting.tmpl references entrest-provided template funcs (getAnnotation, getSortableFields) that ent's default NewTemplate funcmap does not include
  - Helper fn entrestSortingOverride() lives alongside main() in ent/entc.go (the generator is a single //go:build ignore file — no scope for a separate helper package)
metrics:
  duration: 22m
  tasks: 3
  files: 3
  commit: fbf0e60
requirements:
  - ORDER-03
---

# Phase 67 Plan 02: entrest Template Override for Compound Default ORDER BY Summary

## One-liner

Added a project-local override of entrest@v1.0.3's `sorting.tmpl` that injects `(FieldCreated, FieldID)` tie-breakers into all 13 `applySorting<Type>` helpers when the requested sort field matches the entity's declared `DefaultField` ("updated"), wiring it through a custom `entc.Option` that registers entrest's funcmap on the parse-time template — the only path that makes the entrest template-DSL functions resolve inside our in-tree override.

## What shipped

Three atomic changes committed together in `fbf0e60`:

1. **`ent/templates/entrest-sorting/sorting.tmpl`** (new, 184 lines).  Byte-for-byte copy of the upstream `github.com/lrstanley/entrest@v1.0.3/templates/sorting.tmpl` (163 lines including trailing newline) with two additions:
   - 8-line `PROJECT-LOCAL OVERRIDE` header block at the top (above the upstream copyright comment) documenting the change and the upgrade path.
   - 13-line compound tie-break injection inside the `applySorting<Type>` range (lines 167-180 of the new file), placed AFTER the `random` branch and BEFORE the upstream fallback `return`. Gated by `{{- if $defaultSort }}` so it's a no-op when a schema has no DefaultSort — safety net, but every one of our 13 entities has one after Plan 01.

2. **`ent/entc.go`** (modified, +21/-0 lines net).  Added `entrestSortingOverride(path)` helper — a minimal replica of `entc.TemplateDir` that calls `gen.NewTemplate("entrest-override").Funcs(entrest.FuncMaps())` BEFORE `ParseDir`.  Appended to `opts` in `main()` with an inline explanation comment.  Also added `"fmt"` to the import block for the error-wrap call.

3. **`ent/rest/sorting.go`** (regenerated, +143/-0 lines).  Each of the 13 `applySorting<Type>` funcs gained an 11-line `if _field == "updated" { ... }` block (+ a 4-line explanatory comment) immediately before the existing `return _query.Order(withFieldSelector(_field, _order))` line.  SortConfig definitions, edge-sort switches, and the `random` branch are unchanged.

Example (`applySortingNetwork`, lines 640-651 of the regenerated file):

```go
// Phase 67 compound-default tie-break: when the requested sort field is the
// entity's declared default, append (FieldCreated, FieldID) tie-breakers in the
// same order direction for parity with pdbcompat and grpcserver. See
// .planning/phases/67-default-ordering-flip/CONTEXT.md D-02 / D-07 and D-04 clarification.
if _field == "updated" {
    return _query.Order(
        withFieldSelector(_field, _order),
        withFieldSelector(network.FieldCreated, _order),
        withFieldSelector(network.FieldID, _order),
    )
}
return _query.Order(withFieldSelector(_field, _order))
```

## Task record

### Task 1 — Copy upstream sorting.tmpl + inject compound tie-break (commit `fbf0e60`)

Copied `$(go env GOMODCACHE)/github.com/lrstanley/entrest@v1.0.3/templates/sorting.tmpl` verbatim into `ent/templates/entrest-sorting/sorting.tmpl`, then:

- Prepended the 8-line `PROJECT-LOCAL OVERRIDE` block documenting base version + upgrade path.
- Located the `applySorting<Type>` range block (upstream L116-161) and replaced the final 4 lines (`if _field == "random"` → fallback `return`) with the upstream verbatim `random` branch plus a newly-inserted `{{- if $defaultSort }}` block that expands per-entity to the compound-order `if _field == "updated"` Go code.
- Used `{{ $defaultSort | quote }}` for the string-literal comparison and `{{ $t.Package }}.FieldCreated` / `.FieldID` for the package-qualified Go constants — both constants verified to exist in the generated entity packages (e.g. `ent/network/network.go:91,93`).

Everything outside the `applySorting<Type>` body (the `Sorted` struct, `Validate`, `withOrderTerm`, `withFieldSelector`, the `SortConfig` range, `isSpecializedSort`, the outer `define "rest/sorting"` / `end` wrappers) stayed byte-identical to upstream.

Acceptance checks (Task 1 `<verify>`):
- File present ≥150 lines — 184 lines.
- `PROJECT-LOCAL OVERRIDE` header — present.
- `FieldCreated` + `FieldID` lines — present.
- `$defaultSort := ($t|getAnnotation).GetDefaultSort` directive — present.

### Task 2 — Wire the override into ent/entc.go (commit `fbf0e60`)

Initially attempted the plan's literal advice (`entc.TemplateDir("./templates/entrest-sorting")`). First `go generate ./ent` failed fast:

```
template: sorting.tmpl:86: function "getAnnotation" not defined
```

Root cause (verified by reading upstream source):
- `entc.TemplateDir` → internal `templateOption` → `gen.NewTemplate("external")` (`entgo.io/ent@v0.14.6/entc/entc.go:357-366`). `gen.NewTemplate` registers only `gen.Funcs` (the stdlib-ent funcmap — `singular`, `pascal`, `camel`, `snake`, plus a few more). No entrest funcs.
- entrest's `sorting.tmpl` uses `getAnnotation`, `getSortableFields`, and the `zsingular` filter — all registered in entrest's private `funcMap` (`github.com/lrstanley/entrest@v1.0.3/templates.go:15-31`).
- entrest does export the funcmap via `entrest.FuncMaps()` (`templates.go:57`) explicitly for this use case.
- At codegen time, `gen.Graph.templates()` at `graph.go:970` calls `templates.Funcs(rootT.FuncMap)` to merge each extension/external Template's funcmap into the main codegen tree — but this only helps templates that are PARSED at that merge point. `entc.TemplateDir`'s `ParseDir` happens during option evaluation, before the merge, so the funcmap isn't available yet. Parse fails.

Fix: added a small helper `entrestSortingOverride(path string) entc.Option` in `ent/entc.go` that mirrors `templateOption`'s shape but uses `gen.NewTemplate("entrest-override").Funcs(entrest.FuncMaps())` before `ParseDir`. Registered alongside the existing Extensions/FeatureNames options.

Acceptance checks (Task 2 `<verify>`):
- `grep -n 'entrest-sorting' ent/entc.go` — 1 match at line 122 (`entrestSortingOverride("./templates/entrest-sorting")`).
- `go build ./...` — passes.

### Task 3 — Regenerate + verify (commit `fbf0e60`)

`TMPDIR=/tmp/claude-1000 go generate ./ent` produced clean regen output.  `git diff --stat ent/` showed ONLY 2 ent files touched: `entc.go` (my Task 2 edit) and `rest/sorting.go` (regen). +143/-0 in sorting.go — purely additive, confirming no unexpected template-side changes bled into the emit.

Acceptance checks (Task 3 `<verify>`):
- `grep -c 'FieldCreated' ent/rest/sorting.go` = 26 (13 applySorting × 2 lines each — target was ≥13).
- `grep -c 'FieldID' ent/rest/sorting.go` = 26.
- `grep -c 'if _field == "updated"' ent/rest/sorting.go` = 13 (exact match on target).
- `go build ./...` — passes.
- `go test -race ./ent/... -count=1` — passes (ent/schema 1.459s; all others no-test-files — expected for generated code).
- `go test -race ./cmd/peeringdb-plus/... -count=1` — passes (2.574s) — cmd-level integration tests exercise the REST sorting path.
- `go generate ./ent` idempotent — second run produces zero diff against first run's output (`diff /tmp/claude-1000/sorting.go.run1 ent/rest/sorting.go` empty).
- `go generate ./...` (full pipeline — schema + ent + templ + buf) clean — `git status --short` unchanged after the run (only the pre-regen modifications to `.planning/STATE.md`, `ent/entc.go`, `ent/rest/sorting.go` plus my new `ent/templates/` dir remain).

## Files changed

### Template (1 — new)
`ent/templates/entrest-sorting/sorting.tmpl` — 184 lines, project-local override.

### Generator (1 — modified)
`ent/entc.go` — +21/-0 lines. Added `"fmt"` import, new `entrestSortingOverride()` helper, new opts entry.

### Regen output (1 — modified)
`ent/rest/sorting.go` — +143/-0 lines. 13 compound-tie-break blocks injected.

Total: **3 files changed, 363 insertions(+), 0 deletions(-)** — all in commit `fbf0e60`.

## Acceptance criteria

| ID   | Criterion                                                                                         | Status | Evidence                                                                                 |
| ---- | ------------------------------------------------------------------------------------------------- | ------ | ---------------------------------------------------------------------------------------- |
| T1-1 | `ent/templates/entrest-sorting/sorting.tmpl` exists ≥150 lines                                    | PASS   | 184 lines                                                                                |
| T1-2 | Contains `PROJECT-LOCAL OVERRIDE` header with entrest@v1.0.3 reference                            | PASS   | line 3 `Base source: github.com/lrstanley/entrest@v1.0.3/templates/sorting.tmpl`         |
| T1-3 | Contains `$defaultSort := ($t\|getAnnotation).GetDefaultSort (ne $t.ID nil)` directive            | PASS   | line 167                                                                                 |
| T1-4 | Contains `withFieldSelector({{ $t.Package }}.FieldCreated, _order)` + `.FieldID` lines            | PASS   | lines 176-177                                                                            |
| T1-5 | Outer structural template matches upstream verbatim (only addition is the injection block)       | PASS   | `diff` upstream vs ours shows only the header + injection hunks                          |
| T2-1 | `grep 'entrest-sorting' ent/entc.go` returns ≥1 match                                             | PASS   | 1 match on the helper call                                                               |
| T2-2 | `go build ./...` passes                                                                           | PASS   | Clean build                                                                              |
| T3-1 | `grep -c 'FieldCreated' ent/rest/sorting.go` ≥ 13                                                 | PASS   | 26 (13 × 2)                                                                              |
| T3-2 | `grep -c 'FieldID' ent/rest/sorting.go` ≥ 13                                                      | PASS   | 26                                                                                       |
| T3-3 | `grep -c 'if _field == "updated"' ent/rest/sorting.go` = 13                                       | PASS   | exactly 13                                                                               |
| T3-4 | `go build ./...` passes                                                                           | PASS   | Clean                                                                                    |
| T3-5 | `go test -race ./ent/...` passes                                                                  | PASS   | ent/schema 1.459s; all others no-test-files                                              |
| T3-6 | `go test -race ./cmd/peeringdb-plus/...` passes                                                   | PASS   | 2.574s                                                                                   |

**Success criteria (plan-level):**

| ID  | Criterion                                                                                       | Status   |
| --- | ----------------------------------------------------------------------------------------------- | -------- |
| S-1 | Template override committed; regenerated `ent/rest/sorting.go` reflects compound default       | PASS     |
| S-2 | REST list endpoints under the new default emit `ORDER BY updated DESC, created DESC, id DESC`  | DEFERRED — declarative change in codegen; runtime SQL shape asserted by Plan 06 cross-surface E2E |
| S-3 | Explicit `?sort=id&order=asc` still honoured (fallback path unchanged)                          | PASS     — fallback `return _query.Order(withFieldSelector(_field, _order))` is untouched at line 651; only `_field == "updated"` takes the compound branch |
| S-4 | Nested `_set` arrays at `depth>=1` also adopt the new compound order (D-04 clarification)       | INHERITED FROM 67-01 — entrest eager-load template calls `applySorting<Type>` at depth≥1, so the new compound emission reaches nested arrays for free; Plan 06 asserts end-to-end |
| S-5 | CI drift check (`go generate ./...` produces no diff beyond expected) passes                     | PASS     — second regen run is byte-identical                                                       |

## Deviations from plan

### [Rule 3 — Blocking issue] `entc.TemplateDir` cannot resolve entrest template funcs

- **Found during:** Task 3 first regen attempt.
- **Issue:** Plan Task 2 specified `entc.TemplateDir("./templates/entrest-sorting")` as the wiring.  This constructs a `gen.Template` via `gen.NewTemplate("external")` with only ent's base funcmap. `ParseDir` then fails because upstream `sorting.tmpl` uses entrest-provided funcs (`getAnnotation`, `getSortableFields`) not registered on that template: `template: sorting.tmpl:86: function "getAnnotation" not defined`.
- **Fix:** Replaced the `entc.TemplateDir(...)` call with a custom `entrestSortingOverride(path)` helper defined in the same file. It mirrors `entc.TemplateDir`'s shape exactly (`entc.Option` wrapping a `func(*gen.Config) error` that appends a parsed `*gen.Template` to `cfg.Templates`) but invokes `gen.NewTemplate("entrest-override").Funcs(entrest.FuncMaps())` BEFORE `ParseDir` so entrest's funcmap is in scope at parse time. `entrest.FuncMaps()` is publicly exported by entrest for exactly this use case (`templates.go:57` — `// FuncMaps export FuncMaps to use custom templates.`).
- **Files modified:** `ent/entc.go` (Task 2 — +21 net lines vs. the plan's expected ~1-2 line addition).
- **Commit:** `fbf0e60` (same atomic commit as Tasks 1+3, per plan instruction).
- **Plan change impact:** Minor. The acceptance grep (`grep 'entrest-sorting' ent/entc.go`) still matches because the custom helper call still contains that string. All downstream generator behaviour is identical to what the plan intended — the override IS picked up, per Task 3's grep + test evidence.
- **Upstream-alternative rejected:** `entrest.Config.Templates []*gen.Template` (the entrest-idiomatic slot) would have been slightly more canonical but requires either reaching into `entrest.funcMap` (unexported) via the exported `FuncMaps()` anyway, OR making each template a `gen.Template` bound to entrest's `baseTemplates` tree — which would require either a fork or more invasive reflection. The `entrestSortingOverride` helper is a 10-line implementation with a minimal surface, lives next to its only call site, and is easily removable on an entrest upgrade.

## Deferred Issues

### Pre-existing — `cmd/pdb-schema-generate/TestGenerateIndexes` fails since Plan 67-01

Full `go test -race ./...` during Task 3 verification surfaced one pre-existing failure NOT caused by Plan 67-02. Verified by stashing 67-02's working-tree changes and rerunning — the failure reproduces against the clean 67-01 tip. Documented in `.planning/phases/67-default-ordering-flip/deferred-items.md` (item D-67-01).

**Not auto-fixed** per the executor scope-boundary rule (fix only issues directly caused by the current task). Plan 67-01's generator-template update at `cmd/pdb-schema-generate/main.go` line 575 (`indexes, "updated"`) needs a matching 1-line update in `cmd/pdb-schema-generate/main_test.go:447` to admit `"updated"` in the test's allow-list. Recommended fix path: a follow-up `fix(67-01):` quick-task commit before Phase 67 completion, OR bundle into Plan 67-03 if that plan touches `cmd/pdb-schema-generate/`.

## Signals for the phase verifier

```bash
# 1. Template override file exists + has the key markers
test -f ent/templates/entrest-sorting/sorting.tmpl && \
  grep -q 'PROJECT-LOCAL OVERRIDE' ent/templates/entrest-sorting/sorting.tmpl && \
  grep -q 'FieldCreated' ent/templates/entrest-sorting/sorting.tmpl && \
  grep -q 'FieldID' ent/templates/entrest-sorting/sorting.tmpl

# 2. entc.go wires the override through the custom helper
grep -n 'entrestSortingOverride\|entrest.FuncMaps' ent/entc.go
#  line numbers for: the opts entry + the helper definition + the FuncMaps() call inside

# 3. Generated ent/rest/sorting.go has the compound injection in all 13 helpers
grep -c 'if _field == "updated"' ent/rest/sorting.go
# → 13

grep -c 'FieldCreated' ent/rest/sorting.go
# → 26 (13 funcs × 2 lines each: one FieldCreated + one FieldID — actually 26 total FieldCreated counts arise because imported packages list them and each injection emits <pkg>.FieldCreated once)

# 4. Fallback path unchanged for non-default sort fields
grep -A 1 '^	return _query.Order(withFieldSelector(_field, _order))' ent/rest/sorting.go | head -30
#  → Every applySorting ends with the upstream fallback AFTER the compound branch — explicit ?sort=id&order=asc still works.

# 5. Idempotence + build + test
TMPDIR=/tmp/claude-1000 go generate ./ent && git diff --stat ent/rest/sorting.go  # → empty
TMPDIR=/tmp/claude-1000 go build ./...                                              # → ok
TMPDIR=/tmp/claude-1000 go test -race ./ent/... -count=1                             # → ok
TMPDIR=/tmp/claude-1000 go test -race ./cmd/peeringdb-plus/... -count=1             # → ok

# 6. Commit reached HEAD
git log --oneline -1
# → fbf0e60 feat(67-02): entrest template override for compound default ORDER BY
```

## Self-Check

Verified files and commit exist:

- `ent/templates/entrest-sorting/sorting.tmpl` — FOUND, 184 lines, contains `PROJECT-LOCAL OVERRIDE`, `FieldCreated`, `FieldID`, `$defaultSort := ($t|getAnnotation).GetDefaultSort`.
- `ent/entc.go` — FOUND, contains `entrestSortingOverride("./templates/entrest-sorting")` call AND `gen.NewTemplate("entrest-override").Funcs(entrest.FuncMaps())` helper body.
- `ent/rest/sorting.go` — FOUND, contains 13 `if _field == "updated"` branches + 26 `FieldCreated` occurrences + 26 `FieldID` occurrences.
- Commit `fbf0e60` — FOUND in `git log --oneline -1` (`feat(67-02): entrest template override for compound default ORDER BY`).
- `.planning/phases/67-default-ordering-flip/deferred-items.md` — FOUND, documents D-67-01 pre-existing `TestGenerateIndexes` failure from Plan 67-01.

## Self-Check: PASSED

## PLAN 67-02 COMPLETE

Commits (this plan): `fbf0e60`
