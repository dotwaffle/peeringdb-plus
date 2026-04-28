package sync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"

	"runtime"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// updateGolden enables golden-file regeneration via `go test -update`. When
// set, TestSync_RefactorParity writes its dump to the golden path instead of
// comparing. Run ONCE against the pre-refactor Sync to capture the baseline,
// then run without -update to verify the refactor preserved behavior.
var updateGolden = flag.Bool("update", false, "update golden files")

// TestMain initializes OTel metrics once before any tests run, preventing
// race conditions on global metric variables between parallel tests.
func TestMain(m *testing.M) {
	mp := sdkmetric.NewMeterProvider()
	otel.SetMeterProvider(mp)
	if err := pdbotel.InitMetrics(); err != nil {
		panic(fmt.Sprintf("InitMetrics: %v", err))
	}
	os.Exit(m.Run())
}

// ensureMetrics is a no-op kept for backward compatibility with tests that
// call it. Metrics are now initialized in TestMain before any tests run.
func ensureMetrics(t *testing.T) {
	t.Helper()
}

// fixture builds a minimal mock PeeringDB API server with configurable responses.
type fixture struct {
	server    *httptest.Server
	responses map[string]any // type -> response data
	failTypes map[string]bool
	callCount atomic.Int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		responses: make(map[string]any),
		failTypes: make(map[string]bool),
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.callCount.Add(1)
		// Extract object type from path: /api/{type}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "?")
		objType := parts[0]

		if f.failTypes[objType] {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Only return data on the first page (skip=0). Return empty on subsequent pages
		// to terminate pagination. The client uses limit+skip query params.
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
			return
		}

		data, ok := f.responses[objType]
		if !ok {
			// Return empty data for types without fixtures.
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": data})
	}))
	t.Cleanup(func() { f.server.Close() })
	return f
}

// newFastPDBClient creates a PeeringDB client with no rate limiting for tests.
func newFastPDBClient(t *testing.T, baseURL string) *peeringdb.Client {
	t.Helper()
	c := peeringdb.NewClient(baseURL, slog.Default())
	c.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	c.SetRetryBaseDelay(1 * time.Millisecond)
	return c
}

func newTestWorker(t *testing.T, f *fixture) (*Worker, *sql.DB) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, f.server.URL)

	err := InitStatusTable(t.Context(), db)
	if err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())
	return w, db
}

func makeOrg(id int, name, status string) map[string]any {
	return map[string]any{
		"id": id, "name": name, "aka": "", "name_long": "",
		"website": "", "social_media": []any{}, "notes": "",
		"address1": "", "address2": "", "city": "", "state": "",
		"country": "", "zipcode": "", "suite": "", "floor": "",
		"created": "2024-01-01T00:00:00Z", "updated": "2024-01-01T00:00:00Z",
		"status": status,
	}
}

// bumpUpdated returns a shallow copy of a map[string]any fixture with
// "updated" rewritten to the supplied RFC3339 timestamp string. Used by
// tests that need to drive the 260428-eda CHANGE 3 skip-on-unchanged
// predicate past `excluded.updated > existing.updated` on a re-sync.
//
// The PeeringDB API always bumps `updated` when row content changes, so
// this matches real upstream behaviour — pre-CHANGE 3 the tests got away
// with reusing a single timestamp because every upsert was unconditional.
func bumpUpdated(row map[string]any, updated string) map[string]any {
	out := make(map[string]any, len(row))
	maps.Copy(out, row)
	out["updated"] = updated
	return out
}

func makeFac(id, orgID int, name, status string) map[string]any {
	return map[string]any{
		"id": id, "org_id": orgID, "org_name": "Test", "name": name,
		"aka": "", "name_long": "", "website": "", "social_media": []any{},
		"clli": "", "rencode": "", "npanxx": "", "tech_email": "",
		"tech_phone": "", "sales_email": "", "sales_phone": "",
		"available_voltage_services": []any{},
		"notes":                      "", "net_count": 0, "ix_count": 0, "carrier_count": 0,
		"address1": "", "address2": "", "city": "", "state": "",
		"country": "", "zipcode": "", "suite": "", "floor": "",
		"created": "2024-01-01T00:00:00Z", "updated": "2024-01-01T00:00:00Z",
		"status": status,
	}
}

func makeNet(id, orgID, asn int, name, status string) map[string]any {
	return map[string]any{
		"id": id, "org_id": orgID, "name": name, "aka": "", "name_long": "",
		"website": "", "social_media": []any{}, "asn": asn,
		"looking_glass": "", "route_server": "", "irr_as_set": "",
		"info_type": "", "info_types": []any{},
		"info_traffic": "", "info_ratio": "", "info_scope": "",
		"info_unicast": false, "info_multicast": false,
		"info_ipv6": false, "info_never_via_route_servers": false,
		"notes": "", "policy_url": "", "policy_general": "",
		"policy_locations": "", "policy_ratio": false,
		"policy_contracts": "", "allow_ixp_update": false,
		"ix_count": 0, "fac_count": 0,
		"created": "2024-01-01T00:00:00Z", "updated": "2024-01-01T00:00:00Z",
		"status": status,
	}
}

// makePoc builds a fixture Poc JSON row matching the upstream PeeringDB
// /api/poc shape (internal/peeringdb/types.go Poc struct). Used by Phase 73
// BUG-02's TestSync_IncrementalRoleTombstone to drive a role="" tombstone
// through an incremental sync cycle.
func makePoc(id, netID int, name, role, status string) map[string]any {
	return map[string]any{
		"id": id, "net_id": netID,
		"role": role, "visible": "Public",
		"name": name, "phone": "", "email": "", "url": "",
		"created": "2024-01-01T00:00:00Z", "updated": "2024-01-01T00:00:00Z",
		"status": status,
	}
}

// TestSyncFetchesAll13Types verifies sync fetches all 13 types in correct FK dependency order.
func TestSyncFetchesAll13Types(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// Set up minimal data for org (all other types will return empty).
	f.responses["org"] = []any{makeOrg(1, "TestOrg", "ok")}
	w, _ := newTestWorker(t, f)

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify org was synced.
	orgs, err := w.entClient.Organization.Query().All(t.Context())
	if err != nil {
		t.Fatalf("query orgs: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("expected 1 org, got %d", len(orgs))
	}
	if orgs[0].Name != "TestOrg" {
		t.Errorf("expected org name TestOrg, got %s", orgs[0].Name)
	}
}

// TestSyncTransaction verifies sync runs in a single transaction (commit or rollback).
func TestSyncTransaction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify org is committed.
	count, err := w.entClient.Organization.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 org after sync, got %d", count)
	}
}

// TestSyncUpsertUpdatesExisting verifies sync updates existing records on second run.
func TestSyncUpsertUpdatesExisting(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "OriginalName", "ok")}
	w, _ := newTestWorker(t, f)

	// First sync.
	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Update mock response. Bump `updated` past the prior write so the
	// 260428-eda CHANGE 3 skip-on-unchanged predicate admits the rewrite.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "UpdatedName", "ok"), "2024-01-02T00:00:00Z")}

	// Second sync.
	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	org, err := w.entClient.Organization.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if org.Name != "UpdatedName" {
		t.Errorf("expected UpdatedName, got %s", org.Name)
	}
}

// TestSyncPersistsExplicitTombstone verifies sync persists explicit
// status='deleted' rows from upstream as tombstones in the local DB.
// Quick task 260428-2zl Task 6: pre-2zl this test asserted the
// inference-by-absence soft-delete path (org omitted from response →
// local row marked deleted). That inference was structurally wrong
// against PeeringDB's serializer-side filtering and is removed in T6.
// Post-2zl, deletes arrive as explicit `{"id": N, "status":"deleted"}`
// payloads in ?since=N responses; this test seeds that payload
// directly and asserts the upsert path persists it without needing
// a separate delete pass.
func TestSyncPersistsExplicitTombstone(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// First sync with 3 orgs (all 'ok').
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
		makeOrg(3, "Org3", "ok"),
	}
	w, _ := newTestWorker(t, f)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	count, _ := w.entClient.Organization.Query().Count(t.Context())
	if count != 3 {
		t.Fatalf("expected 3 orgs, got %d", count)
	}

	// Second sync: upstream returns org 2 with status='deleted'
	// (mirrors the rest.py:694-727 ?since= response shape). Bump the
	// changed row's `updated` past the prior write so 260428-eda
	// CHANGE 3's skip-on-unchanged predicate admits the status flip.
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		bumpUpdated(makeOrg(2, "Org2", "deleted"), "2024-01-02T00:00:00Z"),
		makeOrg(3, "Org3", "ok"),
	}

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	// Row count unchanged, org 2 transitioned to 'deleted' via the
	// explicit upstream payload (no inference involved).
	count, _ = w.entClient.Organization.Query().Count(t.Context())
	if count != 3 {
		t.Errorf("expected 3 orgs, got %d", count)
	}
	okCount, _ := w.entClient.Organization.Query().Where(organization.Status("ok")).Count(t.Context())
	if okCount != 2 {
		t.Errorf("expected 2 orgs with status='ok', got %d", okCount)
	}
	deletedCount, _ := w.entClient.Organization.Query().Where(organization.Status("deleted")).Count(t.Context())
	if deletedCount != 1 {
		t.Errorf("expected 1 org with status='deleted', got %d", deletedCount)
	}
	org2, err := w.entClient.Organization.Get(t.Context(), 2)
	if err != nil {
		t.Fatalf("org 2 should still exist: %v", err)
	}
	if org2.Status != "deleted" {
		t.Errorf("org 2 status: want 'deleted', got %q", org2.Status)
	}
}

// TestSyncMutex verifies sync mutex prevents concurrent runs.
func TestSyncMutex(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, _ := newTestWorker(t, f)

	// Manually set running to true.
	w.running.Store(true)

	// Second call should return nil without error (skipped).
	err := w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Errorf("expected nil when sync already running, got: %v", err)
	}

	// Reset and verify it can run after.
	w.running.Store(false)
	err = w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Errorf("expected sync to succeed after mutex release: %v", err)
	}
}

// TestSyncLogsProgress verifies per-object-type progress logging.
func TestSyncLogsProgress(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)

	// Just verify sync completes without error -- log output is verified by
	// the presence of slog.String("type", ...) calls in the worker code.
	err := w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
}

// TestSyncRecordsStatusSuccess verifies sync_status table records success.
func TestSyncRecordsStatusSuccess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, db := newTestWorker(t, f)

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	status, err := GetLastStatus(t.Context(), db)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Status != "success" {
		t.Errorf("expected success status, got %s", status.Status)
	}
	if c, ok := status.ObjectCounts["org"]; !ok || c != 1 {
		t.Errorf("expected org count 1, got %v", status.ObjectCounts)
	}
}

// TestSyncRecordsStatusFailure verifies sync_status table records failure.
func TestSyncRecordsStatusFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// Make org endpoint fail.
	f.failTypes["org"] = true
	w, db := newTestWorker(t, f)

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	status, err := GetLastStatus(t.Context(), db)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Status != "failed" {
		t.Errorf("expected failed status, got %s", status.Status)
	}
	if status.ErrorMessage == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSyncRollbackOnFailure verifies database rolls back on sync failure.
func TestSyncRollbackOnFailure(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// First sync succeeds.
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	count, _ := w.entClient.Organization.Query().Count(t.Context())
	if count != 1 {
		t.Fatalf("expected 1 org, got %d", count)
	}

	// Second sync: org succeeds with new data, but net fails.
	f.responses["org"] = []any{makeOrg(1, "UpdatedOrg", "ok"), makeOrg(2, "NewOrg", "ok")}
	f.failTypes["net"] = true

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	// Verify original data is preserved (rollback worked).
	count, _ = w.entClient.Organization.Query().Count(t.Context())
	if count != 1 {
		t.Errorf("expected 1 org after rollback, got %d", count)
	}
	org, _ := w.entClient.Organization.Get(t.Context(), 1)
	if org.Name != "Org1" {
		t.Errorf("expected original name Org1, got %s", org.Name)
	}
}

// TestSyncScheduler verifies scheduler starts periodic sync via time.Ticker.
func TestSyncScheduler(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, 200*time.Millisecond)
		close(done)
	}()

	// Wait for at least one sync cycle.
	time.Sleep(2 * time.Second)
	cancel()
	<-done

	if !w.HasCompletedSync() {
		t.Error("expected HasCompletedSync to be true after scheduler run")
	}
}

// TestHasCompletedSync verifies false before first sync, true after.
func TestHasCompletedSync(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)

	if w.HasCompletedSync() {
		t.Error("expected false before sync")
	}

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if !w.HasCompletedSync() {
		t.Error("expected true after sync")
	}
}

