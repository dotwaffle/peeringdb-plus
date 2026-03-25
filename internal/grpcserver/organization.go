package grpcserver

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// OrganizationService implements the peeringdb.v1.OrganizationService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type OrganizationService struct {
	Client *ent.Client
}

// GetOrganization returns a single organization by ID. Returns NOT_FOUND if
// the organization does not exist.
func (s *OrganizationService) GetOrganization(ctx context.Context, req *pb.GetOrganizationRequest) (*pb.GetOrganizationResponse, error) {
	o, err := s.Client.Organization.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity organization %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get organization %d: %w", req.GetId(), err))
	}
	return &pb.GetOrganizationResponse{Organization: organizationToProto(o)}, nil
}

// ListOrganizations returns a paginated list of organizations ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields (name,
// country, city, status). Multiple filters combine with AND logic.
func (s *OrganizationService) ListOrganizations(ctx context.Context, req *pb.ListOrganizationsRequest) (*pb.ListOrganizationsResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Organization
	if req.Name != nil {
		predicates = append(predicates, organization.NameContainsFold(*req.Name))
	}
	if req.Country != nil {
		predicates = append(predicates, organization.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, organization.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, organization.StatusEQ(*req.Status))
	}

	query := s.Client.Organization.Query().
		Order(ent.Asc(organization.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(organization.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list organizations: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	orgs := make([]*pb.Organization, len(results))
	for i, o := range results {
		orgs[i] = organizationToProto(o)
	}

	return &pb.ListOrganizationsResponse{
		Organizations: orgs,
		NextPageToken: nextPageToken,
	}, nil
}

// organizationToProto converts an ent Organization entity to a protobuf
// Organization message.
func organizationToProto(o *ent.Organization) *pb.Organization {
	return &pb.Organization{
		Id:        int64(o.ID),
		Address1:  stringVal(o.Address1),
		Address2:  stringVal(o.Address2),
		Aka:       stringVal(o.Aka),
		City:      stringVal(o.City),
		Country:   stringVal(o.Country),
		Floor:     stringVal(o.Floor),
		Latitude:  float64PtrVal(o.Latitude),
		Logo:      stringPtrVal(o.Logo),
		Longitude: float64PtrVal(o.Longitude),
		Name:      o.Name,
		NameLong:  stringVal(o.NameLong),
		Notes:     stringVal(o.Notes),
		State:     stringVal(o.State),
		Suite:     stringVal(o.Suite),
		Website:   stringVal(o.Website),
		Zipcode:   stringVal(o.Zipcode),
		NetCount:  int64Val(o.NetCount),
		FacCount:  int64Val(o.FacCount),
		Created:   timestampVal(o.Created),
		Updated:   timestampVal(o.Updated),
		Status:    o.Status,
	}
}
