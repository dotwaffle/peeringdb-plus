package sync

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// wmUpstream is an upstream stub that follows the list rules of
// PeeringDB. A bare list holds the live rows in id order. A ?since=N list
// holds every row whose updated is N or later, in updated order. An
// id__in list holds the requested rows in any status. The stub records
// every request.
type wmUpstream struct {
	srv *httptest.Server

	mu   stdsync.Mutex
	rows map[string]map[int]map[string]any
	// requests holds "<type>?<query>" of each request, in order.
	requests []string
	// after runs once, with mu held, after the first list request (not
	// id__in) of afterType.
	afterType string
	after     func(u *wmUpstream)
	// afterIDs runs once, with mu held, after the first id__in request
	// of afterIDsType.
	afterIDsType string
	afterIDs     func(u *wmUpstream)
	// failSince maps a type to the HTTP status of its ?since= list
	// requests (not id__in).
	failSince map[string]int
}

func newWatermarkUpstream(t *testing.T) *wmUpstream {
	t.Helper()
	u := &wmUpstream{
		rows:      make(map[string]map[int]map[string]any),
		failSince: make(map[string]int),
	}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serveHTTP))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *wmUpstream) serveHTTP(w http.ResponseWriter, r *http.Request) {
	objType := strings.TrimPrefix(r.URL.Path, "/api/")
	q := r.URL.Query()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests = append(u.requests, objType+"?"+r.URL.RawQuery)
	isList := !q.Has("id__in")
	if code := u.failSince[objType]; code != 0 && isList && q.Has("since") {
		w.WriteHeader(code)
		return
	}
	data := u.list(objType, q)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": data})
	if isList && objType == u.afterType && u.after != nil {
		fn := u.after
		u.after = nil
		fn(u)
	}
	if !isList && objType == u.afterIDsType && u.afterIDs != nil {
		fn := u.afterIDs
		u.afterIDs = nil
		fn(u)
	}
}

// list returns the rows of one response. mu must be held.
func (u *wmUpstream) list(objType string, q url.Values) []map[string]any {
	if skip := q.Get("skip"); skip != "" && skip != "0" {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if idIn := q.Get("id__in"); idIn != "" {
		for s := range strings.SplitSeq(idIn, ",") {
			id, _ := strconv.Atoi(s)
			if row, ok := u.rows[objType][id]; ok {
				out = append(out, row)
			}
		}
		return out
	}
	since := int64(-1)
	if v := q.Get("since"); v != "" {
		since, _ = strconv.ParseInt(v, 10, 64)
	}
	for _, row := range u.rows[objType] {
		status := row["status"].(string)
		switch {
		case since >= 0:
			if rowUpdated(row).Unix() < since {
				continue
			}
		case status != "ok" && (objType != "netixlan" || status != "not-operational"):
			continue
		}
		out = append(out, row)
	}
	slices.SortFunc(out, func(a, b map[string]any) int {
		if since >= 0 {
			if c := rowUpdated(a).Compare(rowUpdated(b)); c != 0 {
				return c
			}
		}
		return cmp.Compare(a["id"].(int), b["id"].(int))
	})
	return out
}

func rowUpdated(row map[string]any) time.Time {
	t, _ := time.Parse(time.RFC3339, row["updated"].(string))
	return t
}

// put stores rows of objType, replacing rows with the same id.
func (u *wmUpstream) put(objType string, rows ...map[string]any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.putLocked(objType, rows...)
}

func (u *wmUpstream) putLocked(objType string, rows ...map[string]any) {
	if u.rows[objType] == nil {
		u.rows[objType] = make(map[int]map[string]any)
	}
	for _, row := range rows {
		u.rows[objType][row["id"].(int)] = row
	}
}

// afterFirstList makes fn run once, after the next list request of
// objType is served.
func (u *wmUpstream) afterFirstList(objType string, fn func(u *wmUpstream)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.afterType, u.after = objType, fn
}

// afterFirstIDs makes fn run once, after the next id__in request of
// objType is served.
func (u *wmUpstream) afterFirstIDs(objType string, fn func(u *wmUpstream)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.afterIDsType, u.afterIDs = objType, fn
}

// mark returns the number of requests sent so far.
func (u *wmUpstream) mark() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

// requestsFrom returns the requests sent after mark.
func (u *wmUpstream) requestsFrom(mark int) []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.requests[mark:])
}

// listSince returns the since value of each first-page list request of
// objType sent after mark. A bare list gives "".
func (u *wmUpstream) listSince(mark int, objType string) []string {
	var out []string
	for _, r := range u.requestsFrom(mark) {
		typ, query, _ := strings.Cut(r, "?")
		q, _ := url.ParseQuery(query)
		if typ != objType || q.Has("id__in") || (q.Get("skip") != "" && q.Get("skip") != "0") {
			continue
		}
		out = append(out, q.Get("since"))
	}
	return out
}

// newWatermarkWorker returns an incremental worker over a fresh database
// with a FK backfill cap of fkCap requests. It logs JSON at DEBUG to the
// returned buffer.
func newWatermarkWorker(t *testing.T, u *wmUpstream, fkCap int) (*Worker, *sql.DB, *logBuffer) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	logger, logs := newLogger()
	w := NewWorker(newFastPDBClient(t, u.srv.URL), client, db, WorkerConfig{
		SyncMode:                      config.SyncModeIncremental,
		FKBackfillMaxRequestsPerCycle: fkCap,
	}, logger)
	return w, db, logs
}

