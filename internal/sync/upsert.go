// Package sync orchestrates data synchronization from PeeringDB into the
// local SQLite database using the ent ORM.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// skipUnchangedPredicate emits the WHERE clause for ON CONFLICT DO UPDATE
// that gates writes on the upstream `updated` timestamp. It returns a
// predicate equivalent to:
//
//	excluded.updated > <table>.updated
//	OR <table>.updated IS NULL
//	OR <table>.updated <= '1900-01-01'
//
// Full-mode cycles use `>=` in the first term and can add a cutoff term
// (see below). They also write a row only when one of its columns
// differs (see writeRowDiffers).
//
// The OR-IS-NULL / OR-pre-1900 guards exist because PeeringDB rows
// occasionally land with zero `updated` (legacy rows pre-Phase-X
// migration); we must always allow them through, otherwise a row with
// `updated` permanently at zero would become unwriteable. The literal
// '1900-01-01' is the lower-bound sentinel: any real PeeringDB
// timestamp is post-2000, and modernc.org/sqlite stores Go time.Time{}
// as text '0001-01-01 00:00:00+00:00' under the default value
// converter (verified empirically — a probe
// observed: typeof = "text", raw = "0001-01-01 00:00:00 +0000 UTC",
// SELECT (updated <= '1900-01-01') = 1).
//
// SAME-SECOND DRIFT (known limitation): a row
// edited at upstream within the same second as the prior cursor
// advance will skip on the next incremental cycle. Mitigations:
// (a) the next upstream change will bump `updated` past the cursor and
// reconcile naturally; (b) full-mode cycles carry the reconcile-all
// marker (withReconcileAll, set in syncCycle) which relaxes this
// predicate to `>=` so the cycle reconciles completely, healing rows
// the sync mutated locally without bumping `updated` (orphan-filter FK
// nulls) and backfilling newly added _fold columns. Incremental cycles
// deliberately use strict `>`: `>=` would defeat the optimization
// entirely, since upstream re-sends the rows of the cursor's own second
// (see GetMaxUpdated) and every refetch produces excluded.updated >=
// existing.updated. The bounded same-second-drift risk is the trade.
//
// Full mode keeps the gate against OLDER rows. Upstream serves the
// full-mode bare list from its API cache, which can be hours or days
// stale. Without the gate, a full cycle rewrote rows that incremental
// cycles had already brought up to date with their stale versions,
// `updated` included, and the MAX(updated) cursor was already past them,
// so no later ?since= fetch returned them.
//
// The gate must not keep every newer stored row, though: a raw save of
// an old version moves `updated` backwards. The IX-F import-log rollback
// is not one: it reverts a row through django-reversion under
// create_revision, and handle_version then saves the row again with a
// new `updated`. A raw save outside a revision still can, and only a
// full cycle can repair such a row, so full mode adds the term
//
//	OR <table>.updated < <cutoff>
//
// where cutoff is the newest `updated` in the table's full snapshot. A
// stored version older than the cutoff predates the snapshot query, so
// the snapshot's version is current even when its `updated` is older.
// A stored version at or after the cutoff can be newer than the snapshot
// and is kept. Upstream builds its cache with updated__lte=<build
// start>, so the cutoff is at or before the query.
//
// Implementation note: ent's UpdateWhere predicate is emitted with a
// table qualifier active on the Builder. Calling b.Ident("foo")
// produces `<table>.foo` — so when we want to reference a column on
// the own table we just call Ident("updated") and let the qualifier
// do its thing. For the `excluded.updated` reference (the pseudo-table
// SQLite exposes inside ON CONFLICT DO UPDATE) we emit the literal
// text since "excluded" is a SQL keyword in that context, not a real
// table. The cutoff is bound as a time.Time argument, which the driver
// stores in the same text form as the column. t selects the cutoff and
// the compared columns.
func skipUnchangedPredicate(ctx context.Context, t *schema.Table) *sql.Predicate {
	return sql.P(func(b *sql.Builder) {
		writeSkipUnchanged(ctx, b, t)
	})
}

// writeSkipUnchanged writes the terms of skipUnchangedPredicate to b.
// b must carry the upsert table t as its qualifier.
func writeSkipUnchanged(ctx context.Context, b *sql.Builder, t *schema.Table) {
	cutoffs, full := reconcileAll(ctx)
	if !full {
		writeUpdatedGate(b, " > ", time.Time{}, false)
		return
	}
	// Full-mode cycle: rows with an equal updated pass the gate too,
	// and only a row that differs is written.
	cutoff, hasCutoff := cutoffs[t.Name]
	b.WriteString("(")
	writeUpdatedGate(b, " >= ", cutoff, hasCutoff)
	b.WriteString(") AND (")
	writeRowDiffers(b, t)
	b.WriteString(")")
}

