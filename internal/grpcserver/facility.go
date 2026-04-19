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

// facilityListFilters is the generic filter table consumed by
// applyFacilityListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var facilityListFilters = []filterFn[pb.ListFacilitiesRequest]{
	validatingFilter("id",
		func(r *pb.ListFacilitiesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(facility.FieldID)),
	validatingFilter("org_id",
		func(r *pb.ListFacilitiesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(facility.FieldOrgID)),
	validatingFilter("campus_id",
		func(r *pb.ListFacilitiesRequest) *int64 { return r.CampusId },
		positiveInt64(), fieldEQInt(facility.FieldCampusID)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(facility.FieldName)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Aka },
		fieldContainsFold(facility.FieldAka)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.NameLong },
		fieldContainsFold(facility.FieldNameLong)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Country },
		fieldEQString(facility.FieldCountry)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.City },
		fieldContainsFold(facility.FieldCity)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Status },
		fieldEQString(facility.FieldStatus)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Website },
		fieldEQString(facility.FieldWebsite)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Clli },
		fieldEQString(facility.FieldClli)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Rencode },
		fieldEQString(facility.FieldRencode)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Npanxx },
		fieldEQString(facility.FieldNpanxx)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.TechEmail },
		fieldEQString(facility.FieldTechEmail)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.TechPhone },
		fieldEQString(facility.FieldTechPhone)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.SalesEmail },
		fieldEQString(facility.FieldSalesEmail)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.SalesPhone },
		fieldEQString(facility.FieldSalesPhone)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Property },
		fieldEQString(facility.FieldProperty)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *bool { return r.DiverseServingSubstations },
		fieldEQBool(facility.FieldDiverseServingSubstations)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Notes },
		fieldEQString(facility.FieldNotes)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.RegionContinent },
		fieldEQString(facility.FieldRegionContinent)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.StatusDashboard },
		fieldEQString(facility.FieldStatusDashboard)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Logo },
		fieldEQString(facility.FieldLogo)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Address1 },
		fieldEQString(facility.FieldAddress1)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Address2 },
		fieldEQString(facility.FieldAddress2)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.State },
		fieldEQString(facility.FieldState)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Zipcode },
		fieldEQString(facility.FieldZipcode)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Suite },
		fieldEQString(facility.FieldSuite)),
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.Floor },
		fieldEQString(facility.FieldFloor)),
	// org_name filter -- stored as denormalized field on entity.
	eqFilter(func(r *pb.ListFacilitiesRequest) *string { return r.OrgName },
		fieldEQString(facility.FieldOrgName)),
}

// facilityStreamFilters mirrors facilityListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var facilityStreamFilters = []filterFn[pb.StreamFacilitiesRequest]{
	validatingFilter("org_id",
		func(r *pb.StreamFacilitiesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(facility.FieldOrgID)),
	validatingFilter("campus_id",
		func(r *pb.StreamFacilitiesRequest) *int64 { return r.CampusId },
		positiveInt64(), fieldEQInt(facility.FieldCampusID)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(facility.FieldName)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Aka },
		fieldContainsFold(facility.FieldAka)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.NameLong },
		fieldContainsFold(facility.FieldNameLong)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Country },
		fieldEQString(facility.FieldCountry)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.City },
		fieldContainsFold(facility.FieldCity)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Status },
		fieldEQString(facility.FieldStatus)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Website },
		fieldEQString(facility.FieldWebsite)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Clli },
		fieldEQString(facility.FieldClli)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Rencode },
		fieldEQString(facility.FieldRencode)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Npanxx },
		fieldEQString(facility.FieldNpanxx)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.TechEmail },
		fieldEQString(facility.FieldTechEmail)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.TechPhone },
		fieldEQString(facility.FieldTechPhone)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.SalesEmail },
		fieldEQString(facility.FieldSalesEmail)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.SalesPhone },
		fieldEQString(facility.FieldSalesPhone)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Property },
		fieldEQString(facility.FieldProperty)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *bool { return r.DiverseServingSubstations },
		fieldEQBool(facility.FieldDiverseServingSubstations)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Notes },
		fieldEQString(facility.FieldNotes)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.RegionContinent },
		fieldEQString(facility.FieldRegionContinent)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.StatusDashboard },
		fieldEQString(facility.FieldStatusDashboard)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Logo },
		fieldEQString(facility.FieldLogo)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Address1 },
		fieldEQString(facility.FieldAddress1)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Address2 },
		fieldEQString(facility.FieldAddress2)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.State },
		fieldEQString(facility.FieldState)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Zipcode },
		fieldEQString(facility.FieldZipcode)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Suite },
		fieldEQString(facility.FieldSuite)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.Floor },
		fieldEQString(facility.FieldFloor)),
	eqFilter(func(r *pb.StreamFacilitiesRequest) *string { return r.OrgName },
		fieldEQString(facility.FieldOrgName)),
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

// applyFacilityListFilters builds filter predicates from the generic filter
// table. See facilityListFilters and internal/grpcserver/filter.go.
func applyFacilityListFilters(req *pb.ListFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, facilityListFilters)
}

// applyFacilityStreamFilters builds filter predicates from the generic filter
// table. See facilityStreamFilters and internal/grpcserver/filter.go.
func applyFacilityStreamFilters(req *pb.StreamFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, facilityStreamFilters)
}

// ListFacilities returns a paginated list of facilities under the compound
// default order (-updated, -created, -id) per Phase 67 ORDER-02.
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
				Order(ent.Desc(facility.FieldUpdated), ent.Desc(facility.FieldCreated), ent.Desc(facility.FieldID)).
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

// StreamFacilities streams all matching facilities via compound (updated, id)
// keyset pagination under the (-updated, -created, -id) default order.
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
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.Facility, error) {
			q := s.Client.Facility.Query().
				Order(ent.Desc(facility.FieldUpdated), ent.Desc(facility.FieldCreated), ent.Desc(facility.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.Facility(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(facility.And(castPredicates[predicate.Facility](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    facilityToProto,
		GetID:      func(f *ent.Facility) int { return f.ID },
		GetUpdated: func(f *ent.Facility) time.Time { return f.Updated },
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
