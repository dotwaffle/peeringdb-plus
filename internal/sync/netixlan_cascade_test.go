package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// cascadeLogMsg is the log message of recordCascadeCommitted.
const cascadeLogMsg = "cascaded network deletes to netixlans"

// startupCascadeFailedMsg is the log message of a startup cascade whose
// transaction fails.
const startupCascadeFailedMsg = "startup netixlan cascade failed, the next sync cycle retries it"

// runCascadeTx runs cascadeDeletedNetIxLans with p in a transaction of its
// own, commits it, and records the result as the callers do. The log goes
// to logs.
func runCascadeTx(t *testing.T, client *ent.Client, logs *logBuffer, p cascadePlan) cascadeResult {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	res, err := cascadeNetIxLansInTx(t.Context(), client, p)
	if err != nil {
		t.Fatalf("cascade: %v", err)
	}
	recordCascadeCommitted(t.Context(), logger, p.Mode, res)
	return res
}

// cascadedRow is the state of a seeded T0 row after the cascade.
var cascadedRow = netIxLanRow{Status: "deleted", Operational: false, Updated: cascadeRowTime, Speed: 1}

// cascadeChangeRecords returns the cascade log records in logs other
// than DEBUG records with count 0.
func cascadeChangeRecords(t *testing.T, logs *logBuffer) []map[string]any {
	t.Helper()
	var hits []map[string]any
	for _, rec := range logs.records(t, cascadeLogMsg) {
		if rec["count"] != float64(0) || rec["level"] != "DEBUG" {
			hits = append(hits, rec)
		}
	}
	return hits
}

// wantNoCascadeChangeLog fails the test when logs hold a cascade record
// other than a DEBUG record with count 0.
func wantNoCascadeChangeLog(t *testing.T, logs *logBuffer) {
	t.Helper()
	if hits := cascadeChangeRecords(t, logs); len(hits) != 0 {
		t.Errorf("cascade log records with changes = %v, want none", hits)
	}
}

// wantCascadeLog fails the test unless logs hold exactly one cascade
// record at level with the given counts, among any number of DEBUG
// records with count 0.
func wantCascadeLog(t *testing.T, logs *logBuffer, level, mode string, count, nets, backlog int) {
	t.Helper()
	hits := cascadeChangeRecords(t, logs)
	if len(hits) != 1 {
		t.Fatalf("cascade log records with changes = %v, want one", hits)
	}
	rec := hits[0]
	if rec["level"] != level || rec["mode"] != mode || rec["count"] != float64(count) ||
		rec["nets"] != float64(nets) || rec["backlog"] != float64(backlog) {
		t.Errorf("cascade log = %v, want level=%s mode=%s count=%d nets=%d backlog=%d",
			rec, level, mode, count, nets, backlog)
	}
}

// TestCascadeDeletedNetIxLans verifies the cascade of verified ids: live
// rows of a deleted network whose updated is not later than the
// network's become deleted with operational false and the same updated.
// A row of a live network and a row newer than its network's delete do
// not change. A second run changes nothing, and an empty plan runs no
// statement.
func TestCascadeDeletedNetIxLans(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seedCascadeRows(t, client)
	before := allNetIxLans(t, client)
	logs := &logBuffer{}

	plan := cascadePlan{Mode: "incremental", Gone: []int{9100, 9110, 9111, 9114, 9120}}
	if got, want := runCascadeTx(t, client, logs, plan), (cascadeResult{Rows: 3, Nets: 2, Backlog: 3}); got != want {
		t.Errorf("first run = %+v, want %+v", got, want)
	}
	want := maps.Clone(before)
	for _, id := range []int{9110, 9111, 9120} {
		want[id] = cascadedRow
	}
	if got := allNetIxLans(t, client); !maps.Equal(got, want) {
		t.Errorf("rows after the cascade:\n got %v\nwant %v", got, want)
	}
	wantCascadeLog(t, logs, "WARN", "incremental", 3, 2, 3)

	if got := runCascadeTx(t, client, logs, plan); got != (cascadeResult{}) {
		t.Errorf("second run = %+v, want zero", got)
	}
	recs := logs.records(t, cascadeLogMsg)
	if last := recs[len(recs)-1]; last["level"] != "DEBUG" || last["count"] != float64(0) {
		t.Errorf("second run log = %v, want DEBUG with count 0", last)
	}

	// A statement on a finished transaction fails, so a nil error shows
	// that the empty plan ran none.
	tx, err := client.Tx(t.Context())
	if err != nil {
		t.Fatalf("open tx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if res, err := cascadeDeletedNetIxLans(t.Context(), tx, cascadePlan{Mode: "incremental"}); err != nil || res != (cascadeResult{}) {
		t.Errorf("empty plan = %+v, %v; want zero and no error", res, err)
	}
}

// TestCascadeDeletedNetIxLans_RIRNets verifies the cascade by network:
// the live rows of a network in RIRNets whose updated is not later than
// the network's become deleted, and they do not count as backlog. Rows
// of other networks and a row newer than the network's delete do not
// change.
func TestCascadeDeletedNetIxLans_RIRNets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		plan    cascadePlan
		flipped []int
		want    cascadeResult
	}{
		{"rir_nets", cascadePlan{Mode: "incremental", RIRNets: []int{911}},
			[]int{9110, 9111}, cascadeResult{Rows: 2, Nets: 1, Backlog: 0}},
		{"rir_nets_and_gone", cascadePlan{Mode: "incremental", Gone: []int{9120}, RIRNets: []int{911}},
			[]int{9110, 9111, 9120}, cascadeResult{Rows: 3, Nets: 2, Backlog: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := testutil.SetupClient(t)
			seedCascadeRows(t, client)
			before := allNetIxLans(t, client)
			logs := &logBuffer{}

			if got := runCascadeTx(t, client, logs, tc.plan); got != tc.want {
				t.Errorf("cascade = %+v, want %+v", got, tc.want)
			}
			want := maps.Clone(before)
			for _, id := range tc.flipped {
				want[id] = cascadedRow
			}
			if got := allNetIxLans(t, client); !maps.Equal(got, want) {
				t.Errorf("rows after the cascade:\n got %v\nwant %v", got, want)
			}
			wantCascadeLog(t, logs, "WARN", "incremental", tc.want.Rows, tc.want.Nets, tc.want.Backlog)
		})
	}
}

