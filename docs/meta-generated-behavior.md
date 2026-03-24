# meta.generated Field Behavior in PeeringDB API Responses

**Verified:** 2026-03-24 against beta.peeringdb.com
**Relevant code:** `internal/peeringdb/client.go` (parseMeta), `internal/sync/worker.go` (fetchIncremental fallback)
**Live test:** `internal/peeringdb/client_live_test.go` (TestMetaGeneratedLive, gated by -peeringdb-live flag)

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

### Full Sync Path

The full sync (`internal/sync/worker.go`, Sync method) fetches with `?depth=0` (no limit/skip). `parseMeta()` correctly extracts the `generated` timestamp from the cached response. This timestamp becomes the cursor for subsequent incremental syncs.

### Incremental Sync Path

Incremental sync (`fetchIncremental` in worker.go) fetches with `?depth=0&limit=250&skip=0&since={cursor}`. Since paginated requests always return `meta: {}`, `parseMeta()` returns zero time for every page.

The fallback at `worker.go:731-733` handles this:

```go
generated := result.Meta.Generated
if generated.IsZero() {
    generated = time.Now().Add(-5 * time.Minute)
}
```

This 5-minute buffer overlaps with the most recent sync window, ensuring no objects are missed between sync cycles. For hourly sync intervals, a 5-minute overlap is conservative and safe.

### parseMeta Implementation

`parseMeta` in `client.go:108-119` handles the float64-to-time conversion:

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

Note: Sub-second precision is truncated to integer seconds. This is acceptable because the sync cursor only needs second-level granularity.

## Key Takeaways

1. **meta.generated is a cache artifact, not a guaranteed API field.** Do not rely on its presence for any request that includes `limit`, `skip`, or `since` parameters.

2. **The existing fallback is correct.** The 5-minute buffer in `fetchIncremental` handles the absence of `meta.generated` on paginated responses without data loss.

3. **Full fetch responses are the only reliable source of cache timestamps.** If you need to know when PeeringDB last rebuilt its cache for a type, use a full fetch without parameters.

4. **This behavior is undocumented by PeeringDB.** The only reference is [GitHub issue #776](https://github.com/peeringdb/peeringdb/issues/776), which mentions the field exists but does not document when it appears or is absent.

---
*Verified empirically against beta.peeringdb.com on 2026-03-24*
*Live test: `go test ./internal/peeringdb/ -run TestMetaGeneratedLive -peeringdb-live -v`*
