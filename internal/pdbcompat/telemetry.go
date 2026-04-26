package pdbcompat

import (
	"context"
	"runtime"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// Phase 71 Plan 05 (MEMORY-03, D-06): per-request heap-delta telemetry.
//
// The sampler is called exactly TWICE per request:
//  1. At the top of serveList (via memStatsHeapInuseBytes) to capture the
//     baseline HeapInuse.
//  2. At the terminal path (via defer recordResponseHeapDelta) to capture
//     the exit HeapInuse and compute delta.
//
// The runtime memstats read is stop-the-world (STW) for the duration
// of the call. At our observed heap size (<200 MiB on primary, ~80 MiB
// on replicas post-Phase-66 telemetry), STW is a few microseconds —
// acceptable once per request but NEVER per row. This invariant is
// enforced by only calling the sampler at entry and exit, never inside
// any row iterator or per-object serialiser.

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
//     with endpoint + entity labels (no-op if
//     pdbotel.ResponseHeapDeltaBytes is nil, e.g. if
//     InitResponseHeapHistogram was not called — the request path must
//     never crash over a missing telemetry instrument).
//
// Intended usage: `defer recordResponseHeapDelta(ctx, endpoint, entity,
// startBytes)` at the top of serveList (after startBytes :=
// memStatsHeapInuseBytes()). Every terminal path of the handler — 200
// success, 413 budget-exceeded, 400 filter-error, 500 query-error —
// triggers exactly one observation via the defer.
//
// Negative deltas (end < start) are clamped to 0. GC cycles between
// entry and exit can legitimately shrink HeapInuse; a negative histogram
// sample is not meaningful for "how much heap did this request cost"
// and would confuse operators reading the dashboard.
func recordResponseHeapDelta(ctx context.Context, endpoint, entity string, startBytes int64) {
	endBytes := memStatsHeapInuseBytes()
	delta := endBytes - startBytes
	if delta < 0 {
		delta = 0
	}
	// OTel span attribute. SpanFromContext on a ctx without a tracer
	// provider returns a noop span whose SetAttributes is safe; the
	// IsValid guard is for clarity and to skip the alloc on the
	// attribute.KeyValue slice when nothing will record it.
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.SetAttributes(attribute.Int64("pdbplus.response.heap_delta_bytes", delta))
	}
	// Prometheus histogram. Nil-guarded so a failed registration cannot
	// panic the request path (telemetry is best-effort).
	if pdbotel.ResponseHeapDeltaBytes != nil {
		pdbotel.ResponseHeapDeltaBytes.Record(ctx, delta,
			metric.WithAttributes(
				attribute.String("endpoint", endpoint),
				attribute.String("entity", entity),
			),
		)
	}
}
