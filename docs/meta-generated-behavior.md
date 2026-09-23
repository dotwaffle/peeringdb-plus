# meta.generated Field Behavior in PeeringDB API Responses

**Upstream behaviour verified:** 2026-03-24 against beta.peeringdb.com
(empirical API observations).
**Sync-pipeline impact re-derived:** 2026-06-03 against the v1.18.10+
`MAX(updated)` cursor model.
**Relevant code:** `internal/peeringdb/client.go`
(`parseMeta`, `FetchMeta`),
`internal/peeringdb/stream.go`
(`StreamAll` — earliest-meta aggregation across pages),
`internal/sync/scratch.go` (`stageType`), `internal/sync/cursor.go`
(`GetMaxUpdated` — the current cursor source),
`internal/sync/worker.go`
(`Worker.Sync`, `syncFetchPass`, `stageOneTypeToScratch`).
**Live test:** `internal/peeringdb/client_live_test.go`
(`TestMetaGeneratedLive`, gated by `-peeringdb-live` flag)

## Summary

The `meta.generated` field in PeeringDB API responses is an
**undocumented cache artifact**.
It is present ONLY on full-dataset responses served from PeeringDB's internal
cache layer.
Any parameterized query (using `limit`, `skip`, or `since` parameters) bypasses
the cache and returns an empty meta object (`meta: {}`).

This field is NOT part of PeeringDB's documented API specification.
Its behavior was determined empirically.

