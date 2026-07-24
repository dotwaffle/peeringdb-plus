package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
)

// Carrier fetches a carrier by ID and its related catalog data.
// It returns errors compatible with ent.IsNotFound.
func (s *Service) Carrier(ctx context.Context, id int) (CarrierDetail, error) {
	cr, err := s.client.Carrier.Query().
		Where(carrier.ID(id), carrier.StatusIn("ok", "pending")).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return CarrierDetail{}, fmt.Errorf("query carrier %d: %w", id, err)
	}

	data := CarrierDetail{
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
	carrierFacItems, err := s.client.CarrierFacility.Query().
		Where(carrierfacility.HasCarrierWith(carrier.ID(id)), carrierfacility.StatusIn("ok", "pending")).
		Order(carrierfacility.ByName()).
		All(ctx)
	if err == nil {
		var facRows []CarrierFacilityRow
		for _, cf := range carrierFacItems {
			if cf.FacID == nil {
				continue
			}
			facRows = append(facRows, CarrierFacilityRow{
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
