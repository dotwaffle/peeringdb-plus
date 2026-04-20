# Phase 68: Status × since matrix + limit=0 semantics — Pattern Map

**Mapped:** 2026-04-19
**Files analyzed:** 14 (12 modified + 2 new)
**Analogs found:** 14 / 14 (all have in-repo analogs)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/pdbcompat/registry_funcs.go` | list-handler (×13) | `opts → preds → ent.Query → []any` | self, pre-edit (same file, Phase 67 `.Order(...)` shape) | exact |
| `internal/pdbcompat/filter.go` (+applyStatusMatrix) | filter-helper | `QueryOptions → sql.Selector predicate` | `applySince` (same file, lines 48-54) | exact |
| `internal/pdbcompat/registry.go` | config-struct (Fields maps) | `url.Values → FieldType lookup` | self, current `Fields` literal | exact |
| `internal/pdbcompat/depth.go` | pk-handler (×13) | `(client, id, depth) → ent.Get/Query.Only → map[string]any` | `getOrgWithDepth` (self, lines 55-83) | exact |
| `internal/pdbcompat/response.go` (ParsePaginationParams) | response-envelope / pagination | `url.Values → (limit, skip)` | self, lines 52-70 (bug path) | exact |
| `internal/pdbcompat/handler.go` (serveList depth guardrail) | request-dispatch | `*http.Request → httperr / no-op` | `serveDetail` depth-parse (self, lines 201-208) | partial (opposite direction) |
| `internal/sync/delete.go` | sync-op (×13) | `(tx, remoteIDs) → int, err` via ent Delete → becomes Update | `deleteStaleOrganizations` (self, lines 56-61) | exact |
| `internal/sync/worker.go` (syncStep + syncIncremental) | sync-orchestration | `syncStep.deleteFn(ctx, tx, remoteIDs, cycleStart) → int` | `syncStep` (self, lines 210-213) | role-match (signature grows) |
| `internal/sync/worker.go` (dispatchScratchChunk) | sync-dispatch | `(ctx, tx, name, rows) → syncIncremental[E]` | self, lines 1021-1202 | exact (unconditional call) |
| `internal/sync/filter.go` (DELETED — `filterByStatus` helper in worker.go) | utility | generic `[]E → []E` filter | `filterByStatus` (worker.go:899-919) | exact (being removed) |
| `internal/sync/filter_test.go` (DELETED) | test | per-type status-filter coverage | self (lines 24-210) | exact (being removed) |
| `internal/config/config.go` | config-struct | `env → *Config` with fail-fast | `parseBool("PDBPLUS_INCLUDE_DELETED", true)` (self, lines 179-183) → replaced with `slog.Warn` deprecation | role-match (inverted pattern) |
| `internal/config/config_test.go` (TestLoad_IncludeDeleted) | test | `t.Setenv → Load() → assert` | self (lines 235-274) | exact (semantic flip) |
| **NEW** `internal/pdbcompat/status_matrix_test.go` | test (integration/unit) | `seed → httptest → GET /api/... → assert data[]` | `registry_funcs_ordering_test.go` (self, 340 lines) | exact |
| **NEW** `internal/sync/softdelete_test.go` (or `TestSync_SoftDeleteMarksRows` in `integration_test.go`) | test (integration) | `fixture → Sync → mutate fixture → Sync → assert status='deleted'` | `TestSyncIncludeDeleted` (integration_test.go:363-408) | exact (semantic flip) |
| `docs/CONFIGURATION.md` | docs | env var table | line 43 `PDBPLUS_INCLUDE_DELETED` row | exact |
| **NEW** `CHANGELOG.md` | docs | conventional release notes | `.planning/MILESTONES.md` v1.15/v1.13 entries | partial (MILESTONES is release-retrospective, not conventional CHANGELOG — author needs to pick Keep-a-Changelog shape; see § CHANGELOG bootstrap) |

---

## Pattern Assignments

### `internal/pdbcompat/filter.go` — add `applyStatusMatrix` helper (filter-helper)

**Analog:** `applySince` in the same file.

**Imports pattern** (already satisfied, lines 3-11):
```go
import (
    "entgo.io/ent/dialect/sql"
)
```

**Closest analog to copy** (`internal/pdbcompat/filter.go:48-54`):
```go
// applySince adds an updated >= since filter if Since is set in opts.
func applySince(opts QueryOptions) func(*sql.Selector) {
    if opts.Since == nil {
        return nil
    }
    return sql.FieldGTE("updated", *opts.Since)
}
```

**New helper (what the planner writes)** — mirror the shape exactly:
```go
// applyStatusMatrix returns the upstream rest.py:694-727 status predicate for
// list requests. sinceSet=false => status=ok (rest.py:725); sinceSet=true =>
// status IN (ok, deleted), plus pending when isCampus (rest.py:700-712).
func applyStatusMatrix(isCampus, sinceSet bool) func(*sql.Selector) {
    if !sinceSet {
        return sql.FieldEQ("status", "ok")
    }
    allowed := []string{"ok", "deleted"}
    if isCampus {
        allowed = append(allowed, "pending")
    }
    return sql.FieldIn("status", allowed...)
}
```

**Copy-paste boundaries:** helper signature mirrors `applySince` — accepts booleans, returns `func(*sql.Selector)`. Always returns non-nil (unlike `applySince`) since every list request needs a status predicate. Call site in each closure always appends.

**Gotchas:**
- `sql.FieldIn` takes `...any` — pass `allowed...` after the variadic expansion. Ent's `status` column is `field.String` so string vararg is correct.
- Do NOT return `nil` on the `sinceSet` branch — that would regress STATUS-04 (empty result on `?status=deleted` without since).

---

### `internal/pdbcompat/registry_funcs.go` — inject `applyStatusMatrix` into 13 closures (list-handler)

**Analog:** the 13 `wireXFuncs` closures themselves — pre-edit shape is uniform post-Phase-67. All 13 follow the same 5-line skeleton with a single predicate type swap.

**Copy-paste excerpt — current shape** (`internal/pdbcompat/registry_funcs.go:56-81`, `wireOrgFuncs`):
```go
func wireOrgFuncs() {
    setFuncs(peeringdb.TypeOrg,
        func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error) {
            preds := castPredicates[predicate.Organization](opts.Filters)
            if s := applySince(opts); s != nil {
                preds = append(preds, predicate.Organization(s))
            }
            q := client.Organization.Query().Where(preds...).Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))
            total, err := q.Count(ctx)
            if err != nil {
                return nil, 0, fmt.Errorf("count organizations: %w", err)
            }
            orgs, err := q.Limit(opts.Limit).Offset(opts.Skip).All(ctx)
            ...
