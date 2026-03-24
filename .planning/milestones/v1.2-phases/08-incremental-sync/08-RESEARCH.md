# Phase 8: Incremental Sync - Research

**Researched:** 2026-03-23
**Domain:** PeeringDB API incremental sync, Go functional options, SQLite cursor tracking
**Confidence:** HIGH

## Summary

Phase 8 adds a configurable incremental sync mode to the existing full-sync pipeline. The changes are well-scoped: extend `FetchAll` with a `?since=` parameter using functional options, add a `sync_cursors` SQLite table for per-type timestamp tracking, and branch the sync worker's per-type loop between incremental (upsert-only) and full (upsert + delete stale) paths. On incremental failure for any type, immediate fallback to full sync for that type.

The PeeringDB API supports `?since=<unix_epoch>` to return only objects modified after that timestamp. The official peeringdb-py client uses this same mechanism for incremental sync, tracking the last-known update timestamp per resource type and passing `since + 1` to avoid re-processing boundary objects. Deleted objects can be retrieved via `?status=deleted&since=<epoch>` but the CONTEXT.md decision explicitly accepts that hard deletes are not caught by incremental mode -- users switch to full mode when needed.

**Primary recommendation:** Implement as three distinct layers: (1) client-level `WithSince` functional option on `FetchAll`, (2) database-level `sync_cursors` table for per-type cursor persistence, (3) worker-level mode-aware orchestration with per-type fallback logic. All changes are additive to existing code with no regressions to full sync behavior.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- New env var: `PDBPLUS_SYNC_MODE=full|incremental` (default `full`)
- POST /sync accepts `?mode=full|incremental` query param to override config per-request
- First sync always performs full fetch regardless of mode (no ?since= on empty database)
- FetchAll gains functional options pattern: `WithSince(time.Time)`
- FetchAll returns `FetchResult{Data []json.RawMessage, Meta *FetchMeta}` struct instead of bare slice
- FetchMeta includes the earliest `generated` epoch from PeeringDB's `meta` across all pages
- Parse `meta.generated` (epoch float) from each response page to determine cache timestamp
- Use PeeringDB's `meta.generated` epoch if present -- this is when PeeringDB's cache was snapshotted
- If `generated` not present (live response), fall back to `started_at - 5min` buffer
- Use earliest (oldest) `generated` across all pages for a type (most conservative)
- Quick verification in this phase: check whether depth=0 responses include `meta.generated`
- New `sync_cursors` table: `type TEXT PRIMARY KEY, last_sync_at DATETIME, last_status TEXT`
- Created by extending existing `InitStatusTable` function (same function, both tables)
- sync_status table unchanged -- continues for overall sync history
- Per-type cursors updated only on successful sync commit
- Single transaction (same atomicity as full sync)
- Incremental: upsert changed objects only, skip stale deletion step
- Full sync: existing behavior unchanged (upsert + delete stale)
- Hard deletes not caught by incremental (accepted gap -- user switches to full mode when needed)
- On incremental failure for a type -> immediate full fallback for that type (no incremental retry first)
- Observability: `pdbplus.sync.type.fallback` counter + WARN log + OTel span event

### Claude's Discretion
None specified.

### Deferred Ideas (OUT OF SCOPE)
- Thorough verification of meta.generated behavior across all response types deferred to Phase 9 conformance tool.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SYNC-01 | Configurable sync mode via `PDBPLUS_SYNC_MODE` env var (`full` or `incremental`, default `full`) | Config pattern established in config.go; add SyncMode string field with parseEnum helper |
| SYNC-02 | Optional `?since=` parameter on FetchAll for delta fetches | PeeringDB API supports `?since=<unix_epoch>`; functional options pattern for FetchAll; FetchResult return type |
| SYNC-03 | Per-type last-sync timestamp tracking in extended sync_status table | New sync_cursors table via InitStatusTable; cursor CRUD functions in status.go |
| SYNC-04 | Incremental sync fetches only objects modified since last successful sync per type | Worker mode-aware orchestration; per-type cursor lookup before fetch; skip deleteStale on incremental |
| SYNC-05 | On incremental failure for a type, immediately falls back to full sync for that type | Per-type error handling in sync loop; retry with full fetch on failure; `pdbplus.sync.type.fallback` counter |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib | 1.26 | Language runtime | Project constraint |
| encoding/json | 1.26 | Parse meta.generated from PeeringDB responses | Already used for response parsing |
| database/sql | 1.26 | sync_cursors table DDL and CRUD | Already used for sync_status table |
| time | 1.26 | Unix timestamp conversion for since parameter | Standard time handling |

