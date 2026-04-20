# Phase 68: Status × since matrix + limit=0 semantics — Research

**Researched:** 2026-04-19
**Domain:** `pdbcompat` request filtering semantics + `sync` soft-delete flip + `config` grace-period deprecation
**Confidence:** HIGH (all core claims `[VERIFIED: codebase]` or `[CITED: upstream rest.py]`)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (7)

- **D-01 — `PDBPLUS_INCLUDE_DELETED` removed**: Env var deleted entirely. Sync always persists deleted rows as tombstones (see D-02). Any operator deploys still setting this var see a startup WARN-and-ignore for one milestone (v1.16 → v1.17 grace period), then hard-error if still set by v1.17. Migration note goes in `docs/CONFIGURATION.md` and CHANGELOG.
- **D-02 — Sync flipped to soft-delete**: The 13 `deleteStale*` functions become `markStaleDeleted*` — they run `UPDATE ... SET status='deleted', updated=? WHERE id NOT IN (?)` instead of `DELETE FROM`. Rows stay in DB; `status=deleted + since` queries now return real data. Hard-delete path is removed entirely. Tombstone GC policy deferred to SEED-004.
- **D-03 — Backfill on first post-Phase-68 sync**: First full sync after deploy runs normal soft-delete. Rows hard-deleted BEFORE Phase 68 ships are gone forever — `?status=deleted` returns only rows marked deleted from that sync onward. Documented as a known one-time gap. No retroactive reconstruction from PeeringDB.
- **D-04 — `limit=0` safety ceiling**: No safety ceiling in Phase 68 — `limit=0` returns all rows unconditionally per upstream. Between Phase 68 shipping and Phase 71 landing the memory budget, do not deploy to prod. Execute phases 68 → 69 → 70 → 71 as a coordinated ship; Phase 71's pre-flight row-count × size heuristic is the OOM safeguard.
- **D-05 — Campus `pending` inclusion**: Campus single-object lookups and `since>0` list queries include `status=pending`. Other types do not admit `pending` on list (D-07). Matches upstream `rest.py:712`.
- **D-06 — pk-lookup `status` filter**: All 13 entity types admit `(ok, pending)` on single-object (pk) GET, not just campus. Matches upstream `rest.py:727` `get_queryset` behaviour for non-list requests.
- **D-07 — `?status=deleted` without `since` = empty**: Upstream applies the final `filter(status='ok')` unconditionally on list requests without `since` per `rest.py:725`. Our pdbcompat mirrors this: any list request without `since` filters to `status=ok` regardless of `?status=<anything>` param. Only pk lookups and `since>0` requests allow alternate statuses.

### Claude's Discretion

- Plan split shape (single-plan vs multi-plan)
- Test fixture layout (new file vs extend existing seed helpers)
- WARN message wording + slog attribute keys
- Whether `markStaleDeleted*` stays in `internal/sync/delete.go` (current home) or moves to a new `softdelete.go` — both are defensible; stay-in-place is cheaper
- `updated` timestamp choice on soft-delete UPDATE: sync-cycle start time (captured once per Sync call) vs per-row `time.Now()` — research recommends the sync-cycle start time for determinism

### Deferred Ideas (OUT OF SCOPE)

- Cross-entity traversal filters (Phase 70)
- Memory budget enforcement (Phase 71) — includes the `limit=0` ceiling
- grpcserver / entrest status semantics — Phase 68 is pdbcompat-only
- Tombstone GC scheduler — SEED-004, planted alongside this phase
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **STATUS-01** | pdbcompat list (no `?since`) returns only `status=ok` | `rest.py:725` verbatim — single-line filter. Applied in every 13 list closures in `registry_funcs.go`. |
| **STATUS-02** | pdbcompat pk lookup admits `status IN (ok, pending)` | `rest.py:727` — the `else` branch of `if not self.kwargs`. `handler.serveDetail` → `tc.Get` → `*WithDepth` is the 13-func entry point. |
| **STATUS-03** | pdbcompat list with `?since>0` admits `(ok, deleted)` + `pending` for campus | `rest.py:700-712`. Only consumable once D-02 soft-delete flip lands — pre-Phase-68 there are NO rows with `status='deleted'` in DB. |
| **STATUS-04** | `?status=deleted` without `since` returns empty | Falls out of STATUS-01: the final `status=ok` filter is applied *unconditionally*, so a caller-supplied `?status=deleted` is silently overridden. D-07 calls this out. Per-entity `fields` maps currently include `"status": FieldString` — this research recommends *removing* `status` from the `Fields` maps so the filter.go path never builds a `status=` predicate in the first place (simpler than deleting a predicate after the fact). |
| **STATUS-05** | `PDBPLUS_INCLUDE_DELETED` becomes sync-only / removed | Env var removed entirely per D-01. Grace-period WARN when operator still sets it; hard-error in v1.17. |
| **LIMIT-01** | `?limit=0` = unlimited | `rest.py:734-737`: `if limit > 0: qset[skip:skip+limit] else: qset[skip:]`. Our current `ParsePaginationParams` bugs out `limit=0` and coerces to `DefaultLimit=250`. |
| **LIMIT-02** | `depth>0` still caps at 250 per upstream `API_DEPTH_ROW_LIMIT` | `rest.py:463, 744-748`. Not currently enforced in pdbcompat — new cap logic needs to land. Today `depth=2` is accepted on `handler.serveDetail` only (pk path), not list. Need to confirm this with plan-checker. |
</phase_requirements>

## Executive Summary

