---
phase: 67
slug: default-ordering-flip
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 67 Context: Default ordering flip

## Goal

List endpoints across all three query surfaces return rows in upstream PeeringDB's `(-updated, -created)` order instead of the current `id ASC`, matching `django-handleref` base `Meta.ordering`.

## Requirements

- **ORDER-01** — pdbcompat list ordering flip
- **ORDER-02** — grpcserver List/Stream ordering flip with stable pagination
- **ORDER-03** — entrest default ordering flip via schema annotation

## Locked decisions

- **D-01 — grpcserver cursor strategy**: Compound `(last_updated, last_id)` cursor. Base64-encoded cursor body changes shape; existing proto types for cursor are opaque `bytes`, so no proto regen required — just the encoder/decoder in `internal/grpcserver/pagination.go`. Cursor is stable under concurrent edits because the `id` tiebreaker is monotonic. No grpc-public consumers currently — breaking change is acceptable per cross-cutting rollout D-05.
- **D-02 — entrest default ordering source** (_updated 2026-04-19 after research_): entrest's `WithDefaultOrder` annotation only accepts a single `SortOrder` enum (the REST wire itself is single-field `?sort=<f>&order=<dir>`). Compound `(-updated, -created)` cannot be expressed via annotation. **Implementation: `entc.TemplateDir` template override** — post-process the generated `applySorting*` functions so the default ORDER BY emits both fields. Per-schema `entrest.WithDefaultSort("updated")` + `WithDefaultOrder(Desc)` still declared for documentation and ?sort= override fallback.
- **D-03 — Golden file regeneration strategy**: Regenerate all 39 pdbcompat golden files in one atomic commit (`chore: regenerate goldens under (-updated, -created) default order`). Before committing, `git diff` the regeneration manually; every changed line must be a row reorder — zero structural changes allowed. (_2026-04-19 note:_ current fixtures seed 1 row per type, so the reorder diff is effectively a no-op tautology. Multi-row fixture coverage is **deferred to Phase 72** parity tests per user decision; Phase 67 goldens still regenerate but audit signal comes from unit tests in §§test impact of RESEARCH.md.)
- **D-04 — Ordering scope**: Only list endpoints (`/api/<type>`, `/rest/v1/<type>`, `List*`/`Stream*` RPCs). Single-object lookups by pk are unchanged (no ordering applies).
  - **D-04 clarification (_2026-04-19 after research_):** RESEARCH §G-06 surfaced that entrest's eager-load template (`templates/eagerload.tmpl:29`) calls `applySorting<Type>` on nested `_set` arrays. Therefore adding `WithDefaultOrder(OrderDesc)` at the schema level AND installing the compound-default template override (Plan 02) ALSO reorders nested `_set` arrays at `depth>=1` in entrest `/rest/v1/<type>?depth=N` responses. This is accepted as the correct behaviour because (a) upstream PeeringDB Django serializer sorts nested relations using the same base `Meta.ordering = ("-updated", "-created")`, and (b) no known API consumer depends on id-ascending nested arrays. Plan 06's cross-surface E2E adds an explicit assertion that a `depth=2 /rest/v1/networks` response's nested `netixlan_set` (or equivalent) arrays sort descending by `updated`. pdbcompat and grpcserver nested eager-loads are unaffected by this note — they don't use the entrest template and keep their pre-existing nested ordering.
- **D-05 — Streaming cursor resume semantics**: `since_id` and `updated_since` predicates from v1.7 phases continue to apply BEFORE ordering, not after. Resume from `since_id=N` means "rows with id > N in `(-updated, -created)` order" — matches the compound cursor semantics.
- **D-06 — grpcserver `grpc-total-count` header**: Pre-count query still runs before streaming; count reflects filtered+ordered result cardinality (unchanged). No breaking change on header semantics.
- **D-07 — entrest template override** (_added 2026-04-19_): Override generator via `entc.TemplateDir("ent/templates/entrest-sorting/")` containing a patched `sorting.tmpl` that emits a compound `(-updated, -created, -id)` default when no explicit `?sort=` is present. Template lives in-tree; generator wiring added to `ent/entc.go`. Regenerated `ent/rest/sorting.go` is committed alongside the template.
- **D-08 — Add `updated` indexes in Phase 67** (_added 2026-04-19, corrects false out-of-scope claim_): CONTEXT previously cited non-existent "v1.9 Phase 46 updated indexes". Audit confirmed no `updated` or `created` indexes exist on any of the 13 entities. Phase 67 adds `index.Fields("updated").Annotations(...)` to all 13 `ent/schema/*.go` files (composite `(updated, id)` where `id` is PK-backed) so `ORDER BY updated DESC, id DESC` hits an index scan instead of a full-table sort. Auto-migrate applies on next startup via `migrate.WithDropIndex(true)`/existing migrate flags. Phase 71's memory budget can now assume index-backed ordering for the MEMORY-02 ceiling sizing.

## Out of scope

- GraphQL default ordering — Relay connection spec controls its own ordering via cursor. Keep existing behaviour.
- Web UI / terminal renderer ordering — decided by their own handlers, not these list endpoints.
- Multi-row golden fixtures — deferred to Phase 72 (pdb_api_test.py parity port).

## Dependencies

- **Depends on**: None (first phase of v1.16)
- **Enables**: Phase 68 — status/since matrix builds on the new default-ordering baseline.

## Plan hints for executor

- Touchpoints:
  - `internal/pdbcompat/registry_funcs.go` — 13 list query builders; each `.Order(...)` call switched to compound `(-updated, -created, -id)`
  - `internal/grpcserver/pagination.go` + 13 entity handler files — cursor encode/decode migration from `afterID int` to `(lastUpdated, lastID)`; ORDER BY change in `Query`/`QueryBatch` closures
  - `ent/schema/*.go` — 13 files: add `entrest.WithDefaultSort("updated")` + `WithDefaultOrder(entrest.OrderDesc)` annotation PLUS `index.Fields("updated")` (composite `(updated, id)` via field order) per D-08
  - `cmd/pdb-schema-generate/main.go` — `Annotations()` generator template updated in lock-step (generator strips hand-edits otherwise; CONTEXT.md § Conventions § Schema & Visibility)
  - `ent/entc.go` + new `ent/templates/entrest-sorting/sorting.tmpl` — TemplateDir override for compound default ORDER BY per D-07
  - `internal/pdbcompat/testdata/golden/*.json` — regenerate 39 files (tautological diff per D-03 note; still re-run to flush)
- Required regeneration: `go generate ./...` after schema + template edits; `go test -update ./internal/pdbcompat -run TestGoldenFiles` for goldens.
- Test coverage: cursor-format assertion updates in `grpcserver/pagination_test.go`; per-entity Stream tests assert `(-updated, -created)` order on ≥3-row seed data; pdbcompat unit tests for ordering (pre-existing 1-row goldens are insufficient — add focused `TestDefaultOrdering_*` tests with multi-row in-memory seed per surface).
- Index migration: auto-migrate applies `CREATE INDEX` on primary startup; replica picks up via LiteFS replication. Plan must verify via `sqlite3 ... '.schema'` post-deploy (OBS-04 path from CLAUDE.md).

## References

- ROADMAP.md Phase 67
- REQUIREMENTS.md ORDER-01..03
- Upstream: `django-handleref/src/django_handleref/models.py:95-101` (`class Meta: ordering = ("-updated", "-created")`)
- CLAUDE.md § ConnectRPC / gRPC (pagination + cursor notes)
