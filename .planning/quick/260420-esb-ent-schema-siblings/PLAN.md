---
task: 260420-esb-ent-schema-siblings
created: 2026-04-20
type: quick
---

# Ent schema siblings — restore `go generate ./...` as canonical pipeline

## Objective

`go generate ./...` must produce ZERO drift on a fully-annotated tree. Today it strips 324 lines of hand-edited annotations (Phase 69 `*_fold` + Phase 70 `schemaannot.WithPrepareQueryAllow`) from 13 `ent/schema/*.go` files because `cmd/pdb-schema-generate` regenerates those files from `schema/peeringdb.json` with no knowledge of the new annotation types. CI will fail on the next push because `.github/workflows/ci.yml:28-34` runs the full-tree form and drift-checks `ent/`.

Fix the architecture, not the CI: move all hand-edits off the generated files onto sibling files the regen never touches.

## Scope

### Part 1: `*_fold` fields → ent `Mixin`

**Current state:** 16 `field.String("*_fold")...` declarations live inline in the `Fields()` method body of 6 `ent/schema/*.go` files (network/facility/internetexchange/organization/campus/carrier). They carry `entgql.Skip(entgql.SkipAll)` + `entrest.WithSkip(true)` annotations.

**Target state:** A new `ent/schema/fold_mixin.go` sibling file defines a reusable mixin type:

```go
package schema

import (
    "entgo.io/contrib/entgql"
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "github.com/lrstanley/entrest"
)

// foldMixin contributes `<fieldname>_fold` shadow columns for Phase 69
// Unicode folding. Each fold column is emitted with entgql.Skip + entrest.WithSkip
// so it stays off every wire surface — internal plumbing only.
type foldMixin struct {
    ent.Schema
    fields []string
}

func (m foldMixin) Fields() []ent.Field {
    out := make([]ent.Field, 0, len(m.fields))
    for _, name := range m.fields {
        out = append(out,
            field.String(name+"_fold").
                Optional().
                Default("").
                Annotations(
                    entgql.Skip(entgql.SkipAll),
                    entrest.WithSkip(true),
                ).
                Comment("Phase 69 fold shadow column — internal plumbing"),
        )
    }
    return out
}
```

Then for each of the 6 affected entities, a sibling file `ent/schema/{type}_fold.go` defines the `Mixin()` method:

```go
// ent/schema/network_fold.go
package schema

import "entgo.io/ent"

func (Network) Mixin() []ent.Mixin {
    return []ent.Mixin{
        foldMixin{fields: []string{"name", "aka", "name_long"}},
    }
}
```

Field lists per entity (16 total — matches Phase 69-02 inventory):
- network: `name`, `aka`, `name_long` (3)
- facility: `name`, `aka`, `city` (3)
- internetexchange: `name`, `aka`, `name_long`, `city` (4)
- organization: `name`, `aka`, `city` (3)
- campus: `name` (1)
- carrier: `name`, `aka` (2)

**Remove** the 16 inline `*_fold` `field.String` declarations from the 6 generated schema files. After regen, ent will merge the mixin's fields with the generated fields — the `SetNameFold`/`NameFoldContains` etc accessors must still appear in `ent/*_create.go`.

Verify the mixin works by running `go generate ./ent/...` after the edits and grepping for `SetNameFold` in `ent/network_create.go` (expect hit).

### Part 2: `schemaannot.WithPrepareQueryAllow` → hand-written map

**Current state:** 13 `schemaannot.WithPrepareQueryAllow(...)` annotations on the `Annotations()` method of 13 `ent/schema/*.go` files. `cmd/pdb-compat-allowlist/main.go` walks the ent schema graph via `entc.LoadGraph` and reads these annotations to emit `internal/pdbcompat/allowlist_gen.go`.

**Target state:** A new `ent/schema/pdb_allowlists.go` sibling file exports a hand-written map:

```go
package schema

import "github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/schemaannot"

// PrepareQueryAllows mirrors upstream peeringdb/peeringdb@99e92c72
// serializers.py get_relation_filters() lists 1:1.
// Consumed by cmd/pdb-compat-allowlist.
var PrepareQueryAllows = map[string]schemaannot.AllowlistEntry{
    "net": {
        // Source: serializers.py:2947 NetworkSerializer.get_relation_filters
        Direct: []string{...},
        Via:    map[string][]string{...},
    },
    // ... 12 more
}
```

Copy every current `WithPrepareQueryAllow` argument list verbatim from the 13 schema files into this map. PRESERVE the source-line comments (the `// Source: serializers.py:NNNN ...` lines are load-bearing for future audits).

