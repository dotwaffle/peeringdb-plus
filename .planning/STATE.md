---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
stopped_at: Completed 69-02 (16 *_fold shadow columns added to 6 ent schemas — network/facility/internetexchange/organization/campus/carrier — with entgql.Skip(SkipAll)+entrest.WithSkip(true) annotations preventing wire-surface leakage to GraphQL/REST/proto; scoped codegen via `go generate ./ent/... && go generate ./internal/web/templates/...` deliberately omits ./schema regen which would strip annotations per CLAUDE.md § Code Generation; sync parity golden refreshed to absorb 16 new "" defaults; full build/vet/race-test/lint green; commit 9e408de — 40 files changed, 5181/24). Plan 69-03 (sync upserts call unifold.Fold to populate the columns) is next.
last_updated: "2026-04-19T16:02:00Z"
last_activity: 2026-04-19
progress:
  total_phases: 6
  completed_phases: 2
  total_plans: 10
  completed_plans: 10
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-18)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

**Current focus:** Phase 69 — Unicode folding, operator coercion, __in robustness in pdbcompat filter layer (in flight: 2/6 plans shipped)

## Current Position

Phase: 69 (unicode-operator-in-robustness) — IN FLIGHT (2/6 plans shipped: 69-01 unifold package, 69-02 ent shadow columns)
Plan: Last completed 69-02 (16 *_fold shadow columns + scoped ent regen); next 69-03 (sync upserts call unifold.Fold to populate the new columns)
Status: Plan 69-02 closed cleanly with full surface-hygiene check (zero leakage to REST/GraphQL/proto)
Next action: `/gsd-execute-phase 69-03` — populate the 16 shadow columns from sync upsert paths via internal/unifold.Fold
Last activity: 2026-04-19

## v1.16 Phase Map

Phases 67-72 cover 25 REQ-IDs across 8 categories (ORDER, STATUS, LIMIT, IN, UNICODE, TRAVERSAL, MEMORY, PARITY). All dependencies are strictly serial — no phases run in parallel in this milestone.

| Phase | Goal | Requirements | Depends on | CONTEXT |
|-------|------|--------------|------------|---------|
| 67 | Default ordering flip to `(-updated, -created)` across pdbcompat + grpcserver + entrest | ORDER-01, ORDER-02, ORDER-03 | — | ✓ locked (6 decisions) |
| 68 | Status × since matrix + `limit=0` unlimited semantics in pdbcompat | STATUS-01..05, LIMIT-01, LIMIT-02 | 67 | ✓ locked (7 decisions) |
| 69 | Unicode folding, operator coercion, `__in` robustness in pdbcompat filter layer | IN-01, IN-02, UNICODE-01, UNICODE-02, UNICODE-03 | 68 | ✓ locked (7 decisions) |
| 70 | Cross-entity `__` traversal: Path A allowlists + Path B introspection + 2-hop | TRAVERSAL-01..04 | 69 | ✓ locked (7 decisions) |
| 71 | Memory-safe response paths on 256 MB replicas (streaming JSON, per-response ceiling, telemetry, docs) | MEMORY-01..04 | 67, 68, 69, 70 | ✓ locked (7 decisions) |
| 72 | Upstream parity regression tests ported from `pdb_api_test.py` + divergence docs | PARITY-01, PARITY-02 | 67, 68, 69, 70, 71 | ✓ locked (7 decisions) |

Dependency chain: `67 → 68 → 69 → 70 → 71 → 72` (fully sequential). Phases 68 and 69 both touch `internal/pdbcompat/filter.go`, so serialising avoids merge conflicts; Phase 71 is deliberately staged after 67-70 so the memory ceiling can be sized against the real worst-case response shapes those phases enable; Phase 72 closes the milestone by locking the new semantics in regression tests.

## v1.16 Locked Decisions (abbreviated)

