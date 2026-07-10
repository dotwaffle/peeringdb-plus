package sync

// Cold-start population of the pdbplus_data_type_count gauge.
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
// This primer issues exactly ONE SQL query
// (UNION ALL across the 13 entity tables) against the underlying *sql.DB
// instead of 13 sequential ent Count(ctx) round-trips.

import (
	"context"
	"database/sql"
	"fmt"
)

// InitialObjectCounts runs a one-shot UNION ALL COUNT(*) against each of
// the 13 PeeringDB entity tables and returns the result keyed by
// PeeringDB type name. The keys match those produced by syncSteps() so
// the same atomic cache can be primed by either the startup path (this
// helper) or the OnSyncComplete callback.
//
// Synchronous startup population so the
// pdbplus_data_type_count gauge reports correct values within 30s of
// process start instead of holding zeros until the first sync cycle
// completes (~15 min default, ~1h on unauthenticated instances).
//
// Cost: a single SQL UNION ALL across 13 tables; ~1ms on a primed
// LiteFS DB. Replaces the prior 13 sequential ent Count() calls
// (~15-20ms in aggregate). Counts include all rows regardless of status
// (matching the existing OnSyncComplete cache contract — "raw upserted-
// row count from the latest sync cycle"). Tombstones
// (status="deleted") are rows the dashboard wants to see in "Total
// Objects" until the dormant tombstone-GC work ships. If a future
// requirement wants live-only counts, that's a separate metric.
//
// Privacy: raw SQL bypasses ent's Privacy policy entirely (no Privacy
// Hook fires on db.QueryContext). The COUNT(*) sees every physical row
// regardless of privacy tier — symmetric with the OnSyncComplete writer
// (which runs under privacy.DecisionContext(ctx, privacy.Allow)).
//
// History: this function previously elevated ctx
// to TierUsers via privctx.WithTier to keep Poc.Policy from filtering
// visible!="Public" rows. Without it, the cross-writer disagreement on
// POC counts caused the pdbplus_data_type_count{type="poc"} 2x/0.5x
// oscillation visible on the Grafana "Object Counts Over Time" panel:
// replicas (which only ever ran InitialObjectCounts) held the public-
// only count P while the primary's cache flipped between T ≈ 2P (just
// after a full sync) and tiny incremental deltas, and max by(type)
// across the 8-instance fleet alternated between T and P accordingly.
// The tier elevation was retired entirely: raw SQL
// achieves the same row-set without going through ent privacy at all
// (a COUNT bypass is intentional and safe). See
// the project history for the full incident
// analysis.
//
// Errors are returned wrapped with the type name so an operator can
// see which table failed; partial results are NOT returned — a single
// failure aborts the whole call to keep the contract simple.
func InitialObjectCounts(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	// Honour ctx cancellation up-front so a SIGTERM mid-boot (e.g. Fly
	// killing a stuck instance during cold-start) unwinds promptly. The
	// SQLite driver does check ctx, but on a FUSE-backed LiteFS mount
	// that's still hydrating, syscall blocking can swallow cancellation
	// for seconds at a time.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("initial object counts: %w", err)
	}

	rows, err := db.QueryContext(ctx, initialCountsQuery)
	if err != nil {
		return nil, fmt.Errorf("initial object counts query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	counts := make(map[string]int64, 13)
	for rows.Next() {
		var (
			name  string
			count int64
		)
		if scanErr := rows.Scan(&name, &count); scanErr != nil {
			return nil, fmt.Errorf("scan initial object counts row: %w", scanErr)
		}
		counts[name] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate initial object counts: %w", err)
	}
	return counts, nil
}
