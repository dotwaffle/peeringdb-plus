---
gsd_state_version: 1.0
milestone: null
milestone_name: null
status: between-milestones
last_updated: "2026-05-03T14:15:00Z"
last_activity: 2026-05-03 -- Completed quick task 260503-j6e: modern-Go modernization bundle. 7 atomic commits across 19 hand-written files: slices.SortFunc tree-wide (Go 1.21), range integer + b.Loop() tree-wide (Go 1.22 + 1.24), errors.AsType[T] (Go 1.26), typed atomic.Int32 in capture_test, wg.Go for fan-out (Go 1.25), WithTimeout collapse of cancel-after-sleep, and the codebase's first testing/synctest use in termrender/freshness_test (Go 1.25). Plan-commit 3 (t.Context()) skipped entirely: both audit sites use mid-test cancel for explicit timing, not deferred cleanup. One range-int site in internal/sync/bypass_audit_test.go also skipped: tokenizer mutates i mid-body, range-int silently breaks the state machine. All gates green (build, vet, test -race full tree, golangci-lint, govulncheck). synctest beachhead sets the pattern for queued worker_test.go and caching_test.go conversions per audit; those defer to dedicated quick tasks with proper UAT.
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-22)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Current Position

**Between milestones.** v1.18.0 (Cleanup & Observability Polish) shipped + archived 2026-04-27. Six phases (73-78), 14 plans, 15 requirements all closed. Tag `v1.18.0` will be created at `/gsd-complete-milestone` end. No active milestone — start v1.19+ via `/gsd-new-milestone` when ready.

**Latest deploy:** commit `846c3df` (mid-Phase-77/78 deploy on 2026-04-27T04:11:12Z). All 8 fleet machines healthy. Phase 78's docs/tests-only changes don't require a separate deploy.

**Production state:** v1.17.0 release tag (pre-v1.18.0). Default sync mode = `incremental` running ~15min cadence on the primary. Fleet is the v1.15+ asymmetric configuration: 1 primary `lhr` (persistent volume) + 7 replicas `iad/lax/nrt/sin/syd/gru/jnb` (ephemeral, LiteFS HTTP cold-sync on boot).

## Outstanding Human Verification

All v1.18.0 Phase 78 items closed 2026-04-27 — see archived `.planning/milestones/v1.18.0-phases/78-uat-closeout/UAT-RESULTS.md`.

Remaining backlog (out of scope for v1.18.0; deferred to a future "UI verification sweep" milestone):

- v1.6 / v1.7 / v1.11 UI/visual items (~33 items combined). See `memory/project_human_verification.md`.

## Operator follow-ups (optional, non-blocking)

These were surfaced during v1.18.0 execution and are not required for milestone closure:

