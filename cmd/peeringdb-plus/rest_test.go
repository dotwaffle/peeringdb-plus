package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
)

// restDBCounter provides unique database names for parallel tests.
var restDBCounter atomic.Int64

// restTestServer creates a REST server backed by an in-memory ent client
// seeded with test data and returns an httptest.Server. The caller can
// use the server's URL to make HTTP requests against the REST API.
func restTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	id := restDBCounter.Add(1)
	dsn := fmt.Sprintf("file:rest_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })
	ctx := t.Context()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed Organization.
	org := client.Organization.Create().
		SetID(1).
		SetName("Test Org").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SaveX(ctx)

	// Seed Networks (3 for filter/sort tests).
	for _, n := range []struct {
		id   int
		name string
		asn  int
	}{
		{1, "Alpha Net", 100},
		{2, "Beta Net", 200},
		{3, "Gamma Net", 300},
	} {
		client.Network.Create().
			SetID(n.id).
			SetName(n.name).
			SetAsn(n.asn).
			SetInfoUnicast(true).
			SetInfoMulticast(false).
			SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).
			SetPolicyRatio(false).
			SetAllowIxpUpdate(false).
			SetCreated(now).
			SetUpdated(now).
			SetStatus("ok").
			SetOrganization(org).
			SaveX(ctx)
	}

	// Seed Facility.
	fac := client.Facility.Create().
		SetID(1).
		SetName("Test Facility").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetOrganization(org).
		SaveX(ctx)

	// Seed InternetExchange.
	ix := client.InternetExchange.Create().
		SetID(1).
		SetName("Test IX").
		SetCity("London").
		SetCountry("GB").
		SetRegionContinent("Europe").
		SetMedia("Ethernet").
		SetProtoUnicast(true).
		SetProtoMulticast(false).
		SetProtoIpv6(true).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetOrganization(org).
		SaveX(ctx)

	// Seed IxLan.
	ixlan := client.IxLan.Create().
		SetID(1).
		SetMtu(9000).
		SetDot1qSupport(false).
		SetIxfIxpMemberListURLVisible("Public").
		SetIxfIxpImportEnabled(false).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetInternetExchange(ix).
		SaveX(ctx)

	// Seed IxPrefix.
	client.IxPrefix.Create().
		SetID(1).
		SetProtocol("IPv4").
		SetPrefix("198.51.100.0/24").
		SetInDfz(true).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetIxLan(ixlan).
		SaveX(ctx)

	// Seed IxFacility.
	client.IxFacility.Create().
		SetID(1).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetInternetExchange(ix).
		SetFacility(fac).
		SaveX(ctx)

	// Seed NetworkFacility.
	client.NetworkFacility.Create().
		SetID(1).
		SetLocalAsn(100).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetNetworkID(1).
		SetFacility(fac).
		SaveX(ctx)

	// Seed NetworkIxLan.
	client.NetworkIxLan.Create().
		SetID(1).
		SetSpeed(10000).
		SetAsn(100).
		SetIsRsPeer(false).
		SetBfdSupport(false).
		SetOperational(true).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetNetworkID(1).
		SetIxLan(ixlan).
		SaveX(ctx)

	// Seed Carrier.
	carrier := client.Carrier.Create().
		SetID(1).
		SetName("Test Carrier").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetOrganization(org).
		SaveX(ctx)

	// Seed CarrierFacility.
	client.CarrierFacility.Create().
		SetID(1).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetCarrier(carrier).
		SetFacility(fac).
		SaveX(ctx)

	// Seed Campus.
	client.Campus.Create().
		SetID(1).
		SetName("Test Campus").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetOrganization(org).
		SaveX(ctx)

	// Seed Poc. Phase 59-04 enabled an ent privacy Policy on Poc that
	// filters non-Public rows from anonymous (TierPublic) callers. This
	// REST test hits HTTP endpoints without the PrivacyTier middleware,
	// so the seeded row's visibility must be "Public" (the default) for
	// /pocs to include it in the anonymous listing. Visibility filtering
	// behaviour is exercised in internal/sync/policy_test.go.
	client.Poc.Create().
		SetID(1).
		SetRole("Abuse").
		SetVisible("Public").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetNetworkID(1).
		SaveX(ctx)

	restSrv, err := rest.NewServer(client, &rest.ServerConfig{})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}

	ts := httptest.NewServer(restSrv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestREST_ListAll verifies GET /{type} returns 200 with paginated JSON
// for each of the 13 PeeringDB entity types (REST-01).
func TestREST_ListAll(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	endpoints := []struct {
		name string
		path string
	}{
		{"organizations", "/organizations"},
		{"networks", "/networks"},
		{"facilities", "/facilities"},
		{"internet-exchanges", "/internet-exchanges"},
		{"ix-lans", "/ix-lans"},
		{"ix-prefixes", "/ix-prefixes"},
		{"ix-facilities", "/ix-facilities"},
		{"network-facilities", "/network-facilities"},
		{"network-ix-lans", "/network-ix-lans"},
		{"carriers", "/carriers"},
		{"carrier-facilities", "/carrier-facilities"},
		{"campuses", "/campuses"},
		{"pocs", "/pocs"},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			t.Parallel()

			resp, err := http.Get(ts.URL + ep.path)
			if err != nil {
				t.Fatalf("GET %s: %v", ep.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", ep.path, resp.StatusCode)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			content, ok := body["content"]
			if !ok {
				t.Fatalf("response missing 'content' field for %s", ep.path)
			}
			arr, ok := content.([]any)
			if !ok {
				t.Fatalf("'content' is not an array for %s", ep.path)
			}
			if len(arr) < 1 {
				t.Fatalf("'content' is empty for %s, want >= 1", ep.path)
			}

			totalCount, ok := body["total_count"]
			if !ok {
				t.Fatalf("response missing 'total_count' field for %s", ep.path)
			}
			tc, ok := totalCount.(float64)
			if !ok || tc < 1 {
				t.Fatalf("'total_count' = %v, want >= 1 for %s", totalCount, ep.path)
			}
		})
	}
}

// TestREST_ReadByID verifies GET /{type}/{id} returns 200 with a single
// entity containing the correct ID (REST-01).
func TestREST_ReadByID(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	resp, err := http.Get(ts.URL + "/networks/1")
	if err != nil {
		t.Fatalf("GET /networks/1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /networks/1 status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	id, ok := body["id"]
	if !ok {
		t.Fatal("response missing 'id' field")
	}
	if id.(float64) != 1 {
		t.Fatalf("id = %v, want 1", id)
	}

	name, ok := body["name"]
	if !ok {
		t.Fatal("response missing 'name' field")
	}
	if name != "Alpha Net" {
		t.Fatalf("name = %v, want Alpha Net", name)
	}
}

// TestREST_OpenAPISpec verifies GET /openapi.json returns a valid OpenAPI
// spec with only GET methods (REST-02, D-04).
func TestREST_OpenAPISpec(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	resp, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var spec map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}

	// Check openapi version field exists.
	if _, ok := spec["openapi"]; !ok {
		t.Fatal("spec missing 'openapi' field")
	}

	// Check paths exists and is not empty.
	paths, ok := spec["paths"]
	if !ok {
		t.Fatal("spec missing 'paths' field")
	}
	pathsMap, ok := paths.(map[string]any)
	if !ok || len(pathsMap) == 0 {
		t.Fatal("'paths' is empty or not an object")
	}

	// Verify no non-GET methods exist.
	forbiddenMethods := []string{"post", "put", "patch", "delete"}
	for path, methods := range pathsMap {
		methodMap, ok := methods.(map[string]any)
		if !ok {
			continue
		}
		for _, m := range forbiddenMethods {
			if _, exists := methodMap[m]; exists {
				t.Errorf("path %q has forbidden method %q", path, m)
			}
		}
	}
}

// TestREST_SortAndPaginate verifies sorting and pagination via query parameters (REST-03).
func TestREST_SortAndPaginate(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	t.Run("sort_by_id_desc", func(t *testing.T) {
		t.Parallel()

		resp, err := http.Get(ts.URL + "/networks?sort=id&order=desc")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		content := body["content"].([]any)
		if len(content) < 3 {
			t.Fatalf("expected >= 3 results, got %d", len(content))
		}

		// First result should be ID 3 (Gamma Net) in descending order.
		firstID := content[0].(map[string]any)["id"].(float64)
		if firstID != 3 {
			t.Fatalf("first result id = %v, want 3 (desc sort)", firstID)
		}

		lastID := content[len(content)-1].(map[string]any)["id"].(float64)
		if lastID != 1 {
			t.Fatalf("last result id = %v, want 1 (desc sort)", lastID)
		}
	})

	t.Run("sort_by_id_asc", func(t *testing.T) {
		t.Parallel()

		resp, err := http.Get(ts.URL + "/networks?sort=id&order=asc")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		content := body["content"].([]any)
		firstID := content[0].(map[string]any)["id"].(float64)
		if firstID != 1 {
			t.Fatalf("first result id = %v, want 1 (asc sort)", firstID)
		}
	})

	t.Run("paginate", func(t *testing.T) {
		t.Parallel()

		// Request page 1 with 2 items per page.
		resp, err := http.Get(ts.URL + "/networks?page=1&per_page=2")
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}

		content := body["content"].([]any)
		if len(content) != 2 {
			t.Fatalf("expected 2 results on page 1, got %d", len(content))
		}

		totalCount := body["total_count"].(float64)
		if totalCount != 3 {
			t.Fatalf("total_count = %v, want 3", totalCount)
		}

		isLastPage := body["is_last_page"].(bool)
		if isLastPage {
			t.Fatal("is_last_page should be false on page 1 of 2")
		}

		// Request page 2.
		resp2, err := http.Get(ts.URL + "/networks?page=2&per_page=2")
		if err != nil {
			t.Fatalf("GET page 2: %v", err)
		}
		defer resp2.Body.Close()

		var body2 map[string]any
		if err := json.NewDecoder(resp2.Body).Decode(&body2); err != nil {
			t.Fatalf("decode page 2: %v", err)
		}

		content2 := body2["content"].([]any)
		if len(content2) != 1 {
			t.Fatalf("expected 1 result on page 2, got %d", len(content2))
		}

		isLastPage2 := body2["is_last_page"].(bool)
		if !isLastPage2 {
			t.Fatal("is_last_page should be true on page 2")
		}
	})
}

// TestREST_FieldFiltering verifies per-field filtering on REST endpoints (REST-03).
// Exercises string equality, string contains, int equality, int range, status,
// empty result sets, and combined filter+pagination.
func TestREST_FieldFiltering(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	tests := []struct {
		name       string
		path       string
		wantCount  int
		wantTotal  float64
		checkField string
		checkValue any
	}{
		{
			name:       "string_equality_name",
			path:       "/networks?name.eq=Alpha+Net",
			wantCount:  1,
			wantTotal:  1,
			checkField: "name",
			checkValue: "Alpha Net",
		},
		{
			name:       "string_contains_name",
			path:       "/networks?name.has=Beta",
			wantCount:  1,
			wantTotal:  1,
			checkField: "name",
			checkValue: "Beta Net",
		},
		{
			name:       "int_equality_asn",
			path:       "/networks?asn.eq=200",
			wantCount:  1,
			wantTotal:  1,
			checkField: "asn",
			checkValue: float64(200),
		},
		{
			name:      "int_greater_than_asn",
			path:      "/networks?asn.gt=100",
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:      "int_less_than_asn",
			path:      "/networks?asn.lt=300",
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:       "status_filter",
			path:       "/organizations?status.eq=ok",
			wantCount:  1,
			wantTotal:  1,
			checkField: "status",
			checkValue: "ok",
		},
		{
			name:      "no_results",
			path:      "/networks?name.eq=NonExistent",
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:      "combined_filter_and_pagination",
			path:      "/networks?asn.gt=100&per_page=1",
			wantCount: 1,
			wantTotal: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := http.Get(ts.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", tc.path, resp.StatusCode)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			content, ok := body["content"].([]any)
			if !ok {
				t.Fatalf("'content' is not an array for %s", tc.path)
			}

			if len(content) != tc.wantCount {
				t.Fatalf("content length = %d, want %d for %s", len(content), tc.wantCount, tc.path)
			}

			totalCount, ok := body["total_count"].(float64)
			if !ok {
				t.Fatalf("'total_count' missing or not a number for %s", tc.path)
			}
			if totalCount != tc.wantTotal {
				t.Fatalf("total_count = %v, want %v for %s", totalCount, tc.wantTotal, tc.path)
			}

			if tc.checkField != "" && len(content) > 0 {
				first := content[0].(map[string]any)
				got := first[tc.checkField]
				if got != tc.checkValue {
					t.Fatalf("%s = %v, want %v for %s", tc.checkField, got, tc.checkValue, tc.path)
				}
			}
		})
	}
}

// TestREST_EagerLoad verifies that reading a single entity by ID
// returns eager-loaded edge data (REST-04).
func TestREST_EagerLoad(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	resp, err := http.Get(ts.URL + "/organizations/1")
	if err != nil {
		t.Fatalf("GET /organizations/1: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Ent serializes edges under the "edges" key.
	edges, ok := body["edges"]
	if !ok {
		t.Fatal("response missing 'edges' field -- eager loading not working")
	}

	edgesMap, ok := edges.(map[string]any)
	if !ok {
		t.Fatal("'edges' is not an object")
	}

	// Check that networks edge is populated (3 seeded networks belong to org 1).
	networks, ok := edgesMap["networks"]
	if !ok {
		t.Fatal("edges missing 'networks' key -- eager loading not configured")
	}

	networksArr, ok := networks.([]any)
	if !ok {
		t.Fatal("'networks' is not an array")
	}
	if len(networksArr) != 3 {
		t.Fatalf("expected 3 eager-loaded networks, got %d", len(networksArr))
	}
}

// syncCompletedTracker implements the HasCompletedSync() interface for testing
// readiness middleware behavior.
type syncCompletedTracker struct {
	completed bool
}

func (s *syncCompletedTracker) HasCompletedSync() bool {
	return s.completed
}

// TestREST_Readiness verifies that REST endpoints return 503 before sync
// completes and 200 after (D-14).
func TestREST_Readiness(t *testing.T) {
	t.Parallel()

	id := restDBCounter.Add(1)
	dsn := fmt.Sprintf("file:rest_readiness_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed minimal data for a valid response.
	client.Organization.Create().
		SetID(1).
		SetName("Test Org").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SaveX(t.Context())

	restSrv, err := rest.NewServer(client, &rest.ServerConfig{})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}

	tracker := &syncCompletedTracker{completed: false}

	// Wrap REST handler with readiness middleware matching main.go behavior.
	handler := readinessMiddleware(tracker, http.StripPrefix("/rest/v1", restSrv.Handler()))

	t.Run("503_before_sync", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/rest/v1/organizations", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("before sync: status = %d, want 503", rec.Code)
		}
	})

	// Mark sync complete.
	tracker.completed = true

	t.Run("200_after_sync", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/rest/v1/organizations", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("after sync: status = %d, want 200", rec.Code)
		}
	})
}

// TestREST_WriteMethodsRejected verifies that POST, PUT, PATCH, DELETE
// methods return 404 or 405 on REST endpoints (D-04).
func TestREST_WriteMethodsRejected(t *testing.T) {
	t.Parallel()
	ts := restTestServer(t)

	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/networks"},
		{"PUT", "/networks/1"},
		{"PATCH", "/networks/1"},
		{"DELETE", "/networks/1"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s_%s", tc.method, tc.path), func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("create request: %v", err)
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer resp.Body.Close()

			// Write methods should be rejected with 404 or 405.
			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want 404 or 405", tc.method, tc.path, resp.StatusCode)
			}
		})
	}
}
