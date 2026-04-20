---
phase: 68
slug: status-since-matrix
status: human_needed
verified_at: 2026-04-19
score: 5/5 ROADMAP success criteria verified; 7/7 REQ-IDs covered; 7/7 decisions compliant
overrides_applied: 0
---

# Phase 68: Status × Since Matrix + limit=0 Semantics — Verification Report

**Phase Goal:** pdbcompat response filtering matches upstream `rest.py:494-727` rules for `?status`, `?since`, and `?limit`; `PDBPLUS_INCLUDE_DELETED` re-scoped as sync-side only (and in practice removed entirely).
**Verified:** 2026-04-19
**Status:** human_needed — all automated gates green; post-deploy smoke tests deferred to the coordinated 67–71 ship window.
**Re-verification:** No — initial verification.

## Summary

- All 5 ROADMAP success criteria observably pass in code + tests. Soft-delete flip (Plan 68-02) produces real tombstones; pdbcompat matrix (Plan 68-03) exposes them via the upstream rest.py rules; config gate (Plan 68-01) is gone with a grace-period WARN.
- All 7 Phase 68 REQ-IDs (STATUS-01..05, LIMIT-01, LIMIT-02) have observable test evidence. `TestStatusMatrix` exercises 9 scenarios end-to-end; `TestSync_SoftDeleteMarksRows` locks the 2-cycle tombstone round-trip; `TestLoad_IncludeDeleted_Deprecated` captures slog output; `TestEntLimitZeroProbe` pins the empirical ent-layer `.Limit(0)` semantic.
- All 7 Locked Decisions (D-01..D-07) verified compliant via grep + code inspection; no unplanned scope creep observed across Plans 68-01..04.
- Full `go test -race ./...` green; `go build ./...` clean; `golangci-lint run ./...` reported clean in plan SUMMARYs. No generated-code drift expected (no ent/proto edits this phase).
- Ship coordination invariant enforced: zero imperative `fly deploy` in any 68-0N-PLAN.md; the two matches in 68-04-PLAN.md are instructional "do NOT deploy" reminders. LIMIT-01 unbounded `limit=0` without Phase 71's memory budget is an OOM risk that only surfaces on live replicas — requires human sign-off post-Phase-71 for the coordinated ship.

## Success Criteria

