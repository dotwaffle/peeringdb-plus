package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// Phase 69 Plan 04 — filter-layer tests for:
//   - UNICODE-02 operator coercion (__contains → __icontains, __startswith → __istartswith)
//   - UNICODE-01 shadow-column routing for fields with a _fold sibling
//   - IN-01 json_each __in rewrite (single-bind, bypasses SQLite's 999-variable limit)
//   - IN-02 empty __in short-circuit
//
// All tests exercise pdbcompat end-to-end via httptest (black-box HTTP, independent
// of internal filter.go spellings) except the EXPLAIN QUERY PLAN probe which runs
// SQL directly against the same schema-driven ent client.

// newPhase69Mux is a copy of newMuxForOrdering to avoid coupling to that file's
// test fixtures. Kept local to make the Phase 69 test suite self-contained.
func newPhase69Mux(client *ent.Client) *http.ServeMux {
	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// phase69FetchIDs GETs the URL and returns the sorted list of result IDs.
func phase69FetchIDs(t *testing.T, url string) []int {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test code, local httptest server
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	var env struct {
		Meta json.RawMessage          `json:"meta"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	ids := make([]int, 0, len(env.Data))
	for _, row := range env.Data {
		if v, ok := row["id"].(float64); ok {
			ids = append(ids, int(v))
		}
	}
	return ids
}

// seedFoldedNetwork creates a Network with a non-ASCII name and its _fold
// companion set via unifold.Fold (mirrors the sync worker's Phase 69 path).
func seedFoldedNetwork(t *testing.T, client *ent.Client, id int, asn int, name string) {
	t.Helper()
	ctx := t.Context()
	now := time.Now().UTC()
	_, err := client.Network.Create().
		SetID(id).
		SetName(name).
		SetNameFold(unifold.Fold(name)).
		SetAsn(asn).
		SetWebsite("https://example.com").
		SetStatus("ok").
		SetCreated(now).
		SetUpdated(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed folded network id=%d: %v", id, err)
	}
}

// TestShadowRouting_Network_NameFold — UNICODE-01: a row with name="Zürich Connect"
// (name_fold="zurich connect") must match ?name__contains=zurich, ?name__contains=Zurich,
// and ?name__contains=urich con (URL-encoded space).
func TestShadowRouting_Network_NameFold(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedFoldedNetwork(t, client, 1, 64501, "Zürich Connect")
	seedFoldedNetwork(t, client, 2, 64502, "London Metro")

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	cases := []struct {
		name  string
		query string
		want  []int
	}{
		{"ascii lowercase fold matches diacritic", "name__contains=zurich", []int{1}},
		{"ascii titlecase fold matches diacritic (operator coerced)", "name__contains=Zurich", []int{1}},
		{"substring across word boundary via _fold", "name__contains=urich+con", []int{1}},
		{"non-matching substring returns empty", "name__contains=seattle", []int{}},
		{"startswith on folded column", "name__startswith=Zuri", []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids := phase69FetchIDs(t, srv.URL+"/api/net?"+tc.query)
			if !sameIDs(ids, tc.want) {
				t.Errorf("query %q: got ids %v, want %v", tc.query, ids, tc.want)
			}
		})
	}
}

// TestShadowRouting_Network_NonFoldedField — `website` has no _fold shadow.
// Queries on website must still work via the existing FieldContainsFold path.
func TestShadowRouting_Network_NonFoldedField(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedFoldedNetwork(t, client, 1, 64501, "Zürich Connect")
	// website is indexed into the schema but has NO fold shadow. Update to a
	// distinctive URL so the substring match is unambiguous.
	ctx := t.Context()
	_, err := client.Network.UpdateOneID(1).SetWebsite("https://cloudflare.example.com").Save(ctx)
	if err != nil {
		t.Fatalf("set website: %v", err)
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	// ?website__contains=cloudflare — website is not in foldedFields, so
	// routing falls through to sql.FieldContainsFold(name, value) against the
	// real column. Must match.
	ids := phase69FetchIDs(t, srv.URL+"/api/net?website__contains=cloudflare")
	if !sameIDs(ids, []int{1}) {
		t.Errorf("non-folded field website: got ids %v, want [1]", ids)
	}
}

// TestInJsonEach_Large_Bypasses_SQLite_Limit — IN-01: a large __in list (well
// above the pre-modernc SQLite default of 999 variables) must succeed. modernc
// at v1.48.2 compiles with MAX_VARIABLE_NUMBER=32766, so the old expanded-param
// path would survive 1500, but the rewrite is the correctness-first path: one
// bind regardless of list size. Keep the smoke test modest; the plan-level
// correctness guard is TestInJsonEach_ExplainQueryPlan.
func TestInJsonEach_Large_Bypasses_SQLite_Limit(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	// Seed 1500 networks with ASNs 1..1500. CreateBulk binds each column as
	// a separate parameter; chunk to 100 per batch to stay well under the
	// SQLite variable limit regardless of CGO/modernc defaults.
	const n = 1500
	const chunk = 100
	for start := 0; start < n; start += chunk {
		end := start + chunk
		if end > n {
			end = n
		}
		builders := make([]*ent.NetworkCreate, 0, end-start)
		for i := start; i < end; i++ {
			builders = append(builders, client.Network.Create().
				SetID(i+1).
				SetName(fmt.Sprintf("Net%d", i+1)).
				SetAsn(i+1).
				SetStatus("ok").
				SetCreated(now).
				SetUpdated(now))
		}
		if _, err := client.Network.CreateBulk(builders...).Save(ctx); err != nil {
			t.Fatalf("seed networks [%d,%d): %v", start, end, err)
		}
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	asns := make([]string, n)
	for i := 0; i < n; i++ {
		asns[i] = fmt.Sprintf("%d", i+1)
	}
	query := "asn__in=" + strings.Join(asns, ",") + "&limit=0"
	ids := phase69FetchIDs(t, srv.URL+"/api/net?"+query)
	if len(ids) != n {
		t.Errorf("large __in: got %d rows, want %d (SQLite 999-variable limit should be bypassed)", len(ids), n)
	}
}

// TestInJsonEach_EmptyString_ReturnsEmpty — IN-02: ?asn__in= (empty value) must
// return an empty data array without running SQL.
func TestInJsonEach_EmptyString_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	// Seed some data that MUST be excluded by the empty __in.
	for i := 1; i <= 3; i++ {
		_, err := client.Network.Create().
			SetID(i).SetName(fmt.Sprintf("Net%d", i)).SetAsn(64500 + i).
			SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
		if err != nil {
			t.Fatalf("seed net id=%d: %v", i, err)
		}
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	ids := phase69FetchIDs(t, srv.URL+"/api/net?asn__in=")
	if len(ids) != 0 {
		t.Errorf("empty __in: got ids %v, want [] (per D-06 — Django id__in=[] semantics)", ids)
	}
}

// TestInJsonEach_StringValues — IN on a string field must also work.
func TestInJsonEach_StringValues(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().UTC()
	_, err := client.Network.Create().SetID(1).SetName("alpha").SetAsn(1).
		SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = client.Network.Create().SetID(2).SetName("beta").SetAsn(2).
		SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = client.Network.Create().SetID(3).SetName("gamma").SetAsn(3).
		SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)
	// name__in on website (string FieldString) — use name__in because `name`
	// is in Fields map.
	ids := phase69FetchIDs(t, srv.URL+"/api/net?name__in=alpha,gamma")
	if !sameIDs(ids, []int{1, 3}) {
		t.Errorf("string __in: got ids %v, want [1 3]", ids)
	}
}

// TestInJsonEach_ExplainQueryPlan — D-05 correctness check: modernc.org/sqlite
// keeps json_each(?) as a single bind rather than expanding to N parameters.
// This test runs a raw SQL query through the same driver and confirms the
// EXPLAIN QUERY PLAN output references json_each.
func TestInJsonEach_ExplainQueryPlan(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	// Seed one row so schema is materialised (ent auto-migrate runs on first query).
	ctx := t.Context()
	_, err := client.Network.Create().SetID(1).SetName("Probe").SetAsn(1).
		SetStatus("ok").SetCreated(time.Now()).SetUpdated(time.Now()).Save(ctx)
	if err != nil {
		t.Fatalf("seed probe: %v", err)
	}

	// The production path builds this exact SQL shape via ent sql.ExprP; we
	// replicate the fragment directly to validate SQLite's plan.
	rows, err := db.QueryContext(ctx,
		`EXPLAIN QUERY PLAN SELECT id FROM networks WHERE asn IN (SELECT value FROM json_each(?))`,
		`[1,2,3]`,
	)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	joined := strings.Join(plan, " | ")
	if !strings.Contains(strings.ToLower(joined), "json_each") {
		t.Errorf("EXPLAIN QUERY PLAN did not mention json_each — modernc.org/sqlite may have expanded the bind. plan=%q", joined)
	}
}

// TestCoerce_OnlyContainsAndStartswith_Untouched — D-04 scope guard: all other
// operators MUST behave unchanged.
func TestCoerce_OnlyContainsAndStartswith_Untouched(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().UTC()
	_, err := client.Network.Create().SetID(1).SetName("N1").SetAsn(100).
		SetInfoUnicast(true).SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = client.Network.Create().SetID(2).SetName("N2").SetAsn(2000).
		SetInfoUnicast(false).SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	cases := []struct {
		name  string
		query string
		want  []int
	}{
		{"asn__gt unchanged", "asn__gt=1000", []int{2}},
		{"asn__lt unchanged", "asn__lt=1000", []int{1}},
		{"info_unicast bool exact unchanged", "info_unicast=true", []int{1}},
		{"asn__gte unchanged", "asn__gte=2000", []int{2}},
		{"asn__lte unchanged", "asn__lte=100", []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids := phase69FetchIDs(t, srv.URL+"/api/net?"+tc.query)
			if !sameIDs(ids, tc.want) {
				t.Errorf("query %q: got ids %v, want %v", tc.query, ids, tc.want)
			}
		})
	}
}

// TestPhase68StatusMatrix_Phase69Layering — regression guard: the Phase 68
// status × since matrix and the Phase 69 filter layer compose correctly.
// ?status=deleted&name__contains=foo — status is silently dropped (Phase 68
// removed `status` from Fields), and the contains layer still resolves
// through the folded column (or falls through to FieldContainsFold for
// non-folded fields).
func TestPhase68StatusMatrix_Phase69Layering(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().UTC()

	// Seed one ok row (default: visible in list w/o since) and one deleted.
	_, err := client.Network.Create().SetID(1).SetName("Foo Ok").SetNameFold(unifold.Fold("Foo Ok")).
		SetAsn(1).SetStatus("ok").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err = client.Network.Create().SetID(2).SetName("Foo Dead").SetNameFold(unifold.Fold("Foo Dead")).
		SetAsn(2).SetStatus("deleted").SetCreated(now).SetUpdated(now).Save(ctx)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	srv := httptest.NewServer(newPhase69Mux(client))
	t.Cleanup(srv.Close)

	// ?status=deleted is silently dropped by Fields map (Phase 68). Only
	// status=ok rows are returned by the status matrix. The name__contains
	// is coerced/folded and matches both Foo rows.
	ids := phase69FetchIDs(t, srv.URL+"/api/net?status=deleted&name__contains=foo")
	if !sameIDs(ids, []int{1}) {
		t.Errorf("status+name__contains: got ids %v, want [1] — status=deleted dropped, name filter returns ok row only", ids)
	}
}

// sameIDs reports whether two int slices contain the same elements (order
// independent). Uses a small map; adequate for test-sized inputs.
func sameIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, c := range seen {
		if c != 0 {
			return false
		}
	}
	return true
}

