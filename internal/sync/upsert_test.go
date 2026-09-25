// Tests for the sync upsert layer.
//
// TestUpsertPopulatesFoldColumns anchors the contract that the 6
// upsert functions for entity types with _fold shadow columns (organization,
// network, facility, internetexchange, campus, carrier) populate those
// columns at sync time via unifold.Fold(). End-to-end round-trip asserts
// that `Name: "Zürich GmbH"` → DB → `NameFold: "zurich gmbh"`.
//
// TestUpsert_SkipOnUnchanged anchors the skip-on-unchanged contract:
// per-row skip-on-unchanged via the SQL ON CONFLICT DO UPDATE WHERE
// predicate gates writes on the upstream `updated` timestamp.
package sync

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestUpsert_UnDeletesOnResync verifies the deleted→ok transition: when
// upstream re-delivers a row that had been tombstoned, with a newer
// `updated` timestamp and status "ok", the ON CONFLICT update
// flips the row back to "ok" (auto-undelete). The same row resent
// with an unchanged `updated` is left tombstoned by the skip-on-unchanged
// predicate — upstream advances `updated` on every real change, so a
// status flip always carries a newer timestamp (audit PA5).
func TestUpsert_UnDeletesOnResync(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	base := time.Now().UTC().Truncate(time.Second)
	later := base.Add(time.Second)

	upsertOne := func(t *testing.T, client *ent.Client, org peeringdb.Organization) {
		t.Helper()
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		if _, err := upsertOrganizations(ctx, tx, []peeringdb.Organization{org}); err != nil {
			_ = tx.Rollback()
			t.Fatalf("upsert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	t.Run("newer timestamp un-deletes", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		upsertOne(t, client, peeringdb.Organization{ID: 1, Name: "Reborn", Created: base, Updated: base, Status: "deleted"})
		upsertOne(t, client, peeringdb.Organization{ID: 1, Name: "Reborn", Created: base, Updated: later, Status: "ok"})
		got, err := client.Organization.Get(ctx, 1)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Status != "ok" {
			t.Errorf("status = %q, want %q (deleted→ok un-delete)", got.Status, "ok")
		}
	})

	t.Run("same timestamp is skipped", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		upsertOne(t, client, peeringdb.Organization{ID: 2, Name: "Tomb", Created: base, Updated: base, Status: "deleted"})
		upsertOne(t, client, peeringdb.Organization{ID: 2, Name: "Tomb", Created: base, Updated: base, Status: "ok"})
		got, err := client.Organization.Get(ctx, 2)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if got.Status != "deleted" {
			t.Errorf("status = %q, want %q (same updated → skip-on-unchanged)", got.Status, "deleted")
		}
	})
}

// TestUpsertPopulatesFoldColumns verifies that upsertOrganizations (the
// canonical 3-fold-column case) calls unifold.Fold on Name/Aka/City and
// writes to the sibling *_fold columns via SetNameFold/SetAkaFold/
// SetCityFold. The other 5 affected upsert funcs (network, facility,
// internetexchange, campus, carrier) follow the same pattern; a
// grep-audit covers those.
//
// Covers the unicode-folding sync-side data-population path.
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
	// same fold values (the ON CONFLICT update rewrites the _fold
	// columns).
	tx2, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx2: %v", err)
	}
	t.Cleanup(func() { _ = tx2.Rollback() })

	// Bump Updated past the prior write so the
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

// TestUpsert_SkipOnUnchanged anchors the skip-on-unchanged contract: per-row
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

