package graphql

import (
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/dotwaffle/peeringdb-plus/graph"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestOverlapCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  int
	}{
		// syncStatus: 1*1; status: 3 fields of weight 1.
		{name: "repeated field", query: "{ syncStatus { status status status } }", want: 1 + 9},
		{name: "distinct aliases", query: "{ syncStatus { a: status b: status } }", want: 1 + 1 + 1},
		{name: "inline fragment", query: "{ syncStatus { status ... on SyncStatus { status } } }", want: 1 + 4},
		{
			name:  "fragment spreads and fragment fields",
			query: "{ syncStatus { status ...F ...F } } fragment F on SyncStatus { status }",
			want:  1 + 4 + 4,
		},
		{
			// organizationsList: weight 1 + 1 argument + 4 value nodes
			// (object, list, two ints), 1 argument.
			name:  "argument weight",
			query: "{ organizationsList(where: {idIn: [1, 2]}) { id } }",
			want:  6 + 1 + 1,
		},
		{
			name:  "repeated field with arguments",
			query: "{ organizationsList(limit: 1) { id } organizationsList(limit: 1) { id } }",
			want:  2*6 + 4 + 4,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			doc, err := parser.ParseQuery(&ast.Source{Input: tc.query})
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := overlapCost(doc); got != tc.want {
				t.Errorf("overlapCost = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestOverlapBudget(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	h := NewHandler(graph.NewResolver(client, nil))

	// n copies of one field cost n*n. __typename needs no resolver, so a
	// query within the budget runs without errors.
	repeat := func(n int) string {
		return "{ " + strings.Repeat("__typename ", n) + "}"
	}

	tests := []struct {
		name    string
		query   string
		wantErr string
	}{
		{name: "at budget", query: repeat(1000)},
		{name: "over budget", query: repeat(1001), wantErr: "overlap cost 1002001 exceeds 1000000"},
		{
			// The budgeted rule still runs the gqlparser rule below the
			// budget.
			name:    "conflict under budget",
			query:   "{ syncStatus { status: lastSyncAt status } }",
			wantErr: `"lastSyncAt" and "status" are different fields`,
		},
		{
			name:    "conflict in fragment",
			query:   "{ syncStatus { ...F } } fragment F on SyncStatus { status: lastSyncAt status }",
			wantErr: `"lastSyncAt" and "status" are different fields`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := postGQL(t, h, tc.query)
			if tc.wantErr == "" {
				if len(resp.Errors) > 0 {
					t.Fatalf("unexpected errors: %+v", resp.Errors)
				}
				return
			}
			found := false
			for _, gqlErr := range resp.Errors {
				if strings.Contains(gqlErr.Message, tc.wantErr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no error contains %q; errors: %+v", tc.wantErr, resp.Errors)
			}
		})
	}
}