// rirTombstoneNet returns an upstream tombstone of network id, updated at
// updated, with the signature of the pdb_rir_status reclaim: rir_status
// null and rir_status_updated set.
func rirTombstoneNet(id int, updated time.Time) map[string]any {
	row := bumpUpdated(makeNet(id, 910, 64000+id, "CascadeNet"+strconv.Itoa(id), "deleted"),
		updated.Format(time.RFC3339))
	row["rir_status"] = nil
	row["rir_status_updated"] = cascadeT(-24 * 30).Format(time.RFC3339)
	return row
}

// stageScratchNets stores rows in the "net" table of scratch.
func stageScratchNets(t *testing.T, s *scratchDB, rows ...map[string]any) {
	t.Helper()
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("marshal net %v: %v", row["id"], err)
		}
		if _, err := s.db.ExecContext(t.Context(), `INSERT INTO "net" (id, data) VALUES (?, ?)`, row["id"], data); err != nil {
			t.Fatalf("stage net %v: %v", row["id"], err)
		}
	}
}

// TestRIRTransitionNets verifies the networks that class A acts on: a
// scratch tombstone with the reclaim signature whose network is not
// deleted in the committed DB, or not stored at all. A tombstone of a
// network that is already deleted (re-saved upstream) does not qualify,
// and neither does a row without the exact signature. An empty scratch
// sends no query to the committed DB.
func TestRIRTransitionNets(t *testing.T) {
	t.Parallel()
	scratchRows := func() []map[string]any {
		noRIRStatus := rirTombstoneNet(917, cascadeT(48))
		delete(noRIRStatus, "rir_status")
		rirOK := rirTombstoneNet(913, cascadeT(48))
		rirOK["rir_status"] = "ok"
		noUpdated := rirTombstoneNet(914, cascadeT(48))
		noUpdated["rir_status_updated"] = nil
		live := rirTombstoneNet(916, cascadeT(48))
		live["status"] = "ok"
		return []map[string]any{
			rirTombstoneNet(911, cascadeT(48)),
			rirOK,
			noUpdated,
			rirTombstoneNet(915, cascadeT(48)), // not stored
			live,
			noRIRStatus,
		}
	}

	for _, tc := range []struct {
		name      string
		stored911 string
		want      []int
	}{
		{"transition", "ok", []int{911, 915}},
		{"resaved_tombstone", "deleted", []int{915}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, db := testutil.SetupClientWithDB(t)
			seedCascadeParents(t, client)
			client.Network.UpdateOneID(911).SetStatus(tc.stored911).ExecX(t.Context())
			scratch := newCascadeScratch(t)
			stageScratchNets(t, scratch, scratchRows()...)

			got, err := rirTransitionNets(t.Context(), scratch, db)
			if err != nil {
				t.Fatalf("rirTransitionNets: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("rirTransitionNets = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("empty_scratch", func(t *testing.T) {
		t.Parallel()
		// A query on a closed DB fails, so a nil error shows that
		// rirTransitionNets sent none.
		closed, err := openScratchDB(t.Context(), "")
		if err != nil {
			t.Fatalf("open scratch: %v", err)
		}
		closeScratchDB(t.Context(), closed, nil)
		got, err := rirTransitionNets(t.Context(), newCascadeScratch(t), closed.db)
		if err != nil || got != nil {
			t.Errorf("rirTransitionNets = %v, %v; want nil and no error", got, err)
		}
	})
}

// TestJSONIDs locks the json_each argument: an empty or nil list must be
// "[]", because json_each reads "null" as one NULL row.
func TestJSONIDs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ids  []int
		want string
	}{
		{nil, "[]"},
		{[]int{}, "[]"},
		{[]int{7}, "[7]"},
		{[]int{1, 2}, "[1,2]"},
	} {
		if got := jsonIDs(tc.ids); got != tc.want {
			t.Errorf("jsonIDs(%v) = %q, want %q", tc.ids, got, tc.want)
		}
	}
}

// TestNetIxLanCascadePlans locks the query plans of the cascade SQL. The
// candidate read and the cascade by network must reach netixlans through
// networkixlan_net_id, the cascade by id must update by primary key, and
// the network status read must look networks up by primary key. No plan
// may read the networkixlan_status index, which covers most of the table.
func TestNetIxLanCascadePlans(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	for _, tc := range []struct {
		name  string
		query string
		args  []any
		want  []string
	}{
		{"candidates", cascadeCandidatesSQL, nil,
			[]string{"COVERING INDEX network_status_updated_created_id", "INDEX networkixlan_net_id"}},
		{"verified", cascadeVerifiedSQL, []any{"[1,2]"},
			[]string{"network_ix_lans USING INTEGER PRIMARY KEY"}},
		{"rir_nets", cascadeRIRNetsSQL, []any{"[1,2]"},
			[]string{"INDEX networkixlan_net_id (net_id=?)"}},
		{"live_at_start_nets", liveAtStartNetsSQL, []any{"[1,2]"},
			[]string{"INTEGER PRIMARY KEY"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			plan := cascadeQueryPlan(t, db, tc.query, tc.args)
			for _, want := range tc.want {
				if !strings.Contains(plan, want) {
					t.Errorf("plan %q does not contain %q", plan, want)
				}
			}
			if strings.Contains(plan, "networkixlan_status") {
				t.Errorf("plan %q reads networkixlan_status", plan)
			}
		})
	}
}

// cascadeQueryPlan returns the EXPLAIN QUERY PLAN detail rows of q joined
// with " | ".
func cascadeQueryPlan(t *testing.T, db *sql.DB, q string, args []any) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(plan, " | ")
}

// cascadeLiveVerdicts are upstream rows that keep 9110 and 9111 live. An
// id__in request for 9120 returns nothing, so 9120 is gone.
func cascadeLiveVerdicts() []map[string]any {
	return []map[string]any{
		cascadeVerdictRow(9110, 911, "ok", cascadeRowTime),
		cascadeVerdictRow(9111, 911, "not-operational", cascadeRowTime),
	}
}

// syncCascade runs one sync cycle of w in mode and fails the test on
// error.
func syncCascade(t *testing.T, w *Worker, mode config.SyncMode) {
	t.Helper()
	if err := w.Sync(t.Context(), mode); err != nil {
		t.Fatalf("sync %s: %v", mode, err)
	}
}

// TestSync_CascadesDeletedNetIxLans verifies the cascade in sync cycles
// over the seeded rows (seedCascadeRows).
func TestSync_CascadesDeletedNetIxLans(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T, fkCap int) (*cascadeUpstream, *Worker, *sql.DB, *logBuffer) {
		t.Helper()
		up := newCascadeUpstream(t)
		up.setVerdicts(cascadeLiveVerdicts()...)
		w, db, logs := newCascadeWorker(t, up, fkCap)
		seedCascadeRows(t, w.entClient)
		return up, w, db, logs
	}

	t.Run("verified_backlog", func(t *testing.T) {
		t.Parallel()
		up, w, db, logs := setup(t, 20)
		cursor, err := GetMaxUpdated(t.Context(), db, "network_ix_lans")
		if err != nil {
			t.Fatalf("read cursor: %v", err)
		}
		before := allNetIxLans(t, w.entClient)

		syncCascade(t, w, config.SyncModeIncremental)

		if reqs := up.idInRequests(); len(reqs) != 1 || reqs[0].Get("id__in") != "9110,9111,9120" {
			t.Errorf("id__in requests = %v, want one for 9110,9111,9120", reqs)
		}
		want := maps.Clone(before)
		want[9120] = cascadedRow
		if got := allNetIxLans(t, w.entClient); !maps.Equal(got, want) {
			t.Errorf("rows after the cycle:\n got %v\nwant %v", got, want)
		}
		if after, _ := GetMaxUpdated(t.Context(), db, "network_ix_lans"); !after.Equal(cursor) {
			t.Errorf("netixlan cursor moved from %v to %v", cursor, after)
		}
		wantCascadeLog(t, logs, "WARN", "incremental", 1, 1, 1)
	})

	t.Run("upstream_live_orphan", func(t *testing.T) {
		t.Parallel()
		up, w, _, logs := setup(t, 20)
		relisted := cascadeVerdictRow(9120, 912, "ok", cascadeRowTime)
		relisted["operational"] = false
		up.setVerdicts(append(cascadeLiveVerdicts(), relisted)...)
		up.setBulk("netixlan", relisted)

		syncCascade(t, w, config.SyncModeIncremental)
		syncCascade(t, w, config.SyncModeFull)
		syncCascade(t, w, config.SyncModeIncremental)

		if got := readNetIxLan(t, w.entClient, 9120); got.Status != "ok" {
			t.Errorf("netixlan 9120 status = %q, want ok (upstream serves it live)", got.Status)
		}
		if reqs := up.idInRequests(); len(reqs) != 1 {
			t.Errorf("id__in requests = %d, want 1 (the memo covers the later cycles)", len(reqs))
		}
		for _, rec := range logs.records(t, cascadeLogMsg) {
			if rec["count"] != float64(0) {
				t.Errorf("cascade log = %v, want count 0", rec)
			}
		}
	})

	t.Run("verification_failure", func(t *testing.T) {
		t.Parallel()
		up, w, _, logs := setup(t, 20)
		up.setRespond(func(int, []int) (int, string, bool) {
			return http.StatusInternalServerError, "boom", true
		})

		syncCascade(t, w, config.SyncModeIncremental)
		if got := readNetIxLan(t, w.entClient, 9120); got.Status != "ok" {
			t.Errorf("netixlan 9120 status = %q after a failed verification, want ok", got.Status)
		}
		if recs := logs.records(t, "netixlan cascade verification failed, cascade deferred"); len(recs) != 1 || recs[0]["level"] != "WARN" {
			t.Errorf("verification failure log = %v, want one WARN record", recs)
		}

		up.setRespond(nil)
		ageNetIxLanVerifyMemo(w) // the 6h backoff has passed
		syncCascade(t, w, config.SyncModeIncremental)
		if got := readNetIxLan(t, w.entClient, 9120); got != cascadedRow {
			t.Errorf("netixlan 9120 = %+v after the retry, want %+v", got, cascadedRow)
		}
	})

	t.Run("phase_b_retry", func(t *testing.T) {
		t.Parallel()
		up, w, _, _ := setup(t, 20)
		bad := bumpUpdated(makeOrg(999, "Bad", "ok"), cascadeT(1).Format(time.RFC3339))
		bad["name"] = 12345 // fails the Phase B decode
		up.setBulk("org", bad)
		if err := w.Sync(t.Context(), config.SyncModeIncremental); err == nil {
			t.Fatal("sync with an undecodable org row succeeded, want a Phase B failure")
		}
		if got := readNetIxLan(t, w.entClient, 9120); got.Status != "ok" {
			t.Fatalf("netixlan 9120 status = %q after the rollback, want ok", got.Status)
		}

		up.setBulk("org")
		syncCascade(t, w, config.SyncModeIncremental)
		if got := readNetIxLan(t, w.entClient, 9120); got != cascadedRow {
			t.Errorf("netixlan 9120 = %+v after the retry, want %+v", got, cascadedRow)
		}
		if reqs := up.idInRequests(); len(reqs) != 1 {
			t.Errorf("id__in requests = %d, want 1 (the retry reuses the gone verdict)", len(reqs))
		}
	})

	t.Run("commit_failure", func(t *testing.T) {
		t.Parallel()
		up, w, db, _ := setup(t, 20)
		probe := withCommitProbe(w, db)
		before := allNetIxLans(t, w.entClient)

		probe.fail.Store(true)
		if err := w.Sync(t.Context(), config.SyncModeIncremental); !errors.Is(err, errInjectedCommit) {
			t.Fatalf("sync error = %v, want the injected commit failure", err)
		}
		if got := allNetIxLans(t, w.entClient); !maps.Equal(got, before) {
			t.Errorf("rows after the failed commit:\n got %v\nwant %v", got, before)
		}
		wantNoCascadeChangeLog(t, probe.logs)

		// The retry reuses the gone verdict and commits. It logs the
		// cascade only after the commit.
		probe.fail.Store(false)
		syncCascade(t, w, config.SyncModeIncremental)
		if got := readNetIxLan(t, w.entClient, 9120); got != cascadedRow {
			t.Errorf("netixlan 9120 = %+v after the retry, want %+v", got, cascadedRow)
		}
		if reqs := up.idInRequests(); len(reqs) != 1 {
			t.Errorf("id__in requests = %d, want 1 (the retry reuses the gone verdict)", len(reqs))
		}
		wantNoCascadeChangeLog(t, probe.logsAtCommit())
		wantCascadeLog(t, probe.logs, "WARN", "incremental", 1, 1, 1)
	})

	t.Run("phase_b_retry_deleted_verdict", func(t *testing.T) {
		t.Parallel()
		up, w, _, _ := setup(t, 20)
		up.setVerdicts(append(cascadeLiveVerdicts(), cascadeVerdictRow(9120, 912, "deleted", cascadeT(30)))...)
		bad := bumpUpdated(makeOrg(999, "Bad", "ok"), cascadeT(1).Format(time.RFC3339))
		bad["name"] = 12345 // fails the Phase B decode
		up.setBulk("org", bad)
		if err := w.Sync(t.Context(), config.SyncModeIncremental); err == nil {
			t.Fatal("sync with an undecodable org row succeeded, want a Phase B failure")
		}

		// The staged row went with the failed cycle's scratch DB, so the
		// retry asks again and stages upstream's row again.
		up.setBulk("org")
		syncCascade(t, w, config.SyncModeIncremental)
		want := netIxLanRow{Status: "deleted", Operational: false, Updated: cascadeT(30), Speed: 1000}
		if got := readNetIxLan(t, w.entClient, 9120); got != want {
			t.Errorf("netixlan 9120 = %+v after the retry, want upstream's row %+v", got, want)
		}
		if reqs := up.idInRequests(); len(reqs) != 2 || reqs[1].Get("id__in") != "9120" {
			t.Errorf("id__in requests = %v, want the first request and one for 9120", reqs)
		}
	})

	t.Run("undeleted_net_race", func(t *testing.T) {
		t.Parallel()
		up, w, _, _ := setup(t, 20)
		seedCascadeNetIxLan(t, w.entClient, 9115, 911, "ok", true, cascadeT(96))

		syncCascade(t, w, config.SyncModeIncremental)

		if got := readNetIxLan(t, w.entClient, 9115); got.Status != "ok" {
			t.Errorf("netixlan 9115 status = %q, want ok (saved after the network's delete)", got.Status)
		}
		for _, q := range up.idInRequests() {
			if slices.Contains(strings.Split(q.Get("id__in"), ","), "9115") {
				t.Errorf("id__in request %q asks for 9115", q.Get("id__in"))
			}
		}
	})

	// freshRIRNet stores network 911 live with rirStatus and serves its
	// upstream tombstone with the reclaim signature, as the first cycle
	// after the reclaim sees it. 9120 is stored deleted, so no row is a
	// verification candidate.
	freshRIRNet := func(t *testing.T, up *cascadeUpstream, w *Worker, rirStatus *string) {
		t.Helper()
		w.entClient.Network.UpdateOneID(911).SetStatus("ok").SetUpdated(cascadeRowTime).
			SetNillableRirStatus(rirStatus).ExecX(t.Context())
		w.entClient.NetworkIxLan.UpdateOneID(9120).SetStatus("deleted").ExecX(t.Context())
		up.setBulk("net", rirTombstoneNet(911, cascadeT(48)))
	}

	t.Run("incremental_fresh_rir_net", func(t *testing.T) {
		t.Parallel()
		up, w, _, logs := setup(t, 20)
		freshRIRNet(t, up, w, nil)
		before := allNetIxLans(t, w.entClient)

		syncCascade(t, w, config.SyncModeIncremental)

		want := maps.Clone(before)
		want[9110] = cascadedRow
		want[9111] = cascadedRow
		if got := allNetIxLans(t, w.entClient); !maps.Equal(got, want) {
			t.Errorf("rows after the cycle:\n got %v\nwant %v", got, want)
		}
		if reqs := up.idInRequests(); len(reqs) != 0 {
			t.Errorf("id__in requests = %d, want 0", len(reqs))
		}
		wantCascadeLog(t, logs, "WARN", "incremental", 2, 1, 0)
	})

	t.Run("stored_rir_status_cleared", func(t *testing.T) {
		t.Parallel()
		up, w, _, logs := setup(t, 20)
		rirOK := "ok"
		freshRIRNet(t, up, w, &rirOK)

		syncCascade(t, w, config.SyncModeIncremental)

		// The tombstone is the only net row of its batch and carries a
		// null rir_status. The upsert clears the stored "ok"
		// (resolveWithRow), and class A cascades the netixlans.
		if got := w.entClient.Network.GetX(t.Context(), 911).RirStatus; got != nil {
			t.Fatalf("stored rir_status of network 911 = %q, want nil", *got)
		}
		for _, id := range []int{9110, 9111} {
			if got := readNetIxLan(t, w.entClient, id); got != cascadedRow {
				t.Errorf("netixlan %d = %+v, want %+v", id, got, cascadedRow)
			}
		}
		wantCascadeLog(t, logs, "WARN", "incremental", 2, 1, 0)
	})

	t.Run("resaved_deleted_net", func(t *testing.T) {
		t.Parallel()
		up, w, _, logs := setup(t, 20)
		up.setBulk("net", rirTombstoneNet(911, cascadeT(60)))
		before := allNetIxLans(t, w.entClient)

		syncCascade(t, w, config.SyncModeIncremental)

		if got := w.entClient.Network.GetX(t.Context(), 911).Updated.UTC(); !got.Equal(cascadeT(60)) {
			t.Fatalf("network 911 updated = %v, want the re-saved tombstone's %v", got, cascadeT(60))
		}
		want := maps.Clone(before)
		want[9120] = cascadedRow
		if got := allNetIxLans(t, w.entClient); !maps.Equal(got, want) {
			t.Errorf("rows after the cycle:\n got %v\nwant %v", got, want)
		}
		if reqs := up.idInRequests(); len(reqs) != 1 {
			t.Errorf("id__in requests = %d, want 1", len(reqs))
		}
		wantCascadeLog(t, logs, "WARN", "incremental", 1, 1, 1)
	})

	t.Run("kill_switch", func(t *testing.T) {
		t.Parallel()
		up, w, _, _ := setup(t, 0)

		syncCascade(t, w, config.SyncModeIncremental)

		if reqs := up.idInRequests(); len(reqs) != 0 {
			t.Errorf("id__in requests = %d, want 0 with the FK backfill cap at 0", len(reqs))
		}
		if got := readNetIxLan(t, w.entClient, 9120); got.Status != "ok" {
			t.Errorf("netixlan 9120 status = %q, want ok with verification off", got.Status)
		}
	})
}

