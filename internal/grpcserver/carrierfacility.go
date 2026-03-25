package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CarrierFacilityService implements the peeringdb.v1.CarrierFacilityService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type CarrierFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCarrierFacility returns a single carrier facility by ID. Returns
// NOT_FOUND if the carrier facility does not exist.
func (s *CarrierFacilityService) GetCarrierFacility(ctx context.Context, req *pb.GetCarrierFacilityRequest) (*pb.GetCarrierFacilityResponse, error) {
	cf, err := s.Client.CarrierFacility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity carrierfacility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get carrierfacility %d: %w", req.GetId(), err))
	}
	return &pb.GetCarrierFacilityResponse{CarrierFacility: carrierFacilityToProto(cf)}, nil
}

// ListCarrierFacilities returns a paginated list of carrier facilities ordered
// by ID ascending. Supports page_size, page_token, and optional filter fields
// (carrier_id, fac_id, status). Multiple filters combine with AND logic.
func (s *CarrierFacilityService) ListCarrierFacilities(ctx context.Context, req *pb.ListCarrierFacilitiesRequest) (*pb.ListCarrierFacilitiesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.CarrierFacility
	if req.CarrierId != nil {
		if *req.CarrierId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: carrier_id must be positive"))
		}
		predicates = append(predicates, carrierfacility.CarrierIDEQ(int(*req.CarrierId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		predicates = append(predicates, carrierfacility.FacIDEQ(int(*req.FacId)))
	}
	if req.Status != nil {
		predicates = append(predicates, carrierfacility.StatusEQ(*req.Status))
	}

	query := s.Client.CarrierFacility.Query().
		Order(ent.Asc(carrierfacility.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(carrierfacility.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list carrierfacilities: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.CarrierFacility, len(results))
	for i, cf := range results {
		items[i] = carrierFacilityToProto(cf)
	}

	return &pb.ListCarrierFacilitiesResponse{
		CarrierFacilities: items,
		NextPageToken:     nextPageToken,
	}, nil
}

// StreamCarrierFacilities streams all matching carrier facilities one message at
// a time using batched keyset pagination. Returns Unimplemented until handler
// logic is added.
func (s *CarrierFacilityService) StreamCarrierFacilities(_ context.Context, _ *pb.StreamCarrierFacilitiesRequest, _ *connect.ServerStream[pb.CarrierFacility]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamCarrierFacilities not yet implemented"))
}

// carrierFacilityToProto converts an ent CarrierFacility entity to a protobuf
// CarrierFacility message.
func carrierFacilityToProto(cf *ent.CarrierFacility) *pb.CarrierFacility {
	return &pb.CarrierFacility{
		Id:        int64(cf.ID),
		CarrierId: int64PtrVal(cf.CarrierID),
		FacId:     int64PtrVal(cf.FacID),
		Name:      stringVal(cf.Name),
		Created:   timestampVal(cf.Created),
		Updated:   timestampVal(cf.Updated),
		Status:    cf.Status,
	}
}
