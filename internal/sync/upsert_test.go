// Tests for the sync upsert layer.
//
// TestUpsertPopulatesFoldColumns anchors Phase 69 Plan 03's contract: the 6
// upsert functions for entity types with _fold shadow columns (organization,
// network, facility, internetexchange, campus, carrier) populate those
// columns at sync time via unifold.Fold(). End-to-end round-trip asserts
// that `Name: "Zürich GmbH"` → DB → `NameFold: "zurich gmbh"`.
//
// TestUpsert_SkipOnUnchanged anchors 260428-eda CHANGE 3's contract:
// per-row skip-on-unchanged via the SQL ON CONFLICT DO UPDATE WHERE
// predicate gates writes on the upstream `updated` timestamp.
package sync

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestUpsertPopulatesFoldColumns verifies that upsertOrganizations (the
// canonical 3-fold-column case) calls unifold.Fold on Name/Aka/City and
// writes to the sibling *_fold columns via SetNameFold/SetAkaFold/
// SetCityFold. The other 5 affected upsert funcs (network, facility,
// internetexchange, campus, carrier) follow the same pattern; a
// grep-audit in the plan's verification step covers those.
//
// Closes UNICODE-01 sync-side data-population path.
func TestUpsertPopulatesFoldColumns(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx: %v", err)
	}
	t.Cleanup(func() {
		// Rollback is a no-op after a successful Commit; keeping it
		// here guards against early-return failure paths.
		_ = tx.Rollback()
	})

	now := time.Now().UTC().Truncate(time.Second)
	orgs := []peeringdb.Organization{
		{
			ID:      1,
			Name:    "Zürich GmbH",
			Aka:     "Straße 23",
			City:    "Köln",
			Created: now,
			Updated: now,
			Status:  "ok",
		},
	}
	ids, err := upsertOrganizations(ctx, tx, orgs)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("unexpected ids: %v", ids)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.NameFold != "zurich gmbh" {
		t.Errorf("NameFold: got %q, want %q", got.NameFold, "zurich gmbh")
	}
	if got.AkaFold != "strasse 23" {
		t.Errorf("AkaFold: got %q, want %q", got.AkaFold, "strasse 23")
	}
	if got.CityFold != "koln" {
		t.Errorf("CityFold: got %q, want %q", got.CityFold, "koln")
	}

	// Idempotency check: re-upsert with ASCII variants must produce the
	// same fold values (OnConflictColumns().UpdateNewValues() path rewrites
	// the _fold columns on every cycle).
	tx2, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx2: %v", err)
	}
	t.Cleanup(func() { _ = tx2.Rollback() })

	// 260428-eda CHANGE 3: bump Updated past the prior write so the
	// skip-on-unchanged predicate (excluded.updated > existing.updated)
	// admits this re-upsert. With Updated=now (unchanged) the predicate
	// would correctly leave the row untouched — exercised separately by
	// TestUpsert_SkipOnUnchanged.
	later := now.Add(time.Second)
	orgs2 := []peeringdb.Organization{
		{
			ID:      1,
			Name:    "Zurich GmbH",
			Aka:     "Strasse 23",
			City:    "Koln",
			Created: now,
			Updated: later,
			Status:  "ok",
		},
	}
	if _, err := upsertOrganizations(ctx, tx2, orgs2); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("commit2: %v", err)
	}

	got2, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("read back 2: %v", err)
	}
	if got2.NameFold != "zurich gmbh" {
		t.Errorf("NameFold after re-upsert: got %q, want %q", got2.NameFold, "zurich gmbh")
	}
	if got2.AkaFold != "strasse 23" {
		t.Errorf("AkaFold after re-upsert: got %q, want %q", got2.AkaFold, "strasse 23")
	}
	if got2.CityFold != "koln" {
		t.Errorf("CityFold after re-upsert: got %q, want %q", got2.CityFold, "koln")
	}
}