```

**Target shape (two edits per closure — Phase 68 scope):**

1. **STATUS matrix injection** — insert immediately after the `applySince` block:
```go
preds = append(preds,
    predicate.Organization(applyStatusMatrix(false /*isCampus*/, opts.Since != nil)))
```
For `wireCampusFuncs`, pass `true` instead of `false` (per D-05, rest.py:702-712).

2. **LIMIT-01 conditionalisation** — replace the post-count line:
```go
// BEFORE:
orgs, err := q.Limit(opts.Limit).Offset(opts.Skip).All(ctx)

// AFTER:
q = q.Offset(opts.Skip)
if opts.Limit > 0 {
    q = q.Offset(opts.Skip).Limit(opts.Limit)  // limit combined with offset
}
orgs, err := q.All(ctx)
```
Simpler alternative (recommended) — preserve the single-line assignment:
```go
q2 := q.Offset(opts.Skip)
if opts.Limit > 0 {
    q2 = q2.Limit(opts.Limit)
}
orgs, err := q2.All(ctx)
```

**Copy-paste boundaries:**
- Predicate type name (`predicate.Organization`, `predicate.Network`, etc.) is the only per-closure variable — already established by the `castPredicates[T]` pattern.
- The `isCampus` bool is hard-coded per closure at the call site (not a runtime branch).
- Keep `q.Count(ctx)` running against the full filtered query — count must reflect the status-matrix-filtered row set. Do NOT reuse a pre-status `q` for count.

**Gotchas:**
- Don't hoist `applyStatusMatrix(false, opts.Since != nil)` into `filter.go`'s `ParseFilters` — it has to live per-closure because the predicate type is entity-specific.
- `opts.Since` is `*time.Time`; nil-check is the sense marker for `sinceSet`.
- Phase 67 regenerated 39 goldens against the `.Order(ent.Desc("updated"), ent.Desc("created"), ent.Desc("id"))` shape. Phase 68's changes are status-predicate and limit-conditional — goldens stay green because `testutil/seed.Full` only seeds `status=ok` rows (assumption A4 in research).

---

### `internal/pdbcompat/registry.go` — remove `"status": FieldString` from 13 Fields maps (config-struct)

**Analog:** self — every `TypeConfig.Fields` map has the line, always near the end next to `"created"`/`"updated"`.

**Excerpt to remove** (`internal/pdbcompat/registry.go:93`, representative of 13 sites):
```go
Fields: map[string]FieldType{
    ...
    "created":   FieldTime,
    "updated":   FieldTime,
    "status":    FieldString,     // <-- DELETE this line at 13 locations
},
```

**Sites to edit (grep):** `"status":\s+FieldString` — 13 hits at lines 93, 138, 181, 221, 238, 257, 271, 295, 311, 326, 345, 358, 380.

**Why remove from Fields instead of `reservedParams`:** `filter.go:44-48` silently ignores unknown fields. Removing status from `Fields` means `ParseFilters` silently drops `?status=…` without calling `buildPredicate`. This is exactly D-07's "empty override" semantic. Adding to `reservedParams` (registry.go:59-66) would work too but mixes semantic categories — reserved params are pagination/control params, not filterable columns.

**Copy-paste boundaries:** single-line removal. No reordering.

**Gotchas:**
- Don't also delete the ent `status` column — other paths (`depth.go` post-Phase-68, the soft-delete sync, GraphQL, entrest, grpcserver) still read/write it. This is a **filter-surface-only** removal.
- Phase 72's Divergence Registry will need an entry — pdbcompat explicitly diverges from upstream in that upstream accepts `?status=<value>` for list+since requests (rest.py:700-712 builds `allowed_status` then runs a final `filter(status__in=allowed)`). We silently override regardless of caller input — documented as intentional per D-07.

---

### `internal/pdbcompat/depth.go` — add `StatusIn("ok", "pending")` to pk paths (pk-handler ×13)

**Analog:** `getOrgWithDepth` — the 13 funcs share the same 2-branch shape (`depth>=2` with `.Query().Where(X.ID(id))...Only(ctx)` and `depth<2` with `client.X.Get(ctx, id)`).

**Excerpt — current shape** (`internal/pdbcompat/depth.go:55-83`, `getOrgWithDepth`):
```go
func getOrgWithDepth(ctx context.Context, client *ent.Client, id, depth int) (any, error) {
    if depth >= 2 {
        o, err := client.Organization.Query().
            Where(organization.ID(id)).       // <-- insertion point A
            WithNetworks().WithFacilities().WithInternetExchanges().WithCarriers().WithCampuses().
            Only(ctx)
        ...
    }

    o, err := client.Organization.Get(ctx, id)   // <-- insertion point B (Get has no predicates)
    if err != nil {
        return nil, fmt.Errorf("get organization %d: %w", id, err)
    }
    return organizationFromEnt(o), nil
}
```

**Target shape — both insertion points:**

*Insertion A* — add `StatusIn` to the existing `.Where(...)`:
```go
o, err := client.Organization.Query().
    Where(organization.ID(id), organization.StatusIn("ok", "pending")).   // <-- added
    WithNetworks().WithFacilities().WithInternetExchanges().WithCarriers().WithCampuses().
    Only(ctx)
