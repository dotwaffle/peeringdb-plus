package web

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

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
		slices.SortFunc(ixlans, func(a, b *ent.NetworkIxLan) int {
			return cmp.Compare(a.Name, b.Name)
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
