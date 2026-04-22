---
status: complete
gsd_summary_version: 1.0
plan_id: 260418-gf0
mode: quick
completed: "2026-04-18"
commit: 73bbe04
resolves_backlog: 999.1
---

# Quick Task 260418-gf0: Fix pdb-schema-generate — preserve hand-added Policy()

## What shipped

Resolved backlog 999.1 via Option B (split hand-edited methods into sibling files). `go generate ./...` is now idempotent and safe — the `./ent`-only workaround is retired.

### Code changes

- **NEW:** `ent/schema/poc_policy.go` — `(Poc).Policy()` moved here with all supporting imports (`context`, `pdbent`, `poc`, `privacy`, `privctx`). The generator never touches this file (names must exactly match generated-type filenames for the generator to rewrite them).
- `ent/schema/poc.go` — Policy() removed (now in sibling). Unused imports dropped. Generator-safe.
- `schema/peeringdb.json` — added the `ixf_ixp_member_list_url` field to the `ixlan` fields block (Phase 64 added it only to `ent/schema/ixlan.go`, which the generator strips). Field definition mirrors `_visible` companion shape: `type: string`, `max_length: 255`, `nullable: false`, `default: ""`.
- Regenerated `ent/`, `gen/`, `graph/` to reflect the schema changes.
- `internal/sync/testdata/refactor_parity.golden.json` — regenerated via `-update` flag (ixlan field position shifted since the generator now sorts by JSON iteration order).
- `CLAUDE.md` — updated §Schema hygiene to: (1) drop the `go generate ./ent` workaround, (2) document the sibling-file convention for hand-edited methods on generated schemas.

### Docs

- `.planning/ROADMAP.md` — removed 999.1 backlog section.
- `.planning/phases/999.1-*/` — deleted (no artifacts to preserve).

## Verification

- `go build ./...` → 0
- `go test -race -count=1 -short ./...` → all 22 packages PASS (TestSync_RefactorParity golden regenerated after field position shift)
- `golangci-lint run` → 0 issues
- `govulncheck ./...` → No vulnerabilities found
- `go generate ./...` idempotent (confirmed by diff check across two consecutive runs)

## Deviations

1. **Phase 64 gap surfaced.** Regenerating `schema/peeringdb.json` via the generator stripped the Phase 64 `ixf_ixp_member_list_url` field because Phase 64 had only added it to `ent/schema/ixlan.go` (not the canonical JSON). Added the field to the JSON with the matching shape (`max_length: 255` was required to avoid ent generating `*string` via its "unlimited string implies Nillable" path). The ixlan field ordering shifted from end-of-slice to between `_visible` and `import_enabled` as a side effect — proto is frozen (`entproto.SkipGenFile`) so no wire-compat impact.

2. **TestSync_RefactorParity golden regeneration.** Field ordering shift reordered JSON output; regenerated via `go test -update ./internal/sync/... -run TestSync_RefactorParity` (documented update path in worker_test.go:51-55). Byte count identical (14,561 bytes); only key order changed.

## Coverage

- Addresses backlog 999.1 (harden pdb-schema-generate for hand-edited methods).
- Closes a latent Phase 64 drift bug (ixlan URL field missing from canonical JSON).
- Restores `go generate ./...` as the canonical one-shot command.

## Commits

| Commit | Description |
|--------|-------------|
| `73bbe04` | refactor(quick-260418-gf0): move Poc.Policy to poc_policy.go + add ixlan URL to JSON schema |
