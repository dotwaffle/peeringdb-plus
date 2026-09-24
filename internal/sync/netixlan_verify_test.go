package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// cascadeRowTime is T0 of the netixlan cascade seed rows.
var cascadeRowTime = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

// cascadeT returns T0 plus h hours.
func cascadeT(h int) time.Time {
	return cascadeRowTime.Add(time.Duration(h) * time.Hour)
}

// seedCascadeParents creates org, ix and ixlan 910 and three networks:
// 910 ok at T0, 911 deleted at T0+48h and 912 deleted at T0+24h.
func seedCascadeParents(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := t.Context()
	t0 := cascadeRowTime
	client.Organization.Create().SetID(910).SetName("CascadeOrg").SetNameFold("cascadeorg").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	client.InternetExchange.Create().SetID(910).SetOrgID(910).SetName("CascadeIX").SetNameFold("cascadeix").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	client.IxLan.Create().SetID(910).SetIxID(910).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	for _, n := range []struct {
		id      int
		status  string
		updated time.Time
	}{
		{910, "ok", t0},
		{911, "deleted", cascadeT(48)},
		{912, "deleted", cascadeT(24)},
	} {
		name := "CascadeNet" + strconv.Itoa(n.id)
		client.Network.Create().SetID(n.id).SetOrgID(910).SetName(name).SetNameFold(strings.ToLower(name)).
			SetAsn(64000 + n.id).SetStatus(n.status).SetCreated(t0).SetUpdated(n.updated).SaveX(ctx)
	}
}

// seedCascadeNetIxLan stores one netixlan on ixlan 910. netID 0 stores
// the row without a network.
func seedCascadeNetIxLan(t *testing.T, client *ent.Client, id, netID int, status string, operational bool, updated time.Time) {
	t.Helper()
	b := client.NetworkIxLan.Create().
		SetID(id).SetIxlanID(910).SetIxID(910).
		SetAsn(64000 + netID).SetSpeed(1).SetName("CascadeIX").
		SetOperational(operational).SetStatus(status).
		SetCreated(cascadeRowTime).SetUpdated(updated)
	if netID != 0 {
		b.SetNetID(netID)
	}
	b.SaveX(t.Context())
}

// seedCascadeRows seeds the parents (seedCascadeParents) and these
// netixlans, updated at T0 unless noted:
//
//	9100  net 910 (ok)       ok
//	9110  net 911 (deleted)  ok
//	9111  net 911            not-operational
//	9112  net 911            pending
//	9113  net 911            deleted
//	9114  net 911            ok, updated T0+72h (after the network's delete)
//	9120  net 912 (deleted)  ok, operational false
//	9130  no network         ok
//
// The cascade candidates are 9110, 9111 and 9120.
func seedCascadeRows(t *testing.T, client *ent.Client) {
	t.Helper()
	seedCascadeParents(t, client)
	t0 := cascadeRowTime
	for _, r := range []struct {
		id, netID   int
		status      string
		operational bool
		updated     time.Time
	}{
		{9100, 910, "ok", true, t0},
		{9110, 911, "ok", true, t0},
		{9111, 911, "not-operational", false, t0},
		{9112, 911, "pending", false, t0},
		{9113, 911, "deleted", false, t0},
		{9114, 911, "ok", true, cascadeT(72)},
		{9120, 912, "ok", false, t0},
		{9130, 0, "ok", true, t0},
	} {
		seedCascadeNetIxLan(t, client, r.id, r.netID, r.status, r.operational, r.updated)
	}
}

// seedCandidateRows seeds the parents and n live netixlans on network
// 911, ids first to first+n-1, all of them cascade candidates. It
// returns the ids.
func seedCandidateRows(t *testing.T, client *ent.Client, first, n int) []int {
	t.Helper()
	seedCascadeParents(t, client)
	ids := make([]int, 0, n)
	builders := make([]*ent.NetworkIxLanCreate, 0, n)
	for id := first; id < first+n; id++ {
		ids = append(ids, id)
		builders = append(builders, client.NetworkIxLan.Create().
			SetID(id).SetNetID(911).SetIxlanID(910).SetIxID(910).
			SetAsn(64911).SetSpeed(1).SetName("CascadeIX").
			SetOperational(true).SetStatus("ok").
			SetCreated(cascadeRowTime).SetUpdated(cascadeRowTime))
	}
	for batch := range slices.Chunk(builders, 500) {
		client.NetworkIxLan.CreateBulk(batch...).ExecX(t.Context())
	}
	return ids
}