// deletedCounterValues returns the pdbplus.sync.type.deleted data points
// by type.
func deletedCounterValues(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	m := findMetric(rm, "pdbplus.sync.type.deleted")
	if m == nil {
		t.Fatal("pdbplus.sync.type.deleted not found")
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("pdbplus.sync.type.deleted is %T, want Sum[int64]", m.Data)
	}
	out := make(map[string]int64, len(sum.DataPoints))
	for _, dp := range sum.DataPoints {
		typ, _ := dp.Attributes.Value("type")
		out[typ.AsString()] = dp.Value
	}
	return out
}

// TestSync_CascadeRecordsDeletedCounter verifies that a sync cycle adds
// its cascaded rows to pdbplus.sync.type.deleted{type=netixlan}, and that
// a cycle without cascaded rows adds nothing.
// Not parallel: rebinds the package-level metric instruments.
func TestSync_CascadeRecordsDeletedCounter(t *testing.T) {
	reader := setupMetricTest(t)
	pdbotel.PrewarmCounters(t.Context())
	up := newCascadeUpstream(t)
	up.setVerdicts(cascadeLiveVerdicts()...)
	w, _, _ := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)

	syncCascade(t, w, config.SyncModeIncremental)
	got := deletedCounterValues(t, reader)
	if got["netixlan"] != 1 {
		t.Errorf("netixlan deleted = %d, want 1", got["netixlan"])
	}

	syncCascade(t, w, config.SyncModeIncremental)
	again := deletedCounterValues(t, reader)
	if !maps.Equal(again, got) {
		t.Errorf("a clean cycle changed the counter: %v, then %v", got, again)
	}
	if len(again) != 13 {
		t.Errorf("data points = %d, want the 13 prewarmed series", len(again))
	}
}

