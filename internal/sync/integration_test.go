package sync_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// fixtureTypeMap maps PeeringDB API path names to fixture filenames.
var fixtureTypeMap = map[string]string{
	"org":        "org.json",
	"net":        "net.json",
	"fac":        "fac.json",
	"ix":         "ix.json",
	"poc":        "poc.json",
	"ixlan":      "ixlan.json",
	"ixpfx":      "ixpfx.json",
	"netixlan":   "netixlan.json",
	"netfac":     "netfac.json",
	"ixfac":      "ixfac.json",
	"carrier":    "carrier.json",
	"carrierfac": "carrierfac.json",
	"campus":     "campus.json",
}

// loadFixture reads a fixture file from testdata/fixtures/.
func loadFixture(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", filename))
	if err != nil {
		t.Fatalf("load fixture %s: %v", filename, err)
	}
	return data
}

// fixtureServer holds the mock PeeringDB API server and its fixture data.
type fixtureServer struct {
	server   *httptest.Server
	fixtures map[string]json.RawMessage // type -> raw "data" array
}

// newFixtureServer creates a mock PeeringDB API server that serves
// fixture data from testdata/fixtures/ JSON files.
func newFixtureServer(t *testing.T) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{
		fixtures: make(map[string]json.RawMessage),
	}

	// Load all fixture files.
	for apiType, filename := range fixtureTypeMap {
		raw := loadFixture(t, filename)
		// Parse to extract the "data" array.
		var resp struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("parse fixture %s: %v", filename, err)
		}
		fs.fixtures[apiType] = resp.Data
	}

	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract object type from URL path /api/{type}
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		objType := strings.Split(path, "?")[0]

		// Only return data on first page (skip=0). Return empty on subsequent
		// pages to terminate pagination.
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
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

// setFixtureData replaces the "data" array for a given type on the mock server.
// data should be a JSON-encoded array.
func (fs *fixtureServer) setFixtureData(objType string, data json.RawMessage) {
	fs.fixtures[objType] = data
}

// TestFullSyncWithFixtures verifies the full sync pipeline processes all 13
// fixture types end-to-end and stores them in the database.
func TestFullSyncWithFixtures(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

	ctx := t.Context()

	// Run sync.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify record counts. Phase 68 D-01: PDBPLUS_INCLUDE_DELETED was removed,
	// so the upsert path now unconditionally persists org 3 (status=deleted).
	// Plan 68-02 flips the delete pass to soft-delete; in the meantime the
	// hard-delete pass still runs but org 3's ID is in remoteIDs so it stays.
	tests := []struct {
		name     string
		queryFn  func() (int, error)
		expected int
	}{
		{"organizations", func() (int, error) { return client.Organization.Query().Count(ctx) }, 3},
		{"campuses", func() (int, error) { return client.Campus.Query().Count(ctx) }, 2},
		{"facilities", func() (int, error) { return client.Facility.Query().Count(ctx) }, 2},
		{"carriers", func() (int, error) { return client.Carrier.Query().Count(ctx) }, 1},
		{"carrier_facilities", func() (int, error) { return client.CarrierFacility.Query().Count(ctx) }, 1},
		{"internet_exchanges", func() (int, error) { return client.InternetExchange.Query().Count(ctx) }, 2},
		{"ix_lans", func() (int, error) { return client.IxLan.Query().Count(ctx) }, 2},
		{"ix_prefixes", func() (int, error) { return client.IxPrefix.Query().Count(ctx) }, 2},
		{"ix_facilities", func() (int, error) { return client.IxFacility.Query().Count(ctx) }, 1},
		{"networks", func() (int, error) { return client.Network.Query().Count(ctx) }, 2},
		{"pocs", func() (int, error) { return client.Poc.Query().Count(ctx) }, 1},
		{"network_facilities", func() (int, error) { return client.NetworkFacility.Query().Count(ctx) }, 1},
		{"network_ix_lans", func() (int, error) { return client.NetworkIxLan.Query().Count(ctx) }, 1},
	}

	for _, tt := range tests {
		count, err := tt.queryFn()
		if err != nil {
			t.Errorf("%s: query error: %v", tt.name, err)
			continue
		}
		if count != tt.expected {
			t.Errorf("%s: expected %d, got %d", tt.name, tt.expected, count)
		}
	}

	// Verify specific field values from fixtures.
	net, err := client.Network.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get network 1: %v", err)
	}
	if net.Asn != 65001 {
		t.Errorf("network 1 ASN: expected 65001, got %d", net.Asn)
	}
	if net.Name != "Example Network" {
		t.Errorf("network 1 name: expected Example Network, got %s", net.Name)
	}

	org, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org 1: %v", err)
	}
	if org.Name != "Example Organization" {
		t.Errorf("org 1 name: expected Example Organization, got %s", org.Name)
	}

	// Verify edge traversal: network 1 -> organization.
	linkedOrg, err := net.QueryOrganization().Only(ctx)
	if err != nil {
		t.Fatalf("traverse network->org edge: %v", err)
	}
	if linkedOrg.ID != 1 {
		t.Errorf("network 1 org edge: expected org 1, got org %d", linkedOrg.ID)
	}
	if linkedOrg.Name != "Example Organization" {
		t.Errorf("network 1 org name: expected Example Organization, got %s", linkedOrg.Name)
	}

	// Verify sync_status records success.
	status, err := sync.GetLastStatus(ctx, db)
	if err != nil {
		t.Fatalf("get sync status: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil sync status")
	}
	if status.Status != "success" {
		t.Errorf("sync status: expected success, got %s", status.Status)
	}
}

