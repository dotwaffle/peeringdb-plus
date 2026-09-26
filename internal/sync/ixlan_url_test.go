package sync_test

import (
	"database/sql"
	"log/slog"
	"maps"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// newIxLanURLWorker returns a worker over the fixture server fs and the
// raw database handle of its client.
func newIxLanURLWorker(t *testing.T, fs *fixtureServer) (*sync.Worker, *sql.DB) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	return sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default()), db
}

// ixLanURLColumn returns quote(ixf_ixp_member_list_url) per ixlan id, so
// NULL and the empty string stay apart.
func ixLanURLColumn(t *testing.T, db *sql.DB) map[int]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(),
		`SELECT id, quote(ixf_ixp_member_list_url) FROM ix_lans`)
	if err != nil {
		t.Fatalf("read ix_lans: %v", err)
	}
	defer func() { _ = rows.Close() }()
	got := map[int]string{}
	for rows.Next() {
		var id int
		var v string
		if err := rows.Scan(&id, &v); err != nil {
			t.Fatalf("scan ix_lans: %v", err)
		}
		got[id] = v
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read ix_lans: %v", err)
	}
	return got
}

// TestSync_IxLanURLKeepsNullApartFromEmpty checks that sync stores the
// IX-F member list URL as upstream sends it: null as NULL, "" as "", a
// URL as the URL, and an absent key (the sync caller may not see the
// row) as NULL. All four rows are in one page, so they land in one bulk
// INSERT. Upstream stores both NULL and "" (django-peeringdb
// abstract.py:819-824) and renders NULL as null.
func TestSync_IxLanURLKeepsNullApartFromEmpty(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	const row = `"ix_id":1,"name":"LAN","descr":"","mtu":1500,"dot1q_support":false,` +
		`"rs_asn":null,"arp_sponge":null,"ixf_ixp_import_enabled":false,` +
		`"created":"2024-01-25T09:30:00Z","updated":"2024-08-15T14:00:00Z","status":"ok"`
	fs.setFixtureData("ixlan", []byte(`[`+
		`{"id":1,`+row+`,"ixf_ixp_member_list_url_visible":"Public","ixf_ixp_member_list_url":null},`+
		`{"id":2,`+row+`,"ixf_ixp_member_list_url_visible":"Public","ixf_ixp_member_list_url":""},`+
		`{"id":3,`+row+`,"ixf_ixp_member_list_url_visible":"Public","ixf_ixp_member_list_url":"https://ixf.example.test/m.json"},`+
		`{"id":4,`+row+`,"ixf_ixp_member_list_url_visible":"Users"}`+
		`]`))
	w, db := newIxLanURLWorker(t, fs)
	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := map[int]string{1: "NULL", 2: "''", 3: "'https://ixf.example.test/m.json'", 4: "NULL"}
	if got := ixLanURLColumn(t, db); !maps.Equal(got, want) {
		t.Errorf("ixf_ixp_member_list_url = %v, want %v", got, want)
	}
}

// TestSync_FullModeRewritesLegacyEmptyURL checks the backfill of rows
// that an older binary stored as "" where upstream sends null or no key.
// An incremental cycle keeps them (upstream updated did not change), and
// the next full cycle stores NULL: writeRowDiffers compares with IS NOT,
// and NULL IS NOT the empty string.
func TestSync_FullModeRewritesLegacyEmptyURL(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	w, db := newIxLanURLWorker(t, fs)
	ctx := t.Context()
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// The fixture ixlans 1 and 2 have no URL key.
	nulls := map[int]string{1: "NULL", 2: "NULL"}
	if got := ixLanURLColumn(t, db); !maps.Equal(got, nulls) {
		t.Fatalf("after first sync: %v, want %v", got, nulls)
	}
	if _, err := db.ExecContext(ctx, `UPDATE ix_lans SET ixf_ixp_member_list_url = ''`); err != nil {
		t.Fatalf("store legacy values: %v", err)
	}
	legacy := map[int]string{1: "''", 2: "''"}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("incremental sync: %v", err)
	}
	if got := ixLanURLColumn(t, db); !maps.Equal(got, legacy) {
		t.Errorf("after incremental sync: %v, want %v (no rewrite without a newer updated)", got, legacy)
	}

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("full sync: %v", err)
	}
	if got := ixLanURLColumn(t, db); !maps.Equal(got, nulls) {
		t.Errorf("after full sync: %v, want %v", got, nulls)
	}
}