// TestUpsert_SkipOnUnchanged anchors 260428-eda CHANGE 3: per-row
// skip-on-unchanged via the SQL ON CONFLICT DO UPDATE WHERE predicate
// gates writes on the upstream `updated` timestamp.
//
// Sub-tests:
//   - skip:                same updated → row NOT updated.
//   - update_on_newer:     newer updated → row IS updated.
//   - update_on_zero:      both rows have updated=time.Time{} →
//     predicate's zero-time guard admits the write.
//   - insert_when_absent:  no prior row → INSERT path runs.
//
// The test uses Name as the sentinel: it changes between writes so a
// successful update flips the read-back value.
func TestUpsert_SkipOnUnchanged(t *testing.T) {
	t.Parallel()

	t.Run("skip", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client := testutil.SetupClient(t)

		base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

		tx1, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx1: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx1, []peeringdb.Organization{{
			ID: 1, Name: "A", Created: base, Updated: base, Status: "ok",
		}}); err != nil {
			_ = tx1.Rollback()
			t.Fatalf("first upsert: %v", err)
		}
		if err := tx1.Commit(); err != nil {
			t.Fatalf("commit tx1: %v", err)
		}

		// Second upsert with SAME updated but DIFFERENT name — must skip.
		tx2, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx2: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx2, []peeringdb.Organization{{
			ID: 1, Name: "B", Created: base, Updated: base, Status: "ok",
		}}); err != nil {
			_ = tx2.Rollback()
			t.Fatalf("second upsert: %v", err)
		}
		if err := tx2.Commit(); err != nil {
			t.Fatalf("commit tx2: %v", err)
		}

		got, err := client.Organization.Get(ctx, 1)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Name != "A" {
			t.Errorf("Name = %q, want %q (skip-on-unchanged should have left the row untouched)", got.Name, "A")
		}
	})

	t.Run("update_on_newer", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client := testutil.SetupClient(t)

		base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
		later := base.Add(time.Second)

		tx1, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx1: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx1, []peeringdb.Organization{{
			ID: 1, Name: "A", Created: base, Updated: base, Status: "ok",
		}}); err != nil {
			_ = tx1.Rollback()
			t.Fatalf("first upsert: %v", err)
		}
		if err := tx1.Commit(); err != nil {
			t.Fatalf("commit tx1: %v", err)
		}

		tx2, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx2: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx2, []peeringdb.Organization{{
			ID: 1, Name: "B", Created: base, Updated: later, Status: "ok",
		}}); err != nil {
			_ = tx2.Rollback()
			t.Fatalf("second upsert: %v", err)
		}
		if err := tx2.Commit(); err != nil {
			t.Fatalf("commit tx2: %v", err)
		}

		got, err := client.Organization.Get(ctx, 1)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Name != "B" {
			t.Errorf("Name = %q, want %q (newer updated should have updated the row)", got.Name, "B")
		}
	})

	t.Run("update_on_zero", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client := testutil.SetupClient(t)

		// Both rows carry updated=time.Time{}. The predicate's zero-time
		// guard (<= '1900-01-01' on the modernc text representation
		// '0001-01-01...') admits the second write so a row with no
		// updated value is never permanently frozen.
		zero := time.Time{}

		tx1, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx1: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx1, []peeringdb.Organization{{
			ID: 1, Name: "A", Created: zero, Updated: zero, Status: "ok",
		}}); err != nil {
			_ = tx1.Rollback()
			t.Fatalf("first upsert: %v", err)
		}
		if err := tx1.Commit(); err != nil {
			t.Fatalf("commit tx1: %v", err)
		}

		tx2, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx2: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx2, []peeringdb.Organization{{
			ID: 1, Name: "B", Created: zero, Updated: zero, Status: "ok",
		}}); err != nil {
			_ = tx2.Rollback()
			t.Fatalf("second upsert: %v", err)
		}
		if err := tx2.Commit(); err != nil {
			t.Fatalf("commit tx2: %v", err)
		}

		got, err := client.Organization.Get(ctx, 1)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Name != "B" {
			t.Errorf("Name = %q, want %q (zero-updated rows must always update)", got.Name, "B")
		}
	})

	t.Run("insert_when_absent", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		client := testutil.SetupClient(t)

		base := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx, []peeringdb.Organization{{
			ID: 42, Name: "fresh", Created: base, Updated: base, Status: "ok",
		}}); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert upsert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}

		got, err := client.Organization.Get(ctx, 42)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Name != "fresh" {
			t.Errorf("Name = %q, want %q (INSERT path)", got.Name, "fresh")
		}
	})
}
