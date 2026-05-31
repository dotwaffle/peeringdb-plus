package pdbcompat

import (
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDefaultOrdering_IndexBacked verifies the composite
// (status, updated, created, id) index lets SQLite satisfy the default list
// ordering directly, without materialising a temp B-tree for the sort, on the
// common single-status path (a list request without ?since admits only
// status='ok'). All 13 entities carry the same generated index; networks is
// the representative case. Without the index the planner uses the
// single-column status index and falls back to a filesort.
func TestDefaultOrdering_IndexBacked(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	q := "SELECT * FROM networks WHERE status IN ('ok') " +
		"ORDER BY updated DESC, created DESC, id DESC LIMIT 250"
	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+q)
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
	joined := strings.Join(plan, " | ")
	if strings.Contains(joined, "TEMP B-TREE") {
		t.Errorf("default list ordering still requires a filesort (composite index not used): %s", joined)
	}
	if !strings.Contains(joined, "network_status_updated_created_id") {
		t.Errorf("query plan did not use the composite ordering index: %s", joined)
	}
}