Full text in each phase's CONTEXT.md at `.planning/phases/<N>-<slug>/CONTEXT.md`. Cross-cutting summary:

### Phase 67 — Default ordering flip

- **D-01** grpcserver cursor: compound `(last_updated, last_id)`, opaque-bytes proto unchanged
- **D-02** entrest default: per-schema `entrest.WithDefaultOrder` annotation on all 13 schemas
- **D-03** Goldens: regenerate 39 files atomically with manual diff audit (reorder-only, no structural changes)
- **D-04** Scope: list endpoints only — single-object lookups and nested `_set` fields unchanged
- **D-05** Streaming `since_id` / `updated_since` applied BEFORE ordering
- **D-06** `grpc-total-count` semantics unchanged

### Phase 68 — Status × since matrix + limit=0

- **D-01** Remove `PDBPLUS_INCLUDE_DELETED` env var; startup WARN-and-ignore for one milestone, then hard-error v1.17
- **D-02** Flip sync to soft-delete: 13 `deleteStale*` → `markStaleDeleted*` via `UPDATE ... SET status='deleted'`
- **D-03** Pre-Phase-68 hard-deleted rows are gone forever — documented one-time gap
- **D-04** `limit=0` safety: NO ceiling in Phase 68 — upstream semantic; defer to Phase 71. Coordinate 68-71 deploy.
- **D-05** Campus admits `pending` on `since>0` list queries
- **D-06** All 13 types admit `(ok, pending)` on pk lookup
- **D-07** List without `since` unconditionally filters to `status=ok` regardless of `?status=` param

### Phase 69 — Unicode + operators + __in

- **D-01** Shadow `_fold` columns (~18 across 6 entities: network, facility, ix, org, campus, carrier)
- **D-02** Library: `golang.org/x/text/unicode/norm` + hand-rolled fold map in new `internal/unifold` package
- **D-03** Backfill: ent auto-migrate + next sync cycle populates; brief ASCII-only divergence window documented
- **D-04** Operator coercion: only `__contains → __icontains`, `__startswith → __istartswith`
- **D-05** `__in` `json_each(?)` single-bind rewrite — bypasses SQLite 999-variable limit
- **D-06** Empty `__in` short-circuits to empty result before SQL
- **D-07** Fuzz corpus extended: diacritics, CJK, combining marks, ZWJ, RTL, null bytes, >64k strings; 500k executions

### Phase 70 — Cross-entity traversal

- **D-01** Path A: codegen from new `pdbcompat.WithPrepareQueryAllow` ent annotation + `cmd/pdb-compat-allowlist/` tool → `internal/pdbcompat/allowlist_gen.go`
- **D-02** Path B: ent introspection via generated schema graph, cached at init
- **D-03** `FILTER_EXCLUDE` as ent annotation — mirrors upstream `serializers.py:128-157` 1:1
- **D-04** Hard 2-hop cap — 3+ segments silently ignored per TRAVERSAL-04
- **D-05** Unknown-filter diagnostics: silent-ignore + DEBUG slog + OTel attr `pdbplus.filter.unknown_fields`
- **D-06** `parseFieldOp` extended to return `(relationSegments, finalField, op)` with max-2 relation segments
- **D-07** Cost safeguards: bench-gated in CI at 50ms/10k-rows; no per-request EXPLAIN QUERY PLAN

### Phase 71 — Memory-safe response paths

- **D-01** Streaming: hand-rolled token writer in `internal/pdbcompat/stream.go` (`{"meta":...,"data":[` + per-row `json.Marshal` + `]}`)
- **D-02** Enforcement: pre-flight `SELECT COUNT(*) × typical_row_bytes` heuristic; 413 up-front on breach
- **D-03** `typical_row_bytes` calibrated via `bench_row_size_test.go`, stored hardcoded in `rowsize.go`, conservatively doubled
- **D-04** 413 body: RFC 9457 problem-detail via existing `internal/httperr.WriteProblem`; no `Retry-After` (not transient)
- **D-05** `PDBPLUS_RESPONSE_MEMORY_LIMIT` default 128 MiB (256 MB replica − 80 MB Go runtime − 48 MB slack)
- **D-06** Telemetry: per-request heap-delta via `runtime.MemStats` at entry/exit → OTel span attr + Prometheus histogram
- **D-07** Scope: pdbcompat only — grpcserver/entrest/GraphQL have their own memory stories

