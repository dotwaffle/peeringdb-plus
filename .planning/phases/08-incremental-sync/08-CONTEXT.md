# Phase 8: Incremental Sync - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

Add configurable incremental sync mode that fetches only objects modified since the last successful sync per type, using PeeringDB's `?since=` parameter. Track per-type cursors derived from PeeringDB's `meta.generated` epoch. Automatic fallback to full sync on per-type failure.

</domain>

<decisions>
## Implementation Decisions

### Configuration
- New env var: `PDBPLUS_SYNC_MODE=full|incremental` (default `full`)
- POST /sync accepts `?mode=full|incremental` query param to override config per-request
- First sync always performs full fetch regardless of mode (no ?since= on empty database)

### Client API
- FetchAll gains functional options pattern: `WithSince(time.Time)`
- FetchAll returns `FetchResult{Data []json.RawMessage, Meta *FetchMeta}` struct instead of bare slice
- FetchMeta includes the earliest `generated` epoch from PeeringDB's `meta` across all pages
- Parse `meta.generated` (epoch float) from each response page to determine cache timestamp

### Since Cursor Logic
- Use PeeringDB's `meta.generated` epoch if present — this is when PeeringDB's cache was snapshotted
- If `generated` not present (live response), fall back to `started_at - 5min` buffer
- Use earliest (oldest) `generated` across all pages for a type (most conservative)
- Quick verification in this phase: check whether depth=0 responses include `meta.generated`

### Database
- New `sync_cursors` table: `type TEXT PRIMARY KEY, last_sync_at DATETIME, last_status TEXT`
- Created by extending existing `InitStatusTable` function (same function, both tables)
- sync_status table unchanged — continues for overall sync history
- Per-type cursors updated only on successful sync commit

### Sync Behavior
- Single transaction (same atomicity as full sync)
- Incremental: upsert changed objects only, skip stale deletion step
- Full sync: existing behavior unchanged (upsert + delete stale)
- Hard deletes not caught by incremental (accepted gap — user switches to full mode when needed)

### Failure Handling
- On incremental failure for a type → immediate full fallback for that type (no incremental retry first)
- Observability: `pdbplus.sync.type.fallback` counter + WARN log + OTel span event

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `peeringdb.Client.FetchAll()` in internal/peeringdb/client.go — needs functional options
- `peeringdb.Response[T]` already captures `Meta json.RawMessage` — just not parsed
- `sync.Worker.Sync()` in internal/sync/worker.go — single-transaction, per-type step loop
- `sync.WorkerConfig` struct with `IncludeDeleted bool` and `IsPrimary bool`
- `sync.InitStatusTable()` creates sync_status — extend for sync_cursors
- 13 per-type sync methods follow identical pattern: fetch → filter → upsert → delete stale

### Established Patterns
- Config loaded from env vars with defaults in internal/config/config.go
- OTel metrics defined in internal/otel/metrics.go with `meter.Int64Counter()`
- Per-type metrics use `attribute.String("type", step.name)` attribute
- FetchType[T] is a package-level generic function wrapping FetchAll

### Integration Points
- WorkerConfig needs SyncMode field
- Config needs SyncMode field and PDBPLUS_SYNC_MODE parsing
- Worker.Sync() needs mode-aware logic (incremental path vs full path)
- POST /sync handler in main.go needs ?mode= query param parsing
- FetchAll URL construction at line 68: add &since= when provided

</code_context>

<specifics>
## Specific Ideas

- PeeringDB API documentation states: cached responses include `meta.generated` (epoch float). Requests with `?depth=2` serve cached; `?id__in=` serves live. Depth=0 with pagination is unspecified — verify empirically.
- Use beta.peeringdb.com for any live API verification, not production api.peeringdb.com.

</specifics>

<deferred>
## Deferred Ideas

- Thorough verification of meta.generated behavior across all response types deferred to Phase 9 conformance tool.

</deferred>
