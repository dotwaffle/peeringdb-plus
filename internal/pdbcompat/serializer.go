package pdbcompat

import (
	"context"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/schematypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privfield"
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

// stringsOrEmpty returns an empty, non-nil slice when s is nil so JSON
// serialization emits [] rather than null. Upstream PeeringDB always emits a
// list for ixp_update_exclude (the model default is an empty list) and for
// info_types (a non-null column, 2.83.0 serializers.py:3947-3960). Do not
// use it for fac available_voltage_services: that column is nullable
// upstream, and null is its correct value.
func stringsOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// metaOrEmpty returns an empty, non-nil map when m is nil so JSON
// serialization emits {} rather than null. Upstream PeeringDB always emits
// meta as an object (JSONField default=dict, migration 0159). Rows synced
// before the column existed, or before upstream deployed 2.83.0, have no
// stored document.
func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
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
		InfoTypes:               stringsOrEmpty(n.InfoTypes),
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
		IxpUpdateExclude:        stringsOrEmpty(n.IxpUpdateExclude),
		StatusDashboard:         n.StatusDashboard,
		RIRStatus:               n.RirStatus,
		RIRStatusUpdated:        n.RirStatusUpdated,
		Logo:                    n.Logo,
		Meta:                    metaOrEmpty(n.Meta),
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

// ixMedia is the media value that upstream renders for every ix. The
// field is deprecated, and get_media returns this constant whatever the
// stored value is (2.83.0 serializers.py:4497-4500, #1555).
const ixMedia = "Ethernet"

// internetExchangeFromEnt maps an ent InternetExchange to a peeringdb
// InternetExchange. Media is always ixMedia, as upstream renders it.
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
		Media:                  ixMedia,
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

// pocFromEnt maps an ent Poc to a peeringdb Poc. A deleted contact is
// served with name, phone, email and url blanked, as upstream does
// (peeringdb.Poc.BlankDeletedContact). Every /api/ path that renders a
// poc calls this function, so the rule holds even for a stored tombstone
// that still carries contact data.
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
	}.BlankDeletedContact()
}

// pocsFromEnt maps a slice of ent Pocs to peeringdb Pocs.
func pocsFromEnt(pocs []*ent.Poc) []peeringdb.Poc {
	out := make([]peeringdb.Poc, len(pocs))
	for i, p := range pocs {
		out[i] = pocFromEnt(p)
	}
	return out
}

// ixLanResponse is the /api wire shape of an ixlan. It has the same keys
// in the same order as peeringdb.IxLan. The one difference is
// IXFIXPMemberListURL: a pointer, so the key is present or absent
// independently of the value. peeringdb.IxLan also decodes the upstream
// input in sync, so it keeps a plain string.
type ixLanResponse struct {
	ID                         int       `json:"id"`
	IXID                       int       `json:"ix_id"`
	Name                       string    `json:"name"`
	Descr                      string    `json:"descr"`
	MTU                        int       `json:"mtu"`
	Dot1QSupport               bool      `json:"dot1q_support"`
	RSASN                      *int      `json:"rs_asn"`
	ARPSponge                  *string   `json:"arp_sponge"`
	IXFIXPMemberListURLVisible string    `json:"ixf_ixp_member_list_url_visible"`
	IXFIXPMemberListURL        *string   `json:"ixf_ixp_member_list_url,omitempty"`
	IXFIXPImportEnabled        bool      `json:"ixf_ixp_import_enabled"`
	Created                    time.Time `json:"created"`
	Updated                    time.Time `json:"updated"`
	Status                     string    `json:"status"`
}

// ixLanFromEnt maps an ent IxLan to its /api wire shape and applies the
// field-level privacy of ixf_ixp_member_list_url through
// internal/privfield.Redact.
//
// The caller MUST pass a context that has the privacy tier stamped by
// middleware.PrivacyTier. An unstamped context gets TierPublic
// (fail-closed), as privfield.Redact specifies. ixfMemberListURLOut
// decides the ixf_ixp_member_list_url key.
func ixLanFromEnt(ctx context.Context, l *ent.IxLan) ixLanResponse {
	return ixLanResponse{
		ID:                         l.ID,
		IXID:                       derefInt(l.IxID),
		Name:                       l.Name,
		Descr:                      l.Descr,
		MTU:                        l.Mtu,
		Dot1QSupport:               false, // always false upstream (serializers.py:4304-4307, #903)
		RSASN:                      l.RsAsn,
		ARPSponge:                  l.ArpSponge,
		IXFIXPMemberListURLVisible: l.IxfIxpMemberListURLVisible, // always emitted
		IXFIXPMemberListURL:        ixfMemberListURLOut(ctx, l),
		IXFIXPImportEnabled:        l.IxfIxpImportEnabled,
		Created:                    l.Created,
		Updated:                    l.Updated,
		Status:                     l.Status,
	}
}

// ixfMemberListURLOut returns the ixf_ixp_member_list_url value for the
// caller on ctx, or nil to omit the key.
//
// Upstream deletes the key only when the caller does not have the
// permission for the visibility (2.83.0 permissions.py:344-353). A caller
// that has the permission gets the stored value, also when it is empty.
// Thus the omit flag of privfield.Redact decides the key, not the value.
//
// One exception applies: a Users row with an empty value. An anonymous
// sync does not receive the value of a Users row, and it stores "" for
// it. For such a row, "" does not mean that the URL is empty, so the key
// stays out.
func ixfMemberListURLOut(ctx context.Context, l *ent.IxLan) *string {
	url, omit := privfield.Redact(ctx, l.IxfIxpMemberListURLVisible, l.IxfIxpMemberListURL)
	if omit || (url == "" && l.IxfIxpMemberListURLVisible != "Public") {
		return nil
	}
	return new(url)
}

// ixLansFromEnt maps a slice of ent IxLans to their /api wire shape. It
// passes ctx to ixLanFromEnt, so the tier of the caller decides the
// ixf_ixp_member_list_url key of each row.
func ixLansFromEnt(ctx context.Context, lans []*ent.IxLan) []ixLanResponse {
	out := make([]ixLanResponse, len(lans))
	for i, l := range lans {
		out[i] = ixLanFromEnt(ctx, l)
	}
	return out
}

// ixPrefixFromEnt maps an ent IxPrefix to a peeringdb IxPrefix.
//
// Matches upstream: PeeringDB's live API omits "notes" from ixpfx responses,
// and as of v1.15 the field is dropped from our ent schema too.
// See the project history
func ixPrefixFromEnt(p *ent.IxPrefix) peeringdb.IxPrefix {
	return peeringdb.IxPrefix{
		ID:       p.ID,
		IXLanID:  derefInt(p.IxlanID),
		Protocol: p.Protocol,
		Prefix:   p.Prefix,
		InDFZ:    p.InDfz,
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
		Operational: new(n.Operational),
		NetSideID:   n.NetSideID,
		IXSideID:    n.IxSideID,
		Meta:        metaOrEmpty(n.Meta),
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
