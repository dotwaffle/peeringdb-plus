# meta.generated Field Behavior in PeeringDB API Responses

**Verified:** 2026-03-24 against beta.peeringdb.com (PeeringDB API observations); code references re-verified 2026-04-26.
**Relevant code:** `internal/peeringdb/client.go` (`parseMeta`, `FetchMeta`), `internal/peeringdb/stream.go` (`StreamAll` — earliest-meta aggregation across pages), `internal/sync/scratch.go` (`stageType`), `internal/sync/worker.go` (`Worker.Sync`, `stageOneTypeToScratch`).
**Live test:** `internal/peeringdb/client_live_test.go` (`TestMetaGeneratedLive`, gated by `-peeringdb-live` flag)

## Summary

The `meta.generated` field in PeeringDB API responses is an **undocumented cache artifact**. It is present ONLY on full-dataset responses served from PeeringDB's internal cache layer. Any parameterized query (using `limit`, `skip`, or `since` parameters) bypasses the cache and returns an empty meta object (`meta: {}`).

This field is NOT part of PeeringDB's documented API specification. Its behavior was determined empirically.

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

The `generated` value is a **float64 Unix epoch** with sub-second precision (e.g., `1774328452.459`). It represents when PeeringDB's cache was last rebuilt for this object type. Different types have independent cache generation times.

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

The `meta` object is empty. No `generated` field is present. This is because parameterized queries bypass PeeringDB's cache and query the database directly.

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

## Impact on Sync Pipeline

### Pagination aggregation (`StreamAll`)

`StreamAll` in `internal/peeringdb/stream.go` pages through the response and computes `FetchMeta.Generated` as the **earliest non-zero `meta.generated`** across all pages (lines 96–102). Pages with empty `meta` (Pattern 2) contribute nothing; the field stays zero only if **every** page returned empty meta.

### Full sync path

`Worker.Sync` (`internal/sync/worker.go:346`) calls `stageOneTypeToScratch`, which calls `scratch.stageType(ctx, pdbClient, name, time.Time{})` (`scratch.go:221`) with a zero `since` — i.e. `?depth=0` only, no `since` parameter. The cached first page carries `meta.generated`, so `FetchMeta.Generated` is non-zero in practice.

However, the **cursor written to `sync_cursors` is the sync cycle's `start` timestamp, not the response's `meta.generated`**. See `worker.go:785`:

```go
// Full sync (default, first sync, no cursor, or incremental-fallback).
if _, err := scratch.stageType(ctx, w.pdbClient, name, time.Time{}); err != nil {
    return time.Time{}, false, err
}
return start, false, nil
```

This is conservatively safe: any object modified between `start` and the moment the response was generated will be re-picked-up in the next cycle's `?since=start` window. The sync cycle's `start` is captured once per cycle and reused across all 13 entity types.

### Incremental sync path

When `mode == SyncModeIncremental` and a non-zero cursor exists for the type, `stageOneTypeToScratch` calls `stageType` with the cursor as `since` (`worker.go:760`). Paginated `?since=` requests bypass the cache and return `meta: {}` per page (Pattern 2), so `FetchMeta.Generated` is **zero** for the entire response.

The returned zero-time is propagated directly into the cursor map (`worker.go:744 → recordSuccess → UpsertCursor`). `UpsertCursor` (`internal/sync/status.go:72`) writes the zero timestamp without guarding against it. **There is no fallback like the v1.5-era `time.Now().Add(-5 * time.Minute)` line.** A successful incremental sync therefore writes `last_sync_at = 0` to `sync_cursors`, which would cause the next incremental tick to retry from epoch 0 — equivalent to a full re-sync, which is then absorbed by the same `stageType` machinery without harm.

In practice, `PDBPLUS_SYNC_MODE` defaults to `full` (see `internal/config/config.go`), so the incremental path is opt-in and not exercised on the default deployment.

### `parseMeta` implementation

`parseMeta` in `client.go:169` handles the float64-to-time conversion:

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

Sub-second precision is truncated to integer seconds — acceptable because the sync cursor only needs second-level granularity.

## Key Takeaways

1. **meta.generated is a cache artifact, not a guaranteed API field.** Do not rely on its presence for any request that includes `limit`, `skip`, or `since` parameters.

2. **The full sync cursor uses the sync cycle's `start` timestamp, not `meta.generated`.** This is conservatively safe — any window between `start` and the response's actual generation time gets re-fetched on the next cycle.

3. **The incremental sync cursor will be zero on a clean run.** Paginated `?since=` requests return `meta: {}`, so `FetchMeta.Generated` aggregates to zero. `UpsertCursor` writes the zero value without guarding against it; the next cycle re-enters the full path. There is no `time.Now().Add(-5 * time.Minute)` fallback in current code (that existed pre-v1.5 and was refactored away).

4. **Full fetch responses are the only reliable source of cache timestamps.** If you need to know when PeeringDB last rebuilt its cache for a type, use a full fetch without parameters.

5. **This behavior is undocumented by PeeringDB.** The only reference is [GitHub issue #776](https://github.com/peeringdb/peeringdb/issues/776), which mentions the field exists but does not document when it appears or is absent.

---
*Verified empirically against beta.peeringdb.com on 2026-03-24*
*Live test: `go test ./internal/peeringdb/ -run TestMetaGeneratedLive -peeringdb-live -v`*
