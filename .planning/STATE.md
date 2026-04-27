---
gsd_state_version: 1.0
milestone: v1.18.0
milestone_name: Cleanup & Observability Polish
status: verifying
last_updated: "2026-04-27T00:02:05.306Z"
last_activity: 2026-04-27
progress:
  total_phases: 6
  completed_phases: 3
  total_plans: 8
  completed_plans: 8
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-22)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

**Current focus:** Phase 75 — Code-side Observability Fixes

## Current Position

Phase: 75 (Code-side Observability Fixes) — COMPLETE
Plan: 3 of 3 — all shipped (OBS-01, OBS-02, OBS-04)
Status: Phase complete — ready for verification
Last activity: 2026-04-27 -- Phase 75 Plan 03 (OBS-04 http.route investigation) shipped (commits beb870d, 1734fe6)

## v1.18.0 Phase Map

| Phase | Goal | Requirements | Depends on | Status |
|-------|------|--------------|------------|--------|
| 73 — Code Defect Fixes | Fix campus inflection 500 + drop `poc.role` NotEmpty validator | BUG-01, BUG-02 | — | ✓ shipped 2026-04-26 |
| 74 — Test & CI Debt | Three deferred test failures + 5 lint findings cleared | TEST-01, TEST-02, TEST-03 | — | Ready to plan |
| 75 — Code-side Observability Fixes | Cold-start gauge, zero-rate counter pre-warm, http.route middleware | OBS-01, OBS-02, OBS-04 | — | ✓ Phase complete 2026-04-27 (3/3 plans shipped: OBS-01 ✓, OBS-02 ✓, OBS-04 ✓) |
| 76 — Dashboard Hardening | `service_name` filter sweep + post-canonicalisation metric flow confirmation | OBS-03, OBS-05 | Phase 75 (soft) | Ready to plan (planning before 75 complete is OK; Phase 75 dep is soft) |
| 77 — Telemetry Audit & Cleanup | Loki log-level audit + Tempo trace sampling/batching review | OBS-06, OBS-07 | Phase 75 (hard for OBS-07) | Plan after Phase 75 ships (OBS-07 needs http.route populated) |
| 78 — UAT Closeout | v1.13 CSP + headers verification, v1.5 Phase 20 archive | UAT-01, UAT-02, UAT-03 | — | Ready to plan |

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

**This session 2026-04-26 (Phase 73 plan→execute→verify arc):**

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