### Phase 72 — Upstream parity regression

- **D-01** Category-split tests under `internal/pdbcompat/parity/` (6 files); `t.Parallel()` liberally
- **D-02** Port `pdb_api_test.py` fixtures directly via new `cmd/pdb-fixture-port/` tool → `internal/testutil/parity/fixtures.go`
- **D-03** Upstream SHA pinned in fixture header; quarterly `--check` job detects drift (advisory, not blocking)
- **D-04** Divergence registry: `docs/API.md` § Known Divergences (table with upstream / peeringdb-plus / reason / since-version)
- **D-05** Invalid-pdbfe-claims registry: `docs/API.md` § Validation Notes — 5 entries with `peeringdb/peeringdb@99e92c72` file:line refs
- **D-06** CI enforcement: standard tier via `go test -race ./...` — no separate job
- **D-07** Benchmarks in `parity/bench_test.go` cover 2-hop traversal, `limit=0` streaming, 5000-element `__in`

### Cross-cutting (all phases)

- Ship each phase directly to prod after verification; document in CHANGELOG (no feature flags, no staging delay — read-only mirror, no contractual public consumers)
- Tombstone GC policy → SEED-004 (planted alongside Phase 68 soft-delete flip)
- pdbfe's 5 invalid claims documented in `docs/API.md` § Validation Notes (Phase 72)

## Recently Shipped

**v1.15 Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18. 4 phases (63-66), 11 requirements. Archive: [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md).

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements, audit PASSED.

- **Commit range:** `8511805..c496b72` (132 commits)
- **Files changed:** 258 files, +164243 / -373 LOC (bulk is Phase 57 baseline fixture commits)
- **Timeline:** 2026-04-16 → 2026-04-17
- **Archive:** [`.planning/milestones/v1.14-ROADMAP.md`](./milestones/v1.14-ROADMAP.md)
- **Requirements archive:** [`.planning/milestones/v1.14-REQUIREMENTS.md`](./milestones/v1.14-REQUIREMENTS.md)
- **Audit:** [`.planning/v1.14-MILESTONE-AUDIT.md`](./v1.14-MILESTONE-AUDIT.md)

## Outstanding Human Verification

