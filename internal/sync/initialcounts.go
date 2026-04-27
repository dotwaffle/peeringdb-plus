package sync

// Phase 75 Plan 01 (OBS-01): cold-start population of the
// pdbplus_data_type_count gauge.
//
// Until v1.18.0 the gauge cache (cmd/peeringdb-plus/main.go atomic.Pointer)
// was only ever primed by the OnSyncComplete callback, which fires after the
// first sync cycle completes (~15 min default; ~1 h on unauthenticated
// instances). The OTel ObservableGauge callback that backs
// pdbplus_data_type_count therefore reported zeros for every type during the
// pre-first-sync window of every fresh deploy, rendering the dashboard's
// "Total Objects", "Objects by Type", and "Object Counts Over Time" panels
// flat-zero or "No data".
//
// This file adds the second primer: a synchronous one-shot Count(ctx) per
// entity table at process startup, called from main.go between database.Open
// and pdbotel.InitObjectCountGauges. The cost is ~1-2s on a primed LiteFS
// DB; replicas already cold-sync the DB in 5-45s so the extra latency is
// noise on top of hydration. The sync-completion path is unchanged — it
// just stops being the SOLE primer.

import (
	"context"
	"fmt"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// InitialObjectCounts runs a one-shot Count(ctx) against each of the 13
// PeeringDB entity tables and returns the result keyed by PeeringDB type
// name. The keys match those produced by syncSteps() so the same atomic
// cache can be primed by either the startup path (this helper) or the
// OnSyncComplete callback.
//
// Implements OBS-01 D-01: synchronous startup population so the
// pdbplus_data_type_count gauge reports correct values within 30s of
// process start instead of holding zeros until the first sync cycle
// completes (~15 min default).
//
// Cost: ~1-2s on a primed LiteFS DB (13 sequential SQLite COUNT(*)
// queries against indexed tables). Replicas already cold-sync the DB
// in 5-45s; the added latency is noise on top of hydration.
//
// Errors are returned wrapped with the type name so an operator can see
// which table failed; partial results are NOT returned — a single failure
// aborts the whole call to keep the contract simple. The caller chooses
// whether to fail-fast or proceed with stale zeros.
//
// Note: counts include all rows regardless of status (matching the
// existing OnSyncComplete cache contract — "raw upserted-row count from
// the latest sync cycle"). Phase 68 tombstones (status="deleted") are
// rows the dashboard wants to see in "Total Objects" until tombstone GC
// ships (SEED-004 dormant). If a future requirement wants live-only
// counts, that's a separate metric.
//
// Tier elevation: ctx is stamped with TierUsers via privctx.WithTier
// before each Count call. Without this, Poc.Policy() (the only entity
// with a row-level Privacy policy) would filter visible!="Public" rows
// out of this counter while the OnSyncComplete writer — which runs
// under privacy.DecisionContext(ctx, privacy.Allow) — counts every
// row. The cross-writer disagreement specifically on POC produced the
// `pdbplus_data_type_count{type="poc"}` 2x/0.5x oscillation that was
// visible on the Grafana "Object Counts Over Time" panel: replicas
// (which only ever run InitialObjectCounts) held the public-only count
// `P` while the primary's cache flipped between `T ≈ 2P` (just after a
// full sync) and tiny incremental deltas, and `max by(type)` across
// the 8-instance fleet alternated between `T` and `P` accordingly.
// See .planning/debug/poc-count-doubling-halving.md for the full
// analysis.
//
// We use privctx.WithTier(ctx, TierUsers) rather than
// privacy.DecisionContext(ctx, privacy.Allow) deliberately — the
// internal/sync/bypass_audit_test.go invariant restricts privacy.Allow
// to exactly one production call site (Worker.Sync). TierUsers is the
// documented mechanism for "non-sync tier elevation" (see
// internal/sync/bypass_audit_test.go:208 and internal/privctx/privctx.go
// godoc) and produces the same effect on Poc.Policy without diluting
// the bypass audit.
func InitialObjectCounts(ctx context.Context, client *ent.Client) (map[string]int64, error) {
	// Tier-elevate so Poc.Policy admits visible="Users" rows. Symmetry
	// with the OnSyncComplete writer: both must count the same row set
	// or the gauge oscillates between writer values across the
	// 8-instance fleet (replicas hold this value forever since they
	// never sync).
	ctx = privctx.WithTier(ctx, privctx.TierUsers)

	counts := make(map[string]int64, 13)

	type counter struct {
		name string
		run  func(context.Context) (int, error)
	}
	queries := []counter{
		{"org", func(c context.Context) (int, error) { return client.Organization.Query().Count(c) }},
		{"campus", func(c context.Context) (int, error) { return client.Campus.Query().Count(c) }},
		{"fac", func(c context.Context) (int, error) { return client.Facility.Query().Count(c) }},
		{"carrier", func(c context.Context) (int, error) { return client.Carrier.Query().Count(c) }},
		{"carrierfac", func(c context.Context) (int, error) { return client.CarrierFacility.Query().Count(c) }},
		{"ix", func(c context.Context) (int, error) { return client.InternetExchange.Query().Count(c) }},
		{"ixlan", func(c context.Context) (int, error) { return client.IxLan.Query().Count(c) }},
		{"ixpfx", func(c context.Context) (int, error) { return client.IxPrefix.Query().Count(c) }},
		{"ixfac", func(c context.Context) (int, error) { return client.IxFacility.Query().Count(c) }},
		{"net", func(c context.Context) (int, error) { return client.Network.Query().Count(c) }},
		{"poc", func(c context.Context) (int, error) { return client.Poc.Query().Count(c) }},
		{"netfac", func(c context.Context) (int, error) { return client.NetworkFacility.Query().Count(c) }},
		{"netixlan", func(c context.Context) (int, error) { return client.NetworkIxLan.Query().Count(c) }},
	}

	for _, q := range queries {
		// Honour ctx cancellation between queries so a SIGTERM mid-boot
		// (e.g. Fly killing a stuck instance during cold-start) unwinds
		// promptly rather than running all 13 sequential SQLite COUNT(*)
		// calls to completion. The SQLite driver does check ctx, but on
		// a FUSE-backed LiteFS mount that's still hydrating, syscall
		// blocking can swallow cancellation for seconds at a time.
		// REVIEW WR-02.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("count %s: %w", q.name, err)
		}
		n, err := q.run(ctx)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", q.name, err)
		}
		counts[q.name] = int64(n)
	}
	return counts, nil
}
