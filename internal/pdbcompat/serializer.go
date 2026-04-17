package pdbcompat

import (
	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// derefInt returns the value pointed to by p, or 0 if p is nil.
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// socialMediaFromSchema converts ent schema SocialMedia to peeringdb
// SocialMedia. Both types have identical fields but are in different packages.
func socialMediaFromSchema(sm []schematypes.SocialMedia) []peeringdb.SocialMedia {
	if sm == nil {
		return []peeringdb.SocialMedia{}
	}
	out := make([]peeringdb.SocialMedia, len(sm))
	for i, s := range sm {
		out[i] = peeringdb.SocialMedia{
			Service:    s.Service,
			Identifier: s.Identifier,
		}
	}
	return out
}

// organizationFromEnt maps an ent Organization to a peeringdb Organization.
func organizationFromEnt(o *ent.Organization) peeringdb.Organization {
	return peeringdb.Organization{
		ID:          o.ID,
		Name:        o.Name,
		Aka:         o.Aka,
		NameLong:    o.NameLong,
		Website:     o.Website,
		SocialMedia: socialMediaFromSchema(o.SocialMedia),
		Notes:       o.Notes,
		Logo:        o.Logo,
		Address1:    o.Address1,
		Address2:    o.Address2,
		City:        o.City,
		State:       o.State,
		Country:     o.Country,
		Zipcode:     o.Zipcode,
		Suite:       o.Suite,
		Floor:       o.Floor,
		Latitude:    o.Latitude,
		Longitude:   o.Longitude,
		Created:     o.Created,
		Updated:     o.Updated,
		Status:      o.Status,
	}
}

// organizationsFromEnt maps a slice of ent Organizations to peeringdb
// Organizations.
func organizationsFromEnt(orgs []*ent.Organization) []peeringdb.Organization {
	out := make([]peeringdb.Organization, len(orgs))
	for i, o := range orgs {
		out[i] = organizationFromEnt(o)
	}
	return out
}

// networkFromEnt maps an ent Network to a peeringdb Network.
func networkFromEnt(n *ent.Network) peeringdb.Network {
	return peeringdb.Network{
		ID:                      n.ID,
		OrgID:                   derefInt(n.OrgID),
		Name:                    n.Name,
		Aka:                     n.Aka,
		NameLong:                n.NameLong,
		Website:                 n.Website,
		SocialMedia:             socialMediaFromSchema(n.SocialMedia),
		ASN:                     n.Asn,
		LookingGlass:            n.LookingGlass,
		RouteServer:             n.RouteServer,
		IRRASSet:                n.IrrAsSet,
		InfoType:                n.InfoType,
		InfoTypes:               n.InfoTypes,
		InfoPrefixes4:           n.InfoPrefixes4,
		InfoPrefixes6:           n.InfoPrefixes6,
		InfoTraffic:             n.InfoTraffic,
		InfoRatio:               n.InfoRatio,
		InfoScope:               n.InfoScope,
		InfoUnicast:             n.InfoUnicast,
		InfoMulticast:           n.InfoMulticast,
		InfoIPv6:                n.InfoIpv6,
		InfoNeverViaRouteServer: n.InfoNeverViaRouteServers,
		Notes:                   n.Notes,
		PolicyURL:               n.PolicyURL,
		PolicyGeneral:           n.PolicyGeneral,
		PolicyLocations:         n.PolicyLocations,
		PolicyRatio:             n.PolicyRatio,
		PolicyContracts:         n.PolicyContracts,
		AllowIXPUpdate:          n.AllowIxpUpdate,
		StatusDashboard:         n.StatusDashboard,
		RIRStatus:               n.RirStatus,
		RIRStatusUpdated:        n.RirStatusUpdated,
		Logo:                    n.Logo,
		IXCount:                 n.IxCount,
		FacCount:                n.FacCount,
		NetIXLanUpdated:         n.NetixlanUpdated,
		NetFacUpdated:           n.NetfacUpdated,
		PocUpdated:              n.PocUpdated,
		Created:                 n.Created,
		Updated:                 n.Updated,
		Status:                  n.Status,
	}
}

// networksFromEnt maps a slice of ent Networks to peeringdb Networks.
func networksFromEnt(nets []*ent.Network) []peeringdb.Network {
	out := make([]peeringdb.Network, len(nets))
	for i, n := range nets {
		out[i] = networkFromEnt(n)
	}
	return out
}

// facilityFromEnt maps an ent Facility to a peeringdb Facility.
func facilityFromEnt(f *ent.Facility) peeringdb.Facility {
	return peeringdb.Facility{
		ID:                        f.ID,
		OrgID:                     derefInt(f.OrgID),
		OrgName:                   f.OrgName,
		CampusID:                  f.CampusID,
		Name:                      f.Name,
		Aka:                       f.Aka,
		NameLong:                  f.NameLong,
		Website:                   f.Website,
		SocialMedia:               socialMediaFromSchema(f.SocialMedia),
		CLLI:                      f.Clli,
		Rencode:                   f.Rencode,
		NPANXX:                    f.Npanxx,
		TechEmail:                 f.TechEmail,
		TechPhone:                 f.TechPhone,
		SalesEmail:                f.SalesEmail,
		SalesPhone:                f.SalesPhone,
		Property:                  f.Property,
		DiverseServingSubstations: f.DiverseServingSubstations,
		AvailableVoltageServices:  f.AvailableVoltageServices,
		Notes:                     f.Notes,
		RegionContinent:           f.RegionContinent,
		StatusDashboard:           f.StatusDashboard,
		Logo:                      f.Logo,
		NetCount:                  f.NetCount,
		IXCount:                   f.IxCount,
		CarrierCount:              f.CarrierCount,
		Address1:                  f.Address1,
		Address2:                  f.Address2,
		City:                      f.City,
		State:                     f.State,
		Country:                   f.Country,
		Zipcode:                   f.Zipcode,
		Suite:                     f.Suite,
		Floor:                     f.Floor,
		Latitude:                  f.Latitude,
		Longitude:                 f.Longitude,
		Created:                   f.Created,
		Updated:                   f.Updated,
		Status:                    f.Status,
	}
}

// facilitiesFromEnt maps a slice of ent Facilities to peeringdb Facilities.
func facilitiesFromEnt(facs []*ent.Facility) []peeringdb.Facility {
	out := make([]peeringdb.Facility, len(facs))
	for i, f := range facs {
		out[i] = facilityFromEnt(f)
	}
	return out
}

// internetExchangeFromEnt maps an ent InternetExchange to a peeringdb
// InternetExchange.
func internetExchangeFromEnt(ix *ent.InternetExchange) peeringdb.InternetExchange {
	return peeringdb.InternetExchange{
		ID:                     ix.ID,
		OrgID:                  derefInt(ix.OrgID),
		Name:                   ix.Name,
		Aka:                    ix.Aka,
		NameLong:               ix.NameLong,
		City:                   ix.City,
		Country:                ix.Country,
		RegionContinent:        ix.RegionContinent,
		Media:                  ix.Media,
		Notes:                  ix.Notes,
		ProtoUnicast:           ix.ProtoUnicast,
		ProtoMulticast:         ix.ProtoMulticast,
		ProtoIPv6:              ix.ProtoIpv6,
		Website:                ix.Website,
		SocialMedia:            socialMediaFromSchema(ix.SocialMedia),
		URLStats:               ix.URLStats,
		TechEmail:              ix.TechEmail,
		TechPhone:              ix.TechPhone,
		PolicyEmail:            ix.PolicyEmail,
		PolicyPhone:            ix.PolicyPhone,
		SalesEmail:             ix.SalesEmail,
		SalesPhone:             ix.SalesPhone,
		NetCount:               ix.NetCount,
		FacCount:               ix.FacCount,
		IXFNetCount:            ix.IxfNetCount,
		IXFLastImport:          ix.IxfLastImport,
		IXFImportRequest:       ix.IxfImportRequest,
		IXFImportRequestStatus: ix.IxfImportRequestStatus,
		ServiceLevel:           ix.ServiceLevel,
		Terms:                  ix.Terms,
		StatusDashboard:        ix.StatusDashboard,
		Logo:                   ix.Logo,
		Created:                ix.Created,
		Updated:                ix.Updated,
		Status:                 ix.Status,
	}
}

// internetExchangesFromEnt maps a slice of ent InternetExchanges to peeringdb
// InternetExchanges.
func internetExchangesFromEnt(ixes []*ent.InternetExchange) []peeringdb.InternetExchange {
	out := make([]peeringdb.InternetExchange, len(ixes))
	for i, ix := range ixes {
		out[i] = internetExchangeFromEnt(ix)
	}
	return out
}

// pocFromEnt maps an ent Poc to a peeringdb Poc.
func pocFromEnt(p *ent.Poc) peeringdb.Poc {
	return peeringdb.Poc{
		ID:      p.ID,
		NetID:   derefInt(p.NetID),
		Role:    p.Role,
		Visible: p.Visible,
		Name:    p.Name,
		Phone:   p.Phone,
		Email:   p.Email,
		URL:     p.URL,
		Created: p.Created,
		Updated: p.Updated,
		Status:  p.Status,
	}
}

// pocsFromEnt maps a slice of ent Pocs to peeringdb Pocs.
func pocsFromEnt(pocs []*ent.Poc) []peeringdb.Poc {
	out := make([]peeringdb.Poc, len(pocs))
	for i, p := range pocs {
		out[i] = pocFromEnt(p)
	}
	return out
}

// ixLanFromEnt maps an ent IxLan to a peeringdb IxLan.
func ixLanFromEnt(l *ent.IxLan) peeringdb.IxLan {
	return peeringdb.IxLan{
		ID:                         l.ID,
		IXID:                       derefInt(l.IxID),
		Name:                       l.Name,
		Descr:                      l.Descr,
		MTU:                        l.Mtu,
		Dot1QSupport:               l.Dot1qSupport,
		RSASN:                      l.RsAsn,
		ARPSponge:                  l.ArpSponge,
		IXFIXPMemberListURLVisible: l.IxfIxpMemberListURLVisible,
		IXFIXPImportEnabled:        l.IxfIxpImportEnabled,
		Created:                    l.Created,
		Updated:                    l.Updated,
		Status:                     l.Status,
	}
}

// ixLansFromEnt maps a slice of ent IxLans to peeringdb IxLans.
func ixLansFromEnt(lans []*ent.IxLan) []peeringdb.IxLan {
	out := make([]peeringdb.IxLan, len(lans))
	for i, l := range lans {
		out[i] = ixLanFromEnt(l)
	}
	return out
}

// ixPrefixFromEnt maps an ent IxPrefix to a peeringdb IxPrefix.
// Note: PeeringDB's live API omits "notes" from ixpfx list responses,
// but our compat layer includes it (ent serializes all schema fields).
// Extra fields don't break API consumers — this is a known divergence.
func ixPrefixFromEnt(p *ent.IxPrefix) peeringdb.IxPrefix {
	return peeringdb.IxPrefix{
		ID:       p.ID,
		IXLanID:  derefInt(p.IxlanID),
		Protocol: p.Protocol,
		Prefix:   p.Prefix,
		InDFZ:    p.InDfz,
		Notes:    p.Notes,
		Created:  p.Created,
		Updated:  p.Updated,
		Status:   p.Status,
	}
}

// ixPrefixesFromEnt maps a slice of ent IxPrefixes to peeringdb IxPrefixes.
func ixPrefixesFromEnt(pfxs []*ent.IxPrefix) []peeringdb.IxPrefix {
	out := make([]peeringdb.IxPrefix, len(pfxs))
	for i, p := range pfxs {
		out[i] = ixPrefixFromEnt(p)
	}
	return out
}

// networkIxLanFromEnt maps an ent NetworkIxLan to a peeringdb NetworkIxLan.
func networkIxLanFromEnt(n *ent.NetworkIxLan) peeringdb.NetworkIxLan {
	return peeringdb.NetworkIxLan{
		ID:          n.ID,
		NetID:       derefInt(n.NetID),
		IXID:        n.IxID,
		IXLanID:     derefInt(n.IxlanID),
		Name:        n.Name,
		Notes:       n.Notes,
		Speed:       n.Speed,
		ASN:         n.Asn,
		IPAddr4:     n.Ipaddr4,
		IPAddr6:     n.Ipaddr6,
		IsRSPeer:    n.IsRsPeer,
		BFDSupport:  n.BfdSupport,
		Operational: n.Operational,
		NetSideID:   n.NetSideID,
		IXSideID:    n.IxSideID,
		Created:     n.Created,
		Updated:     n.Updated,
		Status:      n.Status,
	}
}

// networkIxLansFromEnt maps a slice of ent NetworkIxLans to peeringdb
// NetworkIxLans.
func networkIxLansFromEnt(nixls []*ent.NetworkIxLan) []peeringdb.NetworkIxLan {
	out := make([]peeringdb.NetworkIxLan, len(nixls))
	for i, n := range nixls {
		out[i] = networkIxLanFromEnt(n)
	}
	return out
}

// networkFacilityFromEnt maps an ent NetworkFacility to a peeringdb
// NetworkFacility.
func networkFacilityFromEnt(n *ent.NetworkFacility) peeringdb.NetworkFacility {
	return peeringdb.NetworkFacility{
		ID:       n.ID,
		NetID:    derefInt(n.NetID),
		FacID:    derefInt(n.FacID),
		Name:     n.Name,
		City:     n.City,
		Country:  n.Country,
		LocalASN: n.LocalAsn,
		Created:  n.Created,
		Updated:  n.Updated,
		Status:   n.Status,
	}
}

// networkFacilitiesFromEnt maps a slice of ent NetworkFacilities to peeringdb
// NetworkFacilities.
func networkFacilitiesFromEnt(nfacs []*ent.NetworkFacility) []peeringdb.NetworkFacility {
	out := make([]peeringdb.NetworkFacility, len(nfacs))
	for i, n := range nfacs {
		out[i] = networkFacilityFromEnt(n)
	}
	return out
}

// ixFacilityFromEnt maps an ent IxFacility to a peeringdb IxFacility.
func ixFacilityFromEnt(f *ent.IxFacility) peeringdb.IxFacility {
	return peeringdb.IxFacility{
		ID:      f.ID,
		IXID:    derefInt(f.IxID),
		FacID:   derefInt(f.FacID),
		Name:    f.Name,
		City:    f.City,
		Country: f.Country,
		Created: f.Created,
		Updated: f.Updated,
		Status:  f.Status,
	}
}

// ixFacilitiesFromEnt maps a slice of ent IxFacilities to peeringdb
// IxFacilities.
func ixFacilitiesFromEnt(ixfacs []*ent.IxFacility) []peeringdb.IxFacility {
	out := make([]peeringdb.IxFacility, len(ixfacs))
	for i, f := range ixfacs {
		out[i] = ixFacilityFromEnt(f)
	}
	return out
}

// carrierFromEnt maps an ent Carrier to a peeringdb Carrier.
func carrierFromEnt(c *ent.Carrier) peeringdb.Carrier {
	return peeringdb.Carrier{
		ID:          c.ID,
		OrgID:       derefInt(c.OrgID),
		OrgName:     c.OrgName,
		Name:        c.Name,
		Aka:         c.Aka,
		NameLong:    c.NameLong,
		Website:     c.Website,
		SocialMedia: socialMediaFromSchema(c.SocialMedia),
		Notes:       c.Notes,
		FacCount:    c.FacCount,
		Logo:        c.Logo,
		Created:     c.Created,
		Updated:     c.Updated,
		Status:      c.Status,
	}
}

// carriersFromEnt maps a slice of ent Carriers to peeringdb Carriers.
func carriersFromEnt(carriers []*ent.Carrier) []peeringdb.Carrier {
	out := make([]peeringdb.Carrier, len(carriers))
	for i, c := range carriers {
		out[i] = carrierFromEnt(c)
	}
	return out
}

// carrierFacilityFromEnt maps an ent CarrierFacility to a peeringdb
// CarrierFacility.
func carrierFacilityFromEnt(cf *ent.CarrierFacility) peeringdb.CarrierFacility {
	return peeringdb.CarrierFacility{
		ID:        cf.ID,
		CarrierID: derefInt(cf.CarrierID),
		FacID:     derefInt(cf.FacID),
		Name:      cf.Name,
		Created:   cf.Created,
		Updated:   cf.Updated,
		Status:    cf.Status,
	}
}

// carrierFacilitiesFromEnt maps a slice of ent CarrierFacilities to peeringdb
// CarrierFacilities.
func carrierFacilitiesFromEnt(cfs []*ent.CarrierFacility) []peeringdb.CarrierFacility {
	out := make([]peeringdb.CarrierFacility, len(cfs))
	for i, cf := range cfs {
		out[i] = carrierFacilityFromEnt(cf)
	}
	return out
}

// campusFromEnt maps an ent Campus to a peeringdb Campus.
func campusFromEnt(c *ent.Campus) peeringdb.Campus {
	return peeringdb.Campus{
		ID:          c.ID,
		OrgID:       derefInt(c.OrgID),
		OrgName:     c.OrgName,
		Name:        c.Name,
		NameLong:    c.NameLong,
		Aka:         c.Aka,
		Website:     c.Website,
		SocialMedia: socialMediaFromSchema(c.SocialMedia),
		Notes:       c.Notes,
		Country:     c.Country,
		City:        c.City,
		Zipcode:     c.Zipcode,
		State:       c.State,
		Logo:        c.Logo,
		Created:     c.Created,
		Updated:     c.Updated,
		Status:      c.Status,
	}
}

// campusesFromEnt maps a slice of ent Campuses to peeringdb Campuses.
func campusesFromEnt(campuses []*ent.Campus) []peeringdb.Campus {
	out := make([]peeringdb.Campus, len(campuses))
	for i, c := range campuses {
		out[i] = campusFromEnt(c)
	}
	return out
}