// watermarkRows returns the stored watermarks as Unix seconds by type.
func watermarkRows(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT type, max_updated FROM sync_watermark`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int64)
	for rows.Next() {
		var (
			name string
			mark int64
		)
		if err := rows.Scan(&name, &mark); err != nil {
			return nil, err
		}
		out[name] = mark
	}
	return out, rows.Err()
}

// assertWatermark checks the stored watermark of objType. An empty want
// means that the type must have no row.
func assertWatermark(t *testing.T, db *sql.DB, objType, want string) {
	t.Helper()
	marks, err := watermarkRows(t.Context(), db)
	if err != nil {
		t.Errorf("read sync_watermark: %v", err)
		return
	}
	got, ok := marks[objType]
	switch {
	case want == "" && ok:
		t.Errorf("watermark of %s = %s, want no row", objType, time.Unix(got, 0).UTC().Format(time.RFC3339))
	case want == "":
	case !ok:
		t.Errorf("watermark of %s: no row, want %s", objType, want)
	default:
		if g := time.Unix(got, 0).UTC().Format(time.RFC3339); g != want {
			t.Errorf("watermark of %s = %s, want %s", objType, g, want)
		}
	}
}

// wmTime parses an RFC 3339 time.
func wmTime(t *testing.T, rfc3339 string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse %q: %v", rfc3339, err)
	}
	return ts
}

// unixOf returns the Unix seconds of an RFC 3339 time as a string, the
// form of a since parameter.
func unixOf(t *testing.T, rfc3339 string) string {
	t.Helper()
	return strconv.FormatInt(wmTime(t, rfc3339).Unix(), 10)
}

// fixtureRows returns the rows of testdata/fixtures/<objType>.json, with
// each id as an int (wmUpstream sorts on it). Each call reads the file
// again, so the caller can change the rows.
func fixtureRows(t *testing.T, objType string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", objType+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", objType, err)
	}
	var doc struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode fixture %s: %v", objType, err)
	}
	for _, row := range doc.Data {
		row["id"] = int(row["id"].(float64))
	}
	return doc.Data
}

// fixtureRow returns a copy of fixture row id of objType with the key,
// value pairs of kv set.
func fixtureRow(t *testing.T, objType string, id int, kv ...any) map[string]any {
	t.Helper()
	for _, row := range fixtureRows(t, objType) {
		if row["id"] == id {
			return with(row, kv...)
		}
	}
	t.Fatalf("fixture %s has no row %d", objType, id)
	return nil
}

// with returns a copy of row with the key, value pairs of kv set.
func with(row map[string]any, kv ...any) map[string]any {
	out := maps.Clone(row)
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

// newestLive returns the newest updated among the rows of objType that a
// bare list returns, or zero time when there is none.
func (u *wmUpstream) newestLive(objType string) time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	var newest time.Time
	for _, row := range u.rows[objType] {
		status := row["status"].(string)
		if status != "ok" && (objType != "netixlan" || status != "not-operational") {
			continue
		}
		if ts := rowUpdated(row); ts.After(newest) {
			newest = ts
		}
	}
	return newest
}

// setFailSince makes the ?since= list requests of objType fail with code.
// A zero code clears the failure.
func (u *wmUpstream) setFailSince(objType string, code int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.failSince[objType] = code
}

// Times of the watermark scenarios, oldest first.
const (
	wmT0 = "2024-01-01T00:00:00Z"
	wmT1 = "2024-02-01T00:00:00Z"
	wmT2 = "2024-03-01T00:00:00Z"
	wmT3 = "2024-04-01T00:00:00Z"
	wmT4 = "2024-05-01T00:00:00Z"
)

// TestSync_BackfilledParentKeepsCursor is the defect that the watermark
// fixes. FK backfill lands org 2 (updated t3) after the org fetch of the
// cycle. Upstream deleted org 3 (updated t2) between the org fetch and
// the backfill. With a MAX(updated) cursor, the next org fetch starts at
// t3, so it never returns the tombstone of org 3, and org 3 stays live.
// With the watermark, the next org fetch starts at t1, the end of the
// org fetch that the cycle made.
func TestSync_BackfilledParentKeepsCursor(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(3, "Org3", "ok"), wmT1))
	w, db, logs := newWatermarkWorker(t, u, 5)

	mustSync(t, w, config.SyncModeIncremental)
	assertOrg(t, w, 3, "ok", wmT1)
	assertWatermark(t, db, "org", wmT1)

	runBackfillCycle(t, u, w)
	assertOrg(t, w, 2, "ok", wmT3)
	assertOrg(t, w, 3, "ok", wmT1)
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "net", wmT3)
	behind := logs.records(t, "sync watermark behind backfilled rows")
	if len(behind) != 1 || behind[0]["type"] != "org" ||
		behind[0]["watermark"] != wmT1 || behind[0]["max_updated"] != wmT3 {
		t.Errorf("behind logs after cycle 2 = %v, want one for org at %s, max_updated %s", behind, wmT1, wmT3)
	}

	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 3 org since = %q, want %q (the end of the cycle 2 org fetch)", got, want)
	}
	assertOrg(t, w, 3, "deleted", wmT2)
	assertWatermark(t, db, "org", wmT3)

	held := logs.records(t, "sync cursor held behind newest row")
	wantBehind := float64(wmTime(t, wmT3).Unix() - wmTime(t, wmT1).Unix())
	if len(held) != 1 || held[0]["type"] != "org" || held[0]["cursor"] != wmT1 ||
		held[0]["max_updated"] != wmT3 || held[0]["behind_seconds"] != wantBehind {
		t.Errorf("held logs = %v, want one for org at %s, max_updated %s, behind %v s", held, wmT1, wmT3, wantBehind)
	}
	if n := len(logs.records(t, "sync watermark behind backfilled rows")); n != 1 {
		t.Errorf("behind logs after cycle 3 = %d, want still 1", n)
	}
	if recs := logs.records(t, "sync watermark missing, using MAX(updated)"); len(recs) != 0 {
		t.Errorf("missing logs = %v, want none: every table with rows has a watermark", recs)
	}
}

// runBackfillCycle runs cycle 2 of TestSync_BackfilledParentKeepsCursor.
// After the org fetch, upstream deletes org 3 (updated t2) and adds org 2
// (updated t3) with net 11 under it, so FK backfill lands org 2. It
// checks that the cycle fetched org from t1 and sent the backfill
// request.
func runBackfillCycle(t *testing.T, u *wmUpstream, w *Worker) {
	t.Helper()
	org3 := bumpUpdated(makeOrg(3, "Org3", "deleted"), wmT2)
	org2 := bumpUpdated(makeOrg(2, "Org2", "ok"), wmT3)
	net11 := bumpUpdated(makeNet(11, 2, 64511, "Net11", "ok"), wmT3)
	u.afterFirstList("org", func(u *wmUpstream) {
		u.putLocked("org", org3, org2)
		u.putLocked("net", net11)
	})
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 2 org since = %q, want %q", got, want)
	}
	if !slices.Contains(u.requestsFrom(mark), "org?id__in=2&since=1") {
		t.Fatalf("cycle 2 sent no backfill request for org 2: %q", u.requestsFrom(mark))
	}
}

// TestSync_EmptyTableBackfillKeepsOldestRow covers a type whose table is
// empty at the start of a cycle and whose only rows come from two FK
// backfill requests of that cycle. Upstream deletes org 2 between the two
// requests, and org 3 has a later updated. The watermark must stay at
// org 2, so the next cycle fetches the tombstone of org 2.
func TestSync_EmptyTableBackfillKeepsOldestRow(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	w, db, _ := newWatermarkWorker(t, u, 5)

	// The org list of cycle 1 is empty. The fac step then backfills org 2
	// and the net step backfills org 3.
	u.afterFirstList("org", func(u *wmUpstream) {
		u.putLocked("org", bumpUpdated(makeOrg(2, "Org2", "ok"), wmT1))
		u.putLocked("fac", bumpUpdated(makeFac(21, 2, "Fac21", "ok"), wmT1))
		u.putLocked("net", bumpUpdated(makeNet(11, 3, 64511, "Net11", "ok"), wmT3))
	})
	u.afterFirstIDs("org", func(u *wmUpstream) {
		u.putLocked("org",
			bumpUpdated(makeOrg(2, "Org2", "deleted"), wmT2),
			bumpUpdated(makeOrg(3, "Org3", "ok"), wmT3))
	})
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	reqs := u.requestsFrom(mark)
	for _, want := range []string{"org?id__in=2&since=1", "org?id__in=3&since=1"} {
		if !slices.Contains(reqs, want) {
			t.Fatalf("cycle 1 sent no backfill request %q: %q", want, reqs)
		}
	}
	assertOrg(t, w, 2, "ok", wmT1)
	assertOrg(t, w, 3, "ok", wmT3)
	assertWatermark(t, db, "org", wmT1)

	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 2 org since = %q, want %q (the oldest backfilled org)", got, want)
	}
	assertOrg(t, w, 2, "deleted", wmT2)
}

// TestSync_FacCampusBackfillKeepsCampusCursor is the defect of
// TestSync_BackfilledParentKeepsCursor through the campus prefetch of the
// fac step (prefetchStagedFacCampuses). The backfilled campus 2 must not
// move the campus cursor past the tombstone of campus 3.
func TestSync_FacCampusBackfillKeepsCampusCursor(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
	u.put("campus",
		fixtureRow(t, "campus", 1, "updated", wmT0),
		fixtureRow(t, "campus", 1, "id", 3, "name", "Campus3", "updated", wmT1))
	w, db, _ := newWatermarkWorker(t, u, 5)
	ctx := t.Context()

	mustSync(t, w, config.SyncModeIncremental)
	assertWatermark(t, db, "campus", wmT1)

	campus3 := fixtureRow(t, "campus", 1, "id", 3, "name", "Campus3", "status", "deleted", "updated", wmT2)
	campus2 := fixtureRow(t, "campus", 1, "id", 2, "name", "Campus2", "updated", wmT3)
	fac2 := with(bumpUpdated(makeFac(2, 1, "Fac2", "ok"), wmT3), "campus_id", 2)
	u.afterFirstList("campus", func(u *wmUpstream) {
		u.putLocked("campus", campus3, campus2)
		u.putLocked("fac", fac2)
	})
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if !slices.Contains(u.requestsFrom(mark), "campus?id__in=2&since=1") {
		t.Fatalf("cycle 2 sent no backfill request for campus 2: %q", u.requestsFrom(mark))
	}
	if fac := w.entClient.Facility.GetX(ctx, 2); fac.CampusID == nil || *fac.CampusID != 2 {
		t.Errorf("fac 2 campus_id = %v, want 2", fac.CampusID)
	}
	assertWatermark(t, db, "campus", wmT1)

	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "campus"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 3 campus since = %q, want %q", got, want)
	}
	if c := w.entClient.Campus.GetX(ctx, 3); c.Status != "deleted" {
		t.Errorf("campus 3 status = %q, want deleted", c.Status)
	}
	assertWatermark(t, db, "campus", wmT3)
}

// TestSync_GrandparentBackfillKeepsCursor covers a chained backfill:
// carrierfac 2 needs carrier 2, and carrier 2 needs org 2. Both parents
// land after the fetch of their own type, so neither cursor may move past
// the tombstone of its type.
func TestSync_GrandparentBackfillKeepsCursor(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(3, "Org3", "ok"), wmT1))
	u.put("fac", bumpUpdated(makeFac(1, 1, "Fac1", "ok"), wmT0))
	u.put("carrier",
		fixtureRow(t, "carrier", 1, "updated", wmT0),
		fixtureRow(t, "carrier", 1, "id", 3, "name", "Carrier3", "updated", wmT1))
	w, db, _ := newWatermarkWorker(t, u, 5)
	ctx := t.Context()

	mustSync(t, w, config.SyncModeIncremental)
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "carrier", wmT1)

	org3 := bumpUpdated(makeOrg(3, "Org3", "deleted"), wmT2)
	org2 := bumpUpdated(makeOrg(2, "Org2", "ok"), wmT3)
	carrier3 := fixtureRow(t, "carrier", 1, "id", 3, "name", "Carrier3", "status", "deleted", "updated", wmT2)
	carrier2 := fixtureRow(t, "carrier", 1, "id", 2, "org_id", 2, "name", "Carrier2", "updated", wmT3)
	carrierfac2 := fixtureRow(t, "carrierfac", 1, "id", 2, "carrier_id", 2, "updated", wmT3)
	u.afterFirstList("carrier", func(u *wmUpstream) {
		u.putLocked("org", org3, org2)
		u.putLocked("carrier", carrier3, carrier2)
		u.putLocked("carrierfac", carrierfac2)
	})
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	for _, req := range []string{"carrier?id__in=2&since=1", "org?id__in=2&since=1"} {
		if !slices.Contains(u.requestsFrom(mark), req) {
			t.Errorf("cycle 2 sent no request %q: %q", req, u.requestsFrom(mark))
		}
	}
	if _, err := w.entClient.CarrierFacility.Get(ctx, 2); err != nil {
		t.Fatalf("get carrierfac 2: %v", err)
	}
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "carrier", wmT1)

	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	for _, objType := range []string{"org", "carrier"} {
		if got, want := u.listSince(mark, objType), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
			t.Errorf("cycle 3 %s since = %q, want %q", objType, got, want)
		}
	}
	assertOrg(t, w, 3, "deleted", wmT2)
	if c := w.entClient.Carrier.GetX(ctx, 3); c.Status != "deleted" {
		t.Errorf("carrier 3 status = %q, want deleted", c.Status)
	}
	assertWatermark(t, db, "org", wmT3)
	assertWatermark(t, db, "carrier", wmT3)
}

// TestSync_SideFKBackfillKeepsFacCursor covers the per-row backfill of a
// nullable FK: netixlan 1 gets net_side_id 2, a fac that upstream added
// after the fac fetch.
func TestSync_SideFKBackfillKeepsFacCursor(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
	u.put("fac",
		bumpUpdated(makeFac(1, 1, "Fac1", "ok"), wmT0),
		bumpUpdated(makeFac(3, 1, "Fac3", "ok"), wmT1))
	u.put("ix", fixtureRow(t, "ix", 1, "updated", wmT0))
	u.put("ixlan", fixtureRow(t, "ixlan", 1, "updated", wmT0))
	u.put("net", bumpUpdated(makeNet(1, 1, 64501, "Net1", "ok"), wmT0))
	u.put("netixlan", bumpUpdated(makeNetIxLan(1, 1, 1, "ok"), wmT0))
	w, db, _ := newWatermarkWorker(t, u, 5)
	ctx := t.Context()

	mustSync(t, w, config.SyncModeIncremental)
	assertWatermark(t, db, "fac", wmT1)

	fac3 := bumpUpdated(makeFac(3, 1, "Fac3", "deleted"), wmT2)
	fac2 := bumpUpdated(makeFac(2, 1, "Fac2", "ok"), wmT3)
	nix1 := with(bumpUpdated(makeNetIxLan(1, 1, 1, "ok"), wmT3), "net_side_id", 2)
	u.afterFirstList("fac", func(u *wmUpstream) {
		u.putLocked("fac", fac3, fac2)
		u.putLocked("netixlan", nix1)
	})
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if !slices.Contains(u.requestsFrom(mark), "fac?id__in=2&since=1") {
		t.Fatalf("cycle 2 sent no backfill request for fac 2: %q", u.requestsFrom(mark))
	}
	if nix := w.entClient.NetworkIxLan.GetX(ctx, 1); nix.NetSideID == nil || *nix.NetSideID != 2 {
		t.Errorf("netixlan 1 net_side_id = %v, want 2", nix.NetSideID)
	}
	assertWatermark(t, db, "fac", wmT1)

	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "fac"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 3 fac since = %q, want %q", got, want)
	}
	if f := w.entClient.Facility.GetX(ctx, 3); f.Status != "deleted" {
		t.Errorf("fac 3 status = %q, want deleted", f.Status)
	}
	assertWatermark(t, db, "fac", wmT3)
}

// TestSync_WatermarkEqualsMaxWithoutBackfill asserts that a cycle with no
// FK backfill sends the requests of a MAX(updated) cursor, and that the
// watermark of each type is then its MAX(updated). Only an empty table
// (poc here) has no watermark row. The cycles change rows of several
// types, add a tombstone, add a parent and its child in one cycle, and
// include a full cycle.
func TestSync_WatermarkEqualsMaxWithoutBackfill(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	for _, name := range canonicalStepOrder {
		if name != "poc" {
			u.put(name, fixtureRows(t, name)...)
		}
	}
	w, db, _ := newWatermarkWorker(t, u, 5)
	ctx := t.Context()

	maxUpdated := func() map[string]time.Time {
		t.Helper()
		out := make(map[string]time.Time, len(canonicalStepOrder))
		for _, name := range canonicalStepOrder {
			m, err := GetMaxUpdated(ctx, db, entityTables[name])
			if err != nil {
				t.Fatalf("max updated of %s: %v", name, err)
			}
			out[name] = m
		}
		return out
	}
	assertMarks := func(label string) {
		t.Helper()
		marks, err := watermarkRows(ctx, db)
		if err != nil {
			t.Fatalf("%s: read sync_watermark: %v", label, err)
		}
		for name, m := range maxUpdated() {
			got, ok := marks[name]
			switch {
			case m.IsZero() && ok:
				t.Errorf("%s: watermark of empty %s = %d, want no row", label, name, got)
			case m.IsZero():
			case !ok:
				t.Errorf("%s: watermark of %s: no row, want MAX(updated) %d", label, name, m.Unix())
			case got != m.Unix():
				t.Errorf("%s: watermark of %s = %d, want MAX(updated) %d", label, name, got, m.Unix())
			}
		}
	}
	incremental := func(label string) {
		t.Helper()
		before := maxUpdated()
		var want []string
		for _, name := range canonicalStepOrder {
			if m := before[name]; !m.IsZero() {
				want = append(want, fmt.Sprintf("%s?limit=250&skip=0&depth=0&since=%d", name, m.Unix()))
				continue
			}
			want = append(want, name+"?depth=0")
		}
		mark := u.mark()
		mustSync(t, w, config.SyncModeIncremental)
		if got := u.requestsFrom(mark); !slices.Equal(got, want) {
			t.Errorf("%s requests:\n got %q\nwant %q", label, got, want)
		}
		assertMarks(label)
	}

	mustSync(t, w, config.SyncModeIncremental)
	assertMarks("bootstrap")

	u.put("org", fixtureRow(t, "org", 1, "name", "Org One", "updated", "2025-01-01T00:00:00Z"))
	u.put("net", fixtureRow(t, "net", 2, "status", "deleted", "updated", "2025-01-02T00:00:00Z"))
	u.put("ix", fixtureRow(t, "ix", 2, "id", 3, "name", "Third IX", "updated", "2025-01-03T00:00:00Z"))
	u.put("fac", fixtureRow(t, "fac", 1, "notes", "changed", "updated", "2025-01-04T00:00:00Z"))
	u.put("netixlan", fixtureRow(t, "netixlan", 1, "speed", 10000, "updated", "2025-01-05T00:00:00Z"))
	incremental("cycle 2")

	u.put("org", fixtureRow(t, "org", 2, "id", 4, "name", "Org Four", "updated", "2025-02-01T00:00:00Z"))
	u.put("net", fixtureRow(t, "net", 2, "id", 4, "org_id", 4, "asn", 65004, "name", "Net Four", "updated", "2025-02-01T00:00:00Z"))
	u.put("campus", fixtureRow(t, "campus", 1, "notes", "changed", "updated", "2025-02-02T00:00:00Z"))
	incremental("cycle 3")
	incremental("cycle 4")

	// A full cycle sends a bare list, then a window from the older of the
	// cursor and the newest row of the snapshot.
	before := maxUpdated()
	var want []string
	for _, name := range canonicalStepOrder {
		want = append(want, name+"?depth=0")
		since := before[name]
		if newest := u.newestLive(name); !newest.IsZero() && newest.Before(since) {
			since = newest
		}
		if !since.IsZero() {
			want = append(want, fmt.Sprintf("%s?limit=250&skip=0&depth=0&since=%d", name, since.Unix()))
		}
	}
	mark := u.mark()
	mustSync(t, w, config.SyncModeFull)
	if got := u.requestsFrom(mark); !slices.Equal(got, want) {
		t.Errorf("full cycle requests:\n got %q\nwant %q", got, want)
	}
	assertMarks("full cycle")
	incremental("cycle 6")

	for _, req := range u.requestsFrom(0) {
		if strings.Contains(req, "id__in") {
			t.Errorf("request %q: want no FK backfill", req)
		}
	}
}

// TestSync_WatermarkFallsBackToMaxUpdated covers the first cycle after
// the upgrade: a table with rows and no watermark row uses MAX(updated)
// and logs the types once.
func TestSync_WatermarkFallsBackToMaxUpdated(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT1),
		bumpUpdated(makeOrg(2, "Org2", "ok"), wmT2))
	w, db, logs := newWatermarkWorker(t, u, 5)
	ctx := t.Context()
	// A row that a release before the watermark stored.
	w.entClient.Organization.Create().SetID(1).SetName("Org1").SetNameFold("org1").
		SetStatus("ok").SetCreated(wmTime(t, wmT0)).SetUpdated(wmTime(t, wmT1)).SaveX(ctx)

	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("org since = %q, want %q (MAX(updated))", got, want)
	}
	assertWatermark(t, db, "org", wmT2)
	missing := logs.records(t, "sync watermark missing, using MAX(updated)")
	if len(missing) != 1 || !slices.Equal(missing[0]["types"].([]any), []any{"org"}) {
		t.Errorf("missing logs = %v, want one with types [org]", missing)
	}

	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT2)}; !slices.Equal(got, want) {
		t.Errorf("cycle 2 org since = %q, want %q", got, want)
	}
	if n := len(logs.records(t, "sync watermark missing, using MAX(updated)")); n != 1 {
		t.Errorf("missing logs after cycle 2 = %d, want still 1", n)
	}
}

// TestSync_WatermarkMissingTable asserts that a database without the
// sync_watermark table syncs with MAX(updated) cursors and gets the table
// back from the sync transaction.
func TestSync_WatermarkMissingTable(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(2, "Org2", "ok"), wmT1))
	u.put("net", bumpUpdated(makeNet(1, 1, 64501, "Net1", "ok"), wmT1))
	w, db, logs := newWatermarkWorker(t, u, 5)
	ctx := t.Context()
	mustSync(t, w, config.SyncModeIncremental)

	if _, err := db.ExecContext(ctx, `DROP TABLE sync_watermark`); err != nil {
		t.Fatalf("drop sync_watermark: %v", err)
	}
	var want []string
	for _, name := range canonicalStepOrder {
		if name == "org" || name == "net" {
			want = append(want, name+"?limit=250&skip=0&depth=0&since="+unixOf(t, wmT1))
			continue
		}
		want = append(want, name+"?depth=0")
	}
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got := u.requestsFrom(mark); !slices.Equal(got, want) {
		t.Errorf("requests:\n got %q\nwant %q", got, want)
	}
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "net", wmT1)
	missing := logs.records(t, "sync watermark missing, using MAX(updated)")
	if len(missing) != 1 || !slices.Equal(missing[0]["types"].([]any), []any{"org", "net"}) {
		t.Errorf("missing logs = %v, want one with types [org net]", missing)
	}
}

// TestSync_WatermarkClamp asserts that the cursor is never later than
// MAX(updated), and that an empty table starts from a bare list whatever
// its watermark row holds. The cycle then replaces the watermark.
func TestSync_WatermarkClamp(t *testing.T) {
	t.Parallel()

	t.Run("above MAX(updated)", func(t *testing.T) {
		t.Parallel()
		u := newWatermarkUpstream(t)
		u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT1))
		w, db, logs := newWatermarkWorker(t, u, 5)
		mustSync(t, w, config.SyncModeIncremental)
		if _, err := db.ExecContext(t.Context(),
			`UPDATE sync_watermark SET max_updated = ? WHERE type = 'org'`, wmTime(t, wmT4).Unix()); err != nil {
			t.Fatalf("set watermark: %v", err)
		}

		mark := u.mark()
		mustSync(t, w, config.SyncModeIncremental)
		if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
			t.Errorf("org since = %q, want %q (MAX(updated))", got, want)
		}
		assertWatermark(t, db, "org", wmT1)
		if recs := logs.records(t, "sync cursor held behind newest row"); len(recs) != 0 {
			t.Errorf("held logs = %v, want none", recs)
		}
	})

	t.Run("empty table", func(t *testing.T) {
		t.Parallel()
		u := newWatermarkUpstream(t)
		u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
		u.put("fac", bumpUpdated(makeFac(1, 1, "Fac1", "ok"), wmT2))
		w, db, _ := newWatermarkWorker(t, u, 5)
		if _, err := db.ExecContext(t.Context(),
			`INSERT INTO sync_watermark (type, max_updated, updated_at) VALUES ('fac', ?, 'x')`,
			wmTime(t, wmT3).Unix()); err != nil {
			t.Fatalf("insert watermark: %v", err)
		}

		mustSync(t, w, config.SyncModeIncremental)
		if got, want := u.listSince(0, "fac"), []string{"", unixOf(t, wmT2)}; !slices.Equal(got, want) {
			t.Errorf("fac since = %q, want %q (bare list, then the snapshot window)", got, want)
		}
		assertWatermark(t, db, "fac", wmT2)
	})
}

// TestSync_WatermarkReadErrorSendsNoRequest asserts that a watermark that
// is not a positive integer, or a table that cannot be read, fails the
// cycle before the first upstream request. A fallback to MAX(updated)
// would bring back the defect that the watermark fixes. A bad row fails
// the fetch step of its type. A table that cannot be read fails the first
// step.
// Not parallel: it sets the global MeterProvider and TracerProvider.
func TestSync_WatermarkReadErrorSendsNoRequest(t *testing.T) {
	tests := []struct {
		name    string
		setup   []string
		step    string
		wantErr string
	}{
		{
			name:    "text",
			setup:   []string{`INSERT INTO sync_watermark VALUES ('net', 'abc', 'x')`},
			step:    "net",
			wantErr: "fetch net: sync watermark of net is abc",
		},
		{
			name:    "zero",
			setup:   []string{`INSERT INTO sync_watermark VALUES ('net', 0, 'x')`},
			step:    "net",
			wantErr: "fetch net: sync watermark of net is 0 (int64)",
		},
		{
			name:    "negative",
			setup:   []string{`INSERT INTO sync_watermark VALUES ('net', -5, 'x')`},
			step:    "net",
			wantErr: "fetch net: sync watermark of net is -5 (int64)",
		},
		{
			name:    "real",
			setup:   []string{`INSERT INTO sync_watermark VALUES ('net', 1.5, 'x')`},
			step:    "net",
			wantErr: "fetch net: sync watermark of net is 1.5 (float64)",
		},
		{
			name: "no max_updated column",
			setup: []string{
				`DROP TABLE sync_watermark`,
				`CREATE TABLE sync_watermark (type TEXT PRIMARY KEY, value INTEGER)`,
			},
			step:    "org",
			wantErr: "fetch org: read sync watermarks",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := setupMetricTest(t)
			rec := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
			otel.SetTracerProvider(tp)
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

			u := newWatermarkUpstream(t)
			u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
			w, db, _ := newWatermarkWorker(t, u, 5)
			ctx := t.Context()
			for _, stmt := range tc.setup {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					t.Fatalf("%s: %v", stmt, err)
				}
			}

			err := w.Sync(ctx, config.SyncModeIncremental)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("sync error = %v, want %q", err, tc.wantErr)
			}
			if n := u.mark(); n != 0 {
				t.Errorf("upstream requests = %d, want 0: %q", n, u.requestsFrom(0))
			}

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(ctx, &rm); err != nil {
				t.Fatalf("collect: %v", err)
			}
			fetchErrors := map[string]int64{}
			if m := findMetric(rm, "pdbplus.sync.type.fetch_errors"); m != nil {
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Fatalf("pdbplus.sync.type.fetch_errors is %T, want Sum[int64]", m.Data)
				}
				for _, dp := range sum.DataPoints {
					typ, _ := dp.Attributes.Value("type")
					fetchErrors[typ.AsString()] = dp.Value
				}
			}
			if want := map[string]int64{tc.step: 1}; !maps.Equal(fetchErrors, want) {
				t.Errorf("fetch errors by type = %v, want %v", fetchErrors, want)
			}

			var fetchSpans []string
			for _, s := range rec.Ended() {
				if !strings.HasPrefix(s.Name(), "sync-fetch-") {
					continue
				}
				fetchSpans = append(fetchSpans, s.Name())
				if s.Status().Code != codes.Error {
					t.Errorf("%s status = %v, want Error", s.Name(), s.Status().Code)
				}
			}
			if want := []string{"sync-fetch-" + tc.step}; !slices.Equal(fetchSpans, want) {
				t.Errorf("fetch spans = %v, want %v", fetchSpans, want)
			}
		})
	}
}

// TestSync_WatermarkRollsBackWithCycle asserts that the watermarks commit
// and roll back with the rows of the cycle, and that the watermark logs
// come only after the commit.
func TestSync_WatermarkRollsBackWithCycle(t *testing.T) {
	t.Parallel()

	t.Run("write error", func(t *testing.T) {
		t.Parallel()
		u := newWatermarkUpstream(t)
		u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
		u.put("fac", bumpUpdated(makeFac(1, 1, "Fac1", "ok"), wmT0))
		w, db, _ := newWatermarkWorker(t, u, 5)
		ctx := t.Context()
		mustSync(t, w, config.SyncModeIncremental)

		u.put("org", bumpUpdated(makeOrg(1, "Org1 New", "ok"), wmT1))
		u.put("fac", bumpUpdated(makeFac(1, 1, "Fac1 New", "ok"), wmT1))
		// The org watermark changes first, in the same transaction.
		if _, err := db.ExecContext(ctx, `CREATE TRIGGER sync_watermark_abort
			BEFORE UPDATE ON sync_watermark WHEN NEW.type = 'fac'
			BEGIN SELECT RAISE(ABORT, 'injected watermark failure'); END`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
		err := w.Sync(ctx, config.SyncModeIncremental)
		if err == nil || !strings.Contains(err.Error(), "write watermark of fac") {
			t.Fatalf("sync error = %v, want the fac watermark write error", err)
		}
		assertWatermark(t, db, "org", wmT0)
		assertWatermark(t, db, "fac", wmT0)
		assertOrg(t, w, 1, "ok", wmT0)

		if _, err := db.ExecContext(ctx, `DROP TRIGGER sync_watermark_abort`); err != nil {
			t.Fatalf("drop trigger: %v", err)
		}
		mustSync(t, w, config.SyncModeIncremental)
		assertWatermark(t, db, "org", wmT1)
		assertWatermark(t, db, "fac", wmT1)
		assertOrg(t, w, 1, "ok", wmT1)
	})

	t.Run("commit failure", func(t *testing.T) {
		t.Parallel()
		const behindMsg = "sync watermark behind backfilled rows"
		u := newWatermarkUpstream(t)
		u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
		w, db, _ := newWatermarkWorker(t, u, 5)
		probe := withCommitProbe(w, db)
		ctx := t.Context()
		mustSync(t, w, config.SyncModeIncremental)

		org4 := bumpUpdated(makeOrg(4, "Org4", "ok"), wmT2)
		net12 := bumpUpdated(makeNet(12, 4, 64512, "Net12", "ok"), wmT2)
		u.afterFirstList("org", func(u *wmUpstream) {
			u.putLocked("org", org4)
			u.putLocked("net", net12)
		})
		probe.fail.Store(true)
		if err := w.Sync(ctx, config.SyncModeIncremental); !errors.Is(err, errInjectedCommit) {
			t.Fatalf("sync error = %v, want %v", err, errInjectedCommit)
		}
		probe.fail.Store(false)
		assertWatermark(t, db, "org", wmT0)
		assertWatermark(t, db, "net", "")
		assertOrg(t, w, 4, "", "")
		if recs := probe.logs.records(t, behindMsg); len(recs) != 0 {
			t.Errorf("behind logs after the failed commit = %v, want none", recs)
		}

		// The org fetch of the next cycle returns org 4. A new backfill
		// lands org 5.
		org5 := bumpUpdated(makeOrg(5, "Org5", "ok"), wmT3)
		net13 := bumpUpdated(makeNet(13, 5, 64513, "Net13", "ok"), wmT3)
		u.afterFirstList("org", func(u *wmUpstream) {
			u.putLocked("org", org5)
			u.putLocked("net", net13)
		})
		mustSync(t, w, config.SyncModeIncremental)
		assertWatermark(t, db, "org", wmT2)
		assertWatermark(t, db, "net", wmT3)
		recs := probe.logs.records(t, behindMsg)
		if len(recs) != 1 || recs[0]["type"] != "org" || recs[0]["watermark"] != wmT2 || recs[0]["max_updated"] != wmT3 {
			t.Errorf("behind logs = %v, want one for org at %s, max_updated %s", recs, wmT2, wmT3)
		}
		if recs := probe.logsAtCommit().records(t, behindMsg); len(recs) != 0 {
			t.Errorf("behind logs before the commit = %v, want none", recs)
		}
	})
}

// TestSync_FKDroppedNewestRowRetried asserts that the watermark follows
// the stored rows, not the fetched rows. With FK backfill off, the
// orphan filter drops net 12, the newest net of the cycle. The next cycle
// fetches net from the older watermark and lands net 12.
func TestSync_FKDroppedNewestRowRetried(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org", bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0))
	u.put("net", bumpUpdated(makeNet(1, 1, 64501, "Net1", "ok"), wmT0))
	w, db, _ := newWatermarkWorker(t, u, 0)
	ctx := t.Context()
	mustSync(t, w, config.SyncModeIncremental)

	org2 := bumpUpdated(makeOrg(2, "Org2", "ok"), wmT3)
	net12 := bumpUpdated(makeNet(12, 2, 64512, "Net12", "ok"), wmT2)
	u.afterFirstList("org", func(u *wmUpstream) {
		u.putLocked("org", org2)
		u.putLocked("net", net12)
	})
	mustSync(t, w, config.SyncModeIncremental)
	if _, err := w.entClient.Network.Get(ctx, 12); err == nil {
		t.Fatal("net 12 stored, want it dropped (no org 2, FK backfill off)")
	}
	assertWatermark(t, db, "net", wmT0)

	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "net"), []string{unixOf(t, wmT0)}; !slices.Equal(got, want) {
		t.Errorf("cycle 3 net since = %q, want %q", got, want)
	}
	if _, err := w.entClient.Network.Get(ctx, 12); err != nil {
		t.Errorf("get net 12 after cycle 3: %v", err)
	}
	assertWatermark(t, db, "net", wmT2)
	for _, req := range u.requestsFrom(0) {
		if strings.Contains(req, "id__in") {
			t.Errorf("request %q: want no FK backfill", req)
		}
	}
}

// TestSync_FullModeWindowStartsAtWatermark asserts that the tombstone
// window of a full cycle starts at a held watermark, so the daily full
// cycle also lands the tombstone of the backfill gap.
func TestSync_FullModeWindowStartsAtWatermark(t *testing.T) {
	t.Parallel()
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(3, "Org3", "ok"), wmT1))
	w, db, _ := newWatermarkWorker(t, u, 5)
	mustSync(t, w, config.SyncModeIncremental)
	runBackfillCycle(t, u, w)
	assertWatermark(t, db, "org", wmT1)

	mark := u.mark()
	mustSync(t, w, config.SyncModeFull)
	if got, want := u.listSince(mark, "org"), []string{"", unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("full cycle org since = %q, want %q (bare list, then the window from the watermark)", got, want)
	}
	assertOrg(t, w, 3, "deleted", wmT2)
	assertWatermark(t, db, "org", wmT3)
}

// TestSync_WatermarkKeptWhenWindowDiscarded covers a tolerated window
// failure on the incremental fallback path. The held org cursor keeps its
// watermark, so the next cycle fetches the gap again. The fac cursor is
// not held and moves to MAX(updated), as before the watermark.
func TestSync_WatermarkKeptWhenWindowDiscarded(t *testing.T) {
	t.Parallel()
	const keptMsg = "sync watermark kept, tombstone window discarded"
	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(3, "Org3", "ok"), wmT1))
	u.put("fac", bumpUpdated(makeFac(1, 1, "Fac1", "ok"), wmT0))
	w, db, logs := newWatermarkWorker(t, u, 5)
	mustSync(t, w, config.SyncModeIncremental)
	u.put("fac", bumpUpdated(makeFac(2, 1, "Fac2", "ok"), wmT2))
	runBackfillCycle(t, u, w)
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "fac", wmT2)

	u.put("fac", bumpUpdated(makeFac(3, 1, "Fac3", "ok"), wmT4))
	u.setFailSince("org", http.StatusBadRequest)
	u.setFailSince("fac", http.StatusBadRequest)
	mark := u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	// The failed ?since= request, the bare list, and the failed window.
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1), "", unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 3 org since = %q, want %q", got, want)
	}
	assertOrg(t, w, 3, "ok", wmT1)
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "fac", wmT4)
	kept := logs.records(t, keptMsg)
	if len(kept) != 1 || kept[0]["type"] != "org" || kept[0]["watermark"] != wmT1 || kept[0]["level"] != "WARN" {
		t.Errorf("kept logs = %v, want one WARN for org at %s", kept, wmT1)
	}
	if n := len(logs.records(t, "sync watermark behind backfilled rows")); n != 1 {
		t.Errorf("behind logs = %d, want 1 (cycle 2 only)", n)
	}

	u.setFailSince("org", 0)
	u.setFailSince("fac", 0)
	mark = u.mark()
	mustSync(t, w, config.SyncModeIncremental)
	if got, want := u.listSince(mark, "org"), []string{unixOf(t, wmT1)}; !slices.Equal(got, want) {
		t.Errorf("cycle 4 org since = %q, want %q", got, want)
	}
	assertOrg(t, w, 3, "deleted", wmT2)
	assertWatermark(t, db, "org", wmT3)
}

// TestSync_WatermarkSpanAttributes asserts the cursor attributes of the
// fetch spans and the watermark counts on the root span.
// Not parallel: it sets the global TracerProvider.
func TestSync_WatermarkSpanAttributes(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	u := newWatermarkUpstream(t)
	u.put("org",
		bumpUpdated(makeOrg(1, "Org1", "ok"), wmT0),
		bumpUpdated(makeOrg(3, "Org3", "ok"), wmT1))
	w, db, _ := newWatermarkWorker(t, u, 5)
	ctx := t.Context()

	// cycleSpans runs fn and returns the attributes of the spans that
	// ended in it, by span name. A later span replaces an earlier one
	// with the same name.
	cycleSpans := func(fn func()) map[string]map[attribute.Key]attribute.Value {
		t.Helper()
		start := len(rec.Ended())
		fn()
		out := make(map[string]map[attribute.Key]attribute.Value)
		for _, s := range rec.Ended()[start:] {
			attrs := make(map[attribute.Key]attribute.Value)
			for _, a := range s.Attributes() {
				attrs[a.Key] = a.Value
			}
			out[s.Name()] = attrs
		}
		return out
	}
	check := func(label string, attrs map[attribute.Key]attribute.Value, want map[attribute.Key]string) {
		t.Helper()
		for _, key := range []attribute.Key{
			"pdbplus.sync.cursor", "pdbplus.sync.cursor.source", "pdbplus.sync.cursor.behind_seconds",
			"pdbplus.sync.watermarks_written", "pdbplus.sync.watermarks_held",
		} {
			got, ok := attrs[key]
			wantValue, wantOK := want[key]
			switch {
			case ok != wantOK:
				t.Errorf("%s: %s present = %v, want %v", label, key, ok, wantOK)
			case ok && got.String() != wantValue:
				t.Errorf("%s: %s = %s, want %s", label, key, got.String(), wantValue)
			}
		}
	}
	behind := strconv.FormatInt(wmTime(t, wmT3).Unix()-wmTime(t, wmT1).Unix(), 10)

	spans := cycleSpans(func() { mustSync(t, w, config.SyncModeIncremental) })
	check("cycle 1 org", spans["sync-fetch-org"], map[attribute.Key]string{
		"pdbplus.sync.cursor.source": "empty",
	})
	check("cycle 1 root", spans["sync-incremental"], map[attribute.Key]string{
		"pdbplus.sync.watermarks_written": "1",
		"pdbplus.sync.watermarks_held":    "0",
	})

	spans = cycleSpans(func() { runBackfillCycle(t, u, w) })
	check("cycle 2 org", spans["sync-fetch-org"], map[attribute.Key]string{
		"pdbplus.sync.cursor":        wmT1,
		"pdbplus.sync.cursor.source": "watermark",
	})
	check("cycle 2 root", spans["sync-incremental"], map[attribute.Key]string{
		"pdbplus.sync.watermarks_written": "1",
		"pdbplus.sync.watermarks_held":    "1",
	})

	spans = cycleSpans(func() { mustSync(t, w, config.SyncModeIncremental) })
	check("cycle 3 org", spans["sync-fetch-org"], map[attribute.Key]string{
		"pdbplus.sync.cursor":                wmT1,
		"pdbplus.sync.cursor.source":         "watermark",
		"pdbplus.sync.cursor.behind_seconds": behind,
	})
	check("cycle 3 net", spans["sync-fetch-net"], map[attribute.Key]string{
		"pdbplus.sync.cursor":        wmT3,
		"pdbplus.sync.cursor.source": "watermark",
	})
	check("cycle 3 root", spans["sync-incremental"], map[attribute.Key]string{
		"pdbplus.sync.watermarks_written": "1",
		"pdbplus.sync.watermarks_held":    "0",
	})

	if _, err := db.ExecContext(ctx, `DELETE FROM sync_watermark WHERE type = 'org'`); err != nil {
		t.Fatalf("delete org watermark: %v", err)
	}
	spans = cycleSpans(func() { mustSync(t, w, config.SyncModeIncremental) })
	check("cycle 4 org", spans["sync-fetch-org"], map[attribute.Key]string{
		"pdbplus.sync.cursor":        wmT3,
		"pdbplus.sync.cursor.source": "max_updated",
	})
	check("cycle 4 root", spans["sync-incremental"], map[attribute.Key]string{
		"pdbplus.sync.watermarks_written": "1",
		"pdbplus.sync.watermarks_held":    "0",
	})
}

func TestNextWatermark(t *testing.T) {
	t.Parallel()
	t1 := time.Unix(1000, 0).UTC()
	t2 := time.Unix(2000, 0).UTC()
	tests := []struct {
		name            string
		e, s            time.Time
		held, discarded bool
		want            time.Time
	}{
		{name: "newer fetched rows", e: t1, s: t2, want: t2},
		{name: "fetched rows at the cursor", e: t1, s: t1, want: t1},
		{name: "cursor floor", e: t2, s: t1, want: t2},
		{name: "only backfilled rows", e: t1, want: t1},
		{name: "empty table at start", s: t2, want: t2},
		{name: "empty table", want: time.Time{}},
		{name: "held", e: t1, s: t2, held: true, want: t2},
		{name: "held, window discarded", e: t1, s: t2, held: true, discarded: true, want: t1},
		{name: "not held, window discarded", e: t1, s: t2, discarded: true, want: t2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := nextWatermark(tc.e, tc.s, tc.held, tc.discarded); !got.Equal(tc.want) {
				t.Errorf("nextWatermark(%v, %v, %v, %v) = %v, want %v", tc.e, tc.s, tc.held, tc.discarded, got, tc.want)
			}
		})
	}
}

func TestNewSyncCursor(t *testing.T) {
	t.Parallel()
	m := time.Unix(2000, 0).UTC()
	tests := []struct {
		name          string
		maxUpdated    time.Time
		mark          int64
		hasMark       bool
		wantEffective time.Time
		wantHeld      bool
		wantSource    string
		wantBehind    int64
	}{
		{name: "empty table", wantSource: cursorSourceEmpty},
		{name: "empty table with a watermark", mark: 1500, hasMark: true, wantSource: cursorSourceEmpty},
		{name: "no watermark", maxUpdated: m, wantEffective: m, wantSource: cursorSourceMaxUpdated},
		{
			name: "watermark behind", maxUpdated: m, mark: 1500, hasMark: true,
			wantEffective: time.Unix(1500, 0).UTC(), wantHeld: true, wantSource: cursorSourceWatermark, wantBehind: 500,
		},
		{name: "watermark at MAX(updated)", maxUpdated: m, mark: 2000, hasMark: true, wantEffective: m, wantSource: cursorSourceWatermark},
		{name: "watermark ahead", maxUpdated: m, mark: 2500, hasMark: true, wantEffective: m, wantSource: cursorSourceWatermark},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newSyncCursor("org", tc.maxUpdated, tc.mark, tc.hasMark)
			if !c.effective.Equal(tc.wantEffective) || c.held != tc.wantHeld ||
				c.source() != tc.wantSource || c.behindSeconds() != tc.wantBehind {
				t.Errorf("cursor = (effective %v, held %v, source %s, behind %d), want (%v, %v, %s, %d)",
					c.effective, c.held, c.source(), c.behindSeconds(),
					tc.wantEffective, tc.wantHeld, tc.wantSource, tc.wantBehind)
			}
		})
	}
}

func TestReadSyncWatermarks(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	marks, err := readSyncWatermarks(ctx, db)
	if err != nil || len(marks) != 0 {
		t.Fatalf("no table: marks = %v, err = %v, want none", marks, err)
	}
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO sync_watermark VALUES
		('org', 1000, 'x'), ('net', 2000, 'x'), ('retired', 'abc', 'x')`); err != nil {
		t.Fatalf("insert watermarks: %v", err)
	}
	marks, err = readSyncWatermarks(ctx, db)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := map[string]int64{"org": 1000, "net": 2000}; !maps.Equal(marks, want) {
		t.Errorf("marks = %v, want %v (unknown types ignored)", marks, want)
	}
}

