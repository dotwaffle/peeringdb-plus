package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestNextHistoryWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		typ      string
		progress historyProgress
		maxID    int
		wantWin  historyWindow
		wantNext historyProgress
	}{
		{
			name:     "first window",
			typ:      "org",
			maxID:    4500,
			wantWin:  historyWindow{objectType: "org", fromID: 1, toID: 2001},
			wantNext: historyProgress{nextID: 2001},
		},
		{
			name:     "middle window",
			typ:      "org",
			progress: historyProgress{nextID: 2001},
			maxID:    4500,
			wantWin:  historyWindow{objectType: "org", fromID: 2001, toID: 4001},
			wantNext: historyProgress{nextID: 4001},
		},
		{
			name:     "window that would reach max id is the last one",
			typ:      "org",
			progress: historyProgress{nextID: 4001},
			maxID:    4500,
			wantWin:  historyWindow{objectType: "org", fromID: 4001},
			wantNext: historyProgress{nextID: 4001, done: true},
		},
		{
			name:     "window that ends at max id leaves max id to the next window",
			typ:      "org",
			maxID:    2001,
			wantWin:  historyWindow{objectType: "org", fromID: 1, toID: 2001},
			wantNext: historyProgress{nextID: 2001},
		},
		{
			name:     "empty table",
			typ:      "fac",
			maxID:    0,
			wantWin:  historyWindow{objectType: "fac", fromID: 1},
			wantNext: historyProgress{nextID: 1, done: true},
		},
		{
			name:     "campus takes one request",
			typ:      "campus",
			maxID:    300,
			wantWin:  historyWindow{objectType: "campus", fromID: 1},
			wantNext: historyProgress{nextID: 1, done: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			win, next := nextHistoryWindow(tt.typ, tt.progress, tt.maxID)
			if win != tt.wantWin {
				t.Errorf("window = %+v, want %+v", win, tt.wantWin)
			}
			if next != tt.wantNext {
				t.Errorf("next = %+v, want %+v", next, tt.wantNext)
			}
		})
	}
}

func TestHistoryWindowParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		win  historyWindow
		want string
	}{
		{
			win:  historyWindow{objectType: "org", fromID: 1, toID: 2001},
			want: "depth=0&id__gte=1&id__lt=2001&since=1&status=deleted",
		},
		{
			win:  historyWindow{objectType: "netixlan", fromID: 4801},
			want: "depth=0&hide_ix_no_fac=0&id__gte=4801&since=1&status=deleted",
		},
		{
			win:  historyWindow{objectType: "campus", fromID: 1},
			want: "depth=0&id__gte=1&since=1",
		},
	}
	for _, tt := range tests {
		if got := tt.win.params().Encode(); got != tt.want {
			t.Errorf("%+v: params = %q, want %q", tt.win, got, tt.want)
		}
	}
}

// TestHistorySweepTypes checks that the sweep covers every type except
// poc, in step order, and that each type has a table.
func TestHistorySweepTypes(t *testing.T) {
	t.Parallel()
	want := slices.DeleteFunc(slices.Clone(canonicalStepOrder), func(s string) bool { return s == "poc" })
	if !slices.Equal(historySweepTypes, want) {
		t.Errorf("historySweepTypes = %v, want %v", historySweepTypes, want)
	}
	for name := range historySpecs {
		if _, ok := entityTables[name]; !ok {
			t.Errorf("historySpecs type %q has no entityTables entry", name)
		}
	}
}

