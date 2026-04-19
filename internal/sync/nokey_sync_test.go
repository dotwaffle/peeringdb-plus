package sync

// nokey_sync_test.go — Plan 60-05 (SYNC-02): end-to-end integration test
// proving that running the sync worker with no PeeringDB API key, against
// a fake upstream serving the phase 57 anonymous fixtures, produces a DB
// with zero visible="Users" POC rows, and that a subsequent anonymous read
// through the pdbcompat handler observes the same row set the worker stored
// (the privacy filter is effectively a no-op because there's nothing to
// filter).
//
// Why this test exists (from 60-CONTEXT.md D-12, D-13):
//   - The companion VIS-06/VIS-07 tests assert that the privacy policy
//     filters Users-tier POCs correctly WHEN they are present in the DB.
//   - This test asserts the inverse: the no-key deployment topology (a
//     first-class operational mode per SYNC-02) never lands Users-tier
//     rows in the DB in the first place, so the filter has nothing to
//     catch if something upstream of the policy regresses.
//
// Assertion layout:
//   Phase A (upstream boundary):
//     * Fake upstream serves phase 57 anon fixtures for all 13 types.
//     * Worker.Sync runs with an anonymous peeringdb.Client.
//     * DB invariant: Poc.Query().Where(poc.Visible("Users")).Count() == 0.
//     * Sanity: Poc.Query().Count() > 0 (the fake upstream/sync wiring is
//       actually delivering data — a silent break would yield zero total
//       rows and mask the Users assertion).
//     * Proof of anonymity: the fake upstream counts Authorization headers
//       observed across every request, and the assertion requires exactly
//       zero. Without this probe a future accidental key-passthrough could
//       be masked by fixtures that happen not to contain Users rows.
//   Phase B (surface boundary):
//     * Mount pdbcompat.NewHandler on a fresh httptest.Server, stamping
//       privctx.TierPublic on every incoming request.
//     * GET /api/poc?limit=50.
//     * Returned row count must equal the DB row count (Limit 50, bypass
//       ctx) — the filter is a no-op because the DB holds only Public rows.
//     * No row in the response may have visible="Users" — belt-and-braces.

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// anonFixtureTypes enumerates the 13 PeeringDB object types served by the
// fake upstream. Mirrors the canonical sync order in worker.go; the FK-
// orphan filter consumes whichever rows land in the scratch DB for each
// type, so serving all 13 means child rows can actually resolve their
// parents rather than being silently dropped as orphans.
var anonFixtureTypes = []string{
	"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
	"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
}

