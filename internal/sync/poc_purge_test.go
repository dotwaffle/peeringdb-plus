package sync

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// purgeLogMsg is the log message of logPurgedPocs.
const purgeLogMsg = "purged deleted pocs"

// seedPurgePoc writes a poc of net 900 (see seedLegacyPocs) straight to
// the table.
func seedPurgePoc(t *testing.T, client *ent.Client, id int, status, visible string, updated time.Time) {
	t.Helper()
	client.Poc.Create().
		SetID(id).SetNetID(900).SetRole("NOC").SetVisible(visible).
		SetStatus(status).SetCreated(updated).SetUpdated(updated).
		SaveX(t.Context())
}

// storedPocIDs returns the ids of all stored pocs with plain SQL, so the
// poc privacy policy cannot hide a row from the test.
func storedPocIDs(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT id FROM pocs ORDER BY id")
	if err != nil {
		t.Fatalf("read poc ids: %v", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan poc id: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read poc ids: %v", err)
	}
	return ids
}

// TestPurgeDeletedPocs verifies the upstream pdb_delete_pocs rule: a poc
// with status "deleted" whose updated value is pocDeletionPeriod or more
// before now is deleted, at every visibility. A newer tombstone and a
// live poc of any age stay. The count is logged after the commit, as the
// caller does: at INFO, then at DEBUG with count 0.
func TestPurgeDeletedPocs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client, db := testutil.SetupClientWithDB(t)
	seedLegacyPocs(t, client)
	// legacyPocTime is relative to the wall clock, so now is too.
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-pocDeletionPeriod)
	seedPurgePoc(t, client, 20, "deleted", "Public", cutoff.Add(-24*time.Hour))
	seedPurgePoc(t, client, 21, "deleted", "Public", cutoff)
	seedPurgePoc(t, client, 22, "deleted", "Public", cutoff.Add(time.Second))
	seedPurgePoc(t, client, 23, "deleted", "Private", cutoff.Add(-time.Hour))
	seedPurgePoc(t, client, 24, "ok", "Public", cutoff.Add(-90*24*time.Hour))

	run := func() (int, []map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("open tx: %v", err)
		}
		// The zone and the sub-second part of now must not change the
		// cutoff: the DELETE compares the stored text form.
		local := now.Add(500 * time.Millisecond).In(time.FixedZone("UTC-5", -5*60*60))
		n, err := purgeDeletedPocs(ctx, tx, local)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("purge: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
		logPurgedPocs(ctx, logger, n)
		return n, purgeLogRecords(t, &buf)
	}

	n, logs := run()
	if n != 3 {
		t.Errorf("first run deleted %d rows, want 3 (ids 20, 21 and 23)", n)
	}
	if len(logs) != 1 || logs[0]["level"] != "INFO" || logs[0]["count"] != float64(3) {
		t.Errorf("first run log = %v, want one INFO record with count 3", logs)
	}
	// 10, 11 and 13 are deleted, but within the period (legacyPocTime).
	if got, want := storedPocIDs(t, db), []int{10, 11, 12, 13, 22, 24}; !slices.Equal(got, want) {
		t.Errorf("stored pocs = %v, want %v", got, want)
	}

	n, logs = run()
	if n != 0 {
		t.Errorf("second run deleted %d rows, want 0", n)
	}
	if len(logs) != 1 || logs[0]["level"] != "DEBUG" || logs[0]["count"] != float64(0) {
		t.Errorf("second run log = %v, want one DEBUG record with count 0", logs)
	}
}

// purgeLogRecords decodes the JSON log lines that logPurgedPocs wrote to
// buf.
func purgeLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if rec["msg"] == purgeLogMsg {
			out = append(out, rec)
		}
	}
	return out
}

// TestSync_PurgesDeletedPocs verifies that a sync cycle runs the purge:
// a stored tombstone older than the period and one that the cycle lands
// are deleted, and a newer tombstone stays. A cycle whose commit fails
// deletes no poc and does not log the purge. The next cycle logs the
// count after its commit.
func TestSync_PurgesDeletedPocs(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	old := time.Now().UTC().Add(-pocDeletionPeriod - 24*time.Hour).Truncate(time.Second)
	f.responses["poc"] = []any{
		bumpUpdated(makePoc(31, 900, "", "", "deleted"), old.Format(time.RFC3339)),
	}
	w, db := newTestWorker(t, f)
	probe := withCommitProbe(w, db)
	seedLegacyPocs(t, w.entClient)
	seedPurgePoc(t, w.entClient, 30, "deleted", "Public", old)

	probe.fail.Store(true)
	if err := w.Sync(t.Context(), config.SyncModeIncremental); !errors.Is(err, errInjectedCommit) {
		t.Fatalf("sync error = %v, want the injected commit failure", err)
	}
	if got, want := storedPocIDs(t, db), []int{10, 11, 12, 13, 30}; !slices.Equal(got, want) {
		t.Errorf("stored pocs after the failed commit = %v, want %v", got, want)
	}
	if recs := probe.logs.records(t, purgeLogMsg); len(recs) != 0 {
		t.Errorf("purge log after the failed commit = %v, want none", recs)
	}

	probe.fail.Store(false)
	if err := w.Sync(t.Context(), config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, want := storedPocIDs(t, db), []int{10, 11, 12, 13}; !slices.Equal(got, want) {
		t.Errorf("stored pocs = %v, want %v (30 and the landed 31 purged)", got, want)
	}
	if recs := probe.logsAtCommit().records(t, purgeLogMsg); len(recs) != 0 {
		t.Errorf("purge log before the commit = %v, want none", recs)
	}
	if recs := probe.logs.records(t, purgeLogMsg); len(recs) != 1 || recs[0]["level"] != "INFO" || recs[0]["count"] != float64(2) {
		t.Errorf("purge log = %v, want one INFO record with count 2", recs)
	}
}
