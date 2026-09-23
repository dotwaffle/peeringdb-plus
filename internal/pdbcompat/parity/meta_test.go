package parity

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Meta locks the netixlan meta filters of PeeringDB 2.83.0
// against future regression. Upstream rewrites meta__<path>[__op] onto
// typed generated columns before its filter loop
// (serializers.py:3129-3149, rest.py:559-563). The columns come from the
// meta key registry (meta_registry.py:277-313): planned_status_change
// status (text) and date (date), and rfc8950 (boolean, NULL when the key
// is absent). The net keys have no columns, so upstream ignores net
// meta__ filters.
//
// The sub-tests port the meta filter tests of
// tests/test_meta_registry.py and seed the rows of its meta_filter_setup
// fixture (:502-563). Upstream dates are relative to today. These use a
// fixed day, because the filter does not depend on the current date.
//
// upstream: tests/test_meta_registry.py:502-648
func TestParity_Meta(t *testing.T) {
	t.Parallel()

	today := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	day := func(n int) string { return today.AddDate(0, 0, n).Format(time.DateOnly) }

	// Row IDs of the upstream fixture.
	const (
		leaving  = 1 // plan deleted in 30 days, rfc8950 true
		arriving = 2 // not-operational, plan ok in 10 days
		plain    = 3 // no keys
		declined = 4 // rfc8950 false
	)

	// seedMetaFixture seeds the upstream fixture: one ix, and one net
	// with one netixlan per row.
	seedMetaFixture := func(t *testing.T) *ent.Client {
		t.Helper()
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Meta F Org", today)
		c.InternetExchange.Create().
			SetID(1).SetName("Meta F IX").SetNameFold(unifold.Fold("Meta F IX")).SetOrgID(1).
			SetStatus("ok").SetCreated(today).SetUpdated(today).SaveX(ctx)
		c.IxLan.Create().
			SetID(1).SetIxID(1).
			SetStatus("ok").SetCreated(today).SetUpdated(today).SaveX(ctx)

		rows := []struct {
			id     int
			status string
			meta   map[string]any
		}{
			{leaving, "ok", map[string]any{
				"planned_status_change": map[string]any{"status": "deleted", "date": day(30)},
				"rfc8950":               true,
			}},
			{arriving, "not-operational", map[string]any{
				"planned_status_change": map[string]any{"status": "ok", "date": day(10)},
			}},
			{plain, "ok", map[string]any{}},
			{declined, "ok", map[string]any{"rfc8950": false}},
		}
		for _, r := range rows {
			asn := 63400 + r.id
			mustNet(ctx, t, c, r.id, fmt.Sprintf("Meta F Net %d", asn), asn, 1, today)
			c.NetworkIxLan.Create().
				SetID(r.id).SetNetID(r.id).SetIxlanID(1).SetIxID(1).
				SetAsn(asn).SetSpeed(1000).SetName("Meta F IX").
				SetOperational(r.status == "ok").SetStatus(r.status).SetMeta(r.meta).
				SetCreated(today).SetUpdated(today).SaveX(ctx)
		}
		return c
	}

	// ids GETs path and returns the row IDs, sorted.
	ids := func(t *testing.T, c *ent.Client, path string) []int {
		t.Helper()
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status = %d; body=%s", path, status, string(body))
		}
		got := extractIDs(t, body)
		slices.Sort(got)
		return got
	}

	t.Run("planned_status", func(t *testing.T) {
		t.Parallel()
		// upstream: tests/test_meta_registry.py:572-578
		c := seedMetaFixture(t)
		got := ids(t, c, "/api/netixlan?meta__planned_status_change__status=deleted")
		if want := []int{leaving}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("date_comparison_is_typed", func(t *testing.T) {
		t.Parallel()
		// upstream: tests/test_meta_registry.py:582-599 (a row without
		// a plan is NULL and never matches)
		c := seedMetaFixture(t)
		cutoff := day(20)
		got := ids(t, c, "/api/netixlan?meta__planned_status_change__date__lt="+cutoff)
		if want := []int{arriving}; !slices.Equal(got, want) {
			t.Errorf("date__lt=%s: got %v, want %v", cutoff, got, want)
		}
		got = ids(t, c, "/api/netixlan?meta__planned_status_change__date__gt="+cutoff)
		if want := []int{leaving}; !slices.Equal(got, want) {
			t.Errorf("date__gt=%s: got %v, want %v", cutoff, got, want)
		}
	})

	t.Run("rfc8950_true", func(t *testing.T) {
		t.Parallel()
		// upstream: tests/test_meta_registry.py:603-610
		c := seedMetaFixture(t)
		got := ids(t, c, "/api/netixlan?meta__rfc8950=true")
		if want := []int{leaving}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("rfc8950_false_excludes_never_declared", func(t *testing.T) {
		t.Parallel()
		// upstream: tests/test_meta_registry.py:614-632 (the row that
		// never declared rfc8950 is NULL, not false)
		c := seedMetaFixture(t)
		got := ids(t, c, "/api/netixlan?meta__rfc8950=false")
		if want := []int{declined}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("headline_query", func(t *testing.T) {
		t.Parallel()
		// upstream: tests/test_meta_registry.py:636-648 ("who is
		// leaving IX <id> in the next 90 days?")
		c := seedMetaFixture(t)
		got := ids(t, c, "/api/netixlan?ix_id=1"+
			"&meta__planned_status_change__status=deleted"+
			"&meta__planned_status_change__date__lt="+day(90))
		if want := []int{leaving}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("raw_column_names", func(t *testing.T) {
		t.Parallel()
		// upstream: the generated columns are model fields, so their
		// names are filter keys too (rest.py:525-528, :670-683,
		// meta_registry.py:284, :290, :306).
		c := seedMetaFixture(t)
		cases := []struct {
			query string
			want  []int
		}{
			{"meta_planned_status_change_status=ok", []int{arriving}},
			{"meta_planned_status_change_date__gt=" + day(20), []int{leaving}},
			{"meta_rfc8950=false", []int{declined}},
		}
		for _, tc := range cases {
			got := ids(t, c, "/api/netixlan?"+tc.query)
			if !slices.Equal(got, tc.want) {
				t.Errorf("%s: got %v, want %v", tc.query, got, tc.want)
			}
		}
	})

	t.Run("case_insensitive_operator_names_ignored", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:616 (the operator pattern knows
		// lt, lte, gt, gte, contains, startswith and in) and :670 (a
		// key that names no field is ignored). The rewritten key
		// meta_planned_status_change_status__iexact names no field, so
		// the list comes back unfiltered.
		c := seedMetaFixture(t)
		for _, query := range []string{
			"meta__planned_status_change__status__iexact=deleted",
			"meta__planned_status_change__status__icontains=dele",
			"meta__rfc8950__iexact=true",
		} {
			got := ids(t, c, "/api/netixlan?"+query)
			if want := []int{leaving, arriving, plain, declined}; !slices.Equal(got, want) {
				t.Errorf("%s: got %v, want %v (unfiltered)", query, got, want)
			}
		}
	})

	t.Run("DIVERGENCE_date_in_filters", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream fails on __in for the date key. It parses
		// the whole comma-separated value as one datetime (2.83.0
		// rest.py:649), which raises, and the error handler then fails
		// on inst[0] (:651), so upstream returns 400. For a single date,
		// the parse succeeds and v.split(",") on the datetime (:666)
		// raises an unhandled error (500). The mirror matches whole
		// dates. See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := seedMetaFixture(t)
		got := ids(t, c, "/api/netixlan?meta__planned_status_change__date__in="+day(30)+","+day(10))
		if want := []int{leaving, arriving}; !slices.Equal(got, want) {
			t.Errorf("date__in: got %v, want %v", got, want)
		}
	})

	t.Run("net_meta_key_ignored", func(t *testing.T) {
		t.Parallel()
		// upstream: docs/api/object_metadata.md:178-181 (a key that is
		// not filterable is ignored, and the list comes back
		// unfiltered)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "Meta Net Org", today)
		for id, community := range map[int]string{10: "65000:666", 11: "65001:666"} {
			c.Network.Create().
				SetID(id).SetName(fmt.Sprintf("Meta Net %d", id)).
				SetNameFold(unifold.Fold(fmt.Sprintf("Meta Net %d", id))).
				SetAsn(64500 + id).SetOrgID(1).
				SetMeta(map[string]any{"rtbh_community": community}).
				SetStatus("ok").SetCreated(today).SetUpdated(today).SaveX(ctx)
		}
		got := ids(t, c, "/api/net?meta__rtbh_community=65000:666")
		if want := []int{10, 11}; !slices.Equal(got, want) {
			t.Errorf("got %v, want %v (unfiltered)", got, want)
		}
	})
}
