package pdbcompat

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestCheckBudget_UnderBudget confirms a modest request fits comfortably
// under a generous budget and returns (zero, true). Regression guard
// against accidental off-by-one in the comparison.
func TestCheckBudget_UnderBudget(t *testing.T) {
	t.Parallel()
	info, ok := CheckBudget(100, peeringdb.TypeNet, 0, 1<<20) // 1 MiB budget
	if !ok {
		t.Fatalf("CheckBudget ok = false, want true (100 net rows @ depth=0 fits in 1 MiB)")
	}
	if info != (BudgetExceeded{}) {
		t.Errorf("info should be zero value when ok=true, got %+v", info)
	}
}

// TestCheckBudget_OverBudget confirms a large request breaches a tight
// budget and returns a populated BudgetExceeded + false.
func TestCheckBudget_OverBudget(t *testing.T) {
	t.Parallel()
	info, ok := CheckBudget(1_000_000, peeringdb.TypeNet, 2, 1<<20) // 1 MiB
	if ok {
		t.Fatalf("CheckBudget ok = true, want false (1M net rows @ depth=2 exceeds 1 MiB)")
	}
	if info.MaxRows <= 0 {
		t.Errorf("MaxRows should be positive when over budget, got %d", info.MaxRows)
	}
	if info.BudgetBytes != 1<<20 {
		t.Errorf("BudgetBytes = %d, want %d", info.BudgetBytes, 1<<20)
	}
	if info.EstimatedBytes <= info.BudgetBytes {
		t.Errorf("EstimatedBytes (%d) should exceed BudgetBytes (%d)", info.EstimatedBytes, info.BudgetBytes)
	}
	if info.Count != 1_000_000 {
		t.Errorf("Count = %d, want 1000000", info.Count)
	}
	if info.Entity != peeringdb.TypeNet {
		t.Errorf("Entity = %q, want %q", info.Entity, peeringdb.TypeNet)
	}
	if info.Depth != 2 {
		t.Errorf("Depth = %d, want 2", info.Depth)
	}
}

// TestCheckBudget_ZeroBudgetDisabled confirms budgetBytes=0 disables the
// check entirely — SyncMemoryLimit-symmetric semantic (D-05).
func TestCheckBudget_ZeroBudgetDisabled(t *testing.T) {
	t.Parallel()
	// Absurd count at maximum depth — would be billions of bytes under
	// a real budget — but budget=0 bypasses the check.
	info, ok := CheckBudget(1_000_000_000, peeringdb.TypeOrg, 2, 0)
	if !ok {
		t.Fatalf("CheckBudget ok = false with budget=0, want true (disabled)")
	}
	if info != (BudgetExceeded{}) {
		t.Errorf("info should be zero value when budget disabled, got %+v", info)
	}
}

// TestCheckBudget_MaxRowsComputation asserts MaxRows is budget/perRow
// (integer division, floor) and non-zero whenever budget >= perRow.
func TestCheckBudget_MaxRowsComputation(t *testing.T) {
	t.Parallel()
	perRow := TypicalRowBytes(peeringdb.TypeNet, 0)
	if perRow <= 0 {
		t.Fatalf("TypicalRowBytes returned non-positive perRow = %d", perRow)
	}
	budget := int64(4 * perRow) // room for exactly 4 rows
	// Request 100 rows — must breach.
	info, ok := CheckBudget(100, peeringdb.TypeNet, 0, budget)
	if ok {
		t.Fatalf("CheckBudget ok = true, want false (100 rows exceed %d-byte budget)", budget)
	}
	wantMaxRows := int(budget / int64(perRow))
	if info.MaxRows != wantMaxRows {
		t.Errorf("MaxRows = %d, want %d (budget=%d / perRow=%d)", info.MaxRows, wantMaxRows, budget, perRow)
	}
	if info.MaxRows == 0 {
		t.Errorf("MaxRows should never be 0 when budget > 0")
	}
}

