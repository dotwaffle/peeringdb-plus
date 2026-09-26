package pdbcompat

import (
	"encoding/json"
	"math"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParsePyFloat checks the Python float() rules that the distance
// key uses (2.83.0 serializers.py:443-460 calls float()).
func TestParsePyFloat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{in: " 5 ", want: 5},
		{in: "\u00a05\u3000", want: 5},
		{in: "1e3", want: 1000},
		{in: "1_000", want: 1000},
		{in: "+5", want: 5},
		{in: ".5", want: 0.5},
		{in: "5.", want: 5},
		{in: "inf", want: math.Inf(1)},
		{in: "-Infinity", want: math.Inf(-1)},
		{in: "1e999", want: math.Inf(1)},
		{in: "-1e999", want: math.Inf(-1)},
		{in: "1e-400", want: 0},
		{in: "0x10", wantErr: true},
		{in: "0x1p3", wantErr: true},
		{in: "", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "1__000", wantErr: true},
		{in: "_1000", wantErr: true},
		{in: "1000_", wantErr: true},
		// Python strips only white space: U+001C..U+001F and U+200B
		// are errors there too.
		{in: "\x1c5\x1f", wantErr: true},
		{in: "5\u200b", wantErr: true},
		// Python float() reads non-ASCII digits; the mirror does not
		// (a registered divergence).
		{in: "٥", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parsePyFloat(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parsePyFloat(%q) = %v, want an error", tt.in, got)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parsePyFloat(%q) = %v, %v, want %v", tt.in, got, err, tt.want)
		}
	}
	// nan parses; parseDistanceSearch rejects it.
	if got, err := parsePyFloat("nan"); err != nil || !math.IsNaN(got) {
		t.Errorf("parsePyFloat(nan) = %v, %v, want NaN", got, err)
	}
}

