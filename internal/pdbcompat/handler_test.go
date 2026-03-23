package pdbcompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// testEnvelope is used to decode PeeringDB-style JSON responses.
type testEnvelope struct {
	Meta json.RawMessage `json:"meta"`
	Data json.RawMessage `json:"data"`
}

// testErrorMeta is used to decode error metadata from the envelope.
type testErrorMeta struct {
	Error string `json:"error"`
}

// setupTestHandler creates a Handler with 3 test networks for use in tests.
// Returns the handler and a ServeMux with routes registered.
func setupTestHandler(t *testing.T) (*Handler, *http.ServeMux) {
	t.Helper()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	now := time.Now().Truncate(time.Second).UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	// Create 3 networks with different names, ASNs, statuses, and timestamps.
	_, err := client.Network.Create().
		SetName("CloudNet").
		SetAsn(13335).
		SetStatus("ok").
		SetCreated(past).
		SetUpdated(past).
		Save(ctx)
	if err != nil {
		t.Fatalf("create network 1: %v", err)
	}

	_, err = client.Network.Create().
		SetName("EdgeProvider").
		SetAsn(64496).
		SetStatus("ok").
		SetCreated(now).
		SetUpdated(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create network 2: %v", err)
	}

	_, err = client.Network.Create().
		SetName("CloudySkies Corp").
		SetAsn(65000).
		SetStatus("deleted").
		SetCreated(future).
		SetUpdated(future).
		Save(ctx)
	if err != nil {
		t.Fatalf("create network 3: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func TestListEndpoint(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// Data should be a non-empty array.
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data array: %v", err)
	}
	if len(data) != 3 {
		t.Errorf("expected 3 items, got %d", len(data))
	}
}

func TestListEndpointTrailingSlash(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// Without trailing slash.
	req1 := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)

	// With trailing slash per D-02.
	req2 := httptest.NewRequest(http.MethodGet, "/api/net/", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec1.Code != http.StatusOK {
		t.Fatalf("no slash: expected 200, got %d", rec1.Code)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("trailing slash: expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	if rec1.Body.String() != rec2.Body.String() {
		t.Errorf("responses differ:\n  no slash:      %s\n  trailing slash: %s",
			rec1.Body.String(), rec2.Body.String())
	}
}

func TestDetailEndpoint(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// First get list to find the first ID.
	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatalf("unmarshal items: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no items in list")
	}
	firstID := int(items[0]["id"].(float64))

	// Now fetch detail.
	detReq := httptest.NewRequest(http.MethodGet, "/api/net/"+itoa(firstID), nil)
	detRec := httptest.NewRecorder()
	mux.ServeHTTP(detRec, detReq)

	if detRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", detRec.Code, detRec.Body.String())
	}

	var detEnv testEnvelope
	if err := json.Unmarshal(detRec.Body.Bytes(), &detEnv); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}

	// Pitfall 7: single object wrapped in array.
	var detData []json.RawMessage
	if err := json.Unmarshal(detEnv.Data, &detData); err != nil {
		t.Fatalf("unmarshal detail data: %v", err)
	}
	if len(detData) != 1 {
		t.Errorf("expected 1 item in detail, got %d", len(detData))
	}
}

func TestDetailNotFound(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/net/99999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}

	var meta testErrorMeta
	if err := json.Unmarshal(env.Meta, &meta); err != nil {
		t.Fatalf("unmarshal error meta: %v", err)
	}
	if meta.Error == "" {
		t.Error("expected non-empty error message in meta")
	}

	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal error data: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
}

func TestUnknownType(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/badtype", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestIndex(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var index map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}

	// Should have all 13 types.
	if len(index) != 13 {
		t.Errorf("expected 13 types in index, got %d", len(index))
	}

	// Check a few known entries.
	for _, typeName := range []string{"net", "ix", "fac", "org", "poc"} {
		entry, ok := index[typeName]
		if !ok {
			t.Errorf("missing type %q in index", typeName)
			continue
		}
		if entry["list_endpoint"] != "/api/"+typeName {
			t.Errorf("type %q: expected list_endpoint /api/%s, got %q", typeName, typeName, entry["list_endpoint"])
		}
	}
}

func TestPagination(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{"limit 1", "/api/net?limit=1", 1},
		{"limit 2", "/api/net?limit=2", 2},
		{"skip 1", "/api/net?skip=1", 2},
		{"limit 1 skip 1", "/api/net?limit=1&skip=1", 1},
		{"skip all", "/api/net?skip=3", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}

			var env testEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			var data []json.RawMessage
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("unmarshal data: %v", err)
			}
			if len(data) != tt.wantCount {
				t.Errorf("expected %d items, got %d", tt.wantCount, len(data))
			}
		})
	}
}