// markNetIxLansCascaded stores 9110, 9111 and 9120 as the cascade leaves
// them, so no seeded row is a candidate.
func markNetIxLansCascaded(t *testing.T, client *ent.Client) {
	t.Helper()
	client.NetworkIxLan.Update().
		Where(networkixlan.IDIn(9110, 9111, 9120)).
		SetStatus("deleted").SetOperational(false).
		ExecX(t.Context())
}

// TestSync_FullModeKeepsCascadedNetIxLanTombstone verifies that a stale
// full-mode bare list that still lists a cascaded netixlan live, with the
// same updated, does not revive it, whether or not its network is live
// again.
func TestSync_FullModeKeepsCascadedNetIxLanTombstone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		nets          []any
		wantNetStatus string
	}{
		{"net_undeleted", []any{bumpUpdated(makeNet(911, 910, 64911, "CascadeNet911", "ok"), cascadeT(60).Format(time.RFC3339))}, "ok"},
		{"net_still_deleted", nil, "deleted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			w, _, logs := newCascadeWorker(t, up, 20)
			seedCascadeRows(t, w.entClient)
			markNetIxLansCascaded(t, w.entClient)
			stale := cascadeVerdictRow(9110, 911, "ok", cascadeRowTime)
			stale["speed"] = 9
			up.setBulk("netixlan", stale)
			up.setBulk("net", tc.nets...)

			syncCascade(t, w, config.SyncModeFull)

			if got := w.entClient.Network.GetX(t.Context(), 911).Status; got != tc.wantNetStatus {
				t.Errorf("network 911 status = %q, want %q", got, tc.wantNetStatus)
			}
			if got := readNetIxLan(t, w.entClient, 9110); got != cascadedRow {
				t.Errorf("netixlan 9110 = %+v, want %+v (kept against the stale re-list)", got, cascadedRow)
			}
			if reqs := up.idInRequests(); len(reqs) != 0 {
				t.Errorf("id__in requests = %d, want 0", len(reqs))
			}
			for _, rec := range logs.records(t, cascadeLogMsg) {
				if rec["count"] != float64(0) {
					t.Errorf("cascade log = %v, want count 0", rec)
				}
			}
		})
	}
}

