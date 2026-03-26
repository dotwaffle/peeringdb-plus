package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// FacilityService implements the peeringdb.v1.FacilityService ConnectRPC
// handler interface. It queries the ent database layer and converts results to
// protobuf messages.
type FacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetFacility returns a single facility by ID. Returns NOT_FOUND if the
// facility does not exist.
func (s *FacilityService) GetFacility(ctx context.Context, req *pb.GetFacilityRequest) (*pb.GetFacilityResponse, error) {
	f, err := s.Client.Facility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity facility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get facility %d: %w", req.GetId(), err))
	}
	return &pb.GetFacilityResponse{Facility: facilityToProto(f)}, nil
}

func applyFacilityListFilters(req *pb.ListFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(facility.FieldID, int(*req.Id)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(facility.FieldOrgID, int(*req.OrgId)))
	}
	if req.CampusId != nil {
		if *req.CampusId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: campus_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(facility.FieldCampusID, int(*req.CampusId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldWebsite, *req.Website))
	}
	if req.Clli != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldClli, *req.Clli))
	}
	if req.Rencode != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldRencode, *req.Rencode))
	}
	if req.Npanxx != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldNpanxx, *req.Npanxx))
	}
	if req.TechEmail != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldTechEmail, *req.TechEmail))
	}
	if req.TechPhone != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldTechPhone, *req.TechPhone))
	}
	if req.SalesEmail != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSalesEmail, *req.SalesEmail))
	}
	if req.SalesPhone != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSalesPhone, *req.SalesPhone))
	}
	if req.Property != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldProperty, *req.Property))
	}
	if req.DiverseServingSubstations != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldDiverseServingSubstations, *req.DiverseServingSubstations))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldNotes, *req.Notes))
	}
	if req.RegionContinent != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldRegionContinent, *req.RegionContinent))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldLogo, *req.Logo))
	}
	if req.Address1 != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldAddress1, *req.Address1))
	}
	if req.Address2 != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldAddress2, *req.Address2))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldZipcode, *req.Zipcode))
	}
	if req.Suite != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSuite, *req.Suite))
	}
	if req.Floor != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldFloor, *req.Floor))
	}
	// org_name filter -- stored as denormalized field on entity.
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldOrgName, *req.OrgName))
	}
	return preds, nil
}

func applyFacilityStreamFilters(req *pb.StreamFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(facility.FieldOrgID, int(*req.OrgId)))
	}
	if req.CampusId != nil {
		if *req.CampusId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: campus_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(facility.FieldCampusID, int(*req.CampusId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(facility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldWebsite, *req.Website))
	}
	if req.Clli != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldClli, *req.Clli))
	}
	if req.Rencode != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldRencode, *req.Rencode))
	}
	if req.Npanxx != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldNpanxx, *req.Npanxx))
	}
	if req.TechEmail != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldTechEmail, *req.TechEmail))
	}
	if req.TechPhone != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldTechPhone, *req.TechPhone))
	}
	if req.SalesEmail != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSalesEmail, *req.SalesEmail))
	}
	if req.SalesPhone != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSalesPhone, *req.SalesPhone))
	}
	if req.Property != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldProperty, *req.Property))
	}
	if req.DiverseServingSubstations != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldDiverseServingSubstations, *req.DiverseServingSubstations))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldNotes, *req.Notes))
	}
	if req.RegionContinent != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldRegionContinent, *req.RegionContinent))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldLogo, *req.Logo))
	}
	if req.Address1 != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldAddress1, *req.Address1))
	}
	if req.Address2 != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldAddress2, *req.Address2))
	}
	if req.State != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldState, *req.State))
	}
	if req.Zipcode != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldZipcode, *req.Zipcode))
	}
	if req.Suite != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldSuite, *req.Suite))
	}
	if req.Floor != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldFloor, *req.Floor))
	}
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(facility.FieldOrgName, *req.OrgName))
	}
	return preds, nil
}

// ListFacilities returns a paginated list of facilities ordered by ID ascending.
func (s *FacilityService) ListFacilities(ctx context.Context, req *pb.ListFacilitiesRequest) (*pb.ListFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Facility, pb.Facility]{
		EntityName: "facilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Facility, error) {
			q := s.Client.Facility.Query().
				Order(ent.Asc(facility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(facility.And(castPredicates[predicate.Facility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: facilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListFacilitiesResponse{Facilities: items, NextPageToken: nextToken}, nil
}

// StreamFacilities streams all matching facilities one message at a time.
func (s *FacilityService) StreamFacilities(ctx context.Context, req *pb.StreamFacilitiesRequest, stream *connect.ServerStream[pb.Facility]) error {
	return StreamEntities(ctx, StreamParams[ent.Facility, pb.Facility]{
		EntityName:   "facilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Facility.Query()
			if len(preds) > 0 {
				q = q.Where(facility.And(castPredicates[predicate.Facility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Facility, error) {
			q := s.Client.Facility.Query().
				Where(facility.IDGT(afterID)).
				Order(ent.Asc(facility.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(facility.And(castPredicates[predicate.Facility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: facilityToProto,
		GetID:   func(f *ent.Facility) int { return f.ID },
	}, stream)
}

// facilityToProto converts an ent Facility entity to a protobuf Facility
// message.
func facilityToProto(f *ent.Facility) *pb.Facility {
	return &pb.Facility{
		Id:                        int64(f.ID),
		CampusId:                  int64PtrVal(f.CampusID),
		OrgId:                     int64PtrVal(f.OrgID),
		Address1:                  stringVal(f.Address1),
		Address2:                  stringVal(f.Address2),
		Aka:                       stringVal(f.Aka),
		AvailableVoltageServices:  f.AvailableVoltageServices,
		City:                      stringVal(f.City),
		Clli:                      stringVal(f.Clli),
		Country:                   stringVal(f.Country),
		DiverseServingSubstations: boolPtrVal(f.DiverseServingSubstations),
		Floor:                     stringVal(f.Floor),
		Latitude:                  float64PtrVal(f.Latitude),
		Logo:                      stringPtrVal(f.Logo),
		Longitude:                 float64PtrVal(f.Longitude),
		Name:                      f.Name,
		NameLong:                  stringVal(f.NameLong),
		Notes:                     stringVal(f.Notes),
		Npanxx:                    stringVal(f.Npanxx),
		Property:                  stringPtrVal(f.Property),
		RegionContinent:           stringPtrVal(f.RegionContinent),
		Rencode:                   stringVal(f.Rencode),
		SalesEmail:                stringVal(f.SalesEmail),
		SalesPhone:                stringVal(f.SalesPhone),
		State:                     stringVal(f.State),
		StatusDashboard:           stringPtrVal(f.StatusDashboard),
		Suite:                     stringVal(f.Suite),
		TechEmail:                 stringVal(f.TechEmail),
		TechPhone:                 stringVal(f.TechPhone),
		Website:                   stringVal(f.Website),
		Zipcode:                   stringVal(f.Zipcode),
		OrgName:                   stringVal(f.OrgName),
		NetCount:                  int64Val(f.NetCount),
		IxCount:                   int64Val(f.IxCount),
		CarrierCount:              int64Val(f.CarrierCount),
		Created:                   timestampVal(f.Created),
		Updated:                   timestampVal(f.Updated),
		Status:                    f.Status,
	}
}
