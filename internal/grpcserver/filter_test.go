package grpcserver

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/reflect/protoreflect"

	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// testReq is a synthetic proto-shaped request used exclusively by the
// generic filter-layer tests. It deliberately avoids importing pb types
// so that filter.go remains verified in isolation from entity wiring.
type testReq struct {
	ID    *int64
	Name  *string
	Flag  *bool
	Ratio *float64
	Since *time.Time
}

// sqlitePredSQL applies a predicate closure to a fresh sqlite3-dialect
// selector and returns the rendered WHERE clause plus any positional args.
// Used by the predicate builder tests to assert the produced SQL fragment
// references the expected field name. sql.Selector parameterizes literal
// values, so assertions on concrete values should check the args slice,
// not the SQL string.
func sqlitePredSQL(t *testing.T, field string, pred func(*sql.Selector)) (string, []any) {
	t.Helper()
	if pred == nil {
		t.Fatal("predicate closure is nil")
	}
	sel := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("t"))
	pred(sel)
	q, args := sel.Query()
	if !strings.Contains(q, field) {
		t.Errorf("rendered query %q does not mention field %q", q, field)
	}
	return q, args
}

func TestEqFilter_NilPointerReturnsNilNilPredicate(t *testing.T) {
	t.Parallel()

	fn := eqFilter(func(r *testReq) *int64 { return r.ID }, fieldEQInt("id"))
	req := &testReq{} // ID is nil
	pred, err := fn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred != nil {
		t.Error("expected nil predicate when extractor returns nil pointer")
	}
}

func TestEqFilter_NonNilPointerReturnsPredicate(t *testing.T) {
	t.Parallel()

	fn := eqFilter(func(r *testReq) *int64 { return r.ID }, fieldEQInt("id"))
	id := int64(42)
	req := &testReq{ID: &id}
	pred, err := fn(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred == nil {
		t.Fatal("expected non-nil predicate when extractor returns non-nil pointer")
	}
	_, args := sqlitePredSQL(t, "id", pred)
	if len(args) != 1 {
		t.Fatalf("args = %v, want exactly one positional arg", args)
	}
	// fieldEQInt truncates int64 → int at the sql layer.
	if got, ok := args[0].(int); !ok || got != 42 {
		t.Errorf("args[0] = %v (%T), want int(42)", args[0], args[0])
	}
}

func TestValidatingFilter_NilPointerSkipsValidator(t *testing.T) {
	t.Parallel()

	calls := 0
	validate := func(int64) error {
		calls++
		return errors.New("should not be called")
	}
	fn := validatingFilter("id", func(r *testReq) *int64 { return r.ID }, validate, fieldEQInt("id"))
	pred, err := fn(&testReq{}) // ID nil
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pred != nil {
		t.Error("expected nil predicate")
	}
	if calls != 0 {
		t.Errorf("validator called %d times, want 0", calls)
	}
}

func TestValidatingFilter_ValidatorErrorWrappedAsInvalidArgument(t *testing.T) {
	t.Parallel()

	fn := validatingFilter("testfield",
		func(r *testReq) *int64 { return r.ID },
		func(int64) error { return errors.New("boom") },
		fieldEQInt("id"))

	id := int64(1)
	pred, err := fn(&testReq{ID: &id})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if pred != nil {
		t.Error("expected nil predicate on validation failure")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
	}
	msg := err.Error()
	// Format is "invalid filter: <name> <validator_err>" — matches the
	// pre-Phase-56 per-entity error strings so existing grpcserver_test.go
	// containsStr assertions stay green without test modification.
	if !strings.Contains(msg, "invalid filter: testfield ") {
		t.Errorf("error %q does not contain the filter name prefix", msg)
	}
	if !strings.Contains(msg, "boom") {
		t.Errorf("error %q does not contain the underlying validator message", msg)
	}
}

func TestValidatingFilter_ValidatorSuccessCallsPredicate(t *testing.T) {
	t.Parallel()

	predCalls := 0
	pred := func(v int64) func(*sql.Selector) {
		predCalls++
		return fieldEQInt("id")(v)
	}
	fn := validatingFilter("id",
		func(r *testReq) *int64 { return r.ID },
		func(int64) error { return nil },
		pred)

	id := int64(7)
	got, err := fn(&testReq{ID: &id})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil predicate")
	}
	if predCalls != 1 {
		t.Errorf("predicate constructor called %d times, want 1", predCalls)
	}
}

