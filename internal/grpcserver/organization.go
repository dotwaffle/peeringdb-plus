package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// OrganizationService implements the peeringdb.v1.OrganizationService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type OrganizationService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// organizationListFilters is the generic filter table consumed by
// applyOrganizationListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var organizationListFilters = []filterFn[pb.ListOrganizationsRequest]{
	validatingFilter("id",
		func(r *pb.ListOrganizationsRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(organization.FieldID)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Name },
		fieldContainsFold(organization.FieldName)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Aka },
		fieldContainsFold(organization.FieldAka)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.NameLong },
		fieldContainsFold(organization.FieldNameLong)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Country },
		fieldEQString(organization.FieldCountry)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.City },
		fieldContainsFold(organization.FieldCity)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Status },
		fieldEQString(organization.FieldStatus)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Website },
		fieldEQString(organization.FieldWebsite)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Notes },
		fieldEQString(organization.FieldNotes)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Logo },
		fieldEQString(organization.FieldLogo)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Address1 },
		fieldEQString(organization.FieldAddress1)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Address2 },
		fieldEQString(organization.FieldAddress2)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.State },
		fieldEQString(organization.FieldState)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Zipcode },
		fieldEQString(organization.FieldZipcode)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Suite },
		fieldEQString(organization.FieldSuite)),
	eqFilter(func(r *pb.ListOrganizationsRequest) *string { return r.Floor },
		fieldEQString(organization.FieldFloor)),
}

// organizationStreamFilters mirrors organizationListFilters but omits the id
// entry — Stream uses SinceID handled by generic.StreamEntities.
var organizationStreamFilters = []filterFn[pb.StreamOrganizationsRequest]{
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Name },
		fieldContainsFold(organization.FieldName)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Aka },
		fieldContainsFold(organization.FieldAka)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.NameLong },
		fieldContainsFold(organization.FieldNameLong)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Country },
		fieldEQString(organization.FieldCountry)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.City },
		fieldContainsFold(organization.FieldCity)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Status },
		fieldEQString(organization.FieldStatus)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Website },
		fieldEQString(organization.FieldWebsite)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Notes },
		fieldEQString(organization.FieldNotes)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Logo },
		fieldEQString(organization.FieldLogo)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Address1 },
		fieldEQString(organization.FieldAddress1)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Address2 },
		fieldEQString(organization.FieldAddress2)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.State },
		fieldEQString(organization.FieldState)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Zipcode },
		fieldEQString(organization.FieldZipcode)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Suite },
		fieldEQString(organization.FieldSuite)),
	eqFilter(func(r *pb.StreamOrganizationsRequest) *string { return r.Floor },
		fieldEQString(organization.FieldFloor)),
}

// GetOrganization returns a single organization by ID. Returns NOT_FOUND if
// the organization does not exist.
func (s *OrganizationService) GetOrganization(ctx context.Context, req *pb.GetOrganizationRequest) (*pb.GetOrganizationResponse, error) {
	o, err := s.Client.Organization.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity organization %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get organization %d: %w", req.GetId(), err))
	}
	return &pb.GetOrganizationResponse{Organization: organizationToProto(o)}, nil
}

// applyOrganizationListFilters builds filter predicates from the generic
// filter table. See organizationListFilters and internal/grpcserver/filter.go.
func applyOrganizationListFilters(req *pb.ListOrganizationsRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, organizationListFilters)
}

// applyOrganizationStreamFilters builds filter predicates from the generic
// filter table. See organizationStreamFilters and internal/grpcserver/filter.go.
func applyOrganizationStreamFilters(req *pb.StreamOrganizationsRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, organizationStreamFilters)
}

// ListOrganizations returns a paginated list of organizations ordered by ID
// ascending. Supports all pdbcompat-parity filter fields with AND logic.
func (s *OrganizationService) ListOrganizations(ctx context.Context, req *pb.ListOrganizationsRequest) (*pb.ListOrganizationsResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Organization, pb.Organization]{
		EntityName: "organizations",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyOrganizationListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Organization, error) {
			q := s.Client.Organization.Query().
				Order(ent.Asc(organization.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(organization.And(castPredicates[predicate.Organization](preds)...))
			}
			return q.All(ctx)
		},
		Convert: organizationToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListOrganizationsResponse{Organizations: items, NextPageToken: nextToken}, nil
}

// StreamOrganizations streams all matching organizations one message at a time
// using batched keyset pagination.
func (s *OrganizationService) StreamOrganizations(ctx context.Context, req *pb.StreamOrganizationsRequest, stream *connect.ServerStream[pb.Organization]) error {
	return StreamEntities(ctx, StreamParams[ent.Organization, pb.Organization]{
		EntityName:   "organizations",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyOrganizationStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Organization.Query()
			if len(preds) > 0 {
				q = q.Where(organization.And(castPredicates[predicate.Organization](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Organization, error) {
			q := s.Client.Organization.Query().
				Where(organization.IDGT(afterID)).
				Order(ent.Asc(organization.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(organization.And(castPredicates[predicate.Organization](preds)...))
			}
			return q.All(ctx)
		},
		Convert: organizationToProto,
		GetID:   func(o *ent.Organization) int { return o.ID },
	}, stream)
}

// organizationToProto converts an ent Organization entity to a protobuf
// Organization message.
func organizationToProto(o *ent.Organization) *pb.Organization {
	return &pb.Organization{
		Id:        int64(o.ID),
		Address1:  stringVal(o.Address1),
		Address2:  stringVal(o.Address2),
		Aka:       stringVal(o.Aka),
		City:      stringVal(o.City),
		Country:   stringVal(o.Country),
		Floor:     stringVal(o.Floor),
		Latitude:  float64PtrVal(o.Latitude),
		Logo:      stringPtrVal(o.Logo),
		Longitude: float64PtrVal(o.Longitude),
		Name:      o.Name,
		NameLong:  stringVal(o.NameLong),
		Notes:     stringVal(o.Notes),
		State:     stringVal(o.State),
		Suite:     stringVal(o.Suite),
		Website:   stringVal(o.Website),
		Zipcode:   stringVal(o.Zipcode),
		// Phase 63 (D-02): NetCount / FacCount were dropped from the ent
		// Organization schema. The protobuf message still carries the
		// fields (proto is frozen since v1.6); they serialize as the
		// zero-value pointer (absent on the wire).
		Created:   timestampVal(o.Created),
		Updated:   timestampVal(o.Updated),
		Status:    o.Status,
	}
}
