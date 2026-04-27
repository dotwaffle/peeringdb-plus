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
// Test 6: every entry across all six slices has a non-empty Upstream
// citation. Required by threat T-72-02-02 (repudiation mitigation:
// every fixture must trace back to upstream source). Plan 72-03
// extends this to include UnicodeFixtures + InFixtures +
// TraversalFixtures.
func TestAllFixtures_UpstreamCitationPresent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fixtures []Fixture
	}{
		{"ordering", OrderingFixtures},
		{"status", StatusFixtures},
		{"limit", LimitFixtures},
		{"unicode", UnicodeFixtures},
		{"in", InFixtures},
		{"traversal", TraversalFixtures},
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

// TestUnicodeFixtures_Sanity asserts Plan 72-03 Task 2 Test 1:
// UnicodeFixtures must contain ≥32 entries with at least 2 distinct
// non-ASCII inputs (one diacritic + one CJK). Lower-bound 32 = 6
// entities × 4 fields × ≥1 sample minimum, well below the synth
// target of ~216 (6 × 4 × 9).
func TestUnicodeFixtures_Sanity(t *testing.T) {
	t.Parallel()
	const minEntries = 32
	if got := len(UnicodeFixtures); got < minEntries {
		t.Fatalf("UnicodeFixtures has %d entries, want ≥%d", got, minEntries)
	}
	var diacritic, cjk bool
	for _, f := range UnicodeFixtures {
		for _, v := range f.Fields {
			s := unquote(v)
			for _, r := range s {
				if r >= 0x4E00 && r <= 0x9FFF {
					cjk = true
				}
				if r == 'ü' || r == 'ö' || r == 'é' || r == 'ñ' {
					diacritic = true
				}
			}
		}
	}
	if !diacritic {
		t.Error("UnicodeFixtures has no diacritic sample (ü/ö/é/ñ)")
	}
	if !cjk {
		t.Error("UnicodeFixtures has no CJK sample (U+4E00..U+9FFF)")
	}
}

// TestInFixtures_LargeContiguousBlock asserts Plan 72-03 Task 2
// Test 2: InFixtures must contain a contiguous Entity="network"
// block at IDs 100000..105000 (exactly 5001 entries) + the sentinel
// at ID=999999 for the empty-__in test.
func TestInFixtures_LargeContiguousBlock(t *testing.T) {
	t.Parallel()
	byID := map[int]bool{}
	var hasSentinel bool
	for _, f := range InFixtures {
		if f.Entity != "net" {
			continue
		}
		byID[f.ID] = true
		if f.ID == 999999 {
			hasSentinel = true
		}
	}
	if !hasSentinel {
		t.Error("InFixtures missing empty-__in sentinel (ID=999999)")
	}
	const lo, hi = 100000, 105000
	missing := 0
	for id := lo; id <= hi; id++ {
		if !byID[id] {
			missing++
			if missing <= 5 {
				t.Errorf("InFixtures missing network fixture ID %d", id)
			}
		}
	}
	if missing > 5 {
		t.Errorf("InFixtures missing %d total IDs in [%d..%d]", missing, lo, hi)
	}
}

// TestTraversalFixtures_RingAndSilentIgnore asserts Plan 72-03 Task
// 2 Test 3: TraversalFixtures contains ≥1 fixture with __hop="2"
// (verifying Path A or Path B 2-hop coverage) AND ≥1 silent-ignore
// fixture (TRAVERSAL-04 — Phase 70 D-04 hard-2-hop cap silently
// ignores 3+-segment chains).
func TestTraversalFixtures_RingAndSilentIgnore(t *testing.T) {
	t.Parallel()
	var twoHop, silentIgnore bool
	for _, f := range TraversalFixtures {
		if h, ok := f.Fields["__hop"]; ok && unquote(h) == "2" {
			twoHop = true
		}
		if o, ok := f.Fields["__expected_outcome"]; ok && unquote(o) == "silent-ignore" {
			silentIgnore = true
		}
	}
	if !twoHop {
		t.Error("TraversalFixtures has no 2-hop fixture (__hop=\"2\")")
	}
	if !silentIgnore {
		t.Error("TraversalFixtures has no silent-ignore fixture (TRAVERSAL-04)")
	}
}

// TestAllFixtures_NoDuplicateIDsWithinCategoryAllSix extends the
// 72-02 within-category dedup test to include the 3 new category
// slices. Cross-category collisions are allowed by design (each
// category seeds into its own ent client at test runtime).
func TestAllFixtures_NoDuplicateIDsWithinCategoryAllSix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fixtures []Fixture
	}{
		{"unicode", UnicodeFixtures},
		{"in", InFixtures},
		{"traversal", TraversalFixtures},
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
