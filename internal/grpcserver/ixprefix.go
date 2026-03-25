package grpcserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxPrefixService implements the peeringdb.v1.IxPrefixService ConnectRPC
// handler interface. It queries the ent database layer and converts results to
// protobuf messages.
type IxPrefixService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetIxPrefix returns a single IX prefix by ID. Returns NOT_FOUND if the IX
// prefix does not exist.
func (s *IxPrefixService) GetIxPrefix(ctx context.Context, req *pb.GetIxPrefixRequest) (*pb.GetIxPrefixResponse, error) {
	ixp, err := s.Client.IxPrefix.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity ixprefix %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get ixprefix %d: %w", req.GetId(), err))
	}
	return &pb.GetIxPrefixResponse{IxPrefix: ixPrefixToProto(ixp)}, nil
}

// ListIxPrefixes returns a paginated list of IX prefixes ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields
// (ixlan_id, protocol, status). Multiple filters combine with AND logic.
func (s *IxPrefixService) ListIxPrefixes(ctx context.Context, req *pb.ListIxPrefixesRequest) (*pb.ListIxPrefixesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.IxPrefix
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		predicates = append(predicates, ixprefix.IxlanIDEQ(int(*req.IxlanId)))
	}
	if req.Protocol != nil {
		predicates = append(predicates, ixprefix.ProtocolEQ(*req.Protocol))
	}
	if req.Status != nil {
		predicates = append(predicates, ixprefix.StatusEQ(*req.Status))
	}

	query := s.Client.IxPrefix.Query().
		Order(ent.Asc(ixprefix.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(ixprefix.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list ixprefixes: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.IxPrefix, len(results))
	for i, ixp := range results {
		items[i] = ixPrefixToProto(ixp)
	}

	return &pb.ListIxPrefixesResponse{
		IxPrefixes:    items,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamIxPrefixes streams all matching IX prefixes one message at a time using
// batched keyset pagination. Filters match the ListIxPrefixes behavior.
func (s *IxPrefixService) StreamIxPrefixes(ctx context.Context, req *pb.StreamIxPrefixesRequest, stream *connect.ServerStream[pb.IxPrefix]) error {
	// Apply stream timeout.
	if s.StreamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
		defer cancel()
	}

	// Build filter predicates (identical to ListIxPrefixes).
	var predicates []predicate.IxPrefix
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		predicates = append(predicates, ixprefix.IxlanIDEQ(int(*req.IxlanId)))
	}
	if req.Protocol != nil {
		predicates = append(predicates, ixprefix.ProtocolEQ(*req.Protocol))
	}
	if req.Status != nil {
		predicates = append(predicates, ixprefix.StatusEQ(*req.Status))
	}

	// Count total matching records for header metadata.
	countQuery := s.Client.IxPrefix.Query()
	if len(predicates) > 0 {
		countQuery = countQuery.Where(ixprefix.And(predicates...))
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("count ix prefixes: %w", err))
	}
	stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

	// Stream records in batches using keyset pagination.
	lastID := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := s.Client.IxPrefix.Query().
			Where(ixprefix.IDGT(lastID)).
			Order(ent.Asc(ixprefix.FieldID)).
			Limit(streamBatchSize)
		if len(predicates) > 0 {
			query = query.Where(ixprefix.And(predicates...))
		}

		batch, err := query.All(ctx)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream ix prefixes batch after id %d: %w", lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, ixp := range batch {
			if err := stream.Send(ixPrefixToProto(ixp)); err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < streamBatchSize {
			return nil
		}
	}
}

// ixPrefixToProto converts an ent IxPrefix entity to a protobuf IxPrefix
// message.
func ixPrefixToProto(ixp *ent.IxPrefix) *pb.IxPrefix {
	return &pb.IxPrefix{
		Id:       int64(ixp.ID),
		IxlanId:  int64PtrVal(ixp.IxlanID),
		InDfz:    ixp.InDfz,
		Notes:    stringVal(ixp.Notes),
		Prefix:   ixp.Prefix,
		Protocol: stringVal(ixp.Protocol),
		Created:  timestampVal(ixp.Created),
		Updated:  timestampVal(ixp.Updated),
		Status:   ixp.Status,
	}
}