// TestUpsert_BlanksDeletedPocContact verifies that upsertPocs stores a
// deleted contact with name, phone, email and url blanked, on insert and
// on the ok→deleted conflict update, and keeps role, visible and net_id.
// A live contact keeps its contact data.
// upstream: 2.83.0 serializers.py:2941-2954 (#569)
func TestUpsert_BlanksDeletedPocContact(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := base.Add(time.Hour)

	client.Organization.Create().SetID(1).SetName("Org").SetNameFold("org").
		SetStatus("ok").SetCreated(base).SetUpdated(base).SaveX(ctx)
	client.Network.Create().SetID(1).SetOrgID(1).SetName("Net").SetNameFold("net").
		SetAsn(64500).SetStatus("ok").SetCreated(base).SetUpdated(base).SaveX(ctx)

	withContact := func(id int, status string, updated time.Time) peeringdb.Poc {
		return peeringdb.Poc{
			ID: id, NetID: 1, Role: "NOC", Visible: "Public",
			Name: "Jane Doe", Phone: "+1 555 0100", Email: "jane@example.invalid", URL: "https://example.invalid/jane",
			Created: base, Updated: updated, Status: status,
		}
	}
	upsert := func(items ...peeringdb.Poc) {
		t.Helper()
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		if _, err := upsertPocs(ctx, tx, items); err != nil {
			_ = tx.Rollback()
			t.Fatalf("upsert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	// 10: inserted as a tombstone. 11: live. 12: live, then deleted.
	upsert(withContact(10, "deleted", base), withContact(11, "ok", base), withContact(12, "ok", base))
	upsert(withContact(12, "deleted", later))

	for _, tc := range []struct {
		id          int
		wantStatus  string
		wantContact bool
	}{
		{10, "deleted", false},
		{11, "ok", true},
		{12, "deleted", false},
	} {
		got, err := client.Poc.Get(ctx, tc.id)
		if err != nil {
			t.Fatalf("read back poc %d: %v", tc.id, err)
		}
		want := withContact(tc.id, tc.wantStatus, got.Updated)
		if !tc.wantContact {
			want = want.BlankDeletedContact()
		}
		if got.Status != want.Status || got.Name != want.Name || got.Phone != want.Phone ||
			got.Email != want.Email || got.URL != want.URL {
			t.Errorf("poc %d: got status=%q name=%q phone=%q email=%q url=%q, want status=%q name=%q phone=%q email=%q url=%q",
				tc.id, got.Status, got.Name, got.Phone, got.Email, got.URL,
				want.Status, want.Name, want.Phone, want.Email, want.URL)
		}
		if got.Role != "NOC" || got.Visible != "Public" || got.NetID == nil || *got.NetID != 1 {
			t.Errorf("poc %d: role=%q visible=%q net_id=%v, want NOC, Public, 1", tc.id, got.Role, got.Visible, got.NetID)
		}
	}
}

// TestUpsertIxPrefixes_NullPrefixTombstone upserts an ixpfx tombstone
// whose prefix is null, as upstream sends for ixpfx 4185 (deleted
// 2024-09-10). The null decodes to "". The NotEmpty validator of the ent
// field rejected it, and each sync cycle that fetched the row failed.
func TestUpsertIxPrefixes_NullPrefixTombstone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	seedNetIxLanGateParents(t, client, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

	var pfx peeringdb.IxPrefix
	if err := json.Unmarshal([]byte(`{"id":4185,"ixlan_id":1,"protocol":"IPv4","prefix":null,"in_dfz":true,`+
		`"created":"2024-04-15T09:23:10Z","updated":"2024-09-10T08:05:42Z","status":"deleted"}`), &pfx); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx: %v", err)
	}
	if _, err := upsertIxPrefixes(ctx, tx, []peeringdb.IxPrefix{pfx}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("upsert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := client.IxPrefix.Get(ctx, 4185)
	if err != nil {
		t.Fatalf("read back ixpfx 4185: %v", err)
	}
	if got.Prefix != "" || got.Status != "deleted" || got.Protocol != "IPv4" {
		t.Errorf("ixpfx 4185: prefix=%q status=%q protocol=%q, want \"\", deleted, IPv4",
			got.Prefix, got.Status, got.Protocol)
	}
}

// TestUpsertNetworks_ZeroASNTombstone upserts a net tombstone with asn 0,
// as upstream sends for net 21510 (deleted 2019-11-22). The Positive
// validator of the ent asn field rejected it, which would fail each sync
// cycle that fetched the row.
func TestUpsertNetworks_ZeroASNTombstone(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	at := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	client.Organization.Create().SetID(1).SetName("Org").SetNameFold("org").
		SetStatus("ok").SetCreated(at).SetUpdated(at).SaveX(ctx)

	var net peeringdb.Network
	if err := json.Unmarshal([]byte(`{"id":21510,"org_id":1,"name":"Nathan Sales0","asn":0,`+
		`"created":"2019-11-22T15:54:02Z","updated":"2019-11-22T16:12:03Z","status":"deleted"}`), &net); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx: %v", err)
	}
	if _, err := upsertNetworks(ctx, tx, []peeringdb.Network{net}); err != nil {
		_ = tx.Rollback()
		t.Fatalf("upsert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	got, err := client.Network.Get(ctx, 21510)
	if err != nil {
		t.Fatalf("read back net 21510: %v", err)
	}
	if got.Asn != 0 || got.Status != "deleted" || got.Name != "Nathan Sales0" {
		t.Errorf("net 21510: asn=%d status=%q name=%q, want 0, deleted, Nathan Sales0",
			got.Asn, got.Status, got.Name)
	}
}

// TestUpsertNetworkIxLans_TombstoneGate locks netIxLanUpsertPredicate. A
// stored netixlan tombstone is not rewritten by a live row that carries
// the same updated, which only a stale full-mode bare list sends. Every
// other conflict follows skipUnchangedPredicate: a revival with a newer
// updated, the cutoff repair of a stored row older than the snapshot, a
// tombstone-to-tombstone rewrite, and a rewrite of a stored live row.
func TestUpsertNetworkIxLans_TombstoneGate(t *testing.T) {
	t.Parallel()
	u := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	cutoff := u.Add(2 * time.Hour)

	for _, tc := range []struct {
		name         string
		storedStatus string
		full         bool
		status       string
		updated      time.Time
		speed        int
		wantStatus   string
		wantSpeed    int
	}{
		{"full_ok_equal_updated_stays_deleted", "deleted", true, "ok", u, 2, "deleted", 1},
		{"full_not_operational_equal_updated_stays_deleted", "deleted", true, "not-operational", u, 3, "deleted", 1},
		{"full_deleted_equal_updated_is_rewritten", "deleted", true, "deleted", u, 5, "deleted", 5},
		{"full_ok_older_below_cutoff_revives", "deleted", true, "ok", u.Add(-time.Hour), 6, "ok", 6},
		{"full_ok_newer_revives", "deleted", true, "ok", u.Add(time.Hour), 7, "ok", 7},
		{"full_live_row_equal_updated_is_rewritten", "ok", true, "ok", u, 8, "ok", 8},
		{"incremental_ok_equal_updated_stays_deleted", "deleted", false, "ok", u, 9, "deleted", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			client := testutil.SetupClient(t)
			seedNetIxLanGateParents(t, client, u)
			client.NetworkIxLan.Create().
				SetID(9110).SetNetID(1).SetIxlanID(1).SetIxID(1).
				SetAsn(64500).SetSpeed(1).SetName("Gate IX").
				SetOperational(tc.storedStatus == "ok").SetStatus(tc.storedStatus).
				SetCreated(u).SetUpdated(u).SaveX(ctx)

			upsertCtx := ctx
			if tc.full {
				upsertCtx = withReconcileAll(ctx, map[string]time.Time{"network_ix_lans": cutoff})
			}
			tx, err := client.Tx(ctx)
			if err != nil {
				t.Fatalf("open tx: %v", err)
			}
			if _, err := upsertNetworkIxLans(upsertCtx, tx, []peeringdb.NetworkIxLan{{
				ID: 9110, NetID: 1, IXID: 1, IXLanID: 1, Name: "Gate IX",
				Speed: tc.speed, ASN: 64500,
				Created: u, Updated: tc.updated, Status: tc.status,
			}}); err != nil {
				_ = tx.Rollback()
				t.Fatalf("upsert: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}

			got := client.NetworkIxLan.GetX(ctx, 9110)
			if got.Status != tc.wantStatus || got.Speed != tc.wantSpeed {
				t.Errorf("status=%q speed=%d, want status=%q speed=%d",
					got.Status, got.Speed, tc.wantStatus, tc.wantSpeed)
			}
			if tc.wantSpeed == 1 && (got.Operational || !got.Updated.Equal(u)) {
				t.Errorf("kept row changed: operational=%v updated=%s, want false and %s",
					got.Operational, got.Updated, u)
			}
		})
	}
}

// seedNetIxLanGateParents creates org, net, ix and ixlan 1, the parents
// of the netixlan rows in the upsert gate tests.
func seedNetIxLanGateParents(t *testing.T, client *ent.Client, at time.Time) {
	t.Helper()
	ctx := t.Context()
	client.Organization.Create().SetID(1).SetName("Org").SetNameFold("org").
		SetStatus("ok").SetCreated(at).SetUpdated(at).SaveX(ctx)
	client.Network.Create().SetID(1).SetOrgID(1).SetName("Net").SetNameFold("net").
		SetAsn(64500).SetStatus("ok").SetCreated(at).SetUpdated(at).SaveX(ctx)
	client.InternetExchange.Create().SetID(1).SetOrgID(1).SetName("IX").SetNameFold("ix").
		SetStatus("ok").SetCreated(at).SetUpdated(at).SaveX(ctx)
	client.IxLan.Create().SetID(1).SetIxID(1).
		SetStatus("ok").SetCreated(at).SetUpdated(at).SaveX(ctx)
}

// TestNetIxLanUpsertPredicate_SQL locks the WHERE text of the netixlan
// upsert in both modes: the skip terms and the tombstone term each sit in
// their own parentheses, and every column carries the table qualifier.
// The predicate of the other tables has no tombstone term. In full mode,
// the updated terms and the column comparison sit in their own
// parentheses.
func TestNetIxLanUpsertPredicate_SQL(t *testing.T) {
	t.Parallel()
	cutoff := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	where := func(t *testing.T, table string, p *sql.Predicate) string {
		t.Helper()
		q, _ := sql.Dialect(dialect.SQLite).Insert(table).
			Columns("id", "status", "updated").
			Values(1, "ok", cutoff).
			OnConflict(
				sql.ConflictColumns("id"),
				sql.ResolveWithNewValues(),
				sql.UpdateWhere(p),
			).Query()
		_, text, ok := strings.Cut(q, " WHERE ")
		if !ok {
			t.Fatalf("no WHERE in %q", q)
		}
		return text
	}
	const tombstone = " AND (`network_ix_lans`.`status` <> 'deleted' OR excluded.status = 'deleted'" +
		" OR excluded.updated <> `network_ix_lans`.`updated` OR `network_ix_lans`.`updated` IS NULL)"
	// differs is the full-mode term that compares each column outside
	// the primary key.
	differs := func(tb *schema.Table) string {
		var terms []string
		for _, c := range tb.Columns {
			if !slices.Contains(tb.PrimaryKey, c) {
				terms = append(terms, fmt.Sprintf("excluded.`%s` IS NOT `%s`.`%s`", c.Name, tb.Name, c.Name))
			}
		}
		return strings.Join(terms, " OR ")
	}

	for _, tc := range []struct {
		name  string
		ctx   func(context.Context) context.Context
		table string
		pred  func(context.Context) *sql.Predicate
		want  string
	}{
		{
			name: "netixlan_full",
			ctx: func(ctx context.Context) context.Context {
				return withReconcileAll(ctx, map[string]time.Time{"network_ix_lans": cutoff})
			},
			table: "network_ix_lans",
			pred:  netIxLanUpsertPredicate,
			want: "((excluded.updated >= `network_ix_lans`.`updated` OR `network_ix_lans`.`updated` < ?" +
				" OR `network_ix_lans`.`updated` IS NULL OR `network_ix_lans`.`updated` <= '1900-01-01')" +
				" AND (" + differs(migrate.NetworkIxLansTable) + "))" + tombstone,
		},
		{
			name:  "netixlan_incremental",
			ctx:   func(ctx context.Context) context.Context { return ctx },
			table: "network_ix_lans",
			pred:  netIxLanUpsertPredicate,
			want: "(excluded.updated > `network_ix_lans`.`updated`" +
				" OR `network_ix_lans`.`updated` IS NULL OR `network_ix_lans`.`updated` <= '1900-01-01')" + tombstone,
		},
		{
			name: "organizations_full",
			ctx: func(ctx context.Context) context.Context {
				return withReconcileAll(ctx, map[string]time.Time{"organizations": cutoff})
			},
			table: "organizations",
			pred: func(ctx context.Context) *sql.Predicate {
				return skipUnchangedPredicate(ctx, migrate.OrganizationsTable)
			},
			want: "(excluded.updated >= `organizations`.`updated` OR `organizations`.`updated` < ?" +
				" OR `organizations`.`updated` IS NULL OR `organizations`.`updated` <= '1900-01-01')" +
				" AND (" + differs(migrate.OrganizationsTable) + ")",
		},
		{
			name:  "organizations_incremental",
			ctx:   func(ctx context.Context) context.Context { return ctx },
			table: "organizations",
			pred: func(ctx context.Context) *sql.Predicate {
				return skipUnchangedPredicate(ctx, migrate.OrganizationsTable)
			},
			want: "excluded.updated > `organizations`.`updated`" +
				" OR `organizations`.`updated` IS NULL OR `organizations`.`updated` <= '1900-01-01'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := where(t, tc.table, tc.pred(tc.ctx(t.Context()))); got != tc.want {
				t.Errorf("WHERE\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

// TestUpsert_NilValueClearsStoredValue upserts rows in batches of one,
// first with optional values set and then with them nil. The nil values
// must clear the stored ones. Before resolveWithRow, a column that was
// nil on every row of a batch was left out of the INSERT, and ON
// CONFLICT kept the stored value.
func TestUpsert_NilValueClearsStoredValue(t *testing.T) {
	t.Parallel()
	u := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	later := u.Add(time.Second)

	upsert := func(t *testing.T, client *ent.Client, fn func(context.Context, *ent.Tx) error) {
		t.Helper()
		ctx := t.Context()
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		if err := fn(ctx, tx); err != nil {
			_ = tx.Rollback()
			t.Fatalf("upsert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	// seedSide creates campus 1 and facility 1, the targets of the
	// campus and side FKs.
	seedSide := func(t *testing.T, client *ent.Client) {
		t.Helper()
		ctx := t.Context()
		client.Campus.Create().SetID(1).SetOrgID(1).SetName("Campus").SetNameFold("campus").
			SetStatus("ok").SetCreated(u).SetUpdated(u).SaveX(ctx)
		client.Facility.Create().SetID(1).SetOrgID(1).SetName("Fac").SetNameFold("fac").
			SetStatus("ok").SetCreated(u).SetUpdated(u).SaveX(ctx)
	}

	t.Run("netixlan", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNetIxLanGateParents(t, client, u)
		seedSide(t, client)
		row := func(updated time.Time, v4, v6 *string, side *int) peeringdb.NetworkIxLan {
			return peeringdb.NetworkIxLan{
				ID: 9120, NetID: 1, IXID: 1, IXLanID: 1, Name: "IX", Speed: 1, ASN: 64500,
				IPAddr4: v4, IPAddr6: v6, NetSideID: side, IXSideID: side,
				Created: u, Updated: updated, Status: "ok",
			}
		}
		v4, v6, side := "192.0.2.1", "2001:db8::1", 1
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworkIxLans(ctx, tx, []peeringdb.NetworkIxLan{row(u, &v4, &v6, &side)})
			return err
		})
		if got := client.NetworkIxLan.GetX(t.Context(), 9120); got.Ipaddr6 == nil || got.NetSideID == nil {
			t.Fatalf("seed: ipaddr6=%v net_side_id=%v, want both set", got.Ipaddr6, got.NetSideID)
		}
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworkIxLans(ctx, tx, []peeringdb.NetworkIxLan{row(later, nil, nil, nil)})
			return err
		})
		got := client.NetworkIxLan.GetX(t.Context(), 9120)
		if got.Ipaddr4 != nil || got.Ipaddr6 != nil || got.NetSideID != nil || got.IxSideID != nil {
			t.Errorf("ipaddr4=%v ipaddr6=%v net_side_id=%v ix_side_id=%v, want all nil",
				got.Ipaddr4, got.Ipaddr6, got.NetSideID, got.IxSideID)
		}
	})

	t.Run("network", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNetIxLanGateParents(t, client, u)
		row := func(updated time.Time, rir *string, rirUpdated *time.Time, prefixes *int) peeringdb.Network {
			return peeringdb.Network{
				ID: 1, OrgID: 1, Name: "Net", ASN: 64500,
				RIRStatus: rir, RIRStatusUpdated: rirUpdated, InfoPrefixes4: prefixes,
				Created: u, Updated: updated, Status: "ok",
			}
		}
		rir, prefixes := "ok", 10
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworks(ctx, tx, []peeringdb.Network{row(later, &rir, &u, &prefixes)})
			return err
		})
		if got := client.Network.GetX(t.Context(), 1); got.RirStatus == nil {
			t.Fatal("seed: rir_status nil, want set")
		}
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworks(ctx, tx, []peeringdb.Network{row(later.Add(time.Second), nil, nil, nil)})
			return err
		})
		got := client.Network.GetX(t.Context(), 1)
		if got.RirStatus != nil || got.RirStatusUpdated != nil || got.InfoPrefixes4 != nil {
			t.Errorf("rir_status=%v rir_status_updated=%v info_prefixes4=%v, want all nil",
				got.RirStatus, got.RirStatusUpdated, got.InfoPrefixes4)
		}
	})

	t.Run("facility", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		seedNetIxLanGateParents(t, client, u)
		seedSide(t, client)
		row := func(updated time.Time, campusID *int, lat *float64) peeringdb.Facility {
			return peeringdb.Facility{
				ID: 1, OrgID: 1, Name: "Fac", CampusID: campusID, Latitude: lat, Longitude: lat,
				Created: u, Updated: updated, Status: "ok",
			}
		}
		campusID, lat := 1, 51.5
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertFacilities(ctx, tx, []peeringdb.Facility{row(later, &campusID, &lat)})
			return err
		})
		if got := client.Facility.GetX(t.Context(), 1); got.CampusID == nil {
			t.Fatal("seed: campus_id nil, want set")
		}
		upsert(t, client, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertFacilities(ctx, tx, []peeringdb.Facility{row(later.Add(time.Second), nil, nil)})
			return err
		})
		got := client.Facility.GetX(t.Context(), 1)
		if got.CampusID != nil || got.Latitude != nil || got.Longitude != nil {
			t.Errorf("campus_id=%v latitude=%v longitude=%v, want all nil",
				got.CampusID, got.Latitude, got.Longitude)
		}
	})
}