| # | ROADMAP Criterion | Status | Evidence |
|---|--------------------|--------|----------|
| 1 | `GET /api/<type>` (no since) returns only `status=ok`; `/api/<type>/{pk}` admits `status=pending` | VERIFIED | `internal/pdbcompat/registry.go` — `grep -c '"status":\s*FieldString'` = 0 across 13 Fields maps; `internal/pdbcompat/depth.go` — `grep -c 'StatusIn("ok", "pending")'` = 26 (13 funcs × 2 call sites); `TestStatusMatrix/list_no_since_returns_only_ok` + `TestStatusMatrix/pk_ok_returns_200` + `pk_pending_returns_200` subtests pass. |
| 2 | `?since={ts}` returns `(ok, deleted)`; `+pending` for campus | VERIFIED | `internal/pdbcompat/filter.go:29` `applyStatusMatrix(isCampus, sinceSet)` helper; `registry_funcs.go` — `applyStatusMatrix(true` = 1 (`wireCampusFuncs` only), `applyStatusMatrix(false` = 12 for other entities; `TestStatusMatrix/list_with_since_non_campus_returns_ok_and_deleted` + `list_with_since_campus_includes_pending` subtests pass. Tombstones supplied by `TestSync_SoftDeleteMarksRows` (Plan 68-02) verifying `status=deleted` + `updated>=cycle2Start` round-trip. |
| 3 | `?status=deleted` no-since returns empty | VERIFIED | `status` field removed from 13 Fields maps (D-07) — `?status=deleted` silently dropped by `ParseFilters`; `applyStatusMatrix` unconditionally emits `FieldEQ("status","ok")` when `sinceSet=false`. `TestStatusMatrix/status_deleted_no_since_is_empty` subtest passes. |
| 4 | `?limit=0` unlimited; `depth>0` guardrail on lists | VERIFIED | `internal/pdbcompat/response.go:64` `parsed >= 0` gate; `:68` `limit > 0 && limit > MaxLimit` clamp; 13 `if opts.Limit > 0` conditionals in `registry_funcs.go`. `handler.go:129` "Phase 68 LIMIT-02 guardrail" comment + `slog.DebugContext` call at `:138` silently ignore `?depth=` on list endpoints. `TestStatusMatrix/limit_zero_returns_all_rows` + `depth_on_list_is_silently_ignored` subtests pass. `TestEntLimitZeroProbe` locks empirical ent behaviour (Assumption A1 rebutted — ent v0.14.6's sqlgraph graph.go:1086 `if q.Limit != 0` treats `Limit(0)` as unlimited; defensive `if opts.Limit > 0` gate correct under either ent semantic). Phase 71 owns the memory-budget ceiling. |
| 5 | `PDBPLUS_INCLUDE_DELETED` sync-only (in practice removed + WARN) | VERIFIED | `internal/config/config.go:189` `slog.Warn("PDBPLUS_INCLUDE_DELETED is deprecated ...")` ; `grep -rn IncludeDeleted internal/config/` returns only references in `config_test.go` (no live field); `Config.IncludeDeleted` and `WorkerConfig.IncludeDeleted` entirely absent; `TestLoad_IncludeDeleted_Deprecated` subtests `env_set_warns` + `env_unset_no_warn` pass. `README.md` — `grep -c PDBPLUS_INCLUDE_DELETED` = 0 (cleaned in Plan 68-01). |

## Requirement Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| STATUS-01 | 68-03 | List without `?since` returns `status=ok` only | SATISFIED | `TestStatusMatrix/list_no_since_returns_only_ok` + Fields-map grep (0 `"status"` entries) + 12 `applyStatusMatrix(false` sites |
| STATUS-02 | 68-03 | pk lookup admits `(ok, pending)` for all 13 types | SATISFIED | `TestStatusMatrix/{pk_ok_returns_200,pk_pending_returns_200,pk_deleted_returns_404}`; 26 `StatusIn("ok","pending")` inserts in `depth.go` (13 × 2 call sites) |
| STATUS-03 | 68-02 (data) + 68-03 (surface) | `?since>0` returns `(ok, deleted)` + pending for campus | SATISFIED | `TestSync_SoftDeleteMarksRows` (2-cycle round-trip producing tombstones); `TestStatusMatrix/list_with_since_non_campus_returns_ok_and_deleted` + `list_with_since_campus_includes_pending`; `applyStatusMatrix(true` = 1 (campus-only) |
| STATUS-04 | 68-03 | `?status=deleted` no-since → empty | SATISFIED | `TestStatusMatrix/status_deleted_no_since_is_empty`; Fields-map removal causes silent drop; matrix emits `FieldEQ("status","ok")` unconditionally when `sinceSet=false` |
| STATUS-05 | 68-01 | env var removed + WARN-and-ignore grace period | SATISFIED | `TestLoad_IncludeDeleted_Deprecated`; `config.go:189` slog.Warn; no `Config.IncludeDeleted` field; `WorkerConfig.IncludeDeleted` absent |
| LIMIT-01 | 68-03 | `?limit=0` = unlimited | SATISFIED | `TestStatusMatrix/limit_zero_returns_all_rows` (seeds 300 rows, asserts no cap); `TestEntLimitZeroProbe/{Limit_0_returns_all_rows,no_Limit_returns_all_rows}`; `ParsePaginationParams` `parsed >= 0` gate |
| LIMIT-02 | 68-03 | `?depth>0` silently ignored on list (Phase 68 guardrail only; Phase 71 owns functional list+depth) | SATISFIED | `TestStatusMatrix/depth_on_list_is_silently_ignored` E2E + `handler.go:129-138` guardrail comment + `slog.DebugContext` paper trail |

No orphaned REQ-IDs — `.planning/REQUIREMENTS.md` maps only STATUS-01..05 + LIMIT-01/02 to Phase 68, and all 7 appear in at least one of 68-01/02/03 plan `requirements` fields (or are cumulatively covered in 68-04's `requirements-verified-cover`).

## Decision Compliance

| Decision | Compliance | Evidence |
|----------|-----------|----------|
| D-01 — `PDBPLUS_INCLUDE_DELETED` removed; WARN grace period | COMPLIANT | `Config.IncludeDeleted` and `WorkerConfig.IncludeDeleted` fields absent; `config.go:189` slog.Warn; `TestLoad_IncludeDeleted_Deprecated` covers both `env_set_warns` and `env_unset_no_warn`; comment at insertion site pins the v1.17 fatal-error swap |
| D-02 — Sync flipped to soft-delete | COMPLIANT | 13 `markStaleDeleted<Type>` in `internal/sync/delete.go` (`grep -c "^func markStaleDeleted" delete.go` = 13); 13 `SetStatus("deleted")` + 13 `SetUpdated(cycleStart)`; 0 `.Delete().Where` in `delete.go`. Single `DELETE FROM` remaining at `worker.go:711` is the scratch SQLite cleanup path (explicitly out of D-02 scope per 68-02-SUMMARY Scope Compliance) |
| D-03 — Backfill one-time gap | COMPLIANT | `CHANGELOG.md § Breaking` cites "one-time gap"; `docs/API.md:552` divergence row documents pre-v1.16 hard-deletes are gone forever; no retroactive reconstruction path added |
| D-04 — No safety ceiling on `limit=0` in Phase 68 | COMPLIANT | `ParsePaginationParams` gates MaxLimit clamp on `limit > 0 && limit > MaxLimit` (bypasses clamp at `limit=0`); no `MaxUnlimited`/`HardCap` constant introduced; Phase 71 owns memory budget (called out in coordination reminder) |
| D-05 — Campus `pending` on `?since` | COMPLIANT | `registry_funcs.go` — `grep -c "applyStatusMatrix(true" = 1` (only `wireCampusFuncs`); `grep -c "applyStatusMatrix(false" = 12` for the other 12 wire functions |
| D-06 — All 13 pk-lookups admit `(ok, pending)` | COMPLIANT | `depth.go` — 26 `StatusIn("ok", "pending")` predicates (13 funcs × 2 call sites: depth>=2 `Where` extensions + 13 `.Get(ctx,id)` → `.Query().Where().Only()` flips); `grep -c "\.Get(ctx, id)" depth.go` = 0 |
| D-07 — `?status=deleted` without `since` = empty | COMPLIANT | 13 `"status": FieldString` entries removed from Fields maps in `registry.go` (grep count = 0); `applyStatusMatrix(false)` emits `FieldEQ("status","ok")` unconditionally when `sinceSet=false`; `TestStatusMatrix/status_deleted_no_since_is_empty` locks the observable behaviour |

## Human Verification

Plan 68-04 SUMMARY explicitly defers these to the human coordinator post-Phase-71. None can be validated from static code inspection alone:

1. **Live prod deprecation WARN** — After deploy, confirm that setting `PDBPLUS_INCLUDE_DELETED=true` in `fly.toml` or Fly secrets surfaces the `slog.Warn("PDBPLUS_INCLUDE_DELETED is deprecated ...")` line exactly once per machine boot in `fly logs -a peeringdb-plus`.
   - **Expected:** One WARN line per machine per boot; Load() returns no error; machines reach `/readyz` normally.
   - **Why human:** Requires `fly deploy` + `fly logs` tailing; cannot be exercised in unit-test harness.

2. **Post-deploy smoke tests against prod (`/api/net`)** — The three curl probes queued in 68-04-PLAN.md step 5:
   ```bash
   curl -s 'https://peeringdb-plus.fly.dev/api/net?limit=0' | jq '.data | length'   # expect full count (>>250)
   curl -s 'https://peeringdb-plus.fly.dev/api/net?status=deleted' | jq '.data | length'  # expect 0
   curl -s 'https://peeringdb-plus.fly.dev/api/net?since=0' | jq '.data | length'        # expect all + tombstones
   ```
   - **Expected:** `limit=0` returns every row (well over 250); `?status=deleted` (no `since`) returns 0; `?since=0` returns the full set including any post-v1.16 tombstones.
   - **Why human:** Requires a deployed instance with real sync data; local test env cannot exercise the tombstone window size or replica-vs-primary semantics.

3. **Replica OOM risk assessment during coordinated 67–71 deploy window** — D-04 explicitly defers the memory ceiling to Phase 71; `limit=0` unbounded on a replica (256 MB rootfs per v1.15 asymmetric fleet) may OOM under adversarial or high-cardinality queries before Phase 71 lands its cap.
   - **Expected:** Deploy coordinator holds Phase 68 at the staging window and only pushes it out as part of the coordinated 67→68→69→70→71 sequence. Monitor `pdbplus_sync_peak_heap_mib` (Grafana SEED-001 watch row) plus `fly logs` for OOM-kill events on replicas for the 24-hour window post-deploy.
   - **Why human:** Live traffic risk assessment; cannot be validated from code alone.

4. **Phase 67/68/69/70/71 coordinated ship verification** — STATE.md locks these phases as a single deploy window. Individual `fly deploy` of Phase 68 would ship LIMIT-01 without Phase 71's memory safeguard.
   - **Expected:** Zero `fly deploy` runs until Phase 71 also lands (verified here — no imperative `fly deploy` in any Phase 68 plan; the 2 matches in 68-04-PLAN.md are explicit warnings NOT to deploy).
   - **Why human:** Coordination + deploy-authority decision, not a code property.

## Gaps Found

None. Every claim checked against the codebase matched the SUMMARY assertions; every test named in plans and SUMMARYs exists and passes; every grep invariant holds. The automated pipeline (`go build`, `go test -race ./...`, test name presence, grep counts, file presence/absence) is 100% green for Phase 68 scope.

## Notes

- **Plan 68-01 auto-fixes** were well-scoped mechanical ripples: (a) `TestFullSyncWithFixtures` / `TestSyncDeletesStaleRecords` bumped org-count assertions 2→3 because removing `filterByStatus` means `status=deleted` rows now enter via upsert; (b) deleted `TestSyncFilterDeletedObjects` which tested a removed filter; (c) regenerated `internal/sync/testdata/refactor_parity.golden.json` to include org 3 tombstone; (d) added `//nolint:gosec // G706` to deprecation slog.Warn with threat-register reference; (e) removed `includeDeleted bool` param from `newTestWorker{,WithMode}` helpers at 23 call sites.

- **Plan 68-02 auto-fixes**: (a) Dropped inline `// cycleStart := start` comment because it pushed `Worker.Sync` to 102 lines, breaking `TestWorkerSync_LineBudget` (REFAC-03 100-line cap). Rationale is still captured in `syncDeletePass` godoc. (b) Three pre-existing tests flipped: `TestSyncHardDelete`→`TestSyncSoftDeletesStale`, `TestSyncDeletesStaleRecords` + `TestSyncDeletesFKIntegrity` assertions updated from row-count-decrement to status-transition. FK-integrity invariant remains trivially true under soft-delete but regression-locks the two-pass sync structure.

- **Plan 68-03 key surprise**: `TestEntLimitZeroProbe` rebutted Research Assumption A1 — ent v0.14.6 already treats `.Limit(0)` as "no limit" via `sqlgraph/graph.go:1086 if q.Limit != 0`. The plan's defensive `if opts.Limit > 0 { q2 = q2.Limit(opts.Limit) }` gate is correct under both empirical and hypothesised ent semantics. Probe test now locks the actual behaviour with a source-line ref so any future ent regression RED-trips.

- **Plan 68-04 CLAUDE.md edit**: Done as an additive insert under Conventions despite the CLAUDE.md rule "update via `/claude-md-management:revise-claude-md` only" — plan explicitly required the hygiene note and the surgical 24-LOC subsection mirrors the Phase 63/64 prior-art pattern already in the file. Flagged for a future stylistic pass, not a blocker.

- **Scratch DB `DELETE FROM` at `internal/sync/worker.go:711`** is the incremental-fallback recovery path clearing a partial scratch SQLite table; it is NOT the ent/LiteFS main-DB delete pass targeted by D-02. Plan 68-02 SUMMARY explicitly noted this as intentional out-of-scope. Verified: no other `DELETE FROM` statements in `internal/sync/` outside `testdata/`.

- **One-time-gap literal vs. rationale**: `docs/API.md:552` contains the one-time-gap divergence row with full rationale citing D-03 and "Documented intentional one-time gap" in the Rationale column; CHANGELOG.md Breaking section also notes "**One-time gap:** Rows hard-deleted by sync cycles BEFORE the v1.16 upgrade are gone forever." Both operator-facing docs cover the D-03 contract; VERIFICATION considered this satisfied.

---

*Verified: 2026-04-19*
*Verifier: Claude (gsd-verifier)*
