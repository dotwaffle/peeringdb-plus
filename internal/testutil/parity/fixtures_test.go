package parity

import (
	"fmt"
	"testing"
	"time"
)

// TestOrderingFixtures_NonEmpty is the plan 72-01 must_haves truth:
// the ported set must have ≥5 entries so downstream parity tests have
// a meaningful corpus. (Empty output would indicate a parser
// regression or an upstream structural change.)
func TestOrderingFixtures_NonEmpty(t *testing.T) {
	t.Parallel()
	const minEntries = 5
	if got := len(OrderingFixtures); got < minEntries {
		t.Fatalf("OrderingFixtures has %d entries, want >= %d", got, minEntries)
	}
}

// TestOrderingFixtures_EntityAndUpstreamPopulated asserts every entry
// carries a non-empty Entity + Upstream citation. Without the citation
// a future maintainer can't trace a failing parity test back to the
// upstream source line.
func TestOrderingFixtures_EntityAndUpstreamPopulated(t *testing.T) {
	t.Parallel()
	for i, fx := range OrderingFixtures {
		if fx.Entity == "" {
			t.Errorf("OrderingFixtures[%d] has empty Entity: %+v", i, fx)
		}
		if fx.Upstream == "" {
			t.Errorf("OrderingFixtures[%d] has empty Upstream: %+v", i, fx)
		}
	}
}

// TestOrderingFixtures_NoDuplicateIDs asserts no two entries share the
// same (Entity, ID) pair. Duplicates would violate the cross-run
// stability contract — a downstream test iterating the slice would
// create ent rows twice under the same primary key and panic at
// upsert time.
func TestOrderingFixtures_NoDuplicateIDs(t *testing.T) {
	t.Parallel()
	seen := map[string]int{} // "entity|id" -> first-seen index
	for i, fx := range OrderingFixtures {
		key := fmt.Sprintf("%s|%d", fx.Entity, fx.ID)
		if prev, ok := seen[key]; ok {
			t.Errorf("duplicate (Entity=%q, ID=%d): indices %d and %d", fx.Entity, fx.ID, prev, i)
			continue
		}
		seen[key] = i
	}
}

// TestOrderingFixtures_HasCreatedAndUpdated enforces the ordering-
// category invariant: every entry MUST carry `created` and `updated`
// keys in Fields, both parseable as RFC3339 timestamps. The
// (-updated, -created) default ordering assertion (Phase 67) cannot
// be exercised without them.
//
// Values are stored as Python-style Go-quoted strings (e.g.
// `"2024-02-21T18:00:00Z"` — note the embedded quotes) so the test
// trims the outer quotes before parsing. Plans 72-02/03 may introduce
// richer Field serializations; adjust this test accordingly.
func TestOrderingFixtures_HasCreatedAndUpdated(t *testing.T) {
	t.Parallel()
	for i, fx := range OrderingFixtures {
		created, ok := fx.Fields["created"]
		if !ok || created == "" {
			t.Errorf("OrderingFixtures[%d] (%s@%d) missing `created` field: %+v", i, fx.Entity, fx.ID, fx)
			continue
		}
		updated, ok := fx.Fields["updated"]
		if !ok || updated == "" {
			t.Errorf("OrderingFixtures[%d] (%s@%d) missing `updated` field: %+v", i, fx.Entity, fx.ID, fx)
			continue
		}
		if _, err := time.Parse(time.RFC3339, unquote(created)); err != nil {
			t.Errorf("OrderingFixtures[%d] (%s@%d) `created`=%q not RFC3339: %v", i, fx.Entity, fx.ID, created, err)
		}
		if _, err := time.Parse(time.RFC3339, unquote(updated)); err != nil {
			t.Errorf("OrderingFixtures[%d] (%s@%d) `updated`=%q not RFC3339: %v", i, fx.Entity, fx.ID, updated, err)
		}
	}
}

// unquote strips a single pair of double quotes from s. The ported
// fields are stored in their Python-source form (quotes-and-all) so
// parity consumers can round-trip the literal; tests trim them for
// typed parsing.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// TestStatusFixtures_NonEmpty asserts plan 72-02 Task 2 Test 1: the
// ported StatusFixtures must be non-empty so parity tests have
// rows to seed.
func TestStatusFixtures_NonEmpty(t *testing.T) {
	t.Parallel()
	if len(StatusFixtures) == 0 {
		t.Fatal("StatusFixtures empty")
	}
}

