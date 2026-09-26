package database_test

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	_ "github.com/dotwaffle/peeringdb-plus/ent/runtime"
	"github.com/dotwaffle/peeringdb-plus/internal/database"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// TestSchemaCreate_IxLanRebuildKeepsChildFKs migrates a populated
// ix_lans table from the old ixf_ixp_member_list_url column (an empty
// string default) to the new one (no default). SQLite cannot ALTER a column default, so
// Schema.Create rebuilds the table: CREATE new_ix_lans, INSERT ... SELECT,
// DROP TABLE ix_lans, RENAME. ix_prefixes.ixlan_id and
// network_ix_lans.ixlan_id reference ix_lans with ON DELETE SET NULL, so
// the DROP would null both columns on every row if foreign keys were on.
// ent turns foreign_keys off on the pooled connection and then starts
// the migration tx, which gets the same connection only because
// database/sql reuses the most recently freed one. The failure is
// silent (foreign_key_check finds no violation), so this test is its
// only guard. It uses the production DSN and pool (database.Open), with
// the idle pool warm as in production.
//
// The test changes the package global migrate.IxLansColumns, so it does
// not call t.Parallel(). The other tests of this package are parallel,
// and they run after it.
func TestSchemaCreate_IxLanRebuildKeepsChildFKs(t *testing.T) {
	idx := -1
	for i, c := range migrate.IxLansColumns {
		if c.Name == "ixf_ixp_member_list_url" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("ix_lans has no ixf_ixp_member_list_url column")
	}
	urlCol := migrate.IxLansColumns[idx]
	if urlCol.Default != nil {
		t.Fatalf("ixf_ixp_member_list_url Default = %#v, want nil", urlCol.Default)
	}
	t.Cleanup(func() { urlCol.Default = nil })

	for _, traceSQL := range []bool{false, true} {
		t.Run(fmt.Sprintf("traceSQL=%v", traceSQL), func(t *testing.T) {
			ctx := t.Context()
			client, db, err := database.Open(filepath.Join(t.TempDir(), "x.db"), traceSQL)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { _ = client.Close() })
			create := func(t *testing.T) {
				t.Helper()
				if err := client.Schema.Create(ctx,
					migrate.WithDropColumn(true),
					migrate.WithDropIndex(true),
				); err != nil {
					t.Fatalf("schema create: %v", err)
				}
			}

			// The table as the previous binary created it.
			urlCol.Default = ""
			create(t)
			seed.Full(t, client)
			if got := columnDDL(t, db); !strings.Contains(got, "DEFAULT ('')") {
				t.Fatalf("old column DDL = %q, want DEFAULT ('')", got)
			}
			lans := tableRows(t, db, "ix_lans")
			pfxs := tableRows(t, db, "ix_prefixes")
			nixs := tableRows(t, db, "network_ix_lans")
			if len(lans) == 0 || len(pfxs) == 0 || len(nixs) == 0 {
				t.Fatalf("seed rows: ix_lans %d, ix_prefixes %d, network_ix_lans %d, want all > 0",
					len(lans), len(pfxs), len(nixs))
			}
			for _, r := range append(slicesOf(pfxs), slicesOf(nixs)...) {
				if strings.Contains(r, "ixlan_id=NULL") {
					t.Fatalf("seed row has no ixlan_id: %s", r)
				}
			}
			warmPool(t, db, 5)

			// The rebuild.
			urlCol.Default = nil
			create(t)
			if got := columnDDL(t, db); strings.Contains(got, "DEFAULT") {
				t.Errorf("new column DDL = %q, want no DEFAULT", got)
			}
			assertRows(t, db, "ix_lans", lans)
			assertRows(t, db, "ix_prefixes", pfxs)
			assertRows(t, db, "network_ix_lans", nixs)
			assertNoFKViolation(t, db)
			assertForeignKeysOn(t, db, 5)

			// A third run has nothing to change.
			var plan bytes.Buffer
			if err := client.Schema.WriteTo(ctx, &plan,
				migrate.WithDropColumn(true),
				migrate.WithDropIndex(true),
			); err != nil {
				t.Fatalf("schema plan: %v", err)
			}
			if s := plan.String(); strings.Contains(s, "CREATE") || strings.Contains(s, "ALTER") || strings.Contains(s, "DROP") {
				t.Errorf("third migration plan is not empty:\n%s", s)
			}
			create(t)
			assertRows(t, db, "ix_lans", lans)
			assertRows(t, db, "ix_prefixes", pfxs)
			assertRows(t, db, "network_ix_lans", nixs)
		})
	}
}

// columnDDL returns the definition of ixf_ixp_member_list_url in the
// stored CREATE TABLE statement of ix_lans.
func columnDDL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var ddl string
	if err := db.QueryRowContext(t.Context(),
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'ix_lans'`).Scan(&ddl); err != nil {
		t.Fatalf("read ix_lans DDL: %v", err)
	}
	const name = "`ixf_ixp_member_list_url` "
	i := strings.Index(ddl, name)
	if i < 0 {
		t.Fatalf("ix_lans DDL has no %s column: %s", name, ddl)
	}
	def, _, _ := strings.Cut(ddl[i:], ",")
	return def
}

// tableRows returns every row of table as "col=quote(value) ..." text,
// keyed by id.
func tableRows(t *testing.T, db *sql.DB, table string) map[int]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+table)
	if err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("columns of %s: %v", table, err)
	}
	out := map[int]string{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		var b strings.Builder
		id := -1
		for i, c := range cols {
			v := vals[i]
			if c == "id" {
				if n, ok := v.(int64); ok {
					id = int(n)
				}
			}
			if v == nil {
				fmt.Fprintf(&b, "%s=NULL ", c)
			} else {
				fmt.Fprintf(&b, "%s=%q ", c, fmt.Sprint(v))
			}
		}
		out[id] = b.String()
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read %s: %v", table, err)
	}
	return out
}

func slicesOf(m map[int]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// assertRows fails when table no longer holds exactly the rows of want.
func assertRows(t *testing.T, db *sql.DB, table string, want map[int]string) {
	t.Helper()
	got := tableRows(t, db, table)
	if len(got) != len(want) {
		t.Errorf("%s: %d rows, want %d", table, len(got), len(want))
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s row %d changed:\n got %s\nwant %s", table, id, got[id], w)
		}
	}
}

// warmPool takes n connections at the same time and frees them, so the
// pool holds n idle connections, as a production pool does.
func warmPool(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	conns := make([]*sql.Conn, 0, n)
	for range n {
		c, err := db.Conn(t.Context())
		if err != nil {
			t.Fatalf("take connection: %v", err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		_ = c.Close()
	}
}

// assertNoFKViolation fails when foreign_key_check reports a row.
func assertNoFKViolation(t *testing.T, db *sql.DB) {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&n); err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	if n != 0 {
		t.Errorf("foreign_key_check: %d violations, want 0", n)
	}
}

// assertForeignKeysOn takes n connections at the same time (the idle
// ones first, so the one that ran the migration is among them) and
// fails when one of them has foreign keys off.
func assertForeignKeysOn(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	conns := make([]*sql.Conn, 0, n)
	defer func() {
		for _, c := range conns {
			_ = c.Close()
		}
	}()
	for range n {
		c, err := db.Conn(t.Context())
		if err != nil {
			t.Fatalf("take connection: %v", err)
		}
		conns = append(conns, c)
		var on int
		if err := c.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&on); err != nil {
			t.Fatalf("PRAGMA foreign_keys: %v", err)
		}
		if on != 1 {
			t.Errorf("connection %d: foreign_keys = %d, want 1", len(conns), on)
		}
	}
}