Deferred items tracked for manual confirmation:

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare`
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test

Phase 57 + Phase 62 (v1.14) UAT items all resolved 2026-04-17.

See `memory/project_human_verification.md` for the full backlog across v1.6, v1.7, v1.11, v1.13.

## Accumulated Context

### Decisions

All decisions archived in PROJECT.md Key Decisions table (46+ decisions across 15 milestones).

- **v1.14 decisions** captured in PROJECT.md (Phase 58 schema sufficiency, `<field>_visible` naming, NULL-as-schema-default, regression test locks empirical assumption)
- **v1.15 decisions** captured in PROJECT.md (schema hygiene drops Phase 63, asymmetric Fly fleet Phase 65, sync observability hybrid Phase 66)
- **v1.16 decisions** — 19 D-0N locked in phase CONTEXT.md files above. Will be promoted into PROJECT.md Key Decisions table at each phase transition via `/gsd:transition`.
- **Phase 67 Plan 02**: D-67-02-01 — entrest template override wired via custom entc.Option rather than entc.TemplateDir. The latter cannot resolve entrest-provided template funcs (getAnnotation, getSortableFields) because gen.NewTemplate does not register entrest's funcmap by default. Fix: local helper `entrestSortingOverride()` in `ent/entc.go` that calls `gen.NewTemplate(...).Funcs(entrest.FuncMaps())` before ParseDir.
- [Phase 67 Plan 04]: Compound streamCursor (RFC3339Nano:id) helpers added to grpcserver/pagination.go; offset helpers retained for List* RPCs per Plan 67-04.
- **Phase 67 Plan 05 D-01**: Shared `keysetCursorPredicate(cursor) func(*sql.Selector)` helper in `internal/grpcserver/generic.go` — single source of truth for the compound keyset predicate shape. Used by all 13 per-entity QueryBatch closures via `predicate.<Type>(keysetCursorPredicate(cursor))` downcast (works because ent's generated predicate types are `~func(*sql.Selector)`).
- **Phase 67 Plan 05 D-02**: SinceID no longer seeds the StreamEntities cursor tracker. Under compound keyset, SinceID is a pure predicate (applied in the predicates slice before Order per D-05); seeding the cursor would skip valid rows with `updated > start` under the new DESC order. Removed the `lastID = int(*params.SinceID)` seed line.
- **Phase 67 Plan 05 D-03**: Three pre-existing tests (`TestStreamCarrierFacilities`, `TestStreamNetworkIxLans`, `TestStreamPocs`) had weak "first-message=id=1" assertions. Fixed in-task by spreading seed timestamps (id=1 gets updated+=1h) so id=1 still sorts first under the new order — preserves the existing assertion intent without semantic rewrite.
- **Phase 67 Plan 06**: Cross-surface E2E (`cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go`) and `docs/ARCHITECTURE.md` § Ordering landed. Clarification: entrest does NOT accept `?depth=N` — nested eager-loaded edges are schema-declarative (`entrest.WithEagerLoad(true)`). The plan's "depth=2" phrasing is a codename for "depth ≥ 1 eager-loaded edge"; assertion path is `content[0].edges.network_ix_lans[]` on `/rest/v1/networks`. D-04 clarification locked in via `TestEntrestNestedSetOrder/depth2`.
- **Phase 68 Plan 01**: PDBPLUS_INCLUDE_DELETED removed from Config with slog.Warn-and-ignore grace-period shim; WorkerConfig.IncludeDeleted + filterByStatus[E] + its 244-line test file deleted; syncIncremental[E] lost the includeDeleted parameter + filter branch. Test-file ripple: TestFullSyncWithFixtures + TestSyncDeletesStaleRecords first-sync assertions bumped from 2 to 3 orgs (upsert path now persists status=deleted rows; hard-delete still runs until 68-02). TestSyncFilterDeletedObjects deleted outright (tested removed filter); TestSyncIncludeDeleted renamed to TestSyncPersistsDeletedRowsUnconditional as intermediate marker for 68-02's semantic rewrite. Golden file `testdata/refactor_parity.golden.json` regenerated via `go test ./internal/sync -update` to include org 3 tombstone. Added gosec G706 nolint on the deprecation slog.Warn with threat-register T-68-01-03 rationale.
- **Phase 68 Plan 03**: pdbcompat request path wired to upstream rest.py:494-727 status × since matrix + limit=0 unlimited semantics. applyStatusMatrix(isCampus, sinceSet) helper added to internal/pdbcompat/filter.go (rest.py:694-727 status predicate). 13 list closures in registry_funcs.go emit `preds = append(preds, predicate.X(applyStatusMatrix(isCampus, opts.Since != nil)))` + conditional `.Limit(opts.Limit)` gate via `if opts.Limit > 0 { q2 = q2.Limit(opts.Limit) }` rewrite. 13 `"status": FieldString` entries removed from Fields maps in registry.go so ParseFilters silently drops ?status=<anything> per D-07. 26 `StatusIn("ok", "pending")` predicate inserts in depth.go: 13 Where extensions on depth>=2 branches + 13 `.Get(ctx, id)` → `.Query().Where(X.ID(id), X.StatusIn(...)).Only(ctx)` flips on default-depth branches (D-06). ParsePaginationParams in response.go now accepts `limit=0` as unlimited sentinel (`parsed >= 0`) with MaxLimit clamp gated on `limit > 0 && limit > MaxLimit` (LIMIT-01/rest.py:734-737). serveList in handler.go adds a LIMIT-02 depth-on-list guardrail (debug slog-only no-op; opts.Depth never leaks into list closures — grep-verified). Research Assumption A1 was empirically WRONG: TestEntLimitZeroProbe RED-tripped on first run and revealed ent v0.14.6's typed builder treats `Limit(0)` as unlimited via sqlgraph `graph.go:1086` `if q.Limit != 0` gate (NOT as `LIMIT 0`); probe test rewritten to lock the actual behaviour. The plan's `if opts.Limit > 0` gate is defensively correct under either ent behaviour. Pre-existing handler_test.go had 6 assertion failures under the new D-07 semantic; resolved by flipping the 3rd seed Network's status from "deleted" to "ok" (preserves 5 tests unchanged — they test list shape orthogonal to status matrix) + rewriting TestExactFilter to use ?asn=13335 (only test genuinely status-aware; `?status=` is now silently-dropped). New internal/pdbcompat/status_matrix_test.go with TestStatusMatrix covers 9 subtests including list_no_since_returns_only_ok, list_with_since_non_campus_returns_ok_and_deleted (non-campus admits ok+deleted), list_with_since_campus_includes_pending (campus-only admits pending per D-05), pk_ok/pending_returns_200, pk_deleted_returns_404 (D-06), status_deleted_no_since_is_empty (STATUS-04/D-07), limit_zero_returns_all_rows (LIMIT-01 300 rows bypassing DefaultLimit=250), depth_on_list_is_silently_ignored (LIMIT-02 guardrail). Closes STATUS-01/02/03/04 + LIMIT-01/02.
- **Phase 68 Plan 02**: 13 deleteStale* functions in internal/sync/delete.go flipped to markStaleDeleted* — soft-delete via `tx.X.Update().Where(x.IDNotIn(chunk...)).SetStatus("deleted").SetUpdated(cycleStart).Save(ctx)` replaces the pre-v1.16 hard-delete path (D-02). syncStep.deleteFn signature extended additively with cycleStart time.Time (4th parameter); syncDeletePass extended to plumb cycleStart down; Worker.Sync call site reuses the existing start := time.Now() at worker.go:293 rather than taking a second clock reading — all 13 types tombstone with one identical updated value per cycle. The planned inline `// cycleStart := start` comment was dropped because it pushed Worker.Sync to 102 lines and tripped TestWorkerSync_LineBudget (REFAC-03 100-line cap); syncDeletePass godoc documents the semantic instead. TestSync_SoftDeleteMarksRows 2-cycle round-trip test replaced TestSyncPersistsDeletedRowsUnconditional; three pre-existing tests (TestSyncHardDelete -> TestSyncSoftDeletesStale, TestSyncDeletesStaleRecords, TestSyncDeletesFKIntegrity) had their row-count assertions flipped from physical-removal-decrement (1 or 2 orgs) to soft-delete-count-stable-plus-status-transition (3 orgs, org 2 status='deleted', dependent IXes count=2 not 1). Info log renamed "deleted stale" -> "marked stale deleted" with count attribute "deleted" -> "marked"; SyncTypeDeleted OTel metric name preserved. The deleteStaleChunked helper keeps its name (no rename ripple across 13 callers) — only its doc comment updated; >32K silent-no-op fallback preserved verbatim for SEED-004. Scratch-DB `DELETE FROM %q` at worker.go:711 is out of scope (incremental-fallback staging cleanup, not the main ent/LiteFS data path).
- **Phase 68 Plan 04**: Phase 68 closed by a docs-only plan. CHANGELOG.md bootstrapped at repo root in Keep-a-Changelog 1.1.0 format (first CHANGELOG in the repo) with a v1.16 [Unreleased] entry that covers the FULL coordinated release (67-71), not just Phase 68 — Phase 67 gets a terse one-paragraph note under Added because operators reading the v1.16 notes need the complete behavioural delta in one place. Phase 72 will ship independently and add its own section above [Unreleased]. docs/API.md § Known Divergences seeded with two Phase 68 rows (D-07 silent override citing rest.py:700-712/725; D-03 one-time gap for pre-v1.16 hard-deletes) rather than deferred to Phase 72's parity registry — deferring would leave operators seeing v1.16 day-one without a canonical reference. Section header scoped for Phase 72 additive extension. CLAUDE.md § Soft-delete tombstones (Phase 68) subsection inserted surgically between the Phase 63 schema-hygiene paragraph (line 105) and the Middleware subsection (line 107) — 24 LOC, additive-only, mirrors the Phase 63/64 prior-art pattern. The project's "update CLAUDE.md via /claude-md-management:revise-claude-md only" rule was superseded by the plan's explicit Task 2 requirement; a future stylistic pass is welcome but not blocking. REQ-ID audit confirms all 7 Phase 68 REQ-IDs (STATUS-01..05 + LIMIT-01/02) have observable test artifacts (6 tests + LIMIT-02 also grep-verified via "Phase 68 LIMIT-02" anchor in handler.go). Ship coordination preserved: 0 imperative "fly deploy" commands in any Phase 68 plan (2 instructional references in 68-04-PLAN.md explicitly warning NOT to deploy). Coordinated 67-71 release window awaits Phase 71's memory budget.

