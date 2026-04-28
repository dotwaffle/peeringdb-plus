package sync_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// orgJSON builds a minimal valid org JSON body for the backfill tests.
func orgJSON(id int, name, status string) json.RawMessage {
	out, err := json.Marshal(map[string]any{
		"id":           id,
		"name":         name,
		"aka":          "",
		"name_long":    "",
		"website":      "",
		"social_media": []any{},
		"notes":        "",
		"address1":     "",
		"address2":     "",
		"city":         "",
		"state":        "",
		"country":      "US",
		"zipcode":      "",
		"suite":        "",
		"floor":        "",
		"created":      "2026-04-01T00:00:00Z",
		"updated":      "2026-04-01T00:00:00Z",
		"status":       status,
	})
	if err != nil {
		panic(err)
	}
	return out
}

// TestFKCheckParent_BackfillIntegration is the end-to-end happy path:
// a child row references a parent that's missing locally and missing
// from the bulk fetch; the backfill path fetches and lands the parent.
//
// Setup:
//   - bulk /api/org returns empty (no orgs)
//   - bulk /api/net returns one network with org_id=99
//   - backfill /api/org?since=1&id__in=99 returns the parent
//
// Expected:
//   - org 99 lands in the local DB (status=upstream)
//   - net row lands (FK satisfied via backfill)
//   - exactly ONE backfill HTTP call (org)
func TestFKCheckParent_BackfillIntegration(t *testing.T) {
	t.Parallel()

	var backfillRequests atomic.Int32
	var bulkRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		// Backfill request shape: ?since=1&id__in=N
		if r.URL.Query().Get("id__in") != "" {
			backfillRequests.Add(1)
			if typeName == "org" && r.URL.Query().Get("id__in") == "99" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"meta":{},"data":[`))
				_, _ = w.Write(orgJSON(99, "Backfilled Org", "ok"))
				_, _ = w.Write([]byte(`]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		// Bulk request: skip != 0 → empty (terminate pagination)
		bulkRequests.Add(1)
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		// Provide one net referencing missing org 99; everything else empty.
		if typeName == "net" {
			netJSON := mustJSON(map[string]any{
				"id":                           1,
				"org_id":                       99,
				"name":                         "Orphan Net",
				"aka":                          "",
				"name_long":                    "",
				"website":                      "",
				"social_media":                 []any{},
				"asn":                          65001,
				"looking_glass":                "",
				"route_server":                 "",
				"irr_as_set":                   "",
				"info_type":                    "",
				"info_types":                   []any{},
				"info_traffic":                 "",
				"info_ratio":                   "",
				"info_scope":                   "",
				"info_unicast":                 true,
				"info_multicast":               false,
				"info_ipv6":                    true,
				"info_never_via_route_servers": false,
				"notes":                        "",
				"policy_url":                   "",
				"policy_general":               "",
				"policy_locations":             "",
				"policy_ratio":                 false,
				"policy_contracts":             "",
				"allow_ixp_update":             false,
				"ix_count":                     0,
				"fac_count":                    0,
				"created":                      "2026-04-01T00:00:00Z",
				"updated":                      "2026-04-01T00:00:00Z",
				"status":                       "ok",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(netJSON)
			_, _ = w.Write([]byte(`]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 10,
	}, slog.Default())

	ctx := t.Context()
	if err := w.Sync(ctx, "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Org 99 should now exist (backfilled).
	orgCount, err := client.Organization.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	if orgCount != 1 {
		t.Errorf("orgCount = %d, want 1 (backfill should have landed org 99)", orgCount)
	}
	org, err := client.Organization.Get(ctx, 99)
	if err != nil {
		t.Fatalf("get org 99: %v", err)
	}
	if org.Name != "Backfilled Org" {
		t.Errorf("org 99 name = %q, want Backfilled Org", org.Name)
	}

	// Net 1 should exist (FK now satisfied via backfilled parent).
	netCount, err := client.Network.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count nets: %v", err)
	}
	if netCount != 1 {
		t.Errorf("netCount = %d, want 1 (FK should be satisfied via backfill)", netCount)
	}

	// Exactly ONE backfill HTTP call (id__in=99).
	if got := backfillRequests.Load(); got != 1 {
		t.Errorf("backfillRequests = %d, want 1 (no dedup pressure with single child)", got)
	}
}

// TestFKCheckParent_BackfillDedup: two child rows referencing the same
// missing parent → exactly ONE backfill HTTP call (the per-cycle
// fkBackfillTried map short-circuits the second).
func TestFKCheckParent_BackfillDedup(t *testing.T) {
	t.Parallel()

	var backfillRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id__in") != "" {
			backfillRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(orgJSON(99, "Shared Parent", "ok"))
			_, _ = w.Write([]byte(`]}`))
			return
		}
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if typeName == "net" {
			// Two nets, same missing parent org 99.
			net1 := mustJSON(makeMinimalNet(1, 99))
			net2 := mustJSON(makeMinimalNet(2, 99))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(net1)
			_, _ = w.Write([]byte(`,`))
			_, _ = w.Write(net2)
			_, _ = w.Write([]byte(`]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 10,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if got := backfillRequests.Load(); got != 1 {
		t.Errorf("backfillRequests = %d, want 1 (dedup cache should suppress duplicate fetches)", got)
	}
	netCount, _ := client.Network.Query().Count(t.Context())
	if netCount != 2 {
		t.Errorf("netCount = %d, want 2 (both nets land via shared backfilled parent)", netCount)
	}
}

// TestFKCheckParent_BackfillCapZeroDisablesBackfill: cap=0 means no
// backfill is attempted; missing parents drop child rows as before.
func TestFKCheckParent_BackfillCapZeroDisablesBackfill(t *testing.T) {
	t.Parallel()

	var backfillRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id__in") != "" {
			backfillRequests.Add(1)
			// Even if asked, we'd reject — but it shouldn't be asked.
		}
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if typeName == "net" {
			netJSON := mustJSON(map[string]any{
				"id":                           1,
				"org_id":                       99,
				"name":                         "Orphan Net",
				"aka":                          "",
				"name_long":                    "",
				"website":                      "",
				"social_media":                 []any{},
				"asn":                          65001,
				"looking_glass":                "",
				"route_server":                 "",
				"irr_as_set":                   "",
				"info_type":                    "",
				"info_types":                   []any{},
				"info_traffic":                 "",
				"info_ratio":                   "",
				"info_scope":                   "",
				"info_unicast":                 true,
				"info_multicast":               false,
				"info_ipv6":                    true,
				"info_never_via_route_servers": false,
				"notes":                        "",
				"policy_url":                   "",
				"policy_general":               "",
				"policy_locations":             "",
				"policy_ratio":                 false,
				"policy_contracts":             "",
				"allow_ixp_update":             false,
				"ix_count":                     0,
				"fac_count":                    0,
				"created":                      "2026-04-01T00:00:00Z",
				"updated":                      "2026-04-01T00:00:00Z",
				"status":                       "ok",
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(netJSON)
			_, _ = w.Write([]byte(`]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 0, // disabled
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if got := backfillRequests.Load(); got != 0 {
		t.Errorf("backfillRequests = %d, want 0 (cap=0 disables backfill)", got)
	}
	netCount, _ := client.Network.Query().Count(t.Context())
	if netCount != 0 {
		t.Errorf("netCount = %d, want 0 (cap=0 → drop on FK miss)", netCount)
	}
}

// TestFKCheckParent_BackfillCapHitRecordsRatelimited: with cap=1 and 2
// distinct missing parents, the SECOND backfill attempt fires the
// ratelimited path (logged WARN, no HTTP call).
func TestFKCheckParent_BackfillCapHitRecordsRatelimited(t *testing.T) {
	t.Parallel()

	var backfillRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id__in") != "" {
			backfillRequests.Add(1)
			// Always return a row so the FIRST cap slot succeeds.
			id := r.URL.Query().Get("id__in")
			var n int
			_, _ = fmt.Sscanf(id, "%d", &n)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(orgJSON(n, fmt.Sprintf("Recovered %d", n), "ok"))
			_, _ = w.Write([]byte(`]}`))
			return
		}
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if typeName == "net" {
			// Two nets referencing two distinct missing orgs.
			net1 := mustJSON(makeMinimalNet(1, 100))
			net2 := mustJSON(makeMinimalNet(2, 200))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(net1)
			_, _ = w.Write([]byte(`,`))
			_, _ = w.Write(net2)
			_, _ = w.Write([]byte(`]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 1, // cap → only 1 backfill allowed
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Exactly ONE backfill HTTP call (cap was 1).
	if got := backfillRequests.Load(); got != 1 {
		t.Errorf("backfillRequests = %d, want 1 (cap=1)", got)
	}
	// Only one of {org 100, org 200} should have landed.
	orgCount, _ := client.Organization.Query().Count(t.Context())
	if orgCount != 1 {
		t.Errorf("orgCount = %d, want 1 (cap allowed only one backfill)", orgCount)
	}
	// One net dropped (cap-blocked), one net survived (backfill won).
	netCount, _ := client.Network.Query().Count(t.Context())
	if netCount != 1 {
		t.Errorf("netCount = %d, want 1 (one nett survived, one dropped)", netCount)
	}
}

// TestFKCheckParent_BackfillFetchErrorRecordsError: backfill HTTP fetch
// fails with 5xx → result=error, child row dropped.
func TestFKCheckParent_BackfillFetchErrorRecordsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id__in") != "" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if typeName == "net" {
			netJSON := mustJSON(makeMinimalNet(1, 99))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(netJSON)
			_, _ = w.Write([]byte(`]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 5,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	netCount, _ := client.Network.Query().Count(t.Context())
	if netCount != 0 {
		t.Errorf("netCount = %d, want 0 (backfill HTTP error → child dropped)", netCount)
	}
}

// TestFetchRaw_PassesQueryParams asserts that FetchRaw assembles the URL
// with the given url.Values and decodes the response data array.
func TestFetchRaw_PassesQueryParams(t *testing.T) {
	t.Parallel()

	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[{"id":42,"name":"x"}]}`))
	}))
	defer server.Close()

	client := peeringdb.NewClient(server.URL, slog.Default())
	client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	client.SetRetryBaseDelay(0)

	q := url.Values{}
	q.Set("since", "1")
	q.Set("id__in", "42")
	raws, err := client.FetchRaw(t.Context(), "org", q)
	if err != nil {
		t.Fatalf("FetchRaw: %v", err)
	}
	if len(raws) != 1 {
		t.Fatalf("len(raws) = %d, want 1", len(raws))
	}
	if !strings.Contains(capturedURL, "since=1") || !strings.Contains(capturedURL, "id__in=42") {
		t.Errorf("URL missing expected params: %s", capturedURL)
	}
	// Response cloned correctly.
	var v struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raws[0], &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v.ID != 42 || v.Name != "x" {
		t.Errorf("decoded = %+v, want {42, x}", v)
	}
}

