package web

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

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
