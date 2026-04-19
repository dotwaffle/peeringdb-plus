package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CarrierFacilityService implements the peeringdb.v1.CarrierFacilityService
// ConnectRPC handler interface.
type CarrierFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// carrierFacilityListFilters is the generic filter table consumed by
// applyCarrierFacilityListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var carrierFacilityListFilters = []filterFn[pb.ListCarrierFacilitiesRequest]{
	validatingFilter("id",
		func(r *pb.ListCarrierFacilitiesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(carrierfacility.FieldID)),
	validatingFilter("carrier_id",
		func(r *pb.ListCarrierFacilitiesRequest) *int64 { return r.CarrierId },
		positiveInt64(), fieldEQInt(carrierfacility.FieldCarrierID)),
	validatingFilter("fac_id",
		func(r *pb.ListCarrierFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(carrierfacility.FieldFacID)),
	eqFilter(func(r *pb.ListCarrierFacilitiesRequest) *string { return r.Status },
		fieldEQString(carrierfacility.FieldStatus)),
	eqFilter(func(r *pb.ListCarrierFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(carrierfacility.FieldName)),
}

// carrierFacilityStreamFilters mirrors carrierFacilityListFilters but omits
// the id entry — Stream uses SinceID handled by generic.StreamEntities.
var carrierFacilityStreamFilters = []filterFn[pb.StreamCarrierFacilitiesRequest]{
	validatingFilter("carrier_id",
		func(r *pb.StreamCarrierFacilitiesRequest) *int64 { return r.CarrierId },
		positiveInt64(), fieldEQInt(carrierfacility.FieldCarrierID)),
	validatingFilter("fac_id",
		func(r *pb.StreamCarrierFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(carrierfacility.FieldFacID)),
	eqFilter(func(r *pb.StreamCarrierFacilitiesRequest) *string { return r.Status },
		fieldEQString(carrierfacility.FieldStatus)),
	eqFilter(func(r *pb.StreamCarrierFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(carrierfacility.FieldName)),
}

// GetCarrierFacility returns a single carrier facility by ID.
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

// applyCarrierFacilityListFilters builds filter predicates from the generic
// filter table. See carrierFacilityListFilters and internal/grpcserver/filter.go.
func applyCarrierFacilityListFilters(req *pb.ListCarrierFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, carrierFacilityListFilters)
}

// applyCarrierFacilityStreamFilters builds filter predicates from the generic
// filter table. See carrierFacilityStreamFilters and internal/grpcserver/filter.go.
func applyCarrierFacilityStreamFilters(req *pb.StreamCarrierFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, carrierFacilityStreamFilters)
}

// ListCarrierFacilities returns a paginated list of carrier facilities under
// the compound default order (-updated, -created, -id) per Phase 67 ORDER-02.
func (s *CarrierFacilityService) ListCarrierFacilities(ctx context.Context, req *pb.ListCarrierFacilitiesRequest) (*pb.ListCarrierFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.CarrierFacility, pb.CarrierFacility]{
		EntityName: "carrierfacilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.CarrierFacility, error) {
			q := s.Client.CarrierFacility.Query().
				Order(ent.Desc(carrierfacility.FieldUpdated), ent.Desc(carrierfacility.FieldCreated), ent.Desc(carrierfacility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierFacilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListCarrierFacilitiesResponse{CarrierFacilities: items, NextPageToken: nextToken}, nil
}

// StreamCarrierFacilities streams all matching carrier facilities via compound
// (updated, id) keyset pagination under the (-updated, -created, -id) order.
func (s *CarrierFacilityService) StreamCarrierFacilities(ctx context.Context, req *pb.StreamCarrierFacilitiesRequest, stream *connect.ServerStream[pb.CarrierFacility]) error {
	return StreamEntities(ctx, StreamParams[ent.CarrierFacility, pb.CarrierFacility]{
		EntityName:   "carrier facilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.CarrierFacility.Query()
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.CarrierFacility, error) {
			q := s.Client.CarrierFacility.Query().
				Order(ent.Desc(carrierfacility.FieldUpdated), ent.Desc(carrierfacility.FieldCreated), ent.Desc(carrierfacility.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.CarrierFacility(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    carrierFacilityToProto,
		GetID:      func(cf *ent.CarrierFacility) int { return cf.ID },
		GetUpdated: func(cf *ent.CarrierFacility) time.Time { return cf.Updated },
	}, stream)
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
