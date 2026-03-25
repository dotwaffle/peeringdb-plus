package grpcserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxLanService implements the peeringdb.v1.IxLanService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type IxLanService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetIxLan returns a single IX LAN by ID. Returns NOT_FOUND if the IX LAN
// does not exist.
func (s *IxLanService) GetIxLan(ctx context.Context, req *pb.GetIxLanRequest) (*pb.GetIxLanResponse, error) {
	il, err := s.Client.IxLan.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity ixlan %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get ixlan %d: %w", req.GetId(), err))
	}
	return &pb.GetIxLanResponse{IxLan: ixLanToProto(il)}, nil
}

// ListIxLans returns a paginated list of IX LANs ordered by ID ascending.
// Supports page_size, page_token, and optional filter fields (ix_id, name,
// status). Multiple filters combine with AND logic.
func (s *IxLanService) ListIxLans(ctx context.Context, req *pb.ListIxLansRequest) (*pb.ListIxLansResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.IxLan
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		predicates = append(predicates, ixlan.IxIDEQ(int(*req.IxId)))
	}
	if req.Name != nil {
		predicates = append(predicates, ixlan.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, ixlan.StatusEQ(*req.Status))
	}

	query := s.Client.IxLan.Query().
		Order(ent.Asc(ixlan.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(ixlan.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list ixlans: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.IxLan, len(results))
	for i, il := range results {
		items[i] = ixLanToProto(il)
	}

	return &pb.ListIxLansResponse{
		IxLans:        items,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamIxLans streams all matching IX LANs one message at a time using batched
// keyset pagination. Filters match the ListIxLans behavior.
func (s *IxLanService) StreamIxLans(ctx context.Context, req *pb.StreamIxLansRequest, stream *connect.ServerStream[pb.IxLan]) error {
	// Apply stream timeout.
	if s.StreamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
		defer cancel()
	}

	// Build filter predicates (identical to ListIxLans).
	var predicates []predicate.IxLan
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		predicates = append(predicates, ixlan.IxIDEQ(int(*req.IxId)))
	}
	if req.Name != nil {
		predicates = append(predicates, ixlan.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, ixlan.StatusEQ(*req.Status))
	}

	// Count total matching records for header metadata.
	countQuery := s.Client.IxLan.Query()
	if len(predicates) > 0 {
		countQuery = countQuery.Where(ixlan.And(predicates...))
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("count ixlans: %w", err))
	}
	stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

	// Stream records in batches using keyset pagination.
	lastID := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := s.Client.IxLan.Query().
			Where(ixlan.IDGT(lastID)).
			Order(ent.Asc(ixlan.FieldID)).
			Limit(streamBatchSize)
		if len(predicates) > 0 {
			query = query.Where(ixlan.And(predicates...))
		}

		batch, err := query.All(ctx)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream ixlans batch after id %d: %w", lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, il := range batch {
			if err := stream.Send(ixLanToProto(il)); err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < streamBatchSize {
			return nil
		}
	}
}

// ixLanToProto converts an ent IxLan entity to a protobuf IxLan message.
func ixLanToProto(il *ent.IxLan) *pb.IxLan {
	return &pb.IxLan{
		Id:                         int64(il.ID),
		IxId:                       int64PtrVal(il.IxID),
		ArpSponge:                  stringPtrVal(il.ArpSponge),
		Descr:                      stringVal(il.Descr),
		Dot1QSupport:               il.Dot1qSupport,
		IxfIxpImportEnabled:        il.IxfIxpImportEnabled,
		IxfIxpMemberListUrlVisible: stringVal(il.IxfIxpMemberListURLVisible),
		Mtu:                        int64Val(il.Mtu),
		Name:                       stringVal(il.Name),
		RsAsn:                      int64PtrVal(il.RsAsn),
		Created:                    timestampVal(il.Created),
		Updated:                    timestampVal(il.Updated),
		Status:                     il.Status,
	}
}