// TestFKCheckParent_BackfillRecursesIntoGrandparent (v1.18.3): when
// the backfilled parent's OWN parent FK is missing, recursive backfill
// chains into the grandparent before upserting the parent. Concrete
// scenario from production: carrierfac → carrier → org. The bulk
// fetch returns a carrierfac whose carrier_id and fac_id are absent
// locally; backfill of carrier 403 reveals it references org 18985
// which is also absent; org 18985 is recursively backfilled.
//
// Expected:
//   - org 18985 lands (recursive grandparent backfill)
//   - carrier 403 lands (parent backfill, FK now satisfied)
//   - facility 500 lands (parallel parent backfill)
//   - carrierfac 1 lands (FK satisfied via both backfills)
//   - 3 backfill HTTP calls: carrier 403, fac 500, org 18985 (each fired exactly once via dedup)
func TestFKCheckParent_BackfillRecursesIntoGrandparent(t *testing.T) {
	t.Parallel()

	var backfillCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		w.Header().Set("Content-Type", "application/json")

		// Backfill request: ?since=1&id__in=N
		if id := r.URL.Query().Get("id__in"); id != "" {
			backfillCount.Add(1)
			switch {
			case typeName == "carrier" && id == "403":
				_, _ = w.Write([]byte(`{"meta":{},"data":[`))
				_, _ = w.Write(mustJSON(map[string]any{
					"id": 403, "org_id": 18985, "name": "NTT America, Inc.",
					"aka": "", "name_long": "", "website": "", "social_media": []any{},
					"notes": "", "fac_count": 0, "logo": nil,
					"created": "2024-02-08T20:39:07Z", "updated": "2024-02-08T21:44:01Z",
					"status": "ok",
				}))
				_, _ = w.Write([]byte(`]}`))
				return
			case typeName == "org" && id == "18985":
				_, _ = w.Write([]byte(`{"meta":{},"data":[`))
				_, _ = w.Write(orgJSON(18985, "NTT America, Inc.", "deleted"))
				_, _ = w.Write([]byte(`]}`))
				return
			case typeName == "fac" && id == "500":
				_, _ = w.Write([]byte(`{"meta":{},"data":[`))
				_, _ = w.Write(mustJSON(map[string]any{
					"id": 500, "org_id": 18985, "name": "NTT Facility",
					"aka": "", "address1": "", "address2": "", "city": "",
					"clli": "", "country": "US", "diverse_serving_substations": false,
					"floor": "", "geocode_country": "", "geocode_date": nil,
					"latitude": nil, "longitude": nil, "name_long": "", "notes": "",
					"npanxx": "", "rencode": "", "state": "", "suite": "",
					"sales_email": "", "sales_phone": "", "social_media": []any{},
					"available_voltage_services": []string{},
					"region_continent":           "", "tech_email": "", "tech_phone": "",
					"website": "", "zipcode": "", "campus_id": nil, "property": "",
					"status_dashboard": "", "ix_count": 0, "net_count": 0, "carrier_count": 0,
					"created": "2024-02-08T20:39:07Z", "updated": "2024-02-08T21:44:01Z",
					"status": "ok",
				}))
				_, _ = w.Write([]byte(`]}`))
				return
			}
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		// Bulk fetch: skip != 0 → empty (terminate pagination).
		if skip := r.URL.Query().Get("skip"); skip != "" && skip != "0" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		// One carrierfac referencing missing carrier 403 + missing fac 500.
		// Org and carrier and fac bulk paths return empty → triggers backfill.
		if typeName == "carrierfac" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(mustJSON(map[string]any{
				"id": 1, "carrier_id": 403, "fac_id": 500,
				"name":    "NTT @ NTT Facility",
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}))
			_, _ = w.Write([]byte(`]}`))
			return
		}
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 10,
	}, slog.Default())

	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Recursive grandparent landed.
	if n, _ := client.Organization.Query().Count(t.Context()); n != 1 {
		t.Errorf("orgCount = %d, want 1 (recursive grandparent backfill)", n)
	}
	// Direct parents landed.
	if n, _ := client.Carrier.Query().Count(t.Context()); n != 1 {
		t.Errorf("carrierCount = %d, want 1 (parent backfill)", n)
	}
	if n, _ := client.Facility.Query().Count(t.Context()); n != 1 {
		t.Errorf("facCount = %d, want 1 (parent backfill)", n)
	}
	// Original child landed (FK chain satisfied).
	if n, _ := client.CarrierFacility.Query().Count(t.Context()); n != 1 {
		t.Errorf("carrierfacCount = %d, want 1 (FK chain satisfied via recursive backfill)", n)
	}
	// Exactly 3 backfill HTTP calls: carrier 403, fac 500, org 18985.
	// (Org dedup means the org is fetched ONCE even though both
	// carrier 403 and fac 500 reference it.)
	if got := backfillCount.Load(); got != 3 {
		t.Errorf("backfillCount = %d, want 3 (carrier+fac+org via recursive dedup)", got)
	}
}

