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

// networkIxLanListFilters is the generic filter table consumed by
// applyNetworkIxLanListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var networkIxLanListFilters = []filterFn[pb.ListNetworkIxLansRequest]{
	validatingFilter("id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(networkixlan.FieldID)),
	validatingFilter("net_id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(networkixlan.FieldNetID)),
	validatingFilter("ixlan_id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.IxlanId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxlanID)),
	validatingFilter("asn",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.Asn },
		positiveInt64(), fieldEQInt(networkixlan.FieldAsn)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *string { return r.Name },
		fieldContainsFold(networkixlan.FieldName)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *string { return r.Status },
		fieldEQString(networkixlan.FieldStatus)),
	validatingFilter("ix_id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxID)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *int64 { return r.Speed },
		fieldEQInt(networkixlan.FieldSpeed)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *string { return r.Ipaddr4 },
		fieldEQString(networkixlan.FieldIpaddr4)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *string { return r.Ipaddr6 },
		fieldEQString(networkixlan.FieldIpaddr6)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *bool { return r.IsRsPeer },
		fieldEQBool(networkixlan.FieldIsRsPeer)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *bool { return r.BfdSupport },
		fieldEQBool(networkixlan.FieldBfdSupport)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *bool { return r.Operational },
		fieldEQBool(networkixlan.FieldOperational)),
	eqFilter(func(r *pb.ListNetworkIxLansRequest) *string { return r.Notes },
		fieldEQString(networkixlan.FieldNotes)),
	validatingFilter("net_side_id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.NetSideId },
		positiveInt64(), fieldEQInt(networkixlan.FieldNetSideID)),
	validatingFilter("ix_side_id",
		func(r *pb.ListNetworkIxLansRequest) *int64 { return r.IxSideId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxSideID)),
}

// networkIxLanStreamFilters mirrors networkIxLanListFilters but omits the id
// entry — Stream uses SinceID handled by generic.StreamEntities.
var networkIxLanStreamFilters = []filterFn[pb.StreamNetworkIxLansRequest]{
	validatingFilter("net_id",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(networkixlan.FieldNetID)),
	validatingFilter("ixlan_id",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.IxlanId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxlanID)),
	validatingFilter("asn",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.Asn },
		positiveInt64(), fieldEQInt(networkixlan.FieldAsn)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *string { return r.Name },
		fieldContainsFold(networkixlan.FieldName)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *string { return r.Status },
		fieldEQString(networkixlan.FieldStatus)),
	validatingFilter("ix_id",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxID)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.Speed },
		fieldEQInt(networkixlan.FieldSpeed)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *string { return r.Ipaddr4 },
		fieldEQString(networkixlan.FieldIpaddr4)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *string { return r.Ipaddr6 },
		fieldEQString(networkixlan.FieldIpaddr6)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *bool { return r.IsRsPeer },
		fieldEQBool(networkixlan.FieldIsRsPeer)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *bool { return r.BfdSupport },
		fieldEQBool(networkixlan.FieldBfdSupport)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *bool { return r.Operational },
		fieldEQBool(networkixlan.FieldOperational)),
	eqFilter(func(r *pb.StreamNetworkIxLansRequest) *string { return r.Notes },
		fieldEQString(networkixlan.FieldNotes)),
	validatingFilter("net_side_id",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.NetSideId },
		positiveInt64(), fieldEQInt(networkixlan.FieldNetSideID)),
	validatingFilter("ix_side_id",
		func(r *pb.StreamNetworkIxLansRequest) *int64 { return r.IxSideId },
		positiveInt64(), fieldEQInt(networkixlan.FieldIxSideID)),
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

// applyNetworkIxLanListFilters builds filter predicates from the generic
// filter table. See networkIxLanListFilters and internal/grpcserver/filter.go.
func applyNetworkIxLanListFilters(req *pb.ListNetworkIxLansRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkIxLanListFilters)
}

// applyNetworkIxLanStreamFilters builds filter predicates from the generic
// filter table. See networkIxLanStreamFilters and internal/grpcserver/filter.go.
func applyNetworkIxLanStreamFilters(req *pb.StreamNetworkIxLansRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkIxLanStreamFilters)
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
