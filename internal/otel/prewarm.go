package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// PeeringDBEntityTypes is the canonical list of 13 PeeringDB entity type
// names used as the `type` attribute on per-type sync counters. The list
// is hand-copied from internal/sync/worker.go syncSteps() and MUST stay
// in sync with that source — adding a 14th entity to syncSteps without
// updating this list will leave the new type's panels showing "No data"
// until its first real event fires.
//
// internal/otel cannot import internal/sync (would create an import cycle),
// so parity is enforced by manual review + a grep gate in the Phase 75
// Plan 02 acceptance criteria. If this constraint becomes load-bearing,
// promote the canonical list to a leaf package (e.g. internal/pdbtypes)
// that both packages can import.
var PeeringDBEntityTypes = []string{
	"org", "campus", "fac", "carrier", "carrierfac",
	"ix", "ixlan", "ixpfx", "ixfac",
	"net", "poc", "netfac", "netixlan",
}

// PrewarmCounters emits a single zero-valued .Add(ctx, 0, ...) on each of
// the 5 zero-rate sync/role counters with the baseline attribute set so
// every (counter, attribute) tuple registers with the OTel SDK at process
// startup. Without this, OTel cumulative counters only export a series
// after their first non-zero .Add(), which means dashboard panels for
// fallback / fetch-errors / upsert-errors / deletes / role-transitions
// render "No data" rather than "0" on a healthy fleet that hasn't fired
// any of those events yet.
//
// Phase 75 OBS-02 (D-02). The per-type set covers the 4 per-type counters
// (4 × 13 = 52 baseline series). RoleTransitions is the special case:
// it labels by direction not type, so it gets 2 baseline series
// (promoted, demoted) — see internal/sync/worker.go:1634/1651 for the
// production emission sites that establish the direction attribute.
//
// Total baseline series introduced: 52 + 2 = 54.
//
// MUST be called AFTER InitMetrics() has populated the counter package
// vars. The call site in cmd/peeringdb-plus/main.go enforces this
// ordering: InitMetrics() runs first, PrewarmCounters() runs after the
// syncWorker is constructed but before StartScheduler spawns its
// goroutine. If a future refactor reorders the call site, the per-counter
// nil-guard below surfaces the misordering via otel.Handle (a
// no-counter-pre-warm WARN in startup logs) instead of panicking on
// counter.Add against a nil instrument. REVIEW WR-03.
//
// Per GO-CTX-1: ctx is the first parameter.
// Per GO-OBS-5: attribute.String() typed-attr setter rather than raw KV.
func PrewarmCounters(ctx context.Context) {
	// Defensive guard: a nil counter means InitMetrics() did not run (or
	// did not populate this var). Surface via otel.Handle so the failure
	// is grep-able in startup logs and skip the pre-warm for that
	// counter rather than panicking inside the OTel SDK's Add internals.
	// REVIEW WR-03.
	checked := []struct {
		name string
		c    metric.Int64Counter
	}{
		{"SyncTypeFallback", SyncTypeFallback},
		{"SyncTypeFetchErrors", SyncTypeFetchErrors},
		{"SyncTypeUpsertErrors", SyncTypeUpsertErrors},
		{"SyncTypeDeleted", SyncTypeDeleted},
		{"RoleTransitions", RoleTransitions},
	}
	for _, e := range checked {
		if e.c == nil {
			otel.Handle(fmt.Errorf("prewarm: counter %q is nil — InitMetrics() must run first", e.name))
		}
	}

	for _, t := range PeeringDBEntityTypes {
		typeAttr := metric.WithAttributes(attribute.String("type", t))
		if SyncTypeFallback != nil {
			SyncTypeFallback.Add(ctx, 0, typeAttr)
		}
		if SyncTypeFetchErrors != nil {
			SyncTypeFetchErrors.Add(ctx, 0, typeAttr)
		}
		if SyncTypeUpsertErrors != nil {
			SyncTypeUpsertErrors.Add(ctx, 0, typeAttr)
		}
		if SyncTypeDeleted != nil {
			SyncTypeDeleted.Add(ctx, 0, typeAttr)
		}
	}
	// RoleTransitions labels by direction, not type — match the wire
	// shape established by internal/sync/worker.go:1634/1651.
	if RoleTransitions != nil {
		for _, d := range []string{"promoted", "demoted"} {
			RoleTransitions.Add(ctx, 0, metric.WithAttributes(attribute.String("direction", d)))
		}
	}
}
