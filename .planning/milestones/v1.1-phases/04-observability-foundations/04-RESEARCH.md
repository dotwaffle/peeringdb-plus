# Phase 4: Observability Foundations - Research

**Researched:** 2026-03-22
**Domain:** OpenTelemetry instrumentation -- HTTP client tracing, sync metrics wiring, per-type metric instruments
**Confidence:** HIGH

## Summary

Phase 4 completes the OTel instrumentation left incomplete in v1.0. Three concrete gaps exist: (1) the PeeringDB HTTP client produces no trace spans for outbound API calls, (2) the `SyncDuration` and `SyncOperations` metric instruments are registered in `internal/otel/metrics.go` but never recorded by the sync worker, and (3) there are no per-type metrics to observe how individual PeeringDB object types perform during sync. All required OTel packages are already in `go.mod` -- no new dependencies are needed.

The MeterProvider is already initialized in `provider.go` (verified by reading the code and the existing `TestSetup_SetsGlobalMeterProvider` test). The research summary incorrectly flagged this as missing, but the actual codebase shows `autoexport.NewMetricReader` and `sdkmetric.NewMeterProvider` already configured and registered globally via `otel.SetMeterProvider(mp)`. The real gap is purely that the sync worker never calls `.Record()` or `.Add()` on the instruments that `InitMetrics()` registers.

The implementation touches exactly three files for production code (`internal/peeringdb/client.go`, `internal/otel/metrics.go`, `internal/sync/worker.go`) and their corresponding test files. The otelhttp transport wrapping is a one-line change to `NewClient()`. The manual span hierarchy in `FetchAll()` and `doWithRetry()` requires careful context propagation to produce the desired trace tree: `full-sync` > `sync-{type}` > `peeringdb.fetch/{type}` > per-attempt spans > HTTP transport spans. The per-type metric instruments follow the same `InitMetrics()` registration pattern already established.

**Primary recommendation:** Wire otelhttp.NewTransport on the HTTP client, add manual parent spans in FetchAll/doWithRetry for business-level trace hierarchy, then wire .Record()/.Add() calls in the sync worker for both existing and new per-type instruments.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- D-01: Use both otelhttp.NewTransport (automatic HTTP semantic conventions) AND manual parent spans in FetchAll for business-level hierarchy
- D-02: FetchAll parent spans named `peeringdb.fetch/{type}` (e.g. `peeringdb.fetch/net`) -- type in span name, follows OTel RPC naming convention
- D-03: Each doWithRetry attempt gets its own explicit child span with attempt number attribute -- makes retry behavior visible in traces
- D-04: Page-level data recorded as span events on the FetchAll span (page number, count, running total per page)
- D-05: Rate limiter wait time recorded as a span event (`rate_limiter.wait`) with wait duration on each request span
- D-06: MeterProvider already initialized in provider.go -- no changes needed. The fix is wiring `.Record()` and `.Add()` calls in worker.go for the existing SyncDuration and SyncOperations instruments
- D-07: Add 4 new per-type metric instruments: duration histogram, object count, delete count, and separate fetch_errors and upsert_errors counters
- D-08: Flat naming convention with type attribute: `pdbplus.sync.type.duration`, `pdbplus.sync.type.objects`, `pdbplus.sync.type.deleted`, `pdbplus.sync.type.fetch_errors`, `pdbplus.sync.type.upsert_errors` -- all with `type` attribute (net, ix, fac, etc.)
- D-09: Add sync freshness gauge using OTel observable gauge with callback -- only computes seconds-since-last-sync when metric backend scrapes. No background goroutine.
- D-10: Fetch errors (PeeringDB HTTP client failures) and upsert errors (ent database failures) tracked as separate counters, not combined
- D-11: OTel-related tech debt only. Do NOT clean up vestigial DataLoader middleware, config.IsPrimary field, or unused globalid.go exports -- those are separate work items.

