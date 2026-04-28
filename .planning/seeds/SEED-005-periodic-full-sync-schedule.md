# SEED-005: Periodic Full-Sync Schedule for Same-Second-Drift Convergence

**Status:** Dormant — flagged by 260428-eda CHANGE 3.
**Owner:** TBD
**Trigger:** Operator observes a row staying out-of-date past a single sync cycle, OR a scheduled full sync stops being available as an operator escape-hatch.

---

## Background

Quick task **260428-eda CHANGE 3** introduced per-row skip-on-unchanged
in the sync upsert path. Each of the 13 `ON CONFLICT DO UPDATE` sites
now carries a `WHERE` predicate equivalent to:

```
excluded.updated > <table>.updated
OR <table>.updated IS NULL
OR <table>.updated <= '1900-01-01'
```

The strict `>` comparison is deliberate — using `>=` would defeat the
optimisation entirely, since PeeringDB's `?since=N` is inclusive and
every refetch produces `excluded.updated >= existing.updated`. The
trade-off is a bounded **same-second-drift risk**: a row edited at
upstream within the same second as the prior cursor advance will skip
on the next incremental cycle.

In practice this is reconciled by:

- **The next upstream change.** As soon as the row is touched again
  upstream, `updated` bumps past the cursor and the row reconciles
  naturally.
- **Full-mode runs.** `PDBPLUS_SYNC_MODE=full` ignores the skip-on-
  unchanged predicate (every column is rewritten unconditionally via
  `sql.ResolveWithNewValues()` — the predicate gates the write but
  the cycle still ATTEMPTS the write for every row in the response).
  Wait — that's not right; `UpdateWhere` gates on the SQL side, so
  full mode ALSO skips unchanged rows. The actual reconciliation
  guarantee in full mode is that we re-fetch ALL upstream rows (no
  `?since=` cursor), so any row whose `updated` we missed in
  incremental will eventually be admitted by the predicate when its
  `updated` bumps. In the rare same-second-drift case a row could
  stay stale until the next change.

## Proposal

Schedule a periodic full sync (e.g. weekly) regardless of incremental
cadence. This guarantees convergence even for rows trapped in the
same-second-drift gap.

**Knobs to design:**

- `PDBPLUS_FULL_SYNC_INTERVAL` (default: weekly? bi-weekly?)
- Anchor point — Sunday 06:00 UTC? Just-after-midnight in the primary
  region?
- Interaction with `PDBPLUS_SYNC_INTERVAL` — full overrides incremental
  on the scheduled day, then incremental resumes.
- Primary-only vs follow-the-leader (LiteFS replicas can't drive a
  sync; this is a primary-side scheduler concern).

**Alternatives considered:**

1. **Drop the optimisation entirely.** Cost: 270k unnecessary upserts
   per cycle, ~60s of LiteFS replication churn, the very tail this
   plan was written to eliminate. Hard NO.
2. **Use `>=` instead of `>`.** Cost: same 270k upserts on every
   cycle (since `?since=N` is inclusive). Hard NO.
3. **Track `updated` at sub-second precision.** PeeringDB's API
   returns RFC3339 timestamps with second resolution. Upstream change
   needed; not available.
4. **Scheduled full sync (this proposal).** Cheap, opt-in, mirrors
   how operators already think about reconciliation cadence.

## Triggers

- Operator reports a row stuck stale (cross-instance compare via
  `fly ssh console` + `sqlite3` shows different `updated` between
  primary and replicas, with no upstream change since).
- Audit reveals a row whose `updated` matches the cursor advance second
  precisely AND whose content disagrees with upstream content fetched
  manually.
- Future workload sees PeeringDB API push to sub-second `updated`
  precision (would invalidate the same-second-drift assumption
  altogether — lift the skip-on-unchanged predicate to operate on
  the sub-second value).

## Out of scope

- Active-active primary support (separate concern; SEED-003).
- Row-level "last seen" tracking (would inflate every table by 8 bytes
  for marginal diagnostic value).
- Reconciliation alerting (Grafana panel showing
  `time.Since(max(updated))` per type would surface drift trends but
  is a separate observability seed).

## References

- `.planning/quick/260428-eda-sync-optimization-bundle-spans-cursor-in/260428-eda-PLAN.md`
  Task 6 — the predicate definition.
- `internal/sync/upsert.go` `skipUnchangedPredicate` — the helper.
- `CLAUDE.md` § Environment Variables — current sync interval defaults.
