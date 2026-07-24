package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
)

// Facility fetches a facility by ID and its related catalog data.
// It returns errors compatible with ent.IsNotFound.
func (s *Service) Facility(ctx context.Context, id int) (FacilityDetail, error) {
	fac, err := s.client.Facility.Query().
		Where(facility.ID(id), facility.StatusIn("ok", "pending")).
		WithOrganization().
		WithCampus().
		Only(ctx)
	if err != nil {
		return FacilityDetail{}, fmt.Errorf("query facility %d: %w", id, err)
	}

	data := FacilityDetail{
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

	// Eager-load facility networks. The association row's own `name` is the
	// facility name, so resolve the network name from the Network edge.
	facNetItems, err := s.client.NetworkFacility.Query().
		Where(networkfacility.HasFacilityWith(facility.ID(id)), networkfacility.StatusIn("ok", "pending")).
		WithNetwork().
		Order(networkfacility.ByNetworkField(network.FieldName)).
		All(ctx)
	if err == nil {
		netRows := make([]FacNetworkRow, len(facNetItems))
		for i, nf := range facNetItems {
			netName := ""
			if nf.Edges.Network != nil {
				netName = nf.Edges.Network.Name
			}
			netRows[i] = FacNetworkRow{
				NetName: netName,
				ASN:     nf.LocalAsn,
				City:    nf.City,
				Country: nf.Country,
			}
		}
		data.Networks = netRows
	} else {
		slog.Error("eager-load fac networks", slog.Int("fac_id", id), slog.Any("error", err))
	}

	// Eager-load facility IXPs. The association row's own `name` is the facility
	// name, so resolve the exchange name from the InternetExchange edge.
	facIXItems, err := s.client.IxFacility.Query().
		Where(ixfacility.HasFacilityWith(facility.ID(id)), ixfacility.StatusIn("ok", "pending")).
		WithInternetExchange().
		Order(ixfacility.ByInternetExchangeField(internetexchange.FieldName)).
		All(ctx)
	if err == nil {
		var ixRows []FacIXRow
		for _, ixf := range facIXItems {
			if ixf.IxID == nil || ixf.Edges.InternetExchange == nil {
				continue
			}
			ixRows = append(ixRows, FacIXRow{
				IXName: ixf.Edges.InternetExchange.Name,
				IXID:   *ixf.IxID,
			})
		}
		data.IXPs = ixRows
	} else {
		slog.Error("eager-load fac ixps", slog.Int("fac_id", id), slog.Any("error", err))
	}

	// Eager-load facility carriers. The association row's own `name` is the
	// facility name, so resolve the carrier name from the Carrier edge.
	facCarrierItems, err := s.client.CarrierFacility.Query().
		Where(carrierfacility.HasFacilityWith(facility.ID(id)), carrierfacility.StatusIn("ok", "pending")).
		WithCarrier().
		Order(carrierfacility.ByCarrierField(carrier.FieldName)).
		All(ctx)
	if err == nil {
		var carrierRows []FacCarrierRow
		for _, cf := range facCarrierItems {
			if cf.CarrierID == nil || cf.Edges.Carrier == nil {
				continue
			}
			carrierRows = append(carrierRows, FacCarrierRow{
				CarrierName: cf.Edges.Carrier.Name,
				CarrierID:   *cf.CarrierID,
			})
		}
		data.Carriers = carrierRows
	} else {
		slog.Error("eager-load fac carriers", slog.Int("fac_id", id), slog.Any("error", err))
	}

	return data, nil
}
