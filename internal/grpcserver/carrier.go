package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CarrierService implements the peeringdb.v1.CarrierService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type CarrierService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCarrier returns a single carrier by ID. Returns NOT_FOUND if the carrier
// does not exist.
func (s *CarrierService) GetCarrier(ctx context.Context, req *pb.GetCarrierRequest) (*pb.GetCarrierResponse, error) {
	c, err := s.Client.Carrier.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity carrier %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get carrier %d: %w", req.GetId(), err))
	}
	return &pb.GetCarrierResponse{Carrier: carrierToProto(c)}, nil
}

// ListCarriers returns a paginated list of carriers ordered by ID ascending.
// Supports page_size, page_token, and optional filter fields (name, status,
// org_id). Multiple filters combine with AND logic.
func (s *CarrierService) ListCarriers(ctx context.Context, req *pb.ListCarriersRequest) (*pb.ListCarriersResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Carrier
	if req.Name != nil {
		predicates = append(predicates, carrier.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, carrier.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, carrier.OrgIDEQ(int(*req.OrgId)))
	}

	query := s.Client.Carrier.Query().
		Order(ent.Asc(carrier.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(carrier.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list carriers: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	carriers := make([]*pb.Carrier, len(results))
	for i, c := range results {
		carriers[i] = carrierToProto(c)
	}

	return &pb.ListCarriersResponse{
		Carriers:      carriers,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamCarriers streams all matching carriers one message at a time using
// batched keyset pagination. Returns Unimplemented until handler logic is added.
func (s *CarrierService) StreamCarriers(_ context.Context, _ *pb.StreamCarriersRequest, _ *connect.ServerStream[pb.Carrier]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamCarriers not yet implemented"))
}

// carrierToProto converts an ent Carrier entity to a protobuf Carrier message.
func carrierToProto(c *ent.Carrier) *pb.Carrier {
	return &pb.Carrier{
		Id:       int64(c.ID),
		OrgId:    int64PtrVal(c.OrgID),
		Aka:      stringVal(c.Aka),
		Logo:     stringPtrVal(c.Logo),
		Name:     c.Name,
		NameLong: stringVal(c.NameLong),
		Notes:    stringVal(c.Notes),
		Website:  stringVal(c.Website),
		OrgName:  stringVal(c.OrgName),
		FacCount: int64Val(c.FacCount),
		Created:  timestampVal(c.Created),
		Updated:  timestampVal(c.Updated),
		Status:   c.Status,
	}
}
