# Phase 4: Observability Foundations - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Complete OTel instrumentation for the PeeringDB sync pipeline. Every outbound HTTP call to PeeringDB gets traced, every sync step gets measured with per-type metrics, and the existing registered-but-unrecorded metrics get wired to record real values.

</domain>

<decisions>
## Implementation Decisions

### HTTP Client Tracing
- **D-01:** Use both otelhttp.NewTransport (automatic HTTP semantic conventions) AND manual parent spans in FetchAll for business-level hierarchy
- **D-02:** FetchAll parent spans named `peeringdb.fetch/{type}` (e.g. `peeringdb.fetch/net`) — type in span name, follows OTel RPC naming convention
- **D-03:** Each doWithRetry attempt gets its own explicit child span with attempt number attribute — makes retry behavior visible in traces
- **D-04:** Page-level data recorded as span events on the FetchAll span (page number, count, running total per page)
- **D-05:** Rate limiter wait time recorded as a span event (`rate_limiter.wait`) with wait duration on each request span

### Sync Metrics
- **D-06:** MeterProvider already initialized in provider.go — no changes needed. The fix is wiring `.Record()` and `.Add()` calls in worker.go for the existing SyncDuration and SyncOperations instruments
- **D-07:** Add 4 new per-type metric instruments: duration histogram, object count, delete count, and separate fetch_errors and upsert_errors counters
- **D-08:** Flat naming convention with type attribute: `pdbplus.sync.type.duration`, `pdbplus.sync.type.objects`, `pdbplus.sync.type.deleted`, `pdbplus.sync.type.fetch_errors`, `pdbplus.sync.type.upsert_errors` — all with `type` attribute (net, ix, fac, etc.)
- **D-09:** Add sync freshness gauge using OTel observable gauge with callback — only computes seconds-since-last-sync when metric backend scrapes. No background goroutine.
- **D-10:** Fetch errors (PeeringDB HTTP client failures) and upsert errors (ent database failures) tracked as separate counters, not combined

### Scope
- **D-11:** OTel-related tech debt only. Do NOT clean up vestigial DataLoader middleware, config.IsPrimary field, or unused globalid.go exports — those are separate work items.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### OTel Packages (already in go.mod)
- `internal/otel/provider.go` — Existing TracerProvider + MeterProvider + LoggerProvider setup
- `internal/otel/metrics.go` — Existing SyncDuration and SyncOperations instrument registration
- `internal/otel/metrics_test.go` — Existing metric registration tests

### PeeringDB Client
- `internal/peeringdb/client.go` — HTTP client with FetchAll, doWithRetry, rate limiting — needs otelhttp transport and manual spans

### Sync Worker
- `internal/sync/worker.go` — Sync orchestration with per-step loop — needs metric recording calls
- `cmd/peeringdb-plus/main.go` — Application wiring, confirms OTel setup and InitMetrics() call order

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `otelhttp.NewTransport` already imported in main.go (used for server middleware) — same package provides client transport wrapping
- `otel.Tracer("sync")` already used in worker.go for sync-level and per-step spans — extend this pattern
- `otel.Meter("peeringdb-plus")` already used in metrics.go — add new instruments here

### Established Patterns
- Sync step functions return `(count int, deleted int, err error)` — perfect for per-type metric recording without changing the interface
- Worker already has `start := time.Now()` and `time.Since(start)` for duration calculation
- InitMetrics() called after OTel Setup() in main.go — new instruments follow the same init pattern

### Integration Points
- `NewClient()` in peeringdb/client.go — inject otelhttp.NewTransport wrapping http.DefaultTransport
- Per-step loop in `worker.go:Sync()` — add metric recording after each step.fn() call
- `recordFailure()` and post-commit success block — record overall sync metrics here
- `metrics.go:InitMetrics()` — register new per-type instruments here

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

- Clean up vestigial DataLoader middleware — separate tech debt task
- Remove config.IsPrimary field — separate tech debt task
- Remove unused globalid.go exports — separate tech debt task

</deferred>

---

*Phase: 04-observability-foundations*
*Context gathered: 2026-03-22*