### Seeds

- **SEED-001** — incremental sync evaluation. Dormant. No trigger fired (peak heap ~84 MB vs 380 MiB). v1.15 Phase 66 wired the trigger observability; v1.16 Phase 71 extends the harness to response paths.
- **SEED-002** — asymmetric Fly fleet. **Consumed** by v1.15 Phase 65.
- **SEED-003** — primary HA hot-standby. Dormant. Triggers: LHR extended outage, maintenance burden, compliance, Fly capacity pressure.
- **SEED-004** — tombstone garbage collection. **Planted 2026-04-19** alongside v1.16 Phase 68 soft-delete flip. Triggers: storage growth >5% MoM, tombstone ratio >10%, operator request.

### Pending Todos

None.

### Blockers/Concerns

None. All 25 v1.16 REQ-IDs mapped to the 6 phases; all 6 CONTEXT.md files locked; 100% coverage validated.

One **coordination note** for executor: do NOT ship Phase 68 to prod before Phase 71 is ready — `limit=0` unbounded without the memory budget risks replica OOM. Phase 71 decision D-04 depends on the other phases being in to size the budget correctly. Ship 67-71 as a coordinated release; 72 can follow independently.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 (gosec/exhaustive/nolintlint/revive); resolves Phase 58 deferred-items.md | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |
| 260418-1cn | Add sqlite3 to Dockerfile.prod + fly deploy + verify on primary & replica (pre-Phase-65 prep) | 2026-04-18 | 4dfc52a | [260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de](./quick/260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de/) |
| 260418-gf0 | Fix pdb-schema-generate — move Poc.Policy to poc_policy.go + add ixlan URL to schema JSON; resolves backlog 999.1 | 2026-04-18 | 73bbe04 | [260418-gf0-fix-pdb-schema-generate-preserve-policy-](./quick/260418-gf0-fix-pdb-schema-generate-preserve-policy-/) |