// netIxLanRow is the stored state of a netixlan that the cascade can
// change.
type netIxLanRow struct {
	Status      string
	Operational bool
	Updated     time.Time
	Speed       int
}

// readNetIxLan returns the stored state of netixlan id.
func readNetIxLan(t *testing.T, client *ent.Client, id int) netIxLanRow {
	t.Helper()
	n := client.NetworkIxLan.GetX(t.Context(), id)
	return netIxLanRow{Status: n.Status, Operational: n.Operational, Updated: n.Updated.UTC(), Speed: n.Speed}
}

// allNetIxLans returns the stored state of every netixlan, by id.
func allNetIxLans(t *testing.T, client *ent.Client) map[int]netIxLanRow {
	t.Helper()
	out := make(map[int]netIxLanRow)
	for _, n := range client.NetworkIxLan.Query().Order(networkixlan.ByID()).AllX(t.Context()) {
		out[n.ID] = netIxLanRow{Status: n.Status, Operational: n.Operational, Updated: n.Updated.UTC(), Speed: n.Speed}
	}
	return out
}

// cascadeVerdictRow returns an upstream netixlan row on ixlan 910.
func cascadeVerdictRow(id, netID int, status string, updated time.Time) map[string]any {
	return bumpUpdated(makeNetIxLan(id, netID, 910, status), updated.Format(time.RFC3339))
}

// closeConnection is a respond status that makes cascadeUpstream close
// the connection without an HTTP answer.
const closeConnection = -1

// cascadeUpstream stubs upstream for the netixlan cascade tests. A list
// request (no id__in) gets the rows of bulk[type] on its first page. A
// netixlan id__in request gets the verdict row of each requested id that
// has one; an id without a row is absent. respond, when set, can replace
// the answer to the nth netixlan id__in request (1-based, retries
// included) with a status and a raw body, or with closeConnection.
type cascadeUpstream struct {
	server *httptest.Server

	mu       stdsync.Mutex
	bulk     map[string][]any
	verdicts map[int]map[string]any
	respond  func(n int, ids []int) (status int, body string, ok bool)
	idIn     []url.Values
	calls    int
}

func newCascadeUpstream(t *testing.T) *cascadeUpstream {
	t.Helper()
	u := &cascadeUpstream{bulk: map[string][]any{}, verdicts: map[int]map[string]any{}}
	u.server = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.server.Close)
	return u
}

func (u *cascadeUpstream) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typ := strings.TrimPrefix(r.URL.Path, "/api/")
	u.mu.Lock()
	u.calls++
	var rows []any
	idIn := q.Get("id__in")
	switch {
	case idIn != "" && typ == "netixlan":
		u.idIn = append(u.idIn, q)
		n := len(u.idIn)
		var ids []int
		for p := range strings.SplitSeq(idIn, ",") {
			id, _ := strconv.Atoi(p)
			ids = append(ids, id)
			if row, ok := u.verdicts[id]; ok {
				rows = append(rows, row)
			}
		}
		respond := u.respond
		u.mu.Unlock()
		if respond != nil {
			if status, body, ok := respond(n, ids); ok {
				if status == closeConnection {
					if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
						_ = conn.Close()
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
				return
			}
		}
	case idIn == "" && (q.Get("skip") == "" || q.Get("skip") == "0"):
		rows = u.bulk[typ]
		u.mu.Unlock()
	default:
		u.mu.Unlock()
	}
	if rows == nil {
		rows = []any{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": rows})
}

// setBulk sets the list rows of typ.
func (u *cascadeUpstream) setBulk(typ string, rows ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.bulk[typ] = rows
}

// setVerdicts sets the verdict rows by netixlan id.
func (u *cascadeUpstream) setVerdicts(rows ...map[string]any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.verdicts = make(map[int]map[string]any, len(rows))
	for _, row := range rows {
		u.verdicts[row["id"].(int)] = row
	}
}

// setRespond sets the per-request override hook.
func (u *cascadeUpstream) setRespond(fn func(n int, ids []int) (int, string, bool)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.respond = fn
}

// idInRequests returns the query of every netixlan id__in request.
func (u *cascadeUpstream) idInRequests() []url.Values {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.idIn)
}

// requestCount returns the number of requests of any kind.
func (u *cascadeUpstream) requestCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
}

