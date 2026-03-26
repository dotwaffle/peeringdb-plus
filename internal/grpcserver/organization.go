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

func applyOrganizationListFilters(req *pb.ListOrganizationsRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(organization.FieldID, int(*req.Id)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldNotes, *req.Notes))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldLogo, *req.Logo))
	}
	if req.Address1 != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldAddress1, *req.Address1))
	}
	if req.Address2 != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldAddress2, *req.Address2))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldZipcode, *req.Zipcode))
	}
	if req.Suite != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldSuite, *req.Suite))
	}
	if req.Floor != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldFloor, *req.Floor))
	}
	return preds, nil
}

func applyOrganizationStreamFilters(req *pb.StreamOrganizationsRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(organization.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldNotes, *req.Notes))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldLogo, *req.Logo))
	}
	if req.Address1 != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldAddress1, *req.Address1))
	}
	if req.Address2 != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldAddress2, *req.Address2))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldZipcode, *req.Zipcode))
	}
	if req.Suite != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldSuite, *req.Suite))
	}
	if req.Floor != nil {
		preds = append(preds, sql.FieldEQ(organization.FieldFloor, *req.Floor))
	}
	return preds, nil
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
		NetCount:  int64Val(o.NetCount),
		FacCount:  int64Val(o.FacCount),
		Created:   timestampVal(o.Created),
		Updated:   timestampVal(o.Updated),
		Status:    o.Status,
	}
}
