package sync_test

import (
	"context"
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
			w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		data, ok := fs.fixtures[objType]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"meta":{},"data":`))
		w.Write(data)
		w.Write([]byte(`}`))
	}))
	t.Cleanup(func() { fs.server.Close() })

	return fs
}

// setFixtureData replaces the "data" array for a given type on the mock server.
// data should be a JSON-encoded array.
func (fs *fixtureServer) setFixtureData(objType string, data json.RawMessage) {
	fs.fixtures[objType] = data
}

// removeFixtureData removes fixture data for a type, making it return empty.
func (fs *fixtureServer) removeFixtureData(objType string) {
	delete(fs.fixtures, objType)
}

// newIntegrationWorker creates a sync worker wired to the fixture server with
// an in-memory ent client. Returns the worker and a cleanup is handled by t.Cleanup.
func newIntegrationWorker(t *testing.T, fs *fixtureServer, includeDeleted bool) *sync.Worker {
	t.Helper()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		IncludeDeleted: includeDeleted,
	}, slog.Default())

	return w
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

	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		IncludeDeleted: false,
	}, slog.Default())

	ctx := context.Background()

	// Run sync.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Verify record counts. Org has 3 records but org 3 is status=deleted
	// and IncludeDeleted=false, so expect 2.
	tests := []struct {
		name     string
		queryFn  func() (int, error)
		expected int
	}{
		{"organizations", func() (int, error) { return client.Organization.Query().Count(ctx) }, 2},
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
	status, err := sync.GetLastSyncStatus(ctx, db)
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

// TestSyncDeletesStaleRecords verifies that records removed from the remote
// API are deleted from the local database on re-sync.
func TestSyncDeletesStaleRecords(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		IncludeDeleted: false,
	}, slog.Default())

	ctx := context.Background()

	// First sync with full fixtures.
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Verify org 1 and org 2 exist (org 3 is deleted, excluded).
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if orgCount != 2 {
		t.Fatalf("expected 2 orgs after first sync, got %d", orgCount)
	}

	// Remove org 2 and all dependent records from fixture responses.
	// Org 2 is referenced by: campus 2, fac 2, ix 2, net 2.
	// Those in turn are referenced by: ixlan 2, ixpfx 2.
	fs.setFixtureData("org", json.RawMessage(`[
		{
			"id": 1, "name": "Example Organization", "aka": "ExOrg",
			"name_long": "Example Organization Inc.", "website": "https://example.org",
			"social_media": [], "notes": "", "address1": "", "address2": "",
			"city": "San Francisco", "state": "CA", "country": "US", "zipcode": "94105",
			"suite": "", "floor": "",
			"created": "2024-01-01T00:00:00Z", "updated": "2024-06-15T12:30:00Z",
			"status": "ok"
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

	// Second sync should delete org 2 (and its dependents).
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	orgCount, err = client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count after delete: %v", err)
	}
	if orgCount != 1 {
		t.Errorf("expected 1 org after stale deletion, got %d", orgCount)
	}

	// Verify org 1 still exists.
	org, err := client.Organization.Get(ctx, 1)
	if err != nil {
		t.Errorf("org 1 should still exist: %v", err)
	}
	if org != nil && org.Name != "Example Organization" {
		t.Errorf("org 1 name mismatch: %s", org.Name)
	}

	// Verify dependent records of org 2 were also removed.
	ixCount, err := client.InternetExchange.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query ix count: %v", err)
	}
	if ixCount != 1 {
		t.Errorf("expected 1 IX after stale deletion, got %d", ixCount)
	}
}

// TestSyncIncludeDeleted verifies that IncludeDeleted=true includes
// status=deleted records in the database.
func TestSyncIncludeDeleted(t *testing.T) {
	t.Parallel()
	fs := newFixtureServer(t)
	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(fs.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		IncludeDeleted: true,
	}, slog.Default())

	ctx := context.Background()

	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// With IncludeDeleted=true, all 3 orgs should be present.
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("query org count: %v", err)
	}
	if orgCount != 3 {
		t.Errorf("expected 3 orgs with IncludeDeleted=true, got %d", orgCount)
	}

	// Verify org 3 (status=deleted) exists and has correct status.
	org3, err := client.Organization.Get(ctx, 3)
	if err != nil {
		t.Fatalf("get org 3: %v", err)
	}
	if org3.Status != "deleted" {
		t.Errorf("org 3 status: expected deleted, got %s", org3.Status)
	}
	if org3.Name != "Defunct Telecom" {
		t.Errorf("org 3 name: expected Defunct Telecom, got %s", org3.Name)
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

	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		IncludeDeleted: false,
	}, slog.Default())

	ctx := context.Background()

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
