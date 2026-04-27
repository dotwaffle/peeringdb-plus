---
gsd_state_version: 1.0
milestone: v1.18.0
milestone_name: Cleanup & Observability Polish
status: paused
last_updated: "2026-04-27T01:30:00.000Z"
last_activity: 2026-04-27
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 8
  completed_plans: 8
  percent: 50
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-22)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

**Current focus:** v1.18.0 paused after Phase 75. 3 phases remain (76, 77, 78) + milestone lifecycle.

## Current Position

Milestone: v1.18.0 — half done (3/6 phases complete: 73 ✓, 74 ✓, 75 ✓).
Status: PAUSED — user stopped autonomous run after Phase 75 to resume tomorrow.
Last activity: 2026-04-27 -- Phase 75 code review fixes applied (commit c710ac6); awaiting Phases 76/77/78 + lifecycle.

**Resume command (tomorrow):** `/gsd-autonomous --from 76` (continues with Phases 76 → 77 → 78 → audit → complete → cleanup; all CONTEXT.md already locked, no discuss needed).

Alternative if you want phase-by-phase control: `/gsd-plan-phase 76` then `/gsd-execute-phase 76` then `/gsd-plan-phase 77` etc. Phase 78's UAT-01 will pause for your manual CSP DevTools step.

## v1.18.0 Phase Map

| Phase | Goal | Requirements | Depends on | Status |
|-------|------|--------------|------------|--------|
| 73 — Code Defect Fixes | Fix campus inflection 500 + drop `poc.role` NotEmpty validator | BUG-01, BUG-02 | — | ✓ shipped 2026-04-26 |
| 74 — Test & CI Debt | Three deferred test failures + 5 lint findings cleared | TEST-01, TEST-02, TEST-03 | — | ✓ shipped 2026-04-27 (3/3 plans, 13/13 verified, 3 review warnings auto-fixed) |
| 75 — Code-side Observability Fixes | Cold-start gauge, zero-rate counter pre-warm, http.route middleware | OBS-01, OBS-02, OBS-04 | — | ✓ shipped 2026-04-27 (3/3 plans, 10/10 codebase truths verified, human_needed for live confirmation, 4 review warnings auto-fixed) |
| 76 — Dashboard Hardening | `service_name` filter sweep + post-canonicalisation metric flow confirmation | OBS-03, OBS-05 | Phase 75 (soft) | Ready to plan |
| 77 — Telemetry Audit & Cleanup | Loki log-level audit + Tempo trace sampling/batching review | OBS-06, OBS-07 | Phase 75 (hard for OBS-07) | Ready to plan (Phase 75 dep satisfied — OBS-04 http.route empirically populates) |
| 78 — UAT Closeout | v1.13 CSP + headers verification, v1.5 Phase 20 archive | UAT-01, UAT-02, UAT-03 | — | Ready to plan (UAT-01 will need your manual DevTools step) |

Phase numbering continues from v1.16 (last phase 72). v1.17.0 was a release tag (quick task 260426-pms), not a milestone — no phases consumed.

## Locked Decisions (abbreviated)

All 6 phase CONTEXT.md committed `fe724fc` 2026-04-26. Full decisions live in each phase's `CONTEXT.md`; abbreviated below for resume-after-/clear continuity.

**Phase 73 — Code Defect Fixes** ✓ SHIPPED 2026-04-26 (commits `6266b6b`..`8684f2c`)