### Claude's Discretion
No specific items listed -- all decisions are locked.

### Deferred Ideas (OUT OF SCOPE)
- Clean up vestigial DataLoader middleware -- separate tech debt task
- Remove config.IsPrimary field -- separate tech debt task
- Remove unused globalid.go exports -- separate tech debt task
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OBS-01 | OTel MeterProvider is initialized alongside existing TracerProvider | Already done -- `provider.go` lines 62-70 create MeterProvider via autoexport. Test `TestSetup_SetsGlobalMeterProvider` confirms. Just verify, no code changes needed. |
| OBS-02 | PeeringDB HTTP client calls produce OTel trace spans with semantic conventions | otelhttp.NewTransport wraps the HTTP client transport for automatic HTTP-level spans. Manual parent spans in FetchAll (`peeringdb.fetch/{type}`) and doWithRetry (per-attempt with `http.request.resend_count`) provide business-level hierarchy. |
| OBS-03 | Sync worker records values for all registered sync metrics (duration, operations) | Existing `SyncDuration` (Float64Histogram) and `SyncOperations` (Int64Counter) in metrics.go just need `.Record()` and `.Add()` calls in worker.go at sync completion points. |
| OBS-04 | Per-type sync metrics track duration, object count, and delete count for each of the 13 PeeringDB types | 5 new instruments registered in metrics.go: type duration histogram, type objects counter, type deleted counter, type fetch_errors counter, type upsert_errors counter. Plus freshness observable gauge. Recorded per step in the sync loop. |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| go.opentelemetry.io/otel | v1.42.0 | OTel API (Tracer, Meter) | Already in go.mod. Standard Go OTel API. |
| go.opentelemetry.io/otel/metric | v1.42.0 | Metric instrument types | Already in go.mod. Float64Histogram, Int64Counter, Float64ObservableGauge. |
| go.opentelemetry.io/otel/trace | v1.42.0 | Trace span creation | Already in go.mod. Manual span creation for business-level hierarchy. |
| go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | v0.67.0 | HTTP client transport wrapping | Already in go.mod (used for server middleware). Same package provides NewTransport for client-side instrumentation. |
| go.opentelemetry.io/otel/attribute | (transitive) | Span and metric attributes | Already available transitively. Used for type, status, attempt attributes. |

### Supporting (Test Only)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| go.opentelemetry.io/otel/sdk/trace/tracetest | v1.42.0 | In-memory span recorder for tests | Verify span creation, hierarchy, attributes in client_test.go |
| go.opentelemetry.io/otel/sdk/metric/metricdata | v1.42.0 | In-memory metric reader for tests | Verify metric recording values in worker_test.go and metrics_test.go |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| otelhttp.NewTransport | Manual spans in doWithRetry | Would miss automatic HTTP semantic conventions (method, status, url, content_length). otelhttp is the canonical approach. |
| Package-level metric vars | Dependency injection into Worker | Cleaner architecture but changes Worker constructor signature, which is out of scope per D-11. Current pattern (package-level vars set by InitMetrics) works for a single-binary application. |
| Observable gauge for freshness | Background goroutine updating a regular gauge | Observable gauge is semantically correct (value computed on demand), avoids goroutine lifecycle management, and only runs when the metrics backend scrapes. |

**Installation:**
```bash
# No new dependencies needed -- all packages are already in go.mod
```

## Architecture Patterns

### Files Changed (Production)
```
internal/
  peeringdb/
    client.go          # otelhttp.NewTransport + manual spans in FetchAll/doWithRetry
  otel/
    metrics.go         # 5 new per-type instruments + freshness gauge + InitMetrics expansion
  sync/
    worker.go          # .Record()/.Add() calls for all metrics at sync/step completion
```

### Files Changed (Tests)
```
internal/
  peeringdb/
    client_test.go     # Verify span hierarchy with tracetest.SpanRecorder
  otel/
    metrics_test.go    # Verify new instrument registration and recording
  sync/
    worker_test.go     # Verify metric values recorded during sync cycle
```

