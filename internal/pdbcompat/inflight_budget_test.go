package pdbcompat

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestServeList_ConcurrentBudgetPool locks the 2026-06-10 audit fix: the
// per-request budget admits each request in isolation, so two concurrent
// near-budget dumps could jointly materialize ~2x the budget on a 256 MB
// replica. Admission now charges a shared in-flight pool; a request whose
// estimate would overflow the pool gets 503 + Retry-After instead of
// stacking, and the charge releases when serving completes.
func TestServeList_ConcurrentBudgetPool(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	const seedN = 50
	for i := 1; i <= seedN; i++ {
		if _, err := client.Organization.Create().
			SetID(i).SetName("PoolOrg").SetNameFold(unifold.Fold("PoolOrg")).
			SetCreated(now).SetUpdated(now).SetStatus("ok").
			Save(ctx); err != nil {
			t.Fatalf("seed org %d: %v", i, err)
		}
	}

	// Budget admits one full list (estimate = 50 x TypicalRowBytes) with
	// room to spare, but not two.
	estimate := int64(seedN) * int64(TypicalRowBytes("org", 0))
	budget := estimate + estimate/2

	h := NewHandler(client, budget)
	mux := http.NewServeMux()
	h.Register(mux)

	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/org", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	// Baseline: fits with an empty pool.
	if code := get(); code != http.StatusOK {
		t.Fatalf("baseline GET: status %d, want 200", code)
	}

	// Simulate another in-flight near-budget response holding its charge.
	h.inflightBytes.Add(estimate)
	if code := get(); code != http.StatusServiceUnavailable {
		t.Errorf("with pool nearly full: status %d, want 503", code)
	}

	// The release path: once the other response finishes, requests are
	// admitted again — proving the rejection path refunded its charge.
	h.inflightBytes.Add(-estimate)
	if code := get(); code != http.StatusOK {
		t.Errorf("after release: status %d, want 200 (charge leak?)", code)
	}
	if got := h.inflightBytes.Load(); got != 0 {
		t.Errorf("in-flight pool = %d after all requests done, want 0", got)
	}
}