### Supporting
No new dependencies required. All changes use existing stdlib and project packages.

## Architecture Patterns

### Recommended Project Structure
```
internal/
  config/
    config.go           # Add SyncMode field + parsing
    config_test.go      # Add SyncMode test cases
  peeringdb/
    client.go           # Add FetchOption, FetchResult, FetchMeta, WithSince
    client_test.go      # Add tests for WithSince, FetchResult, meta parsing
  sync/
    status.go           # Extend InitStatusTable, add cursor CRUD
    status_test.go      # (new) Test cursor table operations
    worker.go           # Add mode-aware Sync, SyncMode type, per-type fallback
    worker_test.go      # Add incremental sync tests, fallback tests
    integration_test.go # Add incremental sync integration tests
  otel/
    metrics.go          # Add SyncTypeFallback counter
cmd/
  peeringdb-plus/
    main.go             # Wire SyncMode config, pass ?mode= to sync handler
```

### Pattern 1: Functional Options for FetchAll
**What:** Extend FetchAll to accept variadic options that modify fetch behavior without changing existing callers.
**When to use:** When adding optional parameters to an existing function without breaking backward compatibility.
**Example:**
```go
// FetchOption configures optional FetchAll behavior.
type FetchOption func(*fetchConfig)

type fetchConfig struct {
    since time.Time
}

// WithSince sets the ?since= parameter for delta fetches.
func WithSince(t time.Time) FetchOption {
    return func(c *fetchConfig) {
        c.since = t
    }
}

// FetchMeta contains metadata from PeeringDB API responses.
type FetchMeta struct {
    // Generated is the earliest meta.generated epoch across all pages.
    // Zero value means no generated timestamp was found.
    Generated time.Time
}

// FetchResult contains the fetched data and response metadata.
type FetchResult struct {
    Data []json.RawMessage
    Meta FetchMeta
}

// FetchAll pages through all objects. Accepts optional FetchOptions.
func (c *Client) FetchAll(ctx context.Context, objectType string, opts ...FetchOption) (FetchResult, error) {
    var cfg fetchConfig
    for _, opt := range opts {
        opt(&cfg)
    }

    // Build URL with optional since parameter
    url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0", c.baseURL, objectType, pageSize, skip)
    if !cfg.since.IsZero() {
        url += fmt.Sprintf("&since=%d", cfg.since.Unix())
    }
    // ...
}
```

### Pattern 2: Sync Mode Type
**What:** String-typed enum for sync mode with validation.
**When to use:** For configuration values with a fixed set of valid options.
**Example:**
```go
// SyncMode controls how the sync worker fetches data.
type SyncMode string

const (
    SyncModeFull        SyncMode = "full"
    SyncModeIncremental SyncMode = "incremental"
)
```

### Pattern 3: Per-Type Fallback in Sync Loop
**What:** Each type in the sync loop gets its own try-incremental-then-fallback-to-full logic.
**When to use:** When incremental fetch fails for a specific type but others may succeed.
**Example:**
```go
for _, step := range w.syncSteps() {
    cursor := w.getCursor(ctx, step.name)
    if mode == SyncModeIncremental && !cursor.IsZero() {
        count, err := step.incrementalFn(ctx, tx, cursor)
        if err != nil {
            // Log fallback, record metric
            pdbotel.SyncTypeFallback.Add(ctx, 1, typeAttr)
            w.logger.Warn("incremental failed, falling back to full",
                slog.String("type", step.name), slog.String("error", err.Error()))
            // Full fallback
            count, deleted, err = step.fn(ctx, tx)
            if err != nil {
                // True failure -- rollback
                return err
            }
        }
    } else {
        count, deleted, err = step.fn(ctx, tx)
    }
    // Record new cursor from FetchResult.Meta.Generated
}
```

