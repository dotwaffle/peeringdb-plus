package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedMetaFilterRows seeds six live netixlans with different meta
// documents under one ix and returns the client. IDs:
//
//	1 plan deleted 2026-10-23, rfc8950 true (status ok)
//	2 plan ok 2026-10-03 (status not-operational)
//	3 {} (no keys)
//	4 rfc8950 false
//	5 no stored document
//	6 rfc8950 as the number 1 (upstream never sends it, but it tells a
//	  JSON true from 1)
func seedMetaFilterRows(t *testing.T) *ent.Client {
	t.Helper()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	org := c.Organization.Create().SetName("Meta Filter Org").SetStatus("ok").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	ix := c.InternetExchange.Create().SetID(1).SetName("Meta Filter IX").SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	lan := c.IxLan.Create().SetInternetExchange(ix).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	net := c.Network.Create().SetName("Meta Filter Net").SetAsn(65001).SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)

	docs := []struct {
		status string
		meta   map[string]any
	}{
		{"ok", map[string]any{
			"planned_status_change": map[string]any{"status": "deleted", "date": "2026-10-23"},
			"rfc8950":               true,
		}},
		{"not-operational", map[string]any{
			"planned_status_change": map[string]any{"status": "ok", "date": "2026-10-03"},
		}},
		{"ok", map[string]any{}},
		{"ok", map[string]any{"rfc8950": false}},
		{"ok", nil},
		{"ok", map[string]any{"rfc8950": 1}},
	}
	for i, d := range docs {
		b := c.NetworkIxLan.Create().SetID(i + 1).SetNetwork(net).SetIxLan(lan).SetIxID(ix.ID).
			SetAsn(65001).SetSpeed(1000).SetOperational(d.status == "ok").
			SetStatus(d.status).SetCreated(now).SetUpdated(now.Add(time.Duration(i) * time.Minute))
		if d.meta != nil {
			b.SetMeta(d.meta)
		}
		b.SaveX(ctx)
	}
	return c
}

// getMetaFilterIDs GETs /api/netixlan?query and returns the HTTP status
// and the sorted row IDs.
func getMetaFilterIDs(t *testing.T, mux http.Handler, query string) (int, []int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/netixlan?"+query, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	var env struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", query, err)
	}
	ids := make([]int, 0, len(env.Data))
	for _, row := range env.Data {
		ids = append(ids, row.ID)
	}
	slices.Sort(ids)
	return rec.Code, ids
}

// TestMetaFilter_Netixlan locks the netixlan meta filters: key matching
// (meta__<path>, the upstream column names, operator suffixes), the
// typed operators, the absent-key rule and the 400 cases.
//
// upstream: 2.83.0 serializers.py:3129-3149 (key rewrite),
// meta_registry.py:277-313 (column types), rest.py:616-683 (operators).
func TestMetaFilter_Netixlan(t *testing.T) {
	t.Parallel()
	mux := newMuxForOrdering(seedMetaFilterRows(t))
	all := []int{1, 2, 3, 4, 5, 6}

	cases := []struct {
		query string
		want  []int
	}{
		// Text key: case-insensitive, like upstream iexact/icontains.
		{"meta__planned_status_change__status=deleted", []int{1}},
		{"meta__planned_status_change__status=DELETED", []int{1}},
		{"meta__planned_status_change__status__in=ok,deleted", []int{1, 2}},
		{"meta__planned_status_change__status__in=OK", []int{2}},
		{"meta__planned_status_change__status__contains=ELE", []int{1}},
		{"meta__planned_status_change__status__startswith=o", []int{2}},
		{"meta__planned_status_change__status__contains=%25", nil}, // % is literal
		{"meta__planned_status_change__status__startswith=_", nil}, // _ is literal
		{"meta__planned_status_change__status__lt=l", []int{1}},
		{"meta_planned_status_change_status=ok", []int{2}},

		// Date key: exact is a prefix match. Comparisons use the date.
		{"meta__planned_status_change__date=2026-10", []int{1, 2}},
		{"meta__planned_status_change__date=2026-10-23", []int{1}},
		{"meta__planned_status_change__date__lt=2026-10-13", []int{2}},
		{"meta__planned_status_change__date__gt=2026-10-13", []int{1}},
		{"meta__planned_status_change__date__lte=2026-10-23", []int{1, 2}},
		{"meta__planned_status_change__date__gte=2026-10-23", []int{1}},
		{"meta__planned_status_change__date__gt=2026-10-23", nil},
		{"meta__planned_status_change__date__lt=2026-10-23T12:00:00Z", []int{2}},
		{"meta__planned_status_change__date__gte=2026-10-22T23:00:00-05:00", []int{1}},
		{"meta__planned_status_change__date__in=2026-10-03,2026-10-23", []int{1, 2}},
		{"meta_planned_status_change_date__lt=2026-10-13", []int{2}},

		// Boolean key: true and 1 select true, any other value selects
		// false. A row without the key, or with a non-boolean value,
		// never matches.
		{"meta__rfc8950=true", []int{1}},
		{"meta__rfc8950=TRUE", []int{1}},
		{"meta__rfc8950=1", []int{1}},
		{"meta__rfc8950=false", []int{4}},
		{"meta__rfc8950=0", []int{4}},
		{"meta__rfc8950=yes", []int{4}},
		{"meta__rfc8950__in=true,false", []int{1, 4}},
		{"meta_rfc8950=true", []int{1}},

		// Combined with ordinary filters.
		{"ix_id=1&meta__rfc8950=true&meta__planned_status_change__status=deleted", []int{1}},
		{"status=ok&meta__planned_status_change__date=2026-10", []int{1}},

		// Unknown suffixes and partial paths are ignored. The
		// case-insensitive operator names are unknown too: upstream's
		// operator pattern does not match them (rest.py:616).
		{"meta__planned_status_change__status__iexact=Ok", all},
		{"meta__planned_status_change__status__icontains=ELE", all},
		{"meta__planned_status_change__date__istartswith=2026", all},
		{"meta__rfc8950__iexact=true", all},
		{"meta__rfc8950__foo=true", all},
		{"meta__rfc8950__=true", all},
		{"meta__rfc8950x=true", all},
		{"meta__planned_status_change=deleted", all},
		{"meta__preferred_ip_mtu=9000", all},

		// Empty __in returns no rows.
		{"meta__rfc8950__in=", nil},
		{"meta__planned_status_change__status__in=", nil},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			status, ids := getMetaFilterIDs(t, mux, tc.query)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200", status)
			}
			if !slices.Equal(ids, tc.want) {
				t.Errorf("ids = %v, want %v", ids, tc.want)
			}
		})
	}

	for _, query := range []string{
		"meta__planned_status_change__date__contains=2026",
		"meta__planned_status_change__date__lt=soon",
		"meta__planned_status_change__date__in=2026-10-03,never",
		"meta__rfc8950__lt=true",
		"meta__rfc8950__in=maybe",
	} {
		t.Run(query, func(t *testing.T) {
			t.Parallel()
			if status, _ := getMetaFilterIDs(t, mux, query); status != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", status)
			}
		})
	}
}