// TestUpsert_ConflictSetsEveryColumn runs each entity upsert with one row
// whose optional values are nil, and reads the statement it sends. The
// ON CONFLICT DO UPDATE must set every column of the table except the
// primary key, including the columns that the INSERT leaves out. In full
// mode, its WHERE must compare each of those columns, so a row that
// differs only in one column is still written.
func TestUpsert_ConflictSetsEveryColumn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		table  *schema.Table
		upsert func(context.Context, *ent.Tx) error
	}{
		{migrate.OrganizationsTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertOrganizations(ctx, tx, []peeringdb.Organization{{ID: 1}})
			return err
		}},
		{migrate.CampusesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertCampuses(ctx, tx, []peeringdb.Campus{{ID: 1}})
			return err
		}},
		{migrate.FacilitiesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertFacilities(ctx, tx, []peeringdb.Facility{{ID: 1}})
			return err
		}},
		{migrate.CarriersTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertCarriers(ctx, tx, []peeringdb.Carrier{{ID: 1}})
			return err
		}},
		{migrate.CarrierFacilitiesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertCarrierFacilities(ctx, tx, []peeringdb.CarrierFacility{{ID: 1}})
			return err
		}},
		{migrate.InternetExchangesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertInternetExchanges(ctx, tx, []peeringdb.InternetExchange{{ID: 1}})
			return err
		}},
		{migrate.IxLansTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertIxLans(ctx, tx, []peeringdb.IxLan{{ID: 1}})
			return err
		}},
		{migrate.IxPrefixesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertIxPrefixes(ctx, tx, []peeringdb.IxPrefix{{ID: 1, Prefix: "192.0.2.0/24", Protocol: "IPv4"}})
			return err
		}},
		{migrate.IxFacilitiesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertIxFacilities(ctx, tx, []peeringdb.IxFacility{{ID: 1}})
			return err
		}},
		{migrate.NetworksTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworks(ctx, tx, []peeringdb.Network{{ID: 1, Name: "Net", ASN: 64500}})
			return err
		}},
		{migrate.PocsTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertPocs(ctx, tx, []peeringdb.Poc{{ID: 1, Role: "Abuse", Visible: "Public"}})
			return err
		}},
		{migrate.NetworkFacilitiesTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworkFacilities(ctx, tx, []peeringdb.NetworkFacility{{ID: 1}})
			return err
		}},
		{migrate.NetworkIxLansTable, func(ctx context.Context, tx *ent.Tx) error {
			_, err := upsertNetworkIxLans(ctx, tx, []peeringdb.NetworkIxLan{{ID: 1, ASN: 64500, Speed: 1}})
			return err
		}},
	} {
		t.Run(tc.table.Name, func(t *testing.T) {
			t.Parallel()
			stmt := captureUpsert(t, func(ctx context.Context, tx *ent.Tx) error {
				return tc.upsert(withReconcileAll(ctx, nil), tx)
			})
			_, set, ok := strings.Cut(stmt, " DO UPDATE SET ")
			if !ok {
				t.Fatalf("no DO UPDATE SET in %q", stmt)
			}
			set, where, _ := strings.Cut(set, " WHERE ")
			got := map[string]bool{}
			for assignment := range strings.SplitSeq(set, ", ") {
				col, _, _ := strings.Cut(assignment, " = ")
				got[strings.Trim(col, "`")] = true
			}
			compared := map[string]bool{}
			for _, m := range comparedColumn.FindAllStringSubmatch(where, -1) {
				if m[1] != m[2] {
					t.Errorf("excluded.%s compared with %s", m[1], m[2])
				}
				compared[m[1]] = true
			}
			for _, c := range tc.table.Columns {
				want := !slices.Contains(tc.table.PrimaryKey, c)
				if got[c.Name] != want {
					t.Errorf("column %s set=%v, want %v", c.Name, got[c.Name], want)
				}
				if compared[c.Name] != want {
					t.Errorf("column %s compared=%v, want %v", c.Name, compared[c.Name], want)
				}
			}
		})
	}
}

