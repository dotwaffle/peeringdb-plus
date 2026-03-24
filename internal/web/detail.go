package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// handleNetworkDetail renders the network detail page for the given ASN string.
// Looks up the network by ASN (not internal ID) per CONTEXT.md decision.
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
	asn, err := strconv.Atoi(asnStr)
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	net, err := h.client.Network.Query().
		Where(network.Asn(asn)).
		WithOrganization().
		First(r.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("query network by ASN", slog.Int("asn", asn), slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	pocCount, err := h.client.Poc.Query().
		Where(poc.HasNetworkWith(network.ID(net.ID))).
		Count(r.Context())
	if err != nil {
		slog.Error("count network contacts", slog.Int("network_id", net.ID), slog.String("error", err.Error()))
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

	page := PageContent{
		Title:   net.Name,
		Content: templates.NetworkDetailPage(data),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render network detail", slog.Int("asn", asn), slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleIXDetail renders the IXP detail page for the given ID.
func (h *Handler) handleIXDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleFacilityDetail renders the facility detail page for the given ID.
func (h *Handler) handleFacilityDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleOrgDetail renders the organization detail page for the given ID.
func (h *Handler) handleOrgDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleCampusDetail renders the campus detail page for the given ID.
func (h *Handler) handleCampusDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleCarrierDetail renders the carrier detail page for the given ID.
func (h *Handler) handleCarrierDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
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
	// Stubs for Plan 02:
	case "ix", "fac", "org", "campus", "carrier":
		h.handleNotFound(w, r)
	default:
		h.handleNotFound(w, r)
	}
}

// handleNetIXLansFragment returns an HTML fragment listing a network's IX presences.
func (h *Handler) handleNetIXLansFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.NetworkIxLan.Query().
		Where(networkixlan.HasNetworkWith(network.ID(netID))).
		All(r.Context())
	if err != nil {
		slog.Error("query network ixlans", slog.Int("network_id", netID), slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
		slog.Error("render network ixlans fragment", slog.Int("network_id", netID), slog.String("error", err.Error()))
	}
}

// handleNetFacilitiesFragment returns an HTML fragment listing a network's facility presences.
func (h *Handler) handleNetFacilitiesFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.NetworkFacility.Query().
		Where(networkfacility.HasNetworkWith(network.ID(netID))).
		All(r.Context())
	if err != nil {
		slog.Error("query network facilities", slog.Int("network_id", netID), slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
		slog.Error("render network facilities fragment", slog.Int("network_id", netID), slog.String("error", err.Error()))
	}
}

// handleNetContactsFragment returns an HTML fragment listing a network's contacts.
func (h *Handler) handleNetContactsFragment(w http.ResponseWriter, r *http.Request, netID int) {
	items, err := h.client.Poc.Query().
		Where(poc.HasNetworkWith(network.ID(netID))).
		All(r.Context())
	if err != nil {
		slog.Error("query network contacts", slog.Int("network_id", netID), slog.String("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
		slog.Error("render network contacts fragment", slog.Int("network_id", netID), slog.String("error", err.Error()))
	}
}

