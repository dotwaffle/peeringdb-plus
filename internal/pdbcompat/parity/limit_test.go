package parity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Limit locks the v1.16 limit semantics:
//
//   - ?limit=0 returns ALL rows unbounded (matches upstream
//     2.83.0 rest.py:515-518 + :757-760, NOT count-only as some
//     clients incorrectly assume).
//   - ?limit=0 paired with the response budget returns 413 when the
//     precount × TypicalRowBytes exceeds the budget.
//   - ?depth=N above 0 on a list that upstream answers from its live
//     query (a filter key, since other than 0, or ?q=) is cut to 250
//     rows with meta.truncated. A list that upstream serves from its
//     API cache is not cut.
//   - limit, skip and since are parsed as upstream parses them: the
//     last value, Python int() rules, the upstream error texts, and an
//     empty value is a 400. A negative limit serves every row. A
//     negative skip is a 400; DIVERGENCE on a list that upstream
//     serves from its API cache file.
//
// upstream: 2.83.0 peeringdb_server/rest.py:515-518, :757-760 (limit=0 =
// unlimited)
// upstream: 2.83.0 peeringdb_server/pagination.py:21-40
// (UnlimitedIfNoPagePagination; page_size_query_param at :23)
func TestParity_Limit(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("bare_url_and_zero_both_return_all_rows", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:516 (limit defaults to 0) + :760
		// (limit=0 → qset[skip:], no slice). Both bare URL and explicit
		// ?limit=0 return ALL rows from the filtered queryset on
		// upstream — the 250 defaultable page size is opt-in via
		// ?page=N (UnlimitedIfNoPagePagination at pagination.py:21-40
		// only applies pagination when "page" is in query_params).
		//
		// Earlier revisions of this test asserted the bare URL returns
		// 250 rows, which was a parity bug that this fork inherited
		// from a defensive cap on response.go DefaultLimit. Verified
		// 2026-04-28 against upstream live data (parity-results.txt):
		// bare /api/org returned 33,556 rows upstream vs 250 on the
		// then-buggy mirror. DefaultLimit was changed from 250 to 0
		// to restore parity; the response-memory budget is
		// now the sole DoS gate, returning 413 when precount ×
		// TypicalRowBytes exceeds the budget.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		const seedN = 300 // > the historical 250-row default page
		for i := 1; i <= seedN; i++ {
			if _, err := c.Network.Create().
				SetID(i).SetName("LimitNet").SetNameFold(unifold.Fold("LimitNet")).
				SetAsn(60000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i, err)
			}
		}

		// budget=0 disables CheckBudget — exercises the pure
		// limit=0-returns-all path without the 413 layer interfering.
		srv := newTestServer(t, c)

		// Bare URL: returns all 300 rows (matches upstream).
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("bare URL status = %d; body=%s", status, string(body))
		}
		bare := decodeDataArray(t, body)
		if len(bare) != seedN {
			t.Errorf("bare /api/net: got %d rows, want %d "+
				"(upstream returns all rows when neither limit= nor page= is set)",
				len(bare), seedN)
		}

		// Explicit ?limit=0: also returns all 300 rows.
		status, body = httpGet(t, srv, "/api/net?limit=0")
		if status != http.StatusOK {
			t.Fatalf("limit=0 status = %d; body=%s", status, string(body))
		}
		all := decodeDataArray(t, body)
		if len(all) != seedN {
			t.Errorf("?limit=0: got %d rows, want %d (all unbounded)",
				len(all), seedN)
		}
	})

	t.Run("zero_over_budget_returns_413", func(t *testing.T) {
		t.Parallel()
		// pre-flight CheckBudget gate. With a tiny per-response budget
		// and a non-empty result, the count × TypicalRowBytes math
		// returns 413 before the .All() materialises anything. The body
		// has the upstream error form, with max_rows and budget_bytes
		// in meta.
		// synthesised: the budget mechanism is novel to this fork;
		// upstream has no equivalent gate.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed enough rows that even one TypicalRowBytes (~1600B for
		// net) exceeds the 100B budget — guarantees the gate fires.
		for i := 1; i <= 50; i++ {
			if _, err := c.Network.Create().
				SetID(i).SetName("OverBudget").SetNameFold(unifold.Fold("OverBudget")).
				SetAsn(70000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i, err)
			}
		}
		srv := newTestServerWithBudget(t, c, 100) // 100 bytes — tiny

		status, body := httpGet(t, srv, "/api/net?limit=0")
		if status != http.StatusRequestEntityTooLarge {
			t.Fatalf("budget breach: got %d, want 413; body=%s",
				status, string(body))
		}
		assertTopLevelKeys(t, body, "meta")
		m := mustDecodeMetaError(t, body)
		if m.Error == "" {
			t.Errorf("meta.error empty")
		}
		if m.BudgetBytes != 100 {
			t.Errorf("meta.budget_bytes = %d, want 100", m.BudgetBytes)
		}
		// max_rows is the integer divide; budget(100) / perRow(1600
		// for net depth 0) = 0, so the field MAY be 0; we only assert
		// it's set (>= 0 by definition of the integer divide).
		if m.MaxRows < 0 {
			t.Errorf("meta.max_rows = %d, want >= 0", m.MaxRows)
		}
	})

	t.Run("as_set_list_over_budget_413", func(t *testing.T) {
		t.Parallel()
		// synthesised: response memory budget. The as_set list bills 640
		// bytes for each entry and ignores limit and skip, so the 413
		// depends only on the number of entries.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		for i, asn := range []int{64500, 64501} {
			if _, err := c.Network.Create().
				SetID(i + 1).SetName("ASSetNet").SetNameFold(unifold.Fold("ASSetNet")).
				SetAsn(asn).SetIrrAsSet("AS-SET").SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i+1, err)
			}
		}

		srv := newTestServerWithBudget(t, c, 640)
		status, body := httpGet(t, srv, "/api/as_set?limit=1")
		if status != http.StatusRequestEntityTooLarge {
			t.Fatalf("budget 640: status = %d, want 413; body=%s", status, body)
		}
		m := mustDecodeMetaError(t, body)
		if m.MaxRows != 1 || m.BudgetBytes != 640 {
			t.Errorf("budget 640: meta max_rows = %d, budget_bytes = %d, want 1 and 640", m.MaxRows, m.BudgetBytes)
		}

		srv = newTestServerWithBudget(t, c, 1280)
		if status, body := httpGet(t, srv, "/api/as_set"); status != http.StatusOK {
			t.Errorf("budget 1280: status = %d, want 200; body=%s", status, body)
		}
	})

	t.Run("list_depth_filtered_truncates_at_250", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:757-772: a list at depth > 0 that
		// upstream answers from its live query is cut to 250 rows after
		// the skip/limit slice, and meta.truncated says so. A filter key
		// keeps the list off the API cache (api_cache.py:90-124).
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		ids, meta := getDepthList(t, srv, "/api/net?name=DepthProbe&depth=1")
		if len(ids) != 250 || ids[0] != 1 || ids[249] != 250 {
			t.Errorf("ids = %d rows [%v..], want ids 1..250", len(ids), ids[:min(len(ids), 3)])
		}
		assertTruncated(t, meta, "1")
	})

	t.Run("list_depth_exactly_250_not_truncated", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:766: the message is set only when the
		// sliced query holds more than 250 rows.
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 250)
		srv := newTestServer(t, c)
		ids, meta := getDepthList(t, srv, "/api/net?name=DepthProbe&depth=1")
		if len(ids) != 250 {
			t.Errorf("%d rows, want 250", len(ids))
		}
		assertNotTruncated(t, meta)
	})

	t.Run("list_depth_truncation_after_skip_limit", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:757-769: the skip/limit slice comes
		// first, then the count of the sliced query decides the cut.
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 300)
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			query     string
			rows      int
			first     int
			truncated bool
		}{
			{"limit=280&skip=10", 250, 11, true},
			{"skip=10", 250, 11, true},
			{"limit=100", 100, 1, false},
			{"skip=295", 5, 296, false},
		} {
			ids, meta := getDepthList(t, srv, "/api/net?name=DepthProbe&depth=1&"+tc.query)
			if len(ids) != tc.rows || ids[0] != tc.first {
				t.Errorf("?%s: %d rows from id %v, want %d rows from id %d", tc.query, len(ids), ids[:min(len(ids), 1)], tc.rows, tc.first)
			}
			if tc.truncated {
				assertTruncated(t, meta, "1")
			} else {
				assertNotTruncated(t, meta)
			}
		}
	})

	t.Run("list_depth_unfiltered_not_truncated", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 api_cache.py:90-124 (a list with no filter
		// and no since qualifies for the API cache at depth > 0),
		// pdb_api_cache.py:170 (the cache files hold every row) and
		// docs/api/op_list.md:112-114. The cache path never truncates.
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		for _, q := range []string{"depth=1", "depth=2", "depth=1&limit=0", "depth=1&limit=300"} {
			ids, meta := getDepthList(t, srv, "/api/net?"+q)
			if len(ids) != 260 {
				t.Errorf("?%s: %d rows, want 260", q, len(ids))
			}
			assertNotTruncated(t, meta)
		}
		ids, meta := getDepthList(t, srv, "/api/net?depth=1&limit=10&skip=5")
		if len(ids) != 10 || ids[0] != 6 {
			t.Errorf("?depth=1&limit=10&skip=5: ids = %v, want 10 rows from id 6", ids)
		}
		assertNotTruncated(t, meta)
	})

	t.Run("list_depth_ignored_keys_not_truncated", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 models.py:1259-1264 (org_flags) and
		// models.py:6095-6101 (the ix_side_set reverse relation) are
		// filters upstream, so upstream answers from its live query and
		// truncates. The mirror ignores these keys (registered rows
		// DIVERGENCE_unserialized_model_columns_silent_ignore and
		// DIVERGENCE_reverse_set_keys_silent_ignore), and an ignored key
		// does not make the list live, so the mirror serves the
		// unfiltered list in full, as it does at depth 0.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		orgs := make([]*ent.OrganizationCreate, 0, 260)
		facs := make([]*ent.FacilityCreate, 0, 260)
		for i := 1; i <= 260; i++ {
			orgs = append(orgs, c.Organization.Create().
				SetID(i).SetName(fmt.Sprintf("FlagOrg %d", i)).SetNameFold(unifold.Fold(fmt.Sprintf("FlagOrg %d", i))).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0))
			facs = append(facs, c.Facility.Create().
				SetID(i).SetName(fmt.Sprintf("SideFac %d", i)).SetNameFold(unifold.Fold(fmt.Sprintf("SideFac %d", i))).
				SetOrgID(1).SetStatus("ok").SetCreated(t0).SetUpdated(t0))
		}
		c.Organization.CreateBulk(orgs...).ExecX(ctx)
		c.Facility.CreateBulk(facs...).ExecX(ctx)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/org?org_flags=1&depth=1",
			"/api/fac?ix_side_set__asn=64500&depth=1",
		} {
			ids, meta := getDepthList(t, srv, path)
			if len(ids) != 260 {
				t.Errorf("GET %s: %d rows, want 260", path, len(ids))
			}
			assertNotTruncated(t, meta)
		}
	})

	t.Run("list_depth_since_truncates", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 api_cache.py:109-110 (since > 0 keeps the
		// list off the cache), rest.py:744 (updated, id order) and
		// :757-772 (the cut). The updated values run opposite to the ids.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		nets := make([]*ent.NetworkCreate, 0, 260)
		for i := 1; i <= 260; i++ {
			ts := t0.Add(time.Duration(261-i) * time.Second)
			nets = append(nets, c.Network.Create().
				SetID(i).SetName("SinceDepth").SetNameFold(unifold.Fold("SinceDepth")).
				SetAsn(90000+i).SetStatus("ok").SetCreated(ts).SetUpdated(ts))
		}
		c.Network.CreateBulk(nets...).ExecX(ctx)
		srv := newTestServer(t, c)
		ids, meta := getDepthList(t, srv, "/api/net?since=1&depth=1")
		if len(ids) != 250 || ids[0] != 260 || ids[249] != 11 {
			t.Errorf("ids = %d rows, first %v, want ids 260 down to 11", len(ids), ids[:min(len(ids), 3)])
		}
		assertTruncated(t, meta, "1")
	})

	t.Run("list_depth_negative_since_truncates", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 api_cache.py:80 and :109-110 (any since other
		// than 0 keeps the list off the cache) and rest.py:719 (the since
		// matrix applies only for since > 0, so a negative since lists
		// the live rows in id order).
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		ids, meta := getDepthList(t, srv, "/api/net?since=-1&depth=1")
		if len(ids) != 250 || ids[0] != 1 {
			t.Errorf("?since=-1: %d rows from %v, want 250 from id 1", len(ids), ids[:min(len(ids), 1)])
		}
		assertTruncated(t, meta, "1")
		ids, meta = getDepthList(t, srv, "/api/net?since=0&depth=1")
		if len(ids) != 260 {
			t.Errorf("?since=0: %d rows, want 260", len(ids))
		}
		assertNotTruncated(t, meta)
	})

	t.Run("list_depth_noop_filter_keys_truncate", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 serializers.py:3775-3810 (net info_type keys
		// set query_adjusted) and :614-654 (a prepare_query relation key
		// is a filter): both keep the list off the cache
		// (api_cache.py:100-110) even when they match every row.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		mustOrg(ctx, t, c, 1, "NoopOrg", t0)
		facs := make([]*ent.FacilityCreate, 0, 260)
		for i := 1; i <= 260; i++ {
			facs = append(facs, c.Facility.Create().
				SetID(i).SetName(fmt.Sprintf("NoopFac %d", i)).SetNameFold(unifold.Fold(fmt.Sprintf("NoopFac %d", i))).
				SetOrgID(1).SetStatus("ok").SetCreated(t0).SetUpdated(t0))
		}
		c.Facility.CreateBulk(facs...).ExecX(ctx)
		srv := newTestServer(t, c)
		for _, path := range []string{
			"/api/net?info_type__in=,x&depth=1",
			"/api/fac?org_name__iexact=x&depth=1",
		} {
			ids, meta := getDepthList(t, srv, path)
			if len(ids) != 250 {
				t.Errorf("GET %s: %d rows, want 250", path, len(ids))
			}
			assertTruncated(t, meta, "1")
		}
	})

	t.Run("list_depth_q_truncates", func(t *testing.T) {
		t.Parallel()
		// synthesised: ?q= is a mirror extension; the mirror counts it
		// as a filter, so a ?q= list at depth > 0 is cut at 250 rows.
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "QProbe", 260)
		srv := newTestServer(t, c)
		ids, meta := getDepthList(t, srv, "/api/net?q=QProbe&depth=1")
		if len(ids) != 250 {
			t.Errorf("%d rows, want 250", len(ids))
		}
		assertTruncated(t, meta, "1")
	})

	t.Run("list_depth_raw_value_in_message", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:520-523 (depth = int(...)) and
		// :771 (the message prints that int, not the clamped depth).
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		for _, tc := range []struct{ value, text string }{
			{"7", "7"},
			{"007", "7"},
			{"99999999999999999999", "99999999999999999999"},
		} {
			ids, meta := getDepthList(t, srv, "/api/net?name=DepthProbe&depth="+tc.value)
			if len(ids) != 250 {
				t.Errorf("depth=%s: %d rows, want 250", tc.value, len(ids))
			}
			assertTruncated(t, meta, tc.text)
		}
	})

	t.Run("list_depth_negative_is_depth0", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:766 (the cut needs depth > 0) and
		// serializers.py:1032-1039 (a depth of 0 or less renders flat
		// rows).
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?name=DepthProbe&depth=-1")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, headBody(body, 300))
		}
		rows := decodeDataArray(t, body)
		if len(rows) != 260 {
			t.Errorf("%d rows, want 260", len(rows))
		}
		if _, ok := rows[0]["netixlan_set"]; ok {
			t.Errorf("depth=-1 row has netixlan_set: %v", rows[0])
		}
		assertNotTruncated(t, decodeListMeta(t, body))
	})

	t.Run("list_depth_non_integer_400", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:520-523: int() of the value, and the
		// view returns "'depth' needs to be a number" for a ValueError.
		// CPython int() refuses a decimal string of more than 4300
		// digits (sys.int_info.default_max_str_digits).
		c := testutil.SetupClient(t)
		seedDepthNets(t, c, t0, "DepthProbe", 260)
		srv := newTestServer(t, c)
		const want = "'depth' needs to be a number"
		for _, v := range []string{"abc", "", "1.5", "0x1", strings.Repeat("1", 4301)} {
			status, body := httpGet(t, srv, "/api/net?name=DepthProbe&depth="+v)
			if status != http.StatusBadRequest {
				t.Errorf("depth=%.10s: status = %d, want 400", v, status)
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != want {
				t.Errorf("depth=%.10s: meta.error = %q, want %q", v, got, want)
			}
		}
		ids, meta := getDepthList(t, srv, "/api/net?name=DepthProbe&depth="+strings.Repeat("1", 4300))
		if len(ids) != 250 {
			t.Errorf("4300 digits: %d rows, want 250", len(ids))
		}
		assertTruncated(t, meta, strings.Repeat("1", 4300))
	})

	t.Run("list_depth_python_int_forms", func(t *testing.T) {
		t.Parallel()
		// synthesised: upstream parses depth with Python int() (2.83.0
		// rest.py:520-523), which accepts white space at both ends, a
		// sign and single underscores between digits; QueryDict.get
		// returns the last value. A "+" in a query string decodes to a
		// space, so the sign is sent as %2B.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "IntFormOrg", t0)
		mustNet(ctx, t, c, 1, "IntFormNet", 64501, 1, t0)
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			value     string
			wantDepth int
		}{
			{"%202%20", 2},
			{"%2B2", 2},
			{"+2", 2},
			{"0_2", 2},
			{"0&depth=1", 1},
		} {
			status, body := httpGet(t, srv, "/api/org?id=1&depth="+tc.value)
			if status != http.StatusOK {
				t.Errorf("depth=%s: status = %d, want 200; body=%s", tc.value, status, headBody(body, 300))
				continue
			}
			set, _ := decodeDataArray(t, body)[0]["net_set"].([]any)
			if len(set) != 1 {
				t.Errorf("depth=%s: net_set = %v, want one element", tc.value, set)
				continue
			}
			_, isObject := set[0].(map[string]any)
			if isObject != (tc.wantDepth == 2) {
				t.Errorf("depth=%s: net_set[0] = %v, want the depth %d shape", tc.value, set[0], tc.wantDepth)
			}
		}
	})

	t.Run("list_depth_error_order", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:505-523 parses since, skip, limit and
		// depth before the filter loop (:564-683). The mirror checks
		// skip and limit before since; each pair below is still a 400
		// with the upstream text.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		for _, tc := range []struct{ query, want string }{
			{"since=abc&depth=abc", "'since' needs to be a unix timestamp (epoch seconds)"},
			{"depth=abc&skip=abc", "'skip' needs to be a number"},
			{"depth=abc&asn__lt=x", "'depth' needs to be a number"},
			// since is parsed before the filter loop and before the
			// negative-skip check (Django raises that one at the slice,
			// rest.py:757-760).
			{"since=abc&asn__lt=x", "'since' needs to be a unix timestamp (epoch seconds)"},
			{"since=abc&skip=-1", "'since' needs to be a unix timestamp (epoch seconds)"},
		} {
			status, body := httpGet(t, srv, "/api/net?"+tc.query)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", tc.query, status, headBody(body, 300))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != tc.want {
				t.Errorf("?%s: meta.error = %q, want %q", tc.query, got, tc.want)
			}
		}
	})

	t.Run("list_depth_leaf_types_unchanged", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 pdb_api_cache.py:50-58 and
		// serializers.py:1286-1290 (list_exclude): a list row never
		// carries its forward FK object, so the types without reverse
		// sets render the depth-0 row at every depth.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "LeafOrg", t0)
		mustFac(ctx, t, c, 1, "LeafFac", 1, t0)
		mustNet(ctx, t, c, 1, "LeafNet", 64501, 1, t0)
		mustIX(ctx, t, c, 1, "LeafIX", 1, t0)
		mustIxLan(ctx, t, c, 1, "LeafLan", 1, t0)
		mustIxPfx(ctx, t, c, 1, "192.0.2.0/24", 1, t0)
		c.Carrier.Create().SetID(1).SetName("LeafCarrier").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.CarrierFacility.Create().SetID(1).SetCarrierID(1).SetFacID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.NetworkFacility.Create().SetID(1).SetNetID(1).SetFacID(1).SetLocalAsn(64501).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.IxFacility.Create().SetID(1).SetIxID(1).SetFacID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		c.Poc.Create().SetID(1).SetNetID(1).SetRole("NOC").SetName("Leaf NOC").SetVisible("Public").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		nix := make([]*ent.NetworkIxLanCreate, 0, 260)
		for i := 1; i <= 260; i++ {
			nix = append(nix, c.NetworkIxLan.Create().
				SetID(i).SetNetID(1).SetIxlanID(1).SetIxID(1).SetName("LeafIX").
				SetAsn(64501).SetSpeed(1000).SetOperational(true).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0))
		}
		c.NetworkIxLan.CreateBulk(nix...).ExecX(ctx)
		srv := newTestServer(t, c)
		for _, typ := range []string{"netixlan", "netfac", "ixfac", "poc", "ixpfx", "carrierfac", "fac"} {
			_, flat := httpGet(t, srv, "/api/"+typ+"?id=1")
			for _, d := range []string{"1", "2"} {
				status, body := httpGet(t, srv, "/api/"+typ+"?id=1&depth="+d)
				if status != http.StatusOK {
					t.Errorf("%s depth=%s: status = %d; body=%s", typ, d, status, headBody(body, 300))
					continue
				}
				if !bytes.Equal(body, flat) {
					t.Errorf("%s depth=%s row differs from depth 0:\n  got:  %s\n  want: %s", typ, d, headBody(body, 400), headBody(flat, 400))
				}
			}
		}
		ids, meta := getDepthList(t, srv, "/api/netixlan?asn=64501&depth=1")
		if len(ids) != 250 {
			t.Errorf("netixlan: %d rows, want 250", len(ids))
		}
		assertTruncated(t, meta, "1")
	})

	t.Run("error_envelope_meta_error", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 renderers.py:123-148 (4xx: meta.error, no data key),
		// rest.py:809-815 (unique 404 adds "data": []),
		// DRF views.py:167-172 + exceptions.py:194-196 (405 text for a method
		// the route does not map)
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)

		// Errors with no data key: a 400 from a malformed ?limit= and
		// ?since=, and a 404 from a missing PK.
		for _, path := range []string{
			"/api/net?limit=abc",
			"/api/net?since=abc",
			"/api/net/999999",
		} {
			wantStatus := http.StatusBadRequest
			if path == "/api/net/999999" {
				wantStatus = http.StatusNotFound
			}
			status, hdr, body := httpDo(t, srv, http.MethodGet, path, nil)
			if status != wantStatus {
				t.Errorf("GET %s: status = %d, want %d; body=%s", path, status, wantStatus, string(body))
				continue
			}
			assertErrorHeaders(t, "GET "+path, hdr)
			assertTopLevelKeys(t, body, "meta")
			assertMetaKeys(t, body, "error")
			if got := mustDecodeMetaError(t, body).Error; got == "" {
				t.Errorf("GET %s: meta.error is empty", path)
			}
		}

		// The unique-query 404 is the one error body with "data": [].
		for _, path := range []string{"/api/net?id=999999", "/api/net?asn=999999"} {
			status, hdr, body := httpDo(t, srv, http.MethodGet, path, nil)
			if status != http.StatusNotFound {
				t.Errorf("GET %s: status = %d, want 404; body=%s", path, status, string(body))
				continue
			}
			assertErrorHeaders(t, "GET "+path, hdr)
			assertTopLevelKeys(t, body, "data", "meta")
			var env struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("GET %s: decode: %v", path, err)
			}
			if string(env.Data) != "[]" {
				t.Errorf("GET %s: data = %s, want []", path, string(env.Data))
			}
			if got := mustDecodeMetaError(t, body).Error; got != "Entity not found" {
				t.Errorf("GET %s: meta.error = %q, want %q", path, got, "Entity not found")
			}
		}

		// A method that the upstream route does not map: the text and
		// status match upstream. Allow differs (row C), so it is not
		// asserted here.
		for _, tc := range []struct{ method, path string }{
			{http.MethodPut, "/api/net"},
			{http.MethodDelete, "/api/net"},
			{http.MethodPost, "/api/net/1"},
		} {
			status, hdr, body := httpDo(t, srv, tc.method, tc.path, nil)
			if status != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: status = %d, want 405; body=%s", tc.method, tc.path, status, string(body))
				continue
			}
			assertErrorHeaders(t, tc.method+" "+tc.path, hdr)
			assertTopLevelKeys(t, body, "meta")
			want := "Method \"" + tc.method + "\" not allowed."
			if got := mustDecodeMetaError(t, body).Error; got != want {
				t.Errorf("%s %s: meta.error = %q, want %q", tc.method, tc.path, got, want)
			}
		}
	})

	t.Run("DIVERGENCE_problem_json_when_accept_names_it", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream has one renderer (MetaJSONRenderer,
		// 2.83.0 mainsite/settings/__init__.py:1459), so DRF content
		// negotiation answers an Accept header that no range of
		// application/json satisfies with 406 (DRF negotiation.py:78,
		// views.py:410-411). The mirror sends RFC 9457 errors when
		// Accept names application/problem+json, and serves a success
		// response as normal JSON. See docs/API.md § Known Divergences.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		for i := 1; i <= 50; i++ {
			if _, err := c.Network.Create().
				SetID(i).SetName("ProblemOptIn").SetNameFold(unifold.Fold("ProblemOptIn")).
				SetAsn(90000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i, err)
			}
		}
		srv := newTestServer(t, c)
		budgetSrv := newTestServerWithBudget(t, c, 100)
		problemAccept := http.Header{"Accept": {"application/problem+json"}}

		for _, tc := range []struct {
			name       string
			method     string
			path       string
			wantStatus int
			wantDetail string
		}{
			{"bad_limit", http.MethodGet, "/api/net?limit=abc", http.StatusBadRequest, ""},
			{"unique_miss", http.MethodGet, "/api/net?id=999999", http.StatusNotFound, "Entity not found"},
			{"method", http.MethodPut, "/api/net", http.StatusMethodNotAllowed, `Method "PUT" not allowed.`},
		} {
			status, hdr, body := httpDo(t, srv, tc.method, tc.path, problemAccept)
			if status != tc.wantStatus {
				t.Errorf("%s: status = %d, want %d; body=%s", tc.name, status, tc.wantStatus, string(body))
				continue
			}
			if ct := hdr.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("%s: Content-Type = %q, want application/problem+json", tc.name, ct)
			}
			p := mustDecodeProblem(t, body)
			if p.Type != "about:blank" || p.Status != tc.wantStatus {
				t.Errorf("%s: problem type/status = %q/%d, want about:blank/%d", tc.name, p.Type, p.Status, tc.wantStatus)
			}
			if tc.wantDetail != "" && p.Detail != tc.wantDetail {
				t.Errorf("%s: problem.detail = %q, want %q", tc.name, p.Detail, tc.wantDetail)
			}
		}

		status, _, body := httpDo(t, budgetSrv, http.MethodGet, "/api/net?limit=0", problemAccept)
		if status != http.StatusRequestEntityTooLarge {
			t.Fatalf("budget: status = %d, want 413; body=%s", status, string(body))
		}
		if p := mustDecodeProblem(t, body); p.Type != pdbcompat.ResponseTooLargeType || p.BudgetBytes != 100 {
			t.Errorf("budget: problem type/budget_bytes = %q/%d, want %q/100", p.Type, p.BudgetBytes, pdbcompat.ResponseTooLargeType)
		}

		// Accept values that keep the upstream form.
		for _, accept := range []string{
			"application/problem+json;q=0",
			"*/*",
			"application/*",
			"application/json",
			"application/problem+json;q=abc",
			"application/problem+json;q=NaN",
		} {
			status, _, body := httpDo(t, srv, http.MethodGet, "/api/net?limit=abc", http.Header{"Accept": {accept}})
			if status != http.StatusBadRequest {
				t.Errorf("Accept %q: status = %d, want 400", accept, status)
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got == "" {
				t.Errorf("Accept %q: meta.error is empty", accept)
			}
		}

		// Type and parameter names are not case-sensitive.
		_, hdr, body := httpDo(t, srv, http.MethodGet, "/api/net?limit=abc",
			http.Header{"Accept": {"Application/Problem+JSON; q=0.5"}})
		if ct := hdr.Get("Content-Type"); ct != "application/problem+json" {
			t.Errorf("mixed-case Accept: Content-Type = %q, want application/problem+json; body=%s", ct, string(body))
		}

		// A success response ignores Accept: upstream answers 406.
		status, hdr, body = httpDo(t, srv, http.MethodGet, "/api/net", problemAccept)
		if status != http.StatusOK {
			t.Fatalf("success: status = %d, want 200; body=%s", status, string(body))
		}
		if ct := hdr.Get("Content-Type"); ct != "application/json" {
			t.Errorf("success: Content-Type = %q, want application/json", ct)
		}
		if ids := extractIDs(t, body); len(ids) != 50 {
			t.Errorf("success: %d rows, want 50", len(ids))
		}
	})

	t.Run("limit_above_1000_honoured_uncapped", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:757-758 — qset[skip:skip+limit] with NO
		// upper cap. An earlier revision clamped explicit limit to
		// 1000, silently truncating each page for clients paginating
		// with larger windows (rows past the clamp were permanently
		// skipped). The response-memory budget is the cost bound, not
		// a hidden clamp.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		const seedN = 1100
		for i := 1; i <= seedN; i++ {
			if _, err := c.Network.Create().
				SetID(i).SetName("UncappedLimit").SetNameFold(unifold.Fold("UncappedLimit")).
				SetAsn(100000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i, err)
			}
		}
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?limit=5000")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		rows := decodeDataArray(t, body)
		if len(rows) != seedN {
			t.Errorf("limit=5000 over %d rows: got %d, want all %d (no hidden clamp)",
				seedN, len(rows), seedN)
		}
	})

	t.Run("non_numeric_limit_and_skip_return_400", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:511-518 raises RestValidationError
		// ("'limit' needs to be a number") for a limit or skip that
		// int() does not parse. An empty value fails too: QueryDict.get
		// returns "", and int("") raises. skip is checked before limit.
		// Silently ignoring a typo'd limit turned a bounded page
		// request into a full-table dump.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		const (
			limitText = "'limit' needs to be a number"
			skipText  = "'skip' needs to be a number"
		)
		for _, tc := range []struct{ query, want string }{
			{"limit=abc", limitText},
			{"limit=", limitText},
			{"limit=1.5", limitText},
			{"skip=abc", skipText},
			{"skip=", skipText},
			{"skip=abc&limit=abc", skipText},
			{"limit=abc&skip=abc", skipText},
		} {
			status, body := httpGet(t, srv, "/api/net?"+tc.query)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", tc.query, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != tc.want {
				t.Errorf("?%s: meta.error = %q, want %q", tc.query, got, tc.want)
			}
		}
	})

	t.Run("negative_limit_serves_all_rows", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:515-518 parses a negative limit, and
		// :757-760 slices only when limit > 0, so ?limit=-5 serves
		// qset[skip:], every row after skip.
		c := testutil.SetupClient(t)
		seedLimitNets(t, c, t0, 3)
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			query string
			want  []int
		}{
			{"limit=-5", []int{1, 2, 3}},
			{"limit=-1&skip=1", []int{2, 3}},
		} {
			status, body := httpGet(t, srv, "/api/net?"+tc.query)
			if status != http.StatusOK {
				t.Errorf("?%s: status = %d, want 200; body=%s", tc.query, status, string(body))
				continue
			}
			if got := extractIDs(t, body); !equalIntSlice(got, tc.want) {
				t.Errorf("?%s: ids = %v, want %v", tc.query, got, tc.want)
			}
		}
	})

	t.Run("negative_skip_on_filtered_list_returns_400", func(t *testing.T) {
		t.Parallel()
		// upstream: Django db/models/query.py:403-417 raises ValueError
		// for a negative slice (via 2.83.0 rest.py:757-760), and list()
		// returns it as a 400 (:824-827). A list with a filter key, or
		// with a limit of 1 to 250, does not qualify for the API cache
		// (api_cache.py:100-110), so upstream takes this path. The error
		// comes before the serializer, so the unique-query 404 does not
		// fire.
		c := testutil.SetupClient(t)
		seedLimitNets(t, c, t0, 2)
		srv := newTestServer(t, c)
		for _, q := range []string{
			"name=x&skip=-1",
			"skip=-1&limit=10",
			"id=1&skip=-1",
			"id=999&skip=-1",
		} {
			status, body := httpGet(t, srv, "/api/net?"+q)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", q, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != "Negative indexing is not supported." {
				t.Errorf("?%s: meta.error = %q, want %q", q, got, "Negative indexing is not supported.")
			}
		}
		// A filter error wins over the negative skip: upstream builds the
		// filters before it slices.
		status, body := httpGet(t, srv, "/api/net?asn__lt=abc&skip=-1")
		if status != http.StatusBadRequest {
			t.Fatalf("?asn__lt=abc&skip=-1: status = %d, want 400; body=%s", status, string(body))
		}
		if got := mustDecodeMetaError(t, body).Error; !strings.Contains(got, "asn__lt") {
			t.Errorf("?asn__lt=abc&skip=-1: meta.error = %q, want the filter error", got)
		}
	})

	t.Run("DIVERGENCE_negative_skip_on_cacheable_list_returns_400", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream serves a list with no filter key, no
		// since and a limit of 0 or more than 250 from its API cache
		// file (2.83.0 api_cache.py:90-124, settings/__init__.py:399
		// API_CACHE_ENABLED) and slices the rows with Python slices
		// (api_cache.py:136-142), so ?skip=-2 returns the last 2 rows.
		// At depth > 0 every limit qualifies (api_cache.py:100-110), so
		// ?depth=1&skip=-2 also returns the last 2 rows and
		// ?depth=1&limit=-1 returns data[:-1], every row but the last.
		// The mirror returns 400 for every negative skip, as the
		// upstream DB path does, and serves every row for a negative
		// limit. See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		seedLimitNets(t, c, t0, 3)
		srv := newTestServer(t, c)
		ids, _ := getDepthList(t, srv, "/api/net?depth=1&limit=-1")
		if !equalIntSlice(ids, []int{1, 2, 3}) {
			t.Errorf("?depth=1&limit=-1: ids = %v, want [1 2 3] (divergence canary)", ids)
		}
		for _, q := range []string{"skip=-2", "skip=-2&limit=300", "skip=-2&limit=0", "depth=1&skip=-2", "depth=1&skip=-2&limit=10"} {
			status, body := httpGet(t, srv, "/api/net?"+q)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400 (divergence canary); body=%s", q, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != "Negative indexing is not supported." {
				t.Errorf("?%s: meta.error = %q, want %q", q, got, "Negative indexing is not supported.")
			}
		}
	})

	t.Run("limit_python_int_forms", func(t *testing.T) {
		t.Parallel()
		// synthesised: upstream parses limit and skip with Python int()
		// (2.83.0 rest.py:511-518), which accepts white space at both
		// ends, a sign, single underscores between digits and Unicode
		// decimal digits. A "+" in a query string decodes to a space,
		// so the sign is sent as %2B.
		c := testutil.SetupClient(t)
		seedLimitNets(t, c, t0, 12)
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			query string
			want  int
		}{
			{"limit=%202", 2},
			{"limit=%2B2", 2},
			{"limit=1_0", 10},
			{"limit=%EF%BC%92", 2}, // fullwidth digit two
			{"limit=02", 2},
			{"limit=99999999999999999999", 12},
			{"limit=2&skip=%2010", 2},
		} {
			status, body := httpGet(t, srv, "/api/net?"+tc.query)
			if status != http.StatusOK {
				t.Errorf("?%s: status = %d, want 200; body=%s", tc.query, status, string(body))
				continue
			}
			if got := len(extractIDs(t, body)); got != tc.want {
				t.Errorf("?%s: %d rows, want %d", tc.query, got, tc.want)
			}
		}
	})

	t.Run("limit_last_value_wins", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:511-518 reads the value with
		// QueryDict.get, which returns the last value of a repeated key.
		c := testutil.SetupClient(t)
		seedLimitNets(t, c, t0, 3)
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?limit=1&limit=2")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		if got := extractIDs(t, body); !equalIntSlice(got, []int{1, 2}) {
			t.Errorf("?limit=1&limit=2: ids = %v, want [1 2]", got)
		}
	})

	t.Run("empty_since_returns_400", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:505-510: int(float(since)) raises for
		// an empty or non-numeric value, and the view returns this text.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		const want = "'since' needs to be a unix timestamp (epoch seconds)"
		for _, q := range []string{"since=", "since=abc", "since=1&since="} {
			status, body := httpGet(t, srv, "/api/net?"+q)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", q, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != want {
				t.Errorf("?%s: meta.error = %q, want %q", q, got, want)
			}
		}
	})

	t.Run("since_last_value_wins", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:505 reads since with QueryDict.get
		// (the last value) and parses it with int(); :736-744 then keeps
		// the rows updated after it.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		t1 := t0.Add(time.Hour)
		t2 := t0.Add(2 * time.Hour)
		for i, ts := range []time.Time{t1, t2} {
			id := i + 1
			if _, err := c.Network.Create().
				SetID(id).SetName("SinceNet").SetNameFold(unifold.Fold("SinceNet")).
				SetAsn(70000 + id).SetStatus("ok").
				SetCreated(ts).SetUpdated(ts).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", id, err)
			}
		}
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			query string
			want  []int
		}{
			{fmt.Sprintf("since=%d&since=%d", t1.Unix(), t2.Unix()), []int{2}},
			{fmt.Sprintf("since=%d&since=%d", t2.Unix(), t1.Unix()), []int{1, 2}},
			{fmt.Sprintf("since=%%20%d_0", t2.Unix()/10), []int{2}},
		} {
			status, body := httpGet(t, srv, "/api/net?"+tc.query)
			if status != http.StatusOK {
				t.Errorf("?%s: status = %d, want 200; body=%s", tc.query, status, string(body))
				continue
			}
			if got := extractIDs(t, body); !equalIntSlice(got, tc.want) {
				t.Errorf("?%s: ids = %v, want %v", tc.query, got, tc.want)
			}
		}
	})

	t.Run("explicit_limit_200_honoured", func(t *testing.T) {
		t.Parallel()
		// Control: explicit limit < DefaultLimit is honoured exactly.
		// upstream: pdb_api_test.py (explicit limit=N is the most
		// common pagination shape across the corpus).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		for i := 1; i <= 250; i++ {
			if _, err := c.Network.Create().
				SetID(i).SetName("ExplicitLimit").SetNameFold(unifold.Fold("ExplicitLimit")).
				SetAsn(90000 + i).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", i, err)
			}
		}
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?limit=200")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		rows := decodeDataArray(t, body)
		if len(rows) != 200 {
			t.Errorf("explicit limit=200: got %d, want 200", len(rows))
		}
	})
}

