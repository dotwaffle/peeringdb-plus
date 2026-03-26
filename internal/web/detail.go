package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// getFreshness returns the last successful sync time for freshness footer display.
// Returns zero time if db is nil or on query error (footer will be omitted).
func (h *Handler) getFreshness(ctx context.Context) time.Time {
	if h.db == nil {
		return time.Time{}
	}
	t, _ := sync.GetLastSuccessfulSyncTime(ctx, h.db)
	return t
}

// handleNetworkDetail renders the network detail page for the given ASN string.
// Looks up the network by ASN (not internal ID) per CONTEXT.md decision.
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
	asn, err := strconv.Atoi(asnStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryNetwork(r.Context(), asn)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query network", slog.Int("asn", asn), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.NetworkDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render network detail", slog.Int("asn", asn), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryNetwork fetches a network by ASN and all related data for the detail page.
// Returns the fully populated NetworkDetail or an error (including ent.IsNotFound).
func (h *Handler) queryNetwork(ctx context.Context, asn int) (templates.NetworkDetail, error) {
	net, err := h.client.Network.Query().
		Where(network.Asn(asn)).
		WithOrganization().
		First(ctx)
	if err != nil {
		return templates.NetworkDetail{}, fmt.Errorf("query network ASN %d: %w", asn, err)
	}

	pocCount, err := h.client.Poc.Query().
		Where(poc.HasNetworkWith(network.ID(net.ID))).
		Count(ctx)
	if err != nil {
		slog.Error("count network contacts", slog.Int("network_id", net.ID), slog.Any("error", err))
	}

	data := templates.NetworkDetail{
		ID:            net.ID,
		ASN:           net.Asn,
		Name:          net.Name,
		NameLong:      net.NameLong,
		AKA:           net.Aka,
		Website:       net.Website,
		IRRAsSet:      net.IrrAsSet,
		InfoType:      net.InfoType,
		InfoScope:     net.InfoScope,
		InfoTraffic:   net.InfoTraffic,
		InfoRatio:     net.InfoRatio,
		InfoUnicast:   net.InfoUnicast,
		InfoMulticast: net.InfoMulticast,
		InfoIPv6:      net.InfoIpv6,
		LookingGlass:  net.LookingGlass,
		RouteServer:   net.RouteServer,
		PolicyGeneral: net.PolicyGeneral,
		PolicyURL:     net.PolicyURL,
		Notes:         net.Notes,
		Status:        net.Status,
		IXCount:       net.IxCount,
		FacCount:      net.FacCount,
		PocCount:      pocCount,
	}

	if net.InfoPrefixes4 != nil {
		data.InfoPrefixes4 = *net.InfoPrefixes4
	}
	if net.InfoPrefixes6 != nil {
		data.InfoPrefixes6 = *net.InfoPrefixes6
	}
	if net.Edges.Organization != nil {
		data.OrgName = net.Edges.Organization.Name
		data.OrgID = net.Edges.Organization.ID
	}

	// Compute aggregate bandwidth across all IX presences for the section header.
	ixlans, err := h.client.NetworkIxLan.Query().
		Where(networkixlan.HasNetworkWith(network.ID(net.ID))).
		All(ctx)
	if err == nil {
		var totalBW int
		for _, nix := range ixlans {
			totalBW += nix.Speed
		}
		data.AggregateBW = totalBW

		// Build IX presence rows for terminal and JSON rendering.
		// Sort by name to match web UI fragment ordering.
		sort.Slice(ixlans, func(i, j int) bool {
			return ixlans[i].Name < ixlans[j].Name
		})
		ixRows := make([]templates.NetworkIXLanRow, len(ixlans))
		for i, nix := range ixlans {
			row := templates.NetworkIXLanRow{
				IXName:   nix.Name,
				IXID:     nix.IxID,
				Speed:    nix.Speed,
				IsRSPeer: nix.IsRsPeer,
			}
			if nix.Ipaddr4 != nil {
				row.IPAddr4 = *nix.Ipaddr4
			}
			if nix.Ipaddr6 != nil {
				row.IPAddr6 = *nix.Ipaddr6
			}
			ixRows[i] = row
		}
		data.IXPresences = ixRows
	}

	// Build facility presence rows for terminal and JSON rendering.
	facItems, facErr := h.client.NetworkFacility.Query().
		Where(networkfacility.HasNetworkWith(network.ID(net.ID))).
		WithFacility(). // Eager-load facility entity for lat/lng
		Order(networkfacility.ByName()).
		All(ctx)
	if facErr == nil {
		facRows := make([]templates.NetworkFacRow, len(facItems))
		for i, nf := range facItems {
			row := templates.NetworkFacRow{
				FacName:  nf.Name,
				LocalASN: nf.LocalAsn,
				City:     nf.City,
				Country:  nf.Country,
			}
			if nf.FacID != nil {
				row.FacID = *nf.FacID
			}
			if fac := nf.Edges.Facility; fac != nil {
				if fac.Latitude != nil {
					row.Latitude = *fac.Latitude
				}
				if fac.Longitude != nil {
					row.Longitude = *fac.Longitude
				}
			}
			facRows[i] = row
		}
		data.FacPresences = facRows
	} else {
		slog.Error("query network facilities for detail", slog.Int("network_id", net.ID), slog.Any("error", facErr))
	}

	return data, nil
}

// handleIXDetail renders the IXP detail page for the given ID.
func (h *Handler) handleIXDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryIX(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query ix", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.IXDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render ix detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryIX fetches an internet exchange by ID and all related data for the detail page.
// Returns the fully populated IXDetail or an error (including ent.IsNotFound).
func (h *Handler) queryIX(ctx context.Context, id int) (templates.IXDetail, error) {
	ix, err := h.client.InternetExchange.Query().
		Where(internetexchange.ID(id)).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return templates.IXDetail{}, fmt.Errorf("query ix %d: %w", id, err)
	}

	data := templates.IXDetail{
		ID:              ix.ID,
		Name:            ix.Name,
		NameLong:        ix.NameLong,
		AKA:             ix.Aka,
		Website:         ix.Website,
		City:            ix.City,
		Country:         ix.Country,
		RegionContinent: ix.RegionContinent,
		Media:           ix.Media,
		ProtoUnicast:    ix.ProtoUnicast,
		ProtoMulticast:  ix.ProtoMulticast,
		ProtoIPv6:       ix.ProtoIpv6,
		Notes:           ix.Notes,
		Status:          ix.Status,
		NetCount:        ix.NetCount,
		FacCount:        ix.FacCount,
	}

	// Count prefixes via IxLan traversal: InternetExchange -> IxLan -> IxPrefix.
	prefixCount, err := h.client.IxPrefix.Query().
		Where(ixprefix.HasIxLanWith(ixlan.HasInternetExchangeWith(internetexchange.ID(id)))).
		Count(ctx)
	if err == nil {
		data.PrefixCount = prefixCount
	}
	if ix.Edges.Organization != nil {
		data.OrgName = ix.Edges.Organization.Name
		data.OrgID = ix.Edges.Organization.ID
	}

	// Compute aggregate bandwidth and eager-load participant rows.
	ixParticipants, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(id))).
		QueryNetworkIxLans().
		WithNetwork().
		Order(networkixlan.ByAsn()).
		All(ctx)
	if err == nil {
		var totalBW int
		rows := make([]templates.IXParticipantRow, len(ixParticipants))
		for i, nix := range ixParticipants {
			totalBW += nix.Speed
			netName := ""
			if net := nix.Edges.Network; net != nil {
				netName = net.Name
			}
			row := templates.IXParticipantRow{
				NetName:  netName,
				ASN:      nix.Asn,
				Speed:    nix.Speed,
				IsRSPeer: nix.IsRsPeer,
			}
			if nix.Ipaddr4 != nil {
				row.IPAddr4 = *nix.Ipaddr4
			}
			if nix.Ipaddr6 != nil {
				row.IPAddr6 = *nix.Ipaddr6
			}
			rows[i] = row
		}
		data.AggregateBW = totalBW
		data.Participants = rows
	} else {
		slog.Error("eager-load ix participants", slog.Int("ix_id", id), slog.Any("error", err))
	}

	// Eager-load IX facilities with facility coordinates for map rendering.
	ixFacItems, err := h.client.IxFacility.Query().
		Where(ixfacility.HasInternetExchangeWith(internetexchange.ID(id))).
		WithFacility(). // Eager-load facility entity for lat/lng
		Order(ixfacility.ByName()).
		All(ctx)
	if err == nil {
		var facRows []templates.IXFacilityRow
		for _, ixf := range ixFacItems {
			if ixf.FacID == nil {
				continue
			}
			row := templates.IXFacilityRow{
				FacName: ixf.Name,
				FacID:   *ixf.FacID,
				City:    ixf.City,
				Country: ixf.Country,
			}
			if fac := ixf.Edges.Facility; fac != nil {
				if fac.Latitude != nil {
					row.Latitude = *fac.Latitude
				}
				if fac.Longitude != nil {
					row.Longitude = *fac.Longitude
				}
			}
			facRows = append(facRows, row)
		}
		data.Facilities = facRows
	} else {
		slog.Error("eager-load ix facilities", slog.Int("ix_id", id), slog.Any("error", err))
	}

	// Eager-load IX prefixes.
	ixPrefixItems, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(id))).
		QueryIxPrefixes().
		Order(ixprefix.ByPrefix()).
		All(ctx)
	if err == nil {
		prefixRows := make([]templates.IXPrefixRow, len(ixPrefixItems))
		for i, p := range ixPrefixItems {
			prefixRows[i] = templates.IXPrefixRow{
				Prefix:   p.Prefix,
				Protocol: p.Protocol,
				InDFZ:    p.InDfz,
			}
		}
		data.Prefixes = prefixRows
	} else {
		slog.Error("eager-load ix prefixes", slog.Int("ix_id", id), slog.Any("error", err))
	}

	return data, nil
}