func TestLogHistoryCommitted(t *testing.T) {
	t.Parallel()
	allDone := make(map[string]historyProgress, len(historySweepTypes))
	for _, name := range historySweepTypes {
		allDone[name] = historyProgress{nextID: 1, done: true}
	}
	tests := []struct {
		name string
		s    *historySweep
		want []string
	}{
		{name: "nil", s: nil},
		{name: "no requests", s: &historySweep{progress: map[string]historyProgress{}, stop: "budget"}},
		{
			name: "restart with no requests",
			s:    &historySweep{restart: true, progress: map[string]historyProgress{}, stop: "memo"},
			want: []string{"history sweep restarted"},
		},
		{
			name: "progress",
			s:    &historySweep{progress: map[string]historyProgress{"org": {nextID: 2001}}, requests: 1, stop: "budget"},
			want: []string{"history sweep progress"},
		},
		{
			name: "last window",
			s:    &historySweep{progress: allDone, requests: 1, stop: "complete"},
			want: []string{"history sweep progress", "history sweep complete"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger()
			logHistoryCommitted(t.Context(), logger, tt.s)
			var got []string
			for _, msg := range []string{"history sweep restarted", "history sweep progress", "history sweep complete"} {
				if len(logs.records(t, msg)) > 0 {
					got = append(got, msg)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("logged %v, want %v", got, tt.want)
			}
		})
	}
}

// historyServer is an upstream stub. It serves the normal fetch of a
// sync cycle from live, and each history window (a request with id__gte)
// from the deleted rows in its id range. It records the query of each
// history window.
type historyServer struct {
	srv *httptest.Server

	mu      stdsync.Mutex
	live    map[string][]map[string]any
	deleted map[string][]map[string]any
	// fail maps "<type>:<id__gte>" to the HTTP status of that window.
	fail    map[string]int
	windows []string
}

func newHistoryServer(t *testing.T) *historyServer {
	t.Helper()
	hs := &historyServer{
		live:    make(map[string][]map[string]any),
		deleted: make(map[string][]map[string]any),
		fail:    make(map[string]int),
	}
	hs.srv = httptest.NewServer(http.HandlerFunc(hs.serveHTTP))
	t.Cleanup(hs.srv.Close)
	return hs
}

func (hs *historyServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	objType := strings.TrimPrefix(r.URL.Path, "/api/")
	q := r.URL.Query()
	hs.mu.Lock()
	defer hs.mu.Unlock()
	data := []map[string]any{}
	switch {
	case q.Has("id__gte"):
		hs.windows = append(hs.windows, objType+"?"+r.URL.RawQuery)
		if code := hs.fail[objType+":"+q.Get("id__gte")]; code != 0 {
			if code == http.StatusTooManyRequests {
				w.Header().Set("Retry-After", "2200")
			}
			w.WriteHeader(code)
			return
		}
		from, _ := strconv.Atoi(q.Get("id__gte"))
		to := math.MaxInt
		if v := q.Get("id__lt"); v != "" {
			to, _ = strconv.Atoi(v)
		}
		for _, row := range hs.deleted[objType] {
			if id := row["id"].(int); id >= from && id < to {
				data = append(data, row)
			}
		}
	case q.Get("skip") == "" || q.Get("skip") == "0":
		data = append(data, hs.live[objType]...)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": data})
}

// sentWindows returns the queries of the history windows sent so far.
func (hs *historyServer) sentWindows() []string {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return slices.Clone(hs.windows)
}

func (hs *historyServer) setFail(key string, code int) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	if code == 0 {
		delete(hs.fail, key)
		return
	}
	hs.fail[key] = code
}

func (hs *historyServer) setLive(objType string, rows ...map[string]any) {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	hs.live[objType] = rows
}

// newHistoryWorker returns a worker with a history budget of budget
// requests per cycle, over a fresh database.
func newHistoryWorker(t *testing.T, hs *historyServer, budget int) (*Worker, *sql.DB, *logBuffer) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	logger, logs := newLogger()
	w := NewWorker(newFastPDBClient(t, hs.srv.URL), client, db, WorkerConfig{
		HistoryMaxRequestsPerCycle: budget,
	}, logger)
	return w, db, logs
}

// mustSync runs one sync cycle in mode and fails the test on an error.
func mustSync(t *testing.T, w *Worker, mode config.SyncMode) {
	t.Helper()
	if err := w.Sync(t.Context(), mode); err != nil {
		t.Fatalf("sync (%s): %v", mode, err)
	}
}