// writeUpdatedGate writes the updated terms of skipUnchangedPredicate,
// without enclosing parentheses. cmp compares excluded.updated with the
// stored value.
func writeUpdatedGate(b *sql.Builder, cmp string, cutoff time.Time, hasCutoff bool) {
	b.WriteString("excluded.updated" + cmp)
	b.Ident("updated")
	if hasCutoff {
		b.WriteString(" OR ")
		b.Ident("updated")
		b.WriteString(" < ")
		b.Arg(cutoff.UTC())
	}
	b.WriteString(" OR ")
	b.Ident("updated")
	b.WriteString(" IS NULL OR ")
	b.Ident("updated")
	b.WriteString(" <= '1900-01-01'")
}

// netIxLanUpsertPredicate is the ON CONFLICT DO UPDATE WHERE clause of
// the netixlan upsert. It adds a tombstone term to skipUnchangedPredicate:
//
//	(<skipUnchanged>) AND (<table>.status <> 'deleted'
//	  OR excluded.status = 'deleted'
//	  OR excluded.updated <> <table>.updated
//	  OR <table>.updated IS NULL)
//
// A stored tombstone is not rewritten by a live row that carries the same
// updated. Upstream changes updated on every real undelete: pdb_undelete
// saves the row, and an IX-F import-log rollback runs under
// create_revision, so handle_version saves the row again with a new
// updated. Only a stale full-mode bare list sends the live version of a
// tombstone with an equal updated. A revival with a newer updated, the
// cutoff repair of an older stored row, and a tombstone-to-tombstone
// rewrite still pass. Incremental mode already refuses the pair through
// its strict `>`.
//
// Known limitation: the term cannot tell a stale re-list from a delete
// and an undelete upstream in the same second, where the mirror stored
// the delete in between. The mirror stores whole seconds only, so such a
// row stays deleted until upstream changes it again.
//
// The predicate writes its own parentheses. sql.And wraps only a
// predicate with more than one fn, so it would emit `a OR b AND c`, and
// b.Wrap starts a builder without the table qualifier.
func netIxLanUpsertPredicate(ctx context.Context) *sql.Predicate {
	return sql.P(func(b *sql.Builder) {
		b.WriteString("(")
		writeSkipUnchanged(ctx, b, migrate.NetworkIxLansTable)
		b.WriteString(") AND (")
		b.Ident("status")
		b.WriteString(" <> 'deleted' OR excluded.status = 'deleted' OR excluded.updated <> ")
		b.Ident("updated")
		b.WriteString(" OR ")
		b.Ident("updated")
		b.WriteString(" IS NULL)")
	})
}

// resolveWithRow is the ON CONFLICT DO UPDATE action of the entity
// upserts. It sets every column of t outside the primary key from the
// excluded row, so a conflicting row ends up the same as a fresh insert.
//
// ent's ResolveWithNewValues sets only the columns of the INSERT. A bulk
// INSERT lists a column when at least one row of the batch sets it, and
// SetNillable* leaves a nil value unset. So when every row of a batch
// had a nil value, the stored value stayed: a value that upstream
// cleared (a removed ipaddr6, a facility that left its campus) was kept,
// most often in the small batches of incremental cycles. SQLite fills a
// column that the INSERT does not list with its default in the excluded
// row, which is NULL for each such column.
func resolveWithRow(t *schema.Table) sql.ConflictOption {
	return sql.ResolveWith(func(u *sql.UpdateSet) {
		for _, c := range nonKeyColumns(t) {
			u.SetExcluded(c.Name)
		}
	})
}

// writeRowDiffers writes a term that is true when a column that
// resolveWithRow sets differs between the excluded row and the stored
// row:
//
//	excluded.c1 IS NOT <table>.c1 OR excluded.c2 IS NOT <table>.c2 ...
//
// Full mode needs it. The full-mode gate passes rows with an equal
// updated, and every stored row older than the cutoff, so it passes
// nearly every row of the snapshot. SQLite does not rewrite a table cell
// that is unchanged, but it deletes and inserts again the index entries
// of every column in the SET list. So an unchanged row still wrote the
// index pages of its table. A full re-upsert of 20000 unchanged netixlan
// rows wrote 4.3 MiB to the WAL, the size of the table's indexes (SQLite
// 3.53.4). The indexes were 53 MiB of the 121 MiB database on 2026-09-24.
// Each replica applies a commit with all WAL locks held, and readers that
// wait more than about 10s fail with SQLITE_PROTOCOL.
//
// IS NOT is true when one side is NULL and the other is not. The
// excluded row and the stored row come from the same Go values, so an
// unchanged column has the same stored type and bytes.
func writeRowDiffers(b *sql.Builder, t *schema.Table) {
	for i, c := range nonKeyColumns(t) {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteString("excluded." + b.Quote(c.Name) + " IS NOT ")
		b.Ident(c.Name)
	}
}

