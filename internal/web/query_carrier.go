package web

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

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
