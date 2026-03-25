package grpcserver

import (
	"context"
	"fmt"
	"strconv"
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
// batched keyset pagination. Filters match the ListCarriers behavior.
func (s *CarrierService) StreamCarriers(ctx context.Context, req *pb.StreamCarriersRequest, stream *connect.ServerStream[pb.Carrier]) error {
	// Apply stream timeout.
	if s.StreamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
		defer cancel()
	}

	// Build filter predicates (identical to ListCarriers).
	var predicates []predicate.Carrier
	if req.Name != nil {
		predicates = append(predicates, carrier.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, carrier.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, carrier.OrgIDEQ(int(*req.OrgId)))
	}

	// Count total matching records for header metadata.
	countQuery := s.Client.Carrier.Query()
	if len(predicates) > 0 {
		countQuery = countQuery.Where(carrier.And(predicates...))
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("count carriers: %w", err))
	}
	stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

	// Stream records in batches using keyset pagination.
	lastID := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := s.Client.Carrier.Query().
			Where(carrier.IDGT(lastID)).
			Order(ent.Asc(carrier.FieldID)).
			Limit(streamBatchSize)
		if len(predicates) > 0 {
			query = query.Where(carrier.And(predicates...))
		}

		batch, err := query.All(ctx)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream carriers batch after id %d: %w", lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, c := range batch {
			if err := stream.Send(carrierToProto(c)); err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < streamBatchSize {
			return nil
		}
	}
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
