package pdbcompat

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestCanonicalChoices locks the conversion of a filter value to the
// stored form (django-peeringdb 969dd11 fields.py:61-71).
func TestCanonicalChoices(t *testing.T) {
	t.Parallel()
	types := multiChoiceLists["info_types"]
	tests := []struct{ value, want string }{
		{"NSP", "NSP"},
		{"Content,NSP", "NSP,Content"},
		// A choice matches as a substring, and case matters.
		{"Content and junk", "Content"},
		{"content", ""},
		{"", ""},
		{"Route Server Route Collector", "Route Server,Route Collector"},
	}
	for _, tt := range tests {
		if got := canonicalChoices(types, tt.value); got != tt.want {
			t.Errorf("canonicalChoices(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

// TestLegacyInfoTypePatterns locks the net keys that
// finalize_query_params rewrites (2.83.0 serializers.py:3768-3813).
func TestLegacyInfoTypePatterns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key, value string
		want            []string
		wantOK          bool
	}{
		{peeringdb.TypeNet, "info_type", "NSP", []string{"nsp%", "%,nsp,%", "%,nsp"}, true},
		{peeringdb.TypeNet, "info_type__contains", "Con", []string{"%con%"}, true},
		{peeringdb.TypeNet, "info_type__in", "NSP, Content", []string{"%nsp%", "%content%"}, true},
		// An empty item or value matches every network, so no pattern
		// is left, and duplicate items give one pattern.
		{peeringdb.TypeNet, "info_types__in", "", nil, true},
		{peeringdb.TypeNet, "info_type__in", "NSP,,Content", nil, true},
		{peeringdb.TypeNet, "info_type", "", nil, true},
		{peeringdb.TypeNet, "info_types__in", "NSP,nsp, NSP ,Content", []string{"%nsp%", "%content%"}, true},
		{peeringdb.TypeNet, "info_types__startswith", "Ro", []string{"ro%", "%,ro%"}, true},
		// The LIKE wildcards of the value are escaped.
		{peeringdb.TypeNet, "info_type__contains", `a%b_c\`, []string{`%a\%b\_c\\%`}, true},
		// Keys that the method does not handle.
		{peeringdb.TypeNet, "info_types", "NSP", nil, false},
		{peeringdb.TypeNet, "info_types__contains", "NSP", nil, false},
		{peeringdb.TypeNet, "info_type__lt", "NSP", nil, false},
		{peeringdb.TypeNet, "info_type__icontains", "NSP", nil, false},
		{peeringdb.TypeFac, "info_type", "NSP", nil, false},
	}
	for _, tt := range tests {
		got, ok := legacyInfoTypePatterns(tt.typ, tt.key, tt.value)
		if ok != tt.wantOK || !slices.Equal(got, tt.want) {
			t.Errorf("legacyInfoTypePatterns(%s, %q, %q) = %q, %v; want %q, %v",
				tt.typ, tt.key, tt.value, got, ok, tt.want, tt.wantOK)
		}
	}
}

// TestMultiChoiceFilter_StoredString runs the filters against SQLite. It
// checks how the stored string is built from rows that the parity tests
// do not seed: a NULL array, a value that is not in the choice list, and
// values that hold LIKE wildcards.
func TestMultiChoiceFilter_StoredString(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := testutil.SetupClient(t)
	t0 := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	c.Organization.Create().SetID(1).SetName("Org").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	rows := map[int][]string{
		1: nil,
		2: {"New Type", "Government", "NSP"},
		3: {"NSP"},
	}
	for id, types := range rows {
		b := c.Network.Create().SetID(id).SetName("Net").SetAsn(64500 + id).SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0)
		if types != nil {
			b.SetInfoTypes(types)
		}
		b.SaveX(ctx)
	}

	tests := []struct {
		query string
		want  []int
	}{
		{"info_types=", []int{1}},
		// Choices in list order, then the unknown value.
		{"info_types=NSP,Government,New+Type", []int{2}},
		{"info_types__contains=%25", nil},
		{"info_types__contains=_", nil},
		// finalize_query_params: an OR of substring matches.
		{"info_types__in=NSP,xyz", []int{2, 3}},
		{"info_types__in=xyz", nil},
		{"info_type=Government", []int{2}},
		{"info_type=New+Type", []int{2}},
	}
	for _, tt := range tests {
		params, err := url.ParseQuery(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		preds, empty, err := ParseFiltersCtx(WithUnknownFields(ctx), params, Registry[peeringdb.TypeNet])
		if err != nil || empty || len(preds) != 1 {
			t.Fatalf("%s: ParseFiltersCtx = %d preds, empty=%v, err=%v; want 1 pred", tt.query, len(preds), empty, err)
		}
		got, err := c.Network.Query().Where(func(s *sql.Selector) { preds[0](s) }).IDs(ctx)
		if err != nil {
			t.Fatalf("%s: query: %v", tt.query, err)
		}
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: IDs = %v, want %v", tt.query, got, tt.want)
		}
	}
}

// TestBuildMultiChoicePredicate_Errors checks the operators that do not
// apply to a multi-value field.
func TestBuildMultiChoicePredicate_Errors(t *testing.T) {
	t.Parallel()
	if _, err := buildMultiChoicePredicate("info_types", "in", ""); !errors.Is(err, errEmptyIn) {
		t.Errorf("empty __in: err = %v, want errEmptyIn", err)
	}
	if _, err := buildMultiChoicePredicate("info_types", "regex", "x"); err == nil {
		t.Error("unknown operator: err = nil, want an error")
	}
	if _, err := buildMultiChoicePredicate("name", "", "x"); err == nil {
		t.Error("field without a choice list: err = nil, want an error")
	}
}
