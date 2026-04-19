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

// campusListFilters is the generic filter table consumed by
// applyCampusListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var campusListFilters = []filterFn[pb.ListCampusesRequest]{
	validatingFilter("id",
		func(r *pb.ListCampusesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(campus.FieldID)),
	validatingFilter("org_id",
		func(r *pb.ListCampusesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(campus.FieldOrgID)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Name },
		fieldContainsFold(campus.FieldName)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Aka },
		fieldContainsFold(campus.FieldAka)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.NameLong },
		fieldContainsFold(campus.FieldNameLong)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Country },
		fieldEQString(campus.FieldCountry)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.City },
		fieldContainsFold(campus.FieldCity)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Status },
		fieldEQString(campus.FieldStatus)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Website },
		fieldEQString(campus.FieldWebsite)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Notes },
		fieldEQString(campus.FieldNotes)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.State },
		fieldEQString(campus.FieldState)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Zipcode },
		fieldEQString(campus.FieldZipcode)),
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.Logo },
		fieldEQString(campus.FieldLogo)),
	// org_name filter -- stored as denormalized field on entity.
	eqFilter(func(r *pb.ListCampusesRequest) *string { return r.OrgName },
		fieldEQString(campus.FieldOrgName)),
}

// campusStreamFilters mirrors campusListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var campusStreamFilters = []filterFn[pb.StreamCampusesRequest]{
	validatingFilter("org_id",
		func(r *pb.StreamCampusesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(campus.FieldOrgID)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Name },
		fieldContainsFold(campus.FieldName)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Aka },
		fieldContainsFold(campus.FieldAka)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.NameLong },
		fieldContainsFold(campus.FieldNameLong)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Country },
		fieldEQString(campus.FieldCountry)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.City },
		fieldContainsFold(campus.FieldCity)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Status },
		fieldEQString(campus.FieldStatus)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Website },
		fieldEQString(campus.FieldWebsite)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Notes },
		fieldEQString(campus.FieldNotes)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.State },
		fieldEQString(campus.FieldState)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Zipcode },
		fieldEQString(campus.FieldZipcode)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.Logo },
		fieldEQString(campus.FieldLogo)),
	eqFilter(func(r *pb.StreamCampusesRequest) *string { return r.OrgName },
		fieldEQString(campus.FieldOrgName)),
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

// applyCampusListFilters builds filter predicates from the generic filter
// table. See campusListFilters and internal/grpcserver/filter.go.
func applyCampusListFilters(req *pb.ListCampusesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, campusListFilters)
}

// applyCampusStreamFilters builds filter predicates from the generic filter
// table. See campusStreamFilters and internal/grpcserver/filter.go.
func applyCampusStreamFilters(req *pb.StreamCampusesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, campusStreamFilters)
}

// ListCampuses returns a paginated list of campuses under the compound
// default order (-updated, -created, -id) per Phase 67 ORDER-02.
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
				Order(ent.Desc(campus.FieldUpdated), ent.Desc(campus.FieldCreated), ent.Desc(campus.FieldID)).
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

// StreamCampuses streams all matching campuses via compound (updated, id)
// keyset pagination under the (-updated, -created, -id) default order.
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
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.Campus, error) {
			q := s.Client.Campus.Query().
				Order(ent.Desc(campus.FieldUpdated), ent.Desc(campus.FieldCreated), ent.Desc(campus.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.Campus(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(campus.And(castPredicates[predicate.Campus](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    campusToProto,
		GetID:      func(c *ent.Campus) int { return c.ID },
		GetUpdated: func(c *ent.Campus) time.Time { return c.Updated },
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
