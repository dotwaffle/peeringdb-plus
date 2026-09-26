package pdbcompat

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// seedListDepthRows seeds seed.Full plus the children that the list
// depth sets must leave out or order: org 2 with net 9100 (ok) and net
// 9101 (deleted); netixlans 9200-9203 on net 9100 and ixlan 100 (ok,
// not-operational, deleted, pending); campus 41 (pending, org 1); fac
// 32 (deleted, org 1); netfac 299 (net 10, fac 31), whose id is below
// netfac 300 (fac 30), so the facility order differs from the id order;
// a second netixlan of net 11 on ixlan 100, so ixlan 100 net_set has
// two nets; and a Private poc 9002 on net 10.
func seedListDepthRows(t *testing.T, client *ent.Client) *seed.Result {
	t.Helper()
	ctx := t.Context()
	r := seed.Full(t, client)
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	org2 := client.Organization.Create().
		SetID(2).SetName("SecondOrg").SetNameFold(unifold.Fold("SecondOrg")).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	for id, status := range map[int]string{9100: "ok", 9101: "deleted"} {
		client.Network.Create().
			SetID(id).SetOrganization(org2).SetAsn(64000 + id).
			SetName("Net" + strconv.Itoa(id)).SetNameFold(unifold.Fold("Net" + strconv.Itoa(id))).
			SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
	}
	for id, status := range map[int]string{9200: "ok", 9201: "not-operational", 9202: "deleted", 9203: "pending"} {
		client.NetworkIxLan.Create().
			SetID(id).SetNetID(9100).SetIxLan(r.IxLan).SetAsn(73100).SetSpeed(1000).
			SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
	}
	client.NetworkIxLan.Create().
		SetID(9204).SetNetID(r.Network2.ID).SetIxLan(r.IxLan).SetAsn(6939).SetSpeed(1000).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.Campus.Create().
		SetID(41).SetName("Pending Campus").SetNameFold(unifold.Fold("Pending Campus")).
		SetOrgID(r.Org.ID).SetCreated(now).SetUpdated(now).SetStatus("pending").SaveX(ctx)
	client.Facility.Create().
		SetID(32).SetName("Deleted Fac").SetNameFold(unifold.Fold("Deleted Fac")).
		SetOrgID(r.Org.ID).SetCreated(now).SetUpdated(now).SetStatus("deleted").SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(299).SetNetID(r.Network.ID).SetFacID(r.Facility2.ID).SetLocalAsn(13335).
		SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
	client.Poc.Create().
		SetID(9002).SetNetID(r.Network.ID).SetName("Private NOC").SetRole("NOC").
		SetVisible("Private").SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(privacy.DecisionContext(ctx, privacy.Allow))
	return r
}

// setTypes lists the types that have reverse sets.
func setTypes() []string {
	return []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeIX,
		peeringdb.TypeIXLan, peeringdb.TypeCarrier, peeringdb.TypeCampus,
	}
}

// renderListDepth runs ListIDs and ListDepth for one type over opts and
// returns the rendered rows.
func renderListDepth(ctx context.Context, t *testing.T, client *ent.Client, typ string, opts QueryOptions, depth int, sets []childSet) []map[string]any {
	t.Helper()
	tc := Registry[typ]
	ids, err := tc.ListIDs(ctx, client, opts)
	if err != nil {
		t.Fatalf("%s ListIDs: %v", typ, err)
	}
	renders, err := tc.ListDepth(ctx, client, opts, ids, depth, sets)
	if err != nil {
		t.Fatalf("%s ListDepth: %v", typ, err)
	}
	rows := make([]map[string]any, 0, len(renders))
	for _, render := range renders {
		m, ok := render().(map[string]any)
		if !ok {
			t.Fatalf("%s ListDepth renders %T, want map[string]any", typ, render())
		}
		rows = append(rows, m)
	}
	return rows
}