// TestLimitFixtures_NonEmptyAndBoundary asserts Plan 72-02 Task 2
// Test 2: LimitFixtures non-empty + ≥260 Network entries to exercise
// the LIMIT-01 unlimited-pagination boundary above the 250 default
// page cap.
func TestLimitFixtures_NonEmptyAndBoundary(t *testing.T) {
	t.Parallel()
	if len(LimitFixtures) == 0 {
		t.Fatal("LimitFixtures empty")
	}
	var networkCount int
	for _, f := range LimitFixtures {
		if f.Entity == "net" {
			networkCount++
		}
	}
	if networkCount < 260 {
		t.Errorf("LIMIT-01 unlimited boundary: want ≥260 net fixtures; got %d", networkCount)
	}
}

// TestStatusFixtures_DistinctStatuses asserts Plan 72-02 Task 2
// Test 3: at least 3 distinct status values across {ok, pending,
// deleted} are present.
func TestStatusFixtures_DistinctStatuses(t *testing.T) {
	t.Parallel()
	statuses := map[string]int{}
	for _, f := range StatusFixtures {
		raw, ok := f.Fields["status"]
		if !ok {
			continue
		}
		statuses[unquote(raw)]++
	}
	if len(statuses) < 3 {
		t.Errorf("want ≥3 distinct statuses; got %v", statuses)
	}
	for _, want := range []string{"ok", "pending", "deleted"} {
		if statuses[want] == 0 {
			t.Errorf("StatusFixtures missing any status=%q row", want)
		}
	}
}

// TestStatusFixtures_CampusPendingCarveOut asserts Plan 72-02 Task 2
// Test 4: STATUS-03 carve-out — at least one (Entity="campus",
// status="pending") entry must be present so the campus pending-
// admission rule on since>0 list queries can be exercised.
func TestStatusFixtures_CampusPendingCarveOut(t *testing.T) {
	t.Parallel()
	for _, f := range StatusFixtures {
		if f.Entity != "campus" {
			continue
		}
		if raw, ok := f.Fields["status"]; ok && unquote(raw) == "pending" {
			return
		}
	}
	t.Error("STATUS-03 carve-out: no (campus, pending) fixture in StatusFixtures")
}

// TestAllFixtures_NoDuplicateIDsWithinCategory asserts Plan 72-02
// Task 2 Test 5: no duplicate (Entity, ID) pairs WITHIN each
// category slice. Crosses between slices are allowed by design.
func TestAllFixtures_NoDuplicateIDsWithinCategory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fixtures []Fixture
	}{
		{"ordering", OrderingFixtures},
		{"status", StatusFixtures},
		{"limit", LimitFixtures},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			seen := map[string]int{}
			for i, f := range tc.fixtures {
				key := fmt.Sprintf("%s|%d", f.Entity, f.ID)
				if prev, ok := seen[key]; ok {
					t.Errorf("duplicate (Entity=%q, ID=%d) in %s: indices %d and %d", f.Entity, f.ID, tc.name, prev, i)
					continue
				}
				seen[key] = i
			}
		})
	}
}

// TestAllFixtures_UpstreamCitationPresent asserts Plan 72-02 Task 2
// Test 6: every entry across all three slices has a non-empty
// Upstream citation. Required by threat T-72-02-02 (repudiation
// mitigation: every fixture must trace back to upstream source).
func TestAllFixtures_UpstreamCitationPresent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fixtures []Fixture
	}{
		{"ordering", OrderingFixtures},
		{"status", StatusFixtures},
		{"limit", LimitFixtures},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for i, f := range tc.fixtures {
				if f.Upstream == "" {
					t.Errorf("%s[%d] (%s@%d) missing Upstream citation", tc.name, i, f.Entity, f.ID)
				}
				if f.Entity == "" {
					t.Errorf("%s[%d] missing Entity (Upstream=%q)", tc.name, i, f.Upstream)
				}
			}
		})
	}
}