// TestCheckBudget_UnknownEntity confirms an unregistered entity name
// falls through to defaultRowSize (fail-closed) without panic.
func TestCheckBudget_UnknownEntity(t *testing.T) {
	t.Parallel()
	// Large enough count to clearly exceed defaultRowSize × budget.
	info, ok := CheckBudget(1_000_000, "nosuchtype", 0, 1<<20)
	if ok {
		t.Fatalf("CheckBudget ok = true, want false for unknown entity with 1M rows vs 1 MiB budget")
	}
	if info.Entity != "nosuchtype" {
		t.Errorf("Entity = %q, want %q", info.Entity, "nosuchtype")
	}
	// defaultRowSize is the conservative fallback; MaxRows must reflect it.
	wantMaxRows := int(int64(1<<20) / int64(defaultRowSize))
	if info.MaxRows != wantMaxRows {
		t.Errorf("MaxRows = %d, want %d (1 MiB / defaultRowSize=%d)", info.MaxRows, wantMaxRows, defaultRowSize)
	}
}

// TestWriteBudgetProblem_Body asserts the 413 response shape matches D-04
// verbatim: status, Content-Type, and all six required JSON fields plus
// optional instance.
func TestWriteBudgetProblem_Body(t *testing.T) {
	t.Parallel()
	info := BudgetExceeded{
		MaxRows:        50,
		BudgetBytes:    1 << 17, // 131072
		EstimatedBytes: 1 << 20, // 1048576
		Count:          1000,
		Entity:         peeringdb.TypeNet,
		Depth:          2,
	}
	rec := httptest.NewRecorder()
	WriteBudgetProblem(rec, "/api/net", info)

	if rec.Code != 413 {
		t.Errorf("status = %d, want 413", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	if pb := rec.Header().Get("X-Powered-By"); pb != poweredByHeader {
		t.Errorf("X-Powered-By = %q, want %q", pb, poweredByHeader)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "" {
		t.Errorf("Retry-After = %q, want empty (request-shape, not transient)", ra)
	}

	var body struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Status      int    `json:"status"`
		Detail      string `json:"detail"`
		Instance    string `json:"instance"`
		MaxRows     int    `json:"max_rows"`
		BudgetBytes int64  `json:"budget_bytes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v (raw=%s)", err, rec.Body.String())
	}
	if body.Type != ResponseTooLargeType {
		t.Errorf("type = %q, want %q", body.Type, ResponseTooLargeType)
	}
	if body.Title != "Response exceeds memory budget" {
		t.Errorf("title = %q, want %q", body.Title, "Response exceeds memory budget")
	}
	if body.Status != 413 {
		t.Errorf("status = %d, want 413", body.Status)
	}
	if body.Detail == "" {
		t.Errorf("detail must not be empty")
	}
	if body.Instance != "/api/net" {
		t.Errorf("instance = %q, want /api/net", body.Instance)
	}
	if body.MaxRows != 50 {
		t.Errorf("max_rows = %d, want 50", body.MaxRows)
	}
	if body.BudgetBytes != 1<<17 {
		t.Errorf("budget_bytes = %d, want %d", body.BudgetBytes, 1<<17)
	}
}

// TestWriteBudgetProblem_DetailString asserts the human-readable detail
// string cites the count, estimated bytes, and budget in the form
// promised by D-04: "Request would return ~N rows totaling ~B bytes;
// limit is L bytes".
func TestWriteBudgetProblem_DetailString(t *testing.T) {
	t.Parallel()
	info := BudgetExceeded{
		MaxRows:        50,
		BudgetBytes:    131072,
		EstimatedBytes: 1048576,
		Count:          1000,
		Entity:         peeringdb.TypeNet,
		Depth:          2,
	}
	rec := httptest.NewRecorder()
	WriteBudgetProblem(rec, "", info)

	var body struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, needle := range []string{"~1000 rows", "~1048576 bytes", "limit is 131072 bytes"} {
		if !strings.Contains(body.Detail, needle) {
			t.Errorf("detail missing %q; got %q", needle, body.Detail)
		}
	}
}
