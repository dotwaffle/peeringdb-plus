package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CampusService implements the peeringdb.v1.CampusService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type CampusService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCampus returns a single campus by ID. Returns NOT_FOUND if the campus
// does not exist.
func (s *CampusService) GetCampus(ctx context.Context, req *pb.GetCampusRequest) (*pb.GetCampusResponse, error) {
	c, err := s.Client.Campus.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity campus %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get campus %d: %w", req.GetId(), err))
	}
	return &pb.GetCampusResponse{Campus: campusToProto(c)}, nil
}

func applyCampusListFilters(req *pb.ListCampusesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(campus.FieldID, int(*req.Id)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(campus.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldNotes, *req.Notes))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldZipcode, *req.Zipcode))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldLogo, *req.Logo))
	}
	// org_name filter -- stored as denormalized field on entity.
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldOrgName, *req.OrgName))
	}
	return preds, nil
}

func applyCampusStreamFilters(req *pb.StreamCampusesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(campus.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(campus.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldNotes, *req.Notes))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldZipcode, *req.Zipcode))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldLogo, *req.Logo))
	}
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(campus.FieldOrgName, *req.OrgName))
	}
	return preds, nil
}

// ListCampuses returns a paginated list of campuses ordered by ID ascending.
func (s *CampusService) ListCampuses(ctx context.Context, req *pb.ListCampusesRequest) (*pb.ListCampusesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Campus, pb.Campus]{
		EntityName: "campuses",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCampusListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Campus, error) {
			q := s.Client.Campus.Query().
				Order(ent.Asc(campus.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(campus.And(castPredicates[predicate.Campus](preds)...))
			}
			return q.All(ctx)
		},
		Convert: campusToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListCampusesResponse{Campuses: items, NextPageToken: nextToken}, nil
}

// StreamCampuses streams all matching campuses one message at a time.
func (s *CampusService) StreamCampuses(ctx context.Context, req *pb.StreamCampusesRequest, stream *connect.ServerStream[pb.Campus]) error {
	return StreamEntities(ctx, StreamParams[ent.Campus, pb.Campus]{
		EntityName:   "campuses",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCampusStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Campus.Query()
			if len(preds) > 0 {
				q = q.Where(campus.And(castPredicates[predicate.Campus](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Campus, error) {
			q := s.Client.Campus.Query().
				Where(campus.IDGT(afterID)).
				Order(ent.Asc(campus.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(campus.And(castPredicates[predicate.Campus](preds)...))
			}
			return q.All(ctx)
		},
		Convert: campusToProto,
		GetID:   func(c *ent.Campus) int { return c.ID },
	}, stream)
}

// campusToProto converts an ent Campus entity to a protobuf Campus message.
func campusToProto(c *ent.Campus) *pb.Campus {
	return &pb.Campus{
		Id:       int64(c.ID),
		OrgId:    int64PtrVal(c.OrgID),
		Aka:      stringPtrVal(c.Aka),
		City:     stringVal(c.City),
		Country:  stringVal(c.Country),
		Logo:     stringPtrVal(c.Logo),
		Name:     c.Name,
		NameLong: stringPtrVal(c.NameLong),
		Notes:    stringVal(c.Notes),
		State:    stringVal(c.State),
		Website:  stringVal(c.Website),
		Zipcode:  stringVal(c.Zipcode),
		OrgName:  stringVal(c.OrgName),
		Created:  timestampVal(c.Created),
		Updated:  timestampVal(c.Updated),
		Status:   c.Status,
	}
}