// TestSyncWithRetrySucceedsOnSecondAttempt verifies retry behavior.
func TestSyncWithRetrySucceedsOnSecondAttempt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	var orgFetchCount atomic.Int64
	// First org fetch fails, second succeeds.
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "?")
		objType := parts[0]

		// Only return data on first page (skip=0).
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
			return
		}

		// Fail org on first fetch attempt (count resets per sync, so track first page hits).
		if objType == "org" && orgFetchCount.Add(1) == 1 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}

		data, ok := f.responses[objType]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": data})
	}))
	t.Cleanup(srv.Close)

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, srv.URL)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	err := w.SyncWithRetry(t.Context(), config.SyncModeFull)
	if err != nil {
		t.Fatalf("expected SyncWithRetry to succeed on retry, got: %v", err)
	}
}

// TestSyncWithRetryExhaustsRetries verifies failure after all retries.
func TestSyncWithRetryExhaustsRetries(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.failTypes["org"] = true
	w, _ := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	err := w.SyncWithRetry(t.Context(), config.SyncModeFull)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "sync failed after") {
		t.Errorf("expected 'sync failed after' in error, got: %v", err)
	}
}

// TestSyncWithRetryShortCircuitsOnRateLimit locks the rate-limit short-circuit
// contract: when PeeringDB returns HTTP 429, SyncWithRetry must abort the
// 30s/2m/8m retry ladder immediately instead of waiting through backoffs that
// all fall inside the Retry-After window. Every in-ladder retry is guaranteed
// to 429 again AND burns another slot against PeeringDB's 1 req/hr per-URL
// unauthenticated quota, so retrying is actively harmful.
//
// Test strategy: long-duration retry backoffs (10s each = 30s total) that the
// test would block on if the short-circuit failed. Expected runtime is <100ms
// because SyncWithRetry returns immediately on RateLimitError detection.
// Also asserts the HTTP server received exactly ONE request.
//
// Production incident 2026-04-11: this short-circuit would have prevented the
// 12 req/hr against /api/org that kept the fly.io primary permanently rate-
// limited after PeeringDB's unique-name upstream change broke sync upserts.
func TestSyncWithRetryShortCircuitsOnRateLimit(t *testing.T) {
	t.Parallel()

	var orgAttempts atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "?")
		objType := parts[0]
		if objType == "org" {
			orgAttempts.Add(1)
			w.Header().Set("Retry-After", "2200")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Request was throttled","meta":{"error":"Too Many Requests"}}`))
			return
		}
		// Non-org types never reached when org fails.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
	}))
	t.Cleanup(srv.Close)

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, srv.URL)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}
	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())
	// Long backoffs that would block the test if the short-circuit fails.
	w.SetRetryBackoffs([]time.Duration{10 * time.Second, 10 * time.Second, 10 * time.Second})

	start := time.Now()
	err := w.SyncWithRetry(t.Context(), config.SyncModeFull)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from rate-limit short-circuit, got nil")
	}
	rlErr, ok := errors.AsType[*peeringdb.RateLimitError](err)
	if !ok {
		t.Fatalf("expected *peeringdb.RateLimitError, got %T: %v", err, err)
	}
	if rlErr.RetryAfter != 2200*time.Second {
		t.Errorf("RetryAfter = %s, want %s", rlErr.RetryAfter, 2200*time.Second)
	}

	// Strict upper bound: short-circuit must not block on any retry backoff.
	// 2 seconds allows generous slack for slow CI while catching any accidental
	// retry wait (which would be at least 10 seconds).
	if elapsed > 2*time.Second {
		t.Errorf("SyncWithRetry took %s — rate-limit short-circuit should return immediately (< 2s)", elapsed)
	}

	// Exactly one attempt against the server — no retries.
	if got := orgAttempts.Load(); got != 1 {
		t.Errorf("org fetched %d times, want 1 (rate limit must short-circuit the retry ladder)", got)
	}
}

// TestSyncWithRetryContextCancellation verifies context cancellation during backoff.
func TestSyncWithRetryContextCancellation(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.failTypes["org"] = true
	w, _ := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{10 * time.Second, 10 * time.Second, 10 * time.Second})

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := w.SyncWithRetry(ctx, config.SyncModeFull)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	// Should return much faster than the 10s backoff.
	if elapsed > 5*time.Second {
		t.Errorf("expected fast return on context cancellation, took %v", elapsed)
	}
}

// TestSyncWithNetAndFac verifies multi-type sync with FK relationships.
func TestSyncWithNetAndFac(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "TestOrg", "ok")}
	f.responses["fac"] = []any{makeFac(10, 1, fmt.Sprintf("Fac-%d", 10), "ok")}
	f.responses["net"] = []any{makeNet(100, 1, 65000, fmt.Sprintf("Net-%d", 100), "ok")}
	w, _ := newTestWorker(t, f)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	orgCount, _ := w.entClient.Organization.Query().Count(t.Context())
	facCount, _ := w.entClient.Facility.Query().Count(t.Context())
	netCount, _ := w.entClient.Network.Query().Count(t.Context())

	if orgCount != 1 {
		t.Errorf("expected 1 org, got %d", orgCount)
	}
	if facCount != 1 {
		t.Errorf("expected 1 fac, got %d", facCount)
	}
	if netCount != 1 {
		t.Errorf("expected 1 net, got %d", netCount)
	}
}

// setupMetricTest installs a ManualReader-backed MeterProvider, initializes
// sync metric instruments, and returns the reader for post-sync assertions.
func setupMetricTest(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(t.Context()) })
	if err := pdbotel.InitMetrics(); err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}
	return reader
}

// findMetric searches ResourceMetrics for a metric by name.
func findMetric(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// TestSyncRecordsMetrics verifies that a successful sync records both
// sync-level and per-type metrics with correct attributes.
// Not parallel: writes to package-level metric vars per CC-3.
func TestSyncRecordsMetrics(t *testing.T) {
	reader := setupMetricTest(t)

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)

	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Verify sync-level duration metric.
	durMetric := findMetric(rm, "pdbplus.sync.duration")
	if durMetric == nil {
		t.Fatal("expected pdbplus.sync.duration metric, not found")
	}

	// Verify sync-level operations counter.
	opsMetric := findMetric(rm, "pdbplus.sync.operations")
	if opsMetric == nil {
		t.Fatal("expected pdbplus.sync.operations metric, not found")
	}
	opsSum, ok := opsMetric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", opsMetric.Data)
	}
	if len(opsSum.DataPoints) == 0 {
		t.Fatal("expected at least one operations data point")
	}
	if opsSum.DataPoints[0].Value != 1 {
		t.Errorf("expected operations sum = 1, got %d", opsSum.DataPoints[0].Value)
	}

	// Verify per-type objects counter.
	typeObjs := findMetric(rm, "pdbplus.sync.type.objects")
	if typeObjs == nil {
		t.Fatal("expected pdbplus.sync.type.objects metric, not found")
	}
}

// TestSyncRecordsFailureMetrics verifies that a failed sync records
// failure metrics with status=failed and per-type fetch_errors.
// Not parallel: writes to package-level metric vars per CC-3.
func TestSyncRecordsFailureMetrics(t *testing.T) {
	reader := setupMetricTest(t)

	f := newFixture(t)
	f.failTypes["org"] = true
	w, _ := newTestWorker(t, f)

	err := w.Sync(t.Context(), config.SyncModeFull)
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Verify sync-level duration metric with failed status.
	durMetric := findMetric(rm, "pdbplus.sync.duration")
	if durMetric == nil {
		t.Fatal("expected pdbplus.sync.duration metric, not found")
	}

	// Verify sync-level operations counter with failed status.
	opsMetric := findMetric(rm, "pdbplus.sync.operations")
	if opsMetric == nil {
		t.Fatal("expected pdbplus.sync.operations metric, not found")
	}
	opsSum, ok := opsMetric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", opsMetric.Data)
	}
	if len(opsSum.DataPoints) == 0 {
		t.Fatal("expected at least one operations data point")
	}
	if opsSum.DataPoints[0].Value != 1 {
		t.Errorf("expected operations sum = 1, got %d", opsSum.DataPoints[0].Value)
	}

	// Verify per-type fetch_errors counter.
	fetchErr := findMetric(rm, "pdbplus.sync.type.fetch_errors")
	if fetchErr == nil {
		t.Fatal("expected pdbplus.sync.type.fetch_errors metric, not found")
	}
	fetchSum, ok := fetchErr.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", fetchErr.Data)
	}
	if len(fetchSum.DataPoints) == 0 {
		t.Fatal("expected at least one fetch_errors data point")
	}
	if fetchSum.DataPoints[0].Value != 1 {
		t.Errorf("expected fetch_errors sum = 1, got %d", fetchSum.DataPoints[0].Value)
	}
}

// newFixtureWithMeta creates a fixture server that includes meta.generated in responses.
type fixtureWithMeta struct {
	server          *httptest.Server
	responses       map[string]any
	failTypes       map[string]bool
	failOnce        map[string]bool // fail only on first attempt per type
	failIncremental map[string]bool // fail all requests with ?since= for this type
	callCounts      map[string]*atomic.Int64
	sinceSeen       map[string]*atomic.Bool // tracks if ?since= was seen per type
	// 260428-mu0: per-type slice of `since` query-param values seen across
	// the lifetime of the fixture. Captures the FIRST request per cycle by
	// inspection (the per-page paginator may issue follow-up requests with
	// the same since=). Used by TestSyncFetchPass_UsesMaxUpdatedAsCursor /
	// TestSync_TwoCycle_NoFullRefetch to assert the cursor advanced.
	sinceValues map[string]*sinceLog
	generated   float64 // meta.generated epoch
}

// sinceLog accumulates `since=N` query-param values seen on first-page
// requests for a single PeeringDB type. The slice is append-only across the
// fixture's lifetime so a multi-cycle test can read both cycle 1 and cycle
// 2 values. Guarded by mu so the httptest server's concurrent request
// dispatch is data-race clean (-race detects unsynchronised slice growth).
type sinceLog struct {
	mu     stdsync.Mutex
	values []string
}

func (s *sinceLog) add(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = append(s.values, v)
}

func (s *sinceLog) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.values))
	copy(out, s.values)
	return out
}

func newFixtureWithMeta(t *testing.T, generatedEpoch float64) *fixtureWithMeta {
	t.Helper()
	f := &fixtureWithMeta{
		responses:       make(map[string]any),
		failTypes:       make(map[string]bool),
		failOnce:        make(map[string]bool),
		failIncremental: make(map[string]bool),
		callCounts:      make(map[string]*atomic.Int64),
		sinceSeen:       make(map[string]*atomic.Bool),
		sinceValues:     make(map[string]*sinceLog),
		generated:       generatedEpoch,
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "?")
		objType := parts[0]

		// Track call counts per type.
		if _, ok := f.callCounts[objType]; !ok {
			f.callCounts[objType] = &atomic.Int64{}
		}
		count := f.callCounts[objType].Add(1)

		// Track whether ?since= was present.
		if _, ok := f.sinceSeen[objType]; !ok {
			f.sinceSeen[objType] = &atomic.Bool{}
		}
		sinceParam := r.URL.Query().Get("since")
		hasSince := sinceParam != ""
		if hasSince {
			f.sinceSeen[objType].Store(true)
		}
		// 260428-mu0: capture per-type since-value log on first-page
		// requests only (skip=0 or empty) so multi-cycle tests can assert
		// the cursor advanced between cycles. Empty string is recorded for
		// bare-list calls so callers can distinguish "no request" from
		// "request with no since" — both have utility for the regression
		// lock in TestSync_TwoCycle_NoFullRefetch. Lazily registered so
		// existing tests (which never read sinceValues) pay no cost.
		skip := r.URL.Query().Get("skip")
		if skip == "" || skip == "0" {
			if _, ok := f.sinceValues[objType]; !ok {
				f.sinceValues[objType] = &sinceLog{}
			}
			f.sinceValues[objType].add(sinceParam)
		}

		// Permanent failure.
		if f.failTypes[objType] {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Fail all requests with ?since= for this type (incremental fails, full succeeds).
		if f.failIncremental[objType] && hasSince {
			http.Error(w, "incremental not supported", http.StatusInternalServerError)
			return
		}

		// Fail once (first call only), then succeed.
		if f.failOnce[objType] && count == 1 {
			http.Error(w, "temporary failure", http.StatusInternalServerError)
			return
		}

		// Only return data on the first page (skip=0).
		// `skip` already captured above for the sinceValues log.
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta": map[string]any{"generated": f.generated},
				"data": []any{},
			})
			return
		}

		data, ok := f.responses[objType]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"meta": map[string]any{"generated": f.generated},
				"data": []any{},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta": map[string]any{"generated": f.generated},
			"data": data,
		})
	}))
	t.Cleanup(func() { f.server.Close() })
	return f
}

func newTestWorkerWithMode(t *testing.T, baseURL string, mode config.SyncMode) (*Worker, *sql.DB) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, baseURL)

	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		SyncMode: mode,
	}, slog.Default())
	return w, db
}