func TestApplyFilters_EmptySlice(t *testing.T) {
	t.Parallel()

	req := &testReq{}

	predsNil, err := applyFilters(req, nil)
	if err != nil {
		t.Fatalf("applyFilters(nil): %v", err)
	}
	if predsNil == nil {
		t.Error("applyFilters(nil) returned nil slice; want non-nil empty slice")
	}
	if len(predsNil) != 0 {
		t.Errorf("applyFilters(nil) len = %d, want 0", len(predsNil))
	}

	predsEmpty, err := applyFilters(req, []filterFn[testReq]{})
	if err != nil {
		t.Fatalf("applyFilters([]): %v", err)
	}
	if predsEmpty == nil {
		t.Error("applyFilters([]) returned nil slice; want non-nil empty slice")
	}
	if len(predsEmpty) != 0 {
		t.Errorf("applyFilters([]) len = %d, want 0", len(predsEmpty))
	}
}

func TestApplyFilters_AllNilPredicates(t *testing.T) {
	t.Parallel()

	fns := []filterFn[testReq]{
		func(*testReq) (func(*sql.Selector), error) { return nil, nil },
		func(*testReq) (func(*sql.Selector), error) { return nil, nil },
		func(*testReq) (func(*sql.Selector), error) { return nil, nil },
	}
	preds, err := applyFilters(&testReq{}, fns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 0 {
		t.Errorf("len = %d, want 0", len(preds))
	}
}

func TestApplyFilters_MixedNonNilAndNil(t *testing.T) {
	t.Parallel()

	p1 := sql.FieldEQ("a", 1)
	p3 := sql.FieldEQ("c", 3)
	fns := []filterFn[testReq]{
		func(*testReq) (func(*sql.Selector), error) { return p1, nil },
		func(*testReq) (func(*sql.Selector), error) { return nil, nil },
		func(*testReq) (func(*sql.Selector), error) { return p3, nil },
	}
	preds, err := applyFilters(&testReq{}, fns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(preds) != 2 {
		t.Fatalf("len = %d, want 2", len(preds))
	}

	// Verify the emitted predicates are the ones we expect, in order.
	sel1 := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("t"))
	preds[0](sel1)
	q1, _ := sel1.Query()
	if !strings.Contains(q1, "a") {
		t.Errorf("first surviving predicate = %q, want one referencing column a", q1)
	}

	sel2 := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("t"))
	preds[1](sel2)
	q2, _ := sel2.Query()
	if !strings.Contains(q2, "c") {
		t.Errorf("second surviving predicate = %q, want one referencing column c", q2)
	}
}

func TestApplyFilters_ShortCircuitsOnError(t *testing.T) {
	t.Parallel()

	thirdCalls := 0
	sentinel := errors.New("stop")
	fns := []filterFn[testReq]{
		func(*testReq) (func(*sql.Selector), error) { return sql.FieldEQ("a", 1), nil },
		func(*testReq) (func(*sql.Selector), error) { return nil, sentinel },
		func(*testReq) (func(*sql.Selector), error) {
			thirdCalls++
			return sql.FieldEQ("c", 3), nil
		},
	}
	preds, err := applyFilters(&testReq{}, fns)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wrapping sentinel", err)
	}
	if preds != nil {
		t.Errorf("preds = %v, want nil on error", preds)
	}
	if thirdCalls != 0 {
		t.Errorf("third filter invoked %d times, want 0 (short-circuit)", thirdCalls)
	}
}

