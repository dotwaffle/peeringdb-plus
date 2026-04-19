// Tests for the sync upsert layer.
//
// TestUpsertPopulatesFoldColumns anchors Phase 69 Plan 03's contract: the 6
// upsert functions for entity types with _fold shadow columns (organization,
// network, facility, internetexchange, campus, carrier) populate those
// columns at sync time via unifold.Fold(). End-to-end round-trip asserts
// that `Name: "Zürich GmbH"` → DB → `NameFold: "zurich gmbh"`.
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

	orgs2 := []peeringdb.Organization{
		{
			ID:      1,
			Name:    "Zurich GmbH",
			Aka:     "Strasse 23",
			City:    "Koln",
			Created: now,
			Updated: now,
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
