---
phase: 260426-pms
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/config/config.go
  - internal/config/config_test.go
  - CLAUDE.md
  - internal/sync/worker_test.go
  - internal/sync/worker.go
  - ent/schema/organization.go
  - ent/schema/network.go
  - ent/schema/facility.go
  - ent/schema/internetexchange.go
  - ent/schema/carrier.go
  - ent/schema/campus.go
autonomous: true
requirements:
  - SEED-001
quick: true
---

<objective>
Flip default `PDBPLUS_SYNC_MODE` from `full` to `incremental` per SEED-001, now that
the upstream tombstone-via-`?since=` behaviour is empirically confirmed (live spike
2026-04-26 against www.peeringdb.com returned `status="deleted"` rows on a 30-day
window). Add a regression-guard conformance test that drives a tombstone end-to-end
through the worker, audit two pieces of behaviour the flip exposes (the
`markStaleDeleted*` no-op on incremental cycles is already correctly gated; the
`name=""` PII-scrub case on tombstones must clear the `NotEmpty()` validator on
the 6 folded entities), and label sync metrics with `mode={full,incremental}` so
operator dashboards can distinguish.

Purpose: SEED-001 trigger fired (deletion semantics confirmed). The plumbing is
already in place — cursor parsing/persistence (`internal/peeringdb/client.go`),
per-type cursor advance, cursor-zero fallback to full
(`internal/sync/worker.go:842-866`), `markStaleDeleted*` already gated to
full-only via `fromIncremental` map (`syncDeletePass:1361`,
`syncUpsertPass:933`), and 13 entity upserts all chain `.SetStatus(x.Status)`
with `OnConflictColumns(...).UpdateNewValues()`. This change is mostly a
default flip + regression guard.

Output: an instance-default sync mode that converges on deletions in seconds
(15-minute incremental cycles emit tombstones) instead of hours (1-hour full
re-fetch), without breaking first-sync (cursor-zero fallback) or any of the 5
API surfaces.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/STATE.md
@.planning/seeds/SEED-001-incremental-sync-evaluation.md

<interfaces>
<!-- Key contracts the executor must work against. Pre-extracted to avoid -->
<!-- the executor needing to scavenger-hunt through the sync package. -->

From internal/config/config.go:
```go
type SyncMode string

const (
    SyncModeFull        SyncMode = "full"
    SyncModeIncremental SyncMode = "incremental"
)

// In Load() at line ~244:
syncMode, err := parseSyncMode("PDBPLUS_SYNC_MODE", SyncModeFull)
//                                                  ^^^^^^^^^^^^ flip target
```

From internal/sync/worker.go (already-correct gating, do NOT touch):
```go
// stageOneTypeToScratch:842 — falls back to full when cursor is zero.
if mode == config.SyncModeIncremental && !cursor.IsZero() {
    generated, incErr := scratch.stageType(ctx, w.pdbClient, name, cursor)
    if incErr == nil {
        return generated, true, nil // incremental flag set
    }
    // ...fallback to full...
}

// syncUpsertPass:933 — incremental skips remote-ID collection.
if !fromIncremental[step.name] {
    // ...collect remote IDs for delete pass...
    remoteIDsByType[step.name] = ids
}

// syncDeletePass:1366 — types absent from remoteIDsByType are skipped.
remoteIDs, ok := remoteIDsByType[step.name]
if !ok {
    continue // Incremental sync succeeded for this type — no delete needed.
}
```

From internal/sync/worker.go:723-724 (success metric — needs `mode` label):
```go
statusAttr := metric.WithAttributes(attribute.String("status", "success"))
pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), statusAttr)
pdbotel.SyncOperations.Add(ctx, 1, statusAttr)
```

From internal/sync/worker.go:1406-1408 (failure metric — needs `mode` label):
```go
failedAttr := metric.WithAttributes(attribute.String("status", "failed"))
pdbotel.SyncDuration.Record(ctx, time.Since(start).Seconds(), failedAttr)
pdbotel.SyncOperations.Add(ctx, 1, failedAttr)
```

From the 6 folded entity schemas (ent/schema/{organization,network,facility,internetexchange,carrier,campus}.go):
```go
field.String("name").
    NotEmpty().    // <-- this validator will reject tombstones with name=""
    ...,
```

From internal/sync/worker_test.go:841 (existing fixture pattern — match this style):
```go
type fixtureWithMeta struct {
    server          *httptest.Server
    responses       map[string]any
    failTypes       map[string]bool
    failOnce        map[string]bool
    failIncremental map[string]bool
    callCounts      map[string]*atomic.Int64
    sinceSeen       map[string]*atomic.Bool
    generated       float64
}
```
</interfaces>