// nonKeyColumns returns the columns of t outside its primary key.
func nonKeyColumns(t *schema.Table) []*schema.Column {
	cols := make([]*schema.Column, 0, len(t.Columns))
	for _, c := range t.Columns {
		if !slices.Contains(t.PrimaryKey, c) {
			cols = append(cols, c)
		}
	}
	return cols
}

// reconcileAllKey marks a full-mode sync cycle. Its upserts also rewrite
// conflicting rows whose updated equals the stored value, and rows older
// than the table's snapshot cutoff, when a column differs (see
// skipUnchangedPredicate). Carried
// on the cycle context (cycle-scoped data, set once in syncCycle for
// full-mode runs) so the marker reaches the 13 upsert closures without
// widening every signature in the dispatch chain.
type reconcileAllKey struct{}

// withReconcileAll returns ctx marked as a full-mode cycle. cutoffs maps
// each entity table to the newest updated in its full snapshot; a table
// without an entry gets no cutoff term.
func withReconcileAll(ctx context.Context, cutoffs map[string]time.Time) context.Context {
	if cutoffs == nil {
		cutoffs = map[string]time.Time{}
	}
	return context.WithValue(ctx, reconcileAllKey{}, cutoffs)
}

// reconcileAll returns the snapshot cutoffs of a full-mode cycle and
// reports whether ctx carries the full-mode marker.
func reconcileAll(ctx context.Context) (map[string]time.Time, bool) {
	cutoffs, ok := ctx.Value(reconcileAllKey{}).(map[string]time.Time)
	return cutoffs, ok
}

// batchSize is the maximum number of rows in one upsert statement.
//
// The cost to bind the parameters of a statement increases as the square
// of the parameter count. For each parameter, the modernc.org/sqlite
// driver (v1.59.0) searches the argument list for the parameter's
// ordinal. The SQLite parse of each statement occurs one time for each
// statement, so a small batch has more of that cost. Upsert time of a
// second full sync over a populated file DB, at production row counts
// and production pragmas, with one statement for each scratch chunk:
//
//	25 rows  → 8.8 s
//	50 rows  → 8.5 s
//	100 rows → 10.6 s
//	250 rows → 18.0 s
//	500 rows → 30.6 s
//
// With 100-row chunks and 50 rows per statement, the upsert step took
// 8.3-8.4 s, against 10.6-10.9 s with 100 rows per statement.
//
// Memory use did not change with the size (see scratchChunkSize). Without
// a limit, the first org statement failed with "too many SQL variables":
// SQLite accepts at most 32766 parameters in one statement.
const batchSize = 50

// upsertBatch splits items into batches of batchSize, creates a builder for
// each item via buildFn, and executes saveFn for each batch. Returns collected
// IDs from idFn applied to each input item.
func upsertBatch[Item any, Builder any](
	ctx context.Context,
	items []Item,
	idFn func(Item) int,
	buildFn func(Item) Builder,
	saveFn func(context.Context, []Builder) error,
	entityName string,
) ([]int, error) {
	ids := make([]int, 0, len(items))
	builders := make([]Builder, 0, len(items))
	for _, item := range items {
		ids = append(ids, idFn(item))
		builders = append(builders, buildFn(item))
	}
	for i := 0; i < len(builders); i += batchSize {
		end := min(i+batchSize, len(builders))
		if err := saveFn(ctx, builders[i:end]); err != nil {
			return nil, fmt.Errorf("upsert %s batch %d: %w", entityName, i/batchSize, err)
		}
	}
	return ids, nil
}

// convertSocialMedia converts PeeringDB social media structs to ent schema types.
func convertSocialMedia(sm []peeringdb.SocialMedia) []schematypes.SocialMedia {
	if sm == nil {
		return nil
	}
	result := make([]schematypes.SocialMedia, len(sm))
	for i, s := range sm {
		result[i] = schematypes.SocialMedia{
			Service:    s.Service,
			Identifier: s.Identifier,
		}
	}
	return result
}