### Pattern 1: otelhttp Transport Wrapping
**What:** Wrap the HTTP client's transport with `otelhttp.NewTransport` to automatically create trace spans for every outbound HTTP request.
**When to use:** Any Go HTTP client making outbound requests that should be traced.
**Example:**
```go
// Source: https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
func NewClient(baseURL string, logger *slog.Logger) *Client {
    return &Client{
        http: &http.Client{
            Timeout:   30 * time.Second,
            Transport: otelhttp.NewTransport(http.DefaultTransport),
        },
        // ... rest unchanged
    }
}
```
**Automatic span attributes:** `http.request.method`, `server.address`, `server.port`, `url.full`, `http.response.status_code`, `http.request.content_length`, `http.response.content_length`.

### Pattern 2: Manual Parent Span with Context Propagation
**What:** Create explicit parent spans for business-level operations, letting otelhttp transport spans nest underneath via context propagation.
**When to use:** When you need span hierarchy that represents business logic (fetch-by-type, retry attempts) above the HTTP transport layer.
**Example:**
```go
// Source: https://opentelemetry.io/docs/languages/go/instrumentation/#traces
func (c *Client) FetchAll(ctx context.Context, objectType string) ([]json.RawMessage, error) {
    tracer := otel.Tracer("peeringdb")
    ctx, span := tracer.Start(ctx, "peeringdb.fetch/"+objectType)
    defer span.End()

    var all []json.RawMessage
    for skip := 0; ; skip += pageSize {
        page := skip / pageSize
        // ... existing pagination logic ...

        // Add page data as span event (D-04)
        span.AddEvent("page.fetched",
            trace.WithAttributes(
                attribute.Int("page", page),
                attribute.Int("count", len(apiResp.Data)),
                attribute.Int("running_total", len(all)),
            ),
        )
    }
    return all, nil
}
```

### Pattern 3: Per-Attempt Retry Spans with Resend Count
**What:** Each retry attempt gets its own child span with the attempt number as an attribute per OTel HTTP semantic conventions.
**When to use:** HTTP clients with retry logic where individual attempt visibility is needed in traces.
**Example:**
```go
// Source: https://opentelemetry.io/docs/specs/semconv/http/http-spans/
func (c *Client) doWithRetry(ctx context.Context, url string) (*http.Response, error) {
    tracer := otel.Tracer("peeringdb")
    var lastErr error

    for attempt := range maxRetries {
        ctx, attemptSpan := tracer.Start(ctx, "peeringdb.request",
            trace.WithAttributes(
                attribute.Int("http.request.resend_count", attempt),
            ),
        )

        // Rate limiter wait event (D-05)
        waitStart := time.Now()
        if err := c.limiter.Wait(ctx); err != nil {
            attemptSpan.End()
            return nil, fmt.Errorf("rate limiter for %s: %w", url, err)
        }
        waitDuration := time.Since(waitStart)
        if waitDuration > time.Millisecond {
            attemptSpan.AddEvent("rate_limiter.wait",
                trace.WithAttributes(
                    attribute.Float64("wait_duration_ms", float64(waitDuration.Milliseconds())),
                ),
            )
        }

        // ... existing request logic ...
        // otelhttp.NewTransport creates a child span under attemptSpan automatically
        attemptSpan.End()
    }
    return nil, lastErr
}
```

### Pattern 4: Observable Gauge with Callback for Freshness
**What:** An OTel observable gauge that computes sync freshness (seconds since last successful sync) only when the metrics backend scrapes.
**When to use:** When you need a gauge that derives its value from existing state without a background goroutine.
**Example:**
```go
// Source: https://pkg.go.dev/go.opentelemetry.io/otel/metric
meter := otel.Meter("peeringdb-plus")
_, err := meter.Float64ObservableGauge("pdbplus.sync.freshness",
    metric.WithDescription("Seconds since last successful sync"),
    metric.WithUnit("s"),
    metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
        status, err := sync.GetLastSyncStatus(context.Background(), db)
        if err != nil || status == nil || status.Status != "success" {
            return nil // No observation if no successful sync
        }
        o.Observe(time.Since(status.LastSyncAt).Seconds())
        return nil
    }),
)
```

