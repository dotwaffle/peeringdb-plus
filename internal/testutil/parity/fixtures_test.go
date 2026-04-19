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
