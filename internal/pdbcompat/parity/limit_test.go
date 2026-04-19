package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Limit locks the v1.16 limit semantics:
//
//   - LIMIT-01: ?limit=0 returns ALL rows unbounded (matches upstream
//     rest.py:494-497, NOT count-only as some clients incorrectly
//     assume).
//   - LIMIT-01b: ?limit=0 paired with the Phase 71 D-04 response
//     budget returns 413 application/problem+json when the precount
//     × TypicalRowBytes exceeds the budget.
//   - LIMIT-02: ?depth=N on a list endpoint is silently dropped
//     (Phase 68 LIMIT-02 guardrail). DIVERGENCE from upstream which
//     accepts depth on list. See docs/API.md § Known Divergences and
//     CONTEXT.md D-04.
//
// upstream: peeringdb_server/rest.py:494-497 (limit=0 = unlimited)
// upstream: peeringdb_server/rest.py:734-737 (page_size_query_param)
func TestParity_Limit(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("LIMIT-01_zero_returns_all_rows_unbounded", func(t *testing.T) {
		t.Parallel()
		// upstream: pdb_api_test.py (multiple call sites pass limit=0
		// to retrieve full corpora; the call shape is implicit in the
		// fixture-count assertions).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		const seedN = 300 // > DefaultLimit=250, < MaxLimit=1000
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
		// LIMIT-01 path without the Phase 71 413 layer interfering.
		srv := newTestServer(t, c)

		// Control: default limit returns DefaultLimit=250.
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("default limit status = %d; body=%s", status, string(body))
		}
		ctrl := decodeDataArray(t, body)
		if len(ctrl) != 250 {
			t.Errorf("control: got %d rows, want 250 (DefaultLimit)", len(ctrl))
		}

		// LIMIT-01: ?limit=0 returns all 300 rows.
		status, body = httpGet(t, srv, "/api/net?limit=0")
		if status != http.StatusOK {
			t.Fatalf("limit=0 status = %d; body=%s", status, string(body))
		}
		all := decodeDataArray(t, body)
		if len(all) != seedN {
			t.Errorf("LIMIT-01 ?limit=0: got %d rows, want %d (all unbounded)", len(all), seedN)
		}
	})

	t.Run("LIMIT-01b_zero_over_budget_returns_413_problem_json", func(t *testing.T) {
		t.Parallel()
		// Phase 71 D-02/D-04: pre-flight CheckBudget gate. With a tiny
		// per-response budget and a non-empty result, the count ×
		// TypicalRowBytes math returns 413 application/problem+json
		// before the .All() materialises anything.
		// synthesised: phase71-plan-04 (the budget mechanism is novel
		// to this fork; upstream has no equivalent gate).
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

	t.Run("LIMIT-02_depth_on_list_silently_dropped_DIVERGENCE", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream rest.py accepts ?depth on list
		// endpoints and embeds related objects per row. Phase 68
		// LIMIT-02 guardrail (handler.go:163-168) silently drops the
		// param to avoid memory blow-up at scale. Phase 71 owns the
		// safe list+depth implementation; until then the response is
		// IDENTICAL to a no-depth call.
		// See docs/API.md § Known Divergences and CONTEXT.md D-04.
		// synthesised: phase68-plan-03 (the silent-drop is novel to
		// this fork).
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
			t.Errorf("LIMIT-02 DIVERGENCE: ?depth=2 must produce identical id list as no-depth (silent-drop). got plain=%v depth=%v",
				idsPlain, idsDepth)
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