// handleFacilityDetail renders the facility detail page for the given ID.
func (h *Handler) handleFacilityDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryFacility(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query facility", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.FacilityDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render facility detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryFacility fetches a facility by ID and all related data for the detail page.
// Returns the fully populated FacilityDetail or an error (including ent.IsNotFound).
func (h *Handler) queryFacility(ctx context.Context, id int) (templates.FacilityDetail, error) {
	fac, err := h.client.Facility.Query().
		Where(facility.ID(id)).
		WithOrganization().
		WithCampus().
		Only(ctx)
	if err != nil {
		return templates.FacilityDetail{}, fmt.Errorf("query facility %d: %w", id, err)
	}

	data := templates.FacilityDetail{
		ID:           fac.ID,
		Name:         fac.Name,
		NameLong:     fac.NameLong,
		AKA:          fac.Aka,
		Website:      fac.Website,
		Address1:     fac.Address1,
		Address2:     fac.Address2,
		City:         fac.City,
		State:        fac.State,
		Country:      fac.Country,
		Zipcode:      fac.Zipcode,
		CLLI:         fac.Clli,
		Notes:        fac.Notes,
		Status:       fac.Status,
		NetCount:     fac.NetCount,
		IXCount:      fac.IxCount,
		CarrierCount: fac.CarrierCount,
	}
	if fac.RegionContinent != nil {
		data.RegionContinent = *fac.RegionContinent
	}
	if fac.Edges.Organization != nil {
		data.OrgName = fac.Edges.Organization.Name
		data.OrgID = fac.Edges.Organization.ID
	}
	if fac.Edges.Campus != nil {
		data.CampusName = fac.Edges.Campus.Name
		data.CampusID = fac.Edges.Campus.ID
	}
	if fac.Latitude != nil && fac.Longitude != nil {
		if *fac.Latitude != 0 || *fac.Longitude != 0 {
			data.Latitude = *fac.Latitude
			data.Longitude = *fac.Longitude
		}
	}

	// Eager-load facility networks.
	facNetItems, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasFacilityWith(facility.ID(id))).
		Order(networkfacility.ByName()).
		All(ctx)
	if err == nil {
		netRows := make([]templates.FacNetworkRow, len(facNetItems))
		for i, nf := range facNetItems {
			netRows[i] = templates.FacNetworkRow{
				NetName: nf.Name,
				ASN:     nf.LocalAsn,
				City:    nf.City,
				Country: nf.Country,
			}
		}
		data.Networks = netRows
	} else {
		slog.Error("eager-load fac networks", slog.Int("fac_id", id), slog.Any("error", err))
	}

	// Eager-load facility IXPs.
	facIXItems, err := h.client.IxFacility.Query().
		Where(ixfacility.HasFacilityWith(facility.ID(id))).
		Order(ixfacility.ByName()).
		All(ctx)
	if err == nil {
		var ixRows []templates.FacIXRow
		for _, ixf := range facIXItems {
			if ixf.IxID == nil {
				continue
			}
			ixRows = append(ixRows, templates.FacIXRow{
				IXName: ixf.Name,
				IXID:   *ixf.IxID,
			})
		}
		data.IXPs = ixRows
	} else {
		slog.Error("eager-load fac ixps", slog.Int("fac_id", id), slog.Any("error", err))
	}

	// Eager-load facility carriers.
	facCarrierItems, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasFacilityWith(facility.ID(id))).
		Order(carrierfacility.ByName()).
		All(ctx)
	if err == nil {
		var carrierRows []templates.FacCarrierRow
		for _, cf := range facCarrierItems {
			if cf.CarrierID == nil {
				continue
			}
			carrierRows = append(carrierRows, templates.FacCarrierRow{
				CarrierName: cf.Name,
				CarrierID:   *cf.CarrierID,
			})
		}
		data.Carriers = carrierRows
	} else {
		slog.Error("eager-load fac carriers", slog.Int("fac_id", id), slog.Any("error", err))
	}

	return data, nil
}

