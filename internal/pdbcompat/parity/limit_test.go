package parity

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Limit locks the v1.16 limit semantics:
//
//   - ?limit=0 returns ALL rows unbounded (matches upstream
//     rest.py:494-497, NOT count-only as some clients incorrectly
//     assume).
//   - ?limit=0 paired with the response budget returns 413
//     application/problem+json when the precount × TypicalRowBytes
//     exceeds the budget.
//   - ?depth=N on a list endpoint is silently dropped by the
//     guardrail. DIVERGENCE from upstream which accepts depth on
//     list. See docs/API.md § Known Divergences.
//
// upstream: peeringdb_server/rest.py:494-497 (limit=0 = unlimited)
// upstream: peeringdb_server/rest.py:734-737 (page_size_query_param)
func TestParity_Limit(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("bare_url_and_zero_both_return_all_rows", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:495 (limit defaults to 0) + rest.py:737
		// (limit=0 → qset[skip:], no slice). Both bare URL and explicit
		// ?limit=0 return ALL rows from the filtered queryset on
		// upstream — the 250 defaultable page size is opt-in via
		// ?page=N (UnlimitedIfNoPagePagination at rest.py:418-430,
		// only applies pagination when "page" is in query_params).
		//
		// Earlier revisions of this test asserted the bare URL returns
		// 250 rows, which was a parity bug that this fork inherited
		// from a defensive cap on response.go DefaultLimit. Verified
		// 2026-04-28 against upstream live data (parity-results.txt):
		// bare /api/org returned 33,556 rows upstream vs 250 on the
		// then-buggy mirror. DefaultLimit was changed from 250 to 0
		// to restore parity; the response-memory budget is
		// now the sole DoS gate, returning 413 application/problem+json
		// when precount × TypicalRowBytes exceeds the budget.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		const seedN = 300 // > the historical 250 cap, < MaxLimit=1000
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

	t.Run("zero_over_budget_returns_413_problem_json", func(t *testing.T) {
		t.Parallel()
		// pre-flight CheckBudget gate. With a tiny per-response budget
		// and a non-empty result, the count × TypicalRowBytes math
		// returns 413 application/problem+json before the .All()
		// materialises anything.
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
		p := mustDecodeProblem(t, body)
		if p.Status != 413 {
			t.Errorf("problem.status = %d, want 413", p.Status)
		}
		if p.Type == "" {
			t.Errorf("problem.type empty")
		}
		if p.BudgetBytes != 100 {
			t.Errorf("problem.budget_bytes = %d, want 100", p.BudgetBytes)
		}
		// max_rows is the integer divide; budget(100) / perRow(1600
		// for net depth 0) = 0, so the field MAY be 0; we only assert
		// it's set (>= 0 by definition of the integer divide).
		if p.MaxRows < 0 {
			t.Errorf("problem.max_rows = %d, want >= 0", p.MaxRows)
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

	t.Run("DIVERGENCE_error_envelope_problem_json_not_meta_error", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream renders every API error as a
		// {"meta":{"error":"<detail>"},"data":[]} envelope —
		// renderers.py:107/113 at the pinned 99e92c72 does
		// `meta["error"] = data.pop("detail", res.reason_phrase)`.
		// pdbcompat deliberately replaces that with RFC 9457
		// application/problem+json (response.go WriteProblem).
		// Upstream-compatible clients parsing meta.error on 4xx must
		// adapt; the trade is a standards-based, machine-readable
		// error shape shared with the other API surfaces.
		// See docs/API.md § Known Divergences.
		// upstream: peeringdb_server/renderers.py:107-113
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)

		// Both error classes carry the same problem+json envelope:
		// a 400 from a malformed ?limit= and a 404 from a missing PK.
		for _, tc := range []struct {
			name       string
			path       string
			wantStatus int
		}{
			{"bad_limit_400", "/api/net?limit=abc", http.StatusBadRequest},
			{"missing_pk_404", "/api/net/999999", http.StatusNotFound},
		} {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("%s: build request: %v", tc.name, err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s: GET %s: %v", tc.name, tc.path, err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("%s: read body: %v", tc.name, err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("%s: status = %d, want %d; body=%s",
					tc.name, resp.StatusCode, tc.wantStatus, string(body))
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("%s: Content-Type = %q, want application/problem+json", tc.name, ct)
			}
			p := mustDecodeProblem(t, body)
			if p.Status != tc.wantStatus {
				t.Errorf("%s: problem.status = %d, want %d", tc.name, p.Status, tc.wantStatus)
			}
			if p.Title == "" {
				t.Errorf("%s: problem.title empty", tc.name)
			}
			// The upstream envelope key must NOT be present: a client
			// reading meta.error gets nothing — that IS the divergence.
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatalf("%s: unmarshal raw body: %v", tc.name, err)
			}
			if _, hasMeta := raw["meta"]; hasMeta {
				t.Errorf("%s: error body carries upstream-style top-level meta key; divergence registry says it must not: %s",
					tc.name, string(body))
			}
		}
	})

	t.Run("limit_above_1000_honoured_uncapped", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:734-735 — qset[skip:skip+limit] with NO
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
		// upstream: rest.py:490-497 raises RestValidationError
		// ("'limit' needs to be a number") for non-numeric limit/skip.
		// Silently ignoring a typo'd limit turned a bounded page
		// request into a full-table dump.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		for _, q := range []string{"limit=abc", "skip=abc", "limit=-5", "skip=-1"} {
			status, body := httpGet(t, srv, "/api/net?"+q)
			if status != http.StatusBadRequest {
				t.Errorf("?%s: status = %d, want 400; body=%s", q, status, string(body))
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