### Pattern 5: Metric Recording in Sync Loop
**What:** Record per-type metrics after each sync step completes, and sync-level metrics after the full sync completes or fails.
**When to use:** Any loop-based processing where per-iteration metrics are needed.
**Example:**
```go
// Per-step recording inside the sync loop
for _, step := range w.syncSteps() {
    stepStart := time.Now()
    // ... existing span creation ...
    count, deleted, err := step.fn(ctx, tx)
    stepSpan.End()

    typeAttr := metric.WithAttributes(attribute.String("type", step.name))

    if err != nil {
        pdbotel.SyncTypeUpsertErrors.Add(ctx, 1, typeAttr)
        // ... existing rollback logic ...
    }

    // Record per-type metrics (D-07, D-08)
    pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)
    pdbotel.SyncTypeObjects.Add(ctx, int64(count), typeAttr)
    pdbotel.SyncTypeDeleted.Add(ctx, int64(deleted), typeAttr)
}

// After commit or failure -- record sync-level metrics (D-06)
pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(),
    metric.WithAttributes(attribute.String("status", "success")))
pdbotel.SyncOperations.Add(ctx, 1,
    metric.WithAttributes(attribute.String("status", "success")))
```

### Desired Trace Tree
```
full-sync                                    [sync worker, existing]
  sync-org                                   [sync worker, existing]
    peeringdb.fetch/org                      [FetchAll, new - D-02]
      peeringdb.request (attempt=0)          [doWithRetry, new - D-03]
        rate_limiter.wait [event]            [new - D-05]
        HTTP GET (auto by otelhttp)          [otelhttp.NewTransport, new - D-01]
      page.fetched [event, page=0, count=250] [new - D-04]
      peeringdb.request (attempt=0)          [doWithRetry, page 2]
        HTTP GET (auto)
      page.fetched [event, page=1, count=100]
  sync-fac
    peeringdb.fetch/fac
      ...
```

### Anti-Patterns to Avoid
- **Moving retry logic into RoundTripper:** Violates `http.RoundTripper` contract. Rate limiting and retry belong in `doWithRetry()`, not the transport layer.
- **Creating instruments inside Sync():** Re-registers instruments on every sync cycle. Create all instruments once in `InitMetrics()` at startup.
- **Using a background goroutine for freshness gauge:** Observable gauge callbacks are the correct OTel pattern. No goroutine lifecycle management needed.
- **Wrapping the FetchAll span around the entire pagination loop including retries:** Would hide per-page timing. FetchAll span should be the parent, with per-page attempt spans as children.
- **Combining fetch and upsert errors into one counter:** D-10 requires separate counters. Fetch errors (HTTP client) and upsert errors (ent database) have different causes and remediation.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTTP client trace spans | Manual span creation in doWithRetry for HTTP-level attributes | `otelhttp.NewTransport(http.DefaultTransport)` | Automatic HTTP semantic convention compliance, response body lifecycle handling, context propagation |
| Metric instrument types | Custom metric collection structs | `metric.Float64Histogram`, `metric.Int64Counter`, `metric.Float64ObservableGauge` | OTel SDK handles export, aggregation, and backend compatibility |
| In-memory span/metric collection for tests | Custom test recorders | `tracetest.NewInMemoryExporter` + `sdkmetric.NewManualReader` | Officially supported test utilities that match OTel SDK internals |

**Key insight:** The OTel Go SDK provides every building block needed. The work is wiring existing APIs into existing code, not building new abstractions.