func TestFieldEQInt_WrapsSQLFieldEQ(t *testing.T) {
	t.Parallel()

	builder := fieldEQInt("asn")
	pred := builder(int64(13335))
	_, args := sqlitePredSQL(t, "asn", pred)
	if len(args) != 1 {
		t.Fatalf("args = %v, want one positional arg", args)
	}
	// fieldEQInt truncates int64 → int to match pre-Phase-56 behavior.
	if got, ok := args[0].(int); !ok || got != 13335 {
		t.Errorf("args[0] = %v (%T), want int(13335)", args[0], args[0])
	}
}

func TestFieldEQString_WrapsSQLFieldEQ(t *testing.T) {
	t.Parallel()

	builder := fieldEQString("name")
	pred := builder("cloudflare")
	_, args := sqlitePredSQL(t, "name", pred)
	if len(args) != 1 {
		t.Fatalf("args = %v, want one positional arg", args)
	}
	if got, ok := args[0].(string); !ok || got != "cloudflare" {
		t.Errorf("args[0] = %v (%T), want string(\"cloudflare\")", args[0], args[0])
	}
}

func TestFieldContainsFold_WrapsSQLFieldContainsFold(t *testing.T) {
	t.Parallel()

	builder := fieldContainsFold("name")
	pred := builder("cloud")
	q, _ := sqlitePredSQL(t, "name", pred)
	// sql.FieldContainsFold lowercases both sides — the rendered SQL should
	// include a LOWER(...) or similar case-insensitive comparison.
	if !strings.Contains(strings.ToLower(q), "lower") {
		t.Errorf("rendered query %q does not look case-insensitive", q)
	}
}

func TestFieldEQBool_WrapsSQLFieldEQ(t *testing.T) {
	t.Parallel()

	builder := fieldEQBool("info_ipv6")

	// ent renders `sql.FieldEQ(col, true)` as `WHERE col` (no args) and
	// `sql.FieldEQ(col, false)` as `WHERE NOT col` — no positional arg is
	// emitted. Assert on the rendered SQL directly to exercise both
	// branches of the closure.
	predTrue := builder(true)
	qTrue, argsTrue := sqlitePredSQL(t, "info_ipv6", predTrue)
	if len(argsTrue) != 0 {
		t.Errorf("true-arg: got args %v, want none", argsTrue)
	}
	if strings.Contains(qTrue, "NOT") {
		t.Errorf("true-arg: query %q unexpectedly contains NOT", qTrue)
	}

	predFalse := builder(false)
	qFalse, argsFalse := sqlitePredSQL(t, "info_ipv6", predFalse)
	if len(argsFalse) != 0 {
		t.Errorf("false-arg: got args %v, want none", argsFalse)
	}
	if !strings.Contains(qFalse, "NOT") {
		t.Errorf("false-arg: query %q missing NOT clause", qFalse)
	}
}

func TestFieldEQFloat64_WrapsSQLFieldEQ(t *testing.T) {
	t.Parallel()

	builder := fieldEQFloat64("ratio")
	pred := builder(1.5)
	_, args := sqlitePredSQL(t, "ratio", pred)
	if len(args) != 1 {
		t.Fatalf("args = %v, want one positional arg", args)
	}
	if got, ok := args[0].(float64); !ok || got != 1.5 {
		t.Errorf("args[0] = %v (%T), want float64(1.5)", args[0], args[0])
	}
}

func TestFieldInTimeRange_WrapsSQLFieldGT(t *testing.T) {
	t.Parallel()

	builder := fieldInTimeRange("updated")
	ts := time.Unix(1_700_000_000, 0).UTC()
	pred := builder(ts)
	q, args := sqlitePredSQL(t, "updated", pred)
	// Lower-bound-exclusive semantic should render a `>` comparison.
	if !strings.Contains(q, ">") {
		t.Errorf("rendered query %q does not contain a > comparison", q)
	}
	if len(args) != 1 {
		t.Fatalf("args = %v, want one positional arg", args)
	}
	if got, ok := args[0].(time.Time); !ok || !got.Equal(ts) {
		t.Errorf("args[0] = %v (%T), want time.Time(%v)", args[0], args[0], ts)
	}
}

