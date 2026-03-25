package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxFacilityService implements the peeringdb.v1.IxFacilityService ConnectRPC
// handler interface. It queries the ent database layer and converts results to
// protobuf messages.
type IxFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetIxFacility returns a single IX facility by ID. Returns NOT_FOUND if the
// IX facility does not exist.
func (s *IxFacilityService) GetIxFacility(ctx context.Context, req *pb.GetIxFacilityRequest) (*pb.GetIxFacilityResponse, error) {
	ixf, err := s.Client.IxFacility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity ixfacility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get ixfacility %d: %w", req.GetId(), err))
	}
	return &pb.GetIxFacilityResponse{IxFacility: ixFacilityToProto(ixf)}, nil
}

// ListIxFacilities returns a paginated list of IX facilities ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields
// (ix_id, fac_id, country, city, status). Multiple filters combine with AND
// logic.
func (s *IxFacilityService) ListIxFacilities(ctx context.Context, req *pb.ListIxFacilitiesRequest) (*pb.ListIxFacilitiesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.IxFacility
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		predicates = append(predicates, ixfacility.IxIDEQ(int(*req.IxId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		predicates = append(predicates, ixfacility.FacIDEQ(int(*req.FacId)))
	}
	if req.Country != nil {
		predicates = append(predicates, ixfacility.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, ixfacility.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, ixfacility.StatusEQ(*req.Status))
	}

	query := s.Client.IxFacility.Query().
		Order(ent.Asc(ixfacility.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(ixfacility.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list ixfacilities: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.IxFacility, len(results))
	for i, ixf := range results {
		items[i] = ixFacilityToProto(ixf)
	}

	return &pb.ListIxFacilitiesResponse{
		IxFacilities:  items,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamIxFacilities streams all matching IX facilities one message at a time
// using batched keyset pagination. Returns Unimplemented until handler logic is
// added.
func (s *IxFacilityService) StreamIxFacilities(_ context.Context, _ *pb.StreamIxFacilitiesRequest, _ *connect.ServerStream[pb.IxFacility]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamIxFacilities not yet implemented"))
}

// ixFacilityToProto converts an ent IxFacility entity to a protobuf IxFacility
// message.
func ixFacilityToProto(ixf *ent.IxFacility) *pb.IxFacility {
	return &pb.IxFacility{
		Id:      int64(ixf.ID),
		FacId:   int64PtrVal(ixf.FacID),
		IxId:    int64PtrVal(ixf.IxID),
		Name:    stringVal(ixf.Name),
		City:    stringVal(ixf.City),
		Country: stringVal(ixf.Country),
		Created: timestampVal(ixf.Created),
		Updated: timestampVal(ixf.Updated),
		Status:  ixf.Status,
	}
}