// TestMetaFilter_UnknownKeys locks which meta keys count as unknown: an
// unknown operator on a netixlan key, and every meta key on net, which
// has no filterable keys upstream (docs/api/object_metadata.md:178-181).
func TestMetaFilter_UnknownKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typeName string
		params   url.Values
		preds    int
		unknown  []string
	}{
		{peeringdb.TypeNetIXLan, url.Values{"meta__rfc8950": {"true"}, "meta__rfc8950__foo": {"true"}}, 1, []string{"meta__rfc8950__foo"}},
		{peeringdb.TypeNetIXLan, url.Values{"meta_planned_status_change_date__gte": {"2026-10-01"}}, 1, nil},
		{peeringdb.TypeNetIXLan, url.Values{"meta__planned_status_change__status__iexact": {"ok"}}, 0, []string{"meta__planned_status_change__status__iexact"}},
		{peeringdb.TypeNet, url.Values{"meta__rtbh_community": {"65000:666"}}, 0, []string{"meta__rtbh_community"}},
		{peeringdb.TypeNet, url.Values{"meta__rfc8950": {"true"}}, 0, []string{"meta__rfc8950"}},
	}
	for _, tc := range cases {
		ctx := WithUnknownFields(t.Context())
		preds, empty, err := ParseFiltersCtx(ctx, tc.params, Registry[tc.typeName])
		if err != nil || empty {
			t.Fatalf("%s %v: err=%v empty=%v", tc.typeName, tc.params, err, empty)
		}
		if len(preds) != tc.preds {
			t.Errorf("%s %v: %d predicates, want %d", tc.typeName, tc.params, len(preds), tc.preds)
		}
		if got := UnknownFieldsFromCtx(ctx); !slices.Equal(got, tc.unknown) {
			t.Errorf("%s %v: unknown = %v, want %v", tc.typeName, tc.params, got, tc.unknown)
		}
	}
}

// TestMetaFilter_CountMatchesList checks that the budget count and the
// served list apply the same meta predicates (the 413 guarantee).
func TestMetaFilter_CountMatchesList(t *testing.T) {
	t.Parallel()
	c := seedMetaFilterRows(t)
	tc := Registry[peeringdb.TypeNetIXLan]
	for _, q := range []string{"meta__rfc8950=false", "meta__planned_status_change__date__lt=2026-12-01"} {
		params, err := url.ParseQuery(q)
		if err != nil {
			t.Fatal(err)
		}
		preds, _, err := ParseFilters(params, tc)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		opts := QueryOptions{Filters: preds}
		rows, err := tc.List(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: list: %v", q, err)
		}
		n, err := tc.Count(t.Context(), c, opts)
		if err != nil {
			t.Fatalf("%s: count: %v", q, err)
		}
		if n != len(rows) || n == 0 {
			t.Errorf("%s: count %d, list %d rows (want equal and non-zero)", q, n, len(rows))
		}
	}
}