// TestSync_FullModeRevivesNewerNetIxLan verifies that the tombstone term
// of the upsert gate still admits a real revival, which carries a newer
// updated.
func TestSync_FullModeRevivesNewerNetIxLan(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	w, _, _ := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)
	markNetIxLansCascaded(t, w.entClient)
	revived := cascadeVerdictRow(9110, 911, "ok", cascadeT(1))
	revived["speed"] = 9
	up.setBulk("netixlan", revived)

	syncCascade(t, w, config.SyncModeFull)

	want := netIxLanRow{Status: "ok", Operational: true, Updated: cascadeT(1), Speed: 9}
	if got := readNetIxLan(t, w.entClient, 9110); got != want {
		t.Errorf("netixlan 9110 = %+v, want %+v", got, want)
	}
}

// recordRecentSync stores a successful sync ten minutes ago, so that
// StartScheduler does not run a cycle during the test.
func recordRecentSync(t *testing.T, db *sql.DB) {
	t.Helper()
	at := time.Now().Add(-10 * time.Minute)
	id, err := RecordSyncStart(t.Context(), db, at, "incremental")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := RecordSyncComplete(t.Context(), db, id, Status{
		LastSyncAt: at, Duration: time.Second, Status: "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}
}

// waitFor polls cond until it holds. It fails the test when done closes
// first or the deadline passes; the deadline only bounds a broken run.
func waitFor(t *testing.T, cancel context.CancelFunc, done <-chan struct{}, what string, cond func() bool) {
	t.Helper()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !cond() {
		select {
		case <-tick.C:
		case <-done:
			t.Fatalf("scheduler exited before %s", what)
		case <-deadline.C:
			cancel()
			<-done
			t.Fatalf("%s did not happen before the deadline", what)
		}
	}
}

// TestStartScheduler_CascadesNetIxLansAtStartup verifies that a primary
// verifies and cascades the backlog when the scheduler starts, before the
// first sync cycle is due. The last sync is recent, so no cycle runs. The
// cascade log comes after the commit. When the commit fails, no row
// changes, and the failure is the only WARN about the cascade.
func TestStartScheduler_CascadesNetIxLansAtStartup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		failCommit bool
	}{
		{"committed", false},
		{"commit_fails", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			w, db, _ := newCascadeWorker(t, up, 20)
			probe := withCommitProbe(w, db)
			seedCascadeRows(t, w.entClient)
			recordRecentSync(t, db)
			want := allNetIxLans(t, w.entClient)
			if !tc.failCommit {
				for _, id := range []int{9110, 9111, 9120} {
					want[id] = cascadedRow
				}
			}
			probe.fail.Store(tc.failCommit)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			go func() {
				w.StartScheduler(ctx, time.Hour)
				close(done)
			}()
			waitFor(t, cancel, done, "the startup cascade ended", func() bool {
				if tc.failCommit {
					return len(probe.logs.records(t, startupCascadeFailedMsg)) == 1
				}
				// A read error means "not yet": the cascade transaction
				// can lock the shared-cache table.
				var status string
				err := db.QueryRowContext(ctx, "SELECT status FROM network_ix_lans WHERE id = 9120").Scan(&status)
				return err == nil && status == "deleted"
			})
			cancel()
			<-done

			if calls, reqs := up.requestCount(), up.idInRequests(); calls != 1 || len(reqs) != 1 {
				t.Errorf("upstream requests = %d (id__in %d), want exactly the one id__in request", calls, len(reqs))
			}
			if got := allNetIxLans(t, w.entClient); !maps.Equal(got, want) {
				t.Errorf("rows after the startup cascade:\n got %v\nwant %v", got, want)
			}
			if tc.failCommit {
				wantNoCascadeChangeLog(t, probe.logs)
				recs := probe.logs.records(t, startupCascadeFailedMsg)
				if len(recs) != 1 || recs[0]["level"] != "WARN" ||
					!strings.Contains(fmt.Sprint(recs[0]["error"]), errInjectedCommit.Error()) {
					t.Errorf("failure log = %v, want one WARN record with the injected commit error", recs)
				}
			} else {
				wantNoCascadeChangeLog(t, probe.logsAtCommit())
				wantCascadeLog(t, probe.logs, "WARN", "startup", 3, 2, 3)
			}
			if w.Running() {
				t.Error("running latch still held after the startup cascade")
			}
		})
	}
}

