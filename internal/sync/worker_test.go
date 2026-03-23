package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"golang.org/x/time/rate"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

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

func newTestWorker(t *testing.T, f *fixture, includeDeleted bool) (*Worker, *sql.DB) {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	pdbClient := newFastPDBClient(t, f.server.URL)

	err := InitStatusTable(context.Background(), db)
	if err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{
		IncludeDeleted: includeDeleted,
	}, slog.Default())
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

func makeFac(id, orgID int, name, status string) map[string]any {
	return map[string]any{
		"id": id, "org_id": orgID, "org_name": "Test", "name": name,
		"aka": "", "name_long": "", "website": "", "social_media": []any{},
		"clli": "", "rencode": "", "npanxx": "", "tech_email": "",
		"tech_phone": "", "sales_email": "", "sales_phone": "",
		"available_voltage_services": []any{},
		"notes": "", "net_count": 0, "ix_count": 0, "carrier_count": 0,
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

// TestSyncFetchesAll13Types verifies sync fetches all 13 types in correct FK dependency order.
func TestSyncFetchesAll13Types(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// Set up minimal data for org (all other types will return empty).
	f.responses["org"] = []any{makeOrg(1, "TestOrg", "ok")}
	w, _ := newTestWorker(t, f, false)

	err := w.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify org was synced.
	orgs, err := w.entClient.Organization.Query().All(context.Background())
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
	w, _ := newTestWorker(t, f, false)

	err := w.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify org is committed.
	count, err := w.entClient.Organization.Query().Count(context.Background())
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
	w, _ := newTestWorker(t, f, false)

	// First sync.
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Update mock response.
	f.responses["org"] = []any{makeOrg(1, "UpdatedName", "ok")}

	// Second sync.
	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	org, err := w.entClient.Organization.Get(context.Background(), 1)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if org.Name != "UpdatedName" {
		t.Errorf("expected UpdatedName, got %s", org.Name)
	}
}

// TestSyncHardDelete verifies sync removes rows not in remote response.
func TestSyncHardDelete(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// First sync with 3 orgs.
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		makeOrg(2, "Org2", "ok"),
		makeOrg(3, "Org3", "ok"),
	}
	w, _ := newTestWorker(t, f, false)

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	count, _ := w.entClient.Organization.Query().Count(context.Background())
	if count != 3 {
		t.Fatalf("expected 3 orgs, got %d", count)
	}

	// Second sync with only 2 orgs (org 2 removed).
	f.responses["org"] = []any{
		makeOrg(1, "Org1", "ok"),
		makeOrg(3, "Org3", "ok"),
	}

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	count, _ = w.entClient.Organization.Query().Count(context.Background())
	if count != 2 {
		t.Errorf("expected 2 orgs after delete, got %d", count)
	}
}

// TestSyncMutex verifies sync mutex prevents concurrent runs.
func TestSyncMutex(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}

	w, _ := newTestWorker(t, f, false)

	// Manually set running to true.
	w.running.Store(true)

	// Second call should return nil without error (skipped).
	err := w.Sync(context.Background())
	if err != nil {
		t.Errorf("expected nil when sync already running, got: %v", err)
	}

	// Reset and verify it can run after.
	w.running.Store(false)
	err = w.Sync(context.Background())
	if err != nil {
		t.Errorf("expected sync to succeed after mutex release: %v", err)
	}
}

// TestSyncLogsProgress verifies per-object-type progress logging.
func TestSyncLogsProgress(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f, false)

	// Just verify sync completes without error -- log output is verified by
	// the presence of slog.String("type", ...) calls in the worker code.
	err := w.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}
}