// TestFKCheckParent_BackfillDeadlineFallsBackToDrop (v1.18.3): the
// per-cycle backfill deadline short-circuits fkBackfillParent to drop-
// on-miss. Sets a 1ns deadline so the very first backfill attempt
// trips it; verifies result=deadline_exceeded and the orphan child is
// dropped (no fetch issued, no parent inserted).
func TestFKCheckParent_BackfillDeadlineFallsBackToDrop(t *testing.T) {
	t.Parallel()

	var backfillCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("id__in") != "" {
			backfillCount.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if skip := r.URL.Query().Get("skip"); skip != "" && skip != "0" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if typeName == "net" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[`))
			_, _ = w.Write(mustJSON(makeMinimalNet(1, 99)))
			_, _ = w.Write([]byte(`]}`))
			return
		}
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	// 1ns timeout — first backfill attempt fires past deadline.
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: 10,
		FKBackfillTimeout:     1 * time.Nanosecond,
	}, slog.Default())

	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if got := backfillCount.Load(); got != 0 {
		t.Errorf("backfillCount = %d, want 0 (deadline check fires before fetch)", got)
	}
	if n, _ := client.Organization.Query().Count(t.Context()); n != 0 {
		t.Errorf("orgCount = %d, want 0 (deadline → drop, no parent inserted)", n)
	}
	if n, _ := client.Network.Query().Count(t.Context()); n != 0 {
		t.Errorf("netCount = %d, want 0 (deadline → drop child too)", n)
	}
}

// helpers

func mustJSON(v any) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return out
}

func makeMinimalNet(id, orgID int) map[string]any {
	return map[string]any{
		"id":                           id,
		"org_id":                       orgID,
		"name":                         fmt.Sprintf("Net %d", id),
		"aka":                          "",
		"name_long":                    "",
		"website":                      "",
		"social_media":                 []any{},
		"asn":                          65000 + id,
		"looking_glass":                "",
		"route_server":                 "",
		"irr_as_set":                   "",
		"info_type":                    "",
		"info_types":                   []any{},
		"info_traffic":                 "",
		"info_ratio":                   "",
		"info_scope":                   "",
		"info_unicast":                 true,
		"info_multicast":               false,
		"info_ipv6":                    true,
		"info_never_via_route_servers": false,
		"notes":                        "",
		"policy_url":                   "",
		"policy_general":               "",
		"policy_locations":             "",
		"policy_ratio":                 false,
		"policy_contracts":             "",
		"allow_ixp_update":             false,
		"ix_count":                     0,
		"fac_count":                    0,
		"created":                      "2026-04-01T00:00:00Z",
		"updated":                      "2026-04-01T00:00:00Z",
		"status":                       "ok",
	}
}

// makeMinimalCarrier returns a JSON-friendly carrier row referencing
// the given org_id. Mirrors the shape used by the existing recursive
// grandparent test for parity.
func makeMinimalCarrier(id, orgID int) map[string]any {
	return map[string]any{
		"id":           id,
		"org_id":       orgID,
		"name":         fmt.Sprintf("Carrier %d", id),
		"aka":          "",
		"name_long":    "",
		"website":      "",
		"social_media": []any{},
		"notes":        "",
		"fac_count":    0,
		"logo":         nil,
		"created":      "2026-04-01T00:00:00Z",
		"updated":      "2026-04-01T00:00:00Z",
		"status":       "ok",
	}
}

// makeMinimalFac returns a JSON-friendly facility row referencing the
// given org_id. Field set matches what entgo's facility schema requires
// after sync's upsert pipeline; the zero values for optional columns
// satisfy NOT NULL constraints via setter defaults.
func makeMinimalFac(id, orgID int) map[string]any {
	return map[string]any{
		"id":                          id,
		"org_id":                      orgID,
		"name":                        fmt.Sprintf("Fac %d", id),
		"aka":                         "",
		"address1":                    "",
		"address2":                    "",
		"city":                        "",
		"clli":                        "",
		"country":                     "US",
		"diverse_serving_substations": false,
		"floor":                       "",
		"geocode_country":             "",
		"geocode_date":                nil,
		"latitude":                    nil,
		"longitude":                   nil,
		"name_long":                   "",
		"notes":                       "",
		"npanxx":                      "",
		"rencode":                     "",
		"state":                       "",
		"suite":                       "",
		"sales_email":                 "",
		"sales_phone":                 "",
		"social_media":                []any{},
		"available_voltage_services":  []string{},
		"region_continent":            "",
		"tech_email":                  "",
		"tech_phone":                  "",
		"website":                     "",
		"zipcode":                     "",
		"campus_id":                   nil,
		"property":                    "",
		"status_dashboard":            "",
		"ix_count":                    0,
		"net_count":                   0,
		"carrier_count":               0,
		"created":                     "2026-04-01T00:00:00Z",
		"updated":                     "2026-04-01T00:00:00Z",
		"status":                      "ok",
	}
}

// makeMinimalCarrierFac returns a JSON-friendly carrierfac row.
func makeMinimalCarrierFac(id, carrierID, facID int) map[string]any {
	return map[string]any{
		"id":         id,
		"carrier_id": carrierID,
		"fac_id":     facID,
		"name":       fmt.Sprintf("CF %d", id),
		"created":    "2026-04-01T00:00:00Z",
		"updated":    "2026-04-01T00:00:00Z",
		"status":     "ok",
	}
}

// batchedFetchRecorder captures backfill request URLs (the ones with
// ?id__in= set). Used by the TestFKBackfill_BatchedFetch_* tests to
// assert HTTP-call counts and per-call id__in cardinality.
type batchedFetchRecorder struct {
	mu     stdsync.Mutex
	calls  atomic.Int32
	idIns  []string // id__in CSV values, one per recorded backfill request
	byType map[string][]string
}

func newBatchedFetchRecorder() *batchedFetchRecorder {
	return &batchedFetchRecorder{byType: make(map[string][]string)}
}

func (r *batchedFetchRecorder) record(typeName, idIn string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls.Add(1)
	r.idIns = append(r.idIns, idIn)
	r.byType[typeName] = append(r.byType[typeName], idIn)
}

func (r *batchedFetchRecorder) snapshot() (calls int, idIns []string, byType map[string][]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.idIns))
	copy(out, r.idIns)
	bt := make(map[string][]string, len(r.byType))
	for k, v := range r.byType {
		cp := make([]string, len(v))
		copy(cp, v)
		bt[k] = cp
	}
	return int(r.calls.Load()), out, bt
}

// newBatchedTestServer builds an httptest.Server that:
//   - Records every backfill request (id__in set) into the recorder.
//   - For backfill requests, parses id__in CSV and synthesises one
//     row per ID via the per-type rowFn.
//   - For bulk requests (no id__in), returns the per-type bulkData
//     once on skip=0 and an empty page thereafter.
func newBatchedTestServer(
	tb testing.TB,
	rec *batchedFetchRecorder,
	bulkData map[string][]json.RawMessage,
	rowFn map[string]func(id int) json.RawMessage,
) *httptest.Server {
	tb.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		w.Header().Set("Content-Type", "application/json")
		if idIn := r.URL.Query().Get("id__in"); idIn != "" {
			rec.record(typeName, idIn)
			fn, ok := rowFn[typeName]
			if !ok {
				_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
				return
			}
			parts := strings.Split(idIn, ",")
			items := make([]json.RawMessage, 0, len(parts))
			for _, p := range parts {
				var id int
				if _, err := fmt.Sscanf(p, "%d", &id); err != nil {
					continue
				}
				items = append(items, fn(id))
			}
			body, _ := json.Marshal(map[string]any{"meta": map[string]any{}, "data": items})
			_, _ = w.Write(body)
			return
		}
		// Bulk path.
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		data, ok := bulkData[typeName]
		if !ok {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		body, _ := json.Marshal(map[string]any{"meta": map[string]any{}, "data": data})
		_, _ = w.Write(body)
	}))
}

// TestFKBackfill_BatchedFetch_OneRequest: a single chunk of 50 nets
// each referencing a unique missing org_id collapses to ONE batched
// HTTP request via the chunk pre-pass. URL shape MUST be
// ?since=1&id__in=101,102,…,150 (sorted ascending — the pre-pass uses
// slices.Sorted(maps.Keys(...)) for deterministic ordering).
func TestFKBackfill_BatchedFetch_OneRequest(t *testing.T) {
	t.Parallel()

	const numNets = 50
	const orgIDBase = 100 // org_ids 101..150

	nets := make([]json.RawMessage, 0, numNets)
	for i := 1; i <= numNets; i++ {
		nets = append(nets, mustJSON(makeMinimalNet(i, orgIDBase+i)))
	}

	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{
			"net": nets,
		},
		map[string]func(int) json.RawMessage{
			"org": func(id int) json.RawMessage {
				return orgJSON(id, fmt.Sprintf("Org %d", id), "ok")
			},
		},
	)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: numNets * 2,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// All orgs landed via batched backfill.
	if got, _ := client.Organization.Query().Count(t.Context()); got != numNets {
		t.Errorf("orgCount = %d, want %d (batched backfill)", got, numNets)
	}
	// All nets landed (FK satisfied).
	if got, _ := client.Network.Query().Count(t.Context()); got != numNets {
		t.Errorf("netCount = %d, want %d", got, numNets)
	}
	// Exactly ONE backfill HTTP call for the org parent type.
	calls, idIns, byType := rec.snapshot()
	if calls != 1 {
		t.Fatalf("backfill HTTP calls = %d, want 1 (50 distinct misses → 1 batched fetch)", calls)
	}
	if got, want := len(byType["org"]), 1; got != want {
		t.Errorf("backfill calls to /api/org = %d, want %d", got, want)
	}
	// id__in must be sorted ascending (slices.Sorted invariant).
	parts := strings.Split(idIns[0], ",")
	if len(parts) != numNets {
		t.Fatalf("len(id__in) = %d, want %d", len(parts), numNets)
	}
	for i, p := range parts {
		want := fmt.Sprintf("%d", orgIDBase+i+1)
		if p != want {
			t.Errorf("id__in[%d] = %q, want %q (sorted ascending)", i, p, want)
			break
		}
	}
}

// TestFKBackfill_BatchedFetch_ChunksAt100: 250 distinct missing parent
// IDs spread across 3 sync chunks (scratchChunkSize=100) trigger
// exactly 3 batched HTTP requests, one per chunk, with id__in
// cardinalities of 100, 100, 50.
func TestFKBackfill_BatchedFetch_ChunksAt100(t *testing.T) {
	t.Parallel()

	const numNets = 250
	nets := make([]json.RawMessage, 0, numNets)
	for i := 1; i <= numNets; i++ {
		// Unique org_id per net so the pre-pass observes 250 distinct misses.
		nets = append(nets, mustJSON(makeMinimalNet(i, 1000+i)))
	}

	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{"net": nets},
		map[string]func(int) json.RawMessage{
			"org": func(id int) json.RawMessage {
				return orgJSON(id, fmt.Sprintf("Org %d", id), "ok")
			},
		},
	)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: numNets * 2,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	calls, idIns, byType := rec.snapshot()
	if got := len(byType["org"]); got != 3 {
		t.Fatalf("backfill calls to /api/org = %d, want 3 (250 → 3 chunks of 100/100/50)", got)
	}
	if calls != 3 {
		t.Errorf("total backfill calls = %d, want 3", calls)
	}
	wantSizes := []int{100, 100, 50}
	for i, want := range wantSizes {
		got := len(strings.Split(idIns[i], ","))
		if got != want {
			t.Errorf("call[%d] id__in cardinality = %d, want %d", i, got, want)
		}
	}
	if got, _ := client.Organization.Query().Count(t.Context()); got != numNets {
		t.Errorf("orgCount = %d, want %d", got, numNets)
	}
	if got, _ := client.Network.Query().Count(t.Context()); got != numNets {
		t.Errorf("netCount = %d, want %d", got, numNets)
	}
}

// TestFKBackfill_BatchedFetch_RecursiveGrandparents: 50 carrierfacs
// each referencing a unique missing carrier (carrier 1..50) and a
// shared existing fac. Each carrier (when fetched) references a unique
// missing org (org 1001..1050). The chunk pre-pass collapses the
// carrier+org chain into exactly TWO batched HTTP requests — one for
// the 50 carriers, one recursive for the 50 orgs — instead of 100
// per-row HTTP calls under the legacy fkBackfillParent path.
func TestFKBackfill_BatchedFetch_RecursiveGrandparents(t *testing.T) {
	t.Parallel()

	const numCFs = 50
	const sharedFacID = 999
	const sharedFacOrgID = 9999

	// One pre-existing org (the fac's parent — keeps the fac valid in
	// the bulk path so the carrierfac chunk's pre-pass only needs to
	// chase carriers, not facs).
	bulkOrgs := []json.RawMessage{orgJSON(sharedFacOrgID, "Existing Org", "ok")}
	bulkFacs := []json.RawMessage{mustJSON(makeMinimalFac(sharedFacID, sharedFacOrgID))}

	cfs := make([]json.RawMessage, 0, numCFs)
	for i := 1; i <= numCFs; i++ {
		cfs = append(cfs, mustJSON(makeMinimalCarrierFac(i, i, sharedFacID)))
	}

	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{
			"org":        bulkOrgs,
			"fac":        bulkFacs,
			"carrierfac": cfs,
		},
		map[string]func(int) json.RawMessage{
			"carrier": func(id int) json.RawMessage {
				return mustJSON(makeMinimalCarrier(id, 1000+id))
			},
			"org": func(id int) json.RawMessage {
				return orgJSON(id, fmt.Sprintf("Org %d", id), "ok")
			},
		},
	)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: numCFs * 4,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	calls, _, byType := rec.snapshot()
	if calls != 2 {
		t.Fatalf("total backfill calls = %d, want 2 (carriers, orgs)", calls)
	}
	if got := len(byType["carrier"]); got != 1 {
		t.Errorf("carrier backfill calls = %d, want 1", got)
	}
	if got := len(byType["org"]); got != 1 {
		t.Errorf("org backfill calls = %d, want 1 (recursive grandparent)", got)
	}
	// Verify everything landed.
	if got, _ := client.Organization.Query().Count(t.Context()); got != numCFs+1 {
		t.Errorf("orgCount = %d, want %d (1 bulk org + %d recursive grandparents)", got, numCFs+1, numCFs)
	}
	if got, _ := client.Carrier.Query().Count(t.Context()); got != numCFs {
		t.Errorf("carrierCount = %d, want %d (parent backfill)", got, numCFs)
	}
	if got, _ := client.Facility.Query().Count(t.Context()); got != 1 {
		t.Errorf("facCount = %d, want 1 (bulk-loaded)", got)
	}
	if got, _ := client.CarrierFacility.Query().Count(t.Context()); got != numCFs {
		t.Errorf("carrierfacCount = %d, want %d (FK chain satisfied)", got, numCFs)
	}
}

// TestFKBackfill_BatchedFetch_RespectsCap: cap=1 HTTP request, 250
// distinct missing parents → exactly ONE batched HTTP request for the
// first 100 IDs (one chunk = one request budget unit), the other 150
// recorded as fkBackfillRateLimited via the cap pre-flight in
// fkBackfillBatch.
//
// v1.18.5 semantic: cap is now on UNDERLYING HTTP REQUESTS, not rows.
// One FetchByIDs(N) call issues ⌈N/peeringdb.FetchByIDsBatchSize⌉
// underlying HTTP requests. With BatchSize=100 and cap=1, the worker
// can fetch up to 100 IDs (1 request); IDs 101-250 fall through to
// drop-on-miss with result=ratelimited.
func TestFKBackfill_BatchedFetch_RespectsCap(t *testing.T) {
	t.Parallel()

	const numNets = 250
	const requestCap = 1
	const expectedFetchedRows = peeringdb.FetchByIDsBatchSize * requestCap // = 100
	nets := make([]json.RawMessage, 0, numNets)
	for i := 1; i <= numNets; i++ {
		nets = append(nets, mustJSON(makeMinimalNet(i, 1000+i)))
	}

	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{"net": nets},
		map[string]func(int) json.RawMessage{
			"org": func(id int) json.RawMessage {
				return orgJSON(id, fmt.Sprintf("Org %d", id), "ok")
			},
		},
	)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: requestCap,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	calls, idIns, _ := rec.snapshot()
	if calls != requestCap {
		t.Fatalf("backfill HTTP calls = %d, want %d (cap is on requests, not rows)", calls, requestCap)
	}
	gotN := len(strings.Split(idIns[0], ","))
	if gotN != expectedFetchedRows {
		t.Errorf("id__in cardinality = %d, want %d (one request × FetchByIDsBatchSize IDs)", gotN, expectedFetchedRows)
	}
	if got, _ := client.Organization.Query().Count(t.Context()); got != expectedFetchedRows {
		t.Errorf("orgCount = %d, want %d (only cap×batch_size parents fetched)", got, expectedFetchedRows)
	}
	if got, _ := client.Network.Query().Count(t.Context()); got != expectedFetchedRows {
		t.Errorf("netCount = %d, want %d (only cap×batch_size nets had FK satisfied)", got, expectedFetchedRows)
	}
}

// TestFKBackfill_BatchedFetch_RespectsDeadline: a deadline already in
// the past → ZERO HTTP requests; all missing IDs are recorded as
// fkBackfillDeadlineExceeded via the pre-flight check in
// fkBackfillBatch.
func TestFKBackfill_BatchedFetch_RespectsDeadline(t *testing.T) {
	t.Parallel()

	const numNets = 50
	nets := make([]json.RawMessage, 0, numNets)
	for i := 1; i <= numNets; i++ {
		nets = append(nets, mustJSON(makeMinimalNet(i, 1000+i)))
	}

	rec := newBatchedFetchRecorder()
	server := newBatchedTestServer(t, rec,
		map[string][]json.RawMessage{"net": nets},
		map[string]func(int) json.RawMessage{
			"org": func(id int) json.RawMessage {
				return orgJSON(id, fmt.Sprintf("Org %d", id), "ok")
			},
		},
	)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init: %v", err)
	}
	// 1ns timeout — pre-flight deadline check fires before any HTTP.
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{
		FKBackfillMaxRequestsPerCycle: numNets * 2,
		FKBackfillTimeout:     1 * time.Nanosecond,
	}, slog.Default())
	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if calls, _, _ := rec.snapshot(); calls != 0 {
		t.Errorf("backfill HTTP calls = %d, want 0 (deadline pre-flight short-circuits)", calls)
	}
	if got, _ := client.Organization.Query().Count(t.Context()); got != 0 {
		t.Errorf("orgCount = %d, want 0 (deadline → no inserts)", got)
	}
	if got, _ := client.Network.Query().Count(t.Context()); got != 0 {
		t.Errorf("netCount = %d, want 0 (deadline → drop all children)", got)
	}
}
