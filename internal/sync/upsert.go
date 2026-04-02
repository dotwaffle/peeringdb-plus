// Package sync orchestrates data synchronization from PeeringDB into the
// local SQLite database using the ent ORM.
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
	entschema "github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// batchSize limits the number of builders per bulk upsert to stay within
// SQLite's variable count limit.
const batchSize = 500

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
func convertSocialMedia(sm []peeringdb.SocialMedia) []entschema.SocialMedia {
	if sm == nil {
		return nil
	}
	result := make([]entschema.SocialMedia, len(sm))
	for i, s := range sm {
		result[i] = entschema.SocialMedia{
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
				SetStatus(o.Status)
			b.SetNillableLogo(o.Logo)
			b.SetNillableLatitude(o.Latitude)
			b.SetNillableLongitude(o.Longitude)
			return b
		},
		func(ctx context.Context, batch []*ent.OrganizationCreate) error {
			return tx.Organization.CreateBulk(batch...).
				OnConflictColumns(organization.FieldID).
				UpdateNewValues().
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
				SetStatus(c.Status)
			b.SetNillableLogo(c.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.CampusCreate) error {
			return tx.Campus.CreateBulk(batch...).
				OnConflictColumns(campus.FieldID).
				UpdateNewValues().
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
				SetStatus(f.Status)
			b.SetNillableLogo(f.Logo)
			b.SetNillableLatitude(f.Latitude)
			b.SetNillableLongitude(f.Longitude)
			return b
		},
		func(ctx context.Context, batch []*ent.FacilityCreate) error {
			return tx.Facility.CreateBulk(batch...).
				OnConflictColumns(facility.FieldID).
				UpdateNewValues().
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
				SetStatus(c.Status)
			b.SetNillableLogo(c.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.CarrierCreate) error {
			return tx.Carrier.CreateBulk(batch...).
				OnConflictColumns(carrier.FieldID).
				UpdateNewValues().
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
				OnConflictColumns(carrierfacility.FieldID).
				UpdateNewValues().
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
				SetStatus(ix.Status)
			b.SetNillableLogo(ix.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.InternetExchangeCreate) error {
			return tx.InternetExchange.CreateBulk(batch...).
				OnConflictColumns(internetexchange.FieldID).
				UpdateNewValues().
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
				SetIxfIxpMemberListURLVisible(il.IXFIXPMemberListURLVisible).
				SetIxfIxpImportEnabled(il.IXFIXPImportEnabled).
				SetCreated(il.Created).
				SetUpdated(il.Updated).
				SetStatus(il.Status)
		},
		func(ctx context.Context, batch []*ent.IxLanCreate) error {
			return tx.IxLan.CreateBulk(batch...).
				OnConflictColumns(ixlan.FieldID).
				UpdateNewValues().
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
				SetNotes(ip.Notes).
				SetCreated(ip.Created).
				SetUpdated(ip.Updated).
				SetStatus(ip.Status)
		},
		func(ctx context.Context, batch []*ent.IxPrefixCreate) error {
			return tx.IxPrefix.CreateBulk(batch...).
				OnConflictColumns(ixprefix.FieldID).
				UpdateNewValues().
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
				OnConflictColumns(ixfacility.FieldID).
				UpdateNewValues().
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
				SetStatus(n.Status)
			b.SetNillableLogo(n.Logo)
			return b
		},
		func(ctx context.Context, batch []*ent.NetworkCreate) error {
			return tx.Network.CreateBulk(batch...).
				OnConflictColumns(network.FieldID).
				UpdateNewValues().
				Exec(ctx)
		},
		"networks",
	)
}

// upsertPocs bulk upserts points of contact.
func upsertPocs(ctx context.Context, tx *ent.Tx, items []peeringdb.Poc) ([]int, error) {
	return upsertBatch(ctx, items,
		func(p peeringdb.Poc) int { return p.ID },
		func(p peeringdb.Poc) *ent.PocCreate {
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
				OnConflictColumns(poc.FieldID).
				UpdateNewValues().
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
				OnConflictColumns(networkfacility.FieldID).
				UpdateNewValues().
				Exec(ctx)
		},
		"network facilities",
	)
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
				SetOperational(ni.Operational).
				SetNillableNetSideID(ni.NetSideID).
				SetNillableIxSideID(ni.IXSideID).
				SetCreated(ni.Created).
				SetUpdated(ni.Updated).
				SetStatus(ni.Status)
		},
		func(ctx context.Context, batch []*ent.NetworkIxLanCreate) error {
			return tx.NetworkIxLan.CreateBulk(batch...).
				OnConflictColumns(networkixlan.FieldID).
				UpdateNewValues().
				Exec(ctx)
		},
		"network ix lans",
	)
}