// upsertOrganizations bulk upserts organizations into the database.
func upsertOrganizations(ctx context.Context, tx *ent.Tx, orgs []peeringdb.Organization) ([]int, error) {
	return upsertBatch(ctx, orgs,
		func(o peeringdb.Organization) int { return o.ID },
		func(o peeringdb.Organization) *ent.OrganizationCreate {
			b := tx.Organization.Create().
				SetID(o.ID).
				SetName(o.Name).
				SetAka(o.Aka).
				SetNameLong(o.NameLong).
				SetWebsite(o.Website).
				SetSocialMedia(convertSocialMedia(o.SocialMedia)).
				SetNotes(o.Notes).
				SetAddress1(o.Address1).
				SetAddress2(o.Address2).
				SetCity(o.City).
				SetState(o.State).
				SetCountry(o.Country).
				SetZipcode(o.Zipcode).
				SetSuite(o.Suite).
				SetFloor(o.Floor).
				SetCreated(o.Created).
				SetUpdated(o.Updated).
				SetStatus(o.Status).
				SetNameFold(unifold.Fold(o.Name)).
				SetAkaFold(unifold.Fold(o.Aka)).
				SetCityFold(unifold.Fold(o.City))
			b.SetNillableLogo(o.Logo)
			b.SetNillableLatitude(o.Latitude)
			b.SetNillableLongitude(o.Longitude)
			return b
		},
		func(ctx context.Context, batch []*ent.OrganizationCreate) error {
			return tx.Organization.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(organization.FieldID),
					resolveWithRow(migrate.OrganizationsTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.OrganizationsTable)),
				).
				Exec(ctx)
		},
		"organizations",
	)
}

// upsertCampuses bulk upserts campuses into the database.
func upsertCampuses(ctx context.Context, tx *ent.Tx, items []peeringdb.Campus) ([]int, error) {
	return upsertBatch(ctx, items,
		func(c peeringdb.Campus) int { return c.ID },
		func(c peeringdb.Campus) *ent.CampusCreate {
			b := tx.Campus.Create().
				SetID(c.ID).
				SetNillableOrgID(&c.OrgID).
				SetOrgName(c.OrgName).
				SetName(c.Name).
				SetNillableNameLong(c.NameLong).
				SetNillableAka(c.Aka).
				SetWebsite(c.Website).
				SetSocialMedia(convertSocialMedia(c.SocialMedia)).
				SetNotes(c.Notes).
				SetCountry(c.Country).
				SetCity(c.City).
				SetZipcode(c.Zipcode).
				SetState(c.State).
				SetCreated(c.Created).
				SetUpdated(c.Updated).
				SetStatus(c.Status).
				SetNameFold(unifold.Fold(c.Name))
			b.SetNillableLogo(c.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.CampusCreate) error {
			return tx.Campus.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(campus.FieldID),
					resolveWithRow(migrate.CampusesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.CampusesTable)),
				).
				Exec(ctx)
		},
		"campuses",
	)
}

// upsertFacilities bulk upserts facilities into the database.
func upsertFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.Facility) ([]int, error) {
	return upsertBatch(ctx, items,
		func(f peeringdb.Facility) int { return f.ID },
		func(f peeringdb.Facility) *ent.FacilityCreate {
			b := tx.Facility.Create().
				SetID(f.ID).
				SetNillableOrgID(&f.OrgID).
				SetOrgName(f.OrgName).
				SetNillableCampusID(f.CampusID).
				SetName(f.Name).
				SetAka(f.Aka).
				SetNameLong(f.NameLong).
				SetWebsite(f.Website).
				SetSocialMedia(convertSocialMedia(f.SocialMedia)).
				SetClli(f.CLLI).
				SetRencode(f.Rencode).
				SetNpanxx(f.NPANXX).
				SetTechEmail(f.TechEmail).
				SetTechPhone(f.TechPhone).
				SetSalesEmail(f.SalesEmail).
				SetSalesPhone(f.SalesPhone).
				SetNillableProperty(f.Property).
				SetNillableDiverseServingSubstations(f.DiverseServingSubstations).
				SetAvailableVoltageServices(f.AvailableVoltageServices).
				SetNotes(f.Notes).
				SetNillableRegionContinent(f.RegionContinent).
				SetNillableStatusDashboard(f.StatusDashboard).
				SetNetCount(f.NetCount).
				SetIxCount(f.IXCount).
				SetCarrierCount(f.CarrierCount).
				SetAddress1(f.Address1).
				SetAddress2(f.Address2).
				SetCity(f.City).
				SetState(f.State).
				SetCountry(f.Country).
				SetZipcode(f.Zipcode).
				SetSuite(f.Suite).
				SetFloor(f.Floor).
				SetCreated(f.Created).
				SetUpdated(f.Updated).
				SetStatus(f.Status).
				SetNameFold(unifold.Fold(f.Name)).
				SetAkaFold(unifold.Fold(f.Aka)).
				SetCityFold(unifold.Fold(f.City))
			b.SetNillableLogo(f.Logo)
			b.SetNillableLatitude(f.Latitude)
			b.SetNillableLongitude(f.Longitude)
			return b
		},
		func(ctx context.Context, batch []*ent.FacilityCreate) error {
			return tx.Facility.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(facility.FieldID),
					resolveWithRow(migrate.FacilitiesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.FacilitiesTable)),
				).
				Exec(ctx)
		},
		"facilities",
	)
}