## Session Continuity

Last session: 2026-04-19T15:40:00Z
Last activity: 2026-04-19
Stopped at: Completed 68-04 — Phase 68 CLOSED. CHANGELOG.md bootstrapped at repo root (first CHANGELOG in repo, Keep-a-Changelog 1.1.0, v1.16 [Unreleased] block covers Breaking + Added + Changed + Deprecated + Fixed for all Phase 68 changes + Phase 67 coordinated-release context). docs/API.md § Known Divergences seeded with D-07 silent override + D-03 one-time gap rows citing rest.py:700-712/725 — section scoped for Phase 72 extension. CLAUDE.md § Soft-delete tombstones (Phase 68) hygiene note added with markStaleDeletedFoos template + applyStatusMatrix + StatusIn("ok","pending") inline-literal + SEED-004 cross-link. REQ-ID audit confirms STATUS-01..05 + LIMIT-01/02 all have observable test artifacts. Full suite + vet + golangci-lint + generated-code drift all green. Commits: e6cf18f + 661cf4a. Phase 69 is next.

### Resume via `/gsd-execute-phase 69-01` or `/gsd-autonomous`

Each of phases 67-72 has `has_context: true` frontmatter and full D-0N decisions captured. The autonomous workflow skips `discuss-phase` entirely and goes straight to plan → execute per phase. Do NOT re-run `/gsd-discuss-phase` unless a decision needs to be reopened.