> **Historical note.**
> Earlier versions used `meta.generated`
> (aggregated across pages) as the sync cursor.
> That coupling was removed in v1.18.10 —
> the cursor is now derived from `MAX(updated)` on each entity table
> (see [Impact on the sync pipeline](#impact-on-the-sync-pipeline)).
> `meta.generated` is still parsed and returned.
> Since v1.28.1 the worker reports it on the fetch span and in logs only
> (see [Stale snapshots](#stale-snapshots-v1281)).
> The empirical observations below remain accurate
> and are why the field is unsuitable as a cursor.

## Observed Response Structures

### Pattern 1: Full Fetch (cached response)

**Request:** `GET /api/{type}?depth=0` (no limit, skip, or since parameters)

**Response:**

```json
{
  "meta": {
    "generated": 1774328452.459
  },
  "data": [
    {"id": 1, "name": "..."},
    ...
  ]
}
```

The `generated` value is a **float64 Unix epoch** with sub-second precision
(e.g., `1774328452.459`).
It represents when PeeringDB's cache was last rebuilt for this object type.
Different types have independent cache generation times.

**Observed values (2026-03-24):**

| Type | meta.generated | Approximate Time | Data Count |
|------|---------------|------------------|------------|
| net | 1774328452.459 | 2026-03-21 12:20 UTC | 34,235 |
| ix | 1774328965.690 | 2026-03-21 12:29 UTC | 1,302 |
| fac | 1774329027.392 | 2026-03-21 12:30 UTC | 5,855 |
| carrier | 1774329027.782 | 2026-03-21 12:30 UTC | 274 |
| campus | 1774329289.908 | 2026-03-21 12:34 UTC | 74 |
| org | 1774328350.627 | 2026-03-21 12:19 UTC | 33,248 |

### Pattern 2: Paginated/Incremental Request (live database query)

**Request:** `GET /api/{type}?depth=0&limit=250&skip=0&since={unix_timestamp}`

**Response:**

```json
{
  "meta": {},
  "data": [
    {"id": 1, "name": "..."},
    ...
  ]
}
```

The `meta` object is empty.
No `generated` field is present.
This is because parameterized queries bypass PeeringDB's cache
and query the database directly.

Also applies to:

- `?depth=0&limit=N` (limit without since)
- `?depth=0&limit=N&skip=M` (paginated without since)
- `?depth=0&since=T` (since without limit -- still bypasses cache)

### Pattern 3: Empty Result Set

**Request:** `GET /api/{type}?depth=0&since={future_timestamp}`

**Response:**

```json
{
  "meta": {},
  "data": []
}
```

Empty result sets also return an empty meta object.

## Impact on the sync pipeline

### Pagination aggregation (`StreamAll`)

`StreamAll` in `internal/peeringdb/stream.go` pages through the response
and computes `FetchMeta.Generated`
as the **earliest non-zero `meta.generated`** across all pages.
Pages with empty `meta` (Pattern 2) contribute nothing;
the field stays zero only if **every** page returned empty meta.
`scratch.stageType` returns this aggregated value to its caller,
together with the newest `updated` value among the staged rows.
The worker uses neither for the cursor.

### The cursor is `MAX(updated)`, not `meta.generated` (v1.18.10+)

Each sync cycle derives a per-type cursor from
`SELECT updated FROM <table> ORDER BY updated DESC LIMIT 1` — `GetMaxUpdated` in
`internal/sync/cursor.go` — rather than from `meta.generated` or any persisted
`sync_cursors` row.
`syncFetchPass` (`internal/sync/worker.go`) calls
`GetMaxUpdated(ctx, w.db, table)` for each entity table and hands the result to
`stageOneTypeToScratch`:

- **Incremental mode with a non-zero cursor** →
  `scratch.stageType(ctx, pdbClient, name, cursor)`, i.e. a paginated
  `?since=<cursor>` fetch.
  Upstream's `?since=N` filter is strict (`updated > N`),
  but `N` is whole seconds while upstream stores sub-second `updated` values,
  so upstream usually re-sends the boundary row.
  The `updated` column is indexed on all 13 entity tables,
  and re-fetching the boundary row each cycle is an index-backed no-op
  (the `OnConflict` UPDATE is skipped on unchanged rows).
- **Zero cursor (empty table) or full mode** → a bare `?depth=0` list
  (live statuses only: `ok`, plus `not-operational` on netixlan),
  the existing full-sync path,
  then a `?since=` window fetch
  (see [Stale snapshots](#stale-snapshots-v1281)).

The `meta.Generated` value returned by `stageType` is **not** used to advance
the cursor.

**Why the change.**
The earlier design aggregated `meta.generated` into the cursor
and wrote it to `sync_cursors`.
Because `?since=` responses omit `meta.generated`
(Pattern 2),
a successful incremental cycle stored the zero-time,
and the next tick then re-entered the full bare-list path —
an oscillation visible on Grafana
(2026-04-28: `total_objects` flapping between ~1,310 and ~270,180 every 15
minutes).
Deriving the cursor from `MAX(updated)` on the table the sync just wrote removes
the dependency on a response field that incremental requests never return.

**Cross-row-inconsistency escape hatch.**
A pure `?since=` design can permanently miss a row R
(`updated < M`)
if upstream serves a response where a newer row R′
(`updated = M`) is present but R is absent.
`PDBPLUS_FULL_SYNC_INTERVAL`
(default `24h`)
forces a periodic full bare-list re-fetch to recover from this,
independent of the per-cycle `MAX(updated)` cursor.

The `sync_cursors` table
and the `UpsertCursor` / `GetCursor` helpers in `internal/sync/status.go` are no
longer on the cursor-advancement path; cursor advancement is implicit in the
entity tables' own `updated` columns.

### Stale snapshots (v1.28.1)

The bare list is the one request shape that upstream serves from its API cache.
Upstream's `pdb_api_cache` command builds each cache file with
`?updated__lte=<build start>`,
and `api_cache.py` reports the file's modification time as `meta.generated`.
The cache can be hours or days old
(on 2026-09-23 the netixlan cache was from 2026-09-22 23:14:11Z,
before the 2.83.0 release).
A row that changed after the build appears in its old state,
with its old `updated` value.

`meta.generated` is not a safe start for the window that follows the snapshot:
the file is written after the query ran,
and one build took more than 24 minutes between the `net` and `netixlan` files.
The worker starts the window at the earlier of the pre-cycle cursor and
the newest `updated` value in the snapshot (`snapshotWindowStart`).
That value is at or before the query cutoff,
so the window's `?since=` filter matches every row that changed after the build.
The window is paged like any `?since=` fetch,
so rows that share one `updated` value can still be skipped at a page edge
(see `docs/ARCHITECTURE.md` § Daily full reconcile).
Full-mode upserts also keep a stored row whose `updated` value is newer than
the snapshot's version and not older than the snapshot's newest row
(`skipUnchangedPredicate`).

The fetch span carries `pdbplus.sync.snapshot.generated`,
`pdbplus.sync.snapshot.max_updated` and `pdbplus.sync.window.since`.
When the window of a populated table starts at the snapshot, the worker logs
`INFO "window starts at full snapshot"` with `snapshot_lag`.
Only a large lag, or an old `snapshot_generated`, shows a stale cache.

### `parseMeta` implementation

`parseMeta` in `internal/peeringdb/client.go` handles the float64-to-time
conversion:

```go
func parseMeta(raw json.RawMessage) time.Time {
    if len(raw) == 0 {
        return time.Time{}
    }
    var meta struct {
        Generated float64 `json:"generated"`
    }
    if err := json.Unmarshal(raw, &meta); err != nil || meta.Generated == 0 {
        return time.Time{}
    }
    return time.Unix(int64(meta.Generated), 0)
}
```

Sub-second precision is truncated to integer seconds.
This was acceptable when `meta.generated` fed the cursor; it is now immaterial,
since the cursor is `MAX(updated)`.

## Key Takeaways

1. **meta.generated is a cache artifact, not a guaranteed API field.**
   Do not rely on its presence for any request that includes `limit`, `skip`,
   or `since` parameters.

2. **The sync cursor is `MAX(updated)` per entity table, not `meta.generated`.**
   It is derived freshly each cycle by `GetMaxUpdated`
   (`internal/sync/cursor.go`), is never persisted to `sync_cursors`, and does
   not depend on any response metadata.
   `meta.generated` is still parsed and returned; the worker reports it for
   observability only.

3. **Incremental responses omit `meta.generated`, which is exactly why it cannot
   be the cursor.**
   The `MAX(updated)` model sidesteps this:
   upstream usually re-sends the boundary row and `updated` is indexed,
   so the boundary row is re-fetched idempotently.
   `PDBPLUS_FULL_SYNC_INTERVAL`
   (default 24h)
   forces a periodic full sync to recover from pathological upstream cross-row
   inconsistency.

4. **Full fetch responses are the only reliable source of cache timestamps.**
   If you need to know when PeeringDB last rebuilt its cache for a type,
   use a full fetch without parameters.

5. **This behavior is undocumented by PeeringDB.** The only reference is
   [GitHub issue #776](https://github.com/peeringdb/peeringdb/issues/776), which
   mentions the field exists but does not document when it appears or is absent.

---

*Upstream behaviour verified empirically against beta.peeringdb.com on
2026-03-24* *Live test: `go test ./internal/peeringdb/ -run
TestMetaGeneratedLive -peeringdb-live -v`*