// TestWriteSyncWatermarks covers the rule of writeSyncWatermarks on one
// transaction for each call: the backfilled ids, the cursor floor, the
// kept watermark, the empty table, and a second call with the same input
// that changes no row.
func TestWriteSyncWatermarks(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	for id, updated := range map[int]string{1: wmT1, 2: wmT3} {
		client.Organization.Create().SetID(id).SetName("Org" + strconv.Itoa(id)).
			SetNameFold("org" + strconv.Itoa(id)).SetStatus("ok").
			SetCreated(wmTime(t, wmT0)).SetUpdated(wmTime(t, updated)).SaveX(ctx)
	}
	client.Facility.Create().SetID(1).SetOrgID(1).SetName("Fac1").SetNameFold("fac1").
		SetStatus("ok").SetCreated(wmTime(t, wmT0)).SetUpdated(wmTime(t, wmT2)).SaveX(ctx)
	client.Network.Create().SetID(1).SetOrgID(1).SetName("Net1").SetNameFold("net1").SetAsn(64501).
		SetStatus("ok").SetCreated(wmTime(t, wmT0)).SetUpdated(wmTime(t, wmT2)).SaveX(ctx)

	in := watermarkInput{
		cursors: []syncCursor{
			// Org 2 is backfilled: the watermark stays at org 1.
			newSyncCursor("org", wmTime(t, wmT1), 0, false),
			// An empty table writes no row.
			newSyncCursor("campus", time.Time{}, 0, false),
			// Held, and the window was discarded: the watermark stays.
			newSyncCursor("fac", wmTime(t, wmT2), wmTime(t, wmT0).Unix(), true),
			// The only row is backfilled: the cursor is the floor.
			newSyncCursor("net", wmTime(t, wmT2), 0, false),
		},
		discarded:  map[string]bool{"fac": true},
		backfilled: map[string][]int{"org": {2}, "net": {1}},
	}
	write := func(label string, in watermarkInput, now time.Time) (watermarkResult, int64) {
		t.Helper()
		tx, err := client.Tx(ctx)
		if err != nil {
			t.Fatalf("%s: begin: %v", label, err)
		}
		before := totalChanges(ctx, t, tx)
		res, err := writeSyncWatermarks(ctx, tx, in, now)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("%s: writeSyncWatermarks: %v", label, err)
		}
		changed := totalChanges(ctx, t, tx) - before
		if err := tx.Commit(); err != nil {
			t.Fatalf("%s: commit: %v", label, err)
		}
		return res, changed
	}

	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	res, changed := write("first", in, now)
	if res.written != 3 || changed != 3 {
		t.Errorf("first: written = %d, changed rows = %d, want 3 and 3", res.written, changed)
	}
	wantBehind := []watermarkBehind{
		{objectType: "org", watermark: wmTime(t, wmT1), newest: wmTime(t, wmT3)},
		{objectType: "fac", watermark: wmTime(t, wmT0), newest: wmTime(t, wmT2), kept: true},
	}
	if !slices.EqualFunc(res.behind, wantBehind, func(a, b watermarkBehind) bool {
		return a.objectType == b.objectType && a.watermark.Equal(b.watermark) && a.newest.Equal(b.newest) && a.kept == b.kept
	}) {
		t.Errorf("first: behind = %+v, want %+v", res.behind, wantBehind)
	}
	assertWatermark(t, db, "org", wmT1)
	assertWatermark(t, db, "campus", "")
	assertWatermark(t, db, "fac", wmT0)
	assertWatermark(t, db, "net", wmT2)

	res, changed = write("same input", in, now.Add(time.Hour))
	if res.written != 0 || changed != 0 {
		t.Errorf("same input: written = %d, changed rows = %d, want 0 and 0", res.written, changed)
	}
	var stamp string
	if err := db.QueryRowContext(ctx, `SELECT updated_at FROM sync_watermark WHERE type = 'org'`).Scan(&stamp); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	if want := "2026-09-25T12:00:00Z"; stamp != want {
		t.Errorf("org updated_at = %q, want %q (not rewritten)", stamp, want)
	}

	in.discarded = nil
	res, changed = write("window kept", in, now.Add(time.Hour))
	if res.written != 1 || changed != 1 {
		t.Errorf("window kept: written = %d, changed rows = %d, want 1 and 1", res.written, changed)
	}
	assertWatermark(t, db, "fac", wmT2)
}

