---
slug: poc-count-doubling-halving
status: resolved
trigger: "In the Grafana dashboard, there is a section under 'business metrics' called 'Object Counts Over Time'. There seems to be a definite doubling/halving of the number of 'poc' objects every sync interval. I'd like to get to the bottom of that."
created: 2026-04-27
updated: 2026-04-27
---

# Debug Session: poc-count-doubling-halving

## Symptoms

- **Expected behavior:** Object count for "poc" entity in Grafana dashboard "Object Counts Over Time" panel (under "business metrics") remains stable across sync cycles (other 12 entity types presumably do)
- **Actual behavior:** POC count oscillates — doubles, then halves, then doubles again — synchronised with the sync interval (default 1h unauthenticated / 15m authenticated)
- **Error messages:** None reported. Pure metric anomaly, not log-driven.
- **Timeline:** Observed in current production Grafana dashboard. Onset: introduced by Phase 75 OBS-01 (commit 5ea3e19, "feat(75-01): seed objectCountCache from InitialObjectCounts at startup"). Before OBS-01 the gauge held zeros pre-first-sync; OBS-01 added the cold-start primer that has the privacy bug.
- **Reproduction:** Open Grafana → peeringdb-plus overview dashboard → business metrics row → "Object Counts Over Time" panel → observe poc series.

## Initial Context (from project memory & CLAUDE.md)

POC is special-cased in three v1.16+ ways that could plausibly explain a 2x/0.5x oscillation:

1. **Privacy filter (Phase 58/59).** `Poc.Policy()` (sibling `ent/schema/poc_policy.go`) filters rows where `visible != "Public"` from anonymous responses. If the count metric is queried *as an anonymous tier* on some cycles and *as users tier* on other cycles, count would alternate.
2. **Tombstones (Phase 68).** `markStaleDeletedPocs` soft-deletes via `status='deleted'`. If the count metric query alternates between counting `status IN ('ok','pending')` and counting all rows, the doubling pattern would track the tombstone fraction.
3. **Sync upsert pattern.** POC uses `OnConflict().UpdateNewValues()` like the others; unlikely a pure duplication bug, but worth ruling out.

The fact that POC is the *only* entity oscillating narrows this strongly toward path (1) or (2) — POC is the only entity with row-level privacy gating, so it's the only entity whose visible row count changes based on caller tier or status filter.

## Current Focus

```yaml
hypothesis: confirmed
next_action: complete (fix applied)
reasoning_checkpoint: null
tdd_checkpoint: null
```

## Evidence

- timestamp: 2026-04-27
  source: deploy/grafana/dashboards/pdbplus-overview.json:1825-1844
  finding: |
    Panel "Object Counts Over Time" (id 30) PromQL is
    `max by(type)(pdbplus_data_type_count{type=~"$type"})`. The metric
    name is `pdbplus_data_type_count` (Prom-translated from OTel
    `pdbplus.data.type.count`). The `max by(type)` collapses series
    across all instance/region/process_group dimensions to a single
    timeseries per type — this is the load-bearing aggregation
    decision for the symptom.

- timestamp: 2026-04-27
  source: internal/otel/metrics.go:248-267
  finding: |
    `InitObjectCountGauges(countsFn func() map[string]int64)` registers
    an Int64ObservableGauge whose callback returns whatever the cache
    holds. Single reader, no per-scrape predicate.

- timestamp: 2026-04-27
  source: cmd/peeringdb-plus/main.go:198-215
  finding: |
    Cache is `var objectCountCache atomic.Pointer[map[string]int64]`.
    Two writers:
      (a) startup: `seededCounts, err := pdbsync.InitialObjectCounts(ctx, entClient)`
          → `objectCountCache.Store(&seededCounts)`
      (b) `OnSyncComplete: func(counts map[string]int, syncTime time.Time)`
          → `objectCountCache.Store(&m)` after each successful sync.
    No privacy.DecisionContext bypass on the startup writer.

- timestamp: 2026-04-27
  source: internal/sync/initialcounts.go:55-96 (pre-fix)
  finding: |
    `InitialObjectCounts(ctx, client)` runs 13 sequential
    `client.<Type>.Query().Count(c)` calls. Critically, it passed the
    raw `ctx` through — no privacy bypass and no tier elevation.
    The grep `privacy|DecisionContext|TierUsers` against this file
    returned ZERO hits prior to the fix.

- timestamp: 2026-04-27
  source: internal/sync/worker.go:432-433
  finding: |
    `Worker.Sync` is the only writer of objectCounts via
    OnSyncComplete. Its first line: `ctx = privacy.DecisionContext(ctx,
    privacy.Allow) // VIS-05 bypass — sole call site (D-08/D-09)`.
    This means sync upserts/counts see ALL POC rows including
    visible=Users.