Update `cmd/pdb-compat-allowlist/main.go` to import `ent/schema` and read `schema.PrepareQueryAllows` directly as its Path A source, instead of walking annotations via `entc.LoadGraph`. The tool still needs `entc.LoadGraph` for Part B (Edges map emission), so keep that; only the allowlist-reading code path changes.

**Verify:** `internal/pdbcompat/allowlist_gen.go` regenerates byte-identically (or semantically identical — if the generator output is sort-sensitive and the map iteration order differs, lock a sort) after the refactor.

**Remove** the 13 `schemaannot.WithPrepareQueryAllow(...)` lines from the 13 schema files' `Annotations()` methods. After regen, the generated `Annotations()` will no longer include them — that's fine because the codegen tool now reads the hand-written map.

### Part 3: Verify zero drift under `go generate ./...`

After Parts 1 and 2:

```bash
go generate ./...
git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/ internal/pdbcompat/allowlist_gen.go
```

Must exit 0. If not, identify which file drifted and decide:
- Further hand-edits that need mirror treatment (move to sibling)
- Or regenerated-file content that needs `git add`-and-commit as the new canonical state

Run the full test suite to confirm no behavioural regressions:
- `go build ./...`
- `go vet ./...`
- `go test -race ./...`
- `golangci-lint run`
- Specifically: `go test -race ./internal/pdbcompat/parity/...` (the TestParity_* suite must stay green — traversal tests depend on the Allowlists map being correct post-refactor)

### Part 4: Documentation cleanup

Update `CLAUDE.md` § Code Generation:

- REMOVE the `Do NOT run \`go generate ./schema\`` line (no longer a trap — sibling files protect against it).
- REMOVE the need for scoped `go generate ./ent/...` advice; the full `go generate ./...` is safe again.
- ADD a short note documenting the sibling-file convention: "Hand-edited annotations live in sibling files (`{type}_fold.go`, `pdb_allowlists.go`, `fold_mixin.go`, `poc_policy.go`). `cmd/pdb-schema-generate` may regenerate `ent/schema/{type}.go` at any time without stripping hand-edits."

The CLAUDE.md additions I was about to propose in `/claude-md-management:revise-claude-md` are NOW OBSOLETE — skip them.

## Out of scope

- `cmd/pdb-schema-generate` changes (annotation-preservation logic) — sibling-file refactor removes the need.
- `index.Fields("updated")` indexes on schemas (Phase 67) — check whether regen strips these too; if yes, apply the same sibling-file treatment; if no, leave alone.
- Phase 70 `Edges` map — still emitted by cmd/pdb-compat-allowlist via ent-graph walk; no change needed.
- `(Poc).Policy()` in `poc_policy.go` — already uses the sibling-file pattern; no change.

## Verification commands

```bash
# 1. Zero drift on full-tree regen
go generate ./...
git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/ internal/pdbcompat/allowlist_gen.go

# 2. All gates green
go build ./...
go vet ./...
go test -race ./...
golangci-lint run

# 3. Sibling file presence greps
ls ent/schema/fold_mixin.go
ls ent/schema/{network,facility,internetexchange,organization,campus,carrier}_fold.go  # 6 files
ls ent/schema/pdb_allowlists.go

# 4. Annotation removal greps (should return 0 hits in generated schema files)
grep -c "_fold" ent/schema/network.go  # expect 0 (no inline fold decls)
grep -c "WithPrepareQueryAllow" ent/schema/*.go  # expect 0 across all 13

# 5. Mixin wiring greps (should return 6 hits — one per affected entity)
grep -l "foldMixin{" ent/schema/*_fold.go | wc -l  # expect 6

# 6. Hand-written map presence
grep -c "PrepareQueryAllows" ent/schema/pdb_allowlists.go  # expect ≥1
grep -c "schema.PrepareQueryAllows" cmd/pdb-compat-allowlist/main.go  # expect ≥1

# 7. Phase 70 parity tests still pass
go test -race ./internal/pdbcompat/parity/...
```

## Commit

Single atomic commit (or 2 splits if sizing warrants: Part 1 = fields, Part 2 = allowlists):

```
refactor(ent/schema): sibling-files for hand-edits — restore go generate ./... as canonical pipeline

Move Phase 69 *_fold fields into ent foldMixin + 6 {type}_fold.go sibling files.
Move Phase 70 WithPrepareQueryAllow annotations into hand-written
ent/schema/pdb_allowlists.go map consumed directly by
cmd/pdb-compat-allowlist.

`go generate ./...` now produces zero drift — cmd/pdb-schema-generate
can freely regenerate ent/schema/{type}.go without stripping hand-edits.
CI's existing drift check passes clean.

Mirror of the established Poc.Policy sibling-file pattern (v1.15 Phase 63).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
```
