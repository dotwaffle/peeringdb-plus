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

func applyIxPrefixListFilters(req *pb.ListIxPrefixesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixprefix.FieldID, int(*req.Id)))
	}
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixprefix.FieldIxlanID, int(*req.IxlanId)))
	}
	if req.Protocol != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldProtocol, *req.Protocol))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldStatus, *req.Status))
	}
	if req.Prefix != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldPrefix, *req.Prefix))
	}
	if req.InDfz != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldInDfz, *req.InDfz))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldNotes, *req.Notes))
	}
	return preds, nil
}

func applyIxPrefixStreamFilters(req *pb.StreamIxPrefixesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.IxlanId != nil {
		if *req.IxlanId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ixlan_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixprefix.FieldIxlanID, int(*req.IxlanId)))
	}
	if req.Protocol != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldProtocol, *req.Protocol))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldStatus, *req.Status))
	}
	if req.Prefix != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldPrefix, *req.Prefix))
	}
	if req.InDfz != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldInDfz, *req.InDfz))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(ixprefix.FieldNotes, *req.Notes))
	}
	return preds, nil
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