// handleOrgDetail renders the organization detail page for the given ID.
func (h *Handler) handleOrgDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryOrg(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query org", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.OrgDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render org detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryOrg fetches an organization by ID and all related data for the detail page.
// Returns the fully populated OrgDetail or an error (including ent.IsNotFound).
func (h *Handler) queryOrg(ctx context.Context, id int) (templates.OrgDetail, error) {
	org, err := h.client.Organization.Query().
		Where(organization.ID(id)).
		Only(ctx)
	if err != nil {
		return templates.OrgDetail{}, fmt.Errorf("query org %d: %w", id, err)
	}

	// Count non-pre-computed child entity counts.
	ixCount, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org IXPs", slog.Int("org_id", id), slog.Any("error", err))
	}

	campusCount, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	carrierCount, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org carriers", slog.Int("org_id", id), slog.Any("error", err))
	}

	data := templates.OrgDetail{
		ID:           org.ID,
		Name:         org.Name,
		NameLong:     org.NameLong,
		AKA:          org.Aka,
		Website:      org.Website,
		Address1:     org.Address1,
		Address2:     org.Address2,
		City:         org.City,
		State:        org.State,
		Country:      org.Country,
		Zipcode:      org.Zipcode,
		Notes:        org.Notes,
		Status:       org.Status,
		NetCount:     org.NetCount,
		FacCount:     org.FacCount,
		IXCount:      ixCount,
		CampusCount:  campusCount,
		CarrierCount: carrierCount,
	}

	// Eager-load org networks.
	orgNetItems, err := h.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(id))).
		Order(network.ByAsn()).
		All(ctx)
	if err == nil {
		netRows := make([]templates.OrgNetworkRow, len(orgNetItems))
		for i, n := range orgNetItems {
			netRows[i] = templates.OrgNetworkRow{
				NetName: n.Name,
				ASN:     n.Asn,
			}
		}
		data.Networks = netRows
	} else {
		slog.Error("eager-load org networks", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org IXPs.
	orgIXItems, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id))).
		Order(internetexchange.ByName()).
		All(ctx)
	if err == nil {
		ixRows := make([]templates.OrgIXRow, len(orgIXItems))
		for i, ix := range orgIXItems {
			ixRows[i] = templates.OrgIXRow{
				IXName: ix.Name,
				IXID:   ix.ID,
			}
		}
		data.IXPs = ixRows
	} else {
		slog.Error("eager-load org ixps", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org facilities.
	orgFacItems, err := h.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(id))).
		Order(facility.ByName()).
		All(ctx)
	if err == nil {
		facRows := make([]templates.OrgFacilityRow, len(orgFacItems))
		for i, f := range orgFacItems {
			facRows[i] = templates.OrgFacilityRow{
				FacName: f.Name,
				FacID:   f.ID,
				City:    f.City,
				Country: f.Country,
			}
		}
		data.Facs = facRows
	} else {
		slog.Error("eager-load org facilities", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org campuses.
	orgCampusItems, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id))).
		Order(campus.ByName()).
		All(ctx)
	if err == nil {
		campusRows := make([]templates.OrgCampusRow, len(orgCampusItems))
		for i, c := range orgCampusItems {
			campusRows[i] = templates.OrgCampusRow{
				CampusName: c.Name,
				CampusID:   c.ID,
			}
		}
		data.Campuses = campusRows
	} else {
		slog.Error("eager-load org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org carriers.
	orgCarrierItems, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id))).
		Order(carrier.ByName()).
		All(ctx)
	if err == nil {
		carrierRows := make([]templates.OrgCarrierRow, len(orgCarrierItems))
		for i, c := range orgCarrierItems {
			carrierRows[i] = templates.OrgCarrierRow{
				CarrierName: c.Name,
				CarrierID:   c.ID,
			}
		}
		data.Carriers = carrierRows
	} else {
		slog.Error("eager-load org carriers", slog.Int("org_id", id), slog.Any("error", err))
	}

	return data, nil
}

