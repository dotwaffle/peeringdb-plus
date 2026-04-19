package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
)

// maxSQLVars is the maximum number of SQL variables per statement.
// SQLite's default SQLITE_MAX_VARIABLE_NUMBER is 32766.
const maxSQLVars = 30000

// deleteStaleChunked splits remoteIDs into chunks under maxSQLVars and runs
// the per-entity soft-delete closure (Phase 68 D-02: UPDATE ... SET
// status='deleted', updated=cycleStart WHERE id NOT IN (...)). Rows beyond
// maxSQLVars are silently NOT soft-deleted — same edge-case behaviour as the
// pre-v1.16 hard-delete path, flagged for SEED-004 tombstone-GC follow-up
// (Phase 68 research Open Question 4).
//
// For simplicity, since SQLite's limit is 32766 and no single PeeringDB
// type has more than ~35K records, we just pass the full slice if it fits,
// and fall back to a no-op (nothing to soft-delete on first sync) if it
// doesn't.
func deleteStaleChunked(ctx context.Context, remoteIDs []int, deleteFn func([]int) (int, error), typeName string) (int, error) {
	if len(remoteIDs) <= maxSQLVars {
		n, err := deleteFn(remoteIDs)
		if err != nil {
			return 0, fmt.Errorf("mark stale deleted %s: %w", typeName, err)
		}
		return n, nil
	}

	// Over the limit: chunked NOT-IN predicates cannot be AND-combined across
	// chunks because each chunk would re-mark rows kept by earlier chunks.
	// Pre-v1.16 hard-delete shared this fallback (no-op). SEED-004 covers the
	// tombstone-GC strategy and will subsume the >32K case.
	//
	// REVIEW WR-02: emit a WARN so the silent fallthrough is visible to
	// operators. Once any PeeringDB entity crosses the chunk limit, soft-delete
	// stops working for that type without this log signal. SEED-004 trigger
	// candidate.
	slog.WarnContext(ctx, "soft-delete skipped: remoteIDs exceed maxSQLVars chunk limit, SEED-004 trigger candidate",
		slog.String("type", typeName),
		slog.Int("remote_ids", len(remoteIDs)),
		slog.Int("max_vars", maxSQLVars),
	)
	return 0, nil
}

// markStaleDeletedOrganizations soft-deletes local organizations absent from
// the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path so the upstream
// rest.py:700-712 status × since matrix returns tombstones.
func markStaleDeletedOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Organization.Update().
			Where(organization.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "organizations")
}

// markStaleDeletedCampuses soft-deletes local campuses absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCampuses(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Campus.Update().
			Where(campus.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "campuses")
}

// markStaleDeletedFacilities soft-deletes local facilities absent from the
// remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Facility.Update().
			Where(facility.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "facilities")
}

// markStaleDeletedCarriers soft-deletes local carriers absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCarriers(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Carrier.Update().
			Where(carrier.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "carriers")
}

// markStaleDeletedCarrierFacilities soft-deletes local carrier-facilities
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCarrierFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.CarrierFacility.Update().
			Where(carrierfacility.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "carrier facilities")
}

// markStaleDeletedInternetExchanges soft-deletes local internet-exchanges
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedInternetExchanges(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.InternetExchange.Update().
			Where(internetexchange.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "internet exchanges")
}

// markStaleDeletedIxLans soft-deletes local IX-LANs absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxLan.Update().
			Where(ixlan.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix lans")
}

// markStaleDeletedIxPrefixes soft-deletes local IX-prefixes absent from the
// remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxPrefixes(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxPrefix.Update().
			Where(ixprefix.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix prefixes")
}

// markStaleDeletedIxFacilities soft-deletes local IX-facilities absent from
// the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxFacility.Update().
			Where(ixfacility.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix facilities")
}

// markStaleDeletedNetworks soft-deletes local networks absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworks(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Network.Update().
			Where(network.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "networks")
}

// markStaleDeletedPocs soft-deletes local POCs absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedPocs(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Poc.Update().
			Where(poc.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "pocs")
}

// markStaleDeletedNetworkFacilities soft-deletes local network-facilities
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworkFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.NetworkFacility.Update().
			Where(networkfacility.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "network facilities")
}

// markStaleDeletedNetworkIxLans soft-deletes local network-IX-LANs absent
// from the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworkIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.NetworkIxLan.Update().
			Where(networkixlan.IDNotIn(chunk...)).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "network ix lans")
}