// jsonValue returns v after a JSON round trip, so values of different Go
// types with the same JSON compare equal.
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestListDepth_CountsMatchRender checks that the list loaders and the
// childSets counts use the same filters: for each set, the sum of the
// countByParent counts over the served ids equals the number of elements
// that ListDepth renders, at depth 1 and 2, for an anonymous caller and
// for the Users tier.
func TestListDepth_CountsMatchRender(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedListDepthRows(t, client)
	for _, tier := range []privctx.Tier{privctx.TierPublic, privctx.TierUsers} {
		ctx := privctx.WithTier(t.Context(), tier)
		for _, typ := range setTypes() {
			sets := childSets[typ]
			ids, err := Registry[typ].ListIDs(ctx, client, QueryOptions{})
			if err != nil {
				t.Fatalf("%s ListIDs: %v", typ, err)
			}
			for _, depth := range []int{1, 2} {
				rows := renderListDepth(ctx, t, client, typ, QueryOptions{}, depth, sets)
				for _, cs := range sets {
					counts, err := cs.countByParent(ctx, client, ids)
					if err != nil {
						t.Fatalf("%s.%s count: %v", typ, cs.key, err)
					}
					want := 0
					for _, n := range counts {
						want += n
					}
					got := 0
					for _, row := range rows {
						got += reflect.ValueOf(row[cs.key]).Len()
					}
					if got != want {
						t.Errorf("tier %v %s depth %d %s: renders %d elements, countByParent gives %d", tier, typ, depth, cs.key, got, want)
					}
				}
			}
		}
	}
}

// TestListDepth_MatchesDetail checks that a list row at depth 1 and 2 is
// the detail object at the same depth without its forward FK object
// (org, campus, ix): upstream renders the same sets on both paths and
// removes Meta.list_exclude from a list row (2.83.0
// serializers.py:1286-1290). It locks the list loaders to the detail
// getters, including the set order and the back-reference strips.
func TestListDepth_MatchesDetail(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedListDepthRows(t, client)
	ctx := t.Context()
	for _, typ := range setTypes() {
		for _, depth := range []int{1, 2} {
			rows := renderListDepth(ctx, t, client, typ, QueryOptions{}, depth, childSets[typ])
			if len(rows) == 0 {
				t.Fatalf("%s depth %d: no rows", typ, depth)
			}
			for _, row := range rows {
				id := int(jsonValue(t, row["id"]).(float64))
				detail, err := Registry[typ].Get(ctx, client, id, depth)
				if err != nil {
					t.Fatalf("%s/%d depth %d: %v", typ, id, depth, err)
				}
				want, ok := jsonValue(t, detail).(map[string]any)
				if !ok {
					t.Fatalf("%s/%d depth %d: detail is not an object", typ, id, depth)
				}
				for _, k := range []string{"org", "campus", "ix"} {
					delete(want, k)
				}
				if got := jsonValue(t, row); !reflect.DeepEqual(got, any(want)) {
					t.Errorf("%s/%d depth %d:\n list   %v\n detail %v", typ, id, depth, got, want)
				}
			}
		}
	}
}

// TestSelectSets checks that ?fields= selects the sets that it names,
// with the names trimmed, and all sets without it.
func TestSelectSets(t *testing.T) {
	t.Parallel()
	keys := func(sets []childSet) []string {
		out := make([]string, 0, len(sets))
		for _, cs := range sets {
			out = append(out, cs.key)
		}
		return out
	}
	for _, tc := range []struct {
		typ    string
		fields []string
		want   []string
	}{
		{peeringdb.TypeOrg, nil, []string{"net_set", "fac_set", "ix_set", "carrier_set", "campus_set"}},
		{peeringdb.TypeOrg, []string{"name"}, []string{}},
		{peeringdb.TypeOrg, []string{"name", " fac_set", "net_set "}, []string{"net_set", "fac_set"}},
		{peeringdb.TypeNet, []string{"irr_as_set", "poc_set"}, []string{"poc_set"}},
		{peeringdb.TypePoc, nil, []string{}},
	} {
		if got := keys(selectSets(tc.typ, tc.fields)); !slices.Equal(got, tc.want) {
			t.Errorf("selectSets(%s, %q) = %v, want %v", tc.typ, tc.fields, got, tc.want)
		}
	}
}

// TestRegistry_ListDepthMatchesChildSets checks the init invariant:
// every type has ListIDs, and a type has ListDepth if and only if
// childSets lists sets for it.
func TestRegistry_ListDepthMatchesChildSets(t *testing.T) {
	t.Parallel()
	for name, tc := range Registry {
		if tc.ListIDs == nil {
			t.Errorf("%s: ListIDs is nil", name)
		}
		if (tc.ListDepth != nil) != (len(childSets[name]) > 0) {
			t.Errorf("%s: ListDepth set = %v, childSets entries = %d", name, tc.ListDepth != nil, len(childSets[name]))
		}
	}
}

