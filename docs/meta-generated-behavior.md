# Upstream `meta.generated`

This page is about the `meta` object in the upstream response envelope.
For the per-object `meta` field on networks and network IX LAN connections,
see [API.md § Metadata document (`meta`)](API.md#metadata-document-meta).

Upstream PeeringDB serves some list requests from a cache file,
one file for each type and depth
(2.83.0 `api_cache.py:85-88`).
Only these responses carry `meta.generated`.
The value is the modification time of the cache file,
in Unix seconds with a fraction
(`api_cache.py:145`):

```json
{
  "meta": {"generated": 1774328452.459},
  "data": [...]
}
```

Other list responses have no `generated` key.
Their `meta` is usually `{}`.
A `?page=` request adds `meta.pagination`.

A list request uses the cache only when all of these are true
(`api_cache.py:90-124`):

- The request has no filter and no `?since=`.
- At depth 0 (the default), the request has no `?limit=`,
  or a `?limit=` above 250.
- The request is a list, not one object.
- The cache file exists.

`?skip=` does not stop the cache.
Upstream slices the cached rows.

## Effect on the sync

The sync does not use `meta.generated` for its cursor
or for the start of any fetch.
Incremental fetches send `?since=` and `?limit=250`,
so they never get the value.
The sync cursor is the newest `updated` value in each local table
(`GetMaxUpdated`, `internal/sync/cursor.go`).
See [ARCHITECTURE.md § Data flow](ARCHITECTURE.md#data-flow).

`streamDecodeResponse` (`internal/peeringdb/stream.go`)
parses the value into `FetchMeta.Generated`,
rounded down to whole seconds.
`scratch.stageType` returns it,
together with the newest `updated` value among the staged rows.
The worker reports `meta.generated` on the fetch span and in logs only
(see [Stale snapshots](#stale-snapshots)).

## Stale snapshots

Of the requests that the sync sends,
only the bare list of a full fetch comes from the cache.
Upstream's `pdb_api_cache` command builds each cache file with
`?updated__lte=<build start>`,
and `meta.generated` is the modification time of the file.
The cache can be hours or days old
(on 2026-09-23 the netixlan cache was from 2026-09-22 23:14:11Z,
before the 2.83.0 release).
A row that changed after the build appears in its old state,
with its old `updated` value.

`meta.generated` is not a safe start for the window fetch
that follows the snapshot:
the file is written after the query ran,
and one build took more than 24 minutes between the `net` and `netixlan` files.
The worker starts the window at the earlier of the pre-cycle cursor and
the newest `updated` value in the snapshot
(`snapshotWindowStart`, `internal/sync/worker.go`).
That value is at or before the query cutoff,
so the window's `?since=` filter matches every row that changed after the build.
The window is paged like any `?since=` fetch,
so rows that share one `updated` value can still be skipped at a page edge
(see [ARCHITECTURE.md § Daily full reconcile](ARCHITECTURE.md#daily-full-reconcile)).
Full-mode upserts also keep a stored row whose `updated` value is newer than
the snapshot's version and not older than the snapshot's newest row
(`skipUnchangedPredicate`, `internal/sync/upsert.go`).

The fetch span carries `pdbplus.sync.snapshot.generated`,
`pdbplus.sync.snapshot.max_updated` and `pdbplus.sync.window.since`.
When the window of a populated table starts at the snapshot, the worker logs
`INFO "window starts at full snapshot"` with `snapshot_lag`.
Only a large lag, or an old `snapshot_generated`, shows a stale cache.

## Live check

`go test ./internal/peeringdb/ -run TestMetaGeneratedLive -peeringdb-live -v`
sends requests to beta.peeringdb.com.
It checks that a full fetch has `meta.generated`,
and that a `?since=` fetch (paged, and with a future timestamp) does not.