// upsertCarriers bulk upserts carriers into the database.
func upsertCarriers(ctx context.Context, tx *ent.Tx, items []peeringdb.Carrier) ([]int, error) {
	return upsertBatch(ctx, items,
		func(c peeringdb.Carrier) int { return c.ID },
		func(c peeringdb.Carrier) *ent.CarrierCreate {
			b := tx.Carrier.Create().
				SetID(c.ID).
				SetNillableOrgID(&c.OrgID).
				SetOrgName(c.OrgName).
				SetName(c.Name).
				SetAka(c.Aka).
				SetNameLong(c.NameLong).
				SetWebsite(c.Website).
				SetSocialMedia(convertSocialMedia(c.SocialMedia)).
				SetNotes(c.Notes).
				SetFacCount(c.FacCount).
				SetCreated(c.Created).
				SetUpdated(c.Updated).
				SetStatus(c.Status).
				SetNameFold(unifold.Fold(c.Name)).
				SetAkaFold(unifold.Fold(c.Aka))
			b.SetNillableLogo(c.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.CarrierCreate) error {
			return tx.Carrier.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(carrier.FieldID),
					resolveWithRow(migrate.CarriersTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.CarriersTable)),
				).
				Exec(ctx)
		},
		"carriers",
	)
}

// upsertCarrierFacilities bulk upserts carrier-facility associations.
func upsertCarrierFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.CarrierFacility) ([]int, error) {
	return upsertBatch(ctx, items,
		func(cf peeringdb.CarrierFacility) int { return cf.ID },
		func(cf peeringdb.CarrierFacility) *ent.CarrierFacilityCreate {
			return tx.CarrierFacility.Create().
				SetID(cf.ID).
				SetNillableCarrierID(&cf.CarrierID).
				SetNillableFacID(&cf.FacID).
				SetName(cf.Name).
				SetCreated(cf.Created).
				SetUpdated(cf.Updated).
				SetStatus(cf.Status)
		},
		func(ctx context.Context, batch []*ent.CarrierFacilityCreate) error {
			return tx.CarrierFacility.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(carrierfacility.FieldID),
					resolveWithRow(migrate.CarrierFacilitiesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.CarrierFacilitiesTable)),
				).
				Exec(ctx)
		},
		"carrier facilities",
	)
}

// upsertInternetExchanges bulk upserts internet exchanges.
func upsertInternetExchanges(ctx context.Context, tx *ent.Tx, items []peeringdb.InternetExchange) ([]int, error) {
	return upsertBatch(ctx, items,
		func(ix peeringdb.InternetExchange) int { return ix.ID },
		func(ix peeringdb.InternetExchange) *ent.InternetExchangeCreate {
			b := tx.InternetExchange.Create().
				SetID(ix.ID).
				SetNillableOrgID(&ix.OrgID).
				SetName(ix.Name).
				SetAka(ix.Aka).
				SetNameLong(ix.NameLong).
				SetCity(ix.City).
				SetCountry(ix.Country).
				SetRegionContinent(ix.RegionContinent).
				SetMedia(ix.Media).
				SetNotes(ix.Notes).
				SetProtoUnicast(ix.ProtoUnicast).
				SetProtoMulticast(ix.ProtoMulticast).
				SetProtoIpv6(ix.ProtoIPv6).
				SetWebsite(ix.Website).
				SetSocialMedia(convertSocialMedia(ix.SocialMedia)).
				SetURLStats(ix.URLStats).
				SetTechEmail(ix.TechEmail).
				SetTechPhone(ix.TechPhone).
				SetPolicyEmail(ix.PolicyEmail).
				SetPolicyPhone(ix.PolicyPhone).
				SetSalesEmail(ix.SalesEmail).
				SetSalesPhone(ix.SalesPhone).
				SetNetCount(ix.NetCount).
				SetFacCount(ix.FacCount).
				SetIxfNetCount(ix.IXFNetCount).
				SetNillableIxfLastImport(ix.IXFLastImport).
				SetNillableIxfImportRequest(ix.IXFImportRequest).
				SetIxfImportRequestStatus(ix.IXFImportRequestStatus).
				SetServiceLevel(ix.ServiceLevel).
				SetTerms(ix.Terms).
				SetNillableStatusDashboard(ix.StatusDashboard).
				SetCreated(ix.Created).
				SetUpdated(ix.Updated).
				SetStatus(ix.Status).
				SetNameFold(unifold.Fold(ix.Name)).
				SetAkaFold(unifold.Fold(ix.Aka)).
				SetNameLongFold(unifold.Fold(ix.NameLong)).
				SetCityFold(unifold.Fold(ix.City))
			b.SetNillableLogo(ix.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.InternetExchangeCreate) error {
			return tx.InternetExchange.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(internetexchange.FieldID),
					resolveWithRow(migrate.InternetExchangesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.InternetExchangesTable)),
				).
				Exec(ctx)
		},
		"internet exchanges",
	)
}