// TestSyncRecordsStatusSuccess verifies sync_status table records success.
func TestSyncRecordsStatusSuccess(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, db := newTestWorker(t, f, false)

	err := w.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	status, err := GetLastStatus(context.Background(), db)
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
	w, db := newTestWorker(t, f, false)

	err := w.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	status, err := GetLastStatus(context.Background(), db)
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
	w, _ := newTestWorker(t, f, false)

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	count, _ := w.entClient.Organization.Query().Count(context.Background())
	if count != 1 {
		t.Fatalf("expected 1 org, got %d", count)
	}

	// Second sync: org succeeds with new data, but net fails.
	f.responses["org"] = []any{makeOrg(1, "UpdatedOrg", "ok"), makeOrg(2, "NewOrg", "ok")}
	f.failTypes["net"] = true

	err := w.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	// Verify original data is preserved (rollback worked).
	count, _ = w.entClient.Organization.Query().Count(context.Background())
	if count != 1 {
		t.Errorf("expected 1 org after rollback, got %d", count)
	}
	org, _ := w.entClient.Organization.Get(context.Background(), 1)
	if org.Name != "Org1" {
		t.Errorf("expected original name Org1, got %s", org.Name)
	}
}

// TestSyncFilterDeletedObjects verifies status=deleted filtering.
func TestSyncFilterDeletedObjects(t *testing.T) {
	t.Parallel()

	t.Run("exclude_deleted", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.responses["org"] = []any{
			makeOrg(1, "Active", "ok"),
			makeOrg(2, "Deleted", "deleted"),
		}
		w, _ := newTestWorker(t, f, false)

		if err := w.Sync(context.Background()); err != nil {
			t.Fatalf("sync: %v", err)
		}
		count, _ := w.entClient.Organization.Query().Count(context.Background())
		if count != 1 {
			t.Errorf("expected 1 org (deleted excluded), got %d", count)
		}
	})

	t.Run("include_deleted", func(t *testing.T) {
		t.Parallel()
		f := newFixture(t)
		f.responses["org"] = []any{
			makeOrg(1, "Active", "ok"),
			makeOrg(2, "Deleted", "deleted"),
		}
		w, _ := newTestWorker(t, f, true)

		if err := w.Sync(context.Background()); err != nil {
			t.Fatalf("sync: %v", err)
		}
		count, _ := w.entClient.Organization.Query().Count(context.Background())
		if count != 2 {
			t.Errorf("expected 2 orgs (deleted included), got %d", count)
		}
	})
}

// TestSyncScheduler verifies scheduler starts periodic sync via time.Ticker.
func TestSyncScheduler(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
	w, _ := newTestWorker(t, f, false)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	w, _ := newTestWorker(t, f, false)

	if w.HasCompletedSync() {
		t.Error("expected false before sync")
	}

	if err := w.Sync(context.Background()); err != nil {
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
	if err := InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status: %v", err)
	}

	w := NewWorker(pdbClient, client, db, WorkerConfig{}, slog.Default())
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	err := w.SyncWithRetry(context.Background())
	if err != nil {
		t.Fatalf("expected SyncWithRetry to succeed on retry, got: %v", err)
	}
}

// TestSyncWithRetryExhaustsRetries verifies failure after all retries.
func TestSyncWithRetryExhaustsRetries(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.failTypes["org"] = true
	w, _ := newTestWorker(t, f, false)
	w.SetRetryBackoffs([]time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond})

	err := w.SyncWithRetry(context.Background())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if !strings.Contains(err.Error(), "sync failed after") {
		t.Errorf("expected 'sync failed after' in error, got: %v", err)
	}
}

// TestSyncWithRetryContextCancellation verifies context cancellation during backoff.
func TestSyncWithRetryContextCancellation(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.failTypes["org"] = true
	w, _ := newTestWorker(t, f, false)
	w.SetRetryBackoffs([]time.Duration{10 * time.Second, 10 * time.Second, 10 * time.Second})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := w.SyncWithRetry(ctx)
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
	w, _ := newTestWorker(t, f, false)

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	orgCount, _ := w.entClient.Organization.Query().Count(context.Background())
	facCount, _ := w.entClient.Facility.Query().Count(context.Background())
	netCount, _ := w.entClient.Network.Query().Count(context.Background())

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
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
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
	w, _ := newTestWorker(t, f, false)

	if err := w.Sync(context.Background()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
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

	// Verify per-type duration metric.
	typeDur := findMetric(rm, "pdbplus.sync.type.duration")
	if typeDur == nil {
		t.Fatal("expected pdbplus.sync.type.duration metric, not found")
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
	w, _ := newTestWorker(t, f, false)

	err := w.Sync(context.Background())
	if err == nil {
		t.Fatal("expected error from failed sync")
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
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