// TestCascadeNetIxLansAtStartup_RecordsDeletedCounter verifies that the
// startup cascade adds its rows to pdbplus.sync.type.deleted after its
// commit.
// Not parallel: rebinds the package-level metric instruments.
func TestCascadeNetIxLansAtStartup_RecordsDeletedCounter(t *testing.T) {
	reader := setupMetricTest(t)
	up := newCascadeUpstream(t)
	w, _, _ := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)

	w.cascadeNetIxLansAtStartup(t.Context())

	if got := deletedCounterValues(t, reader); got["netixlan"] != 3 {
		t.Errorf("netixlan deleted = %d, want 3", got["netixlan"])
	}
}

// TestStartScheduler_ReplicaSkipsNetIxLanCascade verifies that a replica
// neither asks upstream nor writes.
func TestStartScheduler_ReplicaSkipsNetIxLanCascade(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	w, _, _ := newCascadeWorker(t, up, 20)
	w.config.IsPrimary = func() bool { return false }
	seedCascadeRows(t, w.entClient)
	before := allNetIxLans(t, w.entClient)

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	w.StartScheduler(ctx, time.Hour)

	if calls := up.requestCount(); calls != 0 {
		t.Errorf("upstream requests = %d, want 0 on a replica", calls)
	}
	if got := allNetIxLans(t, w.entClient); !maps.Equal(got, before) {
		t.Errorf("a replica changed rows:\n got %v\nwant %v", got, before)
	}
}