// upsertIxLans bulk upserts IX LANs.
func upsertIxLans(ctx context.Context, tx *ent.Tx, items []peeringdb.IxLan) ([]int, error) {
	return upsertBatch(ctx, items,
		func(il peeringdb.IxLan) int { return il.ID },
		func(il peeringdb.IxLan) *ent.IxLanCreate {
			return tx.IxLan.Create().
				SetID(il.ID).
				SetNillableIxID(&il.IXID).
				SetName(il.Name).
				SetDescr(il.Descr).
				SetMtu(il.MTU).
				SetDot1qSupport(il.Dot1QSupport).
				SetNillableRsAsn(il.RSASN).
				SetNillableArpSponge(il.ARPSponge).
				SetIxfIxpMemberListURL(il.IXFIXPMemberListURL). // auth-gated field
				SetIxfIxpMemberListURLVisible(il.IXFIXPMemberListURLVisible).
				SetIxfIxpImportEnabled(il.IXFIXPImportEnabled).
				SetCreated(il.Created).
				SetUpdated(il.Updated).
				SetStatus(il.Status)
		},
		func(ctx context.Context, batch []*ent.IxLanCreate) error {
			return tx.IxLan.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(ixlan.FieldID),
					resolveWithRow(migrate.IxLansTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.IxLansTable)),
				).
				Exec(ctx)
		},
		"ix lans",
	)
}

// upsertIxPrefixes bulk upserts IX prefixes.
func upsertIxPrefixes(ctx context.Context, tx *ent.Tx, items []peeringdb.IxPrefix) ([]int, error) {
	return upsertBatch(ctx, items,
		func(ip peeringdb.IxPrefix) int { return ip.ID },
		func(ip peeringdb.IxPrefix) *ent.IxPrefixCreate {
			return tx.IxPrefix.Create().
				SetID(ip.ID).
				SetNillableIxlanID(&ip.IXLanID).
				SetProtocol(ip.Protocol).
				SetPrefix(ip.Prefix).
				SetInDfz(ip.InDFZ).
				SetCreated(ip.Created).
				SetUpdated(ip.Updated).
				SetStatus(ip.Status)
		},
		func(ctx context.Context, batch []*ent.IxPrefixCreate) error {
			return tx.IxPrefix.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(ixprefix.FieldID),
					resolveWithRow(migrate.IxPrefixesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.IxPrefixesTable)),
				).
				Exec(ctx)
		},
		"ix prefixes",
	)
}

// upsertIxFacilities bulk upserts IX-facility associations.
func upsertIxFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.IxFacility) ([]int, error) {
	return upsertBatch(ctx, items,
		func(ixf peeringdb.IxFacility) int { return ixf.ID },
		func(ixf peeringdb.IxFacility) *ent.IxFacilityCreate {
			return tx.IxFacility.Create().
				SetID(ixf.ID).
				SetNillableIxID(&ixf.IXID).
				SetNillableFacID(&ixf.FacID).
				SetName(ixf.Name).
				SetCity(ixf.City).
				SetCountry(ixf.Country).
				SetCreated(ixf.Created).
				SetUpdated(ixf.Updated).
				SetStatus(ixf.Status)
		},
		func(ctx context.Context, batch []*ent.IxFacilityCreate) error {
			return tx.IxFacility.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(ixfacility.FieldID),
					resolveWithRow(migrate.IxFacilitiesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.IxFacilitiesTable)),
				).
				Exec(ctx)
		},
		"ix facilities",
	)
}