// seedLimitNets seeds n ok networks with ids 1..n.
func seedLimitNets(t *testing.T, c *ent.Client, ts time.Time, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		if _, err := c.Network.Create().
			SetID(i).SetName("LimitParseNet").SetNameFold(unifold.Fold("LimitParseNet")).
			SetAsn(80000 + i).SetStatus("ok").
			SetCreated(ts).SetUpdated(ts).
			Save(t.Context()); err != nil {
			t.Fatalf("seed net %d: %v", i, err)
		}
	}
}

// assertErrorHeaders checks the headers of an upstream-form /api/ error:
// Content-Type application/json and Vary with the Accept token.
func assertErrorHeaders(t *testing.T, label string, hdr http.Header) {
	t.Helper()
	if ct := hdr.Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s: Content-Type = %q, want application/json", label, ct)
	}
	if !headerHasToken(hdr, "Vary", "Accept") {
		t.Errorf("%s: Vary = %q, want the Accept token", label, hdr.Values("Vary"))
	}
}

// headerHasToken reports whether one of the comma-separated values of
// header key is token (case-insensitive). A substring match is not
// enough: Accept-Encoding contains Accept.
func headerHasToken(hdr http.Header, key, token string) bool {
	for _, v := range hdr.Values(key) {
		for part := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// assertTopLevelKeys checks that the JSON object in body has exactly
// the given top-level keys.
func assertTopLevelKeys(t *testing.T, body []byte, want ...string) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, string(body))
	}
	if got := slices.Sorted(maps.Keys(top)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("top-level keys = %v, want %v; body=%s", got, want, string(body))
	}
}

