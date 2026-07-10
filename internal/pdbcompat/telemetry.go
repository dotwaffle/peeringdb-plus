package pdbcompat

import (
	"context"
	"runtime"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// Per-request heap-delta telemetry.
//
// The sampler is called exactly TWICE per request:
//  1. In dispatch, right after the Registry lookup (via
//     memStatsHeapInuseBytes), to capture the baseline HeapInuse — this
//     covers both the list and detail paths.
//  2. At the terminal path (via defer recordResponseHeapDelta) to capture
//     the exit HeapInuse and compute delta.
//
// The runtime memstats read is stop-the-world (STW) for the duration
// of the call. At our observed heap size (<200 MiB on primary, ~80 MiB
// on replicas), STW is a few microseconds —
// acceptable once per request but NEVER per row. This invariant is
// enforced by only calling the sampler at entry and exit, never inside
// any row iterator or per-object serialiser.
//
// IMPORTANT — read this as process-heap churn, not per-request cost.
// runtime.MemStats.HeapInuse is process-global, so when requests overlap
// (the common case under any real load) the entry/exit delta for one
// request also captures every concurrent request's allocations and any
// GC sweep that ran in between. A single sample is therefore noisy and
// can even be negative (clamped to 0 below). The histogram is only
// meaningful in aggregate: p50/p95/p99 over many samples track how much
// the process heap moves while a handler is in flight, which is what the
// sustained-heap watch panel actually wants. Do NOT read a high percentile as
// "this endpoint allocated N bytes" — attributing the delta to one
// request would require per-goroutine heap accounting, which the Go
// runtime does not expose.

// memStatsHeapInuseBytes samples runtime.MemStats.HeapInuse and returns
// it in bytes. Clamped to int64 range (HeapInuse is uint64; theoretical
// overflow would produce negative int64 values and poison the delta
// arithmetic downstream). Bytes is the canonical Prom unit
// (post-2026-04-26 audit); dashboards format MiB / GiB at render time.
func memStatsHeapInuseBytes() int64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	const maxInt64 = uint64(1<<63 - 1)
	if ms.HeapInuse >= maxInt64 {
		return int64(maxInt64)
	}
	return int64(ms.HeapInuse)
}

// recordResponseHeapDelta samples HeapInuse at call time, computes the
// delta against startBytes, and emits both:
//
//   - OTel span attribute pdbplus.response.heap_delta_bytes on the active
//     span (no-op if the span is not recording / not installed).
//   - Prometheus histogram observation on pdbplus.response.heap_delta
//     with endpoint + entity labels (the instrument is bound at
//     internal/otel package init, so it is never nil).
//
// Intended usage: `defer recordResponseHeapDelta(ctx, endpoint, entity,
// startBytes)` in dispatch after the Registry lookup (after startBytes :=
// memStatsHeapInuseBytes()). Every terminal path of the list and detail
// handlers — 200 success, 400 bad-id/filter-error, 404, 413
// budget-exceeded, 500 query-error, 503 pool-exhausted — triggers exactly
// one observation via the defer.
//
// Negative deltas (end < start) are clamped to 0. GC cycles between
// entry and exit can legitimately shrink HeapInuse; a negative histogram
// sample is not meaningful for "how much heap did this request cost"
// and would confuse operators reading the dashboard.
func recordResponseHeapDelta(ctx context.Context, endpoint, entity string, startBytes int64) {
	endBytes := memStatsHeapInuseBytes()
	delta := max(endBytes-startBytes, 0)
	// OTel span attribute. SpanFromContext on a ctx without a tracer
	// provider returns a noop span whose SetAttributes is safe; the
	// IsValid guard is for clarity and to skip the alloc on the
	// attribute.KeyValue slice when nothing will record it.
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.SetAttributes(attribute.Int64("pdbplus.response.heap_delta_bytes", delta))
	}
	pdbotel.ResponseHeapDeltaBytes.Record(ctx, delta,
		metric.WithAttributes(
			attribute.String("endpoint", endpoint),
			attribute.String("entity", entity),
		),
	)
}