- BUG-01 fixed: `ent/schema/campus_annotations.go` sibling-file mixin with `entsql.Annotation{Table: "campuses"}` makes `entc.LoadGraph` consumers (cmd/pdb-compat-allowlist) see the right table name. `internal/pdbcompat/allowlist_gen.go` `TargetTable: "campus"` count flipped 2→0. `path_a_1hop_fac_campus_name` un-skipped + `DIVERGENCE_` canary removed. DEFER-70-06-01 closed in deferred-items.md and `docs/API.md § Known Divergences`. `ent/schema/campus.go` and `ent/entc.go` byte-unchanged (sibling-file convention preserved).
- BUG-02 fixed: `cmd/pdb-schema-generate/main.go` new `isTombstoneVulnerableField(name)` predicate drops `NotEmpty()` on `role`. Audit confirmed only 2 NotEmpty() in `ent/schema/`: poc.role (DROP — tombstone-vulnerable) + ixprefix.prefix (KEEP — IP prefix is structural row identity). Two regression guards committed: `TestPoc_RoleEmptyAcceptedByValidator` (unit, ~76ms) + `TestSync_IncrementalRoleTombstone` (httptest fake-upstream, ~330ms). Verification 18/18 must-haves passed.
- Out-of-scope finding (recorded in 73-VERIFICATION.md, NOT auto-fixed): existing Org `notWantParts` regression guard at `cmd/pdb-schema-generate/main_test.go:245-251` uses 4-tab source indent vs generator's 3-tab output — guard would never match if NotEmpty re-appeared on Organization. New Phase 73 Poc + role guards use correct `\n\t\t\t` escape strings. Surface in a future hygiene quick task or fold into Phase 74.

**Phase 74 — Test & CI Debt** (TEST-01, TEST-02, TEST-03)

- **D-01**: TEST-01 — auto-derive expected indexes from `schema/peeringdb.json` + entgo declarations (most rigorous of three options; user explicitly picked over deny-list inversion).
- **D-02**: TEST-02 — drop the `$region` template variable entirely from `pdbplus-overview.json`; flip test to assert no orphan template vars.
- **D-03**: TEST-03 — per-finding mix: `filepath.Clean` + nolint where genuinely operator-supplied; fix `exhaustive` properly; remove stale nolintlint directive.

**Phase 75 — Code-side Observability Fixes** (OBS-01, OBS-02, OBS-04)