```

*Insertion B* — replace `Get(ctx, id)` with an equivalent predicate-accepting query:
```go
// BEFORE:
o, err := client.Organization.Get(ctx, id)

// AFTER:
o, err := client.Organization.Query().
    Where(organization.ID(id), organization.StatusIn("ok", "pending")).
    Only(ctx)
```

**Verified: all 13 entities have `.StatusIn` generated methods** — confirmed by `ent/organization/where.go:1394-1396`:
```go
func StatusIn(vs ...string) predicate.Organization {
    return predicate.Organization(sql.FieldIn(FieldStatus, vs...))
}
```
Same pattern exists under `ent/{campus,carrier,carrierfacility,facility,internetexchange,ixfacility,ixlan,ixprefix,network,networkfacility,networkixlan,organization,poc}/where.go`.

**Copy-paste boundaries:**
- Per-entity package import already present — no new imports. Each `depth.go` func already imports its own ent sub-package (see lines 8-22 of `depth.go`).
- `ent.IsNotFound(err)` handling in `handler.go:218` keeps working transparently — a status=`deleted` row returning 404 from the status-filtered query yields the same sentinel as a missing ID would.

**Gotchas:**
- Literal string pair `"ok", "pending"` duplicates D-06's contract in 26 places. Consider a package-level `var pkAllowedStatuses = []string{"ok", "pending"}` — but the planner should confirm the grep-ability trade-off with plan-checker. Inline literals are easier to find via `grep`, a named slice is DRY. Research recommendation: inline literals (matches how `"deleted"` is a bare string in sync/worker.go:914).
- The `Get(ctx, id)` call sites lose their slight optimization (direct PK lookup vs. query with predicate). Negligible overhead — SQLite still hits the PK index first.
- Do NOT change the `depth>=2` `With*()` chain order — Phase 67 goldens and Phase 58 Poc privacy rely on it.

---

### `internal/pdbcompat/response.go` — fix `limit=0` gate (pagination)

**Analog:** self, current buggy shape.

**Excerpt — current** (`internal/pdbcompat/response.go:52-70`):
```go
func ParsePaginationParams(params url.Values) (limit, skip int) {
    limit = DefaultLimit
    if v := params.Get("limit"); v != "" {
        if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {  // <-- BUG
            limit = parsed
        }
    }
    if limit > MaxLimit {
        limit = MaxLimit
    }
    if v := params.Get("skip"); v != "" {
        if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
            skip = parsed
        }
    }
    return limit, skip
}
```

**Target shape** — flip `>` to `>=`, gate `MaxLimit` clamp on `limit > 0`:
```go
func ParsePaginationParams(params url.Values) (limit, skip int) {
    limit = DefaultLimit
    if v := params.Get("limit"); v != "" {
        if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {   // >= not >
            limit = parsed
        }
    }
    if limit > 0 && limit > MaxLimit {   // don't clamp the "0 = unlimited" sentinel
        limit = MaxLimit
    }
    // ... skip branch unchanged ...
    return limit, skip
}
```

**Copy-paste boundaries:** 2 character edits (`>` → `>=`, add `limit > 0 &&`). Keep `DefaultLimit=250` — that is the default WHEN unset, matching upstream where `?limit` unset still uses a product default. `limit=0` is the explicit "no limit" sentinel that honors upstream rest.py:736-737.

**Gotchas:**
- Ent's `.Limit(0)` generates `LIMIT 0` returning ZERO rows — do NOT pass 0 to ent. Gate at the call site (`registry_funcs.go` — see above).
- The return type stays `(int, int)` — no new sentinel type needed. `0` means "unlimited" downstream, and the 13 closures branch on it.

---

### `internal/pdbcompat/handler.go` — list-depth guardrail (request-dispatch)

**Analog:** `serveDetail` depth-parse (self, lines 201-208) — mirror image of what `serveList` does NOT do today.

**Excerpt — current `serveDetail` depth parse** (`internal/pdbcompat/handler.go:201-208`):
```go
depth := 0
if v := params.Get("depth"); v != "" {
    parsed, err := strconv.Atoi(v)
    if err == nil && (parsed == 0 || parsed == 2) {
        depth = parsed
    }
}
```

**Target shape — add to `serveList`** (per research § Open Question 1, recommendation b):
```go
// Phase 68 LIMIT-02 guardrail: upstream rest.py:744-748 caps list+depth at
// API_DEPTH_ROW_LIMIT=250. pdbcompat does not currently support list+depth;
// Phase 71 owns the memory-safe implementation. Until then, silently drop
// ?depth= on list (matches upstream's unsupported-depth handling).
if params.Get("depth") != "" {
    // Ignored by design; keep a debug log for operator visibility.
    // slog is not used elsewhere in this package — no import change.
}
```

**Copy-paste boundaries:** research Open Question 1 flags interpretation ambiguity with plan-checker. Two acceptable outcomes — (a) silent ignore + debug log; (b) 400 Bad Request. Research recommends (a). The planner MUST surface this decision at plan-checker; if the plan-checker selects (b), copy the error shape from `handler.go:134-139`:
```go
WriteProblem(w, httperr.WriteProblemInput{
    Status:   http.StatusBadRequest,
    Detail:   "?depth= is not supported on list endpoints (upstream caps at 250 — tracked by Phase 71)",
    Instance: r.URL.Path,
})
return
```

**Gotchas:**
- Don't accidentally enable list+depth by passing `opts.Depth` through — the `QueryOptions.Depth` field is defined (registry.go:40) but unused in list closures. Keep it unused for Phase 68.
- CONTEXT.md's "ROADMAP success criterion" wording suggests option (a) is expected by the plan-checker. Surface the ambiguity in the plan `<open_questions>` section.

---

### `internal/sync/delete.go` — 13 soft-delete conversions (sync-op)

**Analog:** `deleteStaleOrganizations` (self, lines 56-61) — all 13 funcs share the identical 2-line body shape.

**Excerpt — current** (`internal/sync/delete.go:56-61`):
```go
// deleteStaleOrganizations removes local organizations not present in the remote response.
func deleteStaleOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
    return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
        return tx.Organization.Delete().Where(organization.IDNotIn(chunk...)).Exec(ctx)
    }, "organizations")
}
```

**Target shape** — rename + flip body + add `cycleStart` parameter:
```go
// markStaleDeletedOrganizations marks local organizations absent from the
// remote response as status=deleted with updated=cycleStart, matching
// upstream PeeringDB's tombstone convention. Replaces hard-delete in v1.16
// per Phase 68 D-02 — tombstones enable the rest.py:700-712 status+since
// matrix to return historical deletions instead of silently dropping them.
func markStaleDeletedOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
    return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
        return tx.Organization.Update().
            Where(organization.IDNotIn(chunk...)).
            SetStatus("deleted").
            SetUpdated(cycleStart).
            Save(ctx)
    }, "organizations")
}
```

**Verified: ent generates `Update().Where(...).Save(ctx) (int, error)`** — confirmed `ent/organization_update.go:598-600`:
```go
func (_u *OrganizationUpdate) Save(ctx context.Context) (int, error) {
    return withHooks(ctx, _u.sqlSave, _u.mutation, _u.hooks)
}
```
Signature matches `Delete().Exec(ctx) (int, error)` exactly — outer return type `(int, error)` preserved.

**Sites to edit (grep):** `deleteStale\w+\(ctx context.Context, tx \*ent.Tx` — 13 hits at `internal/sync/delete.go:57, 63, 69, 75, 81, 87, 93, 99, 105, 111, 117, 123, 129`.

**Copy-paste boundaries:**
- Function rename (`deleteStale*` → `markStaleDeleted*`) ripples to 13 call sites in `worker.go:219-231 syncSteps()`.
- Parameter addition (`cycleStart time.Time`) ripples to `syncStep.deleteFn` type at `worker.go:212` and the `step.deleteFn(ctx, tx, remoteIDs)` call site at `worker.go:1240`.
- `deleteStaleChunked` helper signature (line 36) does not change — `cycleStart` is closed over by the per-entity closure.
- `time.Time` import needed — `delete.go` imports only ent packages today; add `"time"` to the block.

**Gotchas:**
- The `deleteStaleChunked` fallback path (lines 43-53) silently returns `0, nil` when `len(remoteIDs) > maxSQLVars` (32766). Post-flip, this means "rows beyond 32K are silently NOT marked deleted" — the research § Open Question 4 flags this for SEED-004 follow-up. Leave the behaviour untouched in Phase 68; update the comment to reflect the new "silently NOT soft-deleted" wording.
- `SetUpdated(cycleStart)` is **load-bearing** for STATUS-03 — `?since=N` queries filter on `updated >= N`, so deleted rows must carry a post-deletion timestamp to be reachable via since queries. Using the original row's `updated` would hide tombstones from `?since=recent`.
- Do NOT use `time.Now()` per closure — all rows marked in one sync cycle MUST share the same `cycleStart` for deterministic test assertions and atomic since-window semantics.

---

### `internal/sync/worker.go` — three related edits (sync-orchestration)

#### 1. `WorkerConfig.IncludeDeleted` field deletion (line 50)

**Current (lines 48-85):**
```go
type WorkerConfig struct {
    IncludeDeleted bool                // <-- DELETE
    IsPrimary      func() bool
    SyncMode       config.SyncMode
    ...
}
```

**Target:** remove the field entirely. Callers in `cmd/peeringdb-plus/main.go:233` and test fixtures (integration_test.go, worker_test.go, nokey_sync_test.go, replay_snapshot_test.go, worker_bench_test.go) must also drop their initialization line.

#### 2. `syncStep.deleteFn` signature change (lines 210-213)

**Current:**
```go
type syncStep struct {
    name     string
    deleteFn func(ctx context.Context, tx *ent.Tx, remoteIDs []int) (deleted int, err error)
}
```

**Target:** add `cycleStart time.Time`:
```go
type syncStep struct {
    name     string
    deleteFn func(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (marked int, err error)
}
```

#### 3. `syncSteps()` rename (lines 215-233) + `syncDeletePass` pass-through (lines 1228-1261)

**Current `syncSteps()`** — rename all 13 entries:
```go
{"org", deleteStaleOrganizations},   // <-- becomes markStaleDeletedOrganizations
```

**Current `syncDeletePass` call site** (`worker.go:1240`):
```go
deleted, stepErr := step.deleteFn(ctx, tx, remoteIDs)
```

**Target:** plumb `cycleStart` from `Worker.Sync` — capture once at the top of `Sync` (worker.go:291 `start := time.Now()` is the natural home — reuse it rather than adding a second clock), then pass through `syncDeletePass` signature:
```go
// Sync top:
start := time.Now()        // already exists; repurpose as cycleStart for soft-delete
// ...
// syncDeletePass signature:
func (w *Worker) syncDeletePass(ctx context.Context, tx *ent.Tx, remoteIDsByType map[string][]int, cycleStart time.Time) error {
    ...
    marked, stepErr := step.deleteFn(ctx, tx, remoteIDs, cycleStart)
    ...
}
```

#### 4. `syncIncremental[E]` drop `includeDeleted` param (lines 969-998) + `dispatchScratchChunk` drop 13 call-sites (lines 1021-1202)

**Current (line 969):**
```go
func syncIncremental[E any](ctx context.Context, tx *ent.Tx, in syncIncrementalInput[E], rows []scratchRow, includeDeleted bool) (int, error) {
    items := make([]E, 0, len(rows))
    for _, r := range rows {
        var v E
        if err := json.Unmarshal(r.raw, &v); err != nil {
            return 0, fmt.Errorf("decode %s id=%d: %w", in.objectType, r.id, err)
        }
        items = append(items, v)
    }
    if !includeDeleted {                  // <-- DELETE entire branch
        items = filterByStatus(items, in.getStatus)
    }
    if in.fkFilter != nil { ... }
    ...
}
```

**Target:** remove `includeDeleted` parameter, remove the entire `if !includeDeleted` branch. All 13 `dispatchScratchChunk` call-sites (lines 1021-1199) drop the `rows, includeDeleted` trailing arg and the leading `includeDeleted := w.config.IncludeDeleted` at line 1022.

**Also delete** (research Open Question 5 confirmed — grep pass verified `filterByStatus` is only referenced in `worker.go` + `filter_test.go`):
- `filterByStatus` function (worker.go:899-919) — generic helper
- Its test file `internal/sync/filter_test.go` (244-line file-delete)
- The `getStatus` field of `syncIncrementalInput[E]` at line 940 — becomes unused. Either delete (cleanest) or keep as defensive hook (research recommends delete).

**Copy-paste boundaries:**
- `syncIncrementalInput[E].getStatus` deletion ripples to 13 closures in `dispatchScratchChunk` (lines 1027, 1034, 1045, 1073, 1084, 1099, 1110, 1121, 1132, 1147, 1158, 1169, 1184). Each case drops one `getStatus: func(v peeringdb.X) string { return v.Status },` line.

**Gotchas:**
- `TestSyncIncludeDeleted` in `integration_test.go:363-408` asserts `IncludeDeleted=true` produces 3 orgs. Post-flip, that is just the base behavior — the test becomes `TestSync_SoftDeleteMarksRows` asserting that a row removed from upstream on cycle 2 is still present in DB with `status='deleted'` and `updated >= cycleStart2`.
- ALL `IncludeDeleted: false` literals across 5 test files (~10 sites, see research § Touchpoints) must be removed, not just toggled — the field is gone.
- The `w.running` atomic and GC tuning at the top of `Sync` (lines 282-316) are irrelevant to this phase — do not touch.

---

### `internal/config/config.go` — deprecation WARN (config-struct)

**Analog:** self — the current `parseBool` + field-assign pattern.

**Excerpt — current** (`internal/config/config.go:179-183`):
```go
includeDeleted, err := parseBool("PDBPLUS_INCLUDE_DELETED", true)
if err != nil {
    return nil, fmt.Errorf("parsing PDBPLUS_INCLUDE_DELETED: %w", err)
}
cfg.IncludeDeleted = includeDeleted
```

**Target — deprecation WARN pattern (v1.16 grace period):**
```go
// PDBPLUS_INCLUDE_DELETED was removed in v1.16 Phase 68 — sync now always
// persists deleted rows as tombstones (D-02), and pdbcompat applies the
// upstream status × since matrix regardless. Grace period: log and ignore
// if still set. Flip to startup error in v1.17 per D-01.
if v := os.Getenv("PDBPLUS_INCLUDE_DELETED"); v != "" {
    slog.Warn("PDBPLUS_INCLUDE_DELETED is deprecated and ignored; remove it from your environment. This will be a startup error in v1.17.",
        slog.String("value", v),
    )
}
```

**Also remove:**
- `Config.IncludeDeleted` field at line 66-67 (struct member).

**No existing `slog.Warn` pattern in `internal/config`** — `slog` is not currently imported there (confirmed via grep). The planner needs to add `"log/slog"` to the import block. This is a one-off; no other package-level `slog.Warn` call exists in `Load()`.

**Copy-paste boundaries:**
- `Load()` has no `ctx` parameter — use package-level `slog.Warn` (default logger), not `slog.WarnContext`. Matches `validatePeeringDBURL` error style which also doesn't thread ctx.
- Attribute key `"value"` reflects the env var value for operator diagnosis — DO NOT log secrets; `PDBPLUS_INCLUDE_DELETED` is a boolean flag, safe to log.
- Keep the `parseBool` helper function (used by `PDBPLUS_CSP_ENFORCE` at line 221) — only the call for `PDBPLUS_INCLUDE_DELETED` is removed.

**Gotchas:**
- No validate() change needed — `IncludeDeleted` has no validate rule.
- v1.17 flip pattern (future work, NOT in Phase 68):
  ```go
  if v := os.Getenv("PDBPLUS_INCLUDE_DELETED"); v != "" {
      return nil, fmt.Errorf("PDBPLUS_INCLUDE_DELETED was removed in v1.16 (value=%q). Remove it from your environment.", v)
  }
  ```
  One-line swap. Document in 68-04 CHANGELOG that the grace period ends at v1.17.

---

### `internal/config/config_test.go` — `TestLoad_IncludeDeleted_Deprecated` (test)

**Analog:** self — current `TestLoad_IncludeDeleted` (lines 235-274) shape has the `t.Setenv` + `Load()` + field assertion pattern. Rewrite the assertion half.

**Excerpt — current** (`internal/config/config_test.go:235-274`):
```go
func TestLoad_IncludeDeleted(t *testing.T) {
    tests := []struct { name, envVal string; want bool; wantErr bool; wantMsg string }{
        {name: "default is true", envVal: "", want: true},
        {name: "explicit true", envVal: "true", want: true},
        {name: "explicit false", envVal: "false", want: false},
        {name: "invalid bool", envVal: "maybe", wantErr: true, wantMsg: "PDBPLUS_INCLUDE_DELETED"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if tt.envVal != "" {
                t.Setenv("PDBPLUS_INCLUDE_DELETED", tt.envVal)
            }
            t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")
            cfg, err := Load()
            // ...
            if cfg.IncludeDeleted != tt.want {
                t.Errorf("IncludeDeleted = %v, want %v", cfg.IncludeDeleted, tt.want)
            }
        })
    }
}
```

**Target shape** — slog capture via `slog.NewTextHandler(&buf, ...)`:
```go
// TestLoad_IncludeDeleted_Deprecated asserts PDBPLUS_INCLUDE_DELETED is ignored
// with a WARN log during the v1.16 → v1.17 grace period (Phase 68 D-01).
func TestLoad_IncludeDeleted_Deprecated(t *testing.T) {
    t.Run("env_set_warns", func(t *testing.T) {
        t.Setenv("PDBPLUS_INCLUDE_DELETED", "true")
        t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")

        var buf bytes.Buffer
        prev := slog.Default()
        slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
        t.Cleanup(func() { slog.SetDefault(prev) })

        if _, err := Load(); err != nil {
            t.Fatalf("Load() unexpected error: %v", err)
        }
        if !strings.Contains(buf.String(), "PDBPLUS_INCLUDE_DELETED is deprecated") {
            t.Fatalf("expected deprecation WARN in log output; got: %q", buf.String())
        }
    })

    t.Run("env_unset_no_warn", func(t *testing.T) {
        t.Setenv("PDBPLUS_DB_PATH", t.TempDir()+"/test.db")
        var buf bytes.Buffer
        prev := slog.Default()
        slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
        t.Cleanup(func() { slog.SetDefault(prev) })

        if _, err := Load(); err != nil {
            t.Fatalf("Load() unexpected error: %v", err)
        }
        if strings.Contains(buf.String(), "PDBPLUS_INCLUDE_DELETED") {
            t.Fatalf("unexpected deprecation log when env unset: %q", buf.String())
        }
    })
}
```

**Copy-paste boundaries:**
- Test file already imports `strings` (evidence: `strings.Contains` usage at line 262). Add `"bytes"` and `"log/slog"` if not already imported.
- `slog.SetDefault` swap + `t.Cleanup` restore mirrors the standard "capture stderr" test pattern. The prior test did not need this — inject carefully.
- Keep `cfg.IncludeDeleted` references OUT of the new test — the field is gone.

**Gotchas:**
- `slog.SetDefault` is package-global — test must NOT run `t.Parallel()` unless each subtest manages its own logger via `slog.New(h).With(...)`. Research recommends sequential; both subtests are single-shot so the serialisation cost is negligible.
- Keep the `name: "invalid bool"` case — if set to garbage, it should still WARN (not error) because the grace period says "log and ignore the value". The prior test asserted err != nil; post-flip, no error is expected regardless of value.

---

### NEW `internal/pdbcompat/status_matrix_test.go` (test)

**Analog:** `internal/pdbcompat/registry_funcs_ordering_test.go` (340 lines, same Phase 67 shape).

**Closest template excerpt** (`registry_funcs_ordering_test.go:25-58`):
```go
func TestDefaultOrdering_Pdbcompat(t *testing.T) {
    t.Parallel()

    t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

    cases := []struct {
        name string
        seed func(t *testing.T, ctx *orderingTestCtx) []int
        path string
    }{
        {"Network", seedThreeNetworks, "/api/net"},
        {"Facility", seedThreeFacilities, "/api/fac"},
        {"InternetExchange", seedThreeIXes, "/api/ix"},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            client := testutil.SetupClient(t)
            octx := &orderingTestCtx{client: client, t0: t0}
            expected := tc.seed(t, octx)

            mux := newMuxForOrdering(client)
            srv := httptest.NewServer(mux)
            t.Cleanup(srv.Close)

            got := fetchIDOrder(t, srv.URL+tc.path)
            if !intSliceEqual(got, expected) {
                t.Fatalf("%s ordering mismatch: got %v, want %v", tc.name, got, expected)
            }
        })
    }
}
```

**Helpers to reuse verbatim from `registry_funcs_ordering_test.go`:**
- `newMuxForOrdering(client *ent.Client) *http.ServeMux` (lines 292-297) — registers pdbcompat on a fresh mux
- `fetchIDOrder(t, url) []int` (lines 301-327) — GETs + decodes PeeringDB envelope
- `intSliceEqual(a, b []int) bool` (lines 329-338)

**New scenarios for Phase 68 coverage** (one sub-test per STATUS-0N / LIMIT-0N req):
- `list_no_since` — seed (ok, pending, deleted) rows, `GET /api/net` → expect only ok
- `list_since` — seed same, `GET /api/net?since=0` → expect (ok, deleted); campus variant expects (ok, pending, deleted)
- `pk_ok` — `GET /api/net/<id-of-ok>` → 200
- `pk_pending` — `GET /api/net/<id-of-pending>` → 200
- `pk_deleted` — `GET /api/net/<id-of-deleted>` → 404
- `status_deleted_no_since` — `GET /api/net?status=deleted` → `data:[]` empty array (D-07)
- `limit_zero_unlimited` — seed 500 rows, `GET /api/net?limit=0` → 500 data items

**Copy-paste boundaries:**
- Use `testutil.SetupClient(t)` (not `testutil.SetupClientWithDB`) — the status/since matrix tests don't need the raw `*sql.DB`.
- Seed directly via `client.Network.Create().SetStatus("pending")...` — no need to extend `testutil/seed` helpers since the fixtures are scenario-specific.
- Reuse `intSliceEqual` and the envelope shape verbatim.

**Gotchas:**
- `testutil/seed.Full` seeds only `status=ok` — keep this assumption stable. Scenario-specific seed in this test file only.
- `?since=0` means "from Unix epoch 0" — results in ALL rows with updated > 1970. Use that (rather than a recent cutoff) to avoid timezone flakiness.
- Campus subtree needs a dedicated fixture — campus requires an Organization parent (FK). Reuse the `seed` helper pattern from `seedThreeNetworks`.

---

### NEW `internal/sync/softdelete_test.go` (or amendment to `integration_test.go`)

**Analog:** `TestSyncIncludeDeleted` in `integration_test.go:363-408`.

**Excerpt — current shape (integration_test.go:363-408):**
```go
// TestSyncIncludeDeleted verifies that IncludeDeleted=true includes
// status=deleted records in the database.
func TestSyncIncludeDeleted(t *testing.T) {
    t.Parallel()
    fs := newFixtureServer(t)
    client, db := testutil.SetupClientWithDB(t)
    // ...
    w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
        IncludeDeleted: true,
    }, slog.Default())
    // ...
    orgCount, err := client.Organization.Query().Count(ctx)
    if orgCount != 3 {
        t.Errorf("expected 3 orgs with IncludeDeleted=true, got %d", orgCount)
    }
    // ...
}
```

**Target shape — `TestSync_SoftDeleteMarksRows`** (semantic flip):
```go
// TestSync_SoftDeleteMarksRows asserts the Phase 68 soft-delete flip (D-02):
// a row removed from upstream on a subsequent sync cycle is marked
// status='deleted' with updated=cycleStart rather than physically deleted.
// Replaces v1.14 TestSyncIncludeDeleted — post-Phase-68 IncludeDeleted is
// unconditional (the env var is removed entirely).
func TestSync_SoftDeleteMarksRows(t *testing.T) {
    t.Parallel()

    // Phase 1: upstream has 3 orgs. Run first sync.
    fs := newFixtureServer(t)          // fixture serves 3 orgs
    client, db := testutil.SetupClientWithDB(t)
    pdbClient := newClientFor(t, fs)
    w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

    beforeCycle2 := time.Now()
    if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
        t.Fatalf("cycle 1 sync: %v", err)
    }

    // Assert all 3 orgs exist with status='ok'.
    count, _ := client.Organization.Query().Count(t.Context())
    if count != 3 { t.Fatalf("cycle 1: want 3 orgs, got %d", count) }

    // Phase 2: mutate fixture to drop org id=3, run second sync.
    fs.DropOrg(3)   // or equivalent — shape depends on fixture server API
    if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
        t.Fatalf("cycle 2 sync: %v", err)
    }

    // Assert org 3 STILL EXISTS with status='deleted' and updated >= beforeCycle2.
    org3, err := client.Organization.Query().Where(organization.ID(3)).Only(t.Context())
    if err != nil { t.Fatalf("org 3 missing after soft-delete: %v", err) }
    if org3.Status != "deleted" {
        t.Errorf("want status=deleted, got %q", org3.Status)
    }
    if !org3.Updated.After(beforeCycle2) {
        t.Errorf("want updated >= beforeCycle2 (%v), got %v", beforeCycle2, org3.Updated)
    }
}
```

**Copy-paste boundaries:**
- `newFixtureServer`, `testutil.SetupClientWithDB`, `newClientFor` are already available in `integration_test.go` — reuse them.
- `fs.DropOrg(3)` — fixture server may or may not have a drop method; research recommends checking `newFixtureServer` internals before assuming. If not present, either (a) add a method or (b) use two separate fixture servers (initial 3-org, followup 2-org) with a single `Worker` reused between them.
- `sync.WorkerConfig{}` literal — no fields needed. Default struct literal.
- Delete entire `IncludeDeleted: false/true` removals across the 5 test files listed in research § Touchpoints (integration_test.go:128, 139, 232, 365, 388, 394, 427, 571; worker_test.go:143, 953, 1804, 2157, 2226, 2289; nokey_sync_test.go:174; replay_snapshot_test.go:71; worker_bench_test.go:627). All are simple field-initializer removals.

**Gotchas:**
- `w.Sync` takes `config.SyncModeFull` literal — incremental mode skips the delete pass (worker.go:1233-1236), so test must use full.
- `beforeCycle2` captured BEFORE cycle 1 runs — this gives a tighter upper bound (the assertion is "updated is recent, not the original fixture timestamp").
- Using `t.Context()` (Go 1.24+) matches existing `integration_test.go` patterns like line 235.

---

### `docs/CONFIGURATION.md` — env var table update (docs)

**Analog:** `docs/CONFIGURATION.md:43` — the `PDBPLUS_INCLUDE_DELETED` row itself.

**Excerpt — current** (`docs/CONFIGURATION.md:42-43`):
```markdown
| `PDBPLUS_SYNC_MEMORY_LIMIT` | No | `400MB` | byte size | Peak Go heap ceiling ... |
| `PDBPLUS_INCLUDE_DELETED` | No | `true` | bool | Include objects with `status=deleted` during sync. Defaults to `true` to match the upstream PeeringDB API, which returns deleted rows in default fetches. Set to `false` to filter them client-side. |
```

**Target:**
1. DELETE the `PDBPLUS_INCLUDE_DELETED` row from the "Sync Worker" table.
2. ADD a new section below "Sync Worker" (or at the end of Environment Variables):

```markdown
### Removed in v1.16

