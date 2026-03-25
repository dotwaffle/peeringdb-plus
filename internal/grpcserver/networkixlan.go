package grpcserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkIxLanService implements the peeringdb.v1.NetworkIxLanService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type NetworkIxLanService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetNetworkIxLan returns a single network IX LAN by ID. Returns NOT_FOUND if
// the network IX LAN does not exist.
func (s *NetworkIxLanService) GetNetworkIxLan(ctx context.Context, req *pb.GetNetworkIxLanRequest) (*pb.GetNetworkIxLanResponse, error) {
	nixl, err := s.Client.NetworkIxLan.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity networkixlan %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get networkixlan %d: %w", req.GetId(), err))
	}
	return &pb.GetNetworkIxLanResponse{NetworkIxLan: networkIxLanToProto(nixl)}, nil
}

// ListNetworkIxLans returns a paginated list of network IX LANs ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields
// (net_id, ixlan_id, asn, name, status). Multiple filters combine with AND
// logic.
func (s *NetworkIxLanService) ListNetworkIxLans(ctx context.Context, req *pb.ListNetworkIxLansRequest) (*pb.ListNetworkIxLansResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.NetworkIxLan
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: net_id must be positive"))
		}
		predicates = append(predicates, networkixlan.NetIDEQ(int(*req.NetId)))
	}
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		predicates = append(predicates, networkixlan.IxlanIDEQ(int(*req.IxlanId)))
	}
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: asn must be positive"))
		}
		predicates = append(predicates, networkixlan.AsnEQ(int(*req.Asn)))
	}
	if req.Name != nil {
		predicates = append(predicates, networkixlan.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, networkixlan.StatusEQ(*req.Status))
	}

	query := s.Client.NetworkIxLan.Query().
		Order(ent.Asc(networkixlan.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(networkixlan.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list networkixlans: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.NetworkIxLan, len(results))
	for i, nixl := range results {
		items[i] = networkIxLanToProto(nixl)
	}

	return &pb.ListNetworkIxLansResponse{
		NetworkIxLans: items,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamNetworkIxLans streams all matching network IX LANs one message at a
// time using batched keyset pagination. Filters match the ListNetworkIxLans
// behavior.
func (s *NetworkIxLanService) StreamNetworkIxLans(ctx context.Context, req *pb.StreamNetworkIxLansRequest, stream *connect.ServerStream[pb.NetworkIxLan]) error {
	// Apply stream timeout.
	if s.StreamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
		defer cancel()
	}

	// Build filter predicates (identical to ListNetworkIxLans).
	var predicates []predicate.NetworkIxLan
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: net_id must be positive"))
		}
		predicates = append(predicates, networkixlan.NetIDEQ(int(*req.NetId)))
	}
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		predicates = append(predicates, networkixlan.IxlanIDEQ(int(*req.IxlanId)))
	}
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: asn must be positive"))
		}
		predicates = append(predicates, networkixlan.AsnEQ(int(*req.Asn)))
	}
	if req.Name != nil {
		predicates = append(predicates, networkixlan.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, networkixlan.StatusEQ(*req.Status))
	}

	// Count total matching records for header metadata.
	countQuery := s.Client.NetworkIxLan.Query()
	if len(predicates) > 0 {
		countQuery = countQuery.Where(networkixlan.And(predicates...))
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("count networkixlans: %w", err))
	}
	stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

	// Stream records in batches using keyset pagination.
	lastID := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := s.Client.NetworkIxLan.Query().
			Where(networkixlan.IDGT(lastID)).
			Order(ent.Asc(networkixlan.FieldID)).
			Limit(streamBatchSize)
		if len(predicates) > 0 {
			query = query.Where(networkixlan.And(predicates...))
		}

		batch, err := query.All(ctx)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream networkixlans batch after id %d: %w", lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, nixl := range batch {
			if err := stream.Send(networkIxLanToProto(nixl)); err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < streamBatchSize {
			return nil
		}
	}
}

// networkIxLanToProto converts an ent NetworkIxLan entity to a protobuf
// NetworkIxLan message.
func networkIxLanToProto(nixl *ent.NetworkIxLan) *pb.NetworkIxLan {
	return &pb.NetworkIxLan{
		Id:          int64(nixl.ID),
		IxSideId:    int64PtrVal(nixl.IxSideID),
		IxlanId:     int64PtrVal(nixl.IxlanID),
		NetId:       int64PtrVal(nixl.NetID),
		NetSideId:   int64PtrVal(nixl.NetSideID),
		Asn:         int64(nixl.Asn),
		BfdSupport:  nixl.BfdSupport,
		Ipaddr4:     stringPtrVal(nixl.Ipaddr4),
		Ipaddr6:     stringPtrVal(nixl.Ipaddr6),
		IsRsPeer:    nixl.IsRsPeer,
		Notes:       stringVal(nixl.Notes),
		Operational: nixl.Operational,
		Speed:       int64(nixl.Speed),
		IxId:        int64Val(nixl.IxID),
		Name:        stringVal(nixl.Name),
		Created:     timestampVal(nixl.Created),
		Updated:     timestampVal(nixl.Updated),
		Status:      nixl.Status,
	}
}