// logBuffer is a goroutine-safe log sink for a JSON slog handler.
type logBuffer struct {
	mu  stdsync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// records returns the decoded log records with message msg.
func (b *logBuffer) records(t *testing.T, msg string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	data := slices.Clone(b.buf.Bytes())
	b.mu.Unlock()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("decode log line: %v", err)
		}
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

// newCascadeWorker returns a primary worker over a fresh database that
// talks to up. fkCap is PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE, which
// also caps verification. The worker logs JSON at DEBUG to the returned
// buffer.
func newCascadeWorker(t *testing.T, up *cascadeUpstream, fkCap int) (*Worker, *sql.DB, *logBuffer) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	logs := &logBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w := NewWorker(newFastPDBClient(t, up.server.URL), client, db, WorkerConfig{
		FKBackfillMaxRequestsPerCycle: fkCap,
	}, logger)
	return w, db, logs
}

// newCascadeScratch opens a scratch DB that the test closes at cleanup.
func newCascadeScratch(t *testing.T) *scratchDB {
	t.Helper()
	s, err := openScratchDB(t.Context())
	if err != nil {
		t.Fatalf("open scratch: %v", err)
	}
	t.Cleanup(func() { closeScratchDB(context.Background(), s, nil) })
	return s
}

// scratchNetIxLan returns the raw scratch row of netixlan id, or nil.
func scratchNetIxLan(t *testing.T, s *scratchDB, id int) []byte {
	t.Helper()
	var data []byte
	err := s.db.QueryRowContext(t.Context(), `SELECT data FROM "netixlan" WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatalf("read scratch netixlan %d: %v", id, err)
	}
	return data
}

// TestVerifyNetIxLanCandidates_Classifies verifies the verdicts of one
// pass: an absent id and an upstream tombstone go to Gone, a live id does
// not, and the upstream tombstone is staged only when its updated is at
// or before the cursor. Only a tombstone after the cursor gets a gone
// memo entry; a staged one must be staged again by a retry. At startup,
// or with an unknown cursor, the tombstone waits for the first cycle; a
// staging error defers it. Neither gets a memo entry. The pass changes
// no stored row.
func TestVerifyNetIxLanCandidates_Classifies(t *testing.T) {
	t.Parallel()
	tombstone := cascadeVerdictRow(9120, 912, "deleted", cascadeT(30))
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name         string
		scratch      bool
		closeScratch bool
		cursor       time.Time
		cursorKnown  bool
		wantGone     []int
		wantStaged   int
		wantMemo9120 bool
		wantWaiting  int
		wantDeferred int
		wantErr      bool
	}{
		{"staged_at_cursor", true, false, cascadeT(30), true, []int{9110, 9120}, 1, false, 0, 0, false},
		{"after_cursor_not_staged", true, false, cascadeT(29), true, []int{9110, 9120}, 0, true, 0, 0, false},
		{"startup_defers_tombstone", false, false, cascadeT(72), true, []int{9110}, 0, false, 1, 0, false},
		{"unknown_cursor_defers_tombstone", true, false, time.Time{}, false, []int{9110}, 0, false, 1, 0, false},
		{"staging_error_defers_tombstone", true, true, cascadeT(72), true, []int{9110}, 0, false, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			up.setVerdicts(cascadeVerdictRow(9111, 911, "not-operational", cascadeRowTime), tombstone)
			w, _, _ := newCascadeWorker(t, up, 20)
			seedCascadeRows(t, w.entClient)
			before := allNetIxLans(t, w.entClient)

			in := netIxLanVerifyInput{NixCursor: tc.cursor, CursorKnown: tc.cursorKnown, Now: now, Mode: "incremental"}
			var s *scratchDB
			if tc.scratch {
				s = newCascadeScratch(t)
				in.Scratch = s
			}
			if tc.closeScratch {
				_ = s.db.Close()
			}
			v := w.verifyNetIxLanCandidates(t.Context(), in)

			reqs := up.idInRequests()
			if len(reqs) != 1 {
				t.Fatalf("id__in requests = %d, want 1", len(reqs))
			}
			if q := reqs[0]; q.Get("id__in") != "9110,9111,9120" || q.Get("since") != "1" || q.Get("hide_ix_no_fac") != "0" {
				t.Errorf("query = %v, want id__in=9110,9111,9120 since=1 hide_ix_no_fac=0", q)
			}
			if !slices.Equal(v.Gone, tc.wantGone) {
				t.Errorf("Gone = %v, want %v", v.Gone, tc.wantGone)
			}
			if v.Candidates != 3 || v.Absent != 1 || v.Live != 1 || v.Deleted != 1 || v.Staged != tc.wantStaged {
				t.Errorf("candidates=%d absent=%d live=%d deleted=%d staged=%d, want 3, 1, 1, 1, %d",
					v.Candidates, v.Absent, v.Live, v.Deleted, v.Staged, tc.wantStaged)
			}
			if v.Waiting != tc.wantWaiting || v.Deferred != tc.wantDeferred {
				t.Errorf("waiting=%d deferred=%d, want %d and %d", v.Waiting, v.Deferred, tc.wantWaiting, tc.wantDeferred)
			}
			if (v.Err != nil) != tc.wantErr {
				t.Errorf("Err = %v, want error %v", v.Err, tc.wantErr)
			}
			if e := w.netIxLanVerifyMemo[9111]; e.Outcome != verifyLive || !e.Updated.Equal(cascadeRowTime) || !e.NetUpdated.Equal(cascadeT(48)) {
				t.Errorf("memo[9111] = %+v, want live for updated T0 and net updated T0+48h", e)
			}
			if e := w.netIxLanVerifyMemo[9110]; e.Outcome != verifyGone {
				t.Errorf("memo[9110] = %+v, want gone", e)
			}
			if e, has9120 := w.netIxLanVerifyMemo[9120]; has9120 != tc.wantMemo9120 || (has9120 && e.Outcome != verifyGone) {
				t.Errorf("memo[9120] = %+v (present %v), want present %v with a gone verdict", e, has9120, tc.wantMemo9120)
			}
			if tc.scratch && !tc.closeScratch {
				got := scratchNetIxLan(t, s, 9120)
				want, _ := json.Marshal(tombstone)
				if tc.wantStaged == 1 && !bytes.Equal(got, want) {
					t.Errorf("scratch 9120 = %s, want the upstream row %s", got, want)
				}
				if tc.wantStaged == 0 && got != nil {
					t.Errorf("scratch 9120 = %s, want no staged row", got)
				}
			}
			if after := allNetIxLans(t, w.entClient); !maps.Equal(before, after) {
				t.Errorf("verification changed stored rows:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// TestVerifyNetIxLanCandidates_Memo verifies the memo: a second pass
// within the TTL sends no request and reuses the gone verdict, an expired
// entry is verified again, a changed stored updated resets the entries of
// that network, and an id that is no longer a candidate is pruned.
func TestVerifyNetIxLanCandidates_Memo(t *testing.T) {
	t.Parallel()
	up := newCascadeUpstream(t)
	up.setVerdicts(
		cascadeVerdictRow(9111, 911, "not-operational", cascadeRowTime),
		cascadeVerdictRow(9120, 912, "ok", cascadeRowTime),
	)
	w, _, _ := newCascadeWorker(t, up, 20)
	seedCascadeRows(t, w.entClient)
	ctx := t.Context()
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	pass := func(at time.Time) netIxLanVerification {
		t.Helper()
		return w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{Now: at, Mode: "incremental", CursorKnown: true})
	}
	lastIDIn := func() string {
		reqs := up.idInRequests()
		return reqs[len(reqs)-1].Get("id__in")
	}

	if v := pass(now); v.Requests != 1 || !slices.Equal(v.Gone, []int{9110}) {
		t.Fatalf("first pass: requests=%d Gone=%v, want 1 and [9110]", v.Requests, v.Gone)
	}
	if v := pass(now); v.Requests != 0 || v.MemoHits != 3 || !slices.Equal(v.Gone, []int{9110}) {
		t.Errorf("second pass: requests=%d memo_hits=%d Gone=%v, want 0, 3 and [9110]", v.Requests, v.MemoHits, v.Gone)
	}
	later := now.Add(25 * time.Hour)
	if v := pass(later); v.Requests != 1 || lastIDIn() != "9110,9111,9120" {
		t.Errorf("pass after the TTL: requests=%d id__in=%q, want 1 and 9110,9111,9120", v.Requests, lastIDIn())
	}

	w.entClient.Network.UpdateOneID(911).SetUpdated(cascadeT(50)).ExecX(ctx)
	if v := pass(later); v.Requests != 1 || lastIDIn() != "9110,9111" || v.MemoHits != 1 {
		t.Errorf("pass after net 911 changed: requests=%d id__in=%q memo_hits=%d, want 1, 9110,9111 and 1",
			v.Requests, lastIDIn(), v.MemoHits)
	}

	w.entClient.NetworkIxLan.UpdateOneID(9111).SetStatus("deleted").ExecX(ctx)
	if v := pass(later); v.Candidates != 2 || v.Requests != 0 {
		t.Errorf("pass after 9111 left the candidates: candidates=%d requests=%d, want 2 and 0", v.Candidates, v.Requests)
	}
	if _, ok := w.netIxLanVerifyMemo[9111]; ok {
		t.Error("memo still holds 9111, which is no longer a candidate")
	}
}

// TestVerifyNetIxLanCandidates_Failures verifies chunk failure handling:
// a failed chunk backs off its ids and does not stop the pass when the
// chunk before it succeeded, retries use smaller chunks so one bad id
// stops blocking the others, a failing endpoint gets one request per
// pass, an unreadable body fails its chunk, an error that says nothing
// about the ids stops the pass, and rows for ids that were not requested
// are ignored.
func TestVerifyNetIxLanCandidates_Failures(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

	t.Run("backoff_and_smaller_chunks", func(t *testing.T) {
		t.Parallel()
		up := newCascadeUpstream(t)
		w, _, _ := newCascadeWorker(t, up, 20)
		ids := seedCandidateRows(t, w.entClient, 20001, 250)
		var mu stdsync.Mutex
		bad := map[int]bool{}
		for _, id := range ids[100:200] {
			bad[id] = true
		}
		up.setRespond(func(_ int, req []int) (int, string, bool) {
			mu.Lock()
			defer mu.Unlock()
			for _, id := range req {
				if bad[id] {
					return http.StatusInternalServerError, "boom", true
				}
			}
			return 0, "", false
		})
		pass := func(at time.Time) netIxLanVerification {
			t.Helper()
			return w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: at, Mode: "incremental", CursorKnown: true})
		}

		v := pass(now)
		if v.Requests != 3 || v.Failed != 100 || v.Absent != 150 || len(v.Gone) != 150 || v.Err == nil {
			t.Fatalf("first pass: requests=%d failed=%d absent=%d gone=%d err=%v, want 3, 100, 150, 150 and an error",
				v.Requests, v.Failed, v.Absent, len(v.Gone), v.Err)
		}
		if e := w.netIxLanVerifyMemo[ids[100]]; e.Outcome != verifyFailed || e.Failures != 1 {
			t.Errorf("memo[%d] = %+v, want failed with Failures=1", ids[100], e)
		}
		if !slices.Equal(v.FailedIDs, ids[100:110]) {
			t.Errorf("FailedIDs = %v, want the first 10 failed ids", v.FailedIDs)
		}

		if v := pass(now.Add(time.Hour)); v.Requests != 0 || v.Backoff != 100 || len(v.Gone) != 150 {
			t.Errorf("pass in backoff: requests=%d backoff=%d gone=%d, want 0, 100 and 150", v.Requests, v.Backoff, len(v.Gone))
		}

		// The bad id is in the second retry chunk, so the chunk before it
		// succeeded and the pass goes on.
		mu.Lock()
		bad = map[int]bool{ids[115]: true}
		mu.Unlock()
		before := len(up.idInRequests())
		v = pass(now.Add(6 * time.Hour))
		reqs := up.idInRequests()[before:]
		if v.Requests != 10 || v.Failed != 10 || v.Absent != 90 {
			t.Errorf("retry pass: requests=%d failed=%d absent=%d, want 10, 10 and 90", v.Requests, v.Failed, v.Absent)
		}
		for _, q := range reqs {
			if n := len(strings.Split(q.Get("id__in"), ",")); n != 10 {
				t.Errorf("retry chunk has %d ids, want 10", n)
			}
		}

		before = len(up.idInRequests())
		v = pass(now.Add(12 * time.Hour))
		reqs = up.idInRequests()[before:]
		if v.Requests != 10 || v.Failed != 1 || v.Absent != 9 || !slices.Equal(v.FailedIDs, []int{ids[115]}) {
			t.Errorf("second retry: requests=%d failed=%d absent=%d failed_ids=%v, want 10, 1, 9 and [%d]",
				v.Requests, v.Failed, v.Absent, v.FailedIDs, ids[115])
		}
		for _, q := range reqs {
			if n := len(strings.Split(q.Get("id__in"), ",")); n != 1 {
				t.Errorf("second retry chunk has %d ids, want 1", n)
			}
		}
		if len(v.Gone) != 249 || slices.Contains(v.Gone, ids[115]) {
			t.Errorf("Gone has %d ids (contains bad id: %v), want the 249 other ids", len(v.Gone), slices.Contains(v.Gone, ids[115]))
		}
		if e := w.netIxLanVerifyMemo[ids[115]]; e.Failures != 3 {
			t.Errorf("memo[%d].Failures = %d, want 3", ids[115], e.Failures)
		}
	})

	// A failed chunk stops the pass unless the chunk before it succeeded,
	// so an endpoint that fails every request gets one request per pass.
	for _, tc := range []struct {
		name         string
		candidates   int
		failFrom     int // first failing request, 1-based
		wantRequests int
		wantAbsent   int
		wantFailed   int
		wantDeferred int
	}{
		{"first_chunk_fails", 250, 1, 1, 0, 100, 150},
		{"two_failed_chunks_in_a_row", 400, 2, 3, 100, 200, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			up.setRespond(func(n int, _ []int) (int, string, bool) {
				if n >= tc.failFrom {
					return http.StatusInternalServerError, "boom", true
				}
				return 0, "", false
			})
			w, _, _ := newCascadeWorker(t, up, 20)
			ids := seedCandidateRows(t, w.entClient, 20001, tc.candidates)
			pass := func(at time.Time) netIxLanVerification {
				t.Helper()
				return w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: at, Mode: "incremental", CursorKnown: true})
			}

			v := pass(now)
			if v.Requests != tc.wantRequests || v.Absent != tc.wantAbsent || v.Failed != tc.wantFailed || v.Deferred != tc.wantDeferred {
				t.Errorf("requests=%d absent=%d failed=%d deferred=%d, want %d, %d, %d and %d",
					v.Requests, v.Absent, v.Failed, v.Deferred, tc.wantRequests, tc.wantAbsent, tc.wantFailed, tc.wantDeferred)
			}
			sent := tc.wantAbsent + tc.wantFailed
			for _, id := range ids[sent:] {
				if _, ok := w.netIxLanVerifyMemo[id]; ok {
					t.Fatalf("memo holds %d, which the pass did not request", id)
				}
			}

			// The next pass skips the failed ids and tries the next chunk.
			before := len(up.idInRequests())
			v = pass(now)
			reqs := up.idInRequests()[before:]
			if v.Backoff != tc.wantFailed || len(reqs) == 0 || !strings.HasPrefix(reqs[0].Get("id__in"), strconv.Itoa(ids[sent])+",") {
				t.Errorf("next pass: backoff=%d requests=%v, want %d and a first request from id %d",
					v.Backoff, reqs, tc.wantFailed, ids[sent])
			}
		})
	}

	for name, body := range map[string]string{
		"undecodable_row": `{"meta":{},"data":[{"id":"9110"}]}`,
		"no_data_array":   `{}`,
		"null_data":       `{"data":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			up.setRespond(func(int, []int) (int, string, bool) { return http.StatusOK, body, true })
			w, _, logs := newCascadeWorker(t, up, 20)
			seedCascadeRows(t, w.entClient)
			v := w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
			if v.Failed != 3 || len(v.Gone) != 0 || v.Absent != 0 {
				t.Errorf("failed=%d gone=%v absent=%d, want 3, none and 0", v.Failed, v.Gone, v.Absent)
			}
			if e := w.netIxLanVerifyMemo[9110]; e.Outcome != verifyFailed || e.Failures != 1 {
				t.Errorf("memo[9110] = %+v, want failed with Failures=1", e)
			}
			if recs := logs.records(t, "netixlan cascade verification failed, cascade deferred"); len(recs) != 1 {
				t.Errorf("failure log records = %d, want 1", len(recs))
			}
			if recs := logs.records(t, "verified netixlan cascade candidates"); len(recs) != 1 || recs[0]["level"] != "WARN" {
				t.Errorf("summary log = %v, want one WARN record", recs)
			}
		})
	}

	// An error that says nothing about the requested ids stops the pass
	// and writes no memo entry for the rest. The second chunk (ids from
	// 20101) gets the error; the transport may retry a closed connection,
	// so the hook matches the chunk, not the request number.
	for _, tc := range []struct {
		name    string
		respond func(cancel context.CancelFunc) (int, string, bool)
	}{
		{"rate_limited", func(context.CancelFunc) (int, string, bool) {
			return http.StatusTooManyRequests, "", true
		}},
		{"waf_block", func(context.CancelFunc) (int, string, bool) {
			return http.StatusForbidden, "<html>Request blocked by AWS WAF</html>", true
		}},
		{"no_response", func(context.CancelFunc) (int, string, bool) {
			return closeConnection, "", true
		}},
		{"context_done", func(cancel context.CancelFunc) (int, string, bool) {
			cancel()
			return http.StatusInternalServerError, "boom", true
		}},
	} {
		t.Run(tc.name+"_stops_pass", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			up := newCascadeUpstream(t)
			up.setRespond(func(_ int, req []int) (int, string, bool) {
				if req[0] == 20101 {
					return tc.respond(cancel)
				}
				return 0, "", false
			})
			w, _, _ := newCascadeWorker(t, up, 20)
			ids := seedCandidateRows(t, w.entClient, 20001, 250)
			v := w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
			if v.Requests != 2 || v.Absent != 100 || v.Deferred != 150 || v.Failed != 0 || v.Err == nil {
				t.Errorf("requests=%d absent=%d deferred=%d failed=%d err=%v, want 2, 100, 150, 0 and an error",
					v.Requests, v.Absent, v.Deferred, v.Failed, v.Err)
			}
			for _, q := range up.idInRequests() {
				if strings.HasPrefix(q.Get("id__in"), "20201,") {
					t.Errorf("the pass sent the third chunk after the error")
				}
			}
			if !slices.Equal(v.Gone, ids[:100]) {
				t.Errorf("Gone = %d ids, want the 100 ids of chunk 1", len(v.Gone))
			}
			for _, id := range ids[100:] {
				if _, ok := w.netIxLanVerifyMemo[id]; ok {
					t.Fatalf("memo holds deferred id %d", id)
				}
			}
		})
	}

	t.Run("context_done_before_pass", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		up := newCascadeUpstream(t)
		w, _, _ := newCascadeWorker(t, up, 20)
		seedCascadeRows(t, w.entClient)
		v := w.verifyNetIxLanCandidates(ctx, netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
		if up.requestCount() != 0 || v.Requests != 0 || len(v.Gone) != 0 || v.Err == nil {
			t.Errorf("upstream=%d requests=%d gone=%v err=%v, want 0, 0, none and an error",
				up.requestCount(), v.Requests, v.Gone, v.Err)
		}
		if len(w.netIxLanVerifyMemo) != 0 {
			t.Errorf("memo = %v, want empty", w.netIxLanVerifyMemo)
		}
	})

	t.Run("extra_rows_ignored", func(t *testing.T) {
		t.Parallel()
		up := newCascadeUpstream(t)
		extra, _ := json.Marshal(map[string]any{"meta": map[string]any{}, "data": []any{
			cascadeVerdictRow(9100, 910, "ok", cascadeRowTime),
			cascadeVerdictRow(9999, 911, "deleted", cascadeT(1)),
			cascadeVerdictRow(9111, 911, "not-operational", cascadeRowTime),
		}})
		up.setRespond(func(int, []int) (int, string, bool) { return http.StatusOK, string(extra), true })
		w, _, _ := newCascadeWorker(t, up, 20)
		seedCascadeRows(t, w.entClient)
		v := w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
		if !slices.Equal(v.Gone, []int{9110, 9120}) || v.Live != 1 || v.Deleted != 0 {
			t.Errorf("Gone=%v live=%d deleted=%d, want [9110 9120], 1 and 0", v.Gone, v.Live, v.Deleted)
		}
		for _, id := range []int{9100, 9999} {
			if _, ok := w.netIxLanVerifyMemo[id]; ok {
				t.Errorf("memo holds %d, which was not requested", id)
			}
		}
	})

	t.Run("no_candidates", func(t *testing.T) {
		t.Parallel()
		up := newCascadeUpstream(t)
		w, _, logs := newCascadeWorker(t, up, 20)
		v := w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
		if v.Candidates != 0 || v.Requests != 0 || up.requestCount() != 0 {
			t.Errorf("candidates=%d requests=%d upstream=%d, want 0", v.Candidates, v.Requests, up.requestCount())
		}
		if recs := logs.records(t, "verified netixlan cascade candidates"); len(recs) != 1 || recs[0]["level"] != "DEBUG" {
			t.Errorf("summary log = %v, want one DEBUG record", recs)
		}
	})
}