// TestParseListFilters_UpstreamFilter checks which keys count in the
// upstream API cache gate (listFilters.upstreamFilter).
func TestParseListFilters_UpstreamFilter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, typ, query string
		want             bool
		wantPreds        bool
	}{
		{"model_field", peeringdb.TypeNet, "name=x", true, true},
		{"empty_in", peeringdb.TypeNet, "id__in=", true, false},
		// upstream sets query_adjusted (serializers.py:3775-3810).
		{"info_type_matches_all", peeringdb.TypeNet, "info_type=", true, false},
		{"info_type_in_matches_all", peeringdb.TypeNet, "info_type__in=,x", true, false},
		// get_relation_filters keeps a key of up to 3 segments
		// (serializers.py:641-654), though prepare_query ignores it.
		{"ignored_seed_form", peeringdb.TypeFac, "org_name__iexact=x", true, false},
		{"unknown_key", peeringdb.TypeNet, "bogus=1", false, false},
		{"reserved_only", peeringdb.TypeNet, "limit=5&depth=1&skip=2&fields=name", false, false},
		// A distance of 0 or less adds no filter and no spatial query
		// upstream (serializers.py:1837-1840), so the cache serves it.
		{"distance_noop", peeringdb.TypeFac, "distance=0", false, false},
		{"distance_search", peeringdb.TypeFac, "distance=10&latitude=50&longitude=8", true, true},
		{"presence_key", peeringdb.TypeNet, "not_ix=1", true, true},
		{"hide_ix_no_fac", peeringdb.TypeIX, "hide_ix_no_fac=1", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("parse %q: %v", tc.query, err)
			}
			ctx := WithUnknownFields(t.Context())
			lf, err := parseListFilters(ctx, params, Registry[tc.typ])
			if err != nil {
				t.Fatalf("parseListFilters(%s?%s): %v", tc.typ, tc.query, err)
			}
			if lf.upstreamFilter != tc.want {
				t.Errorf("upstreamFilter = %v, want %v", lf.upstreamFilter, tc.want)
			}
			if got := len(lf.preds) > 0; got != tc.wantPreds {
				t.Errorf("has predicates = %v, want %v", got, tc.wantPreds)
			}
		})
	}
	// The ignored seed form is still reported as an unknown key.
	ctx := WithUnknownFields(t.Context())
	if _, err := parseListFilters(ctx, url.Values{"org_name__iexact": {"x"}}, Registry[peeringdb.TypeFac]); err != nil {
		t.Fatal(err)
	}
	if got := UnknownFieldsFromCtx(ctx); !slices.Equal(got, []string{"org_name__iexact"}) {
		t.Errorf("unknown fields = %v, want [org_name__iexact]", got)
	}
}

// seedOrgsWithNets seeds orgs 1..len(nets); org i gets nets[i-1] ok
// networks and one deleted network. Network ids start at 1000.
func seedOrgsWithNets(t *testing.T, client *ent.Client, nets []int) {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	netID := 1000
	for i, n := range nets {
		org := client.Organization.Create().
			SetID(i + 1).SetName("Org" + strconv.Itoa(i+1)).SetNameFold(unifold.Fold("Org" + strconv.Itoa(i+1))).
			SetCreated(now).SetUpdated(now).SetStatus("ok").SaveX(ctx)
		for j := 0; j <= n; j++ {
			status := "ok"
			if j == n {
				status = "deleted"
			}
			client.Network.Create().
				SetID(netID).SetOrganization(org).SetAsn(64000 + netID).
				SetName("Net" + strconv.Itoa(netID)).SetNameFold(unifold.Fold("Net" + strconv.Itoa(netID))).
				SetCreated(now).SetUpdated(now).SetStatus(status).SaveX(ctx)
			netID++
		}
	}
}

