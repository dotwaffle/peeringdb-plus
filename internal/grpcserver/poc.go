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

func applyPocListFilters(req *pb.ListPocsRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(poc.FieldID, int(*req.Id)))
	}
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(poc.FieldNetID, int(*req.NetId)))
	}
	if req.Role != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldRole, *req.Role))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(poc.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldStatus, *req.Status))
	}
	if req.Visible != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldVisible, *req.Visible))
	}
	if req.Phone != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldPhone, *req.Phone))
	}
	if req.Email != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldEmail, *req.Email))
	}
	if req.Url != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldURL, *req.Url))
	}
	return preds, nil
}

func applyPocStreamFilters(req *pb.StreamPocsRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(poc.FieldNetID, int(*req.NetId)))
	}
	if req.Role != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldRole, *req.Role))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(poc.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldStatus, *req.Status))
	}
	if req.Visible != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldVisible, *req.Visible))
	}
	if req.Phone != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldPhone, *req.Phone))
	}
	if req.Email != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldEmail, *req.Email))
	}
	if req.Url != nil {
		preds = append(preds, sql.FieldEQ(poc.FieldURL, *req.Url))
	}
	return preds, nil
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
