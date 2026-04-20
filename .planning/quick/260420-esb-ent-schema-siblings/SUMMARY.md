---
task: 260420-esb-ent-schema-siblings
completed: 2026-04-20
type: quick
status: complete
commit: 559b5fa
---

# Ent schema siblings refactor — Summary

## What changed

Moved Phase 69 `*_fold` fields and Phase 70 `WithPrepareQueryAllow` annotations off the generated `ent/schema/{type}.go` files onto sibling files that `cmd/pdb-schema-generate` never touches. `go generate ./...` now produces zero drift on a clean tree — the pipeline that was silently stripping 324 lines of hand-edits on every full-tree regen is fixed architecturally.

Mirror of the v1.15 Phase 63 `ent/schema/poc_policy.go` precedent, extended to cover ent `Mixin()` (`{type}_fold.go`) and codegen-input maps (`pdb_allowlists.go`).

## Files created

- `ent/schema/fold_mixin.go` — reusable `foldMixin` type emits `<fieldname>_fold` shadow columns with `entgql.Skip + entrest.WithSkip` annotations (replaces 16 inline `field.String` decls).
- `ent/schema/{network,facility,internetexchange,organization,campus,carrier}_fold.go` — per-entity `Mixin()` wiring (6 files, 16 fold columns).
- `ent/schema/pdb_allowlists.go` — hand-written `schema.PrepareQueryAllows` map, 13 entries with verbatim upstream `// Source: serializers.py:<line>` comments preserved.

## Files modified

- `cmd/pdb-compat-allowlist/main.go` — now reads `schema.PrepareQueryAllows` directly (new `buildAllowlistEntry` + `goNameFor` helpers). Path B edge-map emission unchanged.
- `cmd/pdb-compat-allowlist/main_test.go` — replaced obsolete `TestDecodeFields` with 4 tests covering the new data path.
- 13 × `ent/schema/{type}.go` — removed `WithPrepareQueryAllow(...)` calls + `schemaannot` import; removed 16 inline `*_fold` `field.String` decls from the 6 folded entities.
- `CLAUDE.md` — Code Generation section rewritten (new sibling-file convention), Schema conventions expanded.
- `internal/sync/testdata/refactor_parity.golden.json` — regenerated. Field ordering shifted (ent Mixin contributes fields ahead of generated fields); byte count identical, pure ordering change.

## Gate results

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race ./...` — PASS across all 32 packages
- `golangci-lint run` — 0 issues
- `go test -race ./internal/pdbcompat/parity/...` — PASS
- `go generate ./...` drift — `git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/ internal/pdbcompat/allowlist_gen.go` → exit 0 ✓
- `internal/pdbcompat/allowlist_gen.go` — byte-identical pre/post refactor (the whole point of the refactor)

## Follow-up

The CI drift check at `.github/workflows/ci.yml:28-34` will now pass cleanly on push. No CI workflow change needed — the existing `go generate ./...` + `git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/` step is now correct.

## Commit

`559b5fa` — `refactor(ent/schema): sibling-files for hand-edits — restore go generate ./... as canonical pipeline`
