package sync

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestFilterByStatus_DeletedExcluded exercises the generic filterByStatus
// helper (REFAC-04, Commit E) once per PeeringDB entity type. The 13
// subtests are deliberately per-type instead of a single generic table
// entry so that if per-type semantics ever diverge in the future (e.g.
// a new "suspended" status for one type), each row forces explicit
// handling. Per CONTEXT.md §REFAC-04 and PITFALLS.md §MP-3 silent
// unification trap.
//
// Each subtest constructs two rows — one with Status="ok" and one with
// Status="deleted" — and asserts filterByStatus excludes the deleted
// row exactly. The constructor literals double as a compile-time
// signature lock against accidental field drift on the peeringdb types.
func TestFilterByStatus_DeletedExcluded(t *testing.T) {
	t.Parallel()

	t.Run("org", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Organization{
			{ID: 1, Name: "ok", Status: "ok"},
			{ID: 2, Name: "dead", Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Organization) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("org filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("campus", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Campus{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Campus) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("campus filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("fac", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Facility{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Facility) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("fac filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("carrier", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Carrier{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Carrier) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("carrier filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("carrierfac", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.CarrierFacility{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.CarrierFacility) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("carrierfac filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("ix", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.InternetExchange{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.InternetExchange) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("ix filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("ixlan", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.IxLan{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.IxLan) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("ixlan filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("ixpfx", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.IxPrefix{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.IxPrefix) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("ixpfx filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("ixfac", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.IxFacility{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.IxFacility) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("ixfac filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("net", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Network{
			{ID: 1, Name: "ok", Status: "ok"},
			{ID: 2, Name: "dead", Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Network) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("net filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("poc", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.Poc{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.Poc) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("poc filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("netfac", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.NetworkFacility{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.NetworkFacility) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("netfac filter: got %+v, want 1 ok row", out)
		}
	})

	t.Run("netixlan", func(t *testing.T) {
		t.Parallel()
		in := []peeringdb.NetworkIxLan{
			{ID: 1, Status: "ok"},
			{ID: 2, Status: "deleted"},
		}
		out := filterByStatus(in, func(v peeringdb.NetworkIxLan) string { return v.Status })
		if len(out) != 1 || out[0].ID != 1 {
			t.Fatalf("netixlan filter: got %+v, want 1 ok row", out)
		}
	})
}

// TestFilterByStatus_EmptyAndAllDeleted verifies edge cases of the
// generic filter helper: the nil/empty input returns a non-nil empty
// slice (allocation profile preserved from the per-type helpers), and
// an all-deleted input returns an empty slice without panicking.
func TestFilterByStatus_EmptyAndAllDeleted(t *testing.T) {
	t.Parallel()

	empty := filterByStatus[peeringdb.Organization](nil, func(v peeringdb.Organization) string { return v.Status })
	if empty == nil {
		t.Errorf("empty input: got nil, want allocated empty slice")
	}
	if len(empty) != 0 {
		t.Errorf("empty input: got len=%d, want 0", len(empty))
	}

	allDeleted := []peeringdb.Organization{
		{ID: 1, Status: "deleted"},
		{ID: 2, Status: "deleted"},
	}
	out := filterByStatus(allDeleted, func(v peeringdb.Organization) string { return v.Status })
	if len(out) != 0 {
		t.Errorf("all-deleted input: got len=%d, want 0", len(out))
	}
}

// TestSyncIncremental_GenericDispatchCallSites is a structural
// regression lock for REFAC-04 (Commit E): the dispatchScratchChunk
// function must call syncIncremental[E] exactly 13 times — once per
// closed-set PeeringDB entity type. This prevents future edits from
// accidentally dropping, duplicating, or short-circuiting a case arm.
//
// The regex matches `syncIncremental(ctx, tx, syncIncrementalInput[`
// which is the canonical call shape enforced by the dispatcher. If
// the dispatcher ever stops using the per-line call pattern (e.g.,
// builds a registry map instead), update this test's matcher in
// lockstep with the new pattern.
func TestSyncIncremental_GenericDispatchCallSites(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}

	matcher := regexp.MustCompile(`syncIncremental\(ctx,\s*tx,\s*syncIncrementalInput\[`)
	matches := matcher.FindAll(src, -1)
	if len(matches) != 13 {
		t.Fatalf("expected 13 syncIncremental[E] dispatch call sites, got %d", len(matches))
	}
}

// TestSyncIncremental_NoStaleFilterFunctions is a structural regression
// lock for REFAC-04 (Commit E): the 13 pre-REFAC-04 filterXByStatus
// functions that used to live in filter.go must not come back. This
// test asserts filter.go is absent AND no sync_*.go file declares any
// of the 13 historical filter function names.
func TestSyncIncremental_NoStaleFilterFunctions(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("filter.go"); !os.IsNotExist(err) {
		t.Fatalf("filter.go must not exist after REFAC-04 (err=%v) — the 13 per-type filter helpers were replaced by the generic filterByStatus", err)
	}

	stale := []string{
		"filterOrgsByStatus",
		"filterCampusesByStatus",
		"filterFacilitiesByStatus",
		"filterCarriersByStatus",
		"filterCarrierFacilitiesByStatus",
		"filterInternetExchangesByStatus",
		"filterIxLansByStatus",
		"filterIxPrefixesByStatus",
		"filterIxFacilitiesByStatus",
		"filterNetworksByStatus",
		"filterPocsByStatus",
		"filterNetworkFacilitiesByStatus",
		"filterNetworkIxLansByStatus",
	}

	// Scan every .go file in the current package directory (excluding
	// test files for the assertion — the test file itself mentions the
	// function names in this list literal, which is the exception).
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isGoSourceFile(name) {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, fn := range stale {
			needle := []byte("func " + fn + "(")
			if bytes.Contains(src, needle) {
				t.Errorf("stale per-type filter function %q still declared in %s — REFAC-04 incomplete", fn, name)
			}
		}
	}
}

// isGoSourceFile reports whether name is a non-test .go source file.
func isGoSourceFile(name string) bool {
	if len(name) < 3 || name[len(name)-3:] != ".go" {
		return false
	}
	// Exclude _test.go files to keep the stale-function-name string
	// literals in this file from triggering a self-match.
	if len(name) > 8 && name[len(name)-8:] == "_test.go" {
		return false
	}
	return true
}
