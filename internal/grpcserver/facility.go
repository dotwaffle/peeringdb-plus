package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// FacilityService implements the peeringdb.v1.FacilityService ConnectRPC
// handler interface. It queries the ent database layer and converts results to
// protobuf messages.
type FacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetFacility returns a single facility by ID. Returns NOT_FOUND if the
// facility does not exist.
func (s *FacilityService) GetFacility(ctx context.Context, req *pb.GetFacilityRequest) (*pb.GetFacilityResponse, error) {
	f, err := s.Client.Facility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity facility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get facility %d: %w", req.GetId(), err))
	}
	return &pb.GetFacilityResponse{Facility: facilityToProto(f)}, nil
}

// ListFacilities returns a paginated list of facilities ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields (name,
// country, city, status, org_id). Multiple filters combine with AND logic.
func (s *FacilityService) ListFacilities(ctx context.Context, req *pb.ListFacilitiesRequest) (*pb.ListFacilitiesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Facility
	if req.Name != nil {
		predicates = append(predicates, facility.NameContainsFold(*req.Name))
	}
	if req.Country != nil {
		predicates = append(predicates, facility.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, facility.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, facility.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, facility.OrgIDEQ(int(*req.OrgId)))
	}

	query := s.Client.Facility.Query().
		Order(ent.Asc(facility.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(facility.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list facilities: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	facilities := make([]*pb.Facility, len(results))
	for i, f := range results {
		facilities[i] = facilityToProto(f)
	}

	return &pb.ListFacilitiesResponse{
		Facilities:    facilities,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamFacilities streams all matching facilities one message at a time using
// batched keyset pagination. Returns Unimplemented until handler logic is added.
func (s *FacilityService) StreamFacilities(_ context.Context, _ *pb.StreamFacilitiesRequest, _ *connect.ServerStream[pb.Facility]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamFacilities not yet implemented"))
}

// facilityToProto converts an ent Facility entity to a protobuf Facility
// message.
func facilityToProto(f *ent.Facility) *pb.Facility {
	return &pb.Facility{
		Id:                        int64(f.ID),
		CampusId:                  int64PtrVal(f.CampusID),
		OrgId:                     int64PtrVal(f.OrgID),
		Address1:                  stringVal(f.Address1),
		Address2:                  stringVal(f.Address2),
		Aka:                       stringVal(f.Aka),
		AvailableVoltageServices:  f.AvailableVoltageServices,
		City:                      stringVal(f.City),
		Clli:                      stringVal(f.Clli),
		Country:                   stringVal(f.Country),
		DiverseServingSubstations: boolPtrVal(f.DiverseServingSubstations),
		Floor:                     stringVal(f.Floor),
		Latitude:                  float64PtrVal(f.Latitude),
		Logo:                      stringPtrVal(f.Logo),
		Longitude:                 float64PtrVal(f.Longitude),
		Name:                      f.Name,
		NameLong:                  stringVal(f.NameLong),
		Notes:                     stringVal(f.Notes),
		Npanxx:                    stringVal(f.Npanxx),
		Property:                  stringPtrVal(f.Property),
		RegionContinent:           stringPtrVal(f.RegionContinent),
		Rencode:                   stringVal(f.Rencode),
		SalesEmail:                stringVal(f.SalesEmail),
		SalesPhone:                stringVal(f.SalesPhone),
		State:                     stringVal(f.State),
		StatusDashboard:           stringPtrVal(f.StatusDashboard),
		Suite:                     stringVal(f.Suite),
		TechEmail:                 stringVal(f.TechEmail),
		TechPhone:                 stringVal(f.TechPhone),
		Website:                   stringVal(f.Website),
		Zipcode:                   stringVal(f.Zipcode),
		OrgName:                   stringVal(f.OrgName),
		NetCount:                  int64Val(f.NetCount),
		IxCount:                   int64Val(f.IxCount),
		CarrierCount:              int64Val(f.CarrierCount),
		Created:                   timestampVal(f.Created),
		Updated:                   timestampVal(f.Updated),
		Status:                    f.Status,
	}
}