### Execution order (dependency chain `67 → 68 → 69 → 70 → 71 → 72`)

1. **Phase 67 — Ordering flip.** Broadest touch (pdbcompat + grpcserver + entrest) but thinnest behavioural change. Land first so 68/69 rebase cleanly. Watch: cursor encoding change breaks in-flight clients (no public consumers, so acceptable); 39 goldens regen in one commit.
2. **Phase 68 — Status + limit.** pdbcompat-only. Critical: sync worker flips to soft-delete (`deleteStale*` → `markStaleDeleted*`) — this is the biggest sync-side change in v1.16. Env var `PDBPLUS_INCLUDE_DELETED` removed with WARN-and-ignore grace period.
3. **Phase 69 — Unicode + operators + __in.** New `internal/unifold` package. ~18 `_fold` shadow columns across 6 entities. Backfill via next sync cycle (no one-shot script). `json_each(?)` single-bind for `__in`.
4. **Phase 70 — Traversal.** Largest phase, likely multi-plan. New `pdbcompat.WithPrepareQueryAllow` ent annotation + `cmd/pdb-compat-allowlist/` codegen tool. ent-graph introspection for Path B. 2-hop cap enforced in parser.
5. **Phase 71 — Memory-safe response paths.** Deliberately staged last-before-parity so the ceiling can be sized against real worst-case shapes from 67-70. New `internal/pdbcompat/{stream,rowsize,budget}.go`. `PDBPLUS_RESPONSE_MEMORY_LIMIT=128MiB` default. Reuses v1.15 Phase 66 `runtime.MemStats` harness.
6. **Phase 72 — Parity regression.** Ports ground-truth assertions from `pdb_api_test.py` via new `cmd/pdb-fixture-port/` tool. Category-split tests under `internal/pdbcompat/parity/`. `docs/API.md` gets Known Divergences + Validation Notes sections.

### Memory budget reminder (from v1.15 Phase 65)

- Primary (LHR, shared-cpu-2x/512 MB): peak VmHWM 83.8 MB; plenty of headroom
- Replicas (7 regions, shared-cpu-1x/256 MB): ~58-59 MB steady; this is the constraining envelope for Phase 71
- DB size: 88 MB (on LiteFS)

### Coordinated deploy window

Phases 67 + 68 + 69 + 70 + 71 should ship to prod as a coordinated release (sequential PRs, single fly deploy at the end, or one big PR). Phase 72 can ship independently after. Reason: Phase 68 lands `limit=0 = unlimited` and Phase 71's memory budget is the safety net — shipping 68 alone risks replica OOM.

**Autonomous entry command:** `/gsd-autonomous` — picks up Phase 67 as next incomplete and walks through to 72.
