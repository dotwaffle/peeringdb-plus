package pdbcompat

import (
	"math"
	"net/url"
	"slices"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
)

// TestLookupCapacityFilter checks the key forms of the ix capacity key.
// Upstream applies the bare key and the operators of
// get_relation_filters (2.83.0 serializers.py:617), on ix only, and
// ignores the other forms.
func TestLookupCapacityFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key string
		wantOp   string
		wantOK   bool
	}{
		{"ix", "capacity", "", true},
		{"ix", "capacity__lt", "lt", true},
		{"ix", "capacity__lte", "lte", true},
		{"ix", "capacity__gt", "gt", true},
		{"ix", "capacity__gte", "gte", true},
		{"ix", "capacity__in", "in", true},
		{"ix", "capacity__contains", "contains", true},
		{"ix", "capacity__startswith", "startswith", true},
		{"ix", "capacity__iexact", "", false},
		{"ix", "capacity__exact", "", false},
		{"ix", "capacity__icontains", "", false},
		{"ix", "capacity__istartswith", "", false},
		{"ix", "capacity__foo__gte", "", false},
		{"ix", "capacity_id", "", false},
		{"ix", "capacity_id__gte", "", false},
		{"ix", "capacityx", "", false},
		{"net", "capacity", "", false},
		{"ixlan", "capacity__gte", "", false},
	}
	for _, tt := range tests {
		op, ok := lookupCapacityFilter(tt.typ, tt.key)
		if op != tt.wantOp || ok != tt.wantOK {
			t.Errorf("lookupCapacityFilter(%q, %q) = (%q, %v), want (%q, %v)", tt.typ, tt.key, op, ok, tt.wantOp, tt.wantOK)
		}
	}
}

// TestBuildCapacityPredicate checks the SQL and the bound values. A
// value binds as an integer: SQLite compares an integer with a text
// argument as unequal.
func TestBuildCapacityPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		op, value string
		wantSQL   string
		wantArg   any
	}{
		{"", " 500 ", "SUM(`network_ix_lans`.`speed`) = ?", 500},
		{"", "٥٠٠", "SUM(`network_ix_lans`.`speed`) = ?", 500},
		{"gte", "1_000", "SUM(`network_ix_lans`.`speed`) >= ?", 1000},
		{"lt", "+7", "SUM(`network_ix_lans`.`speed`) < ?", 7},
		{"lt", "99999999999999999999999", "SUM(`network_ix_lans`.`speed`) < ?", math.MaxInt},
		{"gt", "-99999999999999999999999", "SUM(`network_ix_lans`.`speed`) > ?", math.MinInt},
		{"in", "500, 0,1_0", "SUM(`network_ix_lans`.`speed`) IN (SELECT value FROM json_each(?))", "[500,0,10]"},
		{"contains", "0%_", "CAST(SUM(`network_ix_lans`.`speed`) AS TEXT) LIKE ? ESCAPE '\\'", `%0\%\_%`},
		{"startswith", "", "CAST(SUM(`network_ix_lans`.`speed`) AS TEXT) LIKE ? ESCAPE '\\'", "%"},
	}
	for _, tt := range tests {
		p, err := buildCapacityPredicate(tt.op, tt.value)
		if err != nil {
			t.Fatalf("buildCapacityPredicate(%q, %q): %v", tt.op, tt.value, err)
		}
		s := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("internet_exchanges"))
		p(s)
		q, args := s.Query()
		wantSub := "`internet_exchanges`.`id` IN (SELECT `network_ix_lans`.`ixlan_id` FROM `network_ix_lans` " +
			"WHERE `network_ix_lans`.`status` <> ? GROUP BY `network_ix_lans`.`ixlan_id` HAVING " + tt.wantSQL + ")"
		if !strings.Contains(q, wantSub) {
			t.Errorf("%s=%q: SQL = %q, want it to hold %q", tt.op, tt.value, q, wantSub)
		}
		if want := []any{"deleted", tt.wantArg}; !slices.Equal(args, want) {
			t.Errorf("%s=%q: args = %#v, want %#v", tt.op, tt.value, args, want)
		}
	}
}

// TestBuildCapacityPredicate_Errors checks the values that upstream
// int() rejects: a 400 (rest.py:493-500). contains and startswith do
// not convert the value, so they never fail.
func TestBuildCapacityPredicate_Errors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ op, value string }{
		{"", ""},
		{"", "abc"},
		{"", "1.5"},
		{"lt", "1e3"},
		{"gte", "x"},
		{"gte", "1__0"},
		{"in", ""},
		{"in", "500,"},
		{"in", "500,x"},
	} {
		if _, err := buildCapacityPredicate(tt.op, tt.value); err == nil || !strings.Contains(err.Error(), "is not an integer") {
			t.Errorf("buildCapacityPredicate(%q, %q): err = %v, want an integer error", tt.op, tt.value, err)
		}
	}
	for _, op := range []string{"contains", "startswith"} {
		if _, err := buildCapacityPredicate(op, "abc"); err != nil {
			t.Errorf("buildCapacityPredicate(%q, %q): err = %v, want nil", op, "abc", err)
		}
	}
}

// TestCapacityPlan locks the plan that keeps the capacity filter cheap:
// the GROUP BY subquery is not correlated, so SQLite runs it once per
// statement, and it walks the netixlan ixlan_id index in group order.
func TestCapacityPlan(t *testing.T) {
	t.Parallel()
	for _, params := range []url.Values{
		{"capacity__gte": {"600"}},
		{"capacity__in": {"500,0"}},
		{"capacity__contains": {"00"}},
	} {
		preds, empty, err := ParseFiltersCtx(t.Context(), params, Registry["ix"])
		if err != nil || empty || len(preds) != 1 {
			t.Fatalf("%v: preds=%d empty=%v err=%v, want one predicate", params, len(preds), empty, err)
		}
		list, count := listPlans(t, "ix", QueryOptions{Filters: preds, Limit: 250})
		for name, plan := range map[string]string{"list": list, "count": count} {
			if !strings.Contains(plan, "LIST SUBQUERY") || strings.Contains(plan, "CORRELATED") {
				t.Errorf("%v: %s plan = %q, want a LIST SUBQUERY that is not CORRELATED", params, name, plan)
			}
			if !strings.Contains(plan, "networkixlan_ixlan_id") {
				t.Errorf("%v: %s plan = %q, want the networkixlan_ixlan_id index", params, name, plan)
			}
		}
	}
}
