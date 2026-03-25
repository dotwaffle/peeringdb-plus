package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CampusService implements the peeringdb.v1.CampusService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type CampusService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCampus returns a single campus by ID. Returns NOT_FOUND if the campus
// does not exist.
func (s *CampusService) GetCampus(ctx context.Context, req *pb.GetCampusRequest) (*pb.GetCampusResponse, error) {
	c, err := s.Client.Campus.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity campus %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get campus %d: %w", req.GetId(), err))
	}
	return &pb.GetCampusResponse{Campus: campusToProto(c)}, nil
}

// ListCampuses returns a paginated list of campuses ordered by ID ascending.
// Supports page_size, page_token, and optional filter fields (name, country,
// city, status, org_id). Multiple filters combine with AND logic.
func (s *CampusService) ListCampuses(ctx context.Context, req *pb.ListCampusesRequest) (*pb.ListCampusesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Campus
	if req.Name != nil {
		predicates = append(predicates, campus.NameContainsFold(*req.Name))
	}
	if req.Country != nil {
		predicates = append(predicates, campus.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, campus.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, campus.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, campus.OrgIDEQ(int(*req.OrgId)))
	}

	query := s.Client.Campus.Query().
		Order(ent.Asc(campus.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(campus.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list campuses: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	campuses := make([]*pb.Campus, len(results))
	for i, c := range results {
		campuses[i] = campusToProto(c)
	}

	return &pb.ListCampusesResponse{
		Campuses:      campuses,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamCampuses streams all matching campuses one message at a time using
// batched keyset pagination. Returns Unimplemented until handler logic is added.
func (s *CampusService) StreamCampuses(_ context.Context, _ *pb.StreamCampusesRequest, _ *connect.ServerStream[pb.Campus]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamCampuses not yet implemented"))
}

// campusToProto converts an ent Campus entity to a protobuf Campus message.
func campusToProto(c *ent.Campus) *pb.Campus {
	return &pb.Campus{
		Id:      int64(c.ID),
		OrgId:   int64PtrVal(c.OrgID),
		Aka:     stringPtrVal(c.Aka),
		City:    stringVal(c.City),
		Country: stringVal(c.Country),
		Logo:    stringPtrVal(c.Logo),
		Name:    c.Name,
		NameLong: stringPtrVal(c.NameLong),
		Notes:   stringVal(c.Notes),
		State:   stringVal(c.State),
		Website: stringVal(c.Website),
		Zipcode: stringVal(c.Zipcode),
		OrgName: stringVal(c.OrgName),
		Created: timestampVal(c.Created),
		Updated: timestampVal(c.Updated),
		Status:  c.Status,
	}
}