// TestCascadeNetIxLansAtStartup_SkipsWhileSyncRuns verifies that the
// startup run does not overlap a sync cycle, which runs the cascade
// itself.
func TestCascadeNetIxLansAtStartup_SkipsWhileSyncRuns(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	w, _, logs := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)
	before := allNetIxLans(t, w.entClient)

	w.running.Store(true)
	w.cascadeNetIxLansAtStartup(t.Context())

	if !w.running.Load() {
		t.Error("the skipped startup cascade released the latch of the running cycle")
	}
	if calls := up.requestCount(); calls != 0 {
		t.Errorf("upstream requests = %d, want 0", calls)
	}
	if got := allNetIxLans(t, w.entClient); !maps.Equal(got, before) {
		t.Errorf("rows changed while a sync cycle held the latch:\n got %v\nwant %v", got, before)
	}
	if recs := logs.records(t, "sync cycle running, skipping startup netixlan cascade"); len(recs) != 1 {
		t.Errorf("skip log records = %d, want 1", len(recs))
	}
}

// TestCascadeNetIxLansAtStartup_VerificationFails verifies that a failed
// verification opens no transaction, logs a WARN and releases the latch.
func TestCascadeNetIxLansAtStartup_VerificationFails(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	up.setRespond(func(int, []int) (int, string, bool) {
		return http.StatusInternalServerError, "boom", true
	})
	w, _, logs := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)
	before := allNetIxLans(t, w.entClient)

	w.cascadeNetIxLansAtStartup(t.Context())

	if got := allNetIxLans(t, w.entClient); !maps.Equal(got, before) {
		t.Errorf("rows changed after a failed verification:\n got %v\nwant %v", got, before)
	}
	if recs := logs.records(t, "netixlan cascade verification failed, cascade deferred"); len(recs) != 1 || recs[0]["level"] != "WARN" {
		t.Errorf("verification failure log = %v, want one WARN record", recs)
	}
	if recs := logs.records(t, cascadeLogMsg); len(recs) != 1 || recs[0]["level"] != "DEBUG" || recs[0]["count"] != float64(0) {
		t.Errorf("cascade log = %v, want one DEBUG record with count 0 (no transaction)", recs)
	}
	if w.Running() {
		t.Error("running latch still held after the failed startup cascade")
	}
}