| Variable | Removed in | Replacement | Migration |
|----------|------------|-------------|-----------|
| `PDBPLUS_INCLUDE_DELETED` | v1.16 (Phase 68) | None — deleted rows are now always persisted as tombstones and exposed via `?since=N` or pk lookup per the upstream [rest.py:694-727](...) status × since matrix. | Remove from your environment. A startup WARN is emitted during the v1.16 → v1.17 grace period if still set; it becomes a fatal startup error in v1.17. |
```

**Copy-paste boundaries:** pure markdown edit, no code. Also remove the row from:
- `README.md` (line ~83 per research)
- `CLAUDE.md` env var table (line ~133 per research)

**Gotchas:**
- The Phase 63 "Schema hygiene drops" note in `CLAUDE.md` is a template for how to describe removals — mirror that tone.
- Do NOT regenerate `CLAUDE.md` via `/gsd-docs-update` — it is not user-facing docs. Update manually per project convention.

---

### NEW `CHANGELOG.md` (docs)

**Analog:** `.planning/MILESTONES.md` — the closest in-repo release-note format. MILESTONES.md is retrospective (shipped notes per milestone), not conventional Keep-a-Changelog. No `CHANGELOG.md` exists at repo root today (research § Touchpoints confirmed).

**Excerpt — MILESTONES.md v1.15 entry** (lines 3-15):
```markdown
## v1.15 Infrastructure Polish & Schema Hygiene (Shipped: 2026-04-18)

