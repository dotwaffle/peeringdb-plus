package web

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// queryOrg fetches an organization by ID and all related data for the detail page.
// Returns the fully populated OrgDetail or an error (including ent.IsNotFound).
func (h *Handler) queryOrg(ctx context.Context, id int) (templates.OrgDetail, error) {
	org, err := h.client.Organization.Query().
		Where(organization.ID(id)).
		Only(ctx)
	if err != nil {
		return templates.OrgDetail{}, fmt.Errorf("query org %d: %w", id, err)
	}

	// Count non-pre-computed child entity counts.
	ixCount, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org IXPs", slog.Int("org_id", id), slog.Any("error", err))
	}

	campusCount, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	carrierCount, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org carriers", slog.Int("org_id", id), slog.Any("error", err))
	}

	netCount, err := h.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org networks", slog.Int("org_id", id), slog.Any("error", err))
	}

	facCount, err := h.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(id))).
		Count(ctx)
	if err != nil {
		slog.Error("count org facilities", slog.Int("org_id", id), slog.Any("error", err))
	}

	data := templates.OrgDetail{
		ID:           org.ID,
		Name:         org.Name,
		NameLong:     org.NameLong,
		AKA:          org.Aka,
		Website:      org.Website,
		Address1:     org.Address1,
		Address2:     org.Address2,
		City:         org.City,
		State:        org.State,
		Country:      org.Country,
		Zipcode:      org.Zipcode,
		Notes:        org.Notes,
		Status:       org.Status,
		NetCount:     netCount,
		FacCount:     facCount,
		IXCount:      ixCount,
		CampusCount:  campusCount,
		CarrierCount: carrierCount,
	}

	// Eager-load org networks.
	orgNetItems, err := h.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(id))).
		Order(network.ByAsn()).
		All(ctx)
	if err == nil {
		netRows := make([]templates.OrgNetworkRow, len(orgNetItems))
		for i, n := range orgNetItems {
			netRows[i] = templates.OrgNetworkRow{
				NetName: n.Name,
				ASN:     n.Asn,
			}
		}
		data.Networks = netRows
	} else {
		slog.Error("eager-load org networks", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org IXPs.
	orgIXItems, err := h.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id))).
		Order(internetexchange.ByName()).
		All(ctx)
	if err == nil {
		ixRows := make([]templates.OrgIXRow, len(orgIXItems))
		for i, ix := range orgIXItems {
			ixRows[i] = templates.OrgIXRow{
				IXName: ix.Name,
				IXID:   ix.ID,
			}
		}
		data.IXPs = ixRows
	} else {
		slog.Error("eager-load org ixps", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org facilities.
	orgFacItems, err := h.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(id))).
		Order(facility.ByName()).
		All(ctx)
	if err == nil {
		facRows := make([]templates.OrgFacilityRow, len(orgFacItems))
		for i, f := range orgFacItems {
			facRows[i] = templates.OrgFacilityRow{
				FacName: f.Name,
				FacID:   f.ID,
				City:    f.City,
				Country: f.Country,
			}
		}
		data.Facs = facRows
	} else {
		slog.Error("eager-load org facilities", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org campuses.
	orgCampusItems, err := h.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id))).
		Order(campus.ByName()).
		All(ctx)
	if err == nil {
		campusRows := make([]templates.OrgCampusRow, len(orgCampusItems))
		for i, c := range orgCampusItems {
			campusRows[i] = templates.OrgCampusRow{
				CampusName: c.Name,
				CampusID:   c.ID,
			}
		}
		data.Campuses = campusRows
	} else {
		slog.Error("eager-load org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org carriers.
	orgCarrierItems, err := h.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id))).
		Order(carrier.ByName()).
		All(ctx)
	if err == nil {
		carrierRows := make([]templates.OrgCarrierRow, len(orgCarrierItems))
		for i, c := range orgCarrierItems {
			carrierRows[i] = templates.OrgCarrierRow{
				CarrierName: c.Name,
				CarrierID:   c.ID,
			}
		}
		data.Carriers = carrierRows
	} else {
		slog.Error("eager-load org carriers", slog.Int("org_id", id), slog.Any("error", err))
	}

	return data, nil
}