// handleCampusDetail renders the campus detail page for the given ID.
func (h *Handler) handleCampusDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryCampus(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query campus", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.CampusDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render campus detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryCampus fetches a campus by ID and all related data for the detail page.
// Returns the fully populated CampusDetail or an error (including ent.IsNotFound).
func (h *Handler) queryCampus(ctx context.Context, id int) (templates.CampusDetail, error) {
	c, err := h.client.Campus.Query().
		Where(campus.ID(id)).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return templates.CampusDetail{}, fmt.Errorf("query campus %d: %w", id, err)
	}

	facCount, err := h.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count campus facilities", slog.Int("campus_id", id), slog.Any("error", err))
	}

	data := templates.CampusDetail{
		ID:       c.ID,
		Name:     c.Name,
		Website:  c.Website,
		City:     c.City,
		State:    c.State,
		Country:  c.Country,
		Zipcode:  c.Zipcode,
		Notes:    c.Notes,
		Status:   c.Status,
		FacCount: facCount,
	}
	if c.NameLong != nil {
		data.NameLong = *c.NameLong
	}
	if c.Aka != nil {
		data.AKA = *c.Aka
	}
	if c.Edges.Organization != nil {
		data.OrgName = c.Edges.Organization.Name
		data.OrgID = c.Edges.Organization.ID
	}

	// Eager-load campus facilities.
	campusFacItems, err := h.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(id))).
		Order(facility.ByName()).
		All(ctx)
	if err == nil {
		facRows := make([]templates.CampusFacilityRow, len(campusFacItems))
		for i, f := range campusFacItems {
			facRows[i] = templates.CampusFacilityRow{
				FacName: f.Name,
				FacID:   f.ID,
				City:    f.City,
				Country: f.Country,
			}
		}
		data.Facilities = facRows
	} else {
		slog.Error("eager-load campus facilities", slog.Int("campus_id", id), slog.Any("error", err))
	}

	return data, nil
}

