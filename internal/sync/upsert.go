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
	builders := make([]*ent.OrganizationCreate, 0, len(orgs))
	ids := make([]int, 0, len(orgs))
	for _, org := range orgs {
		ids = append(ids, org.ID)
		b := tx.Organization.Create().
			SetID(org.ID).
			SetName(org.Name).
			SetAka(org.Aka).
			SetNameLong(org.NameLong).
			SetWebsite(org.Website).
			SetSocialMedia(convertSocialMedia(org.SocialMedia)).
			SetNotes(org.Notes).
			SetAddress1(org.Address1).
			SetAddress2(org.Address2).
			SetCity(org.City).
			SetState(org.State).
			SetCountry(org.Country).
			SetZipcode(org.Zipcode).
			SetSuite(org.Suite).
			SetFloor(org.Floor).
			SetCreated(org.Created).
			SetUpdated(org.Updated).
			SetStatus(org.Status)
		b.SetNillableLogo(org.Logo)
		b.SetNillableLatitude(org.Latitude)
		b.SetNillableLongitude(org.Longitude)
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Organization.CreateBulk(builders[i:end]...).
			OnConflictColumns(organization.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert organizations batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertCampuses bulk upserts campuses into the database.
func upsertCampuses(ctx context.Context, tx *ent.Tx, items []peeringdb.Campus) ([]int, error) {
	builders := make([]*ent.CampusCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, c := range items {
		ids = append(ids, c.ID)
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Campus.CreateBulk(builders[i:end]...).
			OnConflictColumns(campus.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert campuses batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertFacilities bulk upserts facilities into the database.
func upsertFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.Facility) ([]int, error) {
	builders := make([]*ent.FacilityCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, f := range items {
		ids = append(ids, f.ID)
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Facility.CreateBulk(builders[i:end]...).
			OnConflictColumns(facility.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert facilities batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertCarriers bulk upserts carriers into the database.
func upsertCarriers(ctx context.Context, tx *ent.Tx, items []peeringdb.Carrier) ([]int, error) {
	builders := make([]*ent.CarrierCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, c := range items {
		ids = append(ids, c.ID)
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Carrier.CreateBulk(builders[i:end]...).
			OnConflictColumns(carrier.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert carriers batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertCarrierFacilities bulk upserts carrier-facility associations.
func upsertCarrierFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.CarrierFacility) ([]int, error) {
	builders := make([]*ent.CarrierFacilityCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, cf := range items {
		ids = append(ids, cf.ID)
		b := tx.CarrierFacility.Create().
			SetID(cf.ID).
			SetNillableCarrierID(&cf.CarrierID).
			SetNillableFacID(&cf.FacID).
			SetName(cf.Name).
			SetCreated(cf.Created).
			SetUpdated(cf.Updated).
			SetStatus(cf.Status)
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.CarrierFacility.CreateBulk(builders[i:end]...).
			OnConflictColumns(carrierfacility.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert carrier facilities batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertInternetExchanges bulk upserts internet exchanges.
func upsertInternetExchanges(ctx context.Context, tx *ent.Tx, items []peeringdb.InternetExchange) ([]int, error) {
	builders := make([]*ent.InternetExchangeCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, ix := range items {
		ids = append(ids, ix.ID)
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.InternetExchange.CreateBulk(builders[i:end]...).
			OnConflictColumns(internetexchange.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert internet exchanges batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertIxLans bulk upserts IX LANs.
func upsertIxLans(ctx context.Context, tx *ent.Tx, items []peeringdb.IxLan) ([]int, error) {
	builders := make([]*ent.IxLanCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, il := range items {
		ids = append(ids, il.ID)
		b := tx.IxLan.Create().
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.IxLan.CreateBulk(builders[i:end]...).
			OnConflictColumns(ixlan.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert ix lans batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertIxPrefixes bulk upserts IX prefixes.
func upsertIxPrefixes(ctx context.Context, tx *ent.Tx, items []peeringdb.IxPrefix) ([]int, error) {
	builders := make([]*ent.IxPrefixCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, ip := range items {
		ids = append(ids, ip.ID)
		b := tx.IxPrefix.Create().
			SetID(ip.ID).
			SetNillableIxlanID(&ip.IXLanID).
			SetProtocol(ip.Protocol).
			SetPrefix(ip.Prefix).
			SetInDfz(ip.InDFZ).
			SetNotes(ip.Notes).
			SetCreated(ip.Created).
			SetUpdated(ip.Updated).
			SetStatus(ip.Status)
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.IxPrefix.CreateBulk(builders[i:end]...).
			OnConflictColumns(ixprefix.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert ix prefixes batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertIxFacilities bulk upserts IX-facility associations.
func upsertIxFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.IxFacility) ([]int, error) {
	builders := make([]*ent.IxFacilityCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, ixf := range items {
		ids = append(ids, ixf.ID)
		b := tx.IxFacility.Create().
			SetID(ixf.ID).
			SetNillableIxID(&ixf.IXID).
			SetNillableFacID(&ixf.FacID).
			SetName(ixf.Name).
			SetCity(ixf.City).
			SetCountry(ixf.Country).
			SetCreated(ixf.Created).
			SetUpdated(ixf.Updated).
			SetStatus(ixf.Status)
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.IxFacility.CreateBulk(builders[i:end]...).
			OnConflictColumns(ixfacility.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert ix facilities batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertNetworks bulk upserts networks into the database.
func upsertNetworks(ctx context.Context, tx *ent.Tx, items []peeringdb.Network) ([]int, error) {
	builders := make([]*ent.NetworkCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, n := range items {
		ids = append(ids, n.ID)
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Network.CreateBulk(builders[i:end]...).
			OnConflictColumns(network.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert networks batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertPocs bulk upserts points of contact.
func upsertPocs(ctx context.Context, tx *ent.Tx, items []peeringdb.Poc) ([]int, error) {
	builders := make([]*ent.PocCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, p := range items {
		ids = append(ids, p.ID)
		b := tx.Poc.Create().
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.Poc.CreateBulk(builders[i:end]...).
			OnConflictColumns(poc.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert pocs batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertNetworkFacilities bulk upserts network-facility associations.
func upsertNetworkFacilities(ctx context.Context, tx *ent.Tx, items []peeringdb.NetworkFacility) ([]int, error) {
	builders := make([]*ent.NetworkFacilityCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, nf := range items {
		ids = append(ids, nf.ID)
		b := tx.NetworkFacility.Create().
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.NetworkFacility.CreateBulk(builders[i:end]...).
			OnConflictColumns(networkfacility.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert network facilities batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}

// upsertNetworkIxLans bulk upserts network-IXLan associations.
func upsertNetworkIxLans(ctx context.Context, tx *ent.Tx, items []peeringdb.NetworkIxLan) ([]int, error) {
	builders := make([]*ent.NetworkIxLanCreate, 0, len(items))
	ids := make([]int, 0, len(items))
	for _, ni := range items {
		ids = append(ids, ni.ID)
		b := tx.NetworkIxLan.Create().
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
		builders = append(builders, b)
	}
	for i := 0; i < len(builders); i += batchSize {
		end := i + batchSize
		if end > len(builders) {
			end = len(builders)
		}
		err := tx.NetworkIxLan.CreateBulk(builders[i:end]...).
			OnConflictColumns(networkixlan.FieldID).
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			return nil, fmt.Errorf("upsert network ix lans batch %d: %w", i/batchSize, err)
		}
	}
	return ids, nil
}