// upsertNetworks bulk upserts networks into the database.
func upsertNetworks(ctx context.Context, tx *ent.Tx, items []peeringdb.Network) ([]int, error) {
	return upsertBatch(ctx, items,
		func(n peeringdb.Network) int { return n.ID },
		func(n peeringdb.Network) *ent.NetworkCreate {
			b := tx.Network.Create().
				SetID(n.ID).
				SetNillableOrgID(&n.OrgID).
				SetName(n.Name).
				SetAka(n.Aka).
				SetNameLong(n.NameLong).
				SetWebsite(n.Website).
				SetSocialMedia(convertSocialMedia(n.SocialMedia)).
				SetAsn(n.ASN).
				SetLookingGlass(n.LookingGlass).
				SetRouteServer(n.RouteServer).
				SetIrrAsSet(n.IRRASSet).
				SetInfoType(n.InfoType).
				SetInfoTypes(n.InfoTypes).
				SetNillableInfoPrefixes4(n.InfoPrefixes4).
				SetNillableInfoPrefixes6(n.InfoPrefixes6).
				SetInfoTraffic(n.InfoTraffic).
				SetInfoRatio(n.InfoRatio).
				SetInfoScope(n.InfoScope).
				SetInfoUnicast(n.InfoUnicast).
				SetInfoMulticast(n.InfoMulticast).
				SetInfoIpv6(n.InfoIPv6).
				SetInfoNeverViaRouteServers(n.InfoNeverViaRouteServer).
				SetNotes(n.Notes).
				SetPolicyURL(n.PolicyURL).
				SetPolicyGeneral(n.PolicyGeneral).
				SetPolicyLocations(n.PolicyLocations).
				SetPolicyRatio(n.PolicyRatio).
				SetPolicyContracts(n.PolicyContracts).
				SetAllowIxpUpdate(n.AllowIXPUpdate).
				SetIxpUpdateExclude(n.IxpUpdateExclude).
				SetNillableStatusDashboard(n.StatusDashboard).
				SetNillableRirStatus(n.RIRStatus).
				SetNillableRirStatusUpdated(n.RIRStatusUpdated).
				SetIxCount(n.IXCount).
				SetFacCount(n.FacCount).
				SetNillableNetixlanUpdated(n.NetIXLanUpdated).
				SetNillableNetfacUpdated(n.NetFacUpdated).
				SetNillablePocUpdated(n.PocUpdated).
				SetCreated(n.Created).
				SetUpdated(n.Updated).
				SetStatus(n.Status).
				SetNameFold(unifold.Fold(n.Name)).
				SetAkaFold(unifold.Fold(n.Aka)).
				SetNameLongFold(unifold.Fold(n.NameLong))
			b.SetNillableLogo(n.Logo)
			b.SetMeta(n.Meta)
			return b
		},
		func(ctx context.Context, batch []*ent.NetworkCreate) error {
			return tx.Network.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(network.FieldID),
					resolveWithRow(migrate.NetworksTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.NetworksTable)),
				).
				Exec(ctx)
		},
		"networks",
	)
}

// upsertPocs bulk upserts points of contact. A deleted contact is stored
// with name, phone, email and url blanked (peeringdb.Poc.BlankDeletedContact),
// whatever upstream sends, so no stored tombstone holds contact data and
// no read path depends on a render-time rule to hide it.
func upsertPocs(ctx context.Context, tx *ent.Tx, items []peeringdb.Poc) ([]int, error) {
	return upsertBatch(ctx, items,
		func(p peeringdb.Poc) int { return p.ID },
		func(p peeringdb.Poc) *ent.PocCreate {
			p = p.BlankDeletedContact()
			return tx.Poc.Create().
				SetID(p.ID).
				SetNillableNetID(&p.NetID).
				SetRole(p.Role).
				SetVisible(p.Visible).
				SetName(p.Name).
				SetPhone(p.Phone).
				SetEmail(p.Email).
				SetURL(p.URL).
				SetCreated(p.Created).
				SetUpdated(p.Updated).
				SetStatus(p.Status)
		},
		func(ctx context.Context, batch []*ent.PocCreate) error {
			return tx.Poc.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(poc.FieldID),
					resolveWithRow(migrate.PocsTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.PocsTable)),
				).
				Exec(ctx)
		},
		"pocs",
	)
}

// upsertNetworkFacilities bulk upserts network-facility associations.
func upsertNetworkFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.NetworkFacility) ([]int, error) {
	return upsertBatch(ctx, items,
		func(nf peeringdb.NetworkFacility) int { return nf.ID },
		func(nf peeringdb.NetworkFacility) *ent.NetworkFacilityCreate {
			return tx.NetworkFacility.Create().
				SetID(nf.ID).
				SetNillableNetID(&nf.NetID).
				SetNillableFacID(&nf.FacID).
				SetName(nf.Name).
				SetCity(nf.City).
				SetCountry(nf.Country).
				SetLocalAsn(nf.LocalASN).
				SetCreated(nf.Created).
				SetUpdated(nf.Updated).
				SetStatus(nf.Status)
		},
		func(ctx context.Context, batch []*ent.NetworkFacilityCreate) error {
			return tx.NetworkFacility.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(networkfacility.FieldID),
					resolveWithRow(migrate.NetworkFacilitiesTable),
					sql.UpdateWhere(skipUnchangedPredicate(ctx, migrate.NetworkFacilitiesTable)),
				).
				Exec(ctx)
		},
		"network facilities",
	)
}