func TestPositiveInt64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       int64
		wantErr bool
	}{
		{name: "zero rejected", v: 0, wantErr: true},
		{name: "negative rejected", v: -1, wantErr: true},
		{name: "min int64 rejected", v: math.MinInt64, wantErr: true},
		{name: "one accepted", v: 1, wantErr: false},
		{name: "max int64 accepted", v: math.MaxInt64, wantErr: false},
	}

	v := positiveInt64()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v(tc.v)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("positiveInt64(%d) = nil, want error", tc.v)
				}
				if !strings.Contains(err.Error(), "must be positive") {
					t.Errorf("error %q does not contain 'must be positive'", err.Error())
				}
				return
			}
			if err != nil {
				t.Errorf("positiveInt64(%d) = %v, want nil", tc.v, err)
			}
		})
	}
}

func TestNonEmptyString(t *testing.T) {
	t.Parallel()

	v := nonEmptyString()

	if err := v(""); err == nil {
		t.Error("nonEmptyString(\"\") = nil, want error")
	} else if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("error %q does not contain 'must not be empty'", err.Error())
	}

	if err := v("x"); err != nil {
		t.Errorf("nonEmptyString(\"x\") = %v, want nil", err)
	}
}

// =======================================================================
// TestAllFilterFieldsExercised — reflection-based coverage gate
//
// Reflects over every ListXRequest / StreamXRequest proto message
// descriptor and asserts that the per-entity filter slice has an entry
// count equal to the number of optional filterable fields on the proto
// descriptor (after excluding the canonical pagination/metadata fields).
//
// This is a count-only check: closures cannot be introspected to prove
// they extract the CORRECT field. The per-entity behavior tests in
// grpcserver_test.go (TestListXFilters etc.) exercise every field
// individually — the combination catches both missing entries (count
// drift) and wrong extractors (behavior test failure).
//
// As of Plan 56-03, all 13 entities are migrated and the gate enforces
// full 13/13 coverage with zero skips.
// =======================================================================

// paginationFields are excluded from per-entity filter coverage. page_size
// and page_token are non-optional int32/string on ListX requests (they do
// not satisfy HasOptionalKeyword, but the explicit exclusion is defensive).
// since_id and updated_since are declared optional on StreamX requests but
// are handled by generic.StreamEntities at generic.go:99-104, not by the
// per-entity filter tables. request_id is reserved for future use.
var paginationFields = map[string]struct{}{
	"page_size":     {},
	"page_token":    {},
	"since_id":      {},
	"updated_since": {},
	"request_id":    {},
}

// entityFilterSpec couples a proto request type with the filter-slice
// length that should match its descriptor count.
type entityFilterSpec struct {
	name          string
	listReq       protoreflect.Message
	streamReq     protoreflect.Message
	listFilters   int // len of the xListFilters slice
	streamFilters int // len of the xStreamFilters slice
}

