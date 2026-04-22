package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
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

// markStaleDeletedJSON marshals remoteIDs to a JSON array and passes it to the
// provided deleteFn. Leverages SQLite's json_each(?) function to bypass the
// 32766 variable limit (maxSQLVars) that previously caused syncs to skip
// deletes on large types like netixlan (200K+ rows).
func markStaleDeletedJSON(remoteIDs []int, deleteFn func(string) (int, error), typeName string) (int, error) {
	jsonIDs, err := json.Marshal(remoteIDs)
	if err != nil {
		return 0, fmt.Errorf("marshal remote ids %s: %w", typeName, err)
	}
	return deleteFn(string(jsonIDs))
}

// markStaleDeletedOrganizations soft-deletes local organizations absent from
// the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path so the upstream
// rest.py:700-712 status × since matrix returns tombstones.
func markStaleDeletedOrganizations(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Organization.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(organization.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "organizations")
}

// markStaleDeletedCampuses soft-deletes local campuses absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCampuses(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Campus.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(campus.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "campuses")
}

// markStaleDeletedFacilities soft-deletes local facilities absent from the
// remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Facility.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(facility.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "facilities")
}

// markStaleDeletedCarriers soft-deletes local carriers absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCarriers(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Carrier.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(carrier.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "carriers")
}

// markStaleDeletedCarrierFacilities soft-deletes local carrier-facilities
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedCarrierFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.CarrierFacility.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(carrierfacility.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "carrier facilities")
}

// markStaleDeletedInternetExchanges soft-deletes local internet-exchanges
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedInternetExchanges(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.InternetExchange.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(internetexchange.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "internet exchanges")
}

// markStaleDeletedIxLans soft-deletes local IX-LANs absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.IxLan.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(ixlan.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix lans")
}

// markStaleDeletedIxPrefixes soft-deletes local IX-prefixes absent from the
// remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxPrefixes(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.IxPrefix.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(ixprefix.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix prefixes")
}

// markStaleDeletedIxFacilities soft-deletes local IX-facilities absent from
// the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedIxFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.IxFacility.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(ixfacility.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "ix facilities")
}

// markStaleDeletedNetworks soft-deletes local networks absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworks(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Network.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(network.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "networks")
}

// markStaleDeletedPocs soft-deletes local POCs absent from the remote
// response by setting status='deleted' and updated=cycleStart (Phase 68 D-02).
// Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedPocs(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.Poc.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(poc.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "pocs")
}

// markStaleDeletedNetworkFacilities soft-deletes local network-facilities
// absent from the remote response by setting status='deleted' and
// updated=cycleStart (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworkFacilities(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.NetworkFacility.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(networkfacility.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "network facilities")
}

// markStaleDeletedNetworkIxLans soft-deletes local network-IX-LANs absent
// from the remote response by setting status='deleted' and updated=cycleStart
// (Phase 68 D-02). Replaces the pre-v1.16 hard-delete path.
func markStaleDeletedNetworkIxLans(ctx context.Context, tx *ent.Tx, remoteIDs []int, cycleStart time.Time) (int, error) {
	return markStaleDeletedJSON(remoteIDs, func(jsonStr string) (int, error) {
		return tx.NetworkIxLan.Update().
			Where(func(s *sql.Selector) {
				s.Where(sql.ExprP(s.C(networkixlan.FieldID)+" NOT IN (SELECT value FROM json_each(?))", jsonStr))
			}).
			SetStatus("deleted").
			SetUpdated(cycleStart).
			Save(ctx)
	}, "network ix lans")
}