1. **Upstream is small and precise.** The whole status × since × limit matrix is ~30 lines in `src/peeringdb_server/rest.py:484-755`. It is already read-through on line-refs above; there is no ambiguity to relitigate.
2. **pdbcompat list path is 13 near-identical closures in `registry_funcs.go` `wire*Funcs()` — not a single switchboard.** Every list closure independently builds `preds`, appends since, runs `.Order(...)`, `.Limit(opts.Limit).Offset(opts.Skip)`. A new shared helper `applyStatusMatrix(opts, isListWithoutSince bool, isCampus bool) func(*sql.Selector)` should be called from each of the 13 closures. Resist any urge to centralise beyond that — the existing `castPredicates[T]` / typed `predicate.Foo` downcast pattern is cheaper than a full list-func rewrite, and Phase 67 just regenerated 39 goldens against that exact shape.
3. **pk path is separate — `handler.serveDetail` → `tc.Get` → `get{Type}WithDepth` in `depth.go`.** 13 funcs, each calls `client.Foo.Get(ctx, id)` or `client.Foo.Query().Where(foo.ID(id)).Only(ctx)`. NO status filter today. D-06 requires `(ok, pending)` — this is a one-line `.Where(foo.StatusIn("ok", "pending"))` addition per function, mirrored at the `Only(ctx)` call site.
4. **`limit=0` is broken today.** `response.go:55-60` `ParsePaginationParams` gates on `parsed > 0` — `?limit=0` silently falls back to `DefaultLimit=250`. The fix is isolated: replace the gate with `parsed >= 0`, and plumb `limit=0` through to ent by converting "0" to a sentinel (either drop `.Limit(...)` entirely or pass a huge-but-safe int). ent's `Limit(0)` is a no-op that returns zero rows — *confirm empirically*, this is the single biggest behavioural trap in the phase. Ent source suggests `Limit(-1)` or skipping `.Limit()` for "no limit"; skipping is the cleanest path.
5. **Sync soft-delete flip is low-risk, high-leverage.** All 13 `deleteStale*` functions live in `internal/sync/delete.go` (NOT `worker.go` as the plan hint says — CORRECTION). They share a `deleteStaleChunked` helper. Keeping that helper and swapping the closure body from `tx.Foo.Delete().Where(foo.IDNotIn(chunk...)).Exec(ctx)` to `tx.Foo.Update().Where(foo.IDNotIn(chunk...)).SetStatus("deleted").SetUpdated(syncCycleStart).Save(ctx)` is 13 ~2-line closure rewrites. Signature `(int, error)` stays — the `int` becomes "rows marked deleted this pass".
6. **`updated` timestamp on soft-deleted rows is load-bearing for STATUS-03.** Upstream's `since` query relies on `updated >= since` to catch newly-deleted rows. If you soft-delete with the *original* row's `updated` timestamp, `?since=NOW-1h` will miss a row deleted 5 minutes ago whose `updated` is 24h old. Every `markStaleDeleted*` MUST set `updated` to the sync cycle start time. Capture the timestamp once at the top of `Worker.Sync` and pass it into the delete pass.
7. **`includeDeleted` on the UPSERT side needs attention too.** Today `syncIncremental[E]` at `worker.go:969` filters out `status=deleted` upstream rows before upserting if `!includeDeleted`. Post-Phase-68, deleted upstream rows MUST be upserted (they represent tombstones coming in from PeeringDB's own history). The D-01 removal of the flag makes this unconditional — just delete the `if !includeDeleted` branch and the `filterByStatus` helper. Confirm the helper isn't used elsewhere via grep before removing.
8. **Config grace-period is a simple 3-line pattern in `Load()`.** Check `os.Getenv("PDBPLUS_INCLUDE_DELETED") != ""` → `slog.WarnContext(ctx, "PDBPLUS_INCLUDE_DELETED is deprecated and ignored; will be a startup error in v1.17")` → remove the field from `Config`. Drop the field, `parseBool` call, `cfg.IncludeDeleted = …` assignment, and the `validate` test for it (there is no such validate call — it's already zero-risk removal). Same pattern will be re-used for v1.17 hard-error: flip `slog.Warn` to `return nil, fmt.Errorf(...)`.
9. **Status column is already indexed on all 13 entities.** `ent/schema/*.go` all have `index.Fields("status")`. The post-Phase-68 query `WHERE status IN ('ok', 'pending')` is index-assisted. For `since>0` queries, ent should pick the `(updated, created, id)` index just added in Phase 67 Plan 01 — the status filter runs as a residual. No new indexes needed; verify by eyeballing `EXPLAIN QUERY PLAN` during testing, not in code.
10. **Golden files need regeneration per affected surface.** Phase 67 regenerated 39 goldens across 13 entities × 3 paths (list / detail / depth). Phase 68 can keep most list/detail goldens unchanged if the seed data is all `status=ok` (which it is — `testutil/seed.Full` does not seed deleted rows). New goldens will be needed for NEW tests (e.g. `list_with_since.json`, `detail_with_pending.json`) rather than regenerating the 39.
11. **Pre-existing `IncludeDeleted` test dependencies ripple.** `integration_test.go:365 TestSyncIncludeDeleted`, `worker_test.go` in ~6 places, and `nokey_sync_test.go`, `replay_snapshot_test.go` all reference the field. Plan must rewrite `TestSyncIncludeDeleted` → `TestSync_SoftDeleteMarksRows` (semantic flip); other call sites just need the field reference removed since `IncludeDeleted: true` becomes the only behaviour.

**Primary recommendation:** Split into 4 plans — (01) config grace-period + `IncludeDeleted` field removal, (02) sync soft-delete flip, (03) pdbcompat status matrix + `limit=0` + depth cap, (04) tests + docs + CHANGELOG — in that sequential order. See § Recommended Plan Split.

## Upstream Ground Truth (rest.py:484-755)

Source: `peeringdb/peeringdb@main` — `src/peeringdb_server/rest.py` (2062 lines, fetched 2026-04-19). [VERIFIED: `gh api repos/peeringdb/peeringdb/contents/src/peeringdb_server/rest.py`]

### The canonical matrix (lines 694-727)

```python
if not self.kwargs:                                   # 695  -- list request
    if since > 0:                                     # 696
        allowed_status = ["ok", "deleted"]            # 700
        if self.model.HandleRef.tag == "campus":      # 702
            allowed_status.append("pending")          # 712
        qset = (
            qset.since(timestamp=..., deleted=True)    # 714-720
                .order_by("updated")                   # 721
                .filter(status__in=allowed_status)     # 722
        )
    else:                                             # 724
        qset = qset.filter(status="ok")               # 725  <-- D-07 single-line origin
else:                                                 # 726  -- pk (single-object) request
    qset = qset.filter(status__in=["ok", "pending"])  # 727  <-- D-06 single-line origin
```

**Key observations:**

- The list-without-`since` branch (`rest.py:725`) is unconditional — it does NOT consult `?status=<anything>` from the query string. That's why our pdbcompat will silently drop a `?status=deleted` filter on list requests without `since`. [CITED: rest.py:725]
- The list-with-`since` branch builds `allowed_status` as a literal list, not a user input. Caller cannot override. [CITED: rest.py:700-712]
- `self.kwargs` is truthy iff the DRF URL route matched `/<pk>/` — our pdbcompat's `handler.serveDetail` is the equivalent code path. [CITED: rest.py:695, 726]
- Campus is the ONLY special case. Every other entity falls into the plain `(ok, deleted)` on `since>0` and `(ok, pending)` on pk. [CITED: rest.py:702-712]

### `limit=0` (lines 494-497, 734-737)

```python
try:
    limit = int(self.request.query_params.get("limit", 0))  # 495  default 0
except ValueError:
    raise RestValidationError({"detail": "'limit' needs to be a number"})

# ...

if limit > 0:                                  # 734
    qset = qset[skip : skip + limit]           # 735
else:                                          # 736
    qset = qset[skip:]                         # 737  -- unlimited (or skip:end)
```

`limit=0` is the *default*, not a special case. Unlimited is the default behaviour. Our pdbcompat's `DefaultLimit=250` is a local product decision that does not match upstream — but LIMIT-01 only requires that `?limit=0` explicitly supplied returns unlimited. The default-when-unset remains `250` in pdbcompat (see `response.go:15`). [CITED: rest.py:495, 734-737]

### depth cap (lines 463, 744-748)

```python
enforced_limit = getattr(settings, "API_DEPTH_ROW_LIMIT", 250)  # 463
# ...
if not is_specific_object_request:                     # 739
    row_count = qset.count()                           # 743
    if enforced_limit and depth > 0 and row_count > enforced_limit:  # 744
        qset = qset[:enforced_limit]                   # 745
        self.request.meta_response["truncated"] = ...  # 746
```

Only applies on **list** requests (not pk) with **depth>0**. Truncates at 250 and adds `meta.truncated` marker. [CITED: rest.py:744-748]

Our pdbcompat `handler.serveList` does not support `?depth=` today (per `handler.go:202-208` depth is parsed only in `serveDetail`). LIMIT-02 says list-with-depth must cap at 250. This implies:

- **Either** pdbcompat `serveList` should also grow a `depth` path (matches upstream), **or**
- LIMIT-02 is a forward-compat commitment that takes effect only once list+depth lands.

Research recommendation: the plan should explicitly *not* add list+depth support in Phase 68 (it's out of scope, belongs in Phase 71's memory-safe paths). Instead, add a one-line guardrail in `handler.serveList`: if `?depth` is present as a query param, reject with 400 or silently ignore. This keeps the phase pdbcompat-status-matrix-only and defers depth+list to 71. LIMIT-02 is then *covered by absence* — no depth list path exists, so no uncapped depth list request is possible. Discuss with plan-checker. **[ASSUMED]** — user may prefer to add depth list support with a 250 cap inside Phase 68.

## pdbcompat Code Map

### List path — the 13× closures

File: `internal/pdbcompat/registry_funcs.go` (406 lines total, ~26 lines per entity).

Template (from `wireOrgFuncs` at lines 56-81, representative of all 13):

```go
setFuncs(peeringdb.TypeOrg,
    func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
        preds := castPredicates[predicate.Organization](opts.Filters)     // filter.go predicates
        if s := applySince(opts); s != nil {                              // registry_funcs.go:49
            preds = append(preds, predicate.Organization(s))
        }
        q := client.Organization.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))  // <- Phase 67 compound order
        total, err := q.Count(ctx)
        if err != nil { return nil, 0, fmt.Errorf("count organizations: %w", err) }
        orgs, err := q.Limit(opts.Limit).Offset(opts.Skip).All(ctx)
        if err != nil { return nil, 0, fmt.Errorf("list organizations: %w", err) }
        // ... serialize and convert to []any
    },
    getOrgWithDepth,
)
```

**Where Phase 68 changes go:**

- A new helper `applyStatusMatrix(isCampus bool, sinceSet bool) func(*sql.Selector)` returning `sql.FieldIn("status", "ok", "deleted"[, "pending"])` on `since>0`, or `sql.FieldEQ("status", "ok")` on no-since. Lives in `filter.go` alongside `applySince`.
- Each of the 13 closures: one new line `preds = append(preds, predicate.Organization(applyStatusMatrix(false, opts.Since != nil)))` after the `applySince` append. For campus, `true`.
- `registry.go:58-66` `reservedParams`: add `"status": true` — OR alternatively remove `"status": FieldString` from all 13 `Fields` maps in `registry.go:71-384`. **Recommend the latter**: status is a compile-time-known column name, and leaving it in `Fields` but reserved would violate the "Fields holds filterable columns" contract. Filter.go line 43-48 already silently ignores unknown fields, so removing status from `Fields` makes `?status=deleted` silently noop — exactly D-07 behaviour.

### pk path — `depth.go`

File: `internal/pdbcompat/depth.go` (260 lines). 13 `get{Type}WithDepth(ctx, client, id, depth)` functions.

Template (from `getOrgWithDepth`, lines 55-83):

```go
func getOrgWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
    if depth >= 2 {
        o, err := client.Organization.Query().
            Where(organization.ID(id)).
            WithNetworks().WithFacilities().WithInternetExchanges().WithCarriers().WithCampuses().
            Only(ctx)   // <- D-06 insertion point A
        // ...
    }

    o, err := client.Organization.Get(ctx, id)   // <- D-06 insertion point B
    // ...
}
```

**Where D-06 changes go:**

- At each `.Where(foo.ID(id))`, add `.Where(foo.StatusIn("ok", "pending"))` (ent generates these static methods per entity — confirmed by `ent/schema/*.go` having `field.String("status")`).
- At each `client.Foo.Get(ctx, id)` call, replace with `client.Foo.Query().Where(foo.ID(id), foo.StatusIn("ok", "pending")).Only(ctx)` — slightly more verbose but needed because `Get` does not accept predicates. Use `ent.IsNotFound(err)` detection continues to work transparently.
- 13 insertion points × 2 code paths each = 26 edits. All mechanical.

### Filter layer — `filter.go`

File: `internal/pdbcompat/filter.go` (205 lines). Nothing to add to `buildPredicate` — D-07 is better served by *removing* status from `Fields` maps (see above). The ParseFilters path will silently ignore `?status=` then.

### Pagination — `response.go`

File: `internal/pdbcompat/response.go` (85 lines).

Current `ParsePaginationParams` (lines 52-70):

```go
func ParsePaginationParams(params url.Values) (limit, skip int) {
    limit = DefaultLimit                                    // 54
    if v := params.Get("limit"); v != "" {                  // 55
        if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {  // 56  <-- BUG: >0 drops limit=0
            limit = parsed
        }
    }
    if limit > MaxLimit {                                   // 60  MaxLimit=1000
        limit = MaxLimit
    }
    // ...
}
```

**LIMIT-01 fix:** change the gate to `parsed >= 0`. Introduce a sentinel: `limit=0` means "no limit". The `MaxLimit=1000` ceiling becomes `limit > 0 && limit > MaxLimit → MaxLimit` (don't cap `limit=0` to `MaxLimit`, or we regress).

Downstream, in each registry_funcs.go closure, the `.Limit(opts.Limit).Offset(opts.Skip)` line needs conditionalisation:

```go
q2 := q.Clone()  // fresh query builder post-count
if opts.Limit > 0 {
    q2 = q2.Limit(opts.Limit)
}
q2 = q2.Offset(opts.Skip)
orgs, err := q2.All(ctx)
```

**Empirical concern [ASSUMED]:** ent's `Limit(0)` behaviour needs a quick check. In ent, `.Limit(0)` generates `LIMIT 0` which returns zero rows (standard SQL). The fix above side-steps this by omitting `.Limit(...)` entirely when `opts.Limit == 0`. Verify with a quick unit test in the plan.

## Sync Worker Code Map

### The 13 `deleteStale*` functions live in `internal/sync/delete.go`

**CORRECTION to CONTEXT.md plan hint:** hint says `internal/sync/worker.go`; actual location is `internal/sync/delete.go` (134 lines). [VERIFIED: grep]

Signature pattern (all 13 identical):

```go
func deleteStaleOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
    return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
        return tx.Organization.Delete().Where(organization.IDNotIn(chunk...)).Exec(ctx)
    }, "organizations")
}
```

Called from `worker.go:215-233 syncSteps()` which returns a slice of `syncStep{name, deleteFn}` structs.

### Flip pattern for D-02

Replace the closure body:

```go
// was:
return tx.Organization.Delete().Where(organization.IDNotIn(chunk...)).Exec(ctx)

// becomes:
return tx.Organization.Update().
    Where(organization.IDNotIn(chunk...)).
    SetStatus("deleted").
    SetUpdated(cycleStart).       // <-- see below; load-bearing
    Save(ctx)                      // returns (int, error); Save returns (int, error) for UpdateMany
```

**Wiring:** `cycleStart time.Time` needs to reach each function. Options:

- (A) Pass it as a new parameter: `func deleteStaleOrganizations(ctx, tx, remoteIDs, cycleStart)` — ripples to the `syncStep.deleteFn` type signature in `worker.go:212`. Clean, but 14 sites change.
- (B) Stash it on `*ent.Tx` via context — brittle. Avoid.
- (C) Use `time.Now()` inside each function — loses determinism across the 13 types within one sync cycle. Acceptable if the cycle is sub-second; not acceptable philosophically.
- (D) Capture it at the top of `syncUpsertPass` / the delete pass caller, and refactor the 13 functions to methods on a small struct holding `tx` + `cycleStart`. Cleanest.

**Recommendation:** Option A (add parameter). It's the most direct, ripples to exactly one signature (the `syncStep.deleteFn` type), and matches Go idiom (GO-CTX-1: context first, then explicit params).

### Rename vs in-place rewrite

Plan hint says `deleteStale*` → `markStaleDeleted*`. Agreeing — the semantic has inverted and the name must follow. Grep for callers:

```
internal/sync/worker.go:219-231  13 call sites in syncSteps() slice
```

Only one call site per function, all in the same table. Rename is 14 edits (13 defs + 13 uses in one slice literal).

### `includeDeleted` must be removed from UPSERT path too

File: `internal/sync/worker.go:969-998` `syncIncremental[E]`.

```go
func syncIncremental[E any](ctx, tx, in, rows, includeDeleted bool) (int, error) {
    items := make([]E, 0, len(rows))
    // ... decode ...
    if !includeDeleted {                     // <-- delete this branch entirely
        items = filterByStatus(items, in.getStatus)
    }
    // ...
}
```

Removing `includeDeleted` parameter ripples to 13 `dispatchScratchChunk` call sites in `worker.go:1022-1180`. All within the same file. Straightforward. After this, upstream `status=deleted` rows flow through normally and upsert with their real `status='deleted'` value — exactly what D-03 anticipates.

Also remove `filterByStatus` helper (grep confirms it's called from only this site — a helper file at `internal/sync/filter.go` currently contains it per the `filter_test.go:21` hit).

## Config Grace Period Pattern

File: `internal/config/config.go` (522 lines).

**Current** (lines 179-183):

```go
includeDeleted, err := parseBool("PDBPLUS_INCLUDE_DELETED", true)
if err != nil {
    return nil, fmt.Errorf("parsing PDBPLUS_INCLUDE_DELETED: %w", err)
}
cfg.IncludeDeleted = includeDeleted
```

**Phase 68 replacement**:

```go
// PDBPLUS_INCLUDE_DELETED was removed in v1.16 Phase 68 — sync now always
// persists deleted rows as tombstones, and pdbcompat applies the upstream
// status × since matrix regardless of this gate. Grace period: warn if set,
// ignore the value. Flip to fail-fast in v1.17 per D-01.
if v := os.Getenv("PDBPLUS_INCLUDE_DELETED"); v != "" {
    slog.Warn("PDBPLUS_INCLUDE_DELETED is deprecated and ignored; remove it from your environment. This will be a startup error in v1.17.",
        slog.String("value", v),
    )
}
```

- Remove `Config.IncludeDeleted` field (struct lines 66-67).
- Remove `cmd/peeringdb-plus/main.go:233` `IncludeDeleted: cfg.IncludeDeleted,` line.
- Remove `WorkerConfig.IncludeDeleted` field (`internal/sync/worker.go:50`). Comment on line 50 says `IncludeDeleted bool` — just delete.
- Remove `TestLoad_IncludeDeleted` from `internal/config/config_test.go:235`. Add new test `TestLoad_IncludeDeleted_Deprecated` that asserts a WARN is logged when the env var is set (requires a `slog.Handler` spy — see `internal/testutil` for existing helpers).

**Logging:** `slog.Warn` without explicit context OK at startup; `Load()` has no ctx param today, and shipping one through would expand scope. Keep it package-level default `slog` call — matches the `validatePeeringDBURL` error style which just returns errors, no slog. Alternative: pass the logger into `Load()`. Current code doesn't, don't introduce it here.

**v1.17 future-flip:** replace `slog.Warn(...)` with `return nil, errors.New("PDBPLUS_INCLUDE_DELETED was removed in v1.16. Remove it from your environment.")`. That's a one-line swap when the time comes.

## Touchpoints & File Inventory

| Decision | File | Action | Lines |
|----------|------|--------|-------|
| D-01 config removal | `internal/config/config.go` | Delete `IncludeDeleted` field, `parseBool` call, assignment; add env-var deprecation WARN | Remove ~7 lines; add ~6 |
| D-01 config test | `internal/config/config_test.go:235` | Rename `TestLoad_IncludeDeleted` → `TestLoad_IncludeDeleted_Deprecated`; assert warn logged instead of value parsed | ~40 line rewrite |
| D-01 docs | `docs/CONFIGURATION.md:43` | Move `PDBPLUS_INCLUDE_DELETED` row from Sync Worker table to new "Removed in v1.16" section | ~5 lines |
| D-01 docs | `README.md:83` | Remove `PDBPLUS_INCLUDE_DELETED` row | 1 line |
| D-01 docs | `CLAUDE.md:133` | Remove `PDBPLUS_INCLUDE_DELETED` row | 1 line |
| D-01 main.go | `cmd/peeringdb-plus/main.go:233` | Remove `IncludeDeleted: cfg.IncludeDeleted,` | 1 line |
| D-01 CHANGELOG | `CHANGELOG.md` (new file) | Create + add deprecation entry; precedent: none exists in repo today | ~20 lines |
| D-02 sync worker | `internal/sync/worker.go:50` | Delete `WorkerConfig.IncludeDeleted` field | 1 line |
| D-02 sync worker | `internal/sync/worker.go:1022-1180` | Remove `includeDeleted := w.config.IncludeDeleted` + drop `includeDeleted` arg from 13 `syncIncremental` calls | ~15 edits |
| D-02 sync worker | `internal/sync/worker.go:969-998` | Remove `includeDeleted bool` param + `if !includeDeleted { filterByStatus(...) }` branch | ~3 lines |
| D-02 sync helpers | `internal/sync/filter.go` | Remove `filterByStatus` helper + its test file `filter_test.go` | Full-file delete |
| D-02 sync delete | `internal/sync/delete.go:56-134` | Rename 13 `deleteStale*` → `markStaleDeleted*`; flip body from `.Delete().Where(IDNotIn(chunk...))` to `.Update().Where(IDNotIn(chunk...)).SetStatus("deleted").SetUpdated(cycleStart)` | 13 × ~3 line edits |
| D-02 sync delete | `internal/sync/delete.go:27-54` | Update `deleteStaleChunked` fallback — the no-op chunk-over-SQLite-limit path currently silently accepts data loss; with soft-delete this stays a no-op but document it | ~5 line comment |
| D-02 sync worker | `internal/sync/worker.go:215-233 syncSteps()` | Update `deleteFn` signature to take `cycleStart time.Time`; update 13 syncStep entries | ~16 lines |
| D-02 sync worker | `internal/sync/worker.go:Sync` | Capture `cycleStart := time.Now()` at top of `Sync`; pass through to delete pass caller | ~2 lines |
| D-02 sync tests | `internal/sync/worker_test.go:143,953,1804,2157,2226,2289` | Remove `IncludeDeleted: …` from all fixture WorkerConfig literals | ~6 lines |
| D-02 sync tests | `internal/sync/integration_test.go:128,139,232,365-408,427,571` | Rewrite `TestSyncIncludeDeleted` → `TestSync_SoftDeleteMarksRows`; assert row *still exists* with `status='deleted'` | ~60 line rewrite |
| D-02 sync tests | `internal/sync/nokey_sync_test.go:174`, `replay_snapshot_test.go:71`, `worker_bench_test.go:627` | Remove `IncludeDeleted: true/false` lines | ~3 lines |
| D-02/03 docs | `CHANGELOG.md` + `docs/CONFIGURATION.md` | Document one-time gap: "Rows hard-deleted before Phase 68 are gone forever; `?status=deleted` is populated going forward from first post-upgrade sync." | ~10 lines |
| D-04 (not in scope this phase) | n/a | Do NOT add a `limit=0` safety ceiling — Phase 71 owns it | 0 |
| D-05 list matrix | `internal/pdbcompat/filter.go` | Add `applyStatusMatrix(isCampus, sinceSet bool) func(*sql.Selector)` helper | ~15 new lines |
| D-05+D-07 list matrix | `internal/pdbcompat/registry_funcs.go` | In each of 13 `wire*Funcs()` closures, append `preds = append(preds, predicate.Foo(applyStatusMatrix(isCampus, opts.Since != nil)))` after `applySince` | 13 × ~2 line edits |
| D-05 list matrix | `internal/pdbcompat/registry.go:71-384` | Remove `"status": FieldString` from all 13 `Fields` maps | 13 lines |
| D-06 pk matrix | `internal/pdbcompat/depth.go:55-260` | Each of 13 `get{Type}WithDepth`: replace `client.Foo.Get(ctx, id)` with `client.Foo.Query().Where(foo.ID(id), foo.StatusIn("ok", "pending")).Only(ctx)`; add same StatusIn predicate to the `depth>=2` `.Where(foo.ID(id))` paths | 26 edits |
| LIMIT-01 pagination | `internal/pdbcompat/response.go:52-70` | Change `parsed > 0` → `parsed >= 0`; keep `MaxLimit` clamp only when `limit > 0` | ~6 lines |
| LIMIT-01 list closures | `internal/pdbcompat/registry_funcs.go` | In each of 13 closures, wrap `.Limit(opts.Limit)` in `if opts.Limit > 0` conditional | 13 × ~3 line edits |
| LIMIT-02 depth cap (deferred) | `internal/pdbcompat/handler.go` | `serveList`: if `?depth` query present, respond with a 400 or no-op (don't accept list+depth until Phase 71). Recommendation: no-op + slog.DebugContext | ~5 lines |
| Phase 67 interaction | `internal/pdbcompat/registry_funcs_ordering_test.go:25` | No change needed — ordering assertions are status-agnostic | 0 |
| Tests (new) | `internal/pdbcompat/status_matrix_test.go` (new) | Table-driven test covering: list-no-since → status=ok only; list-since → (ok,deleted); list-since-campus → (ok,deleted,pending); pk → (ok,pending); `?status=deleted` alone → empty; `?limit=0` → unlimited | ~200 new lines |
| Tests (new) | `internal/pdbcompat/testdata/golden/<type>/list_with_since.json` + `detail_pending.json` | Fresh goldens for new scenarios | 13 × 2 new files |

## Validation Architecture

Nyquist validation is enabled (no `workflow.nyquist_validation: false` in config).

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `github.com/google/go-cmp/cmp` (already in use) |
| Config file | none — `go test ./...` is the entry |
| Quick run command | `go test ./internal/pdbcompat -run TestStatusMatrix -race` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| STATUS-01 | `GET /api/net` returns only `status=ok` when no `since` given | unit | `go test ./internal/pdbcompat -run TestStatusMatrix/list_no_since -race` | Wave 0 |
| STATUS-02 | `GET /api/net/<id>` returns row with `status=pending` | unit | `go test ./internal/pdbcompat -run TestStatusMatrix/pk_pending -race` | Wave 0 |
| STATUS-03 | `GET /api/net?since=N` returns `(ok, deleted)` | integration | `go test ./internal/pdbcompat -run TestStatusMatrix/list_since -race` | Wave 0 |
| STATUS-03 (campus) | `GET /api/campus?since=N` additionally admits `pending` | unit | `go test ./internal/pdbcompat -run TestStatusMatrix/list_since_campus -race` | Wave 0 |
| STATUS-04 | `GET /api/net?status=deleted` (no since) returns empty | unit | `go test ./internal/pdbcompat -run TestStatusMatrix/status_override_ignored -race` | Wave 0 |
| STATUS-05 | Sync persists deleted rows regardless of env var | integration | `go test ./internal/sync -run TestSync_SoftDeleteMarksRows -race` | Wave 0 (rewrite of TestSyncIncludeDeleted) |
| STATUS-05 | `PDBPLUS_INCLUDE_DELETED` env var triggers WARN | unit | `go test ./internal/config -run TestLoad_IncludeDeleted_Deprecated -race` | Wave 0 (rewrite of TestLoad_IncludeDeleted) |
| LIMIT-01 | `?limit=0` returns all matching rows | unit | `go test ./internal/pdbcompat -run TestLimitZero_Unlimited -race` | Wave 0 |
| LIMIT-02 | `?depth>0` + list currently not supported; guardrail test | unit | `go test ./internal/pdbcompat -run TestListDepth_Rejected -race` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/pdbcompat ./internal/sync ./internal/config -race -count=1`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green + `golangci-lint run` + `go vet ./...` + generated-code drift check (`go generate ./... && git diff --exit-code ent/ gen/ graph/`) per CI guard.

### Wave 0 Gaps

- [ ] `internal/pdbcompat/status_matrix_test.go` — new file covering STATUS-01 through STATUS-04 + LIMIT-01; ~200 lines. Table-driven with `httptest.Server` + 13-entity seed data.
- [ ] `internal/config/config_test.go` — add `TestLoad_IncludeDeleted_Deprecated`; needs a slog capture pattern (introduce `slogtest` helper or capture via a `bytes.Buffer` `slog.NewTextHandler`).
- [ ] `internal/sync/integration_test.go` — rewrite `TestSyncIncludeDeleted` to `TestSync_SoftDeleteMarksRows`: seed 3 orgs; remove org 3 from upstream fixture; run sync twice; assert org 3 still in DB with `status='deleted'` and `updated >= cycleStart`.
- [ ] `internal/pdbcompat/testdata/golden/<type>/list_with_since.json` + `detail_pending.json` — 13 × 2 new goldens (if using golden-file harness) or inline JSON assertions (simpler; recommended).
- [ ] Framework install: none (stdlib testing + existing `cmp`).

### Observable Properties to Lock (for the planner's Dimension 8 table)

1. **List without `since` always filters to `status=ok` regardless of `?status=` param.** Assert via `fixture DB with 1 ok + 1 pending + 1 deleted → GET /api/net → 1 row`.
2. **List with `?since=N` admits `ok` AND `deleted`; campus additionally admits `pending`.** Assert for each of 13 entities; campus row-count = 3 vs others = 2.
3. **pk GET admits `ok` OR `pending` but not `deleted`.** Seed 3 rows at ids 1/2/3 with statuses ok/pending/deleted; GET /api/net/1 → 200; GET /api/net/2 → 200; GET /api/net/3 → 404.
4. **`?limit=0` returns all rows up to any other filter.** Seed 500 rows; GET /api/net?limit=0 → 500 data items. (Phase 71 will cap; Phase 68 explicitly should not.)
5. **Soft-delete round-trip: row removed upstream during sync N → sync N+1 marks `status='deleted'`, `updated=cycle_start_N+1`.** Assert no `DELETE FROM` SQL runs (can check via SQL logger or by querying row count before + after = equal).
6. **`PDBPLUS_INCLUDE_DELETED=true` at startup → slog.Warn logged exactly once.** Assert via slog handler spy.
7. **`PDBPLUS_INCLUDE_DELETED=true` at startup → `Config` struct does not contain the field AND sync still persists deleted rows.** Combined assertion on Config reflection + TestSync_SoftDeleteMarksRows.

## Open Questions

1. **LIMIT-02 interpretation.** Upstream caps `depth>0` list responses at 250. pdbcompat currently does NOT support `?depth=` on list endpoints — only on detail. Does Phase 68 (a) add depth-to-list support with the 250 cap included, or (b) add a guardrail rejecting `?depth=` on list endpoints until Phase 71 adds memory-safe list+depth paths? Recommendation in research is (b); discuss-phase owner to confirm. The existing ROADMAP.md success criterion for Phase 68 reads "depth>0 responses continue to cap at the upstream API_DEPTH_ROW_LIMIT=250" which implies (a) is expected but seems out of scope — flag this during planning.

2. **`Limit(0)` semantics in ent.** Asserted in research as "pass no `.Limit()` call" but not empirically verified against ent v0.14. Plan task should include a quick probe test: `client.Foo.Query().Limit(0).All(ctx)` → 0 rows vs all rows? — resolve before the list-closure rewrite lands.

3. **`cycleStart` timestamp capture site.** Whether it lives on `WorkerConfig` (risky — mutable), the `Worker` struct (cleanest; tied to current Sync call), or plumbed through `context.Context` with a typed key (most idiomatic Go). Research recommends Worker-struct field reset in `Sync()` entry. Discuss in planning.

4. **Does `deleteStaleChunked`'s >32K-ID fallback still meaningfully work post-flip?** The fallback at `delete.go:51-53` is a no-op (`return 0, nil`). With soft-delete, the same fallback means "rows beyond 32K are silently NOT marked deleted." With data sizes at ~35K rows for netixlan today, this is on the edge. Separate concern — flag for SEED-004 or a quick follow-up task.

5. **`filterByStatus` helper usage outside sync.** Grep suggests it's only in `internal/sync` — confirm before deleting. `grep -rn filterByStatus` across repo.

6. **Golden file strategy for new scenarios.** Does the plan regen all 13×existing goldens (risk: merge-conflict city with Phase 67's recently regen'd set) or only ADD new goldens? Research recommends ADD-only — existing goldens seed only `status=ok` rows so behaviour is unchanged. Verify the seed helper in `internal/testutil/seed/` does not seed any non-ok status rows.

## Recommended Plan Split

**4 sequential plans**, matching the natural risk gradient (smallest-blast-radius first, so reversibility stays high):

### Plan 68-01: Config grace-period + sync `IncludeDeleted` field removal

**Scope:** D-01 (partial) + D-02 (wire-removal prep).

- Remove `Config.IncludeDeleted` field and `parseBool` call; add env-var deprecation WARN.
- Remove `WorkerConfig.IncludeDeleted` field; drop the wiring in `cmd/peeringdb-plus/main.go:233`.
- Remove `includeDeleted bool` parameter from `syncIncremental[E]` and its 13 call sites in `dispatchScratchChunk`.
- Delete `filterByStatus` helper + test file.
- Rewrite `TestSyncIncludeDeleted` → `TestSyncPersistsDeletedRows` (pre-soft-delete shape: asserts rows with `status=deleted` are upserted; different from the final Plan 68-02 test that asserts soft-delete marks rows).
- Rewrite `TestLoad_IncludeDeleted` → `TestLoad_IncludeDeleted_Deprecated`.
- Docs updates: `docs/CONFIGURATION.md`, `README.md`, `CLAUDE.md` — remove the variable row.

**Rationale:** This plan lands WITHOUT flipping the sync delete path. `deleteStale*` still hard-deletes. The only observable behaviour change is that deleted *upstream* rows are now persisted instead of filtered out — giving us a one-sync-cycle buffer where `status=deleted` rows start arriving in the DB but the delete pass still removes them on the NEXT sync cycle anyway. This is safe: behaviour doesn't actually diverge from today because the net result (rows with deleted status get removed) is unchanged.

**Estimated size:** 1 wave, ~10 files, ~150 lines.

### Plan 68-02: Sync soft-delete flip (`deleteStale*` → `markStaleDeleted*`)

**Scope:** D-02 (main event) + D-03 (documented gap).

- Rename 13 functions + 13 call sites in `syncSteps()`.
- Flip closure body from `.Delete()` to `.Update().SetStatus("deleted").SetUpdated(cycleStart)`.
- Plumb `cycleStart time.Time` through `syncStep.deleteFn` signature (from `Worker.Sync` top-of-function capture).
- Add `TestSync_SoftDeleteMarksRows` — 2 sync-cycle fixture: sync, remove row, sync again, assert row still present with `status='deleted'` and `updated >= cycleStart`.
- Add assertion: no `DELETE FROM` runs (hook into the `database/sql` layer via a query-capture wrapper, or verify by comparing row count pre/post).
- Docs update: `docs/CONFIGURATION.md` + `CHANGELOG.md` — note the one-time gap for rows hard-deleted before Phase 68.

**Rationale:** Sync is at the edge — if this regresses, only the next sync cycle is affected. A rollback is a revert of one commit; no data migration needed (soft-deletes just sit in the DB as inert tombstones).

**Estimated size:** 1 wave, 4 files, ~200 lines.

### Plan 68-03: pdbcompat status matrix + `limit=0` + list-depth guardrail

**Scope:** D-05, D-06, D-07 + LIMIT-01 + LIMIT-02 (as guardrail form).

- Add `applyStatusMatrix(isCampus, sinceSet bool)` helper in `filter.go`.
- Insert in 13 `wire*Funcs` closures in `registry_funcs.go` (2 lines each).
- Remove `"status": FieldString` from 13 `Fields` maps in `registry.go`.
- In 13 `get{Type}WithDepth` functions in `depth.go`, add `StatusIn("ok", "pending")` predicate to both Get paths.
- Fix `ParsePaginationParams` `limit=0` gate in `response.go`.
- In 13 list closures, conditionalise `.Limit(opts.Limit)` behind `if opts.Limit > 0`.
- Add LIMIT-02 guardrail in `handler.serveList`: reject or debug-log `?depth=` on list requests.
- Add `internal/pdbcompat/status_matrix_test.go` — table-driven across 13 entities + 5 scenarios.

**Rationale:** Largest behavioural change in pdbcompat, but cannot land until Plan 68-02 is in (rows with `status=deleted` must exist in the DB before the since-matrix test for `(ok, deleted)` can pass). Sequentially depends on 68-02.

**Estimated size:** 1 wave, 6 files, ~350 lines.

### Plan 68-04: Docs + CHANGELOG + golden regeneration check

**Scope:** Documentation + verification.

- Create `CHANGELOG.md` at repo root with v1.16 entry (milestone header + Phase 67 summary + Phase 68 breaking change noted).
- Update `docs/CONFIGURATION.md` to (a) remove `PDBPLUS_INCLUDE_DELETED` from Sync Worker table, (b) add a "Removed in v1.16" section explaining the grace period.
- Update `docs/API.md` (§ Filters or new § Status matrix) documenting the new `status`/`since` semantics as part of pdbcompat's explicit divergence record. Align with Phase 72's Divergence Registry (D-04 in Phase 72).
- Run `go generate ./...` drift check; no goldens should drift from Phase 68-02/03 changes because seed data uses `status='ok'` only, but verify.
- Manual smoke test checklist:
  - `curl -s 'http://localhost:8080/api/net?limit=0' | jq '.data | length'` → all rows, not 250.
  - `curl -s 'http://localhost:8080/api/net?status=deleted'` → `[]`.
  - `curl -s 'http://localhost:8080/api/net?since=0'` → all rows including deleted.

**Rationale:** Low-risk paper-trail work. Can run in parallel with 68-03 merge if the plan-checker permits, but safer to serialise so CHANGELOG can reference the final commit range.

**Estimated size:** 1 wave, 4 docs files, ~100 lines.

### Cross-plan sequencing notes

- 68-01 → 68-02 → 68-03 → 68-04 is strictly serial; no wave parallelism.
- **Total: ~800 lines across ~20 files**, matching the complexity class of Phase 67 (6 plans, ~1200 lines). Phase 68 is smaller because Phase 67 touched 3 surfaces; 68 is pdbcompat + sync only.
- 68-01's intermediate state is important: after 68-01 ships alone, deleted upstream rows *are* being upserted but the next sync cycle still hard-deletes them. This is a safe intermediate — behaviour converges on sync cadence.
- Recommend NOT shipping 68-01 alone to prod without 68-02 within the same release window — the intermediate state is not a durable production configuration. All four plans should land as one v1.16 release candidate alongside Phases 67+69-71 per STATE.md's coordinated-ship guidance.

## Sources

### Primary (HIGH confidence)

- **Upstream rest.py** (lines 463, 484-755) — `gh api repos/peeringdb/peeringdb/contents/src/peeringdb_server/rest.py` fetched 2026-04-19. [VERIFIED]
- `internal/pdbcompat/handler.go` — dispatch/list/detail flow [VERIFIED: Read]
- `internal/pdbcompat/registry_funcs.go` — 13 list closures [VERIFIED: Read]
- `internal/pdbcompat/registry.go` — `TypeConfig.Fields` maps + `reservedParams` [VERIFIED: Read]
- `internal/pdbcompat/filter.go` — `ParseFilters`, `buildPredicate` [VERIFIED: Read]
- `internal/pdbcompat/response.go` — `ParsePaginationParams`, `DefaultLimit`, `MaxLimit` [VERIFIED: Read]
- `internal/pdbcompat/depth.go` — `get{Type}WithDepth` pk path [VERIFIED: Read]
- `internal/sync/delete.go` — 13 `deleteStale*` funcs + `deleteStaleChunked` [VERIFIED: Read]
- `internal/sync/worker.go` — `WorkerConfig`, `syncSteps`, `syncIncremental`, `dispatchScratchChunk` [VERIFIED: Read]
- `internal/config/config.go` — `Load`, `parseBool`, `IncludeDeleted` field [VERIFIED: Read]
- `ent/schema/*.go` — `field.String("status")` + `index.Fields("status")` on all 13 schemas [VERIFIED: Grep]
- `docs/CONFIGURATION.md:43` — current `PDBPLUS_INCLUDE_DELETED` row format [VERIFIED: Read]

### Phase 67 context (HIGH confidence)

- `internal/pdbcompat/registry_funcs_ordering_test.go` — representative test pattern for phase behaviour lock-in [VERIFIED: Read]
- `.planning/STATE.md` — coordinated-ship guidance, D-0N summaries [VERIFIED: Read]
- `.planning/phases/67-default-ordering-flip/` — plan naming convention + golden regen approach [VERIFIED: ls]

### Secondary (MEDIUM confidence)

- Ent v0.14 `.Limit(0)` behaviour — [ASSUMED: skipping `.Limit(...)` is the unlimited idiom; verify in Plan 68-03 with a probe]
- `filterByStatus` is only called from `internal/sync/worker.go` — [ASSUMED: grep needs a final pass before deletion, see Open Question 5]

## Metadata

**Confidence breakdown:**

- Upstream semantics: **HIGH** — primary source fetched and line-verified.
- pdbcompat code map: **HIGH** — all call sites read in source.
- Sync worker flip pattern: **HIGH** — ent `.Update().Where(IDNotIn(...))` is idiomatic; no hidden edge cases identified.
- Config grace period: **HIGH** — pattern is trivial (env check + slog.Warn).
- Plan split: **MEDIUM** — 4-plan split is judgment; 3-plan is defensible if 68-01 + 68-02 merge.
- Ent `Limit(0)` behaviour: **LOW** — see Open Question 2.

**Research date:** 2026-04-19
**Valid until:** 2026-05-19 (30 days — stable domain, but ent versions and upstream can shift).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Ent's `.Limit(0)` generates `LIMIT 0` returning zero rows, and the correct "unlimited" idiom is omitting `.Limit()` | Exec Summary #4, Plan 68-03 | LIMIT-01 regresses (returns 0 rows instead of all); detectable in the first test run of Plan 68-03 |
| A2 | `filterByStatus` has no callers outside `internal/sync` | Exec Summary #7, Plan 68-01 | Deletion breaks a dependent package at compile time; trivial to catch |
| A3 | LIMIT-02's meaning is "guardrail / no list-depth support" rather than "add list-depth with 250 cap" | Upstream GT § + Plan 68-03 | Scope contest with plan-checker; discuss-phase may have intended (a). Surface as Open Question 1. |
| A4 | `testutil/seed.Full` seeds only `status=ok` rows, so existing goldens don't drift post-Phase-68 | Exec Summary #10, Open Question 6 | Plan 68-04 needs a golden regen commit; recoverable within the plan |
| A5 | `cycleStart := time.Now()` captured once in `Worker.Sync()` is an acceptable granularity (rows marked deleted in a single sync all share `updated`) | § Sync Worker Code Map | Low impact — just means many rows with identical `updated` timestamps, which tests + `?since=N` queries both handle fine |