// TestSyncTombstonesExplicitDeletedRecords verifies that explicit
// status='deleted' rows from upstream's ?since= responses land as
// tombstones on re-sync.
//
// Quick task 260428-2zl Task 6: pre-2zl this test (TestSyncDeletesStaleRecords)
// asserted the inference-by-absence path — drop org 2 from the
// response → local row marked deleted via the markStaleDeleted family.
// That inference was structurally wrong against PeeringDB's
// serializer-side filtering and is removed in T6. Post-2zl, the test
// seeds an explicit `{"id": 2, "status":"deleted"}` payload and
// asserts the upsert path lands the tombstone without any delete
// pass.
func TestSyncTombstonesExplicitDeletedRecords(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

	ctx := t.Context()

	// First sync with full fixtures.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Verify all 3 orgs exist (Phase 68 D-01: upsert persists deleted
	// rows unconditionally).
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if orgCount != 3 {
		t.Fatalf("expected 3 orgs after first sync, got %d", orgCount)
	}

	// Upstream re-sync delivers org 2 as an explicit status='deleted'
	// tombstone (mirrors the ?since= shape per rest.py:694-727).
	// Org 1 stays 'ok'; dependents of org 2 also tombstone.
	// 260428-eda CHANGE 3: bump org 2's `updated` past the cycle-1
	// fixture (which had updated=2024-08-01T09:15:00Z) so the
	// skip-on-unchanged predicate admits the status flip.
	fs.setFixtureData("org", json.RawMessage(`[
		{
			"id": 1, "name": "Example Organization", "aka": "ExOrg",
			"name_long": "Example Organization Inc.", "website": "https://example.org",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "San Francisco", "state": "CA", "country": "US", "zipcode": "94105",
			"suite": "", "floor": "",
			"created": "2024-01-01T00:00:00Z", "updated": "2024-06-15T12:30:00Z",
			"status": "ok"
		},
		{
			"id": 2, "name": "Regional Networks", "aka": "",
			"name_long": "", "website": "",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "", "state": "", "country": "US", "zipcode": "",
			"suite": "", "floor": "",
			"created": "2024-02-01T00:00:00Z", "updated": "2024-09-01T12:30:00Z",
			"status": "deleted"
		}
	]`))
	// Keep only records referencing org 1.
	fs.setFixtureData("campus", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "org_name": "Example Organization",
			"name": "Example Campus West", "name_long": "Example Organization Campus West Coast",
			"aka": "ECW", "website": "", "social_media": [], "notes": "",
			"country": "US", "city": "San Francisco", "zipcode": "94105", "state": "CA",
			"created": "2024-01-15T08:00:00Z", "updated": "2024-07-01T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("fac", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "org_name": "Example Organization", "campus_id": 1,
			"name": "Example DC1", "aka": "", "name_long": "", "website": "",
			"social_media": [], "clli": "", "rencode": "", "npanxx": "",
			"tech_email": "", "tech_phone": "", "sales_email": "", "sales_phone": "",
			"available_voltage_services": [], "notes": "", "net_count": 0, "ix_count": 0,
			"carrier_count": 0, "address1": "", "address2": "", "city": "", "state": "",
			"country": "US", "zipcode": "", "suite": "", "floor": "",
			"created": "2024-01-20T12:00:00Z", "updated": "2024-08-10T10:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ix", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "name": "Bay Area IX", "aka": "", "name_long": "",
			"city": "San Francisco", "country": "US", "region_continent": "North America",
			"media": "Ethernet", "notes": "", "proto_unicast": true, "proto_multicast": false,
			"proto_ipv6": true, "website": "", "social_media": [], "url_stats": "",
			"tech_email": "", "tech_phone": "", "policy_email": "", "policy_phone": "",
			"sales_email": "", "sales_phone": "", "net_count": 0, "fac_count": 0,
			"ixf_net_count": 0, "ixf_last_import": null, "ixf_import_request": null,
			"ixf_import_request_status": "", "service_level": "", "terms": "",
			"created": "2024-01-25T09:00:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ixlan", json.RawMessage(`[
		{
			"id": 1, "ix_id": 1, "name": "Bay Area IX LAN", "descr": "", "mtu": 9000,
			"dot1q_support": false, "rs_asn": 65500, "arp_sponge": null,
			"ixf_ixp_member_list_url_visible": "Public", "ixf_ixp_import_enabled": true,
			"created": "2024-01-25T09:30:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ixpfx", json.RawMessage(`[
		{
			"id": 1, "ixlan_id": 1, "protocol": "IPv4", "prefix": "198.51.100.0/24",
			"in_dfz": false, "notes": "",
			"created": "2024-01-25T10:00:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("net", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "name": "Example Network", "aka": "", "name_long": "",
			"website": "", "social_media": [], "asn": 65001,
			"looking_glass": "", "route_server": "", "irr_as_set": "",
			"info_type": "", "info_types": [],
			"info_traffic": "", "info_ratio": "", "info_scope": "",
			"info_unicast": true, "info_multicast": false, "info_ipv6": true,
			"info_never_via_route_servers": false, "notes": "", "policy_url": "",
			"policy_general": "", "policy_locations": "", "policy_ratio": false,
			"policy_contracts": "", "allow_ixp_update": false,
			"ix_count": 0, "fac_count": 0,
			"created": "2024-01-30T08:00:00Z", "updated": "2024-08-20T10:00:00Z", "status": "ok"
		}
	]`))

	// Second sync: upstream emits org 2 as an explicit tombstone.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Quick task 260428-2zl Task 6: with no delete pass, the local row
	// for org 2 transitions to status='deleted' via the explicit
	// upstream payload (NOT via inference-by-absence). Org 1 remains
	// 'ok'; org 3 (pre-existing 'deleted' from cycle 1's fixture) is
	// untouched.
	orgCount, err = client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if orgCount != 3 {
		t.Errorf("expected 3 orgs, got %d", orgCount)
	}

	// Verify org 1 still exists with status='ok'.
	org, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Errorf("org 1 should still exist: %v", err)
	}
	if org != nil && org.Name != "Example Organization" {
		t.Errorf("org 1 name mismatch: %s", org.Name)
	}
	if org != nil && org.Status != "ok" {
		t.Errorf("org 1 status: want 'ok', got %q", org.Status)
	}

	// Verify org 2 is now tombstoned via the explicit payload.
	org2, err := client.Organization.Get(ctx, 2)
	if err != nil {
		t.Fatalf("org 2 should still exist as tombstone: %v", err)
	}
	if org2.Status != "deleted" {
		t.Errorf("org 2 status: want 'deleted' (from explicit payload), got %q", org2.Status)
	}

	// Quick task 260428-2zl Task 6: dependent records (IXes etc) absent
	// from cycle 2's response stay as-is in the local DB. Pre-2zl the
	// inference-by-absence path would have soft-deleted them; post-2zl
	// upstream is responsible for delivering explicit tombstones for
	// the dependents (which it does via ?since= responses, but this
	// test deliberately omits them to verify NO inference fires). The
	// IX count therefore remains at the cycle-1 value (2 IXes).
	ixCount, err := client.InternetExchange.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query ix count: %v", err)
	}
	if ixCount != 2 {
		t.Errorf("expected 2 IXes (no inference path runs), got %d", ixCount)
	}
}

// TestSync_TombstonePersistedFromExplicitPayload asserts that explicit
// status='deleted' rows from upstream's ?since= payloads land as
// tombstones in the local DB. Quick task 260428-2zl Task 6: pre-2zl
// this test (TestSync_SoftDeleteMarksRows) asserted the
// inference-by-absence path — drop org 1 from the response → local
// row marked deleted via markStaleDeletedOrganizations. That
// inference is removed in T6; the test now seeds the explicit payload
// and asserts the upsert path delivers the same outcome.
//
// Shape:
//  1. Cycle 1 syncs 3 orgs (fixture default). All land; counts stable.
//  2. Cycle 2 keeps org 1 in the response but flips its status to
//     'deleted' (the upstream ?since= shape).
//  3. Row count stays 3; org 1 transitions to 'deleted' with a fresh
//     updated timestamp (per the upstream payload).
//  4. Org 3 (pre-existing status='deleted' in the fixture) remains
//     marked deleted — re-upsert is idempotent.
func TestSync_TombstonePersistedFromExplicitPayload(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	ctx := t.Context()

	// Cycle 1: fixture serves 3 orgs. Run full sync.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("cycle 1 sync: %v", err)
	}
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count after cycle 1: %v", err)
	}
	if orgCount != 3 {
		t.Fatalf("cycle 1: want 3 orgs, got %d", orgCount)
	}
	// Sanity-check org 1 is status='ok' before cycle 2.
	org1, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get org 1 after cycle 1: %v", err)
	}
	if org1.Status != "ok" {
		t.Fatalf("cycle 1: org 1 status = %q, want %q", org1.Status, "ok")
	}

	// Cycle 2 fixture: upstream ?since= response shape, with org 1
	// flipped to 'deleted' (explicit tombstone payload). Pre-2zl this
	// test dropped org 1 from the response and relied on inference;
	// post-2zl the worker only persists what upstream sends.
	// 260428-eda CHANGE 3: bump org 1's `updated` past the cycle 1
	// fixture so the skip-on-unchanged predicate admits the status flip
	// from "ok" to "deleted". Real PeeringDB always bumps `updated` when
	// row content changes; cycle-1 fixture had updated=2024-06-15.
	fs.setFixtureData("org", json.RawMessage(`[
		{
			"id": 1, "name": "Example Organization", "aka": "ExOrg",
			"name_long": "Example Organization Inc.", "website": "https://example.org",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "San Francisco", "state": "CA", "country": "US", "zipcode": "94105",
			"suite": "", "floor": "",
			"created": "2024-01-01T00:00:00Z", "updated": "2024-09-01T12:30:00Z",
			"status": "deleted"
		},
		{
			"id": 2, "name": "Regional Networks", "aka": "",
			"name_long": "", "website": "",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "", "state": "", "country": "US", "zipcode": "",
			"suite": "", "floor": "",
			"created": "2024-02-01T00:00:00Z", "updated": "2024-06-15T12:30:00Z",
			"status": "ok"
		},
		{
			"id": 3, "name": "Defunct Telecom", "aka": "",
			"name_long": "", "website": "",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "", "state": "", "country": "US", "zipcode": "",
			"suite": "", "floor": "",
			"created": "2024-03-01T00:00:00Z", "updated": "2024-06-15T12:30:00Z",
			"status": "deleted"
		}
	]`))

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("cycle 2 sync: %v", err)
	}

	// Row count unchanged.
	orgCount, err = client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count after cycle 2: %v", err)
	}
	if orgCount != 3 {
		t.Fatalf("cycle 2: want 3 orgs still, got %d", orgCount)
	}

	// Org 1 now has status='deleted' from the explicit upstream payload.
	// Quick task 260428-2zl Task 6: pre-2zl, .Updated was set to
	// cycleStart by markStaleDeletedOrganizations; post-2zl it carries
	// whatever value the upstream payload sent (the fixture's
	// "updated": "2024-06-15T12:30:00Z" above).
	org1, err = client.Organization.Get(ctx, 1)
	if err != nil {
		t.Fatalf("org 1 missing after sync: %v", err)
	}
	if org1.Status != "deleted" {
		t.Errorf("org 1 status = %q, want %q", org1.Status, "deleted")
	}

	// Org 2 still has status='ok' (in the cycle-2 remoteIDs).
	org2, err := client.Organization.Get(ctx, 2)
	if err != nil {
		t.Fatalf("get org 2: %v", err)
	}
	if org2.Status != "ok" {
		t.Errorf("org 2 status = %q, want %q", org2.Status, "ok")
	}

	// Org 3 was already status='deleted' from the fixture; it remains so.
	org3, err := client.Organization.Get(ctx, 3)
	if err != nil {
		t.Fatalf("get org 3: %v", err)
	}
	if org3.Status != "deleted" {
		t.Errorf("org 3 status = %q, want %q", org3.Status, "deleted")
	}
}

// TestSyncFKIntegrity_AfterTombstoneCycle verifies that running a sync
// cycle that includes explicit status='deleted' tombstones leaves the
// DB without FK violations. Quick task 260428-2zl Task 6: pre-2zl the
// inference-by-absence delete pass meant the test had to verify that
// the post-delete row set was internally consistent. Post-2zl no rows
// are removed and no FKs change as a result of the sync, so the
// invariant is necessarily preserved — but the foreign_key_check is
// kept as a regression lock against any future schema change that
// might re-introduce hard-delete semantics.
func TestSyncFKIntegrity_AfterTombstoneCycle(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

	ctx := t.Context()

	// First sync with full fixtures.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Cycle 2: org 2 arrives as an explicit tombstone (mirrors ?since=
	// shape). Org 1 remains live; dependents of org 2 are absent from
	// the response (post-2zl no inference fires for them — they stay
	// as-is with their cycle-1 status='ok' values).
	//
	// 260428-eda CHANGE 3: bump org 2's `updated` past the cycle-1
	// fixture (which had updated=2024-08-01T09:15:00Z) so the
	// skip-on-unchanged predicate admits the status flip.
	fs.setFixtureData("org", json.RawMessage(`[
		{
			"id": 1, "name": "Example Organization", "aka": "ExOrg",
			"name_long": "Example Organization Inc.", "website": "https://example.org",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "San Francisco", "state": "CA", "country": "US", "zipcode": "94105",
			"suite": "", "floor": "",
			"created": "2024-01-01T00:00:00Z", "updated": "2024-06-15T12:30:00Z",
			"status": "ok"
		},
		{
			"id": 2, "name": "Regional Networks", "aka": "",
			"name_long": "", "website": "",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "", "state": "", "country": "US", "zipcode": "",
			"suite": "", "floor": "",
			"created": "2024-02-01T00:00:00Z", "updated": "2024-09-01T12:30:00Z",
			"status": "deleted"
		}
	]`))
	fs.setFixtureData("campus", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "org_name": "Example Organization",
			"name": "Example Campus West", "name_long": "Example Organization Campus West Coast",
			"aka": "ECW", "website": "", "social_media": [], "notes": "",
			"country": "US", "city": "San Francisco", "zipcode": "94105", "state": "CA",
			"created": "2024-01-15T08:00:00Z", "updated": "2024-07-01T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("fac", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "org_name": "Example Organization", "campus_id": 1,
			"name": "Example DC1", "aka": "", "name_long": "", "website": "",
			"social_media": [], "clli": "", "rencode": "", "npanxx": "",
			"tech_email": "", "tech_phone": "", "sales_email": "", "sales_phone": "",
			"available_voltage_services": [], "notes": "", "net_count": 0, "ix_count": 0,
			"carrier_count": 0, "address1": "", "address2": "", "city": "", "state": "",
			"country": "US", "zipcode": "", "suite": "", "floor": "",
			"created": "2024-01-20T12:00:00Z", "updated": "2024-08-10T10:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ix", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "name": "Bay Area IX", "aka": "", "name_long": "",
			"city": "San Francisco", "country": "US", "region_continent": "North America",
			"media": "Ethernet", "notes": "", "proto_unicast": true, "proto_multicast": false,
			"proto_ipv6": true, "website": "", "social_media": [], "url_stats": "",
			"tech_email": "", "tech_phone": "", "policy_email": "", "policy_phone": "",
			"sales_email": "", "sales_phone": "", "net_count": 0, "fac_count": 0,
			"ixf_net_count": 0, "ixf_last_import": null, "ixf_import_request": null,
			"ixf_import_request_status": "", "service_level": "", "terms": "",
			"created": "2024-01-25T09:00:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ixlan", json.RawMessage(`[
		{
			"id": 1, "ix_id": 1, "name": "Bay Area IX LAN", "descr": "", "mtu": 9000,
			"dot1q_support": false, "rs_asn": 65500, "arp_sponge": null,
			"ixf_ixp_member_list_url_visible": "Public", "ixf_ixp_import_enabled": true,
			"created": "2024-01-25T09:30:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("ixpfx", json.RawMessage(`[
		{
			"id": 1, "ixlan_id": 1, "protocol": "IPv4", "prefix": "198.51.100.0/24",
			"in_dfz": false, "notes": "",
			"created": "2024-01-25T10:00:00Z", "updated": "2024-08-15T14:00:00Z", "status": "ok"
		}
	]`))
	fs.setFixtureData("net", json.RawMessage(`[
		{
			"id": 1, "org_id": 1, "name": "Example Network", "aka": "", "name_long": "",
			"website": "", "social_media": [], "asn": 65001,
			"looking_glass": "", "route_server": "", "irr_as_set": "",
			"info_type": "", "info_types": [],
			"info_traffic": "", "info_ratio": "", "info_scope": "",
			"info_unicast": true, "info_multicast": false, "info_ipv6": true,
			"info_never_via_route_servers": false, "notes": "", "policy_url": "",
			"policy_general": "", "policy_locations": "", "policy_ratio": false,
			"policy_contracts": "", "allow_ixp_update": false,
			"ix_count": 0, "fac_count": 0,
			"created": "2024-01-30T08:00:00Z", "updated": "2024-08-20T10:00:00Z", "status": "ok"
		}
	]`))

	// Second sync: org 2 lands as an explicit tombstone via the upstream
	// payload (no inference fires).
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Row counts stay the same. Org 2 transitions to status='deleted';
	// org 3 (fixture-seeded status='deleted') stays marked.
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if orgCount != 3 {
		t.Errorf("expected 3 orgs, got %d", orgCount)
	}
	org2, err := client.Organization.Get(ctx, 2)
	if err != nil {
		t.Fatalf("org 2 should still exist as tombstone: %v", err)
	}
	if org2.Status != "deleted" {
		t.Errorf("org 2 status: want 'deleted' (from explicit payload), got %q", org2.Status)
	}

	// Re-enable FK constraints and run foreign_key_check to verify no FK
	// violations exist after the cycle (trivially true post-2zl because
	// rows are never removed, but the check locks the invariant against
	// future hard-delete reintroductions).
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable FK constraints: %v", err)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var table, rowid, parent, fkid string
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			t.Fatalf("scan FK violation: %v", err)
		}
		violations = append(violations, table+":"+rowid+"->"+parent)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate FK violations: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("FK violations after sync delete: %v", violations)
	}
}

// TestSyncIdempotent verifies that running sync twice with the same fixture
// data produces identical results with no duplicates.
func TestSyncIdempotent(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

	ctx := t.Context()

	// First sync.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Record counts after first sync.
	orgCount1, _ := client.Organization.Query().Count(ctx)
	netCount1, _ := client.Network.Query().Count(ctx)
	facCount1, _ := client.Facility.Query().Count(ctx)
	ixCount1, _ := client.InternetExchange.Query().Count(ctx)

	// Second sync with identical data.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	// Verify counts are identical (no duplicates).
	orgCount2, _ := client.Organization.Query().Count(ctx)
	netCount2, _ := client.Network.Query().Count(ctx)
	facCount2, _ := client.Facility.Query().Count(ctx)
	ixCount2, _ := client.InternetExchange.Query().Count(ctx)

	if orgCount1 != orgCount2 {
		t.Errorf("org count changed: %d -> %d", orgCount1, orgCount2)
	}
	if netCount1 != netCount2 {
		t.Errorf("net count changed: %d -> %d", netCount1, netCount2)
	}
	if facCount1 != facCount2 {
		t.Errorf("fac count changed: %d -> %d", facCount1, facCount2)
	}
	if ixCount1 != ixCount2 {
		t.Errorf("ix count changed: %d -> %d", ixCount1, ixCount2)
	}

	// Verify data integrity after idempotent run.
	net, err := client.Network.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get network 1 after second sync: %v", err)
	}
	if net.Asn != 65001 {
		t.Errorf("network 1 ASN after idempotent sync: expected 65001, got %d", net.Asn)
	}
}