// TestParseDistanceSearch checks when a request is a distance search
// and the errors of a bad one (2.83.0 serializers.py:443-460,
// :1837-1887, :2213-2280).
func TestParseDistanceSearch(t *testing.T) {
	t.Parallel()
	const ll = "&latitude=50.1109&longitude=8.6821"
	tests := []struct {
		name, typ, query string
		want             *distanceSearch
		wantErr          string
	}{
		{name: "other type", typ: "net", query: "distance=abc"},
		{name: "campus", typ: "campus", query: "distance=10"},
		{name: "no key", typ: "fac", query: "latitude=1&longitude=2"},
		{name: "zero", typ: "fac", query: "distance=0" + ll},
		{name: "negative", typ: "org", query: "distance=-3"},
		{name: "minus inf", typ: "fac", query: "distance=-inf"},
		{name: "minus overflow", typ: "fac", query: "distance=-1e999"},
		{name: "search", typ: "fac", query: "distance=10" + ll, want: &distanceSearch{lat: 50.1109, lng: 8.6821, km: 10}},
		{name: "first distance", typ: "org", query: "distance=1&distance=500" + ll, want: &distanceSearch{lat: 50.1109, lng: 8.6821, km: 1}},
		{name: "first coordinates", typ: "fac", query: "distance=1&latitude=1&latitude=2&longitude=3&longitude=4", want: &distanceSearch{lat: 1, lng: 3, km: 1}},
		{name: "inf", typ: "fac", query: "distance=inf" + ll, want: &distanceSearch{lat: 50.1109, lng: 8.6821, km: math.Inf(1)}},
		{name: "bad value", typ: "fac", query: "distance=abc", wantErr: "distance: Invalid value"},
		{name: "empty value", typ: "org", query: "distance=", wantErr: "distance: Invalid value"},
		{name: "nan", typ: "org", query: "distance=nan" + ll, wantErr: "distance: Invalid value"},
		{name: "bad latitude", typ: "fac", query: "distance=10&latitude=abc&longitude=8", wantErr: "latitude: Invalid value"},
		{name: "empty latitude", typ: "fac", query: "distance=10&latitude=&longitude=8", wantErr: "latitude: Invalid value"},
		{name: "inf longitude", typ: "fac", query: "distance=10&latitude=1&longitude=inf", wantErr: "longitude: Invalid value"},
		{name: "no location", typ: "fac", query: "distance=10", wantErr: "country: Required for distance filtering; city: Required for distance filtering"},
		{name: "latitude only", typ: "org", query: "distance=10&latitude=50", wantErr: "country: Required for distance filtering; city: Required for distance filtering"},
		{name: "city only", typ: "fac", query: "distance=10&city=X", wantErr: "country: Required for distance filtering"},
		{name: "empty city counts", typ: "fac", query: "distance=10&city=", wantErr: "country: Required for distance filtering"},
		{name: "country only", typ: "org", query: "distance=10&country=DE", wantErr: "city: Required for distance filtering"},
		{name: "city__in is not city", typ: "org", query: "distance=10&country=DE&city__in=X", wantErr: "city: Required for distance filtering"},
		{name: "city and country", typ: "fac", query: "distance=10&city=X&country=DE", wantErr: "distance: needs latitude and longitude"},
		{name: "country__in counts", typ: "org", query: "distance=10&city=X&country__in=DE", wantErr: "distance: needs latitude and longitude"},
		{name: "fac name_search", typ: "fac", query: "distance=10&name_search=X", wantErr: "distance: needs latitude and longitude"},
		{name: "org name_search", typ: "org", query: "distance=10&name_search=X", wantErr: "country: Required for distance filtering; city: Required for distance filtering"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseDistanceSearch(tt.typ, params)
			if tt.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want prefix %q", err, tt.wantErr)
				}
				// The missing-key message names only the missing keys
				// (serializers.py:1856-1865), so it must match in full.
				exact := tt.wantErr == "distance: Invalid value" ||
					strings.HasSuffix(tt.wantErr, "Required for distance filtering")
				if exact && err.Error() != tt.wantErr {
					t.Fatalf("err = %q, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseListFilters_DistanceKeysNotUnknown checks that the keys of a
// distance search are known keys, so they do not reach the unknown-field
// diagnostics, and that the other types record distance as unknown.
func TestParseListFilters_DistanceKeysNotUnknown(t *testing.T) {
	t.Parallel()
	ctx := WithUnknownFields(t.Context())
	params := url.Values{
		"distance": {"10"}, "latitude": {"50.1109"}, "longitude": {"8.6821"},
		"city": {"X"}, "city__in": {"X"}, "state": {"X"}, "zipcode": {"X"}, "address1": {"X"},
	}
	lf, err := parseListFilters(ctx, params, Registry["fac"])
	if err != nil {
		t.Fatalf("parseListFilters: %v", err)
	}
	if len(lf.preds) != 1 || lf.orderBy == nil || lf.emptyResult {
		t.Errorf("preds=%d orderBy=%v empty=%v, want 1 predicate and a sort key", len(lf.preds), lf.orderBy != nil, lf.emptyResult)
	}
	if got := UnknownFieldsFromCtx(ctx); len(got) != 0 {
		t.Errorf("unknown = %v, want none", got)
	}

	// A no-op distance is a known key too.
	ctx = WithUnknownFields(t.Context())
	lf, err = parseListFilters(ctx, url.Values{"distance": {"0"}}, Registry["org"])
	if err != nil || lf.orderBy != nil || len(lf.preds) != 0 {
		t.Fatalf("distance=0: preds=%d orderBy=%v err=%v, want no filter", len(lf.preds), lf.orderBy != nil, err)
	}
	if got := UnknownFieldsFromCtx(ctx); len(got) != 0 {
		t.Errorf("distance=0: unknown = %v, want none", got)
	}

	ctx = WithUnknownFields(t.Context())
	if _, err := parseListFilters(ctx, url.Values{"distance": {"10"}}, Registry["net"]); err != nil {
		t.Fatalf("net: %v", err)
	}
	if got := UnknownFieldsFromCtx(ctx); !slices.Equal(got, []string{"distance"}) {
		t.Errorf("net: unknown = %v, want [distance]", got)
	}
}

// Points for the distance tests: Frankfurt, Offenbach (6.92 km from
// Frankfurt) and Amsterdam (363.4 km from Frankfurt), computed with the
// upstream formula.
var (
	pointFrankfurt = [2]float64{50.1109, 8.6821}
	pointOffenbach = [2]float64{50.0956, 8.7761}
	pointAmsterdam = [2]float64{52.3676, 4.9041}
)

// TestDistanceSearch_ClampsAcosArgument checks that a row at the search
// point is kept when rounding puts the acos argument above 1. At
// latitude 0.015 the unclamped argument is 1.0000000000000002 in SQLite,
// and acos returns NULL for it, so without the clamp the row would drop
// out of every search around its own point.
func TestDistanceSearch_ClampsAcosArgument(t *testing.T) {
	t.Parallel()
	point := [2]float64{0.015, 8.6821}
	c := seedDistanceFacs(t, &point)

	// Precondition: the unclamped expression is NULL for this row.
	unclamped, err := c.Facility.Query().Where(func(s *entsql.Selector) {
		lat, lng := s.C("latitude"), s.C("longitude")
		s.Where(entsql.ExprP("acos(cos(radians(?)) * cos(radians("+lat+")) * cos(radians("+lng+
			") - radians(?)) + sin(radians(?)) * sin(radians("+lat+"))) IS NULL", point[0], point[1], point[0]))
	}).Count(t.Context())
	if err != nil {
		t.Fatalf("unclamped query: %v", err)
	}
	if unclamped != 1 {
		t.Fatalf("the SQLite math at latitude %v no longer rounds above 1 (unclamped NULL rows = %d); pick another point", point[0], unclamped)
	}

	tc := Registry["fac"]
	params, err := url.ParseQuery("distance=1&latitude=0.015&longitude=8.6821")
	if err != nil {
		t.Fatal(err)
	}
	lf, err := parseListFilters(t.Context(), params, tc)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tc.List(t.Context(), c, QueryOptions{Filters: lf.preds, OrderBy: lf.orderBy})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := rowIDs(t, rows); !slices.Equal(got, []int{1}) {
		t.Errorf("list = %v, want [1] (the row at the search point)", got)
	}
}

// seedDistanceFacs seeds org 1 and one ok fac per entry of points, with
// ids from 1. A nil entry has no coordinates.
func seedDistanceFacs(t *testing.T, points ...*[2]float64) *ent.Client {
	t.Helper()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	c.Organization.Create().SetID(1).SetName("DistOrg").SetNameFold("distorg").
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	for i, p := range points {
		b := c.Facility.Create().SetID(i + 1).
			SetName("DistFac").SetNameFold(unifold.Fold("DistFac")).SetOrgID(1).
			SetCity("Frankfurt am Main").SetCityFold(unifold.Fold("Frankfurt am Main")).
			SetState("Hessen").SetAddress1("Kleyerstrasse 90").SetCountry("DE").
			SetStatus("ok").SetCreated(now).SetUpdated(now)
		if p != nil {
			b.SetLatitude(p[0]).SetLongitude(p[1])
		}
		b.SaveX(ctx)
	}
	return c
}

// rowIDs returns the id of each serialized row.
func rowIDs(t *testing.T, rows []any) []int {
	t.Helper()
	ids := make([]int, len(rows))
	for i, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		ids[i] = v.ID
	}
	return ids
}

// TestDistanceSearch_ListAndCountAgree checks that the budget count and
// the served list apply the same distance predicate (the 413 guarantee),
// and that the list is in distance order, then id order.
func TestDistanceSearch_ListAndCountAgree(t *testing.T) {
	t.Parallel()
	// Ids: 1 Amsterdam, 2 Offenbach, 3 Frankfurt, 4 no coordinates, 5
	// Frankfurt.
	c := seedDistanceFacs(t, &pointAmsterdam, &pointOffenbach, &pointFrankfurt, nil, &pointFrankfurt)
	tc := Registry["fac"]
	for _, tt := range []struct {
		query string
		want  []int
	}{
		{"distance=10&latitude=50.1109&longitude=8.6821", []int{3, 5, 2}},
		{"distance=500&latitude=50.1109&longitude=8.6821", []int{3, 5, 2, 1}},
		{"distance=inf&latitude=50.1109&longitude=8.6821", []int{3, 5, 2, 1}},
		{"distance=1&latitude=52.3676&longitude=4.9041", []int{1}},
	} {
		params, err := url.ParseQuery(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		lf, err := parseListFilters(t.Context(), params, tc)
		if err != nil {
			t.Fatalf("%s: %v", tt.query, err)
		}
		opts := QueryOptions{Filters: lf.preds, OrderBy: lf.orderBy}
		rows, err := tc.List(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: list: %v", tt.query, err)
		}
		n, err := tc.Count(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: count: %v", tt.query, err)
		}
		if got := rowIDs(t, rows); !slices.Equal(got, tt.want) {
			t.Errorf("%s: list = %v, want %v", tt.query, got, tt.want)
		}
		if n != len(tt.want) {
			t.Errorf("%s: count = %d, want %d", tt.query, n, len(tt.want))
		}
		// A page of the distance order.
		opts.Skip, opts.Limit = 1, 2
		rows, err = tc.List(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: page: %v", tt.query, err)
		}
		want := tt.want[min(1, len(tt.want)):min(3, len(tt.want))]
		if got := rowIDs(t, rows); !slices.Equal(got, want) {
			t.Errorf("%s skip=1 limit=2: list = %v, want %v", tt.query, got, want)
		}
	}
}
