package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxFacilityService implements the peeringdb.v1.IxFacilityService ConnectRPC
// handler interface.
type IxFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// ixFacilityListFilters is the generic filter table consumed by
// applyIxFacilityListFilters. Entries run in slice order.
var ixFacilityListFilters = []filterFn[pb.ListIxFacilitiesRequest]{
	validatingFilter("id",
		func(r *pb.ListIxFacilitiesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(ixfacility.FieldID)),
	validatingFilter("ix_id",
		func(r *pb.ListIxFacilitiesRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(ixfacility.FieldIxID)),
	validatingFilter("fac_id",
		func(r *pb.ListIxFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(ixfacility.FieldFacID)),
	eqFilter(func(r *pb.ListIxFacilitiesRequest) *string { return r.Country },
		fieldEQString(ixfacility.FieldCountry)),
	eqFilter(func(r *pb.ListIxFacilitiesRequest) *string { return r.City },
		fieldContainsFold(ixfacility.FieldCity)),
	eqFilter(func(r *pb.ListIxFacilitiesRequest) *string { return r.Status },
		fieldEQString(ixfacility.FieldStatus)),
	eqFilter(func(r *pb.ListIxFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(ixfacility.FieldName)),
}

// ixFacilityStreamFilters mirrors ixFacilityListFilters but omits the id
// entry — Stream uses SinceID handled by generic.StreamEntities.
var ixFacilityStreamFilters = []filterFn[pb.StreamIxFacilitiesRequest]{
	validatingFilter("ix_id",
		func(r *pb.StreamIxFacilitiesRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(ixfacility.FieldIxID)),
	validatingFilter("fac_id",
		func(r *pb.StreamIxFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(ixfacility.FieldFacID)),
	eqFilter(func(r *pb.StreamIxFacilitiesRequest) *string { return r.Country },
		fieldEQString(ixfacility.FieldCountry)),
	eqFilter(func(r *pb.StreamIxFacilitiesRequest) *string { return r.City },
		fieldContainsFold(ixfacility.FieldCity)),
	eqFilter(func(r *pb.StreamIxFacilitiesRequest) *string { return r.Status },
		fieldEQString(ixfacility.FieldStatus)),
	eqFilter(func(r *pb.StreamIxFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(ixfacility.FieldName)),
}

// GetIxFacility returns a single IX facility by ID.
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

// applyIxFacilityListFilters builds filter predicates from the generic filter table.
func applyIxFacilityListFilters(req *pb.ListIxFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixFacilityListFilters)
}

// applyIxFacilityStreamFilters builds filter predicates from the generic filter table.
func applyIxFacilityStreamFilters(req *pb.StreamIxFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixFacilityStreamFilters)
}

// ListIxFacilities returns a paginated list of IX facilities under the
// compound default order (-updated, -created, -id) per Phase 67 ORDER-02.
func (s *IxFacilityService) ListIxFacilities(ctx context.Context, req *pb.ListIxFacilitiesRequest) (*pb.ListIxFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.IxFacility, pb.IxFacility]{
		EntityName: "ixfacilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.IxFacility, error) {
			q := s.Client.IxFacility.Query().
				Order(ent.Desc(ixfacility.FieldUpdated), ent.Desc(ixfacility.FieldCreated), ent.Desc(ixfacility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixFacilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListIxFacilitiesResponse{IxFacilities: items, NextPageToken: nextToken}, nil
}

// StreamIxFacilities streams all matching IX facilities via compound
// (updated, id) keyset pagination under the (-updated, -created, -id) order.
func (s *IxFacilityService) StreamIxFacilities(ctx context.Context, req *pb.StreamIxFacilitiesRequest, stream *connect.ServerStream[pb.IxFacility]) error {
	return StreamEntities(ctx, StreamParams[ent.IxFacility, pb.IxFacility]{
		EntityName:   "ixfacilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.IxFacility.Query()
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.IxFacility, error) {
			q := s.Client.IxFacility.Query().
				Order(ent.Desc(ixfacility.FieldUpdated), ent.Desc(ixfacility.FieldCreated), ent.Desc(ixfacility.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.IxFacility(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    ixFacilityToProto,
		GetID:      func(ixf *ent.IxFacility) int { return ixf.ID },
		GetUpdated: func(ixf *ent.IxFacility) time.Time { return ixf.Updated },
	}, stream)
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
