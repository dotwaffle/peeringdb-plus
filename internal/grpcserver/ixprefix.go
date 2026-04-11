package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxPrefixService implements the peeringdb.v1.IxPrefixService ConnectRPC
// handler interface.
type IxPrefixService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// ixPrefixListFilters is the generic filter table consumed by
// applyIxPrefixListFilters. Entries run in slice order.
var ixPrefixListFilters = []filterFn[pb.ListIxPrefixesRequest]{
	validatingFilter("id",
		func(r *pb.ListIxPrefixesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(ixprefix.FieldID)),
	validatingFilter("ixlan_id",
		func(r *pb.ListIxPrefixesRequest) *int64 { return r.IxlanId },
		positiveInt64(), fieldEQInt(ixprefix.FieldIxlanID)),
	eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Protocol },
		fieldEQString(ixprefix.FieldProtocol)),
	eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Status },
		fieldEQString(ixprefix.FieldStatus)),
	eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Prefix },
		fieldEQString(ixprefix.FieldPrefix)),
	eqFilter(func(r *pb.ListIxPrefixesRequest) *bool { return r.InDfz },
		fieldEQBool(ixprefix.FieldInDfz)),
	eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Notes },
		fieldEQString(ixprefix.FieldNotes)),
}

// ixPrefixStreamFilters mirrors ixPrefixListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var ixPrefixStreamFilters = []filterFn[pb.StreamIxPrefixesRequest]{
	validatingFilter("ixlan_id",
		func(r *pb.StreamIxPrefixesRequest) *int64 { return r.IxlanId },
		positiveInt64(), fieldEQInt(ixprefix.FieldIxlanID)),
	eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Protocol },
		fieldEQString(ixprefix.FieldProtocol)),
	eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Status },
		fieldEQString(ixprefix.FieldStatus)),
	eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Prefix },
		fieldEQString(ixprefix.FieldPrefix)),
	eqFilter(func(r *pb.StreamIxPrefixesRequest) *bool { return r.InDfz },
		fieldEQBool(ixprefix.FieldInDfz)),
	eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Notes },
		fieldEQString(ixprefix.FieldNotes)),
}

// GetIxPrefix returns a single IX prefix by ID.
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

// applyIxPrefixListFilters builds filter predicates from the generic filter table.
func applyIxPrefixListFilters(req *pb.ListIxPrefixesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixPrefixListFilters)
}

// applyIxPrefixStreamFilters builds filter predicates from the generic filter table.
func applyIxPrefixStreamFilters(req *pb.StreamIxPrefixesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixPrefixStreamFilters)
}

// ListIxPrefixes returns a paginated list of IX prefixes.
func (s *IxPrefixService) ListIxPrefixes(ctx context.Context, req *pb.ListIxPrefixesRequest) (*pb.ListIxPrefixesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.IxPrefix, pb.IxPrefix]{
		EntityName: "ixprefixes",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxPrefixListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.IxPrefix, error) {
			q := s.Client.IxPrefix.Query().
				Order(ent.Asc(ixprefix.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(ixprefix.And(castPredicates[predicate.IxPrefix](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixPrefixToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListIxPrefixesResponse{IxPrefixes: items, NextPageToken: nextToken}, nil
}

// StreamIxPrefixes streams all matching IX prefixes.
func (s *IxPrefixService) StreamIxPrefixes(ctx context.Context, req *pb.StreamIxPrefixesRequest, stream *connect.ServerStream[pb.IxPrefix]) error {
	return StreamEntities(ctx, StreamParams[ent.IxPrefix, pb.IxPrefix]{
		EntityName:   "ix prefixes",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxPrefixStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.IxPrefix.Query()
			if len(preds) > 0 {
				q = q.Where(ixprefix.And(castPredicates[predicate.IxPrefix](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.IxPrefix, error) {
			q := s.Client.IxPrefix.Query().
				Where(ixprefix.IDGT(afterID)).
				Order(ent.Asc(ixprefix.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(ixprefix.And(castPredicates[predicate.IxPrefix](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixPrefixToProto,
		GetID:   func(ixp *ent.IxPrefix) int { return ixp.ID },
	}, stream)
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
