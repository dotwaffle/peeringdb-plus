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
- **D-02 — entrest default ordering source**: Per-schema `entrest.WithDefaultOrder(entrest.OrderDesc("updated"), entrest.OrderDesc("created"))` annotation added to all 13 ent schemas in `ent/schema/`. Requires `go generate ./...` rerun. Declarative and visible alongside every other entrest annotation; avoids hidden middleware behaviour.
- **D-03 — Golden file regeneration strategy**: Regenerate all 39 pdbcompat golden files in one atomic commit (`chore: regenerate goldens under (-updated, -created) default order`). Before committing, `git diff` the regeneration manually; every changed line must be a row reorder — zero structural changes allowed. If any golden shows a missing/added row, block the commit and debug.
- **D-04 — Ordering scope**: Only list endpoints (`/api/<type>`, `/rest/v1/<type>`, `List*`/`Stream*` RPCs). Single-object lookups by pk are unchanged (no ordering applies). Nested `_set` fields at `depth>=1` keep their current ordering (internal sort, not a list endpoint).
- **D-05 — Streaming cursor resume semantics**: `since_id` and `updated_since` predicates from v1.7 phases continue to apply BEFORE ordering, not after. Resume from `since_id=N` means "rows with id > N in `(-updated, -created)` order" — matches the compound cursor semantics.
- **D-06 — grpcserver `grpc-total-count` header**: Pre-count query still runs before streaming; count reflects filtered+ordered result cardinality (unchanged). No breaking change on header semantics.

## Out of scope

- GraphQL default ordering — Relay connection spec controls its own ordering via cursor. Keep existing behaviour.
- Web UI / terminal renderer ordering — decided by their own handlers, not these list endpoints.
- Performance optimisation for `ORDER BY updated DESC` — existing `updated` indexes on all 13 schemas (added v1.9 Phase 46) cover this query plan.

## Dependencies

- **Depends on**: None (first phase of v1.16)
- **Enables**: Phase 68 — status/since matrix builds on the new default-ordering baseline.

## Plan hints for executor

- Touchpoints: `internal/pdbcompat/registry_funcs.go` (13 list query builders), `internal/grpcserver/{pagination,network,*}.go` (cursor encode/decode + all 13 List/Stream handlers), `ent/schema/*.go` (13 files — add `entrest.WithDefaultOrder`), `internal/pdbcompat/testdata/golden/*.json` (regenerate 39 files).
- Required regeneration: `go generate ./...` after schema edits; `go test -update ./internal/pdbcompat -run TestGoldenFiles` for goldens.
- Test coverage: existing pagination tests need cursor-format assertion updates; grpcserver streaming tests need order assertions; pdbcompat golden comparison is the main ordering-correctness signal.

## References

- ROADMAP.md Phase 67
- REQUIREMENTS.md ORDER-01..03
- Upstream: `django-handleref/src/django_handleref/models.py:95-101` (`class Meta: ordering = ("-updated", "-created")`)
- CLAUDE.md § ConnectRPC / gRPC (pagination + cursor notes)