### Anti-Patterns to Avoid
- **Separate code paths for full vs incremental:** The incremental path should reuse the same upsert functions. Only the fetch (with/without since) and the stale-deletion step differ.
- **Global sync cursor:** Using a single timestamp for all 13 types would mean one failed type resets the cursor for all. Per-type cursors are essential.
- **Storing cursors before commit:** Cursors must only be updated after the transaction commits successfully. If the transaction rolls back, the cursors must remain at their previous values.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Functional options | Custom builder pattern | Standard Go functional options | Idiomatic, zero allocation for zero-options case |
| Epoch parsing | Custom float parser | `time.Unix(int64(epoch), 0)` | Standard library handles all edge cases |
| Config enum validation | String comparison everywhere | Validate once in `config.Load()`, use typed constant | Fail fast per CFG-1, type safety |

## Common Pitfalls

### Pitfall 1: FetchAll Return Type Change Breaks All Callers
**What goes wrong:** Changing `FetchAll` from returning `([]json.RawMessage, error)` to `(FetchResult, error)` breaks all 13 callers in `FetchType` and the per-type sync methods.
**Why it happens:** FetchAll is called directly and through the generic `FetchType[T]` wrapper.
**How to avoid:** Update `FetchType[T]` to call `FetchAll` and extract `.Data` from the result. The per-type sync methods call `FetchType`, not `FetchAll` directly, so they are unaffected by the return type change. Only `FetchType[T]` needs updating.
**Warning signs:** Compile errors in 13+ locations after changing FetchAll signature.

### Pitfall 2: Since Timestamp Off-by-One
**What goes wrong:** Using the exact `meta.generated` timestamp as `since` value re-fetches objects that were already synced at that exact second.
**Why it happens:** PeeringDB's `since` filter is `updated > since`, so using the exact timestamp should not produce duplicates. But the peeringdb-py client adds +1 as a safety measure.
**How to avoid:** Store the `meta.generated` epoch as-is. The upsert operations are idempotent, so re-fetching a few objects is harmless. Do NOT add +1 to the stored cursor -- this could miss objects updated in the same second.
**Warning signs:** Objects appearing to be re-fetched every incremental cycle.

### Pitfall 3: Cursor Update Before Transaction Commit
**What goes wrong:** Updating the sync_cursors table inside the ent transaction, then the transaction rolls back, but the cursor was updated via raw SQL outside the transaction.
**Why it happens:** sync_cursors uses raw SQL (not ent-managed), so it's on a separate connection.
**How to avoid:** Update cursors AFTER the ent transaction commits successfully, not inside the transaction. The cursor update is a separate raw SQL operation.
**Warning signs:** Cursor advances but data doesn't change (inconsistent state).

### Pitfall 4: First Sync Detection
**What goes wrong:** Incremental mode tries to use `?since=0` or `?since=1970-01-01` when no cursor exists.
**Why it happens:** Zero-value time.Time converts to Unix epoch 0.
**How to avoid:** Check if cursor is zero/empty before applying `WithSince`. If no cursor exists for a type, always do full fetch regardless of mode.
**Warning signs:** Fetching all objects with `?since=0` (equivalent to full fetch anyway but semantically wrong).

### Pitfall 5: meta.generated Not Present in All Responses
**What goes wrong:** Assuming `meta.generated` always exists and parsing fails.
**Why it happens:** PeeringDB documentation says cached (depth=2) responses include `meta.generated`, but depth=0 with pagination is unspecified. Live responses (with `?id__in=`) do not include it.
**How to avoid:** Treat `meta.generated` as optional. If absent, fall back to `started_at - 5min` buffer. The CONTEXT.md decision specifies this exact fallback. Phase 9 will verify behavior across all types.
**Warning signs:** Nil pointer dereference or missing cursor after sync.

### Pitfall 6: Mode Override on POST /sync Not Propagated
**What goes wrong:** The `?mode=` query parameter on POST /sync is parsed but never reaches the worker.
**Why it happens:** Current POST /sync handler calls `syncWorker.SyncWithRetry(ctx)` with no mode parameter.
**How to avoid:** Thread the mode through to the worker. Either add a parameter to Sync/SyncWithRetry, or use a separate method like `SyncWithMode(ctx, mode)`.
**Warning signs:** POST /sync?mode=incremental still performs full sync.