// TestListDepthEstimate checks the largest-chunk price of a list at
// depth 1 and 2 against the formula of listDepthEstimate.
func TestListDepthEstimate(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	seedOrgsWithNets(t, client, []int{1, 2, 3, 4, 5})
	ids := []int{1, 2, 3, 4, 5}
	orgRow := int64(TypicalRowBytes(peeringdb.TypeOrg, 0))
	netRow := int64(TypicalRowBytes(peeringdb.TypeNet, 0))
	// rowBytes returns the row estimates of orgs 1..5 with elem bytes per
	// net.
	rowBytes := func(elem int64) []int64 {
		out := make([]int64, 5)
		for i := range out {
			out[i] = orgRow + int64(i+1)*elem
		}
		return out
	}
	// want returns the formula result for chunks of 2.
	want := func(rows []int64) listDepthCost {
		var c listDepthCost
		for start := 0; start < len(rows); start += 2 {
			chunk := rows[start:min(start+2, len(rows))]
			var sum, top int64
			for _, b := range chunk {
				sum += b
				top = max(top, b)
			}
			if cb := sum + 4*top; cb > c.chunkBytes {
				c.chunkBytes, c.chunkRows = cb, len(chunk)
			}
		}
		c.bytes = 16*int64(len(rows)) + c.chunkBytes
		return c
	}
	for _, tc := range []struct {
		name  string
		depth int
		sets  []childSet
		want  listDepthCost
	}{
		{"depth1", 1, childSets[peeringdb.TypeOrg], want(rowBytes(16))},
		{"depth2", 2, childSets[peeringdb.TypeOrg], want(rowBytes(netRow))},
		{"depth2_fields_without_net_set", 2, selectSets(peeringdb.TypeOrg, []string{"fac_set"}), want(rowBytes(0))},
	} {
		got, err := listDepthEstimate(ctx, client, peeringdb.TypeOrg, ids, tc.depth, tc.sets, 2)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: estimate = %+v, want %+v", tc.name, got, tc.want)
		}
	}
	// The whole list in one chunk.
	got, err := listDepthEstimate(ctx, client, peeringdb.TypeOrg, ids, 1, childSets[peeringdb.TypeOrg], 250)
	if err != nil {
		t.Fatal(err)
	}
	rows := rowBytes(16)
	var sum int64
	for _, b := range rows {
		sum += b
	}
	if wantBytes := 16*5 + sum + 4*rows[4]; got.bytes != wantBytes || got.chunkRows != 5 {
		t.Errorf("one chunk: estimate = %+v, want bytes %d over 5 rows", got, wantBytes)
	}

	// A poc that the caller cannot read is not priced.
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for id, visible := range map[int]string{800: "Public", 801: "Users"} {
		client.Poc.Create().
			SetID(id).SetNetID(1000).SetName("Poc").SetRole("NOC").SetVisible(visible).
			SetCreated(now).SetUpdated(now).SetStatus("ok").
			SaveX(privacy.DecisionContext(ctx, privacy.Allow))
	}
	pocSets := selectSets(peeringdb.TypeNet, []string{"poc_set"})
	got, err = listDepthEstimate(ctx, client, peeringdb.TypeNet, []int{1000}, 2, pocSets, 250)
	if err != nil {
		t.Fatal(err)
	}
	row := netRow + int64(TypicalRowBytes(peeringdb.TypePoc, 0))
	if wantBytes := 16 + 5*row; got.bytes != wantBytes {
		t.Errorf("anonymous poc_set: estimate = %d, want %d (Public poc only)", got.bytes, wantBytes)
	}
}

// TestListDepthRenderFactor checks listDepthRenderFactor against the
// allocation of one rendered row: toMap of the serialized seed row, a
// key delete and json.Marshal of the map must allocate at most
// listDepthRenderFactor × the Depth0 figure of the type. It is not
// parallel: the allocation counts of testing.Benchmark are process-wide.
func TestListDepthRenderFactor(t *testing.T) {
	// The race detector makes each allocation larger, so the byte
	// counts are valid only in a build without it (go test ./...).
	if raceEnabled {
		t.Skip("allocation sizes differ under the race detector")
	}
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)
	ctx := t.Context()
	rows := map[string]any{
		peeringdb.TypeOrg:        organizationFromEnt(r.Org),
		peeringdb.TypeNet:        networkFromEnt(r.Network),
		peeringdb.TypeFac:        facilityFromEnt(r.Facility),
		peeringdb.TypeIX:         internetExchangeFromEnt(r.IX),
		peeringdb.TypePoc:        pocFromEnt(r.Poc),
		peeringdb.TypeIXLan:      ixLanFromEnt(ctx, r.IxLanPublic),
		peeringdb.TypeIXPfx:      ixPrefixFromEnt(r.IxPrefix),
		peeringdb.TypeNetIXLan:   networkIxLanFromEnt(r.NetworkIxLan),
		peeringdb.TypeNetFac:     networkFacilityFromEnt(r.NetworkFacility),
		peeringdb.TypeCarrier:    carrierFromEnt(r.Carrier),
		peeringdb.TypeCarrierFac: carrierFacilityFromEnt(r.CarrierFacility),
		peeringdb.TypeCampus:     campusFromEnt(r.Campus),
	}
	// Few iterations are enough: the allocation of one render does not
	// change between runs.
	bt := flag.Lookup("test.benchtime")
	if bt == nil {
		t.Skip("test.benchtime flag not registered")
	}
	old := bt.Value.String()
	if err := bt.Value.Set("200x"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bt.Value.Set(old) }()
	for typ, row := range rows {
		res := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				m := toMap(row)
				delete(m, "org_id")
				if _, err := json.Marshal(m); err != nil {
					b.Fatal(err)
				}
			}
		})
		limit := int64(listDepthRenderFactor * TypicalRowBytes(typ, 0))
		if got := res.AllocedBytesPerOp(); got > limit {
			t.Errorf("%s: render allocates %d bytes, want at most %d (%d × Depth0)", typ, got, limit, listDepthRenderFactor)
		}
	}
}

