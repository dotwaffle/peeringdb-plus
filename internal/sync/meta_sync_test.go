package sync_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// metaTestServer serves one data array per type from rows, which the test
// swaps between cycles, and records which types were fetched with ?since=.
type metaTestServer struct {
	server *httptest.Server
	rows   atomic.Pointer[map[string][]any]
	since  map[string]*atomic.Bool
}

func newMetaTestServer(t *testing.T, rows map[string][]any) *metaTestServer {
	t.Helper()
	s := &metaTestServer{since: make(map[string]*atomic.Bool)}
	for _, typ := range []string{"org", "net", "ix", "ixlan", "netixlan"} {
		s.since[typ] = &atomic.Bool{}
	}
	s.rows.Store(&rows)
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typ := strings.TrimPrefix(r.URL.Path, "/api/")
		w.Header().Set("Content-Type", "application/json")
		if skip := r.URL.Query().Get("skip"); skip != "" && skip != "0" {
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if flag, ok := s.since[typ]; ok && r.URL.Query().Get("since") != "" {
			flag.Store(true)
		}
		data, ok := (*s.rows.Load())[typ]
		if !ok {
			data = []any{}
		}
		_, _ = w.Write([]byte(`{"meta":{},"data":` + string(mustJSON(data)) + `}`))
	}))
	t.Cleanup(s.server.Close)
	return s
}

// metaTestNet returns a net row. meta == nil leaves the key out, the
// shape upstream sent before 2.83.0.
func metaTestNet(id, asn int, updated string, meta map[string]any) map[string]any {
	row := map[string]any{
		"id": id, "org_id": 1, "name": "N", "aka": "", "name_long": "",
		"website": "", "social_media": []any{}, "asn": asn,
		"looking_glass": "", "route_server": "", "irr_as_set": "",
		"info_type": "", "info_types": []any{},
		"info_traffic": "", "info_ratio": "", "info_scope": "",
		"info_unicast": true, "info_multicast": false, "info_ipv6": true,
		"info_never_via_route_servers": false, "notes": "",
		"policy_url": "", "policy_general": "", "policy_locations": "",
		"policy_ratio": false, "policy_contracts": "", "allow_ixp_update": false,
		"ix_count": 0, "fac_count": 0,
		"created": "2026-04-01T00:00:00Z", "updated": updated,
		"status": "ok",
	}
	if meta != nil {
		row["meta"] = meta
	}
	return row
}

func metaTestNetIxLan(updated string, meta map[string]any) map[string]any {
	return map[string]any{
		"id": 10, "net_id": 1, "ix_id": 1, "ixlan_id": 1,
		"name": "", "notes": "", "speed": 10000, "asn": 65001,
		"ipaddr4": "192.0.2.1", "ipaddr6": nil,
		"is_rs_peer": false, "bfd_support": false, "operational": true,
		"net_side_id": nil, "ix_side_id": nil, "meta": meta,
		"created": "2026-04-01T00:00:00Z", "updated": updated,
		"status": "ok",
	}
}

// metaTestRows returns the parent rows every cycle serves unchanged.
func metaTestRows() map[string][]any {
	return map[string][]any{
		"org": {map[string]any{
			"id": 1, "name": "Org1", "aka": "", "name_long": "",
			"website": "", "social_media": []any{}, "notes": "",
			"address1": "", "address2": "", "city": "", "state": "", "country": "US",
			"zipcode": "", "suite": "", "floor": "",
			"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
			"status": "ok",
		}},
		"ix": {map[string]any{
			"id": 1, "org_id": 1, "name": "IX1", "aka": "", "name_long": "",
			"city": "", "country": "US", "region_continent": "",
			"media": "Ethernet", "notes": "",
			"proto_unicast": true, "proto_multicast": false, "proto_ipv6": true,
			"website": "", "social_media": []any{}, "url_stats": "",
			"tech_email": "", "tech_phone": "",
			"policy_email": "", "policy_phone": "",
			"sales_email": "", "sales_phone": "",
			"net_count": 1, "fac_count": 0, "ixf_net_count": 0,
			"ixf_last_import": nil, "ixf_import_request": nil,
			"ixf_import_request_status": "", "service_level": "", "terms": "",
			"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
			"status": "ok",
		}},
		"ixlan": {map[string]any{
			"id": 1, "ix_id": 1, "name": "L1", "descr": "",
			"mtu": 9000, "dot1q_support": false, "rs_asn": 65500,
			"arp_sponge": nil, "ixf_ixp_member_list_url_visible": "Public",
			"ixf_ixp_import_enabled": true,
			"created":                "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
			"status": "ok",
		}},
	}
}