## Code Examples

### Parsing meta.generated from PeeringDB Response
```go
// parseMeta extracts the generated epoch from a PeeringDB API response meta field.
// Returns zero time if meta is empty or generated field is absent.
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

### sync_cursors Table DDL
```sql
CREATE TABLE IF NOT EXISTS sync_cursors (
    type TEXT PRIMARY KEY,
    last_sync_at DATETIME NOT NULL,
    last_status TEXT NOT NULL DEFAULT 'success'
)
```

### Cursor CRUD Operations
```go
// GetCursor returns the last successful sync timestamp for a type.
// Returns zero time if no cursor exists.
func GetCursor(ctx context.Context, db *sql.DB, objType string) (time.Time, error) {
    var lastSyncAt time.Time
    err := db.QueryRowContext(ctx,
        `SELECT last_sync_at FROM sync_cursors WHERE type = ? AND last_status = 'success'`,
        objType,
    ).Scan(&lastSyncAt)
    if err == sql.ErrNoRows {
        return time.Time{}, nil
    }
    if err != nil {
        return time.Time{}, fmt.Errorf("get cursor for %s: %w", err, objType)
    }
    return lastSyncAt, nil
}

// UpsertCursor updates or inserts the sync cursor for a type.
func UpsertCursor(ctx context.Context, db *sql.DB, objType string, lastSyncAt time.Time, status string) error {
    _, err := db.ExecContext(ctx,
        `INSERT INTO sync_cursors (type, last_sync_at, last_status)
         VALUES (?, ?, ?)
         ON CONFLICT(type) DO UPDATE SET last_sync_at = excluded.last_sync_at, last_status = excluded.last_status`,
        objType, lastSyncAt, status,
    )
    if err != nil {
        return fmt.Errorf("upsert cursor for %s: %w", objType, err)
    }
    return nil
}
```

### Config SyncMode Parsing
```go
// SyncMode controls the sync strategy.
type SyncMode string

const (
    SyncModeFull        SyncMode = "full"
    SyncModeIncremental SyncMode = "incremental"
)

