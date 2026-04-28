package sync_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

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
				"id":                            1,
				"org_id":                        99,
				"name":                          "Orphan Net",
				"aka":                           "",
				"name_long":                     "",
				"website":                       "",
				"social_media":                  []any{},
				"asn":                           65001,
				"looking_glass":                 "",
				"route_server":                  "",
				"irr_as_set":                    "",
				"info_type":                     "",
				"info_types":                    []any{},
				"info_traffic":                  "",
				"info_ratio":                    "",
				"info_scope":                    "",
				"info_unicast":                  true,
				"info_multicast":                false,
				"info_ipv6":                     true,
				"info_never_via_route_servers":  false,
				"notes":                         "",
				"policy_url":                    "",
				"policy_general":                "",
				"policy_locations":              "",
				"policy_ratio":                  false,
				"policy_contracts":              "",
				"allow_ixp_update":              false,
				"ix_count":                      0,
				"fac_count":                     0,
				"created":                       "2026-04-01T00:00:00Z",
				"updated":                       "2026-04-01T00:00:00Z",
				"status":                        "ok",
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
		FKBackfillMaxPerCycle: 10,
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
		FKBackfillMaxPerCycle: 10,
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
				"id":                            1,
				"org_id":                        99,
				"name":                          "Orphan Net",
				"aka":                           "",
				"name_long":                     "",
				"website":                       "",
				"social_media":                  []any{},
				"asn":                           65001,
				"looking_glass":                 "",
				"route_server":                  "",
				"irr_as_set":                    "",
				"info_type":                     "",
				"info_types":                    []any{},
				"info_traffic":                  "",
				"info_ratio":                    "",
				"info_scope":                    "",
				"info_unicast":                  true,
				"info_multicast":                false,
				"info_ipv6":                     true,
				"info_never_via_route_servers":  false,
				"notes":                         "",
				"policy_url":                    "",
				"policy_general":                "",
				"policy_locations":              "",
				"policy_ratio":                  false,
				"policy_contracts":              "",
				"allow_ixp_update":              false,
				"ix_count":                      0,
				"fac_count":                     0,
				"created":                       "2026-04-01T00:00:00Z",
				"updated":                       "2026-04-01T00:00:00Z",
				"status":                        "ok",
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
		FKBackfillMaxPerCycle: 0, // disabled
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
		FKBackfillMaxPerCycle: 1, // cap → only 1 backfill allowed
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
		FKBackfillMaxPerCycle: 5,
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
		"id":                            id,
		"org_id":                        orgID,
		"name":                          fmt.Sprintf("Net %d", id),
		"aka":                           "",
		"name_long":                     "",
		"website":                       "",
		"social_media":                  []any{},
		"asn":                           65000 + id,
		"looking_glass":                 "",
		"route_server":                  "",
		"irr_as_set":                    "",
		"info_type":                     "",
		"info_types":                    []any{},
		"info_traffic":                  "",
		"info_ratio":                    "",
		"info_scope":                    "",
		"info_unicast":                  true,
		"info_multicast":                false,
		"info_ipv6":                     true,
		"info_never_via_route_servers":  false,
		"notes":                         "",
		"policy_url":                    "",
		"policy_general":                "",
		"policy_locations":              "",
		"policy_ratio":                  false,
		"policy_contracts":              "",
		"allow_ixp_update":              false,
		"ix_count":                      0,
		"fac_count":                     0,
		"created":                       "2026-04-01T00:00:00Z",
		"updated":                       "2026-04-01T00:00:00Z",
		"status":                        "ok",
	}
}
