package pdbcompat

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"

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
// List and Count closures emit through EXPLAIN QUERY PLAN. A plain list
// on a type with one live status is ordered by id, and the
// single-column status index returns its rows in (status, rowid)
// order, so SQLite needs no sort. The app never runs ANALYZE, so the
// empty test database gets the same plan as production.
func TestPdbcompatListPlan_NoTempBTree(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		typ  string
		opts QueryOptions
	}{
		{"net_default", "net", QueryOptions{Limit: 250, Skip: 30000}},
		{"org_default", "org", QueryOptions{Limit: 250}},
		{"fac_default_unbounded", "fac", QueryOptions{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			list, _ := listPlans(t, tc.typ, tc.opts)
			if strings.Contains(list, "TEMP B-TREE") {
				t.Errorf("list plan sorts in a temp B-tree: %s", list)
			}
		})
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

// recordingDriver keeps the last query that ent sent, so a test can
// run it through EXPLAIN QUERY PLAN.
type recordingDriver struct {
	dialect.Driver
	mu   sync.Mutex
	q    string
	args []any
}

func (d *recordingDriver) Query(ctx context.Context, query string, args, v any) error {
	d.mu.Lock()
	d.q = query
	d.args, _ = args.([]any)
	d.mu.Unlock()
	return d.Driver.Query(ctx, query, args, v)
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
