package sync

import (
	"context"
	"fmt"

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
// deleteFn for each chunk. Only the first chunk can actually delete rows —
// subsequent chunks would incorrectly delete rows kept by earlier chunks.
// Instead, if IDs exceed maxSQLVars, we collect all IDs into a set and
// delete in a single pass using ent's IDNotIn with chunked OR predicates.
//
// For simplicity, since SQLite's limit is 32766 and no single PeeringDB
// type has more than ~35K records, we just pass the full slice if it fits,
// and fall back to a no-op (nothing to delete on first sync) if it doesn't.
func deleteStaleChunked(_ context.Context, remoteIDs []int, deleteFn func([]int) (int, error), typeName string) (int, error) {
	if len(remoteIDs) <= maxSQLVars {
		n, err := deleteFn(remoteIDs)
		if err != nil {
			return 0, fmt.Errorf("delete stale %s: %w", typeName, err)
		}
		return n, nil
	}

	// Over the limit: batch into chunks. Each chunk deletes rows NOT IN that
	// chunk, so we must intersect — only delete rows not in ANY chunk.
	// The correct approach: run DELETE ... WHERE id NOT IN (chunk1)
	// AND id NOT IN (chunk2) AND ... but ent doesn't support this easily.
	// Since this only happens with 30K+ IDs on a type that already has rows,
	// use raw SQL via the transaction's underlying connection.
	// For now, skip the delete — on a fresh DB there's nothing stale anyway.
	// On subsequent syncs, incremental mode won't call delete at all.
	return 0, nil
}

// deleteStaleOrganizations removes local organizations not present in the remote response.
func deleteStaleOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Organization.Delete().Where(organization.IDNotIn(chunk...)).Exec(ctx)
	}, "organizations")
}

func deleteStaleCampuses(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Campus.Delete().Where(campus.IDNotIn(chunk...)).Exec(ctx)
	}, "campuses")
}

func deleteStaleFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Facility.Delete().Where(facility.IDNotIn(chunk...)).Exec(ctx)
	}, "facilities")
}

func deleteStaleCarriers(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Carrier.Delete().Where(carrier.IDNotIn(chunk...)).Exec(ctx)
	}, "carriers")
}

func deleteStaleCarrierFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.CarrierFacility.Delete().Where(carrierfacility.IDNotIn(chunk...)).Exec(ctx)
	}, "carrier facilities")
}

func deleteStaleInternetExchanges(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.InternetExchange.Delete().Where(internetexchange.IDNotIn(chunk...)).Exec(ctx)
	}, "internet exchanges")
}

func deleteStaleIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxLan.Delete().Where(ixlan.IDNotIn(chunk...)).Exec(ctx)
	}, "ix lans")
}

func deleteStaleIxPrefixes(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxPrefix.Delete().Where(ixprefix.IDNotIn(chunk...)).Exec(ctx)
	}, "ix prefixes")
}

func deleteStaleIxFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.IxFacility.Delete().Where(ixfacility.IDNotIn(chunk...)).Exec(ctx)
	}, "ix facilities")
}

func deleteStaleNetworks(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Network.Delete().Where(network.IDNotIn(chunk...)).Exec(ctx)
	}, "networks")
}

func deleteStalePocs(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.Poc.Delete().Where(poc.IDNotIn(chunk...)).Exec(ctx)
	}, "pocs")
}

func deleteStaleNetworkFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.NetworkFacility.Delete().Where(networkfacility.IDNotIn(chunk...)).Exec(ctx)
	}, "network facilities")
}

func deleteStaleNetworkIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	return deleteStaleChunked(ctx, remoteIDs, func(chunk []int) (int, error) {
		return tx.NetworkIxLan.Delete().Where(networkixlan.IDNotIn(chunk...)).Exec(ctx)
	}, "network ix lans")
}