// parseSyncMode validates and returns a SyncMode from a string.
func parseSyncMode(key string, defaultVal SyncMode) (SyncMode, error) {
    v := os.Getenv(key)
    if v == "" {
        return defaultVal, nil
    }
    switch SyncMode(v) {
    case SyncModeFull, SyncModeIncremental:
        return SyncMode(v), nil
    default:
        return "", fmt.Errorf("invalid sync mode %q for %s: must be 'full' or 'incremental'", v, key)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Full re-fetch only | Configurable full/incremental | Phase 8 (this phase) | Reduces API load from ~60 pages/hour to ~1-2 pages/hour for most types |
| FetchAll returns bare slice | FetchAll returns FetchResult with metadata | Phase 8 (this phase) | Enables cursor tracking from response metadata |
| Single sync_status table | sync_status + sync_cursors tables | Phase 8 (this phase) | Per-type cursor tracking for incremental sync |

## Open Questions

1. **Does depth=0 with pagination include meta.generated?**
   - What we know: PeeringDB docs say cached responses (depth=2) include `meta.generated`. The CONTEXT.md notes depth=0 with pagination is "unspecified."
   - What's unclear: Whether the server returns `meta.generated` for depth=0 paginated list endpoints.
   - Recommendation: The CONTEXT.md decision says to do a quick verification in this phase using beta.peeringdb.com. If absent, the fallback (`started_at - 5min`) is already designed. Phase 9 conformance tool will do thorough verification.

2. **PeeringDB since filter semantics: `>=` or `>`?**
   - What we know: PeeringDB-py adds +1 to the since value, suggesting the filter is inclusive (`>=`). PeeringDB docs say "objects updated since then" which is ambiguous.
   - What's unclear: Exact server-side comparison operator.
   - Recommendation: Upserts are idempotent, so re-fetching boundary objects is harmless. Store the exact generated timestamp. If duplicates appear, they'll be upserted with no effect.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | .golangci.yml (golangci-lint v2) |
| Quick run command | `go test ./internal/sync/... ./internal/peeringdb/... ./internal/config/... -count=1 -race` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SYNC-01 | SyncMode config parsing from env var | unit | `go test ./internal/config/... -run TestLoad_SyncMode -count=1` | No -- Wave 0 |
| SYNC-01 | POST /sync ?mode= override | unit | `go test ./cmd/peeringdb-plus/... -run TestSyncModeOverride -count=1` | No -- Wave 0 |
| SYNC-02 | FetchAll with WithSince adds ?since= param | unit | `go test ./internal/peeringdb/... -run TestFetchAllWithSince -count=1` | No -- Wave 0 |
| SYNC-02 | FetchResult includes parsed meta.generated | unit | `go test ./internal/peeringdb/... -run TestFetchMeta -count=1` | No -- Wave 0 |
| SYNC-03 | sync_cursors table creation and CRUD | unit | `go test ./internal/sync/... -run TestCursor -count=1` | No -- Wave 0 |
| SYNC-04 | Incremental sync fetches with since, skips delete | integration | `go test ./internal/sync/... -run TestIncrementalSync -count=1` | No -- Wave 0 |
| SYNC-04 | First sync always full regardless of mode | integration | `go test ./internal/sync/... -run TestFirstSyncAlwaysFull -count=1` | No -- Wave 0 |
| SYNC-05 | Fallback to full on incremental failure | integration | `go test ./internal/sync/... -run TestIncrementalFallback -count=1` | No -- Wave 0 |
| SYNC-05 | Fallback counter metric recorded | unit | `go test ./internal/sync/... -run TestFallbackMetric -count=1` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/sync/... ./internal/peeringdb/... ./internal/config/... -count=1 -race`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/config/config_test.go` -- add SyncMode test cases (extend existing file)
- [ ] `internal/peeringdb/client_test.go` -- add WithSince and FetchResult tests (extend existing file)
- [ ] `internal/sync/status_test.go` -- new file for cursor CRUD tests
- [ ] `internal/sync/worker_test.go` -- add incremental/fallback tests (extend existing file)
- [ ] `internal/sync/integration_test.go` -- add incremental integration tests (extend existing file)

## Sources

### Primary (HIGH confidence)
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) - `?since=` parameter documentation
- [PeeringDB GitHub Issue #776](https://github.com/peeringdb/peeringdb/issues/776) - api-cache race condition, confirms meta.generated exists in cached responses
- [peeringdb-py _update.py](https://github.com/peeringdb/peeringdb-py) - Reference implementation of incremental sync with since+1 pattern
- [peeringdb-py fetch.py](https://github.com/peeringdb/peeringdb-py) - API request construction with since parameter
- Codebase: `internal/peeringdb/client.go` - Current FetchAll implementation
- Codebase: `internal/sync/worker.go` - Current sync worker with 13-type loop
- Codebase: `internal/sync/status.go` - Current sync_status table DDL and CRUD
- Codebase: `internal/config/config.go` - Current config pattern with env parsing

### Secondary (MEDIUM confidence)
- [PeeringDB API Cache Improvements #1065](https://github.com/peeringdb/peeringdb/issues/1065) - Cache generation takes 30+ minutes, confirms separate cache process
- [PeeringDB Faster Queries Blog](https://docs.peeringdb.com/blog/faster_queries/) - Recommends hourly sync, confirms since-based incremental approach

### Tertiary (LOW confidence)
- meta.generated field behavior for depth=0 paginated responses -- unverified, quick check needed during implementation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - All stdlib, no new dependencies
- Architecture: HIGH - Extends well-established patterns already in the codebase (functional options, raw SQL tables, per-type sync loop)
- Pitfalls: HIGH - Verified against peeringdb-py reference implementation and PeeringDB server issues
- meta.generated behavior: LOW - Documented for depth=2 cached responses only; depth=0 paginated behavior unverified

**Research date:** 2026-03-23
**Valid until:** 2026-04-23 (30 days -- stable domain, PeeringDB API changes rarely)