// TestSync_MetaRoundTrip locks the sync side of the PeeringDB 2.83.0 meta
// document (migration 0159): the stored document equals the upstream one,
// an absent key stores no document, and a later ?since= row replaces the
// whole stored document instead of merging keys into it.
func TestSync_MetaRoundTrip(t *testing.T) {
	t.Parallel()

	netMeta := map[string]any{"preferred_ip_mtu": float64(9000), "rtbh_community": "65001:666"}
	plan := map[string]any{
		"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-31"},
		"rfc8950":               true,
	}
	rows := metaTestRows()
	rows["net"] = []any{
		metaTestNet(1, 65001, "2026-04-01T00:00:00Z", netMeta),
		metaTestNet(2, 65002, "2026-04-01T00:00:00Z", nil),
	}
	rows["netixlan"] = []any{metaTestNetIxLan("2026-04-01T00:00:00Z", plan)}
	srv := newMetaTestServer(t, rows)

	client, db := testutil.SetupClientWithDB(t)
	pdbClient := peeringdb.NewClient(srv.server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	ctx := t.Context()

	// Cycle 1: empty tables, so the bare lists are fetched.
	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	net1, err := client.Network.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get net 1: %v", err)
	}
	if diff := cmp.Diff(netMeta, net1.Meta); diff != "" {
		t.Errorf("net 1 meta (-want +got):\n%s", diff)
	}
	net2, err := client.Network.Get(ctx, 2)
	if err != nil {
		t.Fatalf("get net 2: %v", err)
	}
	if net2.Meta != nil {
		t.Errorf("net 2 meta = %v, want nil (upstream sent no meta key)", net2.Meta)
	}
	nixl, err := client.NetworkIxLan.Get(ctx, 10)
	if err != nil {
		t.Fatalf("get netixlan 10: %v", err)
	}
	if diff := cmp.Diff(plan, nixl.Meta); diff != "" {
		t.Errorf("netixlan 10 meta (-want +got):\n%s", diff)
	}

	// Cycle 2: the ?since= window carries net 2 with a new document and
	// netixlan 10 with the plan cleared. Both bump `updated`, as an
	// upstream save does.
	for _, flag := range srv.since {
		flag.Store(false)
	}
	rtbh := map[string]any{"rtbh_community": "65002:666"}
	rows2 := metaTestRows()
	rows2["net"] = []any{metaTestNet(2, 65002, "2026-05-01T00:00:00Z", rtbh)}
	rows2["netixlan"] = []any{metaTestNetIxLan("2026-05-01T00:00:00Z", map[string]any{})}
	srv.rows.Store(&rows2)

	if err := w.Sync(ctx, config.SyncModeIncremental); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}
	for _, typ := range []string{"net", "netixlan"} {
		if !srv.since[typ].Load() {
			t.Errorf("cycle 2 fetched %s without ?since=", typ)
		}
	}
	net2, err = client.Network.Get(ctx, 2)
	if err != nil {
		t.Fatalf("get net 2 after cycle 2: %v", err)
	}
	if diff := cmp.Diff(rtbh, net2.Meta); diff != "" {
		t.Errorf("net 2 meta after cycle 2 (-want +got):\n%s", diff)
	}
	nixl, err = client.NetworkIxLan.Get(ctx, 10)
	if err != nil {
		t.Fatalf("get netixlan 10 after cycle 2: %v", err)
	}
	if nixl.Meta == nil || len(nixl.Meta) != 0 {
		got, _ := json.Marshal(nixl.Meta)
		t.Errorf("netixlan 10 meta after cycle 2 = %s, want {} (document replaced, not merged)", got)
	}
	// net 1 was not in the window and keeps its document.
	net1, err = client.Network.Get(ctx, 1)
	if err != nil {
		t.Fatalf("get net 1 after cycle 2: %v", err)
	}
	if diff := cmp.Diff(netMeta, net1.Meta); diff != "" {
		t.Errorf("net 1 meta after cycle 2 (-want +got):\n%s", diff)
	}
}
