package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
)

// IX fetches an internet exchange by ID and its related catalog data.
// It returns errors compatible with ent.IsNotFound.
func (s *Service) IX(ctx context.Context, id int) (IXDetail, error) {
	ix, err := s.client.InternetExchange.Query().
		Where(internetexchange.ID(id), internetexchange.StatusIn("ok", "pending")).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return IXDetail{}, fmt.Errorf("query ix %d: %w", id, err)
	}

	data := IXDetail{
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
	prefixCount, err := s.client.IxPrefix.Query().
		Where(
			ixprefix.HasIxLanWith(ixlan.HasInternetExchangeWith(internetexchange.ID(id)), ixlan.StatusIn("ok", "pending")),
			ixprefix.StatusIn("ok", "pending"),
		).
		Count(ctx)
	if err == nil {
		data.PrefixCount = prefixCount
	}
	if ix.Edges.Organization != nil {
		data.OrgName = ix.Edges.Organization.Name
		data.OrgID = ix.Edges.Organization.ID
	}

	// Compute aggregate bandwidth and eager-load participant rows.
	ixParticipants, err := s.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(id)), ixlan.StatusIn("ok", "pending")).
		QueryNetworkIxLans().
		Where(networkixlan.StatusIn("ok", "pending")).
		WithNetwork().
		Order(networkixlan.ByAsn()).
		All(ctx)
	if err == nil {
		var totalBW int
		rows := make([]IXParticipantRow, len(ixParticipants))
		for i, nix := range ixParticipants {
			totalBW += nix.Speed
			netName := ""
			if net := nix.Edges.Network; net != nil {
				netName = net.Name
			}
			row := IXParticipantRow{
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
	ixFacItems, err := s.client.IxFacility.Query().
		Where(ixfacility.HasInternetExchangeWith(internetexchange.ID(id)), ixfacility.StatusIn("ok", "pending")).
		WithFacility(). // Eager-load facility entity for lat/lng
		Order(ixfacility.ByName()).
		All(ctx)
	if err == nil {
		var facRows []IXFacilityRow
		for _, ixf := range ixFacItems {
			if ixf.FacID == nil {
				continue
			}
			row := IXFacilityRow{
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
	ixPrefixItems, err := s.client.IxLan.Query().
		Where(ixlan.HasInternetExchangeWith(internetexchange.ID(id)), ixlan.StatusIn("ok", "pending")).
		QueryIxPrefixes().
		Where(ixprefix.StatusIn("ok", "pending")).
		Order(ixprefix.ByPrefix()).
		All(ctx)
	if err == nil {
		prefixRows := make([]IXPrefixRow, len(ixPrefixItems))
		for i, p := range ixPrefixItems {
			prefixRows[i] = IXPrefixRow{
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