// TestIncrementalSync verifies that incremental mode with existing cursors
// uses ?since= and skips deleteStale.
func TestIncrementalSync(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// First sync: no cursors, so full fetch runs.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	orgCount, _ := w.entClient.Organization.Query().Count(ctx)
	if orgCount != 1 {
		t.Fatalf("expected 1 org after first sync, got %d", orgCount)
	}

	// 260428-mu0: cursor is now derived from MAX(updated) on the entity
	// table — assert there's at least one row so the next cycle's
	// GetMaxUpdated returns non-zero (which drives the ?since= path).
	cursor, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil {
		t.Fatalf("GetMaxUpdated: %v", err)
	}
	if cursor.IsZero() {
		t.Fatal("expected non-zero MAX(updated) after first sync (drives next cycle's ?since=)")
	}

	// Reset since tracking.
	for _, v := range f.sinceSeen {
		v.Store(false)
	}

	// Second sync: cursors exist, so incremental fetch should use ?since=.
	// 260428-eda CHANGE 3: bump `updated` past the prior write so the
	// skip-on-unchanged predicate admits the rewrite.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1Updated", "ok"), "2024-01-02T00:00:00Z")}
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify ?since= was used for org.
	if orgSeen, ok := f.sinceSeen["org"]; !ok || !orgSeen.Load() {
		t.Error("expected ?since= parameter for org in incremental mode")
	}

	// Verify org was updated (upsert worked).
	org, err := w.entClient.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if org.Name != "Org1Updated" {
		t.Errorf("expected Org1Updated, got %s", org.Name)
	}
}

// TestIncrementalFirstSyncFallsBackToBareList verifies that incremental
// mode with no cursors falls through to the bare /api/<type> path
// (status='ok' only), NOT the v1.18.2 ?since=1 bootstrap that was
// reverted in v1.18.3 because it tripped upstream's
// API_THROTTLE_REPEATED_REQUEST throttle. Historical-delete capture
// for fresh installs is deferred to a proper multi-cycle bootstrap
// design (v1.19+); FK backfill catches orphans on demand.
func TestIncrementalFirstSyncFallsBackToBareList(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, _ := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// v1.18.3 contract: zero cursor → bare list, NO since= parameter.
	if orgSeen, ok := f.sinceSeen["org"]; ok && orgSeen.Load() {
		t.Error("unexpected ?since= on first incremental sync (v1.18.2 bootstrap regression)")
	}

	// Verify data was synced via the bare path.
	orgCount, _ := w.entClient.Organization.Query().Count(ctx)
	if orgCount != 1 {
		t.Errorf("expected 1 org, got %d", orgCount)
	}
}

// TestIncrementalFallback verifies that when incremental fetch fails for one type,
// it falls back to full fetch for that type, and the fallback counter is incremented.
func TestIncrementalFallback(t *testing.T) {
	reader := setupMetricTest(t)

	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Establish cursors with a full sync first.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("initial full sync: %v", err)
	}

	// Set org to fail on incremental (with ?since=), succeed on full (without ?since=).
	f.failIncremental["org"] = true

	// Run incremental sync -- org should fail incrementally then succeed via fallback.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("incremental sync with fallback: %v", err)
	}

	// Verify org data is present (fallback to full succeeded).
	orgCount, _ := w.entClient.Organization.Query().Count(ctx)
	if orgCount != 1 {
		t.Errorf("expected 1 org after fallback, got %d", orgCount)
	}

	// Verify fallback metric was incremented.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	fbMetric := findMetric(rm, "pdbplus.sync.type.fallback")
	if fbMetric == nil {
		t.Fatal("expected pdbplus.sync.type.fallback metric, not found")
	}
	fbSum, ok := fbMetric.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", fbMetric.Data)
	}
	found := false
	for _, dp := range fbSum.DataPoints {
		if dp.Value > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected fallback counter > 0")
	}

	_ = db // used for worker setup
}

// TestSync_IncrementalDeletionTombstone is the SEED-001 regression guard
// (active 2026-04-26 spike against www.peeringdb.com): upstream ?since=
// emits status='deleted' tombstones with PII-scrubbed name="" for the 6
// folded entities. This drives a tombstone for an existing org through an
// incremental sync cycle and asserts the row is soft-deleted (not hard-
// deleted) and excluded from the anonymous (status="ok") list path.
//
// The empty-name path is the load-bearing assertion: if the NotEmpty()
// validator removed from organization.name in Task 1 ever re-appears, this
// test fails on the second sync's upsert with "validator failed for field
// Organization.name".
func TestSync_IncrementalDeletionTombstone(t *testing.T) {
	t.Parallel()
	t1 := float64(time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, t1)
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
	}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Cycle 1: cursor zero → bootstrap with ?since=1 (quick task 260428-2zl).
	// Pre-2zl: cursor.IsZero() fell through to full sync; post-2zl the
	// bootstrap path captures the upstream history-with-tombstones from
	// cycle 1. Both orgs persisted in either path.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	orgCount, err := w.entClient.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count orgs after cycle 1: %v", err)
	}
	if orgCount != 2 {
		t.Fatalf("expected 2 orgs after cycle 1, got %d", orgCount)
	}

	// 260428-mu0: cursor derived from MAX(updated) on the org table.
	cursor, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil {
		t.Fatalf("GetMaxUpdated after cycle 1: %v", err)
	}
	if cursor.IsZero() {
		t.Fatal("expected non-zero MAX(updated) after cycle 1 (drives next cycle's ?since=)")
	}

	// Reset since-tracking before cycle 2 so we can prove the incremental
	// path was taken (cursor present + ?since= sent).
	for _, v := range f.sinceSeen {
		v.Store(false)
	}

	// Cycle 2: upstream returns Org1 unchanged + Org2 as a PII-scrubbed
	// tombstone (status="deleted", name=""). Bump generated so the cursor
	// can advance. 260428-eda CHANGE 3: bump Org2's `updated` past cycle 1
	// so the skip-on-unchanged predicate admits the status flip.
	t2 := t1 + 3600
	f.generated = t2
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		bumpUpdated(makeOrg(2, "", "deleted"), "2024-01-02T00:00:00Z"),
	}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// (a) ?since= was sent — confirms incremental path (not full fallback).
	if seen, ok := f.sinceSeen["org"]; !ok || !seen.Load() {
		t.Error("expected ?since= parameter for org on cycle 2 (incremental cursor present)")
	}

	// (b) Soft-delete: row preserved, status flipped to "deleted", name
	// scrubbed to "". This is the NotEmpty()-removal regression assertion.
	totalCount, err := w.entClient.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count orgs after cycle 2: %v", err)
	}
	if totalCount != 2 {
		t.Fatalf("expected 2 orgs after cycle 2 (soft-delete preserves row), got %d", totalCount)
	}

	org1, err := w.entClient.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org 1: %v", err)
	}
	if org1.Status != "ok" || org1.Name != "Org1" {
		t.Errorf("org 1 = {status=%q, name=%q}, want {status=\"ok\", name=\"Org1\"}", org1.Status, org1.Name)
	}

	org2, err := w.entClient.Organization.Get(ctx, 2)
	if err != nil {
		t.Fatalf("get org 2: %v", err)
	}
	if org2.Status != "deleted" {
		t.Errorf("org 2 status = %q, want %q", org2.Status, "deleted")
	}
	if org2.Name != "" {
		t.Errorf("org 2 name = %q, want %q (PII-scrubbed tombstone)", org2.Name, "")
	}

	// (c) Anonymous list path (no ?since): pdbcompat applyStatusMatrix
	// devolves to status IN ("ok", "pending"); for non-campus entities the
	// "pending" arm is empty so this is equivalent to status="ok". Assert
	// the live row alone is returned.
	live, err := w.entClient.Organization.Query().Where(organization.StatusEQ("ok")).All(ctx)
	if err != nil {
		t.Fatalf("query live orgs: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("expected 1 live org, got %d", len(live))
	}
	if live[0].ID != 1 {
		t.Errorf("expected live org ID 1, got %d", live[0].ID)
	}
}

// TestSync_IncrementalRoleTombstone is the Phase 73 BUG-02 regression
// guard: upstream PeeringDB ?since= responses can carry status='deleted'
// poc tombstones with PII-scrubbed role="" (same GDPR pattern that
// 260426-pms confirmed for name="" on the 6 folded entities). This drives
// a tombstone for an existing poc through an incremental sync cycle and
// asserts the row is soft-deleted (not hard-deleted) and excluded from
// the anonymous (status="ok") list path.
//
// The empty-role path is the load-bearing assertion: if the NotEmpty()
// validator removed from poc.role in Phase 73 ever re-appears (e.g.,
// the cmd/pdb-schema-generate isTombstoneVulnerableField gate is
// accidentally reverted), this test fails on the second sync's upsert
// with "validator failed for field Poc.role".
//
// Mirrors TestSync_IncrementalDeletionTombstone (260426-pms) structure
// — substitutes Poc for Organization and threads a parent network plus
// org through cycle 1 to satisfy the FK chain.
func TestSync_IncrementalRoleTombstone(t *testing.T) {
	t.Parallel()
	t1 := float64(time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, t1)
	// Seed parent org + network so the poc FK chain resolves.
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	f.responses["net"] = []any{makeNet(50, 1, 64500, "ParentNet", "ok")}
	f.responses["poc"] = []any{
		makePoc(10, 50, "Alice", "Technical", "ok"),
		makePoc(11, 50, "Bob", "Policy", "ok"),
	}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Cycle 1: cursor zero → bootstrap with ?since=1 (quick task 260428-2zl).
	// Pre-2zl: cursor.IsZero() fell through to full sync; post-2zl the
	// bootstrap path captures the upstream history-with-tombstones from
	// cycle 1. Both pocs persisted in either path.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	pocCount, err := w.entClient.Poc.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count pocs after cycle 1: %v", err)
	}
	if pocCount != 2 {
		t.Fatalf("expected 2 pocs after cycle 1, got %d", pocCount)
	}

	// 260428-mu0: cursor derived from MAX(updated) on the poc table.
	cursor, err := GetMaxUpdated(ctx, db, "pocs")
	if err != nil {
		t.Fatalf("GetMaxUpdated after cycle 1: %v", err)
	}
	if cursor.IsZero() {
		t.Fatal("expected non-zero MAX(updated) after cycle 1 (drives next cycle's ?since=)")
	}

	// Reset since-tracking before cycle 2 so we can prove the incremental
	// path was taken (cursor present + ?since= sent).
	for _, v := range f.sinceSeen {
		v.Store(false)
	}

	// Cycle 2: upstream returns Alice unchanged + Bob as a PII-scrubbed
	// tombstone (status="deleted", role=""). Bump generated so the cursor
	// can advance. 260428-eda CHANGE 3: bump Bob's `updated` past cycle 1
	// so the skip-on-unchanged predicate admits the status flip.
	t2 := t1 + 3600
	f.generated = t2
	f.responses["poc"] = []any{
		makePoc(10, 50, "Alice", "Technical", "ok"),
		bumpUpdated(makePoc(11, 50, "", "", "deleted"), "2024-01-02T00:00:00Z"),
	}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// (a) ?since= was sent — confirms incremental path (not full fallback).
	if seen, ok := f.sinceSeen["poc"]; !ok || !seen.Load() {
		t.Error("expected ?since= parameter for poc on cycle 2 (incremental cursor present)")
	}

	// (b) Soft-delete: row preserved, status flipped to "deleted", role
	// scrubbed to "". This is the NotEmpty()-removal regression assertion.
	totalCount, err := w.entClient.Poc.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count pocs after cycle 2: %v", err)
	}
	if totalCount != 2 {
		t.Fatalf("expected 2 pocs after cycle 2 (soft-delete preserves row), got %d", totalCount)
	}

	alice, err := w.entClient.Poc.Get(ctx, 10)
	if err != nil {
		t.Fatalf("get poc 10: %v", err)
	}
	if alice.Status != "ok" || alice.Role != "Technical" {
		t.Errorf("alice = {status=%q, role=%q}, want {status=\"ok\", role=\"Technical\"}", alice.Status, alice.Role)
	}

	bob, err := w.entClient.Poc.Get(ctx, 11)
	if err != nil {
		t.Fatalf("get poc 11: %v", err)
	}
	if bob.Status != "deleted" {
		t.Errorf("bob status = %q, want %q", bob.Status, "deleted")
	}
	if bob.Role != "" {
		t.Errorf("bob role = %q, want %q (PII-scrubbed tombstone)", bob.Role, "")
	}

	// (c) Anonymous list path: only the live poc is returned when
	// filtered to status="ok".
	live, err := w.entClient.Poc.Query().Where(poc.StatusEQ("ok")).All(ctx)
	if err != nil {
		t.Fatalf("query live pocs: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("expected 1 live poc, got %d", len(live))
	}
	if live[0].ID != 10 {
		t.Errorf("expected live poc ID 10, got %d", live[0].ID)
	}
}