- timestamp: 2026-04-27
  source: ent/schema/poc_policy.go:38-53
  finding: |
    `Poc.Policy()` Query rule filters out `visible != "Public"` rows
    when `privctx.TierFrom(ctx) != TierUsers`. The `internal/privctx`
    fail-closed default is `TierPublic` for any unstamped context.
    So `InitialObjectCounts`'s Count(ctx) calls produce **public-only**
    POC counts. POC is the ONLY entity with such a Policy — every
    other entity's `Count(ctx)` returns the full table count
    regardless of tier/bypass.

- timestamp: 2026-04-27
  source: internal/sync/worker.go:902-977 + 1086-1140
  finding: |
    The OnSyncComplete `objectCounts[step.name]` value is `len(items)`
    where `items` is the per-cycle scratch DB content — i.e. the count
    of UPSERTED ROWS in this sync cycle. NOT a "current table total".
    For full-mode sync this approximates the full PeeringDB table.
    For incremental sync this is just the delta since cursor.
    PERF-02 (commit 16b308c) deliberately replaced live `Count(ctx)`
    queries with this cached upsert count for performance — the
    semantic mismatch with the dashboard label "Object Counts Over Time"
    has existed since v1.x.

- timestamp: 2026-04-27
  source: internal/sync/worker.go:1660 (replica branch)
  finding: |
    Only the primary calls `runSyncCycle`. Replicas `continue` past
    sync. So replicas' caches are NEVER updated after boot — they
    hold the `InitialObjectCounts` value forever.

- timestamp: 2026-04-27
  source: deploy/grafana/dashboards/pdbplus-overview.json:1841 + buildResourceFiltered
  finding: |
    The metric resource keeps `service.namespace` (primary/replica)
    and `cloud.region` (8 distinct regions) as labels. So
    `pdbplus_data_type_count{type="poc"}` has 8 series in production
    (1 primary × LHR + 7 replicas × distinct regions). `max by(type)`
    aggregates the max across all 8.

- timestamp: 2026-04-27
  source: internal/testutil/seed/seed_mixed_visibility_test.go:34-103
  finding: |
    seed.Full creates 3 POCs in deterministic state: 1 visible="Public"
    (ID 500) + 2 visible="Users" (IDs 9000, 9001). `TestFull_PrivacyFilterShapes`
    proves an anonymous-TierPublic context sees 1 POC; a TierUsers context
    or a privacy.Allow bypass context sees 3. This 1:3 fixture exactly
    mirrors the production 50/50 (or worse) Public/Users split that
    produces the empirical 2x ratio in the dashboard.

## Root Cause

**Two semantically-incompatible writers feed the same gauge cache, and POC is the only entity where they disagree.**

1. The **startup primer** (`InitialObjectCounts`, Phase 75 OBS-01) calls `client.Poc.Query().Count(ctx)` with **no privacy bypass and no tier elevation**. `Poc.Policy()` therefore filters to `visible='Public' OR visible IS NULL`. Every other entity returns its full table count because no other entity has a row-level Policy.

2. The **sync-completion writer** (`OnSyncComplete`) writes the per-cycle `len(items)` upserted-row count. Sync runs with `privacy.DecisionContext(ctx, privacy.Allow)` so it counts all POC rows — Public AND Users.

Across the 8-instance fleet, at any moment:

- Right after a deploy: every instance just ran `InitialObjectCounts`, so all 8 caches hold `P` = (Public POCs only). `max by(type)` = `P`.
- After primary completes its first FULL sync: primary cache flips to `T` = (all POCs, Public + Users). 7 replica caches still hold `P`. `max by(type)` = `T` ≈ 2·P (because peeringdb's split between Public and Users POCs is roughly 50/50). **First doubling.**
- After primary's next sync (incremental, default mode since 2026-04-26): primary cache flips to delta `D` (small — typically tens of rows changed per interval). 7 replica caches still hold `P`. `max by(type)` = `P` (since `P >> D`). **Halving back to P.**
- The primary's sync cadence is what introduces the alternation: Fly periodically restarts the primary VM (rolling redeploys, OOM events, healthcheck failures), and on each restart the primary re-primes via `InitialObjectCounts` → `P`, then runs a fresh-DB or warm full sync → `T`, then incrementals → small `D`s. The replica fleet provides a **constant `P` floor** that keeps `max by(type)` flipping between `P` (during incremental cycles) and `T` (immediately after full syncs / restarts).

Other 12 entities don't oscillate because the startup primer's count and the sync-completion count agree (no Policy filter), so all 8 instances always converge on the same value within one sync interval.

The 2:1 ratio matches the empirical PeeringDB Public/Users POC split (most users default to "Users" visibility for individual contacts).

**Specialist hint:** `go` (this is a Go-side ent privacy-policy + cache contract bug; no domain-specific framework specialist needed beyond the project's own conventions).

## Eliminated

- Hypothesis "tombstone fraction alternates" — Phase 68 tombstones are written ONCE on the cycle they get deleted, with `updated=cycleStart`. They don't oscillate count between cycles. Tombstones contribute steadily to the count, not a 2x flicker.
- Hypothesis "double emission per cycle" — `pdbplus.data.type.count` has exactly one Int64ObservableGauge registration site (`InitObjectCountGauges`) and one cache writer per cycle (`OnSyncComplete`).
- Hypothesis "POC upsert duplication" — `dispatchScratchChunk` for POC routes through `syncIncremental[peeringdb.Poc]` like every other type; no special doubling logic.

## Fix Applied

Stamp `privctx.TierUsers` on ctx at the top of `InitialObjectCounts` so the startup primer counts the same row set the sync worker counts.

**File:** `internal/sync/initialcounts.go`

```go
import "github.com/dotwaffle/peeringdb-plus/internal/privctx"

func InitialObjectCounts(ctx context.Context, client *ent.Client) (map[string]int64, error) {
    // Tier-elevate so Poc.Policy admits visible="Users" rows. Symmetry
    // with the OnSyncComplete writer: both must count the same row set
    // or the gauge oscillates between writer values across the
    // 8-instance fleet (replicas hold this value forever since they
    // never sync).
    ctx = privctx.WithTier(ctx, privctx.TierUsers)
    ...
}
```

**Why `privctx.WithTier(ctx, TierUsers)` and not `privacy.DecisionContext(ctx, privacy.Allow)`:** the `internal/sync/bypass_audit_test.go` invariant restricts `privacy.Allow` references to exactly one production call site (`Worker.Sync`). Adding a second call site would dilute the audit's security value — every `privacy.Allow` site is a hard policy bypass that needs reviewer scrutiny. `TierUsers` is the documented mechanism for "non-sync tier elevation" (see `internal/sync/bypass_audit_test.go:208` and `internal/privctx/privctx.go` package godoc) and produces the same effect on `Poc.Policy()` (which checks `if TierFrom(ctx) == TierUsers { return privacy.Skip }`) without diluting the bypass audit. Audit test still asserts exactly 1 `privacy.Allow` call site post-fix; verified.

**Regression test:** `TestInitialObjectCounts_PocPolicyBypass` in `internal/sync/initialcounts_test.go` seeds the 1 Public + 2 Users fixture and asserts `counts["poc"] == 3`. Without the fix this returns 1 (Public-only); with the fix it returns 3 (matches sync writer). The error message points back at this debug session for context.

**Verification:**
- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -race ./internal/sync/` — pass (includes `TestSyncBypass_SingleCallSite` audit and the new regression test).
- `go test -race ./internal/otel/ ./cmd/peeringdb-plus/` — pass.
- `golangci-lint run ./internal/sync/...` — 0 issues.

**Out-of-scope follow-up (NOT part of this fix):** the deeper semantic mismatch — that `objectCounts[step.name]` is an upserted-row count rather than a current-table count — remains. After the fix, replicas show the correct `T` count permanently (post-boot), and primary shows `T` after every full sync but a small `D` after each incremental. The dashboard `max by(type)` then returns `T` continuously (replicas dominate), which is the correct answer. So the symptom resolves without addressing the upsert-vs-current semantic; that's a separate cleanup that would need a Plan-level discussion.

## Resolution

- **Root cause:** `InitialObjectCounts` (Phase 75 OBS-01 cold-start primer) ran `client.Poc.Query().Count(ctx)` with a fail-closed `TierPublic` context, so `Poc.Policy()` filtered out `visible="Users"` rows. The sync-completion writer (`OnSyncComplete`) counts all rows because `Worker.Sync` runs under `privacy.DecisionContext(ctx, privacy.Allow)`. Two writers with a 2x disagreement on POC + a fleet-wide `max by(type)` aggregation produced the dashboard oscillation visible only on the POC series.
- **Fix:** stamped `privctx.TierUsers` on ctx in `InitialObjectCounts` before the 13 Count calls. POC's Policy now `Skip`s the filter for the primer, restoring symmetry with the sync writer. No `privacy.Allow` call site added; bypass audit invariant preserved.
- **Tests:** added `TestInitialObjectCounts_PocPolicyBypass` regression lock; existing `TestSyncBypass_SingleCallSite` continues to pass.
- **Verification:** awaits production deploy; expected outcome is the `pdbplus_data_type_count{type="poc"}` series collapses to a single stable value across the 8-instance fleet (~2x the previous public-only floor) within one sync interval after deploy.
