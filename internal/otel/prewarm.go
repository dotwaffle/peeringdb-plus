package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
)

// PrewarmCounters emits a single zero-valued .Add(ctx, 0, ...) on each of
// the 5 zero-rate sync/role counters with the baseline attribute set so
// every (counter, attribute) tuple registers with the OTel SDK at process
// startup. Without this, OTel cumulative counters only export a series
// after their first non-zero .Add(), which means dashboard panels for
// fallback / fetch-errors / upsert-errors / deletes / role-transitions
// render "No data" rather than "0" on a healthy fleet that hasn't fired
// any of those events yet.
//
// The per-type set covers the 4 per-type counters
// (4 × 13 = 52 baseline series). RoleTransitions is the special case:
// it labels by direction not type, so it gets 2 baseline series
// (promoted, demoted) — see internal/sync/worker.go:1634/1651 for the
// production emission sites that establish the direction attribute.
//
// Total baseline series introduced: 52 + 2 = 54.
//
// MUST be called AFTER otel Setup() has installed the real
// MeterProvider — instruments are bound at package init and delegate to
// the provider set by main, so a pre-Setup call would pre-warm a no-op.
// The call site in cmd/peeringdb-plus/main.go runs after the syncWorker
// is constructed but before StartScheduler spawns its goroutine.
func PrewarmCounters(ctx context.Context) {
	for _, t := range pdbtypes.Names() {
		typeAttr := metric.WithAttributes(attribute.String("type", t))
		SyncTypeFallback.Add(ctx, 0, typeAttr)
		SyncTypeFetchErrors.Add(ctx, 0, typeAttr)
		SyncTypeUpsertErrors.Add(ctx, 0, typeAttr)
		SyncTypeDeleted.Add(ctx, 0, typeAttr)
	}
	// RoleTransitions labels by direction, not type — match the wire
	// shape established by internal/sync/worker.go:1634/1651.
	for _, d := range []string{"promoted", "demoted"} {
		RoleTransitions.Add(ctx, 0, metric.WithAttributes(attribute.String("direction", d)))
	}
}