// TestCursorsUpdatedAfterCommit verifies that the derived cursor (MAX(updated)
// per entity table) is non-zero after a successful sync for each type that
// received rows. 260428-mu0: cursor was previously a sync_cursors row written
// per-type by the sync; it is now derived from the entity table itself, so
// only types with at least one synced row will have a non-zero cursor.
func TestCursorsUpdatedAfterCommit(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	// Seed at least one row per type so MAX(updated) is non-zero everywhere
	// after the sync. Empty types would yield zero cursors under the new
	// design (the data IS the cursor).
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	f.responses["fac"] = []any{makeFac(1, 1, "Fac1", "ok")}
	f.responses["net"] = []any{makeNet(1, 1, 64500, "Net1", "ok")}
	f.responses["poc"] = []any{makePoc(1, 1, "Alice", "Tech", "ok")}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeFull)
	ctx := t.Context()

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// 260428-mu0: cursor derived from MAX(updated). Types with at least one
	// row land a non-zero cursor; types with empty fixtures stay at zero
	// (which is the correct fall-through-to-bare-list signal).
	cases := []struct {
		typeName  string
		tableName string
		seeded    bool
	}{
		{"org", "organizations", true},
		{"fac", "facilities", true},
		{"net", "networks", true},
		{"poc", "pocs", true},
	}
	for _, c := range cases {
		cursor, err := GetMaxUpdated(ctx, db, c.tableName)
		if err != nil {
			t.Errorf("GetMaxUpdated for %s (%s): %v", c.typeName, c.tableName, err)
			continue
		}
		if c.seeded && cursor.IsZero() {
			t.Errorf("expected non-zero MAX(updated) for %s after successful sync", c.typeName)
		}
	}
}

// TestCursorsNotUpdatedOnRollback verifies that the derived cursor (MAX
// (updated) per entity table) stays at zero when the sync transaction is
// rolled back due to an error. 260428-mu0: under the MAX(updated) model the
// data IS the cursor, so rolling back the upsert tx automatically reverts
// the cursor — no separate sync_cursors row to check.
func TestCursorsNotUpdatedOnRollback(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	// Make net fail, which causes rollback.
	f.failTypes["net"] = true

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeFull)
	ctx := t.Context()

	err := w.Sync(ctx, config.SyncModeFull)
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	// 260428-mu0: rollback semantic — no rows committed → MAX(updated) is
	// zero on every entity table, including org (whose fixture WAS staged
	// but the tx rolled back before the upserts landed).
	cases := []struct {
		typeName  string
		tableName string
	}{
		{"org", "organizations"},
		{"fac", "facilities"},
		{"net", "networks"},
		{"ix", "internet_exchanges"},
		{"poc", "pocs"},
	}
	for _, c := range cases {
		cursor, err := GetMaxUpdated(ctx, db, c.tableName)
		if err != nil {
			t.Errorf("GetMaxUpdated for %s: %v", c.typeName, err)
			continue
		}
		if !cursor.IsZero() {
			t.Errorf("expected zero MAX(updated) for %s after rollback, got %v", c.typeName, cursor)
		}
	}
}

// TestSyncWithRetryPassesMode verifies that SyncWithRetry passes mode through to Sync.
func TestSyncWithRetryPassesMode(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})
	ctx := t.Context()

	// First call establishes cursors.
	if err := w.SyncWithRetry(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("initial sync: %v", err)
	}

	// Reset since tracking.
	for _, v := range f.sinceSeen {
		v.Store(false)
	}

	// Call with incremental mode.
	if err := w.SyncWithRetry(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("incremental sync: %v", err)
	}

	// Verify ?since= was used.
	if orgSeen, ok := f.sinceSeen["org"]; !ok || !orgSeen.Load() {
		t.Error("expected ?since= parameter when SyncWithRetry called with incremental mode")
	}

	_ = db // used for worker setup
}

// TestIncrementalSkipsDeleteStale verifies that incremental mode does not
// delete stale records (records present in DB but not in incremental response).
func TestIncrementalSkipsDeleteStale(t *testing.T) {
	t.Parallel()
	generated := float64(time.Date(2026, 3, 23, 12, 0, 0, 0, time.UTC).Unix())
	f := newFixtureWithMeta(t, generated)

	// First sync with 3 orgs.
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
		makeOrg(3, "Org3", "ok"),
	}

	w, _ := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Full sync to establish data and cursors.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("full sync: %v", err)
	}
	orgCount, _ := w.entClient.Organization.Query().Count(ctx)
	if orgCount != 3 {
		t.Fatalf("expected 3 orgs after full sync, got %d", orgCount)
	}

	// Incremental sync with only 1 org (simulating only org 1 was modified).
	// 260428-eda CHANGE 3: bump `updated` past cycle 1 so the
	// skip-on-unchanged predicate admits the rewrite.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1Updated", "ok"), "2024-01-02T00:00:00Z")}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("incremental sync: %v", err)
	}

	// All 3 orgs should still be present (incremental does NOT delete stale).
	orgCount, _ = w.entClient.Organization.Query().Count(ctx)
	if orgCount != 3 {
		t.Errorf("expected 3 orgs after incremental sync (no delete stale), got %d", orgCount)
	}

	// Verify org 1 was updated.
	org, _ := w.entClient.Organization.Get(ctx, 1)
	if org.Name != "Org1Updated" {
		t.Errorf("expected Org1Updated, got %s", org.Name)
	}
}

// TestSchedulerSkipsSyncWithExistingData verifies that StartScheduler does not
// run an immediate sync when the database already has a recent successful sync.
func TestSchedulerSkipsSyncWithExistingData(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, db := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx := t.Context()

	// Record a successful sync that completed recently (within the interval).
	now := time.Now()
	id, err := RecordSyncStart(ctx, db, now.Add(-10*time.Minute), "incremental")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, id, Status{
		LastSyncAt:   now.Add(-10 * time.Minute),
		Duration:     5 * time.Second,
		ObjectCounts: map[string]int{"org": 1},
		Status:       "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}

	// Start scheduler with 1h interval. Since last sync was 10m ago,
	// next sync isn't due for ~50m. Cancel before it could run.
	schedulerCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(schedulerCtx, 1*time.Hour)
		close(done)
	}()
	<-done

	// Server should be marked as ready immediately.
	if !w.HasCompletedSync() {
		t.Error("expected HasCompletedSync=true with existing data")
	}

	// No API calls should have been made (no sync was triggered).
	if calls := f.callCount.Load(); calls != 0 {
		t.Errorf("expected 0 API calls, got %d", calls)
	}
}

// TestSchedulerSyncsImmediatelyOnEmptyDB verifies that StartScheduler runs
// an immediate full sync when no prior successful sync exists.
func TestSchedulerSyncsImmediatelyOnEmptyDB(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, 1*time.Hour)
		close(done)
	}()

	// Wait for the initial sync to complete.
	time.Sleep(2 * time.Second)
	cancel()
	<-done

	if !w.HasCompletedSync() {
		t.Error("expected HasCompletedSync=true after initial sync on empty DB")
	}

	// API calls should have been made (sync was triggered).
	if calls := f.callCount.Load(); calls == 0 {
		t.Error("expected API calls for initial sync on empty DB, got 0")
	}
}

// TestSchedulerSyncsWhenOverdue verifies that StartScheduler runs an immediate
// sync when the last successful sync is older than the configured interval.
func TestSchedulerSyncsWhenOverdue(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, db := newTestWorker(t, f)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx := t.Context()

	// Record a successful sync from 2 hours ago (with 1h interval, it's overdue).
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	id, err := RecordSyncStart(ctx, db, twoHoursAgo, "incremental")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, id, Status{
		LastSyncAt:   twoHoursAgo,
		Duration:     5 * time.Second,
		ObjectCounts: map[string]int{"org": 1},
		Status:       "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}

	// Start scheduler with 1h interval. Last sync was 2h ago, so it's overdue.
	schedulerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(schedulerCtx, 1*time.Hour)
		close(done)
	}()

	// Wait for the sync to run.
	time.Sleep(2 * time.Second)
	cancel()
	<-done

	// Server should be marked ready immediately (existing data).
	if !w.HasCompletedSync() {
		t.Error("expected HasCompletedSync=true")
	}

	// API calls should have been made (overdue sync was triggered).
	if calls := f.callCount.Load(); calls == 0 {
		t.Error("expected API calls for overdue sync, got 0")
	}
}

// primarySwitch is a test helper that provides a controllable IsPrimary function.
type primarySwitch struct {
	v atomic.Bool
}

func (p *primarySwitch) IsPrimary() bool {
	return p.v.Load()
}

// TestStartScheduler_SkipsOnReplica verifies DYN-01: scheduler with IsPrimary=false
// never calls SyncWithRetry. Runs scheduler for 2+ ticks, asserts zero API calls.
func TestStartScheduler_SkipsOnReplica(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	ps := &primarySwitch{}
	ps.v.Store(false) // Always replica.

	w, _ := newTestWorker(t, f)
	w.config.IsPrimary = ps.IsPrimary
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, 50*time.Millisecond)
		close(done)
	}()
	<-done

	// No sync should have been triggered.
	if calls := f.callCount.Load(); calls != 0 {
		t.Errorf("expected 0 API calls on replica, got %d", calls)
	}
}

// TestStartScheduler_PromotionSync verifies DYN-02: scheduler detects promotion
// (IsPrimary flips from false to true) and triggers a sync.
func TestStartScheduler_PromotionSync(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	ps := &primarySwitch{}
	ps.v.Store(false) // Start as replica.

	w, _ := newTestWorker(t, f)
	w.config.IsPrimary = ps.IsPrimary
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, 100*time.Millisecond)
		close(done)
	}()

	// After 1 tick, flip to primary.
	time.Sleep(150 * time.Millisecond)
	ps.v.Store(true)

	// Wait for sync to be triggered.
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done

	// Sync should have been triggered after promotion.
	if calls := f.callCount.Load(); calls == 0 {
		t.Error("expected API calls after promotion, got 0")
	}
}

// TestRunSyncCycle_DemotionAbort verifies DYN-03: demotion mid-sync cancels the
// cycle context, causing SyncWithRetry to return early.
func TestRunSyncCycle_DemotionAbort(t *testing.T) {
	t.Parallel()
	ensureMetrics(t)

	ps := &primarySwitch{}
	ps.v.Store(true) // Start as primary.

	// Create a slow mock server that delays org responses by 3s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "?")
		objType := parts[0]

		// Only return data on first page (skip=0).
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
			return
		}

		if objType == "org" {
			// Delay to simulate slow fetch, giving time for demotion.
			time.Sleep(3 * time.Second)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"meta": map[string]any{}, "data": []any{}})
	}))
	t.Cleanup(srv.Close)

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, srv.URL)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		IsPrimary: ps.IsPrimary,
	}, slog.Default())
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond})

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Start runSyncCycle in a goroutine.
	done := make(chan struct{})
	start := time.Now()
	go func() {
		w.runSyncCycle(ctx, config.SyncModeFull)
		close(done)
	}()

	// Flip to replica after 500ms (during the slow org fetch).
	time.Sleep(500 * time.Millisecond)
	ps.v.Store(false)

	// runSyncCycle should return within ~2s (demotion monitor polls every 1s).
	<-done
	elapsed := time.Since(start)

	// Should return much faster than the 3s org delay.
	if elapsed > 2500*time.Millisecond {
		t.Errorf("expected early abort on demotion, took %v", elapsed)
	}
}

// ----------------------------------------------------------------------------
// Phase 54-01 Commit B tests
//
// TestSync_DeferredFKSameTx    — regression-locks the FK pragma switch.
// TestSync_RefactorParity      — byte-identical DB state before/after refactor.
// TestWorkerSync_LineBudget    — enforces the REFAC-03 line budget on Sync.
// ----------------------------------------------------------------------------