func allEntityFilterSpecs() []entityFilterSpec {
	return []entityFilterSpec{
		{
			name:          "network",
			listReq:       (&pb.ListNetworksRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamNetworksRequest{}).ProtoReflect(),
			listFilters:   len(networkListFilters),
			streamFilters: len(networkStreamFilters),
		},
		{
			name:          "facility",
			listReq:       (&pb.ListFacilitiesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamFacilitiesRequest{}).ProtoReflect(),
			listFilters:   len(facilityListFilters),
			streamFilters: len(facilityStreamFilters),
		},
		{
			name:          "internetexchange",
			listReq:       (&pb.ListInternetExchangesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamInternetExchangesRequest{}).ProtoReflect(),
			listFilters:   len(internetExchangeListFilters),
			streamFilters: len(internetExchangeStreamFilters),
		},
		{
			name:          "ixfacility",
			listReq:       (&pb.ListIxFacilitiesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamIxFacilitiesRequest{}).ProtoReflect(),
			listFilters:   len(ixFacilityListFilters),
			streamFilters: len(ixFacilityStreamFilters),
		},
		{
			name:          "ixlan",
			listReq:       (&pb.ListIxLansRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamIxLansRequest{}).ProtoReflect(),
			listFilters:   len(ixLanListFilters),
			streamFilters: len(ixLanStreamFilters),
		},
		{
			name:          "ixprefix",
			listReq:       (&pb.ListIxPrefixesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamIxPrefixesRequest{}).ProtoReflect(),
			listFilters:   len(ixPrefixListFilters),
			streamFilters: len(ixPrefixStreamFilters),
		},
		{
			name:          "networkfacility",
			listReq:       (&pb.ListNetworkFacilitiesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamNetworkFacilitiesRequest{}).ProtoReflect(),
			listFilters:   len(networkFacilityListFilters),
			streamFilters: len(networkFacilityStreamFilters),
		},
		{
			name:          "networkixlan",
			listReq:       (&pb.ListNetworkIxLansRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamNetworkIxLansRequest{}).ProtoReflect(),
			listFilters:   len(networkIxLanListFilters),
			streamFilters: len(networkIxLanStreamFilters),
		},
		{
			name:          "carrier",
			listReq:       (&pb.ListCarriersRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamCarriersRequest{}).ProtoReflect(),
			listFilters:   len(carrierListFilters),
			streamFilters: len(carrierStreamFilters),
		},
		{
			name:          "carrierfacility",
			listReq:       (&pb.ListCarrierFacilitiesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamCarrierFacilitiesRequest{}).ProtoReflect(),
			listFilters:   len(carrierFacilityListFilters),
			streamFilters: len(carrierFacilityStreamFilters),
		},
		{
			name:          "campus",
			listReq:       (&pb.ListCampusesRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamCampusesRequest{}).ProtoReflect(),
			listFilters:   len(campusListFilters),
			streamFilters: len(campusStreamFilters),
		},
		{
			name:          "organization",
			listReq:       (&pb.ListOrganizationsRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamOrganizationsRequest{}).ProtoReflect(),
			listFilters:   len(organizationListFilters),
			streamFilters: len(organizationStreamFilters),
		},
		{
			name:          "poc",
			listReq:       (&pb.ListPocsRequest{}).ProtoReflect(),
			streamReq:     (&pb.StreamPocsRequest{}).ProtoReflect(),
			listFilters:   len(pocListFilters),
			streamFilters: len(pocStreamFilters),
		},
	}
}

// countFilterableFields returns the number of proto fields on the given
// message descriptor that are filter candidates — optional scalar fields
// that are not pagination/metadata.
func countFilterableFields(msg protoreflect.Message) int {
	desc := msg.Descriptor()
	fields := desc.Fields()
	n := 0
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if _, skip := paginationFields[string(fd.Name())]; skip {
			continue
		}
		// HasOptionalKeyword catches proto3 `optional` fields. Non-optional
		// scalars on ListX/StreamX requests do not exist in the current
		// proto surface, but the check is defensive against future proto
		// additions that might use repeated fields or non-optional scalars.
		if !fd.HasOptionalKeyword() {
			continue
		}
		n++
	}
	return n
}

func TestAllFilterFieldsExercised(t *testing.T) {
	t.Parallel()

	for _, spec := range allEntityFilterSpecs() {
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			wantList := countFilterableFields(spec.listReq)
			if spec.listFilters != wantList {
				t.Errorf("%sListFilters: got %d entries, want %d (reflected from %s ListRequest proto descriptor); a new field was likely added without a filter table entry",
					spec.name, spec.listFilters, wantList, spec.name)
			}
			wantStream := countFilterableFields(spec.streamReq)
			if spec.streamFilters != wantStream {
				t.Errorf("%sStreamFilters: got %d entries, want %d (reflected from %s StreamRequest proto descriptor); a new field was likely added without a filter table entry",
					spec.name, spec.streamFilters, wantStream, spec.name)
			}
		})
	}
}