// upsertSingleRaw decodes a single raw PeeringDB JSON object into the
// per-type Go struct and reuses the bulk upsert helper to land it. The
// ON CONFLICT action of the bulk helper (resolveWithRow) handles re-insert
// idempotently — a backfilled row that arrives during a normal Phase B
// upsert will be overwritten by the bulk path without conflict.
//
// The FK-backfill path in fk_backfill.go calls
// this once per missing parent ID. The single-element slice avoids any
// need for separate insert paths and keeps the per-type logic
// (validators, fold setters, nullable wiring) centralised in the bulk
// upsert closures.
//
// fkBackfillBatch must stay the only caller. It records each landed id
// in Worker.fkBackfilled, and writeSyncWatermarks leaves those rows out
// of the next watermark (see watermark.go). A row that another caller
// lands could move the cursor of its type past rows that the cycle never
// fetched. TestUpsertSingleRaw_SingleCaller locks this.
//
// Returns the inserted ID (per upstream contract — the parent we just
// landed) so callers can fkRegisterIDs without a separate query, or 0
// + error on dispatch / decode / upsert failure.
func upsertSingleRaw(ctx context.Context, tx *ent.Tx, parentType string, raw json.RawMessage) (int, error) {
	desc, ok := descriptorByName[parentType]
	if !ok {
		return 0, fmt.Errorf("upsertSingleRaw: unknown parent type %q", parentType)
	}
	return desc.singleUpsert(ctx, tx, raw)
}

// decodeAndUpsertSingle is the shared decode-and-bulk-upsert helper for
// upsertSingleRaw. The generic parameter E carries the per-type Go
// struct; the closures provide id extraction and the bulk upsert call.
// Returns the row's ID (extracted via idFn) or 0 on error.
func decodeAndUpsertSingle[E any](
	ctx context.Context,
	tx *ent.Tx,
	parentType string,
	raw json.RawMessage,
	idFn func(E) int,
	upsert func(context.Context, *ent.Tx, []E) ([]int, error),
) (int, error) {
	var v E
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, fmt.Errorf("decode %s: %w", parentType, err)
	}
	if _, err := upsert(ctx, tx, []E{v}); err != nil {
		return 0, fmt.Errorf("upsert %s: %w", parentType, err)
	}
	return idFn(v), nil
}

// netixlanOperational returns the operational flag to store for ni.
// PeeringDB 2.83.0 derives operational from status on every save
// (models.py:6512) and keeps the field only for a deprecation window.
// When upstream sends the key, store it as sent. When it omits the key,
// derive the flag the same way upstream does.
func netixlanOperational(ni peeringdb.NetworkIxLan) bool {
	if ni.Operational != nil {
		return *ni.Operational
	}
	return ni.Status == "ok"
}

// upsertNetworkIxLans bulk upserts network-IXLan associations.
func upsertNetworkIxLans(ctx context.Context, tx *ent.Tx, items []peeringdb.NetworkIxLan) ([]int, error) {
	return upsertBatch(ctx, items,
		func(ni peeringdb.NetworkIxLan) int { return ni.ID },
		func(ni peeringdb.NetworkIxLan) *ent.NetworkIxLanCreate {
			return tx.NetworkIxLan.Create().
				SetID(ni.ID).
				SetNillableNetID(&ni.NetID).
				SetIxID(ni.IXID).
				SetNillableIxlanID(&ni.IXLanID).
				SetName(ni.Name).
				SetNotes(ni.Notes).
				SetSpeed(ni.Speed).
				SetAsn(ni.ASN).
				SetNillableIpaddr4(ni.IPAddr4).
				SetNillableIpaddr6(ni.IPAddr6).
				SetIsRsPeer(ni.IsRSPeer).
				SetBfdSupport(ni.BFDSupport).
				SetOperational(netixlanOperational(ni)).
				SetNillableNetSideID(ni.NetSideID).
				SetNillableIxSideID(ni.IXSideID).
				SetMeta(ni.Meta).
				SetCreated(ni.Created).
				SetUpdated(ni.Updated).
				SetStatus(ni.Status)
		},
		func(ctx context.Context, batch []*ent.NetworkIxLanCreate) error {
			return tx.NetworkIxLan.CreateBulk(batch...).
				OnConflict(
					sql.ConflictColumns(networkixlan.FieldID),
					resolveWithRow(migrate.NetworkIxLansTable),
					sql.UpdateWhere(netIxLanUpsertPredicate(ctx)),
				).
				Exec(ctx)
		},
		"network ix lans",
	)
}
