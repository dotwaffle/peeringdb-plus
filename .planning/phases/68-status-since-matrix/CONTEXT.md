---
phase: 68
slug: status-since-matrix
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 68 Context: Status × since matrix + limit=0 semantics

## Goal

pdbcompat list and detail responses apply the upstream `rest.py:494-727` status/since matrix: `status=ok` default for lists, `(ok, pending)` for pk lookups, `(ok, deleted)` + `pending`-for-campus when `since>0`. `limit=0` returns all rows. `PDBPLUS_INCLUDE_DELETED` is removed; sync unconditionally persists deleted rows as tombstones via soft-delete.

## Requirements

- **STATUS-01** — list default `status=ok`
- **STATUS-02** — pk lookup `status IN (ok, pending)`
- **STATUS-03** — `since>0` admits deleted (+ pending for campus)
- **STATUS-04** — `?status=deleted` alone returns empty
- **STATUS-05** — sync-only rescoping of `PDBPLUS_INCLUDE_DELETED`
- **LIMIT-01** — `limit=0` = unlimited
- **LIMIT-02** — `depth>0` still caps at 250 per upstream

## Locked decisions

- **D-01 — `PDBPLUS_INCLUDE_DELETED` removed**: Env var deleted entirely. Sync always persists deleted rows as tombstones (see D-02). Any operator deploys still setting this var see a startup WARN-and-ignore for one milestone (v1.16 → v1.17 grace period), then hard-error if still set by v1.17. Migration note goes in `docs/CONFIGURATION.md` and CHANGELOG.
- **D-02 — Sync flipped to soft-delete**: The 13 `deleteStale*` functions in `internal/sync/worker.go` become `markStaleDeleted*` — they run `UPDATE ... SET status='deleted', updated=? WHERE id NOT IN (?)` instead of `DELETE FROM`. Rows stay in DB; `status=deleted + since` queries now return real data. Hard-delete path is removed entirely. Tombstone GC policy deferred to SEED-004.
- **D-03 — Backfill on first post-Phase-68 sync**: First full sync after deploy runs normal soft-delete. Rows hard-deleted BEFORE Phase 68 ships are gone forever — `?status=deleted` returns only rows marked deleted from that sync onward. Documented as a known one-time gap. No retroactive reconstruction from PeeringDB since we don't have historical state.
- **D-04 — `limit=0` safety ceiling**: No safety ceiling in Phase 68 — `limit=0` returns all rows unconditionally per upstream. Between Phase 68 shipping and Phase 71 landing the memory budget, do not deploy to prod. Execute phases 68 → 69 → 70 → 71 as a coordinated ship; Phase 71's pre-flight row-count × size heuristic is the OOM safeguard.
- **D-05 — Campus `pending` inclusion**: Campus single-object lookups and `since>0` list queries include `status=pending`. Other types do not admit `pending` on list (D-06). Matches upstream `rest.py:721` special case.
- **D-06 — pk-lookup `status` filter**: All 13 entity types admit `(ok, pending)` on single-object (pk) GET, not just campus. Matches upstream `rest.py:706-708` `get_queryset` behaviour for non-list requests.
- **D-07 — `?status=deleted` without `since` = empty**: Upstream applies the final `filter(status='ok')` unconditionally on list requests without `since` per `rest.py:725`. Our pdbcompat mirrors this: any list request without `since` filters to `status=ok` regardless of `?status=<anything>` param. Only pk lookups and `since>0` requests allow alternate statuses.

## Out of scope

- Cross-entity traversal filters (Phase 70)
- Memory budget enforcement (Phase 71)
- grpcserver/entrest status semantics — Phase 68 is pdbcompat-only. entrest users get whatever entrest's default filter set supports; grpcserver has its own explicit `status` filter field.
- Tombstone GC scheduler — deferred to SEED-004.

## Dependencies

- **Depends on**: Phase 67 (shared pdbcompat list-path code; order-flip lands first to avoid merge pain in `registry_funcs.go`)
- **Enables**: Phase 69 (status-matrix tests feed into UNICODE-03 fuzz corpus and PARITY-01 regression)

## Plan hints for executor

- Touchpoints: `internal/pdbcompat/{registry_funcs,handler,response}.go`, `internal/pdbcompat/filter.go` (new status matrix helper), `internal/sync/worker.go` (13 deleteStale → markStaleDeleted), `internal/config/config.go` (remove `IncludeDeleted`, add WARN for legacy env), `docs/CONFIGURATION.md`, `CHANGELOG.md`.
- Sync test suite needs new soft-delete fixture: seed row → remove from upstream fixture → assert `status='deleted'` (not absent).
- The `deleteStaleOrganizations`-style functions currently return `(int, error)`; signature can stay for `markStaleDeleted*` returning count of rows marked.

## References

- ROADMAP.md Phase 68
- REQUIREMENTS.md STATUS-01..05, LIMIT-01..02
- Upstream: `peeringdb_server/rest.py:494-497, 694-727`; `pdb_api_test.py:3950-3964` (since/status/campus)
- SEED-004 (tombstone GC — planted alongside this phase)