## Common Pitfalls

### Pitfall 1: Flat Trace Tree from otelhttp Transport Spans
**What goes wrong:** Without manual parent spans, otelhttp creates one span per HTTP round-trip as flat siblings under the sync-type span. Retries produce multiple sibling spans with no grouping by logical fetch operation.
**Why it happens:** `otelhttp.NewTransport` operates at the `http.RoundTripper` level, below the retry loop. Each HTTP request gets its own span from the transport.
**How to avoid:** Create explicit parent spans in `FetchAll()` (`peeringdb.fetch/{type}`) and per-attempt spans in `doWithRetry()`. Pass the enriched context through `http.NewRequestWithContext()` so transport spans nest correctly.
**Warning signs:** Trace visualization shows dozens of flat HTTP client spans under each sync-type span. Missing parent-child relationships.

### Pitfall 2: No-Op Metric Recording Despite Code Being Present
**What goes wrong:** `.Record()` and `.Add()` calls are added but no values appear in the metrics backend.
**Why it happens:** If `InitMetrics()` is called before `Setup()` in main.go, the instruments are created against the no-op global MeterProvider. The current code is correct (Setup before InitMetrics), but reordering would silently break all metrics.
**How to avoid:** Verify the call order in `main.go`: `pdbotel.Setup()` then `pdbotel.InitMetrics()`. This is already correct (lines 54-72 of main.go). Write a test that verifies metrics produce non-zero values using an in-memory reader.
**Warning signs:** All metric instruments are non-nil (they never are nil with OTel), but exported data shows zero values. Tests pass because they check for no-panic, not actual values.

### Pitfall 3: Context Scoping for Attempt Spans in doWithRetry
**What goes wrong:** If the attempt span's context shadows the outer context variable, subsequent retry iterations use a context with an ended span as parent.
**Why it happens:** Go's `for` loop reuses the variable name. `ctx, span := tracer.Start(ctx, ...)` inside the loop makes each attempt span a child of the previous (ended) attempt span instead of all being children of the FetchAll span.
**How to avoid:** Use a different variable name for the attempt context: `attemptCtx, attemptSpan := tracer.Start(ctx, ...)`. Pass `attemptCtx` to `http.NewRequestWithContext()` but keep `ctx` as the FetchAll-level context for the next iteration.
**Warning signs:** Trace tree shows attempt spans chained as grandchildren instead of siblings under the FetchAll span.

### Pitfall 4: Observable Gauge Callback Accessing Database
**What goes wrong:** The freshness gauge callback queries `sync_status` table via `GetLastSyncStatus()`. If the database connection is closed or the query is slow, it blocks the OTel metric collection cycle.
**Why it happens:** OTel callbacks must complete within the collection deadline (context has a timeout). Database queries can be slow under load or if the connection pool is exhausted.
**How to avoid:** The `sync_status` query is trivially fast (single row by ID DESC LIMIT 1). Use `context.Background()` in the callback rather than the callback's context to avoid premature cancellation. Add an early return (observe nothing) if the query fails.
**Warning signs:** Metric collection timeouts in OTel SDK logs. Missing freshness gauge observations.

### Pitfall 5: Distinguishing Fetch Errors from Upsert Errors
**What goes wrong:** The per-type sync step functions (`syncOrganizations`, etc.) combine PeeringDB fetch errors and ent upsert errors into a single returned error. Recording error counters requires distinguishing which phase failed.
**Why it happens:** Each sync step calls `FetchType()` (fetch) then `upsertX()` (upsert) then `deleteStaleX()` (delete). All return errors through the same error path.
**How to avoid:** The sync step functions already have clear separation: fetch happens first, then upsert, then delete. Record `fetch_errors` when the fetch call returns an error, and `upsert_errors` when the upsert/delete calls return an error. However, since the step functions wrap both into a single return, the error counter recording must happen at the point where the error type is known -- which is inside the per-type sync methods, not in the Sync() loop. This means either: (a) recording error metrics inside each sync step function before returning, or (b) modifying the step function return signature to indicate error phase. Option (a) is simpler and aligns with D-11 (minimal changes).
**Warning signs:** All errors counted as the same type. Cannot distinguish HTTP failures from database failures in dashboards.