- **CSP enforcement flip.** Plan 78-02 verified the CSP policy autonomously (PASS). The operator can flip `PDBPLUS_CSP_ENFORCE=true` via `fly secrets set` at any maintenance window with no expected behavioural change. UAT-RESULTS.md provides the rollback recipe if any unexpected violations surface.
- ~~**`OTEL_BSP_*` documentation drift.**~~ Closed by 260503-imn — explicit options dropped, env interface re-engaged.
- **Empirical Tempo trace size validation.** Phase 77 UAT Test 9 (max trace size <2 MB via TraceQL) is structurally argued but not directly empirical (the grafana-cloud MCP didn't expose Tempo TraceQL tools in the verifying session). Operator can validate via the Grafana UI when convenient using the queries listed in 77-UAT.md.

### Quick Tasks Completed

| # | Description | Date | Commit | Status | Directory |
|---|-------------|------|--------|--------|-----------|
| 260427-ojm | replace OnSyncComplete `len(items)` with current-table `Count(ctx)` for `pdbplus_data_type_count` gauge | 2026-04-27 | 18b2337 | | [260427-ojm-replace-onsynccomplete-len-items-with-cu](./quick/260427-ojm-replace-onsynccomplete-len-items-with-cu/) |
| 260427-vvx | build-tag-gated `cmd/loadtest` tool — endpoints sweep + sync simulator + soak mode for deployed instances on Fly.io | 2026-04-27 | d68a8eb | | [260427-vvx-loadtest-script](./quick/260427-vvx-loadtest-script/) |
| 260428-2zl | fk-parity-with-upstream — live FK backfill in `fkCheckParent`, `?since=1` incremental-sync bootstrap (REVERTED in v1.18.3 — tripped upstream throttle), rate-limited HTTP transport (configurable RPS, bounded 429 retry, WAF detection), removed `markStaleDeleted*` inference-by-absence soft-delete, NetworkIxLan side-FK null-on-miss, dropped redundant NetworkIxLan.ix_id check. v1.18.2 deployed → rolled back. | 2026-04-28 | 0b8ca14 | | [260428-2zl-fk-parity-with-upstream-live-fk-backfill](./quick/260428-2zl-fk-parity-with-upstream-live-fk-backfill/) |
| 260428-2zl-hotfix | v1.18.3 hotfix — reverted v1.18.2 since=1 bootstrap, decoupled `GetCursor` from `last_status`, added recursive grandparent FK backfill, added per-cycle backfill timeout (`PDBPLUS_FK_BACKFILL_TIMEOUT=5m` default). Deployed + verified healthy: carrier 403 → org 18985 chain backfilled in first sync cycle. | 2026-04-28 | 0069091 | | (in 260428-2zl dir) |
| 260428-5xt | fk-backfill-dataloader — batch missing-parent FK fetches into one `?id__in=` request per parent type, replacing v1.18.3 per-row pattern. New `peeringdb.Client.FetchByIDs` with 100-ID URL chunking. Recursive grandparent backfill now also batched (BFS by parent type). Critical for truncate-and-resync recovery and long-downtime catch-up; steady-state behavior unchanged. | 2026-04-28 | 5c06c24 | | [260428-5xt-fk-backfill-dataloader-batch-missing-par](./quick/260428-5xt-fk-backfill-dataloader-batch-missing-par/) |
| 260428-84z | add-loadtest-ramp-subcommand — `loadtest ramp` finds inflection point per API surface (pdbcompat/entrest/graphql/connectrpc/webui detail) against a peeringdb-plus deployment. Ramps C=1 ×1.5/2s, triggers on p95 > 2× baseline OR p99 > 1s OR err > 1%, holds 10s past inflection, runs surfaces sequentially, emits markdown table per surface to stdout. Hermetic httptest-driven tests with synthetic latency injection. Operator/developer tool — not deployed. | 2026-04-28 | 5016eb4 | | [260428-84z-add-loadtest-ramp-subcommand](./quick/260428-84z-add-loadtest-ramp-subcommand/) |
| 260428-blj | wire `cfg.Verbose` into ramp mode (prefetch IDs/ASNs summary line + per-error log with path/status/err) and filter `context.Canceled` from `summariseStep` error count so end-of-step cancellation no longer pollutes the inflection signal. Plumbs `stdout io.Writer` through `rampOneSurface` / `runRampStep`; cancelled samples are dropped entirely so `Samples`/`RPS` reflect real measurements. Hermetic httptest-driven tests. Operator/developer tool — not deployed. | 2026-04-28 | e51aa61 | | [260428-blj-wire-cfg-verbose-into-ramp-mode-print-pr](./quick/260428-blj-wire-cfg-verbose-into-ramp-mode-print-pr/) |
| 260428-eda | sync optimization bundle — 6 atomic commits dropping steady-state sync wall-time from ~117s to a target <30s: SQLite DSN pragmas (synchronous=NORMAL, cache_size=-32000, temp_store=MEMORY); per-tx `cache_spill = OFF` to hold dirty pages in cache through 60s upsert burst; `InitialObjectCounts` collapsed from 13 sequential `Count(ctx)` calls to one UNION ALL query; observability spans for the 23s post-commit tail (`sync-commit`, `sync-finalize`, `sync-cursor-updates`, `sync-on-complete`, `sync-record-status`); cursor writes folded into the main upsert tx (D-19 atomicity strengthened — was 14 LiteFS commits per cycle, now 1) with new `pdbplus.sync.cursor_write_caused_rollback` OTel attr; per-row skip-on-unchanged via `excluded.updated > existing.updated` predicate on all 13 OnConflict sites (steady-state idle sync becomes near-no-op at SQLite level). Plus SEED-005 follow-up for periodic full-sync convergence. 11/11 must-haves verified. | 2026-04-28 | cfd47f8 | Verified | [260428-eda-sync-optimization-bundle-spans-cursor-in](./quick/260428-eda-sync-optimization-bundle-spans-cursor-in/) |
| 260428-mu0 | replace meta.generated-based sync cursor with per-table `MAX(updated)` high-water mark; add `PDBPLUS_FULL_SYNC_INTERVAL` escape hatch (default 24h) with `sync_status.mode` column for "last successful full sync" tracking. Fixes v1.13 regression (commit 18d3735) where every other 15-min incremental cycle did a full ~270k bare-list re-fetch (PeeringDB omits `meta.generated` on `?since=` responses → cursor stored as zero → next cycle falls into full path; production confirmed alternating 1310/270190 total_objects every 15 min on `peeringdb-plus` primary). Two atomic commits: cursor mechanism swap (deprecates `GetCursor`/`UpsertCursor` but keeps `sync_cursors` CREATE TABLE for rollback) + 24h escape hatch (idempotent ALTER TABLE adds `mode` column). 9 must-haves passing. Regression-lock test (`TestSync_TwoCycle_NoFullRefetch`) in place. | 2026-04-28 | 8841d32 | Awaiting Verification | [260428-mu0-replace-meta-generated-based-sync-cursor](./quick/260428-mu0-replace-meta-generated-based-sync-cursor/) |
| 260503-fw7 | trim otelhttp + otelconnect cardinality via SDK Views in `internal/otel/provider.go`. Drops the entire `rpc.server.*` family (unused by every dashboard/alert), coarsens `http.server.request.duration` buckets to `{0.01, 0.05, 0.25, 1, 5}` (replacing the pre-existing rpc duration View), and strips `http.method` / scheme / `server.address` attributes via `AllowKeysFilter`. Estimated per-machine reduction: ~6,800 → ~1,200 active series (~80%); fleet-wide ~50k → ~10k. Single atomic commit; new `views_test.go` uses `ManualReader` for hermetic assertions; all gates green (test -race, build, vet, golangci-lint). Verified dashboard panels (`http_route` + `http_response_status_code` labels survive — `=~"5.."` regex still matches). | 2026-05-03 | dcd400d | Awaiting post-deploy Grafana verification | [260503-fw7-trim-otelhttp-otelconnect-cardinality-vi](./quick/260503-fw7-trim-otelhttp-otelconnect-cardinality-vi/) |
| 260503-huo | invert per-route OTel head sampler default in `internal/otel/provider.go`. `DefaultRatio` drops from `in.SampleRate` (default 1.0) to hardcoded `0.01`; `in.SampleRate` redirected onto the four known app surfaces (`/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql`) so the operator escape hatch keeps working for legitimate traffic; new explicit deny-prefixes `/.` (0.001) and `/wp-` (0.001) catch dotfile / WordPress probes. Triggered by a vulnerability scanner (`45.148.10.238`, UA `SecurityScanner/1.0`) that consumed ~2 GB of the 50 GB/month free-tier trace budget on day 2 of the month, peaking at 384 KB/s vs the 500 KB/s free-tier cap and tripping `live_traces_exceeded` discards. Single atomic commit; new `TestSetup_InvertedSamplerDefault` regression-lock test parameterises `SampleRate` over `{0.0, 0.5, 1.0}` to prove the wiring carries through. All gates green (test -race, build, vet, golangci-lint). Estimated next-scanner-sweep reduction: ~100x (~2 GB → ~20 MB). | 2026-05-03 | 03cc619 | Awaiting post-deploy Tempo verification | [260503-huo-invert-sampler-default](./quick/260503-huo-invert-sampler-default/) |
| 260503-imn | doc-drift cleanup bundle — three atomic commits clearing v1.18.0-closeout backlog: (1) `otel: drop redundant OTEL_BSP option overrides` (`d2e7496`) — drops `WithBatchTimeout(5s)` + `WithMaxExportBatchSize(512)` in `internal/otel/provider.go`, re-engages SDK env defaults (which already match), and rewrites the misleading comment; (2) `docs: close DEFER-70-06-01 in CLAUDE.md` (`75823c2`) — replaces stale "Known gap / Fix queued" paragraph with the actual closure note (Phase 73 BUG-01, `ent/schema/campus_annotations.go` sibling-file mixin); (3) `seeds: archive SEED-001 to consumed/` (`44e883a`) — `git mv` to `consumed/`, `status: ready` → `status: consumed`, adds `consumed_in: v1.17.0` + `consumed_by: quick-task-260426-pms`. Behaviour unchanged at default config; gates only ran for fix 1 (Go-touching) — build / vet / test -race / golangci-lint all PASS. | 2026-05-03 | 47401e2 | Doc-only — no deploy gate | [260503-imn-doc-drift-cleanup-bundle](./quick/260503-imn-doc-drift-cleanup-bundle/) |
| 260503-j6e | modern-Go modernization sweep — 7 atomic commits across 19 hand-written files: `slices: replace sort.Slice with slices.SortFunc tree-wide` (`36ee79d`, Go 1.21); `loops: range integer and b.Loop() tree-wide` (`e17698f`, Go 1.22 + 1.24); `errors: use AsType[T] for type-asserted error checks` (`0ad5b4c`, Go 1.26); `visbaseline: use typed atomic.Int32 in capture_test` (`7645e13`, Go 1.19); `loadtest: use wg.Go for discover fan-out` (`9a67274`, Go 1.25); `visbaseline: collapse cancel-after-sleep to WithTimeout` (`575d3c8`); `termrender: use testing/synctest for deterministic test` (`138ece5`, Go 1.25). Plan-commit 3 (`t.Context()`) skipped — both audit sites use mid-test cancel. One range-int site in `internal/sync/bypass_audit_test.go` skipped — tokenizer mutates `i` mid-body. All final-state gates green (build, vet, test -race full tree, golangci-lint, govulncheck). First `testing/synctest` use; sets pattern for queued `worker_test.go` and `caching_test.go` conversions. | 2026-05-03 | 6a6b39a | Doc/refactor only — no deploy gate | [260503-j6e-modern-go-bundle](./quick/260503-j6e-modern-go-bundle/) |

## Accumulated Context

### Seeds

- **SEED-001** — incremental sync evaluation. **Consumed 2026-04-26** by quick task 260426-pms (default `PDBPLUS_SYNC_MODE=incremental` shipped in v1.17.0).
- **SEED-003** — primary HA hot-standby. **Dormant.** Extended 2026-04-27 with the IAD-preferred-primary variant (cost analysis verified against `fly status`; correction that Consul leases are already configured; trigger added for sync-upstream-RTT regression). See `.planning/seeds/SEED-003-primary-ha-hot-standby.md`.
- **SEED-004** — tombstone GC. **Dormant.** Triggers haven't fired (storage growth <5% MoM, tombstone ratio <10%, no operator request). See `.planning/seeds/SEED-004-tombstone-gc.md`.

### Notes for next milestone

- Run `/gsd-new-milestone` to start v1.19+. Theme TBD — possible directions surfaced during v1.18.0 execution: UI verification sweep (~33 items), tag-and-release hygiene (the OTEL_BSP doc drift + similar comment-vs-code mismatches), or a feature cycle now that observability is solid.
- A fresh `.planning/REQUIREMENTS.md` will be created by `/gsd-new-milestone`. The v1.18.0 requirements are archived at `.planning/milestones/v1.18.0-REQUIREMENTS.md`.