// TestVerifyNetIxLanCandidates_CapAndKillSwitch verifies the request cap
// of a pass: at most 10 requests, fewer when
// PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE is lower, and none when it
// is 0.
func TestVerifyNetIxLanCandidates_CapAndKillSwitch(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name         string
		fkCap        int
		wantRequests int
		wantGone     int
		wantDeferred int
		wantLevel    string
	}{
		{"default_cap", 20, 10, 1000, 50, "WARN"},
		{"lower_fk_cap", 3, 3, 300, 750, "WARN"},
		{"kill_switch", 0, 0, 0, 0, "DEBUG"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			w, _, logs := newCascadeWorker(t, up, tc.fkCap)
			ids := seedCandidateRows(t, w.entClient, 30001, 1050)
			v := w.verifyNetIxLanCandidates(t.Context(), netIxLanVerifyInput{Now: now, Mode: "incremental", CursorKnown: true})
			if got := len(up.idInRequests()); got != tc.wantRequests || v.Requests != tc.wantRequests {
				t.Errorf("requests = %d (counted %d), want %d", got, v.Requests, tc.wantRequests)
			}
			if len(v.Gone) != tc.wantGone || v.Deferred != tc.wantDeferred {
				t.Errorf("gone=%d deferred=%d, want %d and %d", len(v.Gone), v.Deferred, tc.wantGone, tc.wantDeferred)
			}
			if !slices.Equal(v.Gone, ids[:tc.wantGone]) {
				t.Error("Gone is not the lowest ids in ascending order")
			}
			if v.Disabled != (tc.fkCap == 0) {
				t.Errorf("Disabled = %v, want %v", v.Disabled, tc.fkCap == 0)
			}
			recs := logs.records(t, "verified netixlan cascade candidates")
			if len(recs) != 1 || recs[0]["level"] != tc.wantLevel || recs[0]["disabled"] != (tc.fkCap == 0) {
				t.Errorf("summary log = %v, want one %s record with disabled=%v", recs, tc.wantLevel, tc.fkCap == 0)
			}
		})
	}
}

