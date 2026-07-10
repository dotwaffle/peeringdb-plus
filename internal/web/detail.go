package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
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
	asn, ok := parseASN(asnStr)
	if !ok {
		httperr.WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid ASN %q: must be between 1 and 4294967295", asnStr),
			Instance: r.URL.Path,
		})
		return
	}

	data, err := h.queryNetwork(r.Context(), int(asn))
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query network", slog.Int("asn", int(asn)), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	page := PageContent{
		Title:       data.Name,
		Description: fmt.Sprintf("%s (AS%d) — peering details, IX presence, and facilities on PeeringDB Plus.", data.Name, data.ASN),
		Canonical:   canonicalURL(r),
		Content:     templates.NetworkDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
		NeedsMap:    true,
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render network detail", slog.Int("asn", int(asn)), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
		Title:       data.Name,
		Description: fmt.Sprintf("%s — participants, peering LAN prefixes, and facilities on PeeringDB Plus.", data.Name),
		Canonical:   canonicalURL(r),
		Content:     templates.IXDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
		NeedsMap:    true,
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render ix detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
		Title:       data.Name,
		Description: fmt.Sprintf("%s — networks, IXPs, and carriers present at this facility on PeeringDB Plus.", data.Name),
		Canonical:   canonicalURL(r),
		Content:     templates.FacilityDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
		NeedsMap:    true,
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render facility detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
		Title:       data.Name,
		Description: fmt.Sprintf("%s — networks, IXPs, and facilities operated by this organization on PeeringDB Plus.", data.Name),
		Canonical:   canonicalURL(r),
		Content:     templates.OrgDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render org detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
		Title:       data.Name,
		Description: fmt.Sprintf("%s — facilities on this campus on PeeringDB Plus.", data.Name),
		Canonical:   canonicalURL(r),
		Content:     templates.CampusDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render campus detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
		Title:       data.Name,
		Description: fmt.Sprintf("%s — facilities served by this carrier on PeeringDB Plus.", data.Name),
		Canonical:   canonicalURL(r),
		Content:     templates.CarrierDetailPage(data),
		Data:        data,
		Freshness:   h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render carrier detail", slog.Int("id", id), slog.Any("error", err))
		h.handleServerError(w, r)
	}
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
	off := fragmentOffset(r)
	items, err := h.client.NetworkIxLan.Query().
		Where(networkixlan.HasNetworkWith(network.ID(netID)), networkixlan.StatusIn("ok", "pending")).
		Order(networkixlan.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query network ixlans", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.NetworkIXLansList(rows, moreURL)
	if off > 0 {
		frag = templates.NetworkIXLansRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render network ixlans fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleNetFacilitiesFragment returns an HTML fragment listing a network's facility presences.
func (h *Handler) handleNetFacilitiesFragment(w http.ResponseWriter, r *http.Request, netID int) {
	off := fragmentOffset(r)
	items, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasNetworkWith(network.ID(netID)), networkfacility.StatusIn("ok", "pending")).
		Order(networkfacility.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query network facilities", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.NetworkFacilitiesList(rows, moreURL)
	if off > 0 {
		frag = templates.NetworkFacilitiesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render network facilities fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleNetContactsFragment returns an HTML fragment listing a network's contacts.
func (h *Handler) handleNetContactsFragment(w http.ResponseWriter, r *http.Request, netID int) {
	off := fragmentOffset(r)
	items, err := h.client.Poc.Query().
		Where(poc.HasNetworkWith(network.ID(netID)), poc.StatusIn("ok", "pending")).
		Order(poc.ByRole(), poc.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query network contacts", slog.Int("network_id", netID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.NetworkContactsList(rows, moreURL)
	if off > 0 {
		frag = templates.NetworkContactsRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render network contacts fragment", slog.Int("network_id", netID), slog.Any("error", err))
	}
}

// handleIXParticipantsFragment returns an HTML fragment listing an IXP's participants.
// Uses the IxLan -> NetworkIxLan path (do NOT go directly from InternetExchange).
func (h *Handler) handleIXParticipantsFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	off := fragmentOffset(r)
	items, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(ixID)), ixlan.StatusIn("ok", "pending")).
		QueryNetworkIxLans().
		Where(networkixlan.StatusIn("ok", "pending")).
		WithNetwork().
		Order(networkixlan.ByAsn()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query ix participants", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.IXParticipantsList(rows, moreURL)
	if off > 0 {
		frag = templates.IXParticipantsRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render ix participants fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleIXFacilitiesFragment returns an HTML fragment listing an IXP's facilities.
func (h *Handler) handleIXFacilitiesFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	off := fragmentOffset(r)
	items, err := h.client.IxFacility.Query().
		Where(ixfacility.HasInternetExchangeWith(internetexchange.ID(ixID)), ixfacility.StatusIn("ok", "pending")).
		Order(ixfacility.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query ix facilities", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.IXFacilitiesList(rows, moreURL)
	if off > 0 {
		frag = templates.IXFacilitiesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render ix facilities fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleIXPrefixesFragment returns an HTML fragment listing an IXP's prefixes.
// Uses the IxLan -> IxPrefix path (do NOT go directly from InternetExchange).
func (h *Handler) handleIXPrefixesFragment(w http.ResponseWriter, r *http.Request, ixID int) {
	off := fragmentOffset(r)
	items, err := h.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(ixID)), ixlan.StatusIn("ok", "pending")).
		QueryIxPrefixes().
		Where(ixprefix.StatusIn("ok", "pending")).
		Order(ixprefix.ByPrefix()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query ix prefixes", slog.Int("ix_id", ixID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.IXPrefixRow, len(items))
	for i, p := range items {
		rows[i] = templates.IXPrefixRow{
			Prefix:   p.Prefix,
			Protocol: p.Protocol,
			InDFZ:    p.InDfz,
		}
	}

	frag := templates.IXPrefixesList(rows, moreURL)
	if off > 0 {
		frag = templates.IXPrefixesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render ix prefixes fragment", slog.Int("ix_id", ixID), slog.Any("error", err))
	}
}

// handleFacNetworksFragment returns an HTML fragment listing a facility's networks.
func (h *Handler) handleFacNetworksFragment(w http.ResponseWriter, r *http.Request, facID int) {
	off := fragmentOffset(r)
	items, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasFacilityWith(facility.ID(facID)), networkfacility.StatusIn("ok", "pending")).
		WithNetwork().
		Order(networkfacility.ByNetworkField(network.FieldName)).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query fac networks", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.FacNetworkRow, len(items))
	for i, nf := range items {
		netName := ""
		if nf.Edges.Network != nil {
			netName = nf.Edges.Network.Name
		}
		rows[i] = templates.FacNetworkRow{
			NetName: netName,
			ASN:     nf.LocalAsn,
			City:    nf.City,
			Country: nf.Country,
		}
	}

	frag := templates.FacNetworksList(rows, moreURL)
	if off > 0 {
		frag = templates.FacNetworksRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render fac networks fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleFacIXPsFragment returns an HTML fragment listing a facility's IXPs.
func (h *Handler) handleFacIXPsFragment(w http.ResponseWriter, r *http.Request, facID int) {
	off := fragmentOffset(r)
	items, err := h.client.IxFacility.Query().
		Where(ixfacility.HasFacilityWith(facility.ID(facID)), ixfacility.StatusIn("ok", "pending")).
		WithInternetExchange().
		Order(ixfacility.ByInternetExchangeField(internetexchange.FieldName)).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query fac ixps", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.FacIXPsList(rows, moreURL)
	if off > 0 {
		frag = templates.FacIXPsRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render fac ixps fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleFacCarriersFragment returns an HTML fragment listing a facility's carriers.
func (h *Handler) handleFacCarriersFragment(w http.ResponseWriter, r *http.Request, facID int) {
	off := fragmentOffset(r)
	items, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasFacilityWith(facility.ID(facID)), carrierfacility.StatusIn("ok", "pending")).
		WithCarrier().
		Order(carrierfacility.ByCarrierField(carrier.FieldName)).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query fac carriers", slog.Int("fac_id", facID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	var rows []templates.FacCarrierRow
	for _, cf := range items {
		if cf.CarrierID == nil || cf.Edges.Carrier == nil {
			continue
		}
		rows = append(rows, templates.FacCarrierRow{
			CarrierName: cf.Edges.Carrier.Name,
			CarrierID:   *cf.CarrierID,
		})
	}

	frag := templates.FacCarriersList(rows, moreURL)
	if off > 0 {
		frag = templates.FacCarriersRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render fac carriers fragment", slog.Int("fac_id", facID), slog.Any("error", err))
	}
}

// handleOrgNetworksFragment returns an HTML fragment listing an org's networks.
func (h *Handler) handleOrgNetworksFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	off := fragmentOffset(r)
	items, err := h.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(orgID)), network.StatusIn("ok", "pending")).
		Order(network.ByAsn()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query org networks", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.OrgNetworkRow, len(items))
	for i, n := range items {
		rows[i] = templates.OrgNetworkRow{
			NetName: n.Name,
			ASN:     n.Asn,
		}
	}

	frag := templates.OrgNetworksList(rows, moreURL)
	if off > 0 {
		frag = templates.OrgNetworksRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render org networks fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgIXPsFragment returns an HTML fragment listing an org's IXPs.
func (h *Handler) handleOrgIXPsFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	off := fragmentOffset(r)
	items, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(orgID)), internetexchange.StatusIn("ok", "pending")).
		Order(internetexchange.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query org ixps", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.OrgIXRow, len(items))
	for i, ix := range items {
		rows[i] = templates.OrgIXRow{
			IXName: ix.Name,
			IXID:   ix.ID,
		}
	}

	frag := templates.OrgIXPsList(rows, moreURL)
	if off > 0 {
		frag = templates.OrgIXPsRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render org ixps fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgFacilitiesFragment returns an HTML fragment listing an org's facilities.
func (h *Handler) handleOrgFacilitiesFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	off := fragmentOffset(r)
	items, err := h.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(orgID)), facility.StatusIn("ok", "pending")).
		Order(facility.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query org facilities", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.OrgFacilityRow, len(items))
	for i, f := range items {
		rows[i] = templates.OrgFacilityRow{
			FacName: f.Name,
			FacID:   f.ID,
			City:    f.City,
			Country: f.Country,
		}
	}

	frag := templates.OrgFacilitiesList(rows, moreURL)
	if off > 0 {
		frag = templates.OrgFacilitiesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render org facilities fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgCampusesFragment returns an HTML fragment listing an org's campuses.
func (h *Handler) handleOrgCampusesFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	off := fragmentOffset(r)
	items, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(orgID)), campus.StatusIn("ok", "pending")).
		Order(campus.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query org campuses", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.OrgCampusRow, len(items))
	for i, c := range items {
		rows[i] = templates.OrgCampusRow{
			CampusName: c.Name,
			CampusID:   c.ID,
		}
	}

	frag := templates.OrgCampusesList(rows, moreURL)
	if off > 0 {
		frag = templates.OrgCampusesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render org campuses fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleOrgCarriersFragment returns an HTML fragment listing an org's carriers.
func (h *Handler) handleOrgCarriersFragment(w http.ResponseWriter, r *http.Request, orgID int) {
	off := fragmentOffset(r)
	items, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(orgID)), carrier.StatusIn("ok", "pending")).
		Order(carrier.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query org carriers", slog.Int("org_id", orgID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.OrgCarrierRow, len(items))
	for i, c := range items {
		rows[i] = templates.OrgCarrierRow{
			CarrierName: c.Name,
			CarrierID:   c.ID,
		}
	}

	frag := templates.OrgCarriersList(rows, moreURL)
	if off > 0 {
		frag = templates.OrgCarriersRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render org carriers fragment", slog.Int("org_id", orgID), slog.Any("error", err))
	}
}

// handleCampusFacilitiesFragment returns an HTML fragment listing a campus's facilities.
func (h *Handler) handleCampusFacilitiesFragment(w http.ResponseWriter, r *http.Request, campusID int) {
	off := fragmentOffset(r)
	items, err := h.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(campusID)), facility.StatusIn("ok", "pending")).
		Order(facility.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query campus facilities", slog.Int("campus_id", campusID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

	rows := make([]templates.CampusFacilityRow, len(items))
	for i, f := range items {
		rows[i] = templates.CampusFacilityRow{
			FacName: f.Name,
			FacID:   f.ID,
			City:    f.City,
			Country: f.Country,
		}
	}

	frag := templates.CampusFacilitiesList(rows, moreURL)
	if off > 0 {
		frag = templates.CampusFacilitiesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render campus facilities fragment", slog.Int("campus_id", campusID), slog.Any("error", err))
	}
}

// handleCarrierFacilitiesFragment returns an HTML fragment listing a carrier's facilities.
func (h *Handler) handleCarrierFacilitiesFragment(w http.ResponseWriter, r *http.Request, carrierID int) {
	off := fragmentOffset(r)
	items, err := h.client.CarrierFacility.Query().
		Where(carrierfacility.HasCarrierWith(carrier.ID(carrierID)), carrierfacility.StatusIn("ok", "pending")).
		Order(carrierfacility.ByName()).
		Offset(off).
		Limit(fragmentPageSize + 1).
		All(r.Context())
	if err != nil {
		slog.Error("query carrier facilities", slog.Int("carrier_id", carrierID), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	items, hasMore := trimPage(items, fragmentPageSize)
	moreURL := fragmentMoreURL(r, off, hasMore)

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

	frag := templates.CarrierFacilitiesList(rows, moreURL)
	if off > 0 {
		frag = templates.CarrierFacilitiesRows(rows, moreURL)
	}
	if err := frag.Render(r.Context(), w); err != nil {
		slog.Error("render carrier facilities fragment", slog.Int("carrier_id", carrierID), slog.Any("error", err))
	}
}