// assertMetaKeys checks that the meta object in body has exactly the
// given keys.
func assertMetaKeys(t *testing.T, body []byte, want ...string) {
	t.Helper()
	var top struct {
		Meta map[string]json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, string(body))
	}
	if got := slices.Sorted(maps.Keys(top.Meta)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
		t.Errorf("meta keys = %v, want %v; body=%s", got, want, string(body))
	}
}

// seedDepthNets seeds n ok nets with ids 1..n and the same name, with
// no org, created and updated at ts.
func seedDepthNets(t *testing.T, c *ent.Client, ts time.Time, name string, n int) {
	t.Helper()
	nets := make([]*ent.NetworkCreate, 0, n)
	for i := 1; i <= n; i++ {
		nets = append(nets, c.Network.Create().
			SetID(i).SetName(name).SetNameFold(unifold.Fold(name)).
			SetAsn(90000+i).SetStatus("ok").
			SetCreated(ts).SetUpdated(ts))
	}
	if err := c.Network.CreateBulk(nets...).Exec(t.Context()); err != nil {
		t.Fatalf("seed %d nets: %v", n, err)
	}
}

// getDepthList GETs a list that must return 200 and returns its row ids
// and its meta object.
func getDepthList(t *testing.T, srv *httptest.Server, path string) ([]int, map[string]any) {
	t.Helper()
	status, body := httpGet(t, srv, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s: status = %d, want 200; body=%s", path, status, headBody(body, 300))
	}
	return extractIDs(t, body), decodeListMeta(t, body)
}

// decodeListMeta returns the meta object of a list response.
func decodeListMeta(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode meta: %v", err)
	}
	return env.Meta
}

// assertTruncated checks meta.truncated against the upstream text
// (2.83.0 rest.py:771) with depthText as the printed depth.
func assertTruncated(t *testing.T, meta map[string]any, depthText string) {
	t.Helper()
	want := "Your search query (with depth " + depthText + ") returned more than 250 rows and has been truncated. Please be more specific in your filters, use the limit and skip parameters to page through the resultset or drop the depth parameter"
	if got, _ := meta["truncated"].(string); got != want {
		t.Errorf("meta.truncated = %.120q, want %.120q", got, want)
	}
}

// assertNotTruncated checks that meta has no truncated key.
func assertNotTruncated(t *testing.T, meta map[string]any) {
	t.Helper()
	if v, ok := meta["truncated"]; ok {
		t.Errorf("meta.truncated = %.80v, want no key", v)
	}
}
