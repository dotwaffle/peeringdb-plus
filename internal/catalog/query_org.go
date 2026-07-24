package catalog

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
)

// Organization fetches an organization by ID and its related catalog data.
// It returns errors compatible with ent.IsNotFound.
func (s *Service) Organization(ctx context.Context, id int) (OrgDetail, error) {
	org, err := s.client.Organization.Query().
		Where(organization.ID(id), organization.StatusIn("ok", "pending")).
		Only(ctx)
	if err != nil {
		return OrgDetail{}, fmt.Errorf("query org %d: %w", id, err)
	}

	// Count non-pre-computed child entity counts.
	ixCount, err := s.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id)), internetexchange.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count org IXPs", slog.Int("org_id", id), slog.Any("error", err))
	}

	campusCount, err := s.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id)), campus.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	carrierCount, err := s.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id)), carrier.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count org carriers", slog.Int("org_id", id), slog.Any("error", err))
	}

	netCount, err := s.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(id)), network.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count org networks", slog.Int("org_id", id), slog.Any("error", err))
	}

	facCount, err := s.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(id)), facility.StatusIn("ok", "pending")).
		Count(ctx)
	if err != nil {
		slog.Error("count org facilities", slog.Int("org_id", id), slog.Any("error", err))
	}

	data := OrgDetail{
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
	orgNetItems, err := s.client.Network.Query().
		Where(network.HasOrganizationWith(organization.ID(id)), network.StatusIn("ok", "pending")).
		Order(network.ByAsn()).
		All(ctx)
	if err == nil {
		netRows := make([]OrgNetworkRow, len(orgNetItems))
		for i, n := range orgNetItems {
			netRows[i] = OrgNetworkRow{
				NetName: n.Name,
				ASN:     n.Asn,
			}
		}
		data.Networks = netRows
	} else {
		slog.Error("eager-load org networks", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org IXPs.
	orgIXItems, err := s.client.InternetExchange.Query().
		Where(internetexchange.HasOrganizationWith(organization.ID(id)), internetexchange.StatusIn("ok", "pending")).
		Order(internetexchange.ByName()).
		All(ctx)
	if err == nil {
		ixRows := make([]OrgIXRow, len(orgIXItems))
		for i, ix := range orgIXItems {
			ixRows[i] = OrgIXRow{
				IXName: ix.Name,
				IXID:   ix.ID,
			}
		}
		data.IXPs = ixRows
	} else {
		slog.Error("eager-load org ixps", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org facilities.
	orgFacItems, err := s.client.Facility.Query().
		Where(facility.HasOrganizationWith(organization.ID(id)), facility.StatusIn("ok", "pending")).
		Order(facility.ByName()).
		All(ctx)
	if err == nil {
		facRows := make([]OrgFacilityRow, len(orgFacItems))
		for i, f := range orgFacItems {
			facRows[i] = OrgFacilityRow{
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
	orgCampusItems, err := s.client.Campus.Query().
		Where(campus.HasOrganizationWith(organization.ID(id)), campus.StatusIn("ok", "pending")).
		Order(campus.ByName()).
		All(ctx)
	if err == nil {
		campusRows := make([]OrgCampusRow, len(orgCampusItems))
		for i, c := range orgCampusItems {
			campusRows[i] = OrgCampusRow{
				CampusName: c.Name,
				CampusID:   c.ID,
			}
		}
		data.Campuses = campusRows
	} else {
		slog.Error("eager-load org campuses", slog.Int("org_id", id), slog.Any("error", err))
	}

	// Eager-load org carriers.
	orgCarrierItems, err := s.client.Carrier.Query().
		Where(carrier.HasOrganizationWith(organization.ID(id)), carrier.StatusIn("ok", "pending")).
		Order(carrier.ByName()).
		All(ctx)
	if err == nil {
		carrierRows := make([]OrgCarrierRow, len(orgCarrierItems))
		for i, c := range orgCarrierItems {
			carrierRows[i] = OrgCarrierRow{
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