**Phases completed:** 5 phases, 9 plans, 11 tasks

**Key accomplishments:**

- Dropped three ent schema fields (ixprefix.notes, organization.fac_count, organization.net_count) across all layers ...
- Serializer-layer field-level privacy primitive ...
...
```

**Target shape — Keep-a-Changelog ([keepachangelog.com/en/1.1.0/](https://keepachangelog.com/en/1.1.0/)) v1.16 stub:**
```markdown
# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Historical release notes prior to v1.16 live in `.planning/MILESTONES.md`.

## [Unreleased] — v1.16

### Breaking

- **Removed `PDBPLUS_INCLUDE_DELETED` environment variable.** Sync now
  always persists deleted rows as tombstones (soft-delete via
  `UPDATE ... SET status='deleted'`). During the v1.16 → v1.17 grace
  period, setting this variable triggers a startup WARN and is ignored.
  Remove it from your environment.
  *One-time gap:* rows hard-deleted by sync cycles BEFORE the v1.16
  upgrade are gone forever — `?status=deleted` and `?since=N` queries
  populate going forward from the first post-upgrade sync cycle.

### Added

- **pdbcompat status × since matrix** matching upstream `rest.py:694-727`.
  List requests without `?since` return only `status=ok`. List requests
  with `?since=N` admit `(ok, deleted)`, plus `pending` for campus.
  Single-object GETs (`/api/<type>/<id>`) admit `(ok, pending)`.