// comparedColumn matches one column comparison of the full-mode WHERE.
var comparedColumn = regexp.MustCompile("excluded\\.`(\\w+)` IS NOT `\\w+`\\.`(\\w+)`")

// captureUpsert runs upsert in a transaction on a new database and
// returns the INSERT statement it sends. Foreign keys are off, so the
// rows need no parents; the transaction is rolled back.
func captureUpsert(t *testing.T, upsert func(context.Context, *ent.Tx) error) string {
	t.Helper()
	ctx := t.Context()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared&_pragma=foreign_keys(1)"
	db, err := stdsql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// One connection, so the pragma below applies to the transaction.
	db.SetMaxOpenConns(1)
	var stmt string
	drv := dialect.DebugWithContext(sql.OpenDB(dialect.SQLite, db), func(_ context.Context, v ...any) {
		if s := fmt.Sprint(v...); strings.Contains(s, "INSERT INTO") {
			stmt = s
		}
	})
	client := ent.NewClient(ent.Driver(drv))
	// Migrate a copy: the migration writes to the tables it gets, and
	// the subtests run in parallel (enttest does the same).
	tables, err := schema.CopyTables(migrate.Tables)
	if err != nil {
		t.Fatalf("copy tables: %v", err)
	}
	if err := migrate.Create(ctx, client.Schema, tables); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("foreign keys off: %v", err)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("open tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsert(ctx, tx); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	stmt, _, _ = strings.Cut(stmt, " args=")
	return stmt
}
