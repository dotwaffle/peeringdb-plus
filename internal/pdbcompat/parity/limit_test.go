package parity

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

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
//   - ?depth=N on a list endpoint is silently dropped by the
//     guardrail. DIVERGENCE from upstream which accepts depth on
//     list. See docs/API.md § Known Divergences.
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

	t.Run("depth_on_list_silently_dropped_DIVERGENCE", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream rest.py accepts ?depth on list
		// endpoints and embeds related objects per row. The list
		// guardrail (handler.go:163-168) silently drops the param to
		// avoid memory blow-up at scale; until a safe list+depth
		// implementation lands the response is IDENTICAL to a
		// no-depth call.
		// See docs/API.md § Known Divergences.
		// synthesised: the silent-drop is novel to this fork.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		for _, id := range []int{1, 2} {
			if _, err := c.Network.Create().
				SetID(id).SetName("DepthProbe").SetNameFold(unifold.Fold("DepthProbe")).
				SetAsn(80000 + id).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0.Add(time.Duration(id) * time.Hour)).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", id, err)
			}
		}
		srv := newTestServer(t, c)

		statusPlain, bodyPlain := httpGet(t, srv, "/api/net")
		statusDepth, bodyDepth := httpGet(t, srv, "/api/net?depth=2")
		if statusPlain != http.StatusOK || statusDepth != http.StatusOK {
			t.Fatalf("plain=%d depth=%d (both must be 200)", statusPlain, statusDepth)
		}
		idsPlain := extractIDs(t, bodyPlain)
		idsDepth := extractIDs(t, bodyDepth)
		if !equalIntSlice(idsPlain, idsDepth) {
			t.Errorf("DIVERGENCE: ?depth=2 must produce identical id list as no-depth (silent-drop). got plain=%v depth=%v",
				idsPlain, idsDepth)
		}
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
		// ("'limit' needs to be a number") for non-numeric limit/skip.
		// Silently ignoring a typo'd limit turned a bounded page
		// request into a full-table dump. A negative skip fails too:
		// Django rejects the negative slice (:757-760) with ValueError,
		// which list() turns into a 400 (:824-827).
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		for _, q := range []string{"limit=abc", "skip=abc", "skip=-1"} {
			status, body := httpGet(t, srv, "/api/net?"+q)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", q, status, string(body))
			}
		}
	})

	t.Run("DIVERGENCE_negative_limit_returns_400", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream parses a negative limit (2.83.0
		// rest.py:515-518) and then slices only when limit > 0
		// (:757-760), so ?limit=-5 returns every row. The mirror
		// rejects a negative limit with 400, like a non-numeric one.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?limit=-5")
		if status != http.StatusBadRequest {
			t.Errorf("?limit=-5: status = %d, want 400 (divergence canary); body=%s", status, string(body))
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
