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
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestUpsert_UnDeletesOnResync verifies the deleted→ok transition: when
// upstream re-delivers a row that had been tombstoned, with a newer
// `updated` timestamp and status "ok", the OnConflict UpdateNewValues
// path flips the row back to "ok" (auto-undelete). The same row resent
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
	// same fold values (OnConflictColumns().UpdateNewValues() path rewrites
	// the _fold columns on every cycle).
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
// The predicate of the other tables does not change.
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
			want: "(excluded.updated >= `network_ix_lans`.`updated` OR `network_ix_lans`.`updated` < ?" +
				" OR `network_ix_lans`.`updated` IS NULL OR `network_ix_lans`.`updated` <= '1900-01-01')" + tombstone,
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
			pred:  func(ctx context.Context) *sql.Predicate { return skipUnchangedPredicate(ctx, "organizations") },
			want: "excluded.updated >= `organizations`.`updated` OR `organizations`.`updated` < ?" +
				" OR `organizations`.`updated` IS NULL OR `organizations`.`updated` <= '1900-01-01'",
		},
		{
			name:  "organizations_incremental",
			ctx:   func(ctx context.Context) context.Context { return ctx },
			table: "organizations",
			pred:  func(ctx context.Context) *sql.Predicate { return skipUnchangedPredicate(ctx, "organizations") },
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
