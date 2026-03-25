package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// PocService implements the peeringdb.v1.PocService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type PocService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetPoc returns a single point of contact by ID. Returns NOT_FOUND if the POC
// does not exist.
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

// ListPocs returns a paginated list of points of contact ordered by ID
// ascending. Supports page_size, page_token, and optional filter fields
// (net_id, role, name, status). Multiple filters combine with AND logic.
func (s *PocService) ListPocs(ctx context.Context, req *pb.ListPocsRequest) (*pb.ListPocsResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Poc
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: net_id must be positive"))
		}
		predicates = append(predicates, poc.NetIDEQ(int(*req.NetId)))
	}
	if req.Role != nil {
		predicates = append(predicates, poc.RoleEQ(*req.Role))
	}
	if req.Name != nil {
		predicates = append(predicates, poc.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, poc.StatusEQ(*req.Status))
	}

	query := s.Client.Poc.Query().
		Order(ent.Asc(poc.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(poc.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list pocs: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	pocs := make([]*pb.Poc, len(results))
	for i, p := range results {
		pocs[i] = pocToProto(p)
	}

	return &pb.ListPocsResponse{
		Pocs:          pocs,
		NextPageToken: nextPageToken,
	}, nil
}

// StreamPocs streams all matching points of contact one message at a time using
// batched keyset pagination. Returns Unimplemented until handler logic is added.
func (s *PocService) StreamPocs(_ context.Context, _ *pb.StreamPocsRequest, _ *connect.ServerStream[pb.Poc]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamPocs not yet implemented"))
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
