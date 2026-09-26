package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// scrubLogMsg is the log message of logScrubbedPocContacts.
const scrubLogMsg = "scrubbed contact fields of deleted pocs"

// legacyPocTime is the created/updated time of the seeded legacy rows.
// No test fixture sends these pocs, so a sync cycle never rewrites them.
// It is within pocDeletionPeriod of now, so purgeDeletedPocs keeps the
// deleted rows.
var legacyPocTime = time.Now().UTC().Add(-7 * 24 * time.Hour).Truncate(time.Second)

// seedLegacyPocs writes pocs straight to the table, the way the removed
// inference-by-absence code left them: status "deleted" with contact
// data. The rows, by id:
//
//	10  deleted, all four contact fields set
//	11  deleted, already blank
//	12  ok, all four contact fields set
//	13  deleted, only email set
func seedLegacyPocs(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := t.Context()
	client.Organization.Create().SetID(900).SetName("LegacyOrg").SetNameFold("legacyorg").
		SetStatus("ok").SetCreated(legacyPocTime).SetUpdated(legacyPocTime).SaveX(ctx)
	client.Network.Create().SetID(900).SetOrgID(900).SetName("LegacyNet").SetNameFold("legacynet").
		SetAsn(64900).SetStatus("ok").SetCreated(legacyPocTime).SetUpdated(legacyPocTime).SaveX(ctx)
	for _, p := range []struct {
		id                       int
		status                   string
		name, phone, email, link string
	}{
		{10, "deleted", "Jane Doe", "+1 555 0100", "jane@example.invalid", "https://example.invalid/jane"},
		{11, "deleted", "", "", "", ""},
		{12, "ok", "Jim Roe", "+1 555 0101", "jim@example.invalid", "https://example.invalid/jim"},
		{13, "deleted", "", "", "noc@example.invalid", ""},
	} {
		client.Poc.Create().
			SetID(p.id).SetNetID(900).SetRole("NOC").SetVisible("Public").
			SetName(p.name).SetPhone(p.phone).SetEmail(p.email).SetURL(p.link).
			SetStatus(p.status).SetCreated(legacyPocTime).SetUpdated(legacyPocTime).
			SaveX(ctx)
	}
}

// assertPocContact fails the test when the stored contact fields of poc id
// do not match want (name, phone, email, url), or when its updated value
// moved.
func assertPocContact(t *testing.T, client *ent.Client, id int, want [4]string) {
	t.Helper()
	got, err := client.Poc.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("read poc %d: %v", id, err)
	}
	if have := [4]string{got.Name, got.Phone, got.Email, got.URL}; have != want {
		t.Errorf("poc %d contact = %q, want %q", id, have, want)
	}
	if got.Role != "NOC" || got.Visible != "Public" {
		t.Errorf("poc %d role=%q visible=%q, want NOC, Public", id, got.Role, got.Visible)
	}
	if !got.Updated.Equal(legacyPocTime) {
		t.Errorf("poc %d updated = %v, want %v (the scrub must not move the cursor)", id, got.Updated, legacyPocTime)
	}
}

// scrubLogRecords decodes the JSON log lines that logScrubbedPocContacts
// wrote to buf.
func scrubLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if rec["msg"] == scrubLogMsg {
			out = append(out, rec)
		}
	}
	return out
}

