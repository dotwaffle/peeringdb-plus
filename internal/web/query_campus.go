package web

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

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
