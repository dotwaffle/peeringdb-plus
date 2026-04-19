package pdbcompat

import "testing"

// TestRegistryInvariant_AllListHaveCount documents the Phase 71 WR-01
// contract: every Registry entry with a non-nil List MUST have a non-nil
// Count, because serveList's pre-flight budget check (handler.go) is
// gated on `tc.Count != nil`. A missing CountFunc would silently bypass
// the 413 guardrail.
//
// The package's init() already panics at startup if this invariant is
// violated (registry_funcs.go) — this test documents the contract so a
// future contributor who adds a new entity understands the intent, and
// so the test suite asserts the invariant holds today independent of
// init() execution order.
func TestRegistryInvariant_AllListHaveCount(t *testing.T) {
	t.Parallel()
	for name, tc := range Registry {
		if tc.List != nil && tc.Count == nil {
			t.Errorf("Registry[%q] has List without CountFunc — Phase 71 WR-01 invariant violated", name)
		}
	}
}