// TestNoKeySync is the SYNC-02 end-to-end invariant: a no-API-key sync
// against an anon-only upstream must land zero visible="Users" rows in the
// DB, and the surface read on the resulting DB must be a no-op for the
// privacy filter (response matches DB contents 1:1, within the requested
// limit).
//
// Hermetic, deterministic, no live PeeringDB traffic, no CI secrets.
// t.Parallel() is safe because both the in-memory SQLite DB and both
// httptest.Servers are per-test-instance.
func TestNoKeySync(t *testing.T) {
	t.Parallel()

	// --- Phase A wiring -------------------------------------------------

	// Load the 13 anon fixtures from phase 57 at test start. A missing
	// fixture is fatal — this catches sparse-checkout misconfiguration,
	// partial worktrees, and accidental removal. We never silently
	// fall back to empty-data for types the plan promises to cover.
	//
	// The path is held as a single literal ("../../testdata/visibility-baseline/beta/anon/api")
	// on purpose so a future tree rename surfaces as a trivial grep hit
	// rather than an opaque filepath.Join string split across pieces.
	const fixtureDir = "../../testdata/visibility-baseline/beta/anon/api"
	fixtures := make(map[string][]byte, len(anonFixtureTypes))
	for _, typeName := range anonFixtureTypes {
		path := filepath.Join(fixtureDir, typeName, "page-1.json")
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read anon fixture %s: %v (phase 57 fixtures missing — check sparse checkout / worktree state)", path, err)
		}
		fixtures[typeName] = b
	}

	// authHeaderCount records how many requests arrived at the fake
	// upstream with any Authorization header set. The production code
	// path for an anonymous client is never to add one; a non-zero
	// value here means the client was NOT anonymous and the whole
	// premise of this test (SYNC-02) has regressed. Atomic because
	// the worker fetches serially in current code, but future parallel
	// fetches must not silently corrupt the count.
	var authHeaderCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authHeaderCount.Add(1)
		}
		// Path shape is /api/{type}; strip the /api/ prefix then drop
		// any query string (there should not be one in full-sync mode,
		// but be defensive in case the client switches to paginated
		// URLs in the future).
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/"), "?", 2)
		objType := parts[0]

		// Full-sync mode issues `/api/{type}?depth=0` with no skip
		// param — the skip branch below only matters if the client
		// switches to paginated mode (SyncModeIncremental). Return an
		// empty-data envelope to terminate pagination on any skip != 0.
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		body, ok := fixtures[objType]
		if !ok {
			// Unknown type — return an empty-data envelope so the
			// worker records zero rows for that type and moves on.
			// This also covers `/api/` (the index), which the sync
			// worker never requests but which belt-and-braces guards
			// against probe requests leaking.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	// Ent client + raw DB (in-memory SQLite, per-test isolated).
	client, db := testutil.SetupClientWithDB(t)

	// Anonymous peeringdb client: NewClient without WithAPIKey means
	// no Authorization header is ever added (see
	// internal/peeringdb/client.go:131 — the `if c.apiKey != ""` guard).
	// Rate limit defanged so the test runs in milliseconds rather than
	// the production 20-req/min pace.
	pdbClient := peeringdb.NewClient(srv.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(1 * time.Millisecond)

	// sync_status table is created lazily by RecordSyncStart in production;
	// tests construct the table up front so Sync's first call does not
	// stumble on a missing schema.
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// --- Phase A assertions --------------------------------------------

	// Bypass context for assertion-side queries: the privacy policy
	// would otherwise silently filter out Users rows on read, which
	// would defeat the whole point of asserting "zero Users rows in
	// the DB". The bypass lets us observe the true row state.
	//
	// This test file is explicitly exempted from the
	// TestSyncBypass_SingleCallSite audit (see bypass_audit_test.go
	// line 116 — `_test.go` files are skipped). The exemption exists
	// because tests legitimately need to inspect DB contents free of
	// policy filtering to verify the policy's behaviour.
	bypass := privacy.DecisionContext(t.Context(), privacy.Allow)

	usersCount, err := client.Poc.Query().Where(poc.Visible("Users")).Count(bypass)
	if err != nil {
		t.Fatalf("count users POCs: %v", err)
	}
	if usersCount != 0 {
		t.Fatalf("SYNC-02 violation: %d Users-tier POCs in DB after no-key sync, want 0", usersCount)
	}

	totalCount, err := client.Poc.Query().Count(bypass)
	if err != nil {
		t.Fatalf("count all POCs: %v", err)
	}
	if totalCount == 0 {
		// Zero rows would mask the Users assertion — 0 == 0 trivially.
		// A real break here means the fake upstream, the FK-orphan
		// filter, or the upsert path has regressed. The exact survivor
		// count is intentionally NOT asserted: the anon fixtures may
		// contain POC rows whose parent Networks are missing from the
		// net fixture (or filtered out as orphans themselves), and the
		// expected count drifts with every fixture refresh.
		t.Fatalf("no POCs synced at all — fake upstream or sync wiring broken")
	}
	t.Logf("no-key sync persisted %d POC rows, 0 Users-tier", totalCount)

	if got := authHeaderCount.Load(); got != 0 {
		t.Fatalf("worker sent %d Authorization headers — must be 0 for anonymous sync (SYNC-02 proof-of-anonymity)", got)
	}

	// --- Phase B: surface read through pdbcompat -----------------------

	// Mount pdbcompat's /api/{type} handler on a fresh mux and stamp
	// TierPublic on every request. Production stamps the tier via
	// middleware.PrivacyTier at the edge of the chain; the inline
	// stamp here is sufficient to exercise the surface's interaction
	// with the ent privacy policy for this test.
	//
	// We deliberately do NOT wire the full buildMiddlewareChain here —
	// plans 60-02..04 own that surface-level coverage. This test's
	// contract is narrower: "the row set the surface returns is
	// exactly the row set the worker persisted, given no Users rows
	// exist to filter".
	mux := http.NewServeMux()
	pdbcompat.NewHandler(client).Register(mux)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := privctx.WithTier(r.Context(), privctx.TierPublic)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	surfaceSrv := httptest.NewServer(handler)
	t.Cleanup(surfaceSrv.Close)

	resp, err := http.Get(surfaceSrv.URL + "/api/poc?limit=50")
	if err != nil {
		t.Fatalf("GET /api/poc: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("GET /api/poc: status %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read /api/poc body: %v", err)
	}

	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode /api/poc response: %v", err)
	}

	// The TierPublic response must be identical (in row count) to the
	// equivalent bypass query — the filter is a no-op when the DB
	// holds only Public rows.
	dbPocs, err := client.Poc.Query().Limit(50).All(bypass)
	if err != nil {
		t.Fatalf("query DB POCs (bypass): %v", err)
	}
	if len(env.Data) != len(dbPocs) {
		t.Fatalf("surface returned %d rows, DB has %d (bypass, limit=50) — privacy filter is NOT a no-op on Public-only data",
			len(env.Data), len(dbPocs))
	}

	// Belt-and-braces: no response row may have visible="Users".
	// Phase A already asserted the DB has zero Users rows, but a
	// regression that injects Users rows at the surface (e.g. a
	// resolver bug that adds synthetic Users rows to the response)
	// would slip past the Phase A check. This guard catches it.
	for i, row := range env.Data {
		if vis, _ := row["visible"].(string); vis == "Users" {
			t.Fatalf("visible=Users row #%d leaked through anonymous surface: %+v", i, row)
		}
	}
}