- **pdbcompat `?limit=0` semantics** match upstream `rest.py:734-737`:
  an explicit `limit=0` returns all matching rows (the default-when-unset
  remains `250`). Reserve deployment until Phase 71 ships the memory
  budget — see DEPLOYMENT.md § coordinated ship.

### Changed

- **Sync now soft-deletes** instead of hard-deleting. 13 `deleteStale*`
  functions renamed to `markStaleDeleted*`; they run
  `UPDATE ... SET status='deleted', updated=<cycle_start>` instead of
  `DELETE FROM`. Tombstone GC policy is deferred to SEED-004.
```

**Copy-paste boundaries:**
- This is a bootstrap — no prior CHANGELOG to copy from, so establish the K-a-C format for future phases to inherit.
- Prefer `## [Unreleased]` header initially; flip to `## [v1.16] - 2026-XX-XX` at ship time.
- Do NOT include internal planning references (Phase 68, D-01, STATUS-01) — CHANGELOG is user-facing, MILESTONES and planning docs are internal.

**Gotchas:**
- Project convention doesn't mandate K-a-C — the planner should briefly confirm at plan-checker (research § Plan 68-04). The alternative is to extend MILESTONES.md with a v1.16 entry and skip a root CHANGELOG. Research recommends CHANGELOG.md bootstrap because the Phase 68 breaking change (env var removal + gap note) belongs in user-facing docs.

