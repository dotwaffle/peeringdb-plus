package pdbcompat

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDefaultOrdering_IndexBacked verifies the composite
// (status, updated, created, id) index lets SQLite read the
// (-updated, -created, -id) order without a temp B-tree when the query
// filters on one status. entrest and the ConnectRPC List/Stream RPCs
// use this order, but they add no default status filter: only a list
// with a client-supplied status filter gets this plan, and their
// default lists sort in a temp B-tree. In pdbcompat, the ?since=
// COUNT(*) reads the index as a covering index; pdbcompat lists use id
// order instead (see TestPdbcompatListPlan_NoTempBTree). All 13
// entities carry the same generated index; networks is the
// representative case.
func TestDefaultOrdering_IndexBacked(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	q := "SELECT * FROM networks WHERE status = 'ok' " +
		"ORDER BY updated DESC, created DESC, id DESC LIMIT 250"
	joined := explainPlan(t, db, q, nil)
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("default list ordering still requires a filesort (composite index not used): %s", joined)
	}
	if !strings.Contains(joined, "network_status_updated_created_id") {
		t.Errorf("query plan did not use the composite ordering index: %s", joined)
	}
}

// TestPdbcompatListPlan_NoTempBTree runs the SQL that the Registry
// List and Count closures emit through EXPLAIN QUERY PLAN. The app never
// runs ANALYZE, so the empty test database gets the same plan as
// production.
//
//   - A plain list on a type with one live status is ordered by id. The
//     single-column status index returns its rows in (status, rowid)
//     order, so SQLite needs no sort.
//   - A plain netixlan list (two live statuses) and every ?since= list
//     wrap the status set in likely(). SQLite then reads the rowid
//     table (id order) or the updated index (?since= order) and needs
//     no sort. The COUNT query still reads a covering status index.
func TestPdbcompatListPlan_NoTempBTree(t *testing.T) {
	t.Parallel()
	since := time.Unix(1, 0)
	for _, tc := range []struct {
		name      string
		typ       string
		opts      QueryOptions
		wantList  string // substring of the list plan
		wantCount string // substring of the count plan
	}{
		{"net_default", "net", QueryOptions{Limit: 250, Skip: 30000},
			"USING INDEX network_status (status=?)", "COVERING INDEX network_status"},
		{"org_default", "org", QueryOptions{Limit: 250},
			"USING INDEX organization_status (status=?)", "COVERING INDEX organization_status"},
		{"fac_default_unbounded", "fac", QueryOptions{},
			"USING INDEX facility_status (status=?)", "COVERING INDEX facility_status"},
		{"netixlan_default", "netixlan", QueryOptions{Limit: 250, Skip: 30000},
			"SCAN network_ix_lans", "COVERING INDEX networkixlan_status"},
		{"netixlan_default_unbounded", "netixlan", QueryOptions{},
			"SCAN network_ix_lans", "COVERING INDEX networkixlan_status"},
		{"net_since", "net", QueryOptions{Since: &since, Limit: 250},
			"USING INDEX network_updated (updated>?)", "COVERING INDEX"},
		{"netixlan_since", "netixlan", QueryOptions{Since: &since, Limit: 250},
			"USING INDEX networkixlan_updated (updated>?)", "COVERING INDEX"},
		{"campus_since_unbounded", "campus", QueryOptions{Since: &since},
			"USING INDEX campus_updated (updated>?)", "COVERING INDEX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			list, count := listPlans(t, tc.typ, tc.opts)
			if strings.Contains(list, "TEMP B-TREE") {
				t.Errorf("list plan sorts in a temp B-tree: %s", list)
			}
			if !strings.Contains(list, tc.wantList) {
				t.Errorf("list plan = %q, want it to contain %q", list, tc.wantList)
			}
			if !strings.Contains(count, tc.wantCount) {
				t.Errorf("count plan = %q, want it to contain %q", count, tc.wantCount)
			}
		})
	}
}

// TestPdbcompatIPAddr6Plan_UsesIndex checks that the bare netixlan
// ipaddr6 filter reads the networkixlan_ipaddr6 index in the List and
// the Count query. The filter compares the canonical text with a plain
// =; a LOWER(ipaddr6) comparison read every row.
func TestPdbcompatIPAddr6Plan_UsesIndex(t *testing.T) {
	t.Parallel()
	preds, empty, err := ParseFilters(url.Values{"ipaddr6": {"2001:7F8:0:0::1"}}, Registry["netixlan"])
	if err != nil || empty || len(preds) != 1 {
		t.Fatalf("ParseFilters: preds=%d empty=%v err=%v, want one predicate", len(preds), empty, err)
	}
	const want = "USING INDEX networkixlan_ipaddr6 (ipaddr6=?)"
	list, count := listPlans(t, "netixlan", QueryOptions{Filters: preds, Limit: 250})
	if !strings.Contains(list, want) {
		t.Errorf("list plan = %q, want it to contain %q", list, want)
	}
	if !strings.Contains(count, want) {
		t.Errorf("count plan = %q, want it to contain %q", count, want)
	}
}

// listPlans runs the Registry List and Count closures for typ against
// an empty database and returns the EXPLAIN QUERY PLAN output of the
// SQL each one sent.
func listPlans(t *testing.T, typ string, opts QueryOptions) (list, count string) {
	t.Helper()
	_, db := testutil.SetupClientWithDB(t)
	rec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
	client := ent.NewClient(ent.Driver(rec))
	tc := Registry[typ]

	if _, err := tc.List(t.Context(), client, opts); err != nil {
		t.Fatalf("list %s: %v", typ, err)
	}
	q, args := rec.lastQuery(t)
	list = explainPlan(t, db, q, args)

	if _, err := tc.Count(t.Context(), client, opts); err != nil {
		t.Fatalf("count %s: %v", typ, err)
	}
	q, args = rec.lastQuery(t)
	count = explainPlan(t, db, q, args)
	return list, count
}

// recordingDriver keeps the queries that ent sent, so a test can run
// them through EXPLAIN QUERY PLAN.
type recordingDriver struct {
	dialect.Driver
	mu   sync.Mutex
	q    string
	args []any
	all  []recordedQuery
}

// recordedQuery is one query that recordingDriver saw.
type recordedQuery struct {
	q    string
	args []any
}

func (d *recordingDriver) Query(ctx context.Context, query string, args, v any) error {
	d.mu.Lock()
	d.q = query
	d.args, _ = args.([]any)
	d.all = append(d.all, recordedQuery{q: d.q, args: d.args})
	d.mu.Unlock()
	return d.Driver.Query(ctx, query, args, v)
}

// queries returns every query recorded so far.
func (d *recordingDriver) queries() []recordedQuery {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.all)
}

func (d *recordingDriver) lastQuery(t *testing.T) (string, []any) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.q == "" {
		t.Fatal("no query recorded")
	}
	return d.q, d.args
}

// explainPlan returns the EXPLAIN QUERY PLAN detail rows of q joined
// with " | ".
func explainPlan(t *testing.T, db *sql.DB, q string, args []any) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer rows.Close()
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