<empirical_findings>
Live spike against www.peeringdb.com on 2026-04-26 (recorded in
SEED-001 § Empirical findings):

`GET /api/poc?since=<30d-ago>&limit=200` returned 101 rows: 96 `status="ok"`,
5 `status="deleted"`. Tombstone fields:
- `id` present
- `status="deleted"`
- `updated` bumped to deletion timestamp
- `name=""` (PII scrubbed — GDPR-style soft-delete)
- other PII fields scrubbed similarly

This is the regression we're locking in.
</empirical_findings>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Flip default + audit name validators</name>
  <files>internal/config/config.go, internal/config/config_test.go, ent/schema/organization.go, ent/schema/network.go, ent/schema/facility.go, ent/schema/internetexchange.go, ent/schema/carrier.go, ent/schema/campus.go, CLAUDE.md</files>
  <action>
**Subtask 1A — flip the default in `internal/config/config.go`:**

1. Change line 244 from `parseSyncMode("PDBPLUS_SYNC_MODE", SyncModeFull)` to `parseSyncMode("PDBPLUS_SYNC_MODE", SyncModeIncremental)`.
2. Update the godoc comment on `Config.SyncMode` (line ~88): `Default is "full".` → `Default is "incremental" (SEED-001 active 2026-04-26 — upstream ?since= empirically confirmed to emit status='deleted' tombstones for converged deletion semantics). 'full' remains an explicit operator override for first-sync, recovery, and escape-hatch use.`
3. Add a one-line `slog.Info` in `Load()` adjacent to the existing "sync interval configured" announcement, surfacing the resolved sync mode at startup so operators see what mode actually took effect:
   ```go
   slog.Info("sync mode configured", slog.String("mode", string(cfg.SyncMode)))
   ```
   Place it AFTER the "sync interval configured" log line; both are operator-visible startup announcements.

**Subtask 1B — add a config_test.go regression case** mirroring the existing `parseSyncMode` test pattern (table-driven). The new test asserts:
- Unset `PDBPLUS_SYNC_MODE` → `SyncModeIncremental`.
- Explicit `"full"` → `SyncModeFull`.
- Explicit `"incremental"` → `SyncModeIncremental`.
- Explicit `"bogus"` → error.

Look at how existing tests use `t.Setenv` + table-driven cases in `config_test.go` and match the style. Mark the test as `t.Parallel()` only if the existing config_test.go pattern uses parallel — DO NOT add `t.Parallel()` if the file's tests rely on `t.Setenv` (Go's testing framework forbids `t.Parallel` in tests that use `t.Setenv`).

**Subtask 1C — relax NotEmpty on name fields for the 6 folded entities** that face tombstones with PII-scrubbed `name=""`:

The empirical spike confirmed upstream emits tombstones with `name=""` (PII scrub). The 6 folded entities (`organization`, `network`, `facility`, `internetexchange`, `carrier`, `campus`) currently declare `field.String("name").NotEmpty()` — this validator runs on every upsert and will reject incoming tombstones, breaking the entire sync cycle on the first deletion that arrives via `?since=`.

The fix: remove `NotEmpty()` from the `name` field on all 6 folded entity schemas. This is the minimal change. The non-empty contract was a pdbcompat-leaning assumption from when sync rejected `status='deleted'` rows entirely (pre-Phase 68); it's no longer correct now that tombstones are first-class.