---

## Shared Patterns

### Predicate-append after `applySince`

**Source:** `internal/pdbcompat/registry_funcs.go:60-62` (`wireOrgFuncs`)
**Apply to:** all 13 `wire*Funcs` closures

```go
preds := castPredicates[predicate.Organization](opts.Filters)
if s := applySince(opts); s != nil {
    preds = append(preds, predicate.Organization(s))
}
// Phase 68: add status matrix AFTER since — order-insensitive (both are WHERE-clause AND).
preds = append(preds, predicate.Organization(applyStatusMatrix(false, opts.Since != nil)))
```

### Ent `UpdateMany` signature preservation

**Source:** `ent/organization_update.go:598-600`
**Apply to:** all 13 `markStaleDeleted*` functions in `delete.go`

```go
// ent generates this shape for every Update builder — Save returns (int, error)
// matching the Delete-then-Exec shape, so the deleteStaleChunked callback
// signature survives verbatim.
func (_u *OrganizationUpdate) Save(ctx context.Context) (int, error) {
    return withHooks(ctx, _u.sqlSave, _u.mutation, _u.hooks)
}
```

### Campus as the sole "pending on list" special case

**Source:** Upstream `rest.py:702, 712` + CONTEXT.md D-05
**Apply to:** `applyStatusMatrix(isCampus, sinceSet)` callers in `registry_funcs.go`