## Code Examples

### New Metric Instruments in metrics.go
```go
// Source: existing InitMetrics pattern in internal/otel/metrics.go
// Per-type instruments (D-07, D-08)
var (
    SyncTypeDuration    metric.Float64Histogram
    SyncTypeObjects     metric.Int64Counter
    SyncTypeDeleted     metric.Int64Counter
    SyncTypeFetchErrors metric.Int64Counter
    SyncTypeUpsertErrors metric.Int64Counter
)

// Inside InitMetrics():
SyncTypeDuration, err = meter.Float64Histogram("pdbplus.sync.type.duration",
    metric.WithDescription("Duration of sync per PeeringDB object type"),
    metric.WithUnit("s"),
    metric.WithExplicitBucketBoundaries(0.5, 1, 2, 5, 10, 30, 60),
)

SyncTypeObjects, err = meter.Int64Counter("pdbplus.sync.type.objects",
    metric.WithDescription("Number of objects synced per type"),
    metric.WithUnit("{object}"),
)

SyncTypeDeleted, err = meter.Int64Counter("pdbplus.sync.type.deleted",
    metric.WithDescription("Number of objects deleted per type"),
    metric.WithUnit("{object}"),
)

SyncTypeFetchErrors, err = meter.Int64Counter("pdbplus.sync.type.fetch_errors",
    metric.WithDescription("PeeringDB API fetch errors per type"),
    metric.WithUnit("{error}"),
)

SyncTypeUpsertErrors, err = meter.Int64Counter("pdbplus.sync.type.upsert_errors",
    metric.WithDescription("Database upsert errors per type"),
    metric.WithUnit("{error}"),
)
```

### Freshness Observable Gauge Registration
```go
// Source: https://pkg.go.dev/go.opentelemetry.io/otel/metric
// The freshness gauge needs access to the database, so it must be registered
// after database setup. This requires a separate init function or passing db
// to InitMetrics. Since InitMetrics currently takes no args, add a new function.

// InitFreshnessGauge registers the sync freshness observable gauge.
// Must be called after database is opened.
func InitFreshnessGauge(db *sql.DB) error {
    meter := otel.Meter("peeringdb-plus")
    _, err := meter.Float64ObservableGauge("pdbplus.sync.freshness",
        metric.WithDescription("Seconds since last successful sync"),
        metric.WithUnit("s"),
        metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
            status, err := sync.GetLastSyncStatus(context.Background(), db)
            if err != nil || status == nil || status.Status != "success" {
                return nil
            }
            o.Observe(time.Since(status.LastSyncAt).Seconds())
            return nil
        }),
    )
    return err
}
```

### Test Pattern: Verify Spans with tracetest
```go
// Source: https://pkg.go.dev/go.opentelemetry.io/otel/sdk/trace/tracetest
func TestFetchAllCreatesSpanHierarchy(t *testing.T) {
    t.Setenv("OTEL_TRACES_EXPORTER", "none")

    exporter := tracetest.NewInMemoryExporter()
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithSyncer(exporter),
    )
    otel.SetTracerProvider(tp)
    t.Cleanup(func() { tp.Shutdown(context.Background()) })

    // ... create client, mock server, call FetchAll ...

    spans := exporter.GetSpans()
    // Verify: peeringdb.fetch/{type} parent span exists
    // Verify: peeringdb.request child spans with attempt attribute
    // Verify: HTTP GET transport spans nested under attempt spans
}
```

