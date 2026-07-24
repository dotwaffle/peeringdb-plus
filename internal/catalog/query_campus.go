package catalog

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
)

// Campus fetches a campus by ID and its related catalog data.
// It returns errors compatible with ent.IsNotFound.
func (s *Service) Campus(ctx context.Context, id int) (CampusDetail, error) {
	c, err := s.client.Campus.Query().
		Where(campus.ID(id), campus.StatusIn("ok", "pending")).
		WithOrganization().
		Only(ctx)
	if err != nil {
		return CampusDetail{}, fmt.Errorf("query campus %d: %w", id, err)
	}

	facCount, err := s.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(id)), facility.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count campus facilities", slog.Int("campus_id", id), slog.Any("error", err))
	}

	data := CampusDetail{
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
	campusFacItems, err := s.client.Facility.Query().
		Where(facility.HasCampusWith(campus.ID(id)), facility.StatusIn("ok", "pending")).
		Order(facility.ByName()).
		All(ctx)
	if err == nil {
		facRows := make([]CampusFacilityRow, len(campusFacItems))
		for i, f := range campusFacItems {
			facRows[i] = CampusFacilityRow{
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