// TestScrubDeletedPocContacts verifies the data repair: it blanks the
// contact fields of each deleted poc that holds any, leaves live pocs and
// updated alone, and changes nothing on a second run. The count is logged
// after the commit, as the callers do: at WARN, then at DEBUG with count
// 0.
func TestScrubDeletedPocContacts(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	seedLegacyPocs(t, client)

	run := func() (int, []map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		n, err := scrubDeletedPocContacts(ctx, tx)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("scrub: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		logScrubbedPocContacts(ctx, logger, n)
		return n, scrubLogRecords(t, &buf)
	}

	n, logs := run()
	if n != 2 {
		t.Errorf("first run changed %d rows, want 2 (ids 10 and 13)", n)
	}
	if len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["count"] != float64(2) {
		t.Errorf("first run log = %v, want one WARN record with count 2", logs)
	}
	blank := [4]string{}
	assertPocContact(t, client, 10, blank)
	assertPocContact(t, client, 11, blank)
	assertPocContact(t, client, 12, [4]string{"Jim Roe", "+1 555 0101", "jim@example.invalid", "https://example.invalid/jim"})
	assertPocContact(t, client, 13, blank)

	n, logs = run()
	if n != 0 {
		t.Errorf("second run changed %d rows, want 0 (idempotent)", n)
	}
	if len(logs) != 1 || logs[0]["level"] != "DEBUG" || logs[0]["count"] != float64(0) {
		t.Errorf("second run log = %v, want one DEBUG record with count 0", logs)
	}
}

// TestSync_ScrubsLegacyPocTombstones verifies that a sync cycle runs the
// repair: legacy tombstones that upstream no longer sends lose their
// contact data, and a live poc keeps it. A cycle whose commit fails
// changes no poc and does not log the scrub. The next cycle logs the
// count after its commit.
func TestSync_ScrubsLegacyPocTombstones(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, db := newTestWorker(t, f)
	probe := withCommitProbe(w, db)
	seedLegacyPocs(t, w.entClient)
	jim := [4]string{"Jim Roe", "+1 555 0101", "jim@example.invalid", "https://example.invalid/jim"}

	probe.fail.Store(true)
	if err := w.Sync(t.Context(), config.SyncModeIncremental); !errors.Is(err, errInjectedCommit) {
		t.Fatalf("sync error = %v, want the injected commit failure", err)
	}
	assertPocContact(t, w.entClient, 10, [4]string{"Jane Doe", "+1 555 0100", "jane@example.invalid", "https://example.invalid/jane"})
	if recs := probe.logs.records(t, scrubLogMsg); len(recs) != 0 {
		t.Errorf("scrub log after the failed commit = %v, want none", recs)
	}

	probe.fail.Store(false)
	if err := w.Sync(t.Context(), config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	blank := [4]string{}
	assertPocContact(t, w.entClient, 10, blank)
	assertPocContact(t, w.entClient, 12, jim)
	assertPocContact(t, w.entClient, 13, blank)
	if recs := probe.logsAtCommit().records(t, scrubLogMsg); len(recs) != 0 {
		t.Errorf("scrub log before the commit = %v, want none", recs)
	}
	if recs := probe.logs.records(t, scrubLogMsg); len(recs) != 1 || recs[0]["level"] != "WARN" || recs[0]["count"] != float64(2) {
		t.Errorf("scrub log = %v, want one WARN record with count 2", recs)
	}
}

// pocContactSQL reads the stored contact fields of poc id with plain SQL,
// so the poc privacy policy cannot hide the row from the test.
func pocContactSQL(t *testing.T, db *sql.DB, id int) [4]string {
	t.Helper()
	var c [4]string
	if err := db.QueryRowContext(t.Context(),
		"SELECT name, phone, email, url FROM pocs WHERE id = ?", id,
	).Scan(&c[0], &c[1], &c[2], &c[3]); err != nil {
		t.Fatalf("read poc %d: %v", id, err)
	}
	return c
}

// TestStartScheduler_ScrubsPocTombstonesAtStartup verifies that a primary
// runs the repair when the scheduler starts, before the first sync cycle
// is due. The last sync is recent, so no cycle runs in the test. Poc 14
// is Private, which the query policy hides, so the test checks that the
// UPDATE reaches it without a privacy bypass.
func TestStartScheduler_ScrubsPocTombstonesAtStartup(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, db := newTestWorker(t, f)
	var buf bytes.Buffer
	w.logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	seedLegacyPocs(t, w.entClient)
	w.entClient.Poc.Create().
		SetID(14).SetNetID(900).SetRole("NOC").SetVisible("Private").
		SetName("Pat Private").SetEmail("pat@example.invalid").
		SetStatus("deleted").SetCreated(legacyPocTime).SetUpdated(legacyPocTime).
		SaveX(t.Context())

	now := time.Now()
	id, err := RecordSyncStart(t.Context(), db, now.Add(-10*time.Minute), "incremental")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := RecordSyncComplete(t.Context(), db, id, Status{
		LastSyncAt: now.Add(-10 * time.Minute), Duration: time.Second, Status: "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, time.Hour)
		close(done)
	}()

	// The startup scrub sends no completion event, so wait until its
	// transaction commits. A read error means "not yet": the scrub
	// transaction can lock the shared-cache table. The deadline only
	// bounds a broken scrub.
	scrubbed := func() bool {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM pocs WHERE id = 10").Scan(&name)
		return err == nil && name == ""
	}
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !scrubbed() {
		select {
		case <-tick.C:
		case <-done:
			t.Fatal("scheduler exited before the startup poc scrub committed")
		case <-deadline.C:
			cancel()
			<-done
			t.Fatal("startup poc scrub did not commit before the deadline")
		}
	}
	cancel()
	<-done

	if calls := f.callCount.Load(); calls != 0 {
		t.Errorf("got %d upstream calls, want 0 (no sync cycle is due)", calls)
	}
	blank := [4]string{}
	for _, id := range []int{10, 11, 13, 14} {
		if got := pocContactSQL(t, db, id); got != blank {
			t.Errorf("poc %d contact = %q, want blank", id, got)
		}
	}
	if got, want := pocContactSQL(t, db, 12), [4]string{"Jim Roe", "+1 555 0101", "jim@example.invalid", "https://example.invalid/jim"}; got != want {
		t.Errorf("live poc 12 contact = %q, want %q", got, want)
	}
	if logs := scrubLogRecords(t, &buf); len(logs) != 1 || logs[0]["level"] != "WARN" || logs[0]["count"] != float64(3) {
		t.Errorf("scrub log = %v, want one WARN record with count 3 (ids 10, 13, 14)", logs)
	}
	if w.Running() {
		t.Error("running latch still held after the startup scrub")
	}
}

// TestScrubPocContactsAtStartup_CommitFails verifies that a startup scrub
// whose commit fails keeps the contact data and logs only the failure.
// The next run logs the count after its commit.
func TestScrubPocContactsAtStartup_CommitFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, db := newTestWorker(t, f)
	probe := withCommitProbe(w, db)
	seedLegacyPocs(t, w.entClient)

	probe.fail.Store(true)
	w.scrubPocContactsAtStartup(t.Context())
	if got, want := pocContactSQL(t, db, 10), [4]string{"Jane Doe", "+1 555 0100", "jane@example.invalid", "https://example.invalid/jane"}; got != want {
		t.Errorf("poc 10 contact = %q after the failed commit, want %q", got, want)
	}
	if recs := probe.logs.records(t, scrubLogMsg); len(recs) != 0 {
		t.Errorf("scrub log after the failed commit = %v, want none", recs)
	}
	recs := probe.logs.records(t, "startup poc contact scrub failed, the next sync cycle retries it")
	if len(recs) != 1 || recs[0]["level"] != "WARN" ||
		!strings.Contains(fmt.Sprint(recs[0]["error"]), errInjectedCommit.Error()) {
		t.Errorf("failure log = %v, want one WARN record with the injected commit error", recs)
	}

	probe.fail.Store(false)
	w.scrubPocContactsAtStartup(t.Context())
	if got := pocContactSQL(t, db, 10); got != [4]string{} {
		t.Errorf("poc 10 contact = %q after the retry, want blank", got)
	}
	if recs := probe.logsAtCommit().records(t, scrubLogMsg); len(recs) != 0 {
		t.Errorf("scrub log before the commit = %v, want none", recs)
	}
	if recs := probe.logs.records(t, scrubLogMsg); len(recs) != 1 || recs[0]["level"] != "WARN" || recs[0]["count"] != float64(2) {
		t.Errorf("scrub log = %v, want one WARN record with count 2", recs)
	}
	if w.Running() {
		t.Error("running latch still held after the startup scrub")
	}
}

// TestStartScheduler_ReplicaSkipsPocScrub verifies that a replica does not
// write: its database is read-only, and the primary runs the repair.
func TestStartScheduler_ReplicaSkipsPocScrub(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, db := newTestWorker(t, f)
	w.config.IsPrimary = func() bool { return false }
	seedLegacyPocs(t, w.entClient)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	w.StartScheduler(ctx, time.Hour)

	if got, want := pocContactSQL(t, db, 10), [4]string{"Jane Doe", "+1 555 0100", "jane@example.invalid", "https://example.invalid/jane"}; got != want {
		t.Errorf("poc 10 contact = %q, want %q (a replica must not write)", got, want)
	}
}

// TestScrubPocContactsAtStartup_SkipsWhileSyncRuns verifies that the
// startup run does not overlap a sync cycle. The cycle holds the running
// latch and runs the repair itself.
func TestScrubPocContactsAtStartup_SkipsWhileSyncRuns(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	w, db := newTestWorker(t, f)
	seedLegacyPocs(t, w.entClient)

	w.running.Store(true)
	w.scrubPocContactsAtStartup(t.Context())

	if !w.running.Load() {
		t.Error("the skipped startup scrub released the latch of the running cycle")
	}
	if got := pocContactSQL(t, db, 10); got == [4]string{} {
		t.Error("poc 10 was scrubbed while a sync cycle held the latch")
	}
}
