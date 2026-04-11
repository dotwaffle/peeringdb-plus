package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

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

// carrierListFilters is the generic filter table consumed by
// applyCarrierListFilters. Entries run in slice order. The OrgName entry
// represents the denormalized org_name field stored on the entity.
var carrierListFilters = []filterFn[pb.ListCarriersRequest]{
	validatingFilter("id",
		func(r *pb.ListCarriersRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(carrier.FieldID)),
	validatingFilter("org_id",
		func(r *pb.ListCarriersRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(carrier.FieldOrgID)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Name },
		fieldContainsFold(carrier.FieldName)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Aka },
		fieldContainsFold(carrier.FieldAka)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.NameLong },
		fieldContainsFold(carrier.FieldNameLong)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Status },
		fieldEQString(carrier.FieldStatus)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Website },
		fieldEQString(carrier.FieldWebsite)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Notes },
		fieldEQString(carrier.FieldNotes)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.OrgName },
		fieldEQString(carrier.FieldOrgName)),
	eqFilter(func(r *pb.ListCarriersRequest) *string { return r.Logo },
		fieldEQString(carrier.FieldLogo)),
}

// carrierStreamFilters mirrors carrierListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var carrierStreamFilters = []filterFn[pb.StreamCarriersRequest]{
	validatingFilter("org_id",
		func(r *pb.StreamCarriersRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(carrier.FieldOrgID)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Name },
		fieldContainsFold(carrier.FieldName)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Aka },
		fieldContainsFold(carrier.FieldAka)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.NameLong },
		fieldContainsFold(carrier.FieldNameLong)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Status },
		fieldEQString(carrier.FieldStatus)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Website },
		fieldEQString(carrier.FieldWebsite)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Notes },
		fieldEQString(carrier.FieldNotes)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.OrgName },
		fieldEQString(carrier.FieldOrgName)),
	eqFilter(func(r *pb.StreamCarriersRequest) *string { return r.Logo },
		fieldEQString(carrier.FieldLogo)),
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

// applyCarrierListFilters builds filter predicates from the generic filter table.
func applyCarrierListFilters(req *pb.ListCarriersRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, carrierListFilters)
}

// applyCarrierStreamFilters builds filter predicates from the generic filter table.
func applyCarrierStreamFilters(req *pb.StreamCarriersRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, carrierStreamFilters)
}

// ListCarriers returns a paginated list of carriers ordered by ID ascending.
func (s *CarrierService) ListCarriers(ctx context.Context, req *pb.ListCarriersRequest) (*pb.ListCarriersResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Carrier, pb.Carrier]{
		EntityName: "carriers",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Carrier, error) {
			q := s.Client.Carrier.Query().
				Order(ent.Asc(carrier.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListCarriersResponse{Carriers: items, NextPageToken: nextToken}, nil
}

// StreamCarriers streams all matching carriers one message at a time.
func (s *CarrierService) StreamCarriers(ctx context.Context, req *pb.StreamCarriersRequest, stream *connect.ServerStream[pb.Carrier]) error {
	return StreamEntities(ctx, StreamParams[ent.Carrier, pb.Carrier]{
		EntityName:   "carriers",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Carrier.Query()
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Carrier, error) {
			q := s.Client.Carrier.Query().
				Where(carrier.IDGT(afterID)).
				Order(ent.Asc(carrier.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierToProto,
		GetID:   func(c *ent.Carrier) int { return c.ID },
	}, stream)
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