### Test Pattern: Verify Metrics with ManualReader
```go
// Source: https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric
func TestSyncRecordsMetrics(t *testing.T) {
    reader := sdkmetric.NewManualReader()
    mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
    otel.SetMeterProvider(mp)
    t.Cleanup(func() { mp.Shutdown(context.Background()) })

    // Re-init metrics against the test MeterProvider
    if err := pdbotel.InitMetrics(); err != nil {
        t.Fatalf("InitMetrics: %v", err)
    }

    // ... run sync ...

    var rm metricdata.ResourceMetrics
    if err := reader.Collect(context.Background(), &rm); err != nil {
        t.Fatalf("Collect: %v", err)
    }

    // Walk rm.ScopeMetrics to find pdbplus.sync.duration, verify it has a non-zero value
    // Walk to find pdbplus.sync.operations with status=success attribute
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `otel.Float64Gauge` (experimental) | `metric.Float64ObservableGauge` (stable) | OTel Go v1.31.0 (2024) | Float64Gauge API stabilized. Observable gauges are the correct pattern for computed values like freshness. |
| Manual HTTP span creation in every client | `otelhttp.NewTransport` wrapping | OTel contrib v0.40+ (2023) | Transport wrapping is canonical. 2,691+ importers on pkg.go.dev. |
| `metric.WithFloat64Callback` only at creation | `meter.RegisterCallback` for multi-instrument callbacks | OTel Go v1.28+ | RegisterCallback allows post-creation callback registration. For this phase, WithFloat64Callback at creation time is sufficient. |

**Deprecated/outdated:**
- The research SUMMARY.md incorrectly states "MeterProvider not initialized." This was true in an earlier version but `provider.go` now contains full MeterProvider initialization (lines 62-70). The test `TestSetup_SetsGlobalMeterProvider` confirms it. The tech debt is purely in the sync worker not calling `.Record()`.

## Open Questions

1. **Freshness gauge database dependency**
   - What we know: The freshness gauge callback needs `*sql.DB` to query `sync_status`. This means it cannot be registered in `InitMetrics()` (which takes no arguments).
   - What's unclear: Whether to add a `db` parameter to `InitMetrics()`, create a separate `InitFreshnessGauge(db)` function, or register the gauge in `main.go` directly.
   - Recommendation: Create a separate `InitFreshnessGauge(db *sql.DB)` function in `internal/otel/metrics.go` and call it from `main.go` after database setup. This keeps `InitMetrics()` unchanged (no signature changes to existing code) and follows the single-responsibility principle. Alternatively, the freshness gauge can be registered directly in `main.go` since it is the only instrument that needs database access.

2. **Error metric recording location**
   - What we know: D-10 requires separate fetch and upsert error counters. The sync step functions combine both error sources into a single return.
   - What's unclear: Whether to record error metrics inside each per-type sync method (13 methods, repetitive) or restructure the Sync() loop to distinguish error phases.
   - Recommendation: Record error metrics inside the Sync() loop by splitting the error handling. Each step function can be wrapped: call the fetch portion, if error -> record fetch_error + return. Call the upsert portion, if error -> record upsert_error + return. This avoids modifying all 13 sync methods. However, since the current step functions combine fetch+upsert into one function, the cleanest approach is to record the total error at the Sync() loop level and examine the error message to determine the type (fetch errors contain "fetch" in the error message, upsert errors contain "upsert"). Alternatively, have each sync step function record its own error metrics before returning -- more accurate but requires touching all 13 methods.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + go test -race |
| Config file | None needed -- Go test convention |
| Quick run command | `go test ./internal/otel/... ./internal/peeringdb/... ./internal/sync/... -count=1 -race` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OBS-01 | MeterProvider is initialized (verify only) | unit | `go test ./internal/otel/... -run TestSetup_SetsGlobalMeterProvider -count=1` | Exists |
| OBS-02 | HTTP client produces trace spans with hierarchy | unit | `go test ./internal/peeringdb/... -run TestFetchAllCreatesSpan -count=1 -race` | Wave 0 |
| OBS-03 | Sync worker records SyncDuration and SyncOperations | unit | `go test ./internal/sync/... -run TestSyncRecordsMetrics -count=1 -race` | Wave 0 |
| OBS-04 | Per-type metrics recorded for each of 13 types | unit | `go test ./internal/sync/... -run TestSyncRecordsPerTypeMetrics -count=1 -race` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/otel/... ./internal/peeringdb/... ./internal/sync/... -count=1 -race`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/peeringdb/client_test.go` -- add span hierarchy verification tests using `tracetest.NewInMemoryExporter`
- [ ] `internal/sync/worker_test.go` -- add metric value verification tests using `sdkmetric.NewManualReader`
- [ ] `internal/otel/metrics_test.go` -- add tests for new per-type instrument registration and freshness gauge callback

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All code | Yes | 1.26.1 | -- |
| golangci-lint | Linting | Yes | 2.11.3 | -- |
| OTel Go SDK | All instrumentation | Yes | v1.42.0 (in go.mod) | -- |
| otelhttp | HTTP transport wrapping | Yes | v0.67.0 (in go.mod) | -- |

**Missing dependencies with no fallback:** None
**Missing dependencies with fallback:** None

## Sources

### Primary (HIGH confidence)
- `internal/otel/provider.go` -- verified MeterProvider is already initialized (lines 62-70)
- `internal/otel/metrics.go` -- verified SyncDuration and SyncOperations are registered but never recorded
- `internal/sync/worker.go` -- verified no `.Record()` or `.Add()` calls exist
- `internal/peeringdb/client.go` -- verified no otelhttp transport wrapping, no manual spans
- [OTel Go instrumentation guide](https://opentelemetry.io/docs/languages/go/instrumentation/) -- manual span creation, metric instruments, observable gauges
- [OTel HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/) -- client span naming, `http.request.resend_count` for retries
- [otelhttp pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) -- NewTransport API, options, automatic attributes
- [OTel metric API pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/otel/metric) -- Float64ObservableGauge, callback registration, Observer interface

### Secondary (MEDIUM confidence)
- `.planning/research/ARCHITECTURE.md` -- span hierarchy design and integration patterns
- `.planning/research/PITFALLS.md` -- retry span hierarchy, transport layer violations, duplicate instrument registration
- `.planning/research/SUMMARY.md` -- overall phase structure and risk assessment (note: MeterProvider claim corrected)

### Tertiary (LOW confidence)
- None -- all findings verified against codebase source and official documentation.

## Project Constraints (from CLAUDE.md)

Directives extracted from CLAUDE.md that apply to this phase:

- **CS-0 (MUST):** Modern Go code guidelines
- **CS-5 (MUST):** Input structs for functions receiving more than 2 arguments (relevant if InitMetrics signature changes)
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **CC-2 (MUST):** Tie goroutine lifetime to context (relevant: no background goroutines for freshness gauge)
- **OBS-1 (MUST):** Structured logging with slog -- already established
- **OBS-4 (SHOULD):** OpenTelemetry for observability -- this is the entire phase
- **OBS-5 (SHOULD):** Use attribute setters like `slog.String()` -- already established
- **T-1 (MUST):** Table-driven tests, deterministic and hermetic
- **T-2 (MUST):** Run `-race` in CI, add `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **API-1 (MUST):** Document exported items
- **G-1 (MUST):** `go vet ./...` passes
- **G-2 (MUST):** `golangci-lint run` passes
- **G-3 (MUST):** `go test -race ./...` passes

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all packages already in go.mod, no new dependencies
- Architecture: HIGH -- file change scope is narrow (3 production files), patterns well-established in codebase
- Pitfalls: HIGH -- all pitfalls verified against actual source code and OTel documentation

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (stable -- OTel Go SDK v1.42.0 is a mature release)