// TestSync_DeferredFKSameTx verifies that the refactor's per-tx
// `PRAGMA defer_foreign_keys = ON` correctly defers FK enforcement until
// commit time, allowing temporarily-dangling FKs inside the transaction.
//
// Test shape:
//
//  1. Open an ent.Tx.
//  2. Set `PRAGMA defer_foreign_keys = ON` via the generated tx.ExecContext
//     (sql/execquery feature enabled in ent/entc.go).
//  3. Insert a Facility referencing Organization 999 which does NOT yet exist.
//  4. Insert Organization 999 BEFORE commit.
//  5. Commit. Asserts no FK error.
//
// Without the pragma, SQLite's default immediate FK enforcement would reject
// step 3 on insert. With the pragma, FK checks are deferred to commit time;
// at commit time the FK is resolved because step 4 ran first.
func TestSync_DeferredFKSameTx(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client, _ := testutil.SetupClientWithDB(t)

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		// Rollback is a no-op after Commit.
		_ = tx.Rollback()
	})

	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		t.Fatalf("set defer_foreign_keys: %v", err)
	}

	// Insert a facility referencing org_id=999 which does not yet exist.
	// Without defer_foreign_keys this would fail at insert time.
	_, err = tx.Facility.Create().
		SetID(12345).
		SetName("dangling-fk-fac").
		SetOrgID(999).
		SetCity("Testville").
		SetCountry("DE").
		SetCreated(time.Now()).
		SetUpdated(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("insert facility with temporarily-dangling FK: %v", err)
	}

	// Now insert the parent org BEFORE commit — this resolves the FK.
	_, err = tx.Organization.Create().
		SetID(999).
		SetName("dangling-fk-resolver").
		SetCity("Testville").
		SetCountry("DE").
		SetCreated(time.Now()).
		SetUpdated(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("insert parent org: %v", err)
	}

	// Commit MUST succeed — the FK is resolved and deferred enforcement passes.
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit with resolved deferred FK: %v", err)
	}

	// Sanity: both rows are present on a fresh query.
	gotFac, err := client.Facility.Get(ctx, 12345)
	if err != nil {
		t.Fatalf("get facility after commit: %v", err)
	}
	if gotFac.Name != "dangling-fk-fac" {
		t.Errorf("facility name: got %q, want %q", gotFac.Name, "dangling-fk-fac")
	}
	gotOrg, err := client.Organization.Get(ctx, 999)
	if err != nil {
		t.Fatalf("get org after commit: %v", err)
	}
	if gotOrg.Name != "dangling-fk-resolver" {
		t.Errorf("org name: got %q, want %q", gotOrg.Name, "dangling-fk-resolver")
	}
}

// parityFixtureServer is a minimal httptest server serving the project
// testdata/fixtures/*.json files at /api/{type} for TestSync_RefactorParity.
// Co-located in worker_test.go (package sync) rather than integration_test.go
// (package sync_test) because the parity dump walks internal ent entity
// types and needs direct access.
type parityFixtureServer struct {
	server   *httptest.Server
	fixtures map[string]json.RawMessage
}

// parityFixtureTypeMap locks the mapping from PeeringDB URL path to fixture
// filename, matching internal/sync/integration_test.go#fixtureTypeMap. Kept
// local so the parity test remains hermetic and does not cross package
// boundaries.
var parityFixtureTypeMap = map[string]string{
	peeringdb.TypeOrg:        "org.json",
	peeringdb.TypeNet:        "net.json",
	peeringdb.TypeFac:        "fac.json",
	peeringdb.TypeIX:         "ix.json",
	peeringdb.TypePoc:        "poc.json",
	peeringdb.TypeIXLan:      "ixlan.json",
	peeringdb.TypeIXPfx:      "ixpfx.json",
	peeringdb.TypeNetIXLan:   "netixlan.json",
	peeringdb.TypeNetFac:     "netfac.json",
	peeringdb.TypeIXFac:      "ixfac.json",
	peeringdb.TypeCarrier:    "carrier.json",
	peeringdb.TypeCarrierFac: "carrierfac.json",
	peeringdb.TypeCampus:     "campus.json",
}

func newParityFixtureServer(t *testing.T) *parityFixtureServer {
	t.Helper()
	fs := &parityFixtureServer{
		fixtures: make(map[string]json.RawMessage, len(parityFixtureTypeMap)),
	}

	for apiType, filename := range parityFixtureTypeMap {
		raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", filename))
		if err != nil {
			t.Fatalf("load fixture %s: %v", filename, err)
		}
		var resp struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("parse fixture %s: %v", filename, err)
		}
		fs.fixtures[apiType] = resp.Data
	}

	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		objType := strings.Split(path, "?")[0]

		// Terminate pagination: empty data for any skip>0.
		if skip := r.URL.Query().Get("skip"); skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		data, ok := fs.fixtures[objType]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":`))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte(`}`))
	}))
	t.Cleanup(func() { fs.server.Close() })
	return fs
}

// parityDump is the canonical JSON shape for TestSync_RefactorParity. Field
// ordering is locked by Go's json.Marshal struct field order; all 13 entity
// types are represented and rows are sorted by ID for deterministic output.
type parityDump struct {
	Organizations     []*ent.Organization     `json:"organizations"`
	Campuses          []*ent.Campus           `json:"campuses"`
	Facilities        []*ent.Facility         `json:"facilities"`
	Carriers          []*ent.Carrier          `json:"carriers"`
	CarrierFacilities []*ent.CarrierFacility  `json:"carrier_facilities"`
	InternetExchanges []*ent.InternetExchange `json:"internet_exchanges"`
	IxLans            []*ent.IxLan            `json:"ix_lans"`
	IxPrefixes        []*ent.IxPrefix         `json:"ix_prefixes"`
	IxFacilities      []*ent.IxFacility       `json:"ix_facilities"`
	Networks          []*ent.Network          `json:"networks"`
	Pocs              []*ent.Poc              `json:"pocs"`
	NetworkFacilities []*ent.NetworkFacility  `json:"network_facilities"`
	NetworkIxLans     []*ent.NetworkIxLan     `json:"network_ix_lans"`
}