// handleCarrierDetail renders the carrier detail page for the given ID.
func (h *Handler) handleCarrierDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	data, err := h.queryCarrier(r.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query carrier", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:     data.Name,
		Content:   templates.CarrierDetailPage(data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render carrier detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}

// queryCarrier fetches a carrier by ID and all related data for the detail page.
// Returns the fully populated CarrierDetail or an error (including ent.IsNotFound).
func (h *Handler) queryCarrier(ctx context.Context, id int) (templates.CarrierDetail, error) {
	cr, err := h.client.Carrier.Query().
		Where(carrier.ID(id)).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return templates.CarrierDetail{}, fmt.Errorf("query carrier %d: %w", id, err)
	}

	data := templates.CarrierDetail{
		ID:       cr.ID,
		Name:     cr.Name,
		NameLong: cr.NameLong,
		AKA:      cr.Aka,
		Website:  cr.Website,
		Notes:    cr.Notes,
		Status:   cr.Status,
		FacCount: cr.FacCount,
	}
	if cr.Edges.Organization != nil {
		data.OrgName = cr.Edges.Organization.Name
		data.OrgID = cr.Edges.Organization.ID
	}

	// Eager-load carrier facilities.
	carrierFacItems, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasCarrierWith(carrier.ID(id))).
		Order(carrierfacility.ByName()).
		All(ctx)
	if err == nil {
		var facRows []templates.CarrierFacilityRow
		for _, cf := range carrierFacItems {
			if cf.FacID == nil {
				continue
			}
			facRows = append(facRows, templates.CarrierFacilityRow{
				FacName: cf.Name,
				FacID:   *cf.FacID,
			})
		}
		data.Facilities = facRows
	} else {
		slog.Error("eager-load carrier facilities", slog.Int("carrier_id", id), slog.Any("error", err))
	}

	return data, nil
}