Only `wireCampusFuncs` passes `true`. All other 12 pass `false`. Not a runtime branch — compile-time constant per closure.

### Slog capture in tests

**Source:** Not currently used in `internal/config/config_test.go` — introduce via `slog.NewTextHandler(&buf, ...)` + `slog.SetDefault` + `t.Cleanup` restore. Pattern is idiomatic Go; no existing analog in this repo's config tests.

**Apply to:** `TestLoad_IncludeDeleted_Deprecated` only. If broader adoption, consider a shared helper in `internal/testutil` — OUT OF SCOPE for Phase 68.

---

## No Analog Found

None. All 14 files have in-repo analogs. The only "net-new" shape is the slog-capture test pattern, but `slog.NewTextHandler` is stdlib.

---

## Metadata

**Analog search scope:**
- `internal/pdbcompat/` (all Go files + test files)
- `internal/sync/` (delete.go, worker.go, filter_test.go, integration_test.go, worker_test.go)
- `internal/config/` (config.go, config_test.go)
- `ent/organization/where.go` + `ent/organization_update.go` (representative ent generated code)
- `docs/CONFIGURATION.md` + `.planning/MILESTONES.md`
- `.planning/phases/67-default-ordering-flip/` (cross-phase reference for ordering-flip pattern)

**Files scanned:** 18 source files + 5 docs/planning files
**Pattern extraction date:** 2026-04-19