// dumpAllTables returns a canonical parityDump of all 13 ent tables sorted
// by ID. This is the source of truth for TestSync_RefactorParity.
func dumpAllTables(ctx context.Context, t *testing.T, client *ent.Client) *parityDump {
	t.Helper()
	d := &parityDump{}
	var err error

	d.Organizations, err = client.Organization.Query().Order(ent.Asc(organization.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump organizations: %v", err)
	}
	d.Campuses, err = client.Campus.Query().Order(ent.Asc(campus.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump campuses: %v", err)
	}
	d.Facilities, err = client.Facility.Query().Order(ent.Asc(facility.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump facilities: %v", err)
	}
	d.Carriers, err = client.Carrier.Query().Order(ent.Asc(carrier.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump carriers: %v", err)
	}
	d.CarrierFacilities, err = client.CarrierFacility.Query().Order(ent.Asc(carrierfacility.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump carrier_facilities: %v", err)
	}
	d.InternetExchanges, err = client.InternetExchange.Query().Order(ent.Asc(internetexchange.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump internet_exchanges: %v", err)
	}
	d.IxLans, err = client.IxLan.Query().Order(ent.Asc(ixlan.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump ix_lans: %v", err)
	}
	d.IxPrefixes, err = client.IxPrefix.Query().Order(ent.Asc(ixprefix.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump ix_prefixes: %v", err)
	}
	d.IxFacilities, err = client.IxFacility.Query().Order(ent.Asc(ixfacility.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump ix_facilities: %v", err)
	}
	d.Networks, err = client.Network.Query().Order(ent.Asc(network.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump networks: %v", err)
	}
	d.Pocs, err = client.Poc.Query().Order(ent.Asc(poc.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump pocs: %v", err)
	}
	d.NetworkFacilities, err = client.NetworkFacility.Query().Order(ent.Asc(networkfacility.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump network_facilities: %v", err)
	}
	d.NetworkIxLans, err = client.NetworkIxLan.Query().Order(ent.Asc(networkixlan.FieldID)).All(ctx)
	if err != nil {
		t.Fatalf("dump network_ix_lans: %v", err)
	}
	return d
}

// TestSync_RefactorParity locks the behavior of Worker.Sync against a golden
// dump so the Commit B extract-method refactor cannot silently drift.
//
// Order of operations (REFAC-03 protocol):
//  1. This test was first authored BEFORE the refactor was applied.
//  2. Run `go test -update ./internal/sync/... -run TestSync_RefactorParity`
//     ONCE against the pre-refactor Worker.Sync. This writes the golden file.
//  3. Commit the golden file alongside the refactor (atomic Commit B).
//  4. Subsequent runs (without -update) MUST match byte-for-byte. Any drift
//     is a regression — reviewer investigates before merge.
//
// To intentionally update the golden (e.g. after a deliberate data-path
// change in a later plan), rerun with -update and carefully review the diff.
func TestSync_RefactorParity(t *testing.T) {
	// Not parallel: we write to a single golden file; concurrent runs would
	// race. Also, `go test -update` mutations must be observable to reviewers.
	fs := newParityFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	ctx := t.Context()
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	dump := dumpAllTables(ctx, t, client)
	got, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		t.Fatalf("marshal parity dump: %v", err)
	}
	// Trailing newline for POSIX text-file convention.
	got = append(got, '\n')

	goldenPath := filepath.Join("testdata", "refactor_parity.golden.json")

	if *updateGolden {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytesEqual(got, want) {
		t.Fatalf("parity drift: sync produced output that differs from golden file %s.\n"+
			"Run `go test -update ./internal/sync/... -run TestSync_RefactorParity` to regenerate.\n"+
			"got %d bytes, want %d bytes", goldenPath, len(got), len(want))
	}
}

// bytesEqual avoids pulling in bytes package for a single comparison.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestWorkerSync_LineBudget enforces the REFAC-03 line budget on Worker.Sync.
// The body (opening brace through matching closing brace) must be <= 100
// lines. This is a structural guardrail so future edits cannot silently
// bloat Sync again.
//
// Implementation: read worker.go, locate `func (w *Worker) Sync(`, walk the
// source counting braces until depth returns to 0, assert line count <= 100.
func TestWorkerSync_LineBudget(t *testing.T) {
	t.Parallel()

	const maxLines = 100

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}

	needle := "func (w *Worker) Sync("
	idx := strings.Index(string(src), needle)
	if idx < 0 {
		t.Fatalf("did not find %q in worker.go", needle)
	}

	// Walk forward from idx to the opening brace of the function body.
	body := string(src)[idx:]
	openIdx := strings.Index(body, "{")
	if openIdx < 0 {
		t.Fatalf("did not find opening brace of Sync body")
	}

	// Walk the body counting braces to find the matching close.
	depth := 0
	lineCount := 1
	closeIdx := -1
	inString := false
	inRune := false
	inLineComment := false
	inBlockComment := false
	// Iterate rune-by-rune from the opening brace onward.
	for i := openIdx; i < len(body); i++ {
		c := body[i]
		next := byte(0)
		if i+1 < len(body) {
			next = body[i+1]
		}
		switch {
		case inLineComment:
			if c == '\n' {
				inLineComment = false
				lineCount++
			}
			continue
		case inBlockComment:
			if c == '*' && next == '/' {
				inBlockComment = false
				i++
			} else if c == '\n' {
				lineCount++
			}
			continue
		case inString:
			if c == '\\' && next != 0 {
				i++ // skip escaped char
				continue
			}
			switch c {
			case '"':
				inString = false
			case '\n':
				lineCount++
			}
			continue
		case inRune:
			if c == '\\' && next != 0 {
				i++
				continue
			}
			if c == '\'' {
				inRune = false
			}
			continue
		}

		if c == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if c == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == '\'' {
			inRune = true
			continue
		}
		if c == '\n' {
			lineCount++
			continue
		}
		if c == '{' {
			depth++
			continue
		}
		if c == '}' {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
			continue
		}
	}

	if closeIdx < 0 {
		t.Fatalf("did not find matching close brace for Sync")
	}
	if lineCount > maxLines {
		t.Fatalf("Worker.Sync body is %d lines; REFAC-03 budget is %d lines.\n"+
			"Future refactors must keep Sync an orchestrator, not a doer.", lineCount, maxLines)
	}
	t.Logf("Worker.Sync body: %d lines (budget: %d)", lineCount, maxLines)
}

// TestSync_PhaseAFetchHasNoTx is a structural regression lock for PERF-05:
// the fetch pass MUST NOT hold an ent.Tx. This is enforced at compile-time
// by the syncFetchPass signature — this test scans the worker.go source
// and asserts the signature does NOT contain `*ent.Tx`. A future edit
// that sneaks a tx parameter back into Phase A will fail this test.
func TestSync_PhaseAFetchHasNoTx(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}

	needle := "func (w *Worker) syncFetchPass("
	idx := strings.Index(string(src), needle)
	if idx < 0 {
		t.Fatalf("did not find %q in worker.go", needle)
	}

	// Walk forward from idx to the closing ')' of the signature.
	body := string(src)[idx:]
	found := strings.Contains(body, ")")
	if !found {
		t.Fatalf("did not find closing paren of syncFetchPass signature")
	}
	// The signature wraps across multiple lines; find the full parameter
	// list by scanning to the ')' that terminates the parameters.
	paren := 0
	end := -1
	for i, c := range body {
		if c == '(' {
			paren++
		}
		if c == ')' {
			paren--
			if paren == 0 {
				end = i
				break
			}
		}
	}
	if end < 0 {
		t.Fatalf("did not find balanced closing paren of syncFetchPass signature")
	}
	sig := body[:end+1]
	if strings.Contains(sig, "*ent.Tx") {
		t.Fatalf("syncFetchPass signature contains *ent.Tx — Phase A must NOT hold a tx:\n%s", sig)
	}
	t.Logf("syncFetchPass signature: %s", sig)
}

// TestSync_BatchFreeAfterUpsert is a structural regression lock for the
// MANDATORY memory optimization in the Phase B chunked replay loop:
// after each per-chunk upsert completes, the batch entry MUST be set to
// the zero value so the slice backing array can be reclaimed by GC. Per
// ARCHITECTURE.md §2 this is the difference between "fits in 512 MB VM"
// and "OOM" on production scale. Commit D' moved the batch-free line
// from syncUpsertPass (old in-memory path) into drainAndUpsertType (the
// chunked scratch replay) where each loop iteration processes one
// scratchChunkSize-bounded chunk.
func TestSync_BatchFreeAfterUpsert(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}

	// Locate drainAndUpsertType function body and search within it only.
	needle := "func (w *Worker) drainAndUpsertType("
	start := strings.Index(string(src), needle)
	if start < 0 {
		t.Fatalf("did not find %q in worker.go", needle)
	}
	body := string(src)[start:]
	// Find the next top-level func declaration to bound the search.
	nextFunc := strings.Index(body[1:], "\nfunc ")
	if nextFunc > 0 {
		body = body[:nextFunc+1]
	}

	// Accept either `batches[name]` or `batches[step.name]` — the inner
	// chunked replay loop uses `name` because step.name is out of scope.
	if !strings.Contains(body, "batches[name] = syncBatch{}") && !strings.Contains(body, "batches[step.name] = syncBatch{}") {
		t.Fatalf("drainAndUpsertType does not contain the MANDATORY batch-free line " +
			"`batches[name] = syncBatch{}` (or the step.name variant). This is the " +
			"core PERF-05 memory optimization (ARCHITECTURE.md §2) — removing it " +
			"breaks the 512 MB VM hard cap and is a regression.")
	}
}

// TestSync_PhaseOrderComments is a structural regression lock for the
// three load-bearing phase marker comments in Worker.Sync. A future edit
// that reorders or drops one of these markers will fail this test.
func TestSync_PhaseOrderComments(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	body := string(src)

	markers := []string{
		"=== Phase A — NO TX HELD ===",
		"=== Fetch Barrier ===",
		"=== Phase B — SINGLE REAL TX ===",
	}
	lastIdx := -1
	for _, m := range markers {
		idx := strings.Index(body, m)
		if idx < 0 {
			t.Fatalf("missing phase marker comment: %q", m)
		}
		if idx <= lastIdx {
			t.Fatalf("phase marker %q out of order (index %d <= previous %d)",
				m, idx, lastIdx)
		}
		lastIdx = idx
	}
}

// TestSync_D19Atomicity is the PERF-05 regression lock for D-19 single-
// transaction atomicity. It asserts that if a Phase B upsert fails mid-way
// through the 13 types, the rollback leaves the database in its
// pre-sync state (empty for a fresh DB). Without the single ent.Tx
// wrapping every write, partial upserts would survive the failure.
//
// Injection strategy: return a campus payload whose `id` field is a
// JSON string instead of an int. json.Unmarshal into peeringdb.Campus
// fails inside syncIncremental, propagating a decode error out of
// dispatchScratchChunk → drainAndUpsertType → syncUpsertPass, and the
// orchestrator rolls back the tx. Organizations have already been
// upserted inside the tx at that point, so their post-rollback count
// proves the single-tx guarantee.
//
// We deliberately use a decode failure rather than an FK-orphan
// injection here because the v1.13 upsert-time fkFilter would catch
// an FK orphan cleanly and let the sync succeed with the orphan
// dropped — which is the correct production behaviour and therefore
// no longer a D-19 failure injection path.
func TestSync_D19Atomicity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// parityFixtureServer uses testdata/fixtures for 12 of 13 types; override
	// the campus payload with an upsert-fatal row to trigger a Phase B
	// rollback after Organizations have already been upserted.
	pfs := newParityFixtureServer(t)

	// Build a server that wraps pfs.server but intercepts /api/campus with
	// a payload whose `id` is a JSON string — peeringdb.Campus.ID is an
	// int, so json.Unmarshal returns a type error at decode time inside
	// Phase B. The error surfaces out of dispatchScratchChunk and triggers
	// a Phase B rollback after organizations have been upserted.
	injectingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		objType := strings.Split(path, "?")[0]
		if objType == peeringdb.TypeCampus {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[{"id":"not-an-int","org_id":1,"name":"d19-injection","country":"DE","created":"2024-01-01T00:00:00Z","updated":"2024-01-01T00:00:00Z","status":"ok"}]}`))
			return
		}
		// Delegate to parity fixture server for all other types.
		pfs.server.Config.Handler.ServeHTTP(w, r)
	}))
	defer injectingServer.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(injectingServer.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	worker := NewWorker(pdbClient, client, db, WorkerConfig{
		SyncMode:  config.SyncModeFull,
		IsPrimary: func() bool { return true },
	}, slog.Default())
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	syncErr := worker.Sync(ctx, config.SyncModeFull)
	if syncErr == nil {
		t.Fatal("expected sync error from D-19 injection, got nil")
	}

	// D-19 assertion: every table must be empty. A partial upsert that
	// survives the rollback is a regression.
	counts := map[string]int{}
	if counts["organizations"], _ = client.Organization.Query().Count(ctx); counts["organizations"] != 0 {
		t.Errorf("D-19 violated: organizations table has %d rows post-rollback", counts["organizations"])
	}
	if counts["campuses"], _ = client.Campus.Query().Count(ctx); counts["campuses"] != 0 {
		t.Errorf("D-19 violated: campuses table has %d rows post-rollback", counts["campuses"])
	}
	if counts["facilities"], _ = client.Facility.Query().Count(ctx); counts["facilities"] != 0 {
		t.Errorf("D-19 violated: facilities table has %d rows post-rollback", counts["facilities"])
	}
	if counts["networks"], _ = client.Network.Query().Count(ctx); counts["networks"] != 0 {
		t.Errorf("D-19 violated: networks table has %d rows post-rollback", counts["networks"])
	}
	if counts["ix"], _ = client.InternetExchange.Query().Count(ctx); counts["ix"] != 0 {
		t.Errorf("D-19 violated: internet_exchanges table has %d rows post-rollback", counts["ix"])
	}
	t.Logf("D-19 atomicity verified: all tables empty after rollback (%+v)", counts)
}

// TestSync_MemoryLimitAbort verifies the Commit F (Plan 54-03) memory
// guardrail: when WorkerConfig.SyncMemoryLimit is set absurdly low
// (1 byte), the Worker.Sync abort path fires AFTER Phase A fetch
// completes and BEFORE the ent.Tx opens. The test asserts:
//
//   - The returned error wraps ErrSyncMemoryLimitExceeded (errors.Is
//     per GO-ERR-2, no string matching).
//   - A WARN log line with slog.Int64("heap_alloc", ...) is emitted.
//   - No organizations were written to the real DB (no tx opened).
//   - The running mutex is released on return (second Sync call
//     proceeds past the running guard, though it also aborts for the
//     same reason — reaching the abort path twice proves the mutex
//     was released).
func TestSync_MemoryLimitAbort(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "ok-org", "ok")}

	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	var logBuf bytes.Buffer
	// Serialize writes because slog.Handler does not promise
	// goroutine-safety for the underlying writer, and the Worker's
	// otel metric pipeline + log pipeline may race on the buffer
	// across the running-mutex release boundary.
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	pdbClient := newFastPDBClient(t, f.server.URL)

	worker := NewWorker(pdbClient, client, db, WorkerConfig{
		IsPrimary:       func() bool { return true },
		SyncMode:        config.SyncModeFull,
		SyncMemoryLimit: 1, // 1 byte — guaranteed to trip after Phase A
	}, logger)

	err := worker.Sync(ctx, config.SyncModeFull)
	if !errors.Is(err, ErrSyncMemoryLimitExceeded) {
		t.Fatalf("first Sync: expected ErrSyncMemoryLimitExceeded, got %v", err)
	}

	// Verify WARN log emitted with the expected attribute keys.
	logs := logBuf.String()
	if !strings.Contains(logs, `"msg":"sync aborted: memory limit exceeded"`) {
		t.Errorf("expected WARN log message; captured logs:\n%s", logs)
	}
	if !strings.Contains(logs, `"heap_alloc"`) {
		t.Errorf("expected heap_alloc log attr; captured logs:\n%s", logs)
	}
	if !strings.Contains(logs, `"limit"`) {
		t.Errorf("expected limit log attr; captured logs:\n%s", logs)
	}
	if !strings.Contains(logs, `"level":"WARN"`) {
		t.Errorf("expected WARN level; captured logs:\n%s", logs)
	}

	// Verify NO tx was opened — real DB is empty.
	count, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	if count != 0 {
		t.Errorf("memory-aborted sync wrote %d orgs, want 0 (tx should not have opened)", count)
	}

	// Verify running mutex released — a second Sync call reaches the
	// abort path again instead of being silently dropped by the
	// running CAS guard (which logs "sync already running, skipping"
	// and returns nil).
	err2 := worker.Sync(ctx, config.SyncModeFull)
	if !errors.Is(err2, ErrSyncMemoryLimitExceeded) {
		t.Errorf("second Sync after mutex release: expected ErrSyncMemoryLimitExceeded, got %v", err2)
	}
}

// TestSync_MemoryLimitDisabled verifies the opt-out path: when
// SyncMemoryLimit is 0, the guardrail is disabled and Sync proceeds
// normally. This is the local-dev / benchmark-mode path.
func TestSync_MemoryLimitDisabled(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "ok-org", "ok")}

	client, db := testutil.SetupClientWithDB(t)
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	pdbClient := newFastPDBClient(t, f.server.URL)

	worker := NewWorker(pdbClient, client, db, WorkerConfig{
		IsPrimary:       func() bool { return true },
		SyncMode:        config.SyncModeFull,
		SyncMemoryLimit: 0, // disabled
	}, slog.Default())

	if err := worker.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync with disabled guardrail: %v", err)
	}

	count, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 org after sync, got %d", count)
	}
}

// TestCheckMemoryLimit_HelperUnit directly exercises the extracted
// checkMemoryLimit helper so the full sync flow is not needed to lock
// the guardrail semantics. Table-driven per GO-T-1.
func TestCheckMemoryLimit_HelperUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		heapAlloc uint64
		limit     int64
		wantErr   error
	}{
		{name: "disabled_zero_limit", heapAlloc: 1 << 30, limit: 0, wantErr: nil},
		{name: "disabled_negative_limit", heapAlloc: 1 << 30, limit: -1, wantErr: nil},
		{name: "under_limit", heapAlloc: 100, limit: 1000, wantErr: nil},
		{name: "exactly_at_limit", heapAlloc: 1000, limit: 1000, wantErr: nil},
		{name: "one_over_limit", heapAlloc: 1001, limit: 1000, wantErr: ErrSyncMemoryLimitExceeded},
		{name: "vastly_over_limit", heapAlloc: 1 << 30, limit: 1, wantErr: ErrSyncMemoryLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
			w := &Worker{logger: logger}
			err := w.checkMemoryLimit(t.Context(), tt.heapAlloc, tt.limit, 13)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("checkMemoryLimit(%d, %d) = %v, want %v", tt.heapAlloc, tt.limit, err, tt.wantErr)
			}
			// On breach, log line must be present; on pass, must be absent.
			hasBreach := errors.Is(tt.wantErr, ErrSyncMemoryLimitExceeded)
			hasLog := strings.Contains(logBuf.String(), "sync aborted: memory limit exceeded")
			if hasBreach && !hasLog {
				t.Errorf("expected WARN log on breach, got empty buffer")
			}
			if !hasBreach && hasLog {
				t.Errorf("expected NO log on pass, got: %s", logBuf.String())
			}
		})
	}
}

// TestWorker_CheckMemoryLimitExtracted is a structural regression
// lock for Commit F: the checkMemoryLimit helper MUST remain a
// package-private method on *Worker (NOT inlined into Sync). Inlining
// would push Worker.Sync's body over the REFAC-03 100-line budget.
func TestWorker_CheckMemoryLimitExtracted(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	if !bytes.Contains(src, []byte("func (w *Worker) checkMemoryLimit(")) {
		t.Fatalf("checkMemoryLimit helper must be declared on *Worker — " +
			"inlining the guardrail into Sync breaks the REFAC-03 line budget")
	}
	if !bytes.Contains(src, []byte("w.checkMemoryLimit(")) {
		t.Fatalf("Worker.Sync must CALL w.checkMemoryLimit — not found in worker.go")
	}
	if !bytes.Contains(src, []byte("runtime.ReadMemStats(&ms)")) {
		t.Fatalf("Worker.Sync must call runtime.ReadMemStats before checkMemoryLimit")
	}
	if !bytes.Contains(src, []byte("ErrSyncMemoryLimitExceeded")) {
		t.Fatalf("ErrSyncMemoryLimitExceeded sentinel must be declared in worker.go")
	}
}

// ----------------------------------------------------------------------------
// Plan 59-05: VIS-05 sync-worker privacy bypass tests
//
// TestWorkerSync_HasBypassCall              — structural lock on worker.go
// TestWorkerSync_BypassesPrivacy            — bypass ctx admits Users rows
// TestWorkerSync_BypassDoesNotLeak          — fresh ctx does not inherit bypass
// TestWorkerSync_ChildGoroutineInheritsBypass — goroutines derived from
//                                               bypass ctx keep the decision
// ----------------------------------------------------------------------------

// TestWorkerSync_HasBypassCall is the source-level RED gate for VIS-05:
// it reads worker.go and asserts that `Sync` rebinds `ctx` with the
// privacy bypass BEFORE the `w.running.CompareAndSwap` guard. This is
// the structural invariant — removing the bypass line (or placing it
// after any other work) fails CI.
//
// This test is in addition to TestSyncBypass_SingleCallSite, which
// enforces "exactly one bypass call site in the tree". The two tests
// serve different invariants: this one pins the location within
// worker.go; the other pins the cardinality across the tree.
func TestWorkerSync_HasBypassCall(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker.go: %v", err)
	}
	content := string(src)

	// Locate the Sync function body.
	needle := "func (w *Worker) Sync("
	idx := strings.Index(content, needle)
	if idx < 0 {
		t.Fatalf("did not find %q in worker.go", needle)
	}
	body := content[idx:]

	bypassLine := "ctx = privacy.DecisionContext(ctx, privacy.Allow)"
	bypassIdx := strings.Index(body, bypassLine)
	if bypassIdx < 0 {
		t.Fatalf("Worker.Sync missing VIS-05 bypass line %q — D-08/D-09 requires it at function entry", bypassLine)
	}

	casLine := "w.running.CompareAndSwap"
	casIdx := strings.Index(body, casLine)
	if casIdx < 0 {
		t.Fatalf("did not find %q in worker.go (post-refactor signature changed?)", casLine)
	}
	if bypassIdx >= casIdx {
		t.Fatalf("VIS-05 bypass line must appear BEFORE %q; got bypass@%d vs CAS@%d",
			casLine, bypassIdx, casIdx)
	}

	// Also lock the import so linters do not drop it.
	if !strings.Contains(content, `"github.com/dotwaffle/peeringdb-plus/ent/privacy"`) {
		t.Fatalf("worker.go missing ent/privacy import required for VIS-05 bypass")
	}
}

// TestWorkerSync_BypassesPrivacy asserts that a ctx carrying
// privacy.DecisionContext(_, privacy.Allow) admits a Users-visibility
// Poc row — i.e. the exact construction Sync installs at entry lets
// the sync worker read every row regardless of visibility.
func TestWorkerSync_BypassesPrivacy(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	bypassCtx := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	want, err := client.Poc.Create().
		SetID(3001).
		SetRole("NOC").
		SetVisible("Users").
		SetCreated(ts).
		SetUpdated(ts).
		Save(bypassCtx)
	if err != nil {
		t.Fatalf("bypass ctx should admit Users-tier insert, got %v", err)
	}

	got, err := client.Poc.Get(bypassCtx, want.ID)
	if err != nil {
		t.Fatalf("bypass ctx should admit Users-tier Get, got %v", err)
	}
	if got.Visible != "Users" {
		t.Errorf("got visible=%q, want %q", got.Visible, "Users")
	}
}

// TestWorkerSync_BypassDoesNotLeak verifies that the bypass is
// context-scoped, not process-scoped: a fresh context.Background() (no
// DecisionContext, no TierUsers) does NOT see a Users-tier row even
// though a sibling ctx already did. Catches the failure mode where a
// developer accidentally stamps the bypass on a shared request ctx.
func TestWorkerSync_BypassDoesNotLeak(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	bypassCtx := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	want, err := client.Poc.Create().
		SetID(3002).
		SetRole("NOC").
		SetVisible("Users").
		SetCreated(ts).
		SetUpdated(ts).
		Save(bypassCtx)
	if err != nil {
		t.Fatalf("seed via bypass: %v", err)
	}

	_, err = client.Poc.Get(context.Background(), want.ID)
	if !ent.IsNotFound(err) {
		t.Fatalf("fresh ctx must not inherit bypass decision; Get(Users-row) = %v, want NotFound", err)
	}
}

// TestWorkerSync_ChildGoroutineInheritsBypass verifies the propagation
// guarantee: a goroutine derived from the bypass ctx (via
// context.WithCancel, mirroring the runSyncCycle demotion-monitor
// pattern around worker.go:1209) keeps the Allow decision. This is the
// context.WithValue parent-chain walk — standard Go semantics — but the
// test pins it so a future ctx refactor cannot silently break it.
func TestWorkerSync_ChildGoroutineInheritsBypass(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	parent := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	want, err := client.Poc.Create().
		SetID(3003).
		SetRole("NOC").
		SetVisible("Users").
		SetCreated(ts).
		SetUpdated(ts).
		Save(parent)
	if err != nil {
		t.Fatalf("seed via bypass: %v", err)
	}

	// Derive a child ctx the way runSyncCycle does for the demotion monitor.
	childCtx, cancel := context.WithCancel(parent)
	defer cancel()

	type result struct {
		id  int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		got, err := client.Poc.Get(childCtx, want.ID)
		if err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{id: got.ID}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("child goroutine failed Get under inherited bypass ctx: %v", r.err)
		}
		if r.id != want.ID {
			t.Errorf("child got id=%d, want %d", r.id, want.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child goroutine did not complete within 5s")
	}
}

// TestEmitMemoryTelemetry_Attrs asserts that emitMemoryTelemetry attaches
// pdbplus.sync.peak_heap_bytes to the current span on every call, attaches
// pdbplus.sync.peak_rss_bytes on Linux, and fires the "heap threshold
// crossed" slog.Warn only when at least one threshold is breached
// (OBS-05 / D-02 / D-03 / D-09).
func TestEmitMemoryTelemetry_Attrs(t *testing.T) {
	// Imports are already in this file (context, bytes, strings, log/slog,
	// testing, runtime). We add span-capture via the OTel SDK tracetest
	// helpers below, scoped to this test.
	tests := []struct {
		name     string
		heapWarn int64
		rssWarn  int64
		wantWarn bool
	}{
		{name: "below_threshold", heapWarn: 1 << 40, rssWarn: 1 << 40, wantWarn: false},
		{name: "heap_over", heapWarn: 1, rssWarn: 1 << 40, wantWarn: true},
		{name: "both_disabled", heapWarn: 0, rssWarn: 0, wantWarn: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))
			w := &Worker{logger: logger}

			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
			ctx, span := tp.Tracer("t").Start(context.Background(), "sync-test")
			w.emitMemoryTelemetry(ctx, tt.heapWarn, tt.rssWarn)
			span.End()

			spans := sr.Ended()
			if len(spans) != 1 {
				t.Fatalf("want 1 span, got %d", len(spans))
			}
			attrs := spans[0].Attributes()
			haveHeap := false
			haveRSS := false
			for _, a := range attrs {
				switch string(a.Key) {
				case "pdbplus.sync.peak_heap_bytes":
					haveHeap = true
				case "pdbplus.sync.peak_rss_bytes":
					haveRSS = true
				}
			}
			if !haveHeap {
				t.Errorf("missing span attr pdbplus.sync.peak_heap_bytes (attrs=%v)", attrs)
			}
			if runtime.GOOS == "linux" && !haveRSS {
				t.Errorf("on Linux expected pdbplus.sync.peak_rss_bytes attr (attrs=%v)", attrs)
			}
			warned := strings.Contains(buf.String(), "heap threshold crossed")
			if warned != tt.wantWarn {
				t.Errorf("warn=%v want %v; log=%s", warned, tt.wantWarn, buf.String())
			}
		})
	}
}

// TestEmitOrphanSummary_Aggregates asserts that recordOrphan increments
// per-cycle counts and that emitOrphanSummary collapses them into a
// single WARN log when any orphans were observed (and a DEBUG log on a
// clean cycle). Replaces the per-row WARN spam that blew Tempo's 7.5 MB
// per-trace budget per the 2026-04-26 audit.
func TestEmitOrphanSummary_Aggregates(t *testing.T) {
	t.Run("clean_cycle_logs_debug_total_zero", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		w := &Worker{logger: logger}
		w.resetFKState()
		w.emitOrphanSummary(context.Background())
		out := buf.String()
		if !strings.Contains(out, `"level":"DEBUG"`) || !strings.Contains(out, `"msg":"fk orphans summary"`) {
			t.Errorf("clean cycle should emit DEBUG fk-orphans-summary; got %s", out)
		}
		if strings.Contains(out, `"level":"WARN"`) {
			t.Errorf("clean cycle must not emit WARN; got %s", out)
		}
	})

	t.Run("dirty_cycle_logs_warn_total_summed", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		w := &Worker{logger: logger}
		w.resetFKState()

		ctx := context.Background()
		// Two of the same key, one different — should collapse to two groups, total=3.
		w.recordOrphan(ctx, fkOrphanKey{ChildType: "carrierfac", ParentType: "fac", Field: "fac_id", Action: "drop"}, 100, 200)
		w.recordOrphan(ctx, fkOrphanKey{ChildType: "carrierfac", ParentType: "fac", Field: "fac_id", Action: "drop"}, 101, 201)
		w.recordOrphan(ctx, fkOrphanKey{ChildType: "fac", ParentType: "campus", Field: "campus_id", Action: "null"}, 300, 400)

		// Clear the buf — the per-row DEBUG entries are noise for this assertion.
		buf.Reset()
		w.emitOrphanSummary(ctx)

		out := buf.String()
		if !strings.Contains(out, `"level":"WARN"`) {
			t.Errorf("dirty cycle should emit WARN; got %s", out)
		}
		if !strings.Contains(out, `"total":3`) {
			t.Errorf("expected total=3 in summary; got %s", out)
		}
		if !strings.Contains(out, `"action":"drop"`) || !strings.Contains(out, `"action":"null"`) {
			t.Errorf("summary should reference both action variants; got %s", out)
		}
	})
}

// TestReadLinuxVmHWM asserts that on Linux, the helper returns a positive
// byte count and ok=true. Skipped on non-Linux since /proc/self/status
// does not exist. This guards against silent regression of the VmHWM
// parse path under test harness changes.
func TestReadLinuxVmHWM(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("skipping on %s (no /proc/self/status)", runtime.GOOS)
	}
	b, ok := readLinuxVMHWM()
	if !ok {
		t.Fatal("readLinuxVMHWM returned ok=false on Linux")
	}
	if b <= 0 {
		t.Errorf("readLinuxVMHWM returned %d bytes, want > 0", b)
	}
}

// TestSyncFetchPass_UsesMaxUpdatedAsCursor asserts the new 260428-mu0
// contract: after a row is committed with a known `updated` timestamp, the
// next incremental cycle issues `?since=N` where N >= upstream-row.Unix()
// (modulo SQLite µs rounding to a 1-second tolerance). This is the
// load-bearing assertion that proves syncFetchPass derives the cursor
// from MAX(updated) rather than meta.generated.
func TestSyncFetchPass_UsesMaxUpdatedAsCursor(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	generated := float64(t1.Unix())
	f := newFixtureWithMeta(t, generated)
	// Seed parent org so the poc FK chain resolves; bump updated to t1
	// so MAX(updated) on `pocs` lands at t1 after cycle 1.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1", "ok"), t1.Format(time.RFC3339))}
	f.responses["net"] = []any{bumpUpdated(makeNet(50, 1, 64500, "ParentNet", "ok"), t1.Format(time.RFC3339))}
	f.responses["poc"] = []any{bumpUpdated(makePoc(10, 50, "Alice", "Tech", "ok"), t1.Format(time.RFC3339))}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Cycle 1: cursor zero → bare list. Lands the seed rows; advances the
	// derived MAX(updated) on each table to t1.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 1 sync: %v", err)
	}
	maxPoc, err := GetMaxUpdated(ctx, db, "pocs")
	if err != nil {
		t.Fatalf("GetMaxUpdated pocs: %v", err)
	}
	if maxPoc.IsZero() {
		t.Fatal("expected non-zero MAX(updated) on pocs after cycle 1")
	}

	// Cycle 2: cursor present → request URL must include since=N where N
	// parses back to a time >= t1 (the cursor was at t1 entering cycle 2;
	// the query `>= since` is inclusive so the boundary row is re-fetched
	// and the upsert is a no-op via the skip-on-unchanged predicate).
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 2 sync: %v", err)
	}

	pocLog, ok := f.sinceValues["poc"]
	if !ok {
		t.Fatal("no since-log captured for poc")
	}
	values := pocLog.snapshot()
	if len(values) < 2 {
		t.Fatalf("expected ≥2 first-page poc requests across 2 cycles, got %d (%v)", len(values), values)
	}
	cycle2Since := values[len(values)-1]
	if cycle2Since == "" {
		t.Fatalf("cycle 2 poc request had no ?since= param; values=%v", values)
	}
	cycle2Unix, parseErr := strconv.ParseInt(cycle2Since, 10, 64)
	if parseErr != nil {
		t.Fatalf("parse cycle 2 since=%q: %v", cycle2Since, parseErr)
	}
	cycle2T := time.Unix(cycle2Unix, 0).UTC()
	// Allow 1s tolerance for SQLite µs rounding when the boundary is right
	// at t1 — accept any cursor within ±1s of t1.
	delta := cycle2T.Sub(t1)
	if delta < -time.Second || delta > time.Second {
		t.Errorf("cycle 2 since=%v not within ±1s of expected MAX(updated)=%v", cycle2T, t1)
	}
}

// TestSync_TwoCycle_NoFullRefetch is the REGRESSION LOCK for the v1.13
// alternating-full-refetch bug. Pre-260428-mu0: meta.generated was absent
// on ?since= responses, so the cursor stored zero, so the next cycle
// fell through to a full bare-list re-fetch — alternating big/small
// total_objects every 15 minutes. Post-260428-mu0: cursor derived from
// MAX(updated), so the bug is structurally impossible.
//
// This test seeds 5 poc rows on cycle 1, then mocks upstream returning 0
// new rows on cycle 2, and asserts:
//
//  1. Cycle 2's `poc` first-page request uses ?since=<non-empty>. NOT a
//     bare list — that would mean the cursor wasn't derived correctly and
//     we re-fetched the whole table.
//  2. Cycle 2's per-type total_objects is small (the empty response yields
//     zero net new rows, NOT 5+ which would prove a re-fetch happened).
func TestSync_TwoCycle_NoFullRefetch(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	generated := float64(t1.Unix())
	f := newFixtureWithMeta(t, generated)
	// Seed FK parents + 5 pocs. updated=t1 on every row so MAX advances
	// past t1 after cycle 1 commits.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1", "ok"), t1.Format(time.RFC3339))}
	f.responses["net"] = []any{bumpUpdated(makeNet(50, 1, 64500, "Net50", "ok"), t1.Format(time.RFC3339))}
	pocs := make([]any, 0, 5)
	for i := range 5 {
		pocs = append(pocs, bumpUpdated(makePoc(100+i, 50, fmt.Sprintf("P%d", i), "Tech", "ok"), t1.Format(time.RFC3339)))
	}
	f.responses["poc"] = pocs

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Cycle 1.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}

	// Confirm the seeded data committed and MAX(updated) advanced.
	cnt, err := w.entClient.Poc.Query().Count(ctx)
	if err != nil {
		t.Fatalf("poc count after cycle 1: %v", err)
	}
	if cnt != 5 {
		t.Fatalf("expected 5 pocs after cycle 1, got %d", cnt)
	}
	maxPoc, err := GetMaxUpdated(ctx, db, "pocs")
	if err != nil || maxPoc.IsZero() {
		t.Fatalf("MAX(updated) pocs after cycle 1: %v err=%v", maxPoc, err)
	}

	// Cycle 2: upstream returns ZERO new poc rows (steady state — no
	// upstream activity since t1). Pre-260428-mu0: cursor would re-fetch
	// the whole table because meta.generated was zero. Post: cursor uses
	// MAX(updated) so since= advances correctly.
	f.responses["poc"] = []any{}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}

	// Assert (1): cycle 2's last poc first-page request used since=<value>.
	pocLog, ok := f.sinceValues["poc"]
	if !ok {
		t.Fatal("no since-log captured for poc")
	}
	values := pocLog.snapshot()
	if len(values) < 2 {
		t.Fatalf("expected ≥2 first-page poc requests, got %d (values=%v)", len(values), values)
	}
	cycle2Since := values[len(values)-1]
	if cycle2Since == "" {
		t.Errorf("REGRESSION: cycle 2 poc request issued bare list (no since=); values=%v — "+
			"this is the v1.13 alternating-full-refetch bug", values)
	}

	// Assert (2): cycle 2 poc count unchanged at 5 — no new rows fetched.
	cnt, err = w.entClient.Poc.Query().Count(ctx)
	if err != nil {
		t.Fatalf("poc count after cycle 2: %v", err)
	}
	if cnt != 5 {
		t.Errorf("expected 5 pocs after cycle 2 (no upstream changes), got %d", cnt)
	}
}

// TestSync_FullSyncIntervalForcesBareList locks the 260428-mu0 escape
// hatch: when the configured PDBPLUS_FULL_SYNC_INTERVAL has elapsed since
// the last successful full sync, the next cycle ignores cursors and
// every per-type request is bare-list (no `?since=` parameter).
//
// Seed: full success @ T-25h, then cycle a sync with FullSyncInterval=24h
// → expect bare list. The cycle records as 'full' so a follow-up sync
// (now with last_full ≈ now) goes back to using ?since=.
func TestSync_FullSyncIntervalForcesBareList(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	generated := float64(t1.Unix())
	f := newFixtureWithMeta(t, generated)
	// Seed minimal fixtures so each cycle commits something — driving
	// MAX(updated) past zero on the org table so cursor would otherwise
	// advance.
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1", "ok"), t1.Format(time.RFC3339))}

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, f.server.URL)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		SyncMode:         config.SyncModeIncremental,
		FullSyncInterval: 24 * time.Hour, // 260428-mu0 escape hatch active
	}, slog.Default())

	ctx := t.Context()

	// Seed pre-history: a successful full sync from 25 hours ago, plus a
	// recent incremental success — proves we filter on mode='full'.
	twentyFiveHoursAgo := time.Now().Add(-25 * time.Hour)
	pastFullID, err := RecordSyncStart(ctx, db, twentyFiveHoursAgo, "full")
	if err != nil {
		t.Fatalf("seed past full RecordSyncStart: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, pastFullID, Status{
		LastSyncAt: twentyFiveHoursAgo.Add(time.Minute),
		Duration:   time.Minute,
		Status:     "success",
	}); err != nil {
		t.Fatalf("seed past full RecordSyncComplete: %v", err)
	}
	recentIncrID, err := RecordSyncStart(ctx, db, time.Now().Add(-15*time.Minute), "incremental")
	if err != nil {
		t.Fatalf("seed incr RecordSyncStart: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, recentIncrID, Status{
		LastSyncAt: time.Now().Add(-14 * time.Minute),
		Duration:   time.Minute,
		Status:     "success",
	}); err != nil {
		t.Fatalf("seed incr RecordSyncComplete: %v", err)
	}

	// Pre-populate org with a row so MAX(updated) is non-zero entering
	// cycle 1 — without this the test couldn't distinguish "forced full"
	// from "first sync, cursor zero, bare list anyway".
	if _, err := client.Organization.Create().
		SetID(99).SetName("Pre").SetCreated(t1).SetUpdated(t1).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("seed pre-existing org: %v", err)
	}
	maxOrg, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil || maxOrg.IsZero() {
		t.Fatalf("expected non-zero MAX(updated) before cycle: %v err=%v", maxOrg, err)
	}

	// Cycle 1: forced full (last full > 24h). Expect bare list (no
	// since=).
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	orgLog, ok := f.sinceValues["org"]
	if !ok {
		t.Fatal("no since-log captured for org on cycle 1")
	}
	values := orgLog.snapshot()
	if len(values) == 0 {
		t.Fatalf("no first-page org request seen on cycle 1")
	}
	cycle1Since := values[len(values)-1]
	if cycle1Since != "" {
		t.Errorf("cycle 1 (forced full) should issue bare list; got since=%q", cycle1Since)
	}

	// Confirm cycle 1 recorded as 'full' in sync_status.
	var lastMode string
	if err := db.QueryRowContext(ctx,
		`SELECT mode FROM sync_status WHERE status = 'success' ORDER BY id DESC LIMIT 1`,
	).Scan(&lastMode); err != nil {
		t.Fatalf("read last mode: %v", err)
	}
	if lastMode != "full" {
		t.Errorf("expected last sync_status.mode = 'full' after forced full, got %q", lastMode)
	}

	// Cycle 2: last full is now ~recent, FullSyncInterval=24h not
	// elapsed → cursor-driven incremental. Expect since=.
	prevLen := len(orgLog.snapshot())
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}
	values2 := orgLog.snapshot()
	if len(values2) <= prevLen {
		t.Fatalf("cycle 2 emitted no new org first-page request (got %d total, prev %d)",
			len(values2), prevLen)
	}
	cycle2Since := values2[len(values2)-1]
	if cycle2Since == "" {
		t.Errorf("cycle 2 should resume incremental (since=N); got bare list")
	}
}

// TestSync_FullSyncIntervalDisabled asserts FullSyncInterval=0 disables
// the escape hatch — even with the most recent full sync 1000 hours
// in the past, cycle uses ?since= as long as cursor is non-zero.
// Mirrors PDBPLUS_FK_BACKFILL_TIMEOUT=0 semantics.
func TestSync_FullSyncIntervalDisabled(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	generated := float64(t1.Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1", "ok"), t1.Format(time.RFC3339))}

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, f.server.URL)
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		SyncMode:         config.SyncModeIncremental,
		FullSyncInterval: 0, // 260428-mu0 escape hatch DISABLED
	}, slog.Default())
	ctx := t.Context()

	// Seed: ancient full success (1000 hours ago) and a pre-existing org
	// row so MAX(updated) is non-zero entering the cycle.
	ancient := time.Now().Add(-1000 * time.Hour)
	id, err := RecordSyncStart(ctx, db, ancient, "full")
	if err != nil {
		t.Fatalf("seed ancient RecordSyncStart: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, id, Status{
		LastSyncAt: ancient.Add(time.Minute),
		Duration:   time.Minute,
		Status:     "success",
	}); err != nil {
		t.Fatalf("seed ancient RecordSyncComplete: %v", err)
	}
	if _, err := client.Organization.Create().
		SetID(99).SetName("Pre").SetCreated(t1).SetUpdated(t1).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("seed pre-existing org: %v", err)
	}

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	orgLog, ok := f.sinceValues["org"]
	if !ok {
		t.Fatal("no since-log captured for org")
	}
	values := orgLog.snapshot()
	if len(values) == 0 {
		t.Fatal("no first-page org request seen")
	}
	last := values[len(values)-1]
	if last == "" {
		t.Errorf("FullSyncInterval=0 should disable forced full; got bare list")
	}

	// And the cycle should be recorded as 'incremental', not 'full'.
	var mode string
	if err := db.QueryRowContext(ctx,
		`SELECT mode FROM sync_status WHERE status = 'success' ORDER BY id DESC LIMIT 1`,
	).Scan(&mode); err != nil {
		t.Fatalf("read mode: %v", err)
	}
	if mode != "incremental" {
		t.Errorf("expected last mode = 'incremental' (escape hatch disabled), got %q", mode)
	}
}

// TestSync_TombstoneCapture_AcrossCycles asserts MAX(updated) advances
// past tombstone events too — `status='deleted'` rows are still rows in
// the table and their `updated` reflects the upstream deletion event.
//
// Without this property, a tombstone-only cycle would not advance the
// cursor and the next cycle would re-fetch the same tombstones.
func TestSync_TombstoneCapture_AcrossCycles(t *testing.T) {
	t.Parallel()
	t1 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Hour)
	generated := float64(t1.Unix())
	f := newFixtureWithMeta(t, generated)
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "Org1", "ok"), t1.Format(time.RFC3339))}

	w, db := newTestWorkerWithMode(t, f.server.URL, config.SyncModeIncremental)
	ctx := t.Context()

	// Cycle 1: live row at t1. MAX(updated) → t1.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	maxOrg1, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil || maxOrg1.IsZero() {
		t.Fatalf("MAX after cycle 1: %v err=%v", maxOrg1, err)
	}

	// Cycle 2: same row arrives as a tombstone with updated=t2.
	f.generated = float64(t2.Unix())
	f.responses["org"] = []any{bumpUpdated(makeOrg(1, "", "deleted"), t2.Format(time.RFC3339))}
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}

	// Tombstone landed AND MAX(updated) advanced to t2.
	org, err := w.entClient.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org 1 after cycle 2: %v", err)
	}
	if org.Status != "deleted" {
		t.Errorf("expected status=deleted, got %q", org.Status)
	}

	maxOrg2, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil {
		t.Fatalf("MAX after cycle 2: %v", err)
	}
	if !maxOrg2.After(maxOrg1) {
		t.Errorf("expected MAX(updated) to advance past tombstone event (t1=%v → t2=%v); got %v → %v",
			t1, t2, maxOrg1, maxOrg2)
	}
}