// TestNetIxLanVerifyChunks locks the chunk plan: ids that never failed go
// first in chunks of 100, then ids with one failure in chunks of 10, then
// the rest one by one; the request cap cuts the plan and counts the ids
// left over.
func TestNetIxLanVerifyChunks(t *testing.T) {
	t.Parallel()
	seq := func(first, n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = first + i
		}
		return out
	}
	sizes := func(chunks [][]int) []int {
		out := make([]int, len(chunks))
		for i, c := range chunks {
			out[i] = len(c)
		}
		return out
	}
	failures := map[int]int{}
	for _, id := range seq(1000, 15) {
		failures[id] = 1
	}
	failures[2000] = 2
	failures[2001] = 5
	mixed := slices.Concat(seq(1000, 15), []int{2001, 2000}, seq(1, 150), []int{3000})

	for _, tc := range []struct {
		name         string
		ids          []int
		maxRequests  int
		wantSizes    []int
		wantFirst    []int
		wantDeferred int
	}{
		{"empty", nil, 10, []int{}, nil, 0},
		{"fresh_ids", seq(1, 250), 10, []int{100, 100, 50}, []int{1, 101, 201}, 0},
		{"tiers_in_order", mixed, 10, []int{100, 51, 10, 5, 1, 1}, []int{1, 101, 1000, 1010, 2000, 2001}, 0},
		{"cap_cuts_plan", mixed, 3, []int{100, 51, 10}, []int{1, 101, 1000}, 7},
		{"zero_cap", seq(1, 5), 0, []int{}, nil, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			chunks, deferred := netIxLanVerifyChunks(tc.ids, failures, tc.maxRequests)
			if got := sizes(chunks); !slices.Equal(got, tc.wantSizes) {
				t.Errorf("chunk sizes = %v, want %v", got, tc.wantSizes)
			}
			for i, want := range tc.wantFirst {
				if i < len(chunks) && chunks[i][0] != want {
					t.Errorf("chunk %d starts at %d, want %d", i, chunks[i][0], want)
				}
			}
			if deferred != tc.wantDeferred {
				t.Errorf("deferred = %d, want %d", deferred, tc.wantDeferred)
			}
		})
	}
}

// ageNetIxLanVerifyMemo ends every memo entry of w, as if their TTL or
// backoff had passed. The sync cycle reads the memo clock from
// time.Now, so the sync-level tests age the entries instead.
func ageNetIxLanVerifyMemo(w *Worker) {
	for id, e := range w.netIxLanVerifyMemo {
		e.Until = time.Now().Add(-time.Second)
		w.netIxLanVerifyMemo[id] = e
	}
}
