package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkIxLanService implements the peeringdb.v1.NetworkIxLanService
// ConnectRPC handler interface.
type NetworkIxLanService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetNetworkIxLan returns a single network IX LAN by ID.
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

func applyNetworkIxLanListFilters(req *pb.ListNetworkIxLansRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldID, int(*req.Id)))
	}
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNetID, int(*req.NetId)))
	}
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxlanID, int(*req.IxlanId)))
	}
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldAsn, int(*req.Asn)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(networkixlan.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldStatus, *req.Status))
	}
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxID, int(*req.IxId)))
	}
	if req.Speed != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldSpeed, int(*req.Speed)))
	}
	if req.Ipaddr4 != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIpaddr4, *req.Ipaddr4))
	}
	if req.Ipaddr6 != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIpaddr6, *req.Ipaddr6))
	}
	if req.IsRsPeer != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIsRsPeer, *req.IsRsPeer))
	}
	if req.BfdSupport != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldBfdSupport, *req.BfdSupport))
	}
	if req.Operational != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldOperational, *req.Operational))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNotes, *req.Notes))
	}
	if req.NetSideId != nil {
		if *req.NetSideId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_side_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNetSideID, int(*req.NetSideId)))
	}
	if req.IxSideId != nil {
		if *req.IxSideId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_side_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxSideID, int(*req.IxSideId)))
	}
	return preds, nil
}

func applyNetworkIxLanStreamFilters(req *pb.StreamNetworkIxLansRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNetID, int(*req.NetId)))
	}
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxlanID, int(*req.IxlanId)))
	}
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldAsn, int(*req.Asn)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(networkixlan.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldStatus, *req.Status))
	}
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxID, int(*req.IxId)))
	}
	if req.Speed != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldSpeed, int(*req.Speed)))
	}
	if req.Ipaddr4 != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIpaddr4, *req.Ipaddr4))
	}
	if req.Ipaddr6 != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIpaddr6, *req.Ipaddr6))
	}
	if req.IsRsPeer != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIsRsPeer, *req.IsRsPeer))
	}
	if req.BfdSupport != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldBfdSupport, *req.BfdSupport))
	}
	if req.Operational != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldOperational, *req.Operational))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNotes, *req.Notes))
	}
	if req.NetSideId != nil {
		if *req.NetSideId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_side_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldNetSideID, int(*req.NetSideId)))
	}
	if req.IxSideId != nil {
		if *req.IxSideId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_side_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkixlan.FieldIxSideID, int(*req.IxSideId)))
	}
	return preds, nil
}

// ListNetworkIxLans returns a paginated list of network IX LANs.
func (s *NetworkIxLanService) ListNetworkIxLans(ctx context.Context, req *pb.ListNetworkIxLansRequest) (*pb.ListNetworkIxLansResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.NetworkIxLan, pb.NetworkIxLan]{
		EntityName: "networkixlans",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkIxLanListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.NetworkIxLan, error) {
			q := s.Client.NetworkIxLan.Query().
				Order(ent.Asc(networkixlan.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(networkixlan.And(castPredicates[predicate.NetworkIxLan](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkIxLanToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListNetworkIxLansResponse{NetworkIxLans: items, NextPageToken: nextToken}, nil
}

// StreamNetworkIxLans streams all matching network IX LANs.
func (s *NetworkIxLanService) StreamNetworkIxLans(ctx context.Context, req *pb.StreamNetworkIxLansRequest, stream *connect.ServerStream[pb.NetworkIxLan]) error {
	return StreamEntities(ctx, StreamParams[ent.NetworkIxLan, pb.NetworkIxLan]{
		EntityName:   "networkixlans",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkIxLanStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.NetworkIxLan.Query()
			if len(preds) > 0 {
				q = q.Where(networkixlan.And(castPredicates[predicate.NetworkIxLan](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.NetworkIxLan, error) {
			q := s.Client.NetworkIxLan.Query().
				Where(networkixlan.IDGT(afterID)).
				Order(ent.Asc(networkixlan.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(networkixlan.And(castPredicates[predicate.NetworkIxLan](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkIxLanToProto,
		GetID:   func(nixl *ent.NetworkIxLan) int { return nixl.ID },
	}, stream)
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