// newListDepthMux returns a mux over a Handler with the given budget and
// chunk size.
func newListDepthMux(client *ent.Client, budget int64, chunk int) (*Handler, *http.ServeMux) {
	h := NewHandler(client, budget)
	h.listDepthChunk = chunk
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

// getList sends a GET to mux and returns the recorder.
func getList(mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// listIDs decodes data[].id of a list response.
func listIDs(t *testing.T, body []byte) []int {
	t.Helper()
	var env struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	out := make([]int, 0, len(env.Data))
	for _, row := range env.Data {
		out = append(out, row.ID)
	}
	return out
}

// TestListDepth_ChunkOrder checks that the rows of a list loaded in
// chunks keep the list order: (updated, id) with ?since.
func TestListDepth_ChunkOrder(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	// id -> hours after base: the updated order is 4, 2, 5, 1, 3.
	for id, h := range map[int]int{1: 4, 2: 2, 3: 5, 4: 1, 5: 3} {
		client.Organization.Create().
			SetID(id).SetName("Org" + strconv.Itoa(id)).SetNameFold(unifold.Fold("Org" + strconv.Itoa(id))).
			SetCreated(base).SetUpdated(base.Add(time.Duration(h) * time.Hour)).SetStatus("ok").SaveX(ctx)
	}
	_, mux := newListDepthMux(client, 0, 2)
	rec := getList(mux, "/api/org?since=1&depth=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if got, want := listIDs(t, rec.Body.Bytes()), []int{4, 2, 5, 1, 3}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

// TestListDepth_LoadsInChunks checks that a list at depth > 0 loads its
// rows in chunks of listDepthChunk ids, one query per chunk, and sends no
// count query when the budget is off and the list is not live.
func TestListDepth_LoadsInChunks(t *testing.T) {
	t.Parallel()
	seedClient, db := testutil.SetupClientWithDB(t)
	seedOrgsWithNets(t, seedClient, []int{0, 1, 2, 0, 1})
	rec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
	client := ent.NewClient(ent.Driver(rec))
	_, mux := newListDepthMux(client, 0, 2)
	resp := getList(mux, "/api/org?depth=1")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d: %s", resp.Code, resp.Body.String())
	}
	if got := listIDs(t, resp.Body.Bytes()); !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("ids = %v, want [1 2 3 4 5]", got)
	}
	chunks := 0
	for _, q := range rec.queries() {
		if strings.Contains(strings.ToUpper(q.q), "COUNT(") {
			t.Errorf("budget off, unfiltered list: count query sent: %s", q.q)
		}
		if strings.HasPrefix(q.q, "SELECT") && strings.Contains(q.q, "FROM `organizations`") && strings.Contains(q.q, "json_each") {
			chunks++
		}
	}
	if chunks != 3 {
		t.Errorf("chunk queries = %d, want 3 (5 rows in chunks of 2)", chunks)
	}
}

// snapshotWriter records, at each Write, the bytes and the counters
// that snap returns.
type snapshotWriter struct {
	*httptest.ResponseRecorder
	snap   func() [2]int
	writes []string
	snaps  [][2]int
}

func (w *snapshotWriter) Write(b []byte) (int, error) {
	w.writes = append(w.writes, string(b))
	w.snaps = append(w.snaps, w.snap())
	return w.ResponseRecorder.Write(b)
}

// TestListDepth_RendersLazily checks that a list at depth > 0 loads one
// chunk before the first row is written and builds each row map only
// when the stream pulls the row. listDepthEstimate prices one rendered
// row per chunk (listDepthRenderFactor), so a render of a whole chunk
// at once would break the estimate.
func TestListDepth_RendersLazily(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedOrgsWithNets(t, client, []int{0, 1, 2, 0, 1})
	h := NewHandler(client, 0)
	h.listDepthChunk = 2

	tc := Registry[peeringdb.TypeOrg]
	inner := tc.ListDepth
	var loads, renders int
	tc.ListDepth = func(ctx context.Context, c *ent.Client, opts QueryOptions, ids []int, depth int, sets []childSet) ([]func() any, error) {
		loads++
		rows, err := inner(ctx, c, opts, ids, depth, sets)
		for i, render := range rows {
			rows[i] = func() any {
				renders++
				return render()
			}
		}
		return rows, err
	}
	w := &snapshotWriter{
		ResponseRecorder: httptest.NewRecorder(),
		snap:             func() [2]int { return [2]int{loads, renders} },
	}
	h.serveList(tc, w, httptest.NewRequest(http.MethodGet, "/api/org?depth=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if got := listIDs(t, w.Body.Bytes()); !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("ids = %v, want [1 2 3 4 5]", got)
	}

	// The rows follow the data-open write, with a "," write between two
	// rows.
	open := slices.Index(w.writes, `,"data":[`)
	if open < 0 {
		t.Fatalf("no data-open write in %q", w.writes)
	}
	want := [][2]int{{1, 1}, {1, 2}, {2, 3}, {2, 4}, {3, 5}} // {loads, renders} at rows 1-5
	for i, wantSnap := range want {
		idx := open + 1 + 2*i
		if idx >= len(w.snaps) {
			t.Fatalf("row %d: no write", i+1)
		}
		if got := w.snaps[idx]; got != wantSnap {
			t.Errorf("row %d written after %d chunk loads and %d renders, want %d and %d",
				i+1, got[0], got[1], wantSnap[0], wantSnap[1])
		}
	}

	// A closure of renderListDepthChunk builds its row only when called.
	orgs := client.Organization.Query().Order(ent.Asc("id")).Limit(2).AllX(t.Context())
	converts := 0
	convert := func(o *ent.Organization) any {
		converts++
		return organizationFromEnt(o)
	}
	rows, err := renderListDepthChunk(t.Context(), client, orgs, []int{orgs[0].ID, orgs[1].ID}, 1,
		selectSets(peeringdb.TypeOrg, nil), convert)
	if err != nil {
		t.Fatalf("renderListDepthChunk: %v", err)
	}
	if converts != 0 {
		t.Errorf("renderListDepthChunk converted %d rows before a pull, want 0", converts)
	}
	rows[0]()
	if converts != 1 {
		t.Errorf("one pull converted %d rows, want 1", converts)
	}
}

// TestServeList_DepthSpanAttributes checks the span attributes of a
// list at depth > 0 under a budget: the depth, the truncation flag, the
// estimate and the number of chunks.
func TestServeList_DepthSpanAttributes(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedOrgsWithNets(t, client, []int{0, 1, 2, 0, 1})
	_, mux := newListDepthMux(client, 1<<30, 2)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-list-depth")
	req := httptest.NewRequest(http.MethodGet, "/api/org?depth=1&name__startswith=Org", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	span.End()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	attrs := map[attribute.Key]attribute.Value{}
	for _, s := range exporter.GetSpans() {
		for _, a := range s.Attributes {
			attrs[a.Key] = a.Value
		}
	}
	if v, ok := attrs["pdbplus.list.depth"]; !ok || v.AsInt64() != 1 {
		t.Errorf("pdbplus.list.depth = %v (present=%v), want 1", v.String(), ok)
	}
	if v, ok := attrs["pdbplus.list.truncated"]; !ok || v.AsBool() {
		t.Errorf("pdbplus.list.truncated = %v (present=%v), want false", v.String(), ok)
	}
	if v, ok := attrs["pdbplus.list.estimated_bytes"]; !ok || v.AsInt64() <= 0 {
		t.Errorf("pdbplus.list.estimated_bytes = %v (present=%v), want > 0", v.String(), ok)
	}
	if v, ok := attrs["pdbplus.list.chunks"]; !ok || v.AsInt64() != 3 {
		t.Errorf("pdbplus.list.chunks = %v (present=%v), want 3 (5 rows in chunks of 2)", v.String(), ok)
	}
}

// TestListDepth_IxlanURLRedacted checks that an ixlan element of
// ix.ixlan_set at list depth 2 goes through privfield.Redact: for an
// anonymous caller the Users ixlan has no ixf_ixp_member_list_url key,
// and the Public ixlans keep it. The Users tier sees all three. The
// Public ixlan with a NULL URL keeps the key with null at both tiers
// (upstream DRF renders None as null).
func TestListDepth_IxlanURLRedacted(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seed.Full(t, client)
	const publicNullID = 102
	client.IxLan.Create().SetID(publicNullID).SetIxID(20).
		SetIxfIxpMemberListURLVisible("Public").
		SetStatus("ok").SetCreated(seed.Timestamp).SetUpdated(seed.Timestamp).
		SaveX(t.Context())
	_, mux := newListDepthMux(client, 0, defaultListDepthChunk)
	for _, tc := range []struct {
		tier     privctx.Tier
		wantURLs map[int]bool
	}{
		{privctx.TierPublic, map[int]bool{seed.IxLanGatedID: false, seed.IxLanPublicID: true, publicNullID: true}},
		{privctx.TierUsers, map[int]bool{seed.IxLanGatedID: true, seed.IxLanPublicID: true, publicNullID: true}},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/ix?id=20&depth=2", nil)
		req = req.WithContext(privctx.WithTier(req.Context(), tc.tier))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("tier %v: status %d: %s", tc.tier, rec.Code, rec.Body.String())
		}
		var env struct {
			Data []struct {
				IxlanSet []map[string]any `json:"ixlan_set"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || len(env.Data) != 1 {
			t.Fatalf("tier %v: decode: %v; body=%s", tc.tier, err, rec.Body.String())
		}
		seen := map[int]bool{}
		for _, l := range env.Data[0].IxlanSet {
			id := int(l["id"].(float64))
			url, has := l["ixf_ixp_member_list_url"]
			seen[id] = true
			if has != tc.wantURLs[id] {
				t.Errorf("tier %v: ixlan %d has URL key = %v, want %v", tc.tier, id, has, tc.wantURLs[id])
			}
			if id == publicNullID && url != nil {
				t.Errorf("tier %v: ixlan %d URL = %#v, want null", tc.tier, id, url)
			}
			if _, ok := l["ixf_ixp_member_list_url_visible"]; !ok {
				t.Errorf("tier %v: ixlan %d lost its _visible companion", tc.tier, id)
			}
		}
		if len(seen) != 3 {
			t.Errorf("tier %v: ixlan_set ids = %v, want the 3 ixlans of ix 20", tc.tier, seen)
		}
	}
}

// TestServeList_DepthFanOutOver413 checks that a list at depth 2 prices
// its sets: one org with 100 nets is over a 64 KiB budget at depth 2
// (max_rows 0) and fits at depth 0; five orgs of 20 nets each get
// max_rows from the formula of listDepthCost.exceeded.
func TestServeList_DepthFanOutOver413(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedOrgsWithNets(t, client, []int{100})
	_, mux := newListDepthMux(client, 64<<10, defaultListDepthChunk)
	rec := getList(mux, "/api/org?depth=2")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("depth=2: status %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
	if got := decodeMaxRows(t, rec.Body.Bytes()); got != 0 {
		t.Errorf("depth=2: max_rows = %d, want 0 (one row over the budget)", got)
	}
	if rec := getList(mux, "/api/org?depth=0"); rec.Code != http.StatusOK {
		t.Errorf("depth=0: status %d, want 200", rec.Code)
	}

	client2 := testutil.SetupClient(t)
	seedOrgsWithNets(t, client2, []int{20, 20, 20, 20, 20})
	row := int64(TypicalRowBytes(peeringdb.TypeOrg, 0) + 20*TypicalRowBytes(peeringdb.TypeNet, 0))
	chunkBytes := 5*row + listDepthRenderFactor*row
	budget := chunkBytes * 3 / 5
	_, mux2 := newListDepthMux(client2, budget, defaultListDepthChunk)
	rec = getList(mux2, "/api/org?depth=2")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("5 orgs: status %d, want 413; body=%s", rec.Code, rec.Body.String())
	}
	if got, want := decodeMaxRows(t, rec.Body.Bytes()), int(budget*5/chunkBytes); got != want {
		t.Errorf("5 orgs: max_rows = %d, want %d", got, want)
	}
}

// decodeMaxRows returns meta.max_rows of a 413 body.
func decodeMaxRows(t *testing.T, body []byte) int {
	t.Helper()
	var env struct {
		Meta struct {
			MaxRows *int `json:"max_rows"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Meta.MaxRows == nil {
		t.Fatalf("decode max_rows: %v; body=%s", err, body)
	}
	return *env.Meta.MaxRows
}

// TestServeList_DepthPricesLargestChunk checks that a list loaded in
// chunks is priced by its most expensive chunk plus the id slice, not by
// the whole response. Six orgs in chunks of 2; org 3 has 10 nets.
func TestServeList_DepthPricesLargestChunk(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedOrgsWithNets(t, client, []int{0, 0, 10, 0, 0, 0})
	org := int64(TypicalRowBytes(peeringdb.TypeOrg, 0))
	hub := org + 10*listIDBytes
	want := 16*6 + (org + hub + listDepthRenderFactor*hub)
	_, over := newListDepthMux(client, want-1, 2)
	rec := getList(over, "/api/org?depth=1")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("budget %d: status %d, want 413; body=%s", want-1, rec.Code, rec.Body.String())
	}
	if msg := decodeTestMetaError(t, rec.Body.Bytes(), "meta"); !strings.Contains(msg, "~"+strconv.FormatInt(want, 10)+" bytes") {
		t.Errorf("413 message = %q, want the estimate %d", msg, want)
	}
	_, fits := newListDepthMux(client, want, 2)
	if rec := getList(fits, "/api/org?depth=1"); rec.Code != http.StatusOK {
		t.Errorf("budget %d: status %d, want 200; body=%s", want, rec.Code, rec.Body.String())
	}
}

// TestServeList_DepthPoolCharged checks the in-flight pool on the list
// depth path: a full pool returns 503 with Retry-After: 1, and a served
// list releases its charge.
func TestServeList_DepthPoolCharged(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedOrgsWithNets(t, client, []int{1, 2})
	const budget = 1 << 20
	h, mux := newListDepthMux(client, budget, defaultListDepthChunk)
	h.inflightBytes.Store(budget - 10)
	rec := getList(mux, "/api/org?depth=2")
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "1" {
		t.Fatalf("full pool: status %d Retry-After %q, want 503 and 1; body=%s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	h.inflightBytes.Store(0)
	if rec := getList(mux, "/api/org?depth=2"); rec.Code != http.StatusOK {
		t.Fatalf("empty pool: status %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := h.inflightBytes.Load(); got != 0 {
		t.Errorf("pool after a served list = %d, want 0", got)
	}
}

// TestServeList_DepthTruncatesLeafTypes checks that a type without
// reverse sets serves its depth-0 rows at depth > 0, cut to 250 rows
// with meta.truncated on a filtered list, with the budget on and off.
func TestServeList_DepthTruncatesLeafTypes(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	r := seed.Full(t, client)
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	builders := make([]*ent.NetworkIxLanCreate, 0, 260)
	for i := range 260 {
		builders = append(builders, client.NetworkIxLan.Create().
			SetID(10000+i).SetNetID(r.Network.ID).SetIxlanID(r.IxLan.ID).SetAsn(64999).SetSpeed(1000).
			SetCreated(now).SetUpdated(now).SetStatus("ok"))
	}
	client.NetworkIxLan.CreateBulk(builders...).SaveX(ctx)
	for _, budget := range []int64{0, 128 << 20} {
		_, mux := newListDepthMux(client, budget, defaultListDepthChunk)
		rec := getList(mux, "/api/netixlan?asn=64999&depth=2")
		if rec.Code != http.StatusOK {
			t.Fatalf("budget %d: status %d; body=%s", budget, rec.Code, rec.Body.String())
		}
		var env struct {
			Meta map[string]string `json:"meta"`
			Data []map[string]any  `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		if len(env.Data) != 250 || !strings.Contains(env.Meta["truncated"], "(with depth 2)") {
			t.Errorf("budget %d: %d rows, meta %v; want 250 rows and the truncated message", budget, len(env.Data), env.Meta)
		}
		for _, k := range []string{"net", "ixlan"} {
			if _, ok := env.Data[0][k]; ok {
				t.Errorf("budget %d: row has the forward FK object %q", budget, k)
			}
		}
	}
}
