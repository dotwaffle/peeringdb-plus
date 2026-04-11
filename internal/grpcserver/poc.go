package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// PocService implements the peeringdb.v1.PocService ConnectRPC handler
// interface.
type PocService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// pocListFilters is the generic filter table consumed by applyPocListFilters.
// Entries run in slice order.
var pocListFilters = []filterFn[pb.ListPocsRequest]{
	validatingFilter("id",
		func(r *pb.ListPocsRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(poc.FieldID)),
	validatingFilter("net_id",
		func(r *pb.ListPocsRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(poc.FieldNetID)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Role },
		fieldEQString(poc.FieldRole)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Name },
		fieldContainsFold(poc.FieldName)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Status },
		fieldEQString(poc.FieldStatus)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Visible },
		fieldEQString(poc.FieldVisible)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Phone },
		fieldEQString(poc.FieldPhone)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Email },
		fieldEQString(poc.FieldEmail)),
	eqFilter(func(r *pb.ListPocsRequest) *string { return r.Url },
		fieldEQString(poc.FieldURL)),
}

// pocStreamFilters mirrors pocListFilters but omits the id entry — Stream
// uses SinceID handled by generic.StreamEntities.
var pocStreamFilters = []filterFn[pb.StreamPocsRequest]{
	validatingFilter("net_id",
		func(r *pb.StreamPocsRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(poc.FieldNetID)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Role },
		fieldEQString(poc.FieldRole)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Name },
		fieldContainsFold(poc.FieldName)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Status },
		fieldEQString(poc.FieldStatus)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Visible },
		fieldEQString(poc.FieldVisible)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Phone },
		fieldEQString(poc.FieldPhone)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Email },
		fieldEQString(poc.FieldEmail)),
	eqFilter(func(r *pb.StreamPocsRequest) *string { return r.Url },
		fieldEQString(poc.FieldURL)),
}

// GetPoc returns a single point of contact by ID.
func (s *PocService) GetPoc(ctx context.Context, req *pb.GetPocRequest) (*pb.GetPocResponse, error) {
	p, err := s.Client.Poc.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity poc %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get poc %d: %w", req.GetId(), err))
	}
	return &pb.GetPocResponse{Poc: pocToProto(p)}, nil
}

// applyPocListFilters builds filter predicates from the generic filter table.
func applyPocListFilters(req *pb.ListPocsRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, pocListFilters)
}

// applyPocStreamFilters builds filter predicates from the generic filter table.
func applyPocStreamFilters(req *pb.StreamPocsRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, pocStreamFilters)
}

// ListPocs returns a paginated list of points of contact.
func (s *PocService) ListPocs(ctx context.Context, req *pb.ListPocsRequest) (*pb.ListPocsResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Poc, pb.Poc]{
		EntityName: "pocs",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyPocListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Poc, error) {
			q := s.Client.Poc.Query().
				Order(ent.Asc(poc.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(poc.And(castPredicates[predicate.Poc](preds)...))
			}
			return q.All(ctx)
		},
		Convert: pocToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListPocsResponse{Pocs: items, NextPageToken: nextToken}, nil
}

// StreamPocs streams all matching points of contact.
func (s *PocService) StreamPocs(ctx context.Context, req *pb.StreamPocsRequest, stream *connect.ServerStream[pb.Poc]) error {
	return StreamEntities(ctx, StreamParams[ent.Poc, pb.Poc]{
		EntityName:   "pocs",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyPocStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Poc.Query()
			if len(preds) > 0 {
				q = q.Where(poc.And(castPredicates[predicate.Poc](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Poc, error) {
			q := s.Client.Poc.Query().
				Where(poc.IDGT(afterID)).
				Order(ent.Asc(poc.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(poc.And(castPredicates[predicate.Poc](preds)...))
			}
			return q.All(ctx)
		},
		Convert: pocToProto,
		GetID:   func(p *ent.Poc) int { return p.ID },
	}, stream)
}

// pocToProto converts an ent Poc entity to a protobuf Poc message.
func pocToProto(p *ent.Poc) *pb.Poc {
	return &pb.Poc{
		Id:      int64(p.ID),
		NetId:   int64PtrVal(p.NetID),
		Email:   stringVal(p.Email),
		Name:    stringVal(p.Name),
		Phone:   stringVal(p.Phone),
		Role:    p.Role,
		Url:     stringVal(p.URL),
		Visible: stringVal(p.Visible),
		Created: timestampVal(p.Created),
		Updated: timestampVal(p.Updated),
		Status:  p.Status,
	}
}