For each of the 6 schema files, locate the `field.String("name")` declaration and delete the `.NotEmpty().` line from the chain. Preserve all other annotations, comments (e.g. `organization.go:73` "PeeringDB permits duplicates" comment), and the index declaration. Do NOT touch the `_fold` sibling files. Do NOT touch any other entity (`poc`, `ixlan`, etc. — those don't have folded names AND poc has its own visibility/PII story already handled by Phase 64).

After editing the schemas, run `go generate ./...` from the project root to regenerate the ent client. `go generate` is idempotent on a clean tree per the CI drift gate; the only diff in `ent/` should be the absence of the `validators.NotEmpty` wrapper on those 6 name fields.

If for any reason the executor decides removing `NotEmpty()` is the wrong call (e.g. the schema validator does NOT actually run on update paths under `OnConflictColumns(...).UpdateNewValues()`), they MUST instead write a Go-level test that proves an upsert of a `name=""` tombstone for an existing row succeeds without removing the validator — i.e. the validator is bypassed by the upsert path. The test in Task 2 (`TestSync_IncrementalDeletionTombstone`) is the proof. If that test passes WITHOUT the NotEmpty removal, revert the schema edits in this subtask. Either way, the conformance test in Task 2 is the source of truth.

**Subtask 1D — update CLAUDE.md env var table** (the row for `PDBPLUS_SYNC_MODE`):
- Change "Default | `full`" → "Default | `incremental`"
- Update the description: "Sync strategy: `full` or `incremental`. Default flipped 2026-04-26 (SEED-001 trigger fired — upstream `?since=` confirmed to emit `status='deleted'` tombstones). `full` remains an explicit operator override for first-sync, recovery, and operator escape-hatch."
- The CLAUDE.md `Sync observability` subsection already references SEED-001; do NOT add new prose there.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go test -race ./internal/config/... &amp;&amp; go build ./... &amp;&amp; go generate ./... &amp;&amp; git diff --quiet ent/ gen/ graph/ internal/web/templates/ &amp;&amp; gofmt -s -l internal/config/config.go ent/schema/*.go CLAUDE.md | (! grep .)</automated>
  </verify>
  <done>
- `internal/config/config.go` default flipped to `SyncModeIncremental`; godoc updated; startup log line added.
- `internal/config/config_test.go` has a 4-case table-driven test covering unset + explicit values + invalid value.
- All 6 folded-entity schemas (`organization`, `network`, `facility`, `internetexchange`, `carrier`, `campus`) no longer call `.NotEmpty()` on the `name` field.
- `go generate ./...` produces zero drift outside the expected ent regeneration delta (re: the 6 name fields).
- CLAUDE.md env var table reflects the new default.
- `go build ./...` and `go test -race ./internal/config/...` pass.
  </done>
</task>

<task type="auto">
  <name>Task 2: Conformance test — incremental tombstone end-to-end</name>
  <files>internal/sync/worker_test.go</files>
  <action>
Add a single regression-guard test, `TestSync_IncrementalDeletionTombstone`, to `internal/sync/worker_test.go`. Match the existing `fixtureWithMeta` + `newTestWorkerWithMode(t, ..., config.SyncModeIncremental)` patterns at lines 829-933 (do NOT introduce a third fixture type — extend or reuse `fixtureWithMeta`).

**Test shape (deterministic, table-driven not appropriate here — single end-to-end scenario):**

```
1. Stand up fixtureWithMeta with generated = epoch_T1.
2. f.responses["org"] = []any{ makeOrg(1, "Org1", "ok"), makeOrg(2, "Org2", "ok") }.
3. w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental).
4. First sync: w.Sync(ctx, config.SyncModeIncremental).
   - Cursor is zero → falls back to full sync per stageOneTypeToScratch:842-866.
   - Assert both orgs exist with status="ok".
   - Assert cursor for "org" is now non-zero (UpsertCursor stamped meta.generated).
5. Reset f.sinceSeen["org"].
6. Bump f.generated to epoch_T2 (T1 + 1 hour).
7. f.responses["org"] = []any{
     makeOrg(1, "Org1", "ok"),                         // unchanged
     makeOrg(2, "", "deleted"),                        // tombstone — PII scrubbed
   }
   (Use the existing makeOrg helper; pass empty name and "deleted" status.)
8. Second sync: w.Sync(ctx, config.SyncModeIncremental).
   - Assert ?since= was sent (f.sinceSeen["org"].Load() == true) — confirms incremental path.
   - Assert org 1 still exists with status="ok" and name="Org1".
   - Assert org 2 exists with status="deleted" and name="" — the row was flipped, not hard-deleted.
   - Assert org count is still 2 (soft-delete preserves row).
9. Anonymous list query: simulate the pdbcompat path that excludes tombstones.
   At the ent layer that's `Organization.Query().Where(organization.Status("ok"))` — this is what applyStatusMatrix devolves to when ?since is unset. Assert this returns ONLY org 1 (count == 1, ID == 1).
   (We do NOT need to drive a real HTTP request through the pdbcompat handler — that surface is covered by parity tests in internal/pdbcompat/parity/. Here we're proving the underlying sync state is correct.)
```

**Two important notes:**

1. **Empty-name tombstone is the load-bearing assertion.** Step 7's `makeOrg(2, "", "deleted")` creates a tombstone with `name=""`. If Task 1 Subtask 1C correctly removes the `NotEmpty()` validator (or proves it's bypassed by upsert), this test passes. If the validator still rejects the row, this test fails with a clear error during the second sync. THIS is the test that proves the empty-name PII path works end-to-end.

2. **No `t.Parallel()`** if `TestMain` (line 59) or shared metric state interferes. Spot-check the existing `TestIncrementalSync` pattern (line 937) — it uses `t.Parallel()`, so this test should too.

Place the new test immediately after `TestIncrementalFallback` (~line 1078) so all incremental tests cluster together. Add a one-line godoc above the test referencing the SEED-001 spike date and the "name='' is the GDPR PII-scrub path" rationale.

Do NOT add a separate "second folded entity" sub-test (e.g. networks). One end-to-end org case is sufficient — if `name=""` works for organization, the same path works for the other 5 by construction (identical schema pattern, identical upsert builder shape, identical `OnConflictColumns(...).UpdateNewValues()` chain). The 6 schemas were modified uniformly in Task 1.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go test -race -run TestSync_IncrementalDeletionTombstone ./internal/sync/... -count=1 -v</automated>
  </verify>
  <done>
- `TestSync_IncrementalDeletionTombstone` lives in `internal/sync/worker_test.go`, clustered with the other incremental tests.
- Test drives 2 sync cycles via `fixtureWithMeta` and asserts: (a) `?since=` was sent on cycle 2, (b) tombstone with `name=""` flipped org 2's status to `deleted` without hard-deleting, (c) anonymous filter (`status="ok"`) returns only the live row.
- `go test -race -run TestSync_IncrementalDeletionTombstone ./internal/sync/... -count=1` passes.
- Full `go test -race ./internal/sync/...` passes (no regressions in adjacent tests, especially `TestSyncSoftDeletesStale`, `TestIncrementalSync`, `TestIncrementalFirstSyncFull`, `TestIncrementalFallback`).
  </done>
</task>

<task type="auto">
  <name>Task 3: Sync metrics — add mode label</name>
  <files>internal/sync/worker.go</files>
  <action>
Add `mode={full,incremental}` as a low-cardinality label on the `pdbplus.sync.duration` and `pdbplus.sync.operations` metrics so dashboards/alerts can distinguish full from incremental cycle behaviour after the default flip.

Two call sites to extend (both already pass `mode` as a context value via the OTel span name `sync-+string(mode)`, so the label is purely additive):

**Site 1 — `recordSuccess`** at `internal/sync/worker.go:721-724`:
```go
// BEFORE
elapsed := time.Since(start)
statusAttr := metric.WithAttributes(attribute.String("status", "success"))
pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), statusAttr)
pdbotel.SyncOperations.Add(ctx, 1, statusAttr)

// AFTER
elapsed := time.Since(start)
attrs := metric.WithAttributes(
    attribute.String("status", "success"),
    attribute.String("mode", string(mode)),
)
pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(), attrs)
pdbotel.SyncOperations.Add(ctx, 1, attrs)
```
The function already accepts `mode config.SyncMode` (line 706) — no signature change needed.

**Site 2 — `recordFailure`** at `internal/sync/worker.go:1406-1408`. The current `recordFailure` signature does NOT carry the mode (see line 1400: `func (w *Worker) recordFailure(ctx context.Context, statusID int64, start time.Time, syncErr error)`). Two options:

- (a) Extend the signature to add `mode config.SyncMode` and update all 3 call sites in `Sync` (the 5 `w.recordFailure` and `w.rollbackAndRecord` calls).
- (b) Stash the mode on the worker as a per-call field, OR derive it from the active span name.

Option (a) is the cleaner choice (explicit dataflow > implicit context lookup, GO-CFG-2). Implement it:

1. Change signature: `func (w *Worker) recordFailure(ctx context.Context, mode config.SyncMode, statusID int64, start time.Time, syncErr error)`.
2. Mirror the same change to `rollbackAndRecord` (line 688): adds `mode` as the 2nd parameter.
3. Update the metric attrs in `recordFailure` to mirror Site 1:
   ```go
   attrs := metric.WithAttributes(
       attribute.String("status", "failed"),
       attribute.String("mode", string(mode)),
   )
   pdbotel.SyncDuration.Record(ctx, time.Since(start).Seconds(), attrs)
   pdbotel.SyncOperations.Add(ctx, 1, attrs)
   ```
4. Update the 4 call sites in `Worker.Sync` (lines 480, 489, 496, 504, 516, 520, 525) — every `recordFailure(ctx, statusID, start, ...)` and `rollbackAndRecord(ctx, tx, statusID, start, ...)` call now threads `mode` through.

**Cardinality check:** `mode` adds a 2× factor to existing `status` label (which is already 2 values: success/failed). Total label combinations on `pdbplus_sync_operations_total`: 2 × 2 = 4. Well under any cardinality concern.

Do NOT add `mode` to the per-type metrics (`SyncTypeObjects`, `SyncTypeDeleted`, `SyncTypeFetchErrors`, `SyncTypeUpsertErrors`, `SyncTypeFallback`, `SyncTypeOrphans`) — those are already labelled by `type` (13 values) and adding a `mode` factor would bloat them to 26 combos for arguably less operator value. SEED-001 explicitly says "label-extend existing ones; do NOT add new metrics."

Do NOT add `mode` to the per-cycle span attributes (`pdbplus.sync.peak_heap_bytes`, `pdbplus.sync.peak_rss_bytes`, etc.) — span-level dimensionality is unbounded; the parent span name is already `sync-full` or `sync-incremental` (line 440), which is the operator-visible signal in Tempo/Jaeger.

Update the existing `metrics_test.go` `TestSyncOperations_AddDoesNotPanic` if it asserts a specific number of attrs — confirm by grep before editing.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go test -race ./internal/sync/... ./internal/otel/... -count=1 &amp;&amp; go vet ./... &amp;&amp; gofmt -s -l internal/sync/worker.go | (! grep .)</automated>
  </verify>
  <done>
- `recordSuccess` and `recordFailure` (and `rollbackAndRecord`) emit `pdbplus.sync.duration` + `pdbplus.sync.operations` with both `status` AND `mode` labels.
- `recordFailure` and `rollbackAndRecord` signatures extended with `mode config.SyncMode` (immediately after `ctx`).
- All call sites in `Worker.Sync` updated.
- No new metrics introduced; per-type metrics untouched.
- `go test -race ./internal/sync/... ./internal/otel/...` passes; `go vet ./...` clean; `gofmt -s` clean.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| upstream `peeringdb.com` → sync worker | untrusted JSON input crosses here, including `status="deleted"` tombstones with PII-scrubbed empty fields |
| sync worker → ent.Tx | row writes; validators run client-side before SQL |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-pms-01 | Tampering | sync upsert | mitigate | `name=""` tombstone reaches upsert builder; `NotEmpty()` validator removed from 6 folded entities so the upsert path accepts the PII-scrub case. Conformance test in Task 2 locks the behaviour. |
| T-pms-02 | DoS | first-sync / cursor recovery | accept | Existing `stageOneTypeToScratch:842` cursor-zero fallback already handles cursor-loss and first-sync. No new attack surface from the default flip. |
| T-pms-03 | Information Disclosure | metric labels | accept | New `mode` label is operator-visible only (2 values); no PII or secret on the wire. |
| T-pms-04 | Repudiation | sync mode change | mitigate | Startup `slog.Info("sync mode configured", mode=...)` logs the resolved mode; failure metrics carry `mode` so post-incident review can distinguish full vs incremental cycles. |
</threat_model>

<verification>
End-to-end sanity:
1. `go build ./...` — full repo builds.
2. `go test -race ./...` — full suite passes.
3. `go generate ./...` — produces zero drift on a clean tree per CI drift gate, modulo the expected ent regeneration delta from removing `NotEmpty()` on 6 schemas.
4. `golangci-lint run` — passes with project config.
5. `grep -n 'PDBPLUS_SYNC_MODE.*incremental' internal/config/config.go CLAUDE.md` — both default + docs reference the new default.
6. `grep -n 'attribute.String("mode"' internal/sync/worker.go` — 2 occurrences (recordSuccess + recordFailure).
</verification>

<success_criteria>
1. Default `PDBPLUS_SYNC_MODE` is `incremental`; `full` remains a valid explicit override.
2. CLAUDE.md env var table reflects the new default.
3. The `name=""` PII-scrubbed tombstone path is regression-locked: an end-to-end test drives a tombstone for an existing org through an incremental sync cycle and asserts the row is soft-deleted.
4. `pdbplus.sync.operations{status,mode}` and `pdbplus.sync.duration{status,mode}` carry the new `mode` label so dashboards distinguish full vs incremental.
5. No new metrics introduced; per-type metrics unchanged.
6. No regressions in existing sync tests, especially `TestSyncSoftDeletesStale`, `TestIncrementalSync`, `TestIncrementalFirstSyncFull`, `TestIncrementalFallback`.
7. `go test -race ./...`, `go vet ./...`, `golangci-lint run`, and `go generate ./...` (with no drift) all pass.
</success_criteria>

<output>
After completion, create `.planning/quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/260426-pms-SUMMARY.md`.
</output>