func TestSinceFilter(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// Use a timestamp between the first and second network's updated time.
	// The first network was created 2 hours ago, so "since" 1 hour ago should
	// exclude it and return 2 networks.
	sinceTS := time.Now().Add(-1 * time.Hour).Unix()

	req := httptest.NewRequest(http.MethodGet, "/api/net?since="+itoa(int(sinceTS)), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	// Should return 2 networks (now and future, but not past).
	if len(data) != 2 {
		t.Errorf("expected 2 items after since filter, got %d", len(data))
	}
}

func TestQueryFilterContains(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// name__contains=cloud should match CloudNet and CloudySkies Corp (case-insensitive).
	req := httptest.NewRequest(http.MethodGet, "/api/net?name__contains=cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 items matching cloud, got %d", len(data))
	}
}

func TestExactFilter(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// status=ok should match 2 networks.
	req := httptest.NewRequest(http.MethodGet, "/api/net?status=ok", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 items with status=ok, got %d", len(data))
	}
}

func TestResponseHeaders(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if pb := rec.Header().Get("X-Powered-By"); pb == "" {
		t.Error("missing X-Powered-By header")
	}
}

func TestResultsSortedByID(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(env.Data, &items); err != nil {
		t.Fatalf("unmarshal items: %v", err)
	}

	ids := make([]int, len(items))
	for i, item := range items {
		ids[i] = int(item["id"].(float64))
	}

	if !sort.IntsAreSorted(ids) {
		t.Errorf("results not sorted by ID: %v", ids)
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// ?q=cloud on /api/net should match CloudNet and CloudySkies Corp
	// (case-insensitive search across name, aka, name_long, irr_as_set).
	req := httptest.NewRequest(http.MethodGet, "/api/net?q=cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 items matching 'cloud', got %d", len(data))
	}
}

func TestSearchEmpty(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// ?q= with empty value returns all results (no filtering).
	req := httptest.NewRequest(http.MethodGet, "/api/net?q=", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) != 3 {
		t.Errorf("expected 3 items (all), got %d", len(data))
	}
}

func TestSearchFacility(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second).UTC()

	// Create facilities with different names.
	for _, name := range []string{"Equinix NY5", "CoreSite LA1", "Equinix DA1"} {
		_, err := client.Facility.Create().
			SetName(name).
			SetCreated(now).
			SetUpdated(now).
			SetStatus("ok").
			Save(ctx)
		if err != nil {
			t.Fatalf("create facility %q: %v", name, err)
		}
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/fac?q=equinix", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 facilities matching 'equinix', got %d", len(data))
	}
}

func TestFieldProjection(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// ?fields=id,name on /api/net returns objects with only those fields.
	req := httptest.NewRequest(http.MethodGet, "/api/net?fields=id,name", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected at least 1 item")
	}

	for _, obj := range data {
		if _, ok := obj["id"]; !ok {
			t.Error("projected object missing 'id'")
		}
		if _, ok := obj["name"]; !ok {
			t.Error("projected object missing 'name'")
		}
		// Should NOT have fields like asn, status, etc.
		if _, ok := obj["asn"]; ok {
			t.Error("projected object should not have 'asn'")
		}
		if _, ok := obj["status"]; ok {
			t.Error("projected object should not have 'status'")
		}
	}
}

func TestFieldProjectionUnknown(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// ?fields=id,name,bogus: unknown field 'bogus' is silently ignored.
	req := httptest.NewRequest(http.MethodGet, "/api/net?fields=id,name,bogus", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var env testEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var data []map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected at least 1 item")
	}

	obj := data[0]
	if _, ok := obj["id"]; !ok {
		t.Error("projected object missing 'id'")
	}
	if _, ok := obj["name"]; !ok {
		t.Error("projected object missing 'name'")
	}
	if _, ok := obj["bogus"]; ok {
		t.Error("projected object should not have 'bogus'")
	}
}

func TestFieldProjectionWithDepth(t *testing.T) {
	t.Parallel()
	_, mux := setupDepthTestData(t)

	// Get list to find an org ID.
	req := httptest.NewRequest(http.MethodGet, "/api/org?limit=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var env testEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	var items []map[string]any
	_ = json.Unmarshal(env.Data, &items)
	orgID := int(items[0]["id"].(float64))

	// Detail with depth=2 and fields=id,name should project top-level
	// but _set objects should be unaffected.
	detReq := httptest.NewRequest(http.MethodGet,
		"/api/org/"+itoa(orgID)+"?depth=2&fields=id,name", nil)
	detRec := httptest.NewRecorder()
	mux.ServeHTTP(detRec, detReq)

	if detRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", detRec.Code, detRec.Body.String())
	}

	var detEnv testEnvelope
	_ = json.Unmarshal(detRec.Body.Bytes(), &detEnv)
	var detItems []map[string]any
	_ = json.Unmarshal(detEnv.Data, &detItems)
	if len(detItems) != 1 {
		t.Fatalf("expected 1 item, got %d", len(detItems))
	}

	obj := detItems[0]
	// id and name should be present.
	if _, ok := obj["id"]; !ok {
		t.Error("projected detail missing 'id'")
	}
	if _, ok := obj["name"]; !ok {
		t.Error("projected detail missing 'name'")
	}
	// _set fields should still be present (projection does not remove them).
	if _, ok := obj["net_set"]; !ok {
		t.Error("projected detail missing 'net_set' (should be preserved)")
	}
}

// itoa is a test helper to convert int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	if neg {
		result = "-" + result
	}
	return result
}