// TestCascadeNetIxLansAtStartup_DefersDeletedVerdicts verifies that the
// startup run does not derive a tombstone for a row that upstream returns
// deleted. The wait is expected, so the summary stays at INFO. The first
// cycle stages upstream's row, so the stored row carries upstream's
// updated.
func TestCascadeNetIxLansAtStartup_DefersDeletedVerdicts(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	up.setVerdicts(append(cascadeLiveVerdicts(), cascadeVerdictRow(9120, 912, "deleted", cascadeT(30)))...)
	w, _, logs := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)

	w.cascadeNetIxLansAtStartup(t.Context())
	if got := readNetIxLan(t, w.entClient, 9120); got.Status != "ok" {
		t.Fatalf("netixlan 9120 status = %q after startup, want ok (deferred to the first cycle)", got.Status)
	}
	if _, ok := w.netIxLanVerifyMemo[9120]; ok {
		t.Error("memo holds the deferred id 9120")
	}
	recs := logs.records(t, "verified netixlan cascade candidates")
	if len(recs) != 1 || recs[0]["level"] != "INFO" || recs[0]["waiting"] != float64(1) || recs[0]["deferred"] != float64(0) {
		t.Errorf("startup summary = %v, want one INFO record with waiting=1 and deferred=0", recs)
	}

	syncCascade(t, w, config.SyncModeIncremental)
	want := netIxLanRow{Status: "deleted", Operational: false, Updated: cascadeT(30), Speed: 1000}
	if got := readNetIxLan(t, w.entClient, 9120); got != want {
		t.Errorf("netixlan 9120 = %+v after the first cycle, want upstream's row %+v", got, want)
	}
	if reqs := up.idInRequests(); len(reqs) != 2 || reqs[1].Get("id__in") != "9120" {
		t.Errorf("id__in requests = %v, want the startup request and one for 9120", reqs)
	}
}

// TestCascadeNetIxLansAtStartup_RecoversPanic verifies the panic
// firewall: a panic during the startup run is logged at ERROR with a
// stack, the latch is released, no row changes, and the scheduler keeps
// running until its context ends. A nil upstream client makes the
// verification request panic.
func TestCascadeNetIxLansAtStartup_RecoversPanic(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	logs := &logBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w := NewWorker(nil, client, db, WorkerConfig{FKBackfillMaxRequestsPerCycle: 20}, logger)
	seedCascadeRows(t, client)
	before := allNetIxLans(t, client)

	w.cascadeNetIxLansAtStartup(t.Context())

	const msg = "startup netixlan cascade panic recovered"
	recs := logs.records(t, msg)
	if len(recs) != 1 || recs[0]["level"] != "ERROR" {
		t.Fatalf("panic log = %v, want one ERROR record", recs)
	}
	if stack, _ := recs[0]["stack"].(string); !strings.Contains(stack, "cascadeNetIxLansAtStartup") {
		t.Errorf("panic log stack = %q, want the goroutine stack", stack)
	}
	if w.Running() {
		t.Error("running latch still held after the recovered panic")
	}
	if got := allNetIxLans(t, client); !maps.Equal(got, before) {
		t.Errorf("rows changed after the recovered panic:\n got %v\nwant %v", got, before)
	}

	recordRecentSync(t, db)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, time.Hour)
		close(done)
	}()
	waitFor(t, cancel, done, "the second recovered panic", func() bool {
		return len(logs.records(t, msg)) == 2
	})
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("StartScheduler did not return after its context ended")
	}
}