// handleFragment dispatches lazy-loaded fragment requests.
// Fragment URLs follow the pattern: {parent_type}/{parent_id}/{relation}
func (h *Handler) handleFragment(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) != 3 {
		h.handleNotFound(w, r)
		return
	}

	parentType := parts[0]
	parentID, err := strconv.Atoi(parts[1])
	if err != nil {
		h.handleNotFound(w, r)
		return
	}
	relation := parts[2]

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	switch parentType {
	case "net":
		switch relation {
		case "ixlans":
			h.handleNetIXLansFragment(w, r, parentID)
		case "facilities":
			h.handleNetFacilitiesFragment(w, r, parentID)
		case "contacts":
			h.handleNetContactsFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	case "ix":
		switch relation {
		case "participants":
			h.handleIXParticipantsFragment(w, r, parentID)
		case "facilities":
			h.handleIXFacilitiesFragment(w, r, parentID)
		case "prefixes":
			h.handleIXPrefixesFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	case "fac":
		switch relation {
		case "networks":
			h.handleFacNetworksFragment(w, r, parentID)
		case "ixps":
			h.handleFacIXPsFragment(w, r, parentID)
		case "carriers":
			h.handleFacCarriersFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	case "org":
		switch relation {
		case "networks":
			h.handleOrgNetworksFragment(w, r, parentID)
		case "ixps":
			h.handleOrgIXPsFragment(w, r, parentID)
		case "facilities":
			h.handleOrgFacilitiesFragment(w, r, parentID)
		case "campuses":
			h.handleOrgCampusesFragment(w, r, parentID)
		case "carriers":
			h.handleOrgCarriersFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	case "campus":
		switch relation {
		case "facilities":
			h.handleCampusFacilitiesFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	case "carrier":
		switch relation {
		case "facilities":
			h.handleCarrierFacilitiesFragment(w, r, parentID)
		default:
			h.handleNotFound(w, r)
		}
	default:
		h.handleNotFound(w, r)
	}
}

// handleNetIXLansFragment returns an HTML fragment listing a network's IX presences.
func (h *Handler) handleNetIXLansFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.NetworkIxLan.Query().
		Where(networkixlan.HasNetworkWith(network.ID(netID))).
		Order(networkixlan.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query network ixlans", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.NetworkIXLanRow, len(items))
	for i, nix := range items {
		row := templates.NetworkIXLanRow{
			IXName:   nix.Name,
			IXID:     nix.IxID,
			Speed:    nix.Speed,
			IsRSPeer: nix.IsRsPeer,
		}
		if nix.Ipaddr4 != nil {
			row.IPAddr4 = *nix.Ipaddr4
		}
		if nix.Ipaddr6 != nil {
			row.IPAddr6 = *nix.Ipaddr6
		}
		rows[i] = row
	}

	if err := templates.NetworkIXLansList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render network ixlans fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleNetFacilitiesFragment returns an HTML fragment listing a network's facility presences.
func (h *Handler) handleNetFacilitiesFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasNetworkWith(network.ID(netID))).
		Order(networkfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query network facilities", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.NetworkFacRow, len(items))
	for i, nf := range items {
		row := templates.NetworkFacRow{
			FacName:  nf.Name,
			LocalASN: nf.LocalAsn,
			City:     nf.City,
			Country:  nf.Country,
		}
		if nf.FacID != nil {
			row.FacID = *nf.FacID
		}
		rows[i] = row
	}

	if err := templates.NetworkFacilitiesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render network facilities fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleNetContactsFragment returns an HTML fragment listing a network's contacts.
func (h *Handler) handleNetContactsFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.Poc.Query().
		Where(poc.HasNetworkWith(network.ID(netID))).
		Order(poc.ByRole(), poc.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query network contacts", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.ContactRow, len(items))
	for i, p := range items {
		rows[i] = templates.ContactRow{
			Name:  p.Name,
			Role:  p.Role,
			Email: p.Email,
			Phone: p.Phone,
			URL:   p.URL,
		}
	}

	if err := templates.NetworkContactsList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render network contacts fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleIXParticipantsFragment returns an HTML fragment listing an IXP's participants.
// Uses the IxLan -> NetworkIxLan path (Research Pitfall 1: do NOT go directly from InternetExchange).
func (h *Handler) handleIXParticipantsFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	items, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(ixID))).
		QueryNetworkIxLans().
		WithNetwork().
		Order(networkixlan.ByAsn()).
		All(r.Context())
	if err != nil {
		slog.Error("query ix participants", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.IXParticipantRow, len(items))
	for i, nix := range items {
		netName := ""
		if net := nix.Edges.Network; net != nil {
			netName = net.Name
		}
		row := templates.IXParticipantRow{
			NetName:  netName,
			ASN:      nix.Asn,
			Speed:    nix.Speed,
			IsRSPeer: nix.IsRsPeer,
		}
		if nix.Ipaddr4 != nil {
			row.IPAddr4 = *nix.Ipaddr4
		}
		if nix.Ipaddr6 != nil {
			row.IPAddr6 = *nix.Ipaddr6
		}
		rows[i] = row
	}

	if err := templates.IXParticipantsList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render ix participants fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleIXFacilitiesFragment returns an HTML fragment listing an IXP's facilities.
func (h *Handler) handleIXFacilitiesFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	items, err := h.client.IxFacility.Query().
		Where(ixfacility.HasInternetExchangeWith(internetexchange.ID(ixID))).
		Order(ixfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query ix facilities", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	var rows []templates.IXFacilityRow
	for _, ixf := range items {
		if ixf.FacID == nil {
			continue
		}
		rows = append(rows, templates.IXFacilityRow{
			FacName: ixf.Name,
			FacID:   *ixf.FacID,
			City:    ixf.City,
			Country: ixf.Country,
		})
	}

	if err := templates.IXFacilitiesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render ix facilities fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleIXPrefixesFragment returns an HTML fragment listing an IXP's prefixes.
// Uses the IxLan -> IxPrefix path (Research Pitfall 2: do NOT go directly from InternetExchange).
func (h *Handler) handleIXPrefixesFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	items, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(ixID))).
		QueryIxPrefixes().
		Order(ixprefix.ByPrefix()).
		All(r.Context())
	if err != nil {
		slog.Error("query ix prefixes", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.IXPrefixRow, len(items))
	for i, p := range items {
		rows[i] = templates.IXPrefixRow{
			Prefix:   p.Prefix,
			Protocol: p.Protocol,
			InDFZ:    p.InDfz,
		}
	}

	if err := templates.IXPrefixesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render ix prefixes fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleFacNetworksFragment returns an HTML fragment listing a facility's networks.
func (h *Handler) handleFacNetworksFragment(w http.ResponseWriter, r *http.Request, facID int) {
	items, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasFacilityWith(facility.ID(facID))).
		Order(networkfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query fac networks", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.FacNetworkRow, len(items))
	for i, nf := range items {
		rows[i] = templates.FacNetworkRow{
			NetName: nf.Name,
			ASN:     nf.LocalAsn,
			City:    nf.City,
			Country: nf.Country,
		}
	}

	if err := templates.FacNetworksList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render fac networks fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleFacIXPsFragment returns an HTML fragment listing a facility's IXPs.
func (h *Handler) handleFacIXPsFragment(w http.ResponseWriter, r *http.Request, facID int) {
	items, err := h.client.IxFacility.Query().
		Where(ixfacility.HasFacilityWith(facility.ID(facID))).
		WithInternetExchange().
		Order(ixfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query fac ixps", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	var rows []templates.FacIXRow
	for _, ixf := range items {
		if ixf.IxID == nil || ixf.Edges.InternetExchange == nil {
			continue
		}
		rows = append(rows, templates.FacIXRow{
			IXName: ixf.Edges.InternetExchange.Name,
			IXID:   *ixf.IxID,
		})
	}

	if err := templates.FacIXPsList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render fac ixps fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleFacCarriersFragment returns an HTML fragment listing a facility's carriers.
func (h *Handler) handleFacCarriersFragment(w http.ResponseWriter, r *http.Request, facID int) {
	items, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasFacilityWith(facility.ID(facID))).
		Order(carrierfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query fac carriers", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	var rows []templates.FacCarrierRow
	for _, cf := range items {
		if cf.CarrierID == nil {
			continue
		}
		rows = append(rows, templates.FacCarrierRow{
			CarrierName: cf.Name,
			CarrierID:   *cf.CarrierID,
		})
	}

	if err := templates.FacCarriersList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render fac carriers fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleOrgNetworksFragment returns an HTML fragment listing an org's networks.
func (h *Handler) handleOrgNetworksFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	items, err := h.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(orgID))).
		Order(network.ByAsn()).
		All(r.Context())
	if err != nil {
		slog.Error("query org networks", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.OrgNetworkRow, len(items))
	for i, n := range items {
		rows[i] = templates.OrgNetworkRow{
			NetName: n.Name,
			ASN:     n.Asn,
		}
	}

	if err := templates.OrgNetworksList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render org networks fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgIXPsFragment returns an HTML fragment listing an org's IXPs.
func (h *Handler) handleOrgIXPsFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	items, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(orgID))).
		Order(internetexchange.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query org ixps", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.OrgIXRow, len(items))
	for i, ix := range items {
		rows[i] = templates.OrgIXRow{
			IXName: ix.Name,
			IXID:   ix.ID,
		}
	}

	if err := templates.OrgIXPsList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render org ixps fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgFacilitiesFragment returns an HTML fragment listing an org's facilities.
func (h *Handler) handleOrgFacilitiesFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	items, err := h.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(orgID))).
		Order(facility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query org facilities", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.OrgFacilityRow, len(items))
	for i, f := range items {
		rows[i] = templates.OrgFacilityRow{
			FacName: f.Name,
			FacID:   f.ID,
			City:    f.City,
			Country: f.Country,
		}
	}

	if err := templates.OrgFacilitiesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render org facilities fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgCampusesFragment returns an HTML fragment listing an org's campuses.
func (h *Handler) handleOrgCampusesFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	items, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(orgID))).
		Order(campus.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query org campuses", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.OrgCampusRow, len(items))
	for i, c := range items {
		rows[i] = templates.OrgCampusRow{
			CampusName: c.Name,
			CampusID:   c.ID,
		}
	}

	if err := templates.OrgCampusesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render org campuses fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgCarriersFragment returns an HTML fragment listing an org's carriers.
func (h *Handler) handleOrgCarriersFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	items, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(orgID))).
		Order(carrier.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query org carriers", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.OrgCarrierRow, len(items))
	for i, c := range items {
		rows[i] = templates.OrgCarrierRow{
			CarrierName: c.Name,
			CarrierID:   c.ID,
		}
	}

	if err := templates.OrgCarriersList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render org carriers fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleCampusFacilitiesFragment returns an HTML fragment listing a campus's facilities.
func (h *Handler) handleCampusFacilitiesFragment(w http.ResponseWriter, r *http.Request, campusID int) {
	items, err := h.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(campusID))).
		Order(facility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query campus facilities", slog.Int("campus_id", campusID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	rows := make([]templates.CampusFacilityRow, len(items))
	for i, f := range items {
		rows[i] = templates.CampusFacilityRow{
			FacName: f.Name,
			FacID:   f.ID,
			City:    f.City,
			Country: f.Country,
		}
	}

	if err := templates.CampusFacilitiesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render campus facilities fragment", slog.Int("campus_id", campusID), slog.Any("error", err))
	}
}

// handleCarrierFacilitiesFragment returns an HTML fragment listing a carrier's facilities.
func (h *Handler) handleCarrierFacilitiesFragment(w http.ResponseWriter, r *http.Request, carrierID int) {
	items, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasCarrierWith(carrier.ID(carrierID))).
		Order(carrierfacility.ByName()).
		All(r.Context())
	if err != nil {
		slog.Error("query carrier facilities", slog.Int("carrier_id", carrierID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	var rows []templates.CarrierFacilityRow
	for _, cf := range items {
		if cf.FacID == nil {
			continue
		}
		rows = append(rows, templates.CarrierFacilityRow{
			FacName: cf.Name,
			FacID:   *cf.FacID,
		})
	}

	if err := templates.CarrierFacilitiesList(rows).Render(r.Context(), w); err != nil {
		slog.Error("render carrier facilities fragment", slog.Int("carrier_id", carrierID), slog.Any("error", err))
	}
}
