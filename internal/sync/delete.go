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

// deleteStaleOrganizations removes local organizations not present in the remote response.
func deleteStaleOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Organization.Delete().
		Where(organization.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale organizations: %w", err)
	}
	return n, nil
}

// deleteStaleCampuses removes local campuses not present in the remote response.
func deleteStaleCampuses(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Campus.Delete().
		Where(campus.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale campuses: %w", err)
	}
	return n, nil
}

// deleteStaleFacilities removes local facilities not present in the remote response.
func deleteStaleFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Facility.Delete().
		Where(facility.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale facilities: %w", err)
	}
	return n, nil
}

// deleteStaleCarriers removes local carriers not present in the remote response.
func deleteStaleCarriers(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Carrier.Delete().
		Where(carrier.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale carriers: %w", err)
	}
	return n, nil
}

// deleteStaleCarrierFacilities removes local carrier-facility associations not present in the remote response.
func deleteStaleCarrierFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.CarrierFacility.Delete().
		Where(carrierfacility.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale carrier facilities: %w", err)
	}
	return n, nil
}

// deleteStaleInternetExchanges removes local internet exchanges not present in the remote response.
func deleteStaleInternetExchanges(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.InternetExchange.Delete().
		Where(internetexchange.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale internet exchanges: %w", err)
	}
	return n, nil
}

// deleteStaleIxLans removes local IX LANs not present in the remote response.
func deleteStaleIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.IxLan.Delete().
		Where(ixlan.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale ix lans: %w", err)
	}
	return n, nil
}

// deleteStaleIxPrefixes removes local IX prefixes not present in the remote response.
func deleteStaleIxPrefixes(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.IxPrefix.Delete().
		Where(ixprefix.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale ix prefixes: %w", err)
	}
	return n, nil
}

// deleteStaleIxFacilities removes local IX-facility associations not present in the remote response.
func deleteStaleIxFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.IxFacility.Delete().
		Where(ixfacility.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale ix facilities: %w", err)
	}
	return n, nil
}

// deleteStaleNetworks removes local networks not present in the remote response.
func deleteStaleNetworks(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Network.Delete().
		Where(network.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale networks: %w", err)
	}
	return n, nil
}

// deleteStalePocs removes local POCs not present in the remote response.
func deleteStalePocs(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.Poc.Delete().
		Where(poc.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale pocs: %w", err)
	}
	return n, nil
}

// deleteStaleNetworkFacilities removes local network-facility associations not present in the remote response.
func deleteStaleNetworkFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.NetworkFacility.Delete().
		Where(networkfacility.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale network facilities: %w", err)
	}
	return n, nil
}

// deleteStaleNetworkIxLans removes local network-IXLan associations not present in the remote response.
func deleteStaleNetworkIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int) (int, error) {
	n, err := tx.NetworkIxLan.Delete().
		Where(networkixlan.IDNotIn(remoteIDs...)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete stale network ix lans: %w", err)
	}
	return n, nil
}