// historyRows reads the sync_history_sweep table as "type:next_id:done".
func historyRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT type, next_id, done FROM sync_history_sweep ORDER BY type`)
	if err != nil {
		t.Fatalf("read sync_history_sweep: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var (
			name       string
			next, done int
		)
		if err := rows.Scan(&name, &next, &done); err != nil {
			t.Fatalf("scan sync_history_sweep: %v", err)
		}
		out = append(out, name+":"+strconv.Itoa(next)+":"+strconv.Itoa(done))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sync_history_sweep: %v", err)
	}
	return out
}

// assertOrg checks the stored status and updated of an organization.
// An empty status means that the row must not exist.
func assertOrg(t *testing.T, w *Worker, id int, status, updated string) {
	t.Helper()
	org, err := w.entClient.Organization.Get(t.Context(), id)
	if status == "" {
		if err == nil {
			t.Errorf("org %d stored (status %q), want absent", id, org.Status)
		}
		return
	}
	if err != nil {
		t.Fatalf("get org %d: %v", id, err)
	}
	if org.Status != status || org.Updated.UTC().Format(time.RFC3339) != updated {
		t.Errorf("org %d = %s updated %s, want %s updated %s",
			id, org.Status, org.Updated.UTC().Format(time.RFC3339), status, updated)
	}
}

// orgWindows are the queries of the three org windows when the largest
// stored org id is 4500.
var orgWindows = []string{
	"org?depth=0&id__gte=1&id__lt=2001&since=1&status=deleted",
	"org?depth=0&id__gte=2001&id__lt=4001&since=1&status=deleted",
	"org?depth=0&id__gte=4001&since=1&status=deleted",
}

// seedHistoryOrgs gives hs live orgs 1, 2, 7 and 4500 (the org cursor is
// 2024-06-01) and org tombstones in all three org windows.
func seedHistoryOrgs(hs *historyServer) {
	hs.setLive("org",
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
		makeOrg(7, "Org7", "ok"),
		bumpUpdated(makeOrg(4500, "Org4500", "ok"), "2024-06-01T00:00:00Z"),
	)
	hs.deleted["org"] = []map[string]any{
		bumpUpdated(makeOrg(2, "Org2", "deleted"), "2024-03-01T00:00:00Z"),
		bumpUpdated(makeOrg(7, "Org7", "deleted"), "2024-04-01T00:00:00Z"),
		bumpUpdated(makeOrg(10, "Org10", "deleted"), "2023-06-01T00:00:00Z"),
		bumpUpdated(makeOrg(3000, "Org3000", "deleted"), "2023-07-01T00:00:00Z"),
		bumpUpdated(makeOrg(5000, "Org5000", "deleted"), "2024-02-01T00:00:00Z"),
		bumpUpdated(makeOrg(9999, "Org9999", "deleted"), "2099-01-01T00:00:00Z"),
	}
}

// TestSync_HistorySweepLandsTombstones runs the sweep over the org
// windows with a budget of two requests per cycle.
func TestSync_HistorySweepLandsTombstones(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	seedHistoryOrgs(hs)
	w, db, logs := newHistoryWorker(t, hs, 2)

	// The first cycle bootstraps: every cursor is zero, so the sweep
	// sends nothing.
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 0 {
		t.Fatalf("first cycle sent history windows %v, want none", got)
	}

	// Org 7 changes after the bootstrap. The normal fetch of the next
	// cycle stages the new version, and the history row (older) must not
	// replace it in scratch.
	hs.setLive("org",
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
		bumpUpdated(makeOrg(7, "Org7", "ok"), "2024-05-01T00:00:00Z"),
		bumpUpdated(makeOrg(4500, "Org4500", "ok"), "2024-06-01T00:00:00Z"),
	)
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); !slices.Equal(got, orgWindows[:2]) {
		t.Fatalf("second cycle windows = %v, want %v", got, orgWindows[:2])
	}
	assertOrg(t, w, 10, "deleted", "2023-06-01T00:00:00Z")
	assertOrg(t, w, 2, "deleted", "2024-03-01T00:00:00Z")
	assertOrg(t, w, 7, "ok", "2024-05-01T00:00:00Z")
	assertOrg(t, w, 3000, "deleted", "2023-07-01T00:00:00Z")
	assertOrg(t, w, 5000, "", "")
	if got, want := historyRows(t, db), []string{"org:4001:0"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
	recs := logs.records(t, "history sweep progress")
	if len(recs) != 1 {
		t.Fatalf("got %d progress logs, want 1", len(recs))
	}
	if r := recs[0]; r["requests"] != 2.0 || r["rows"] != 4.0 || r["dropped"] != 0.0 ||
		r["stop"] != "budget" || r["next_type"] != "org" || r["next_id"] != 4001.0 {
		t.Errorf("progress log = %v", r)
	}

	// The third cycle sends the last org window, which has no upper
	// bound. It drops org 9999, whose updated is later than the org
	// cursor. The sweep stops after the last window of a type.
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); !slices.Equal(got, orgWindows) {
		t.Fatalf("windows after third cycle = %v, want %v", got, orgWindows)
	}
	assertOrg(t, w, 5000, "deleted", "2024-02-01T00:00:00Z")
	assertOrg(t, w, 9999, "", "")
	if got, want := historyRows(t, db), []string{"org:4001:1"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
	recs = logs.records(t, "history sweep progress")
	if r := recs[len(recs)-1]; r["rows"] != 1.0 || r["dropped"] != 1.0 ||
		r["stop"] != "type_done" || r["next_type"] != "campus" {
		t.Errorf("progress log = %v", r)
	}
}

// TestSync_HistorySweepTypeOrder checks that the next type starts in a
// later cycle than the last window of its parent type, so a swept child
// finds its swept parent with FK backfill off. It also checks that a type
// with an empty table (campus) does not stop the later types.
func TestSync_HistorySweepTypeOrder(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	hs.setLive("org", makeOrg(1, "Org1", "ok"))
	hs.setLive("fac", makeFac(1, 1, "Fac1", "ok"))
	hs.deleted["org"] = []map[string]any{
		bumpUpdated(makeOrg(2, "Org2", "deleted"), "2023-06-01T00:00:00Z"),
	}
	hs.deleted["fac"] = []map[string]any{
		bumpUpdated(makeFac(3, 2, "Fac3", "deleted"), "2023-07-01T00:00:00Z"),
	}
	w, db, logs := newHistoryWorker(t, hs, 5)

	mustSync(t, w, config.SyncModeIncremental)
	mustSync(t, w, config.SyncModeIncremental)
	orgWindow := "org?depth=0&id__gte=1&since=1&status=deleted"
	if got, want := hs.sentWindows(), []string{orgWindow}; !slices.Equal(got, want) {
		t.Fatalf("second cycle windows = %v, want %v", got, want)
	}
	assertOrg(t, w, 2, "deleted", "2023-06-01T00:00:00Z")

	// campus is empty and waits. fac goes on, and fac 3 finds org 2.
	mustSync(t, w, config.SyncModeIncremental)
	facWindow := "fac?depth=0&id__gte=1&since=1&status=deleted"
	if got, want := hs.sentWindows(), []string{orgWindow, facWindow}; !slices.Equal(got, want) {
		t.Fatalf("third cycle windows = %v, want %v", got, want)
	}
	fac, err := w.entClient.Facility.Get(t.Context(), 3)
	if err != nil {
		t.Fatalf("get fac 3: %v", err)
	}
	if fac.Status != "deleted" {
		t.Errorf("fac 3 status = %q, want deleted", fac.Status)
	}
	if got, want := historyRows(t, db), []string{"fac:1:1", "org:1:1"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
	recs := logs.records(t, "history sweep progress")
	if r := recs[len(recs)-1]; r["stop"] != "type_done" || r["next_type"] != "campus" {
		t.Errorf("progress log = %v", r)
	}

	// Every type that is not done has an empty table.
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 2 {
		t.Errorf("fourth cycle sent windows: %v", got)
	}
	if n := len(logs.records(t, "history sweep complete")); n != 0 {
		t.Errorf("got %d complete logs, want 0 (campus is not done)", n)
	}
}

// TestSync_HistorySweepStopsOnRateLimit checks that a 429 stops the
// sweep without failing the cycle, and that the memo keeps the window
// from being sent again within historyMemoTTL.
func TestSync_HistorySweepStopsOnRateLimit(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	seedHistoryOrgs(hs)
	hs.setFail("org:1", http.StatusTooManyRequests)
	w, db, logs := newHistoryWorker(t, hs, 5)

	mustSync(t, w, config.SyncModeIncremental)
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); !slices.Equal(got, orgWindows[:1]) {
		t.Fatalf("windows = %v, want %v", got, orgWindows[:1])
	}
	if n := len(logs.records(t, "history sweep stopped by upstream rate limit")); n != 1 {
		t.Errorf("got %d rate-limit WARNs, want 1", n)
	}
	if got := historyRows(t, db); len(got) != 0 {
		t.Errorf("progress = %v, want none", got)
	}
	assertOrg(t, w, 10, "", "")

	// Upstream recovers, but the window was sent less than
	// historyMemoTTL ago.
	hs.setFail("org:1", 0)
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); len(got) != 1 {
		t.Fatalf("memo did not hold the window: windows = %v", got)
	}

	// After the memo TTL the sweep sends the window again.
	for k := range w.historyMemo {
		w.historyMemo[k] = time.Now().Add(-historyMemoTTL)
	}
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := hs.sentWindows(), []string{orgWindows[0], orgWindows[0], orgWindows[1], orgWindows[2]}; !slices.Equal(got, want) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
	assertOrg(t, w, 10, "deleted", "2023-06-01T00:00:00Z")
	if got, want := historyRows(t, db), []string{"org:4001:1"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
}

// TestSync_HistorySweepFailureKeepsEarlierWindows checks that a window
// that fails stops the sweep without failing the cycle, and that the
// windows before it keep their rows and progress.
func TestSync_HistorySweepFailureKeepsEarlierWindows(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	seedHistoryOrgs(hs)
	hs.setFail("org:2001", http.StatusBadGateway)
	w, db, logs := newHistoryWorker(t, hs, 5)

	mustSync(t, w, config.SyncModeIncremental)
	mustSync(t, w, config.SyncModeIncremental)
	assertOrg(t, w, 10, "deleted", "2023-06-01T00:00:00Z")
	assertOrg(t, w, 3000, "", "")
	if got, want := historyRows(t, db), []string{"org:2001:0"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
	if n := len(logs.records(t, "history sweep stopped: window failed")); n != 1 {
		t.Errorf("got %d window-failed WARNs, want 1", n)
	}
}

// TestSync_HistorySweepOff checks that the sweep sends nothing in a full
// cycle and when its budget is 0.
func TestSync_HistorySweepOff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		budget int
		mode   config.SyncMode
	}{
		{name: "full cycle", budget: 5, mode: config.SyncModeFull},
		{name: "budget 0", budget: 0, mode: config.SyncModeIncremental},
		{name: "budget 0 restart", budget: 0, mode: config.SyncModeHistory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hs := newHistoryServer(t)
			seedHistoryOrgs(hs)
			w, db, _ := newHistoryWorker(t, hs, tt.budget)
			mustSync(t, w, config.SyncModeIncremental)
			mustSync(t, w, tt.mode)
			if got := hs.sentWindows(); len(got) != 0 {
				t.Errorf("windows = %v, want none", got)
			}
			if got := historyRows(t, db); len(got) != 0 {
				t.Errorf("progress = %v, want none", got)
			}
		})
	}
}

// TestSync_HistorySweepRestart checks that POST /sync?mode=history
// deletes the stored progress and starts again at the first window.
func TestSync_HistorySweepRestart(t *testing.T) {
	t.Parallel()
	hs := newHistoryServer(t)
	seedHistoryOrgs(hs)
	w, db, logs := newHistoryWorker(t, hs, 5)

	mustSync(t, w, config.SyncModeIncremental)
	mustSync(t, w, config.SyncModeIncremental)
	if got := hs.sentWindows(); !slices.Equal(got, orgWindows) {
		t.Fatalf("windows = %v, want %v", got, orgWindows)
	}
	// A stale row of another type must go too.
	if _, err := db.ExecContext(t.Context(),
		`INSERT INTO sync_history_sweep (type, next_id, done, updated_at) VALUES ('fac', 99, 0, 'x')`); err != nil {
		t.Fatalf("insert stale progress: %v", err)
	}

	for k := range w.historyMemo {
		w.historyMemo[k] = time.Now().Add(-historyMemoTTL)
	}
	mustSync(t, w, config.SyncModeHistory)
	if got, want := hs.sentWindows(), slices.Concat(orgWindows, orgWindows); !slices.Equal(got, want) {
		t.Fatalf("windows = %v, want %v", got, want)
	}
	if got, want := historyRows(t, db), []string{"org:4001:1"}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
	if n := len(logs.records(t, "history sweep restarted")); n != 1 {
		t.Errorf("got %d restart logs, want 1", n)
	}
	var mode string
	if err := db.QueryRowContext(t.Context(),
		`SELECT mode FROM sync_status ORDER BY id DESC LIMIT 1`).Scan(&mode); err != nil {
		t.Fatalf("read sync_status mode: %v", err)
	}
	if mode != string(config.SyncModeIncremental) {
		t.Errorf("sync_status mode = %q, want %q", mode, config.SyncModeIncremental)
	}
}

// TestResolveEffectiveMode_HistoryNeverFull checks that a history cycle
// is incremental even when a full cycle is due.
func TestResolveEffectiveMode_HistoryNeverFull(t *testing.T) {
	t.Parallel()
	w, _ := newTestWorkerWithMode(t, "http://127.0.0.1:0", config.SyncModeIncremental)
	w.config.FullSyncInterval = time.Hour
	ctx := context.Background()
	if got := w.resolveEffectiveMode(ctx, config.SyncModeIncremental); got != config.SyncModeFull {
		t.Fatalf("incremental with no full sync = %q, want full (test premise)", got)
	}
	if got := w.resolveEffectiveMode(ctx, config.SyncModeHistory); got != config.SyncModeIncremental {
		t.Errorf("history = %q, want incremental", got)
	}
}