// totalChanges returns SQLite total_changes() on the connection of tx.
func totalChanges(ctx context.Context, t *testing.T, tx *ent.Tx) int64 {
	t.Helper()
	rows, err := tx.QueryContext(ctx, `SELECT total_changes()`)
	if err != nil {
		t.Fatalf("total_changes: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var n int64
	if !rows.Next() {
		t.Fatalf("total_changes: no row: %v", rows.Err())
	}
	if err := rows.Scan(&n); err != nil {
		t.Fatalf("scan total_changes: %v", err)
	}
	return n
}

// TestUpsertSingleRaw_SingleCaller locks the rule of Worker.fkBackfilled:
// fkBackfillBatch is the only caller of upsertSingleRaw, and
// upsertSingleRaw is the only code that calls a singleUpsert closure. A
// row that another path lands after the fetch of its type would move the
// watermark past rows that the cycle never fetched.
func TestUpsertSingleRaw_SingleCaller(t *testing.T) {
	t.Parallel()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	fset := token.NewFileSet()
	uses := map[string][]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range f.Decls {
			owner, root := "package scope", ast.Node(decl)
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body == nil {
					continue
				}
				owner, root = fn.Name.Name, fn.Body
			}
			ast.Inspect(root, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.Ident:
					if n.Name == "upsertSingleRaw" {
						uses["upsertSingleRaw"] = append(uses["upsertSingleRaw"], owner)
					}
				case *ast.SelectorExpr:
					if n.Sel.Name == "singleUpsert" {
						uses["singleUpsert"] = append(uses["singleUpsert"], owner)
					}
				}
				return true
			})
		}
	}
	want := map[string][]string{
		"upsertSingleRaw": {"fkBackfillBatch"},
		"singleUpsert":    {"upsertSingleRaw"},
	}
	for name, owners := range want {
		if !slices.Equal(uses[name], owners) {
			t.Errorf("uses of %s in %v, want only in %v", name, uses[name], owners)
		}
	}
}