- **D-01**: OBS-01 — synchronous one-shot `COUNT(*)` per-table at process init (~1-2s startup cost acceptable); seeds the same cache currently primed by sync-completion. **✓ SHIPPED Plan 75-01 2026-04-26 (commits `0c8ca6d`..`5ea3e19`):** new `internal/sync/initialcounts.go` `InitialObjectCounts(ctx, *ent.Client)` helper (status-agnostic counts, fail-fast on error per GO-CFG-1) wired into `cmd/peeringdb-plus/main.go` between `database.Open` and `pdbotel.InitObjectCountGauges`. 3 unit tests lock the contract. OnSyncComplete unchanged.
- **D-02**: OBS-02 — pre-warm 13 types × 4 per-type metrics + 2 directions × RoleTransitions = 54 baseline series (CONTEXT.md's "65" was a miscount; PLAN.md `must_haves.truths` corrected to 52 + 2 = 54). Status dimension self-populates. **✓ SHIPPED Plan 75-02 2026-04-26 (commits `f2dcacc`..`49edf98`):** new `internal/otel/prewarm.go` `PrewarmCounters(ctx)` + `PeeringDBEntityTypes` exports; single call site in `cmd/peeringdb-plus/main.go` between syncWorker construction and StartScheduler spawn; 3 unit tests lock the contract. Plan 75-02 deviations: 1 acceptance-gate-fix (PLAN's A7 grep regex too loose for self-documenting file; tighter `\.Add\(` anchored regex returns expected 5 — no source change, recorded as patterns-established for future plans).
- **D-03**: OBS-04 — investigate root cause + fix `routeTagMiddleware`. Suspects: `r.Pattern` empty for non-`METHOD /path` routes, middleware ordering, or labeler-context replacement downstream. **✓ SHIPPED Plan 75-03 2026-04-27 (commits `beb870d`, `1734fe6`):** empirical evidence (in-process slog probe + standalone sdkmetric.NewManualReader test through production-shaped chain) refutes all 3 code-bug hypotheses; root cause is sparse-traffic / Prometheus-staleness artifact. Fix shape: defensive E2E test (cmd/peeringdb-plus/route_tag_e2e_test.go — 3 tests, 5 sub-tests for /healthz, /api/{rest...}, /rest/v1/, /graphql, /ui/{rest...}) + doc-comment expansion on routeTagMiddleware documenting WHY the middleware exists despite otelhttp v0.68.0 emitting http.route natively (intervening r.WithContext middleware hides r.Pattern from otelhttp's local r). No body change. No new instrumentation library. OBS-04-INVESTIGATION.md captures the full evidence trail with otelhttp v0.68.0 source-line citations. Plan 75-03 deviations: 3 transient diagnostic auto-fixes (slog.LevelDebug bumped to LevelInfo, sync_status pre-seeded to bypass readiness gate, transient zzz_metric_probe_test.go for direct metric-record verification) — all reverted before commit.

Plan 75-01 deviations: 1 lint auto-fix (revive package-comments detached) folded into GREEN commit; 1 stale acceptance baseline (`os.Exit(1)` count `<=12` was based on 11+1; actual file has 13+1=14 — structural intent preserved, recorded as patterns-established note for future plans to use structural rather than count-equality assertions).

**Phase 76 — Dashboard Hardening** (OBS-03, OBS-05)

- **D-01**: OBS-03 — both `$service` template var AND explicit `{service_name="$service"}` in every `go_*` panel query. Affected panels: Goroutines, Heap Memory, Allocation Rate, GC Goal, Live Heap by Instance.
- **D-02**: OBS-05 — confirm only. Verify new `_bytes_bucket` flowing on v1.17.0+. NO panel-description docs, NO Prom drop rule for legacy `_kib_KiB_bucket`. (User diverged from recommended "confirm + document".)

**Phase 77 — Telemetry Audit & Cleanup** (OBS-06, OBS-07)

- **D-01**: OBS-06 — audit + fix in same phase. Produce `AUDIT.md` AND apply slog level changes inline. Targets: per-step sync DEBUG, FK-orphan summary verification, `_visible` redaction logs, sync-cycle entry/exit, health-check 200s.
- **D-02**: OBS-07 — **scope expansion**: audit + add proactive per-endpoint sampling rules (user picked the most aggressive option). Implementation via `sdktrace.ParentBased` composite sampler dispatching on `http.route`. **HARD DEPENDENCY on Phase 75 OBS-04** — without `http.route` populating, route-based sampler can't dispatch correctly.

**Phase 78 — UAT Closeout** (UAT-01, UAT-02, UAT-03)

- **D-01**: Hybrid — Claude drives UAT-02 (curl headers, body cap, slowloris Go probe) directly; user drives UAT-01 (CSP DevTools on `/ui/`, `/ui/asn/13335`, `/ui/compare` with `PDBPLUS_CSP_ENFORCE=true`). Claude produces UAT-RESULTS.md template.
- **D-02**: UAT-03 — just relocate `.planning/milestones/v1.5-phases/20-deferred-human-verification/` to a `.archived/` sibling; trust the 2026-03-24 verification record. No re-verification.

**Notable user-driven divergences from recommended options:**

- TEST-01: picked "auto-derive from schema source" (most rigorous) over recommended deny-list inversion. More upfront work but immune to this whole class of test rot.
- OBS-05: picked "confirm only" over recommended "confirm + document". No panel-description note for the legacy series.
- OBS-07: picked "audit + add new sampling rules proactively" over recommended "audit + tune if off-baseline". Phase 77 now has more code work + a hard dep on Phase 75.

## Recently Shipped

**v1.18.0 Phase 73 — Code Defect Fixes** — shipped 2026-04-26. 2 plans (73-01 + 73-02), 18/18 verification, 16 commits (`6266b6b`..`8684f2c`). BUG-01 (campus inflection 500→200) and BUG-02 (poc.role NotEmpty drop) fully closed and regression-locked. NOT YET DEPLOYED — fixes are merged to main locally but not pushed/deployed.

**v1.16 Django-compat Correctness** — shipped 2026-04-19, archived 2026-04-22. 6 phases (67-72), 25 requirements. `pdbcompat` surface now has full behavioural parity with upstream PeeringDB.

**v1.15 Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18. 4 phases (63-66), 11 requirements.

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements.

## Outstanding Human Verification

Tracked under v1.18.0 Phase 78 (UAT-01, UAT-02, UAT-03):

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare` — under UAT-01
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test — under UAT-02
- **Phase 20 (v1.5):** stale deferred-items pointer dir relocation — under UAT-03

See `memory/project_human_verification.md` for the full backlog (the v1.6/v1.7/v1.11 ~33 UI/visual items are explicitly out of scope per REQUIREMENTS.md and deferred to a future "UI verification sweep" milestone).

## Accumulated Context

### Seeds

- **SEED-001** — incremental sync evaluation. **Consumed 2026-04-26** by quick task 260426-pms (default `PDBPLUS_SYNC_MODE=incremental` shipped in v1.17.0).
- **SEED-002** — asymmetric Fly fleet. **Consumed** by v1.15 Phase 65.
- **SEED-003** — primary HA hot-standby. Dormant.
- **SEED-004** — tombstone garbage collection. **Planted 2026-04-19**. Explicitly out of scope for v1.18.0 per user 2026-04-26 ("data volume tiny").

### Pending Todos

None.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |
| 260418-1cn | Add sqlite3 to Dockerfile.prod + fly deploy | 2026-04-18 | 4dfc52a | [260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de](./quick/260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de/) |
| 260418-gf0 | Fix pdb-schema-generate — resolves backlog 999.1 | 2026-04-18 | 73bbe04 | [260418-gf0-fix-pdb-schema-generate-preserve-policy-](./quick/260418-gf0-fix-pdb-schema-generate-preserve-policy-/) |
| 260419-ski | Auth-conditional PDBPLUS_SYNC_INTERVAL default | 2026-04-19 | c242d90 | [260419-ski-auth-sync-interval](./quick/260419-ski-auth-sync-interval/) |
| 20260420-esb | Ent schema siblings refactor | 2026-04-20 | 559b5fa | [20260420-esb-ent-schema-siblings](./quick/20260420-esb-ent-schema-siblings/) |
| 20260422-remove-nightly-bench | Remove the "Nightly bench" GitHub Actions workflow | 2026-04-22 | 3ca8590 | [20260422-remove-nightly-bench](./quick/20260422-remove-nightly-bench/) |
| 260426-jke | Observability cleanup — log noise, trace overflow, redundant metric names, dashboard repair | 2026-04-26 | cef357a | [260426-jke-obs-cleanup](./quick/260426-jke-obs-cleanup/) |
| 260426-lod | Observability label gaps — GC-allowlisted resource attrs (service.namespace/cloud.region) + http.route via post-dispatch labeler | 2026-04-26 | cf1b558 | [260426-lod-observability-label-gaps-gc-allowlisted-](./quick/260426-lod-observability-label-gaps-gc-allowlisted-/) |
| 260426-mei | Production observability — Prom alert rules (6, 2-tier severity) + dashboard panel 35 re-sourced to per-instance go.memory.used | 2026-04-26 | d2d337d | [260426-mei-production-observability-alerts-per-inst](./quick/260426-mei-production-observability-alerts-per-inst/) |
| 260426-pms | Flip default PDBPLUS_SYNC_MODE to incremental (SEED-001) — codegen-layer NotEmpty() drop on `name` for 6 folded entities + tombstone conformance test + sync metrics `mode` label | 2026-04-26 | a3ae545 | [260426-pms-flip-default-pdbplus-sync-mode-to-increm](./quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/) |

## Session Continuity

**Session 2026-04-27 (continuation — Phases 74 + 75 shipped via /gsd-autonomous):**

User invoked `/gsd-autonomous` to drive remaining phases end-to-end. Workflow shipped Phase 74 and Phase 75, then user requested stop after Phase 75 to resume tomorrow.

**Phase 74 — Test & CI Debt (shipped 2026-04-27)**

- 3 plans in single Wave 1 (parallel via worktree isolation, disjoint files_modified):
  - 74-01 (TEST-01): exported `ExpectedIndexesFor(apiPath, ot)` in `cmd/pdb-schema-generate/main.go` (thin wrapper over `generateIndexes`); rewrote `TestGenerateIndexes` as derivation-driven with `legacy_net_fixture` + `per_entity_from_schema_source` sub-tests across all 13 entities. Hand-rolled `slicesEqual` later replaced by stdlib `slices.Equal` indirectly via review fix. Commits `0d8fa18`, `45a7a5d`, `6de3c94`, `378fe65`.
  - 74-02 (TEST-02): dropped `$region` template variable from `pdbplus-overview.json` along with all 5 `{cloud_region=~"$region"}` panel selectors (introduced same day by commit `e0fc349`). Replaced `TestDashboard_RegionVariableUsed` with `TestDashboard_NoOrphanTemplateVars` structural invariant. Discovered 2nd orphan (`process_group`) and wired into panel 35's `service_namespace=~"$process_group"` selector instead of silently exempting. Commits `9df8fd2`, `5713d93`, `aeb3bc4`.
  - 74-03 (TEST-03): applied `filepath.Clean()` + canonical "operator-supplied by contract" leading comments at G304 sites in `internal/visbaseline/{redactcli,reportcli,checkpoint}.go`. Removed stale `//nolint:gosec` directives at 4/5 sites (Clean satisfies gosec → directives become nolintlint-stale). One targeted `//nolint:gosec // G122` retained at the WalkDir TOCTOU site. Commits `f3e0056`, `9f5aacb`, `96b249a`.
- Verification 13/13 must-haves passed (commit `97b2ca1`).
- Code review found 3 warnings + 4 info, auto-fix applied 3/3 warnings (`b67afef`, `33a6e79`, `7f31e06` + meta `46a95a1`):
  - WR-01: boundary-aware regex for orphan-template-var match (was unbounded `strings.Contains`)
  - WR-02: orphan check now walks raw JSON tree covering all string leaves (not just expr + datasource UID)
  - WR-03: dropped tautological `ExpectedIndexesFor` round-trip assertions (helper literally wraps generator)

**Phase 75 — Code-side Observability Fixes (shipped 2026-04-27)**

- 3 plans across 3 sequential waves (all 3 plans modify `cmd/peeringdb-plus/main.go` so worktree parallel execution would conflict):
  - 75-01 (OBS-01) per D-01: new `internal/sync/initialcounts.go` `InitialObjectCounts(ctx, *ent.Client)` (status-agnostic Count(ctx) on all 13 entity tables, fail-fast on error per GO-CFG-1). Wired into `cmd/peeringdb-plus/main.go:199` between `database.Open` and `pdbotel.InitObjectCountGauges`. 3 unit tests lock the contract. Commits `0c8ca6d`, `0eae6a1`, `5ea3e19`, `fca765d`.
  - 75-02 (OBS-02) per D-02: new `internal/otel/prewarm.go` `PrewarmCounters(ctx)` + `PeeringDBEntityTypes` exports. Pre-warms 4 per-type counters × 13 types + RoleTransitions × 2 directions = 54 baseline series (CONTEXT.md's "65" was a miscount; PLAN corrected to 52 + 2 = 54). Wired between syncWorker construction and StartScheduler spawn at `cmd/peeringdb-plus/main.go:293`. 3 unit tests. Commits `f2dcacc`, `9cd30f6`, `49edf98`, `5cf8e02`.
  - 75-03 (OBS-04) per D-03: empirical investigation refuted all 3 code-bug hypotheses for the production-only-/healthz observation. `routeTagMiddleware` works correctly for all 5 production route families. Root cause: sparse-traffic / Prometheus-staleness artifact. Fix shape (Shape 2 + doc-only): new 3-test E2E suite at `cmd/peeringdb-plus/route_tag_e2e_test.go` locks the http.route contract through production-shaped middleware chain (`captureLabelerMW → privacyTierLikeMW → routeTagMiddleware → mux`); doc-comment expansion on `routeTagMiddleware` documents WHY the middleware exists despite otelhttp v0.68.0 emitting `http.route` natively. Body unchanged. Investigation evidence trail in `OBS-04-INVESTIGATION.md` (207 lines). Commits `beb870d`, `1734fe6`, `23caf55`.
- Verification status: `human_needed` — 10/10 codebase truths green; 3/4 roadmap success criteria require post-deploy operator confirmation (gauge populates within 30s, zero-rate panels render `0` not `No data`, `count by(http_route)(...)` returns ≥5 distinct routes during normal traffic). Commit `c5119b7`.
- Code review found 4 warnings + 3 info, auto-fix applied 4/4 warnings (`c7d6657`, `0d6f01c`, `b430973`, `9210bf2` + meta `c710ac6`):
  - WR-01: TestPrewarmCounters_NoError now sets `OTEL_METRICS_EXPORTER=none` (consistent with sibling tests)
  - WR-02: InitialObjectCounts now checks `ctx.Err()` between 13 sequential queries (GO-CTX-2)
  - WR-03: PrewarmCounters guards against nil counters via `otel.Handle` (no panic, no docs lie)
  - WR-04: TestPeeringDBEntityTypes_ParityNote replaced with real set-equality assertion citing Phase 75 D-02

**Phase 75 deferred items for follow-up:**

- 3 INFO findings from review (out of `--auto` scope): IN-01 (slicesEqual reinvents stdlib), IN-02 (process_group var queries primary-only metric), IN-03 (filepath.Clean comments overstate Clean's role). Run `/gsd-code-review-fix 75 --all` to address if desired.
- 75-VERIFICATION.md "human_needed" operator checklist: 4 post-deploy queries to run after `fly deploy` to confirm OBS-01/02/04 land correctly in production. The phase is "verified" from a codebase contract perspective; live confirmation is deployment-time.

**Phase 76/77/78 status entering tomorrow's session:**

All three remaining phases have CONTEXT.md committed (`fe724fc` 2026-04-26) — no discuss step needed. Phase 77's hard dependency on Phase 75's OBS-04 is satisfied (E2E test proves http.route populates through production-shaped chain). Phase 76 (dashboard) and Phase 77 (telemetry audit) overlap on `deploy/grafana/` JSON edits — sequence carefully or accept potential re-merge work. Phase 78's UAT-01 (CSP DevTools) requires manual user step.

**Local commits since session start (Phase 74 + 75 work, 28 commits NOT YET PUSHED):**

```
c710ac6  docs(phase-75): code review fixes applied — 4/4 warnings fixed
9210bf2  fix(75-WR-04): replace TestPeeringDBEntityTypes_ParityNote with real assertion
b430973  fix(75-WR-03): PrewarmCounters guards against nil counters via otel.Handle
0d6f01c  fix(75-WR-02): InitialObjectCounts checks ctx.Err() between queries
c7d6657  fix(75-WR-01): TestPrewarmCounters_NoError sets OTEL_METRICS_EXPORTER=none
1747691  docs(phase-75): code review — 0 blocker, 4 warning, 3 info
c5119b7  docs(phase-75): verification — 10/10 codebase truths green, human_needed
23caf55  docs(75-03): complete OBS-04 http.route investigation plan
1734fe6  feat(75-03): lock OBS-04 http.route invariant via E2E + clarify doc
beb870d  docs(75-03): OBS-04 investigation — sparse-traffic, not a code bug
5cf8e02  docs(75-02): complete OBS-02 zero-rate counter pre-warm plan
49edf98  feat(75-02): wire PrewarmCounters into startup ordering
9cd30f6  feat(75-02): implement PrewarmCounters helper (GREEN)
f2dcacc  test(75-02): add failing prewarm tests (RED)
fca765d  docs(75-01): complete cold-start gauge population plan
5ea3e19  feat(75-01): seed objectCountCache from InitialObjectCounts at startup
0eae6a1  feat(75-01): add InitialObjectCounts helper for cold-start gauge
0c8ca6d  test(75-01): add failing tests for InitialObjectCounts (RED)
c1a4adc  docs(75): plan phase 75 — code-side observability fixes
46a95a1  docs(phase-74): code review fixes applied — 3/3 warnings fixed
7f31e06  fix(74-WR-03): drop tautological ExpectedIndexesFor assertions
33a6e79  fix(74-WR-02): orphan check walks raw JSON tree, covers all string leaves
b67afef  fix(74-WR-01): boundary-aware regex for orphan template-var match
5a1285d  docs(phase-74): code review — 0 blocker, 3 warning, 4 info
97b2ca1  docs(phase-74): verification passed — 13/13 must-haves verified
514654a  docs(phase-74): update tracking after wave 1 — all 3 plans complete
4c9d483  chore: merge executor worktree (worktree-agent-affe5281e4ccff6ce)
28c627d  chore: merge executor worktree (worktree-agent-ac164fad9e9353af2)
92a0a27  docs(74): create phase plan
```

Plus the prior 22 Phase 73 commits still local-only since `634c96a` — **50 unpushed commits total** as of stop time.

**Resume protocol for tomorrow's session:**

1. `/clear` to reset context (recommended — current run accumulated significant context).
2. Run `/gsd-autonomous --from 76` to drive remaining 3 phases + lifecycle in one continuous run. CONTEXT.md is locked for all 3 — no discuss prompts. Phase 78 will pause for the CSP DevTools step.
3. Alternatively run phase-by-phase: `/gsd-plan-phase 76` → `/gsd-execute-phase 76` → repeat for 77 and 78.
4. After all phases complete: `/gsd-audit-milestone` → `/gsd-complete-milestone v1.18.0` → `/gsd-cleanup`.
5. Push when ready: 50 local commits awaiting push. The Phase 75 OBS-01/02/04 fixes are NOT in production until pushed and `fly deploy` runs. Production `pdbplus_data_type_count` will continue to show false zeros until deploy. You can push partial milestone progress (commits already on main locally) without breaking anything since each commit is atomic.

**Pre-existing untracked editor dirs in working tree:** `.idea/`, `.vscode/` — pre-existed at session start, not created by this session. Add to `.gitignore` if desired or leave alone.

---

**Prior session 2026-04-26 (Phase 73 plan→execute→verify arc):**

1. `/gsd-next` after `/clear` → routed to `/gsd-plan-phase 73` (CONTEXT.md locked, no plans yet).
2. Skipped research (rich CONTEXT.md, well-understood bug fix); skipped pattern-mapper (CONTEXT.md plan hints already named analogue files including `260426-pms` precedent + `cmd/pdb-schema-generate/main.go` `isNameField`).
3. Planner produced 2 plans (73-01, 73-02) in single Wave 1, disjoint `files_modified`, both autonomous. Initial commit `6266b6b`.
4. Plan-checker first pass: 1 BLOCKER + 4 WARNINGS. Blocker: D-03 unit test descoped into Subtask 2C advisory prose with no enforcing acceptance criterion. Warnings: D-02 audit table omitted "no NotEmpty found" rows for 7 of 13 entities; Task 2 verify command had `! cmd1 | cmd2` shell-quoting fragility; Plan 73-01 referenced nonexistent Plan 73-03; Task 1 acceptance said "exactly 2" while verify said "≥ 2".
5. Revision iteration 1/3: planner promoted unit test to Task 4 with `<files>`/`<verify>`/`<acceptance_criteria>`/`<done>` + `must_haves.artifacts` entry; expanded audit table to all 13 entities; rewrote shell command to `test "$(... \| grep -cF ...)" -eq 0`; removed stale 73-03 ref; reconciled "≥ 2" form. Commit `b3f374d`.
6. Plan-checker second pass: VERIFICATION PASSED. All 5 prior issues fixed, no new issues introduced.
7. Auto-advanced to execute-phase via `auto_advance: true` config + `--no-transition` flag.
8. Wave 1 spawn: 2 parallel worktree executors (sequential dispatch, run_in_background). 73-01 finished in ~11min (4 commits including SUMMARY); 73-02 finished in ~19min (6 commits including SUMMARY). Both Self-Checks passed.
9. Worktree merge ceremony: pre-merge deletion check empty for both; both merged with `--no-ff --no-edit`; bulk-deletion audit clean; orchestrator-owned files (STATE.md, ROADMAP.md) restored from backup; worktrees unlocked + removed; branches deleted. Merge commits `22199a3` (73-01) + `d830501` (73-02).
10. Post-merge gates: `go build ./...` clean; `go test -count=1` on affected packages all OK; `go generate ./...` zero drift on first post-merge run (idempotent).
11. ROADMAP.md plan-progress updated for both plans (`5ac40c8`).
12. Verifier spawned: 18/18 must-haves PASSED. VERIFICATION.md written. Phase marked complete via `gsd-sdk query phase.complete 73` (`8684f2c`).
13. Execute-phase ended cleanly per `--no-transition` (no auto-advance to phase 74).

**Out-of-scope finding from 73-02 (informational, not blocking):** Existing Organization regression guard at `cmd/pdb-schema-generate/main_test.go:245-251` uses 4-tab source indent vs generator's 3-tab output — would never match if NotEmpty re-appeared on Org. The new Phase 73 Poc + role guards added by 73-02 use correct `\n\t\t\t` escape strings. Recorded in 73-VERIFICATION.md as a follow-up candidate.

**Local-only commits** (22 since `634c96a`, NOT YET PUSHED):

```
8684f2c  docs(phase-73): complete phase execution
5ac40c8  docs(phase-73): update tracking after wave 1
d830501  chore: merge executor worktree (73-02)
22199a3  chore: merge executor worktree (73-01)
01a6bc2  docs(73-02): complete code defect fixes plan
25cc834  test(73-02): TestPoc_RoleEmptyAcceptedByValidator unit guard
b4d2b84  test(73-02): TestSync_IncrementalRoleTombstone httptest guard
32ca6c7  docs(73-01): complete plan
0e98048  chore(73-02): regenerate ent artefacts
e7f8f20  docs(73-01): close DEFER-70-06-01
d2ba190  feat(73-02): drop NotEmpty for poc.role at codegen layer
e4c4478  test(73-01): un-skip E2E + flip parity canary
0dc7b4a  test(73-02): RED phase
583caba  fix(73-01): pin Campus SQL table name via sibling-file mixin
b3f374d  docs(73): revise plans
6266b6b  docs(73): plan phase 73
4a97f57  docs(state): capture session continuity before /clear (prior session)
fe724fc  docs(planning): lock CONTEXT.md for v1.18.0 phases 73-78
41ef406  docs: create milestone v1.18.0 roadmap
0e955ca  docs: define milestone v1.18.0 requirements
4be90f4  docs: start milestone v1.18.0 Cleanup & Observability Polish
d2acb43  docs(planning): retarget next milestone to v1.18.0
```

The user said "/clear before I continue" — review the 22 commits and push when appropriate. Phase 73 fixes are NOT deployed until pushed and `fly deploy` runs.

**Production state:** v1.17.0 deployed across 8-machine asymmetric Fly fleet (1 primary lhr + 7 replicas). Default sync mode = `incremental` running ~15min cadence. BUG-01 + BUG-02 fixes are local-only; production still has the bugs (campus traversal returns 500; poc.role NotEmpty would block any future role-empty tombstone). Dashboard at `https://dotwaffle.grafana.net/d/pdbplus-overview` (do NOT commit URL to repo per memory `feedback_no_pii_in_repo.md`).

**Ready for next session:** any of `/gsd-plan-phase 74`, `/gsd-plan-phase 75`, or `/gsd-plan-phase 78` (all three independent — could plan all three in parallel sessions). Phase 76 plan can also start (its dep on 75 is soft); Phase 77 should wait until 75 ships (OBS-07 hard-deps on OBS-04's `http.route` populating). All five remaining phases have CONTEXT.md locked (committed `fe724fc`) — no need to re-discuss.

`/gsd-progress` will show updated milestone status; `/gsd-ship` would package the local commits for review/PR if user wants to push before continuing.
