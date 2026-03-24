---
phase: 04-observability-foundations
plan: 01
subsystem: observability
tags: [otel, tracing, otelhttp, spans, peeringdb-client]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: PeeringDB HTTP client (internal/peeringdb/client.go) with pagination and retry logic
provides:
  - OTel trace spans on all outbound PeeringDB HTTP requests via otelhttp transport
  - Business-level span hierarchy (FetchAll parent > per-attempt children > HTTP transport spans)
  - Page-level span events with page number, count, running total
  - Rate limiter wait events on attempt spans when wait exceeds 1ms
affects: [04-observability-foundations, sync-debugging, production-monitoring]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "otelhttp.NewTransport wrapping for automatic HTTP client span creation"
    - "Manual parent span in FetchAll for business-level trace hierarchy"
    - "Per-attempt child spans with resend_count attribute in retry loop"
    - "Context scoping (attemptCtx vs ctx) to keep retry spans as siblings not chained"
    - "tracetest.InMemoryExporter for span hierarchy verification in tests"

key-files:
  created: []
  modified:
    - internal/peeringdb/client.go
    - internal/peeringdb/client_test.go

key-decisions:
  - "Used attemptCtx variable to avoid context shadowing in retry loop (Pitfall 3 from research)"
  - "Trace tests are non-parallel since they mutate global TracerProvider"

patterns-established:
  - "OTel span hierarchy: tracer.Start(ctx, name) for parent, tracer.Start(ctx, name) with different var for child siblings"
  - "Span events for page-level data (AddEvent with trace.WithAttributes)"
  - "tracetest.InMemoryExporter + sdktrace.WithSyncer for test span verification"

requirements-completed: [OBS-02]

# Metrics
duration: 4min
completed: 2026-03-22
---

# Phase 4 Plan 1: PeeringDB Client Tracing Summary

**OTel trace spans on PeeringDB HTTP client with otelhttp transport wrapping and manual span hierarchy (FetchAll parent, per-attempt children, page events, rate limiter wait events)**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-22T21:37:00Z
- **Completed:** 2026-03-22T21:41:32Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Every outbound HTTP request to PeeringDB now produces an OTel trace span with HTTP semantic conventions via otelhttp.NewTransport
- FetchAll creates a named parent span `peeringdb.fetch/{type}` grouping all pagination requests with page.fetched events
- Each doWithRetry attempt produces its own child span with `http.request.resend_count` attribute
- Rate limiter wait time recorded as span events when wait exceeds 1ms
- Three new tests verify span hierarchy, page events, and per-attempt spans using tracetest.InMemoryExporter

## Task Commits

Each task was committed atomically:

1. **Task 1: Add otelhttp transport and manual span hierarchy** - `51f9cb7` (feat)
2. **Task 2: Add span hierarchy verification tests** - `3b720c0` (test)

## Files Created/Modified
- `internal/peeringdb/client.go` - Added otelhttp transport, FetchAll parent span with page events, per-attempt child spans in doWithRetry with rate limiter wait events
- `internal/peeringdb/client_test.go` - Added TestFetchAllCreatesSpanHierarchy, TestFetchAllRecordsPageEvents, TestDoWithRetryCreatesPerAttemptSpans with tracetest.InMemoryExporter

## Decisions Made
- Used `attemptCtx` variable name (not `ctx`) in doWithRetry loop to avoid context shadowing, ensuring retry attempt spans are siblings under the FetchAll span rather than chained grandchildren (per Pitfall 3 from research)
- Made trace tests non-parallel since they mutate the global TracerProvider; existing tests remain parallel and work with any provider

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- PeeringDB HTTP client tracing complete, ready for Plan 02 (sync metrics wiring)
- The trace hierarchy integrates with the existing sync worker spans (full-sync > sync-{type} > peeringdb.fetch/{type} > peeringdb.request)

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 04-observability-foundations*
*Completed: 2026-03-22*
