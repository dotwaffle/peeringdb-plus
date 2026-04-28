package sync_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestFKFilter_NetworkIxLan_NullsSideFKOnMiss is the regression guard for
// quick task 260428-2zl Task 4. NetworkIxLan side FKs (net_side_id,
// ix_side_id) are upstream-declared as `null=True, on_delete=SET_NULL`.
// Pre-2zl the worker would drop the entire NetworkIxLan row when either
// side FK pointed at a missing facility. Post-2zl: the row survives,
// the side FK is nulled in place, and an orphan is recorded with
// action="null".
func TestFKFilter_NetworkIxLan_NullsSideFKOnMiss(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		// Suppress backfill (cap=0 in worker config below) — the test
		// asserts the SET_NULL contract, not the backfill recovery
		// path.
		if r.URL.Query().Get("id__in") != "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		var data json.RawMessage
		switch typeName {
		case "org":
			data = mustJSON([]any{map[string]any{
				"id": 1, "name": "Org1", "aka": "", "name_long": "",
				"website": "", "social_media": []any{}, "notes": "",
				"address1": "", "address2": "", "city": "", "state": "", "country": "US",
				"zipcode": "", "suite": "", "floor": "",
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}})
		case "net":
			data = mustJSON([]any{map[string]any{
				"id": 1, "org_id": 1, "name": "N1", "aka": "", "name_long": "",
				"website": "", "social_media": []any{}, "asn": 65001,
				"looking_glass": "", "route_server": "", "irr_as_set": "",
				"info_type": "", "info_types": []any{},
				"info_traffic": "", "info_ratio": "", "info_scope": "",
				"info_unicast": true, "info_multicast": false, "info_ipv6": true,
				"info_never_via_route_servers": false, "notes": "",
				"policy_url": "", "policy_general": "", "policy_locations": "",
				"policy_ratio": false, "policy_contracts": "", "allow_ixp_update": false,
				"ix_count": 0, "fac_count": 0,
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}})
		case "ix":
			data = mustJSON([]any{map[string]any{
				"id": 1, "org_id": 1, "name": "IX1", "aka": "", "name_long": "",
				"city": "", "country": "US", "region_continent": "",
				"media": "Ethernet", "notes": "",
				"proto_unicast": true, "proto_multicast": false, "proto_ipv6": true,
				"website": "", "social_media": []any{}, "url_stats": "",
				"tech_email": "", "tech_phone": "",
				"policy_email": "", "policy_phone": "",
				"sales_email": "", "sales_phone": "",
				"net_count": 0, "fac_count": 0, "ixf_net_count": 0,
				"ixf_last_import": nil, "ixf_import_request": nil,
				"ixf_import_request_status": "",
				"service_level": "", "terms": "",
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}})
		case "ixlan":
			data = mustJSON([]any{map[string]any{
				"id": 1, "ix_id": 1, "name": "L1", "descr": "",
				"mtu": 9000, "dot1q_support": false, "rs_asn": 65500,
				"arp_sponge": nil, "ixf_ixp_member_list_url_visible": "Public",
				"ixf_ixp_import_enabled": true,
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}})
		case "netixlan":
			// netixlan referencing missing fac 999 in net_side_id.
			data = mustJSON([]any{map[string]any{
				"id": 10, "net_id": 1, "ix_id": 1, "ixlan_id": 1,
				"name": "", "notes": "", "speed": 10000, "asn": 65001,
				"ipaddr4": "192.0.2.1", "ipaddr6": nil,
				"is_rs_peer": false, "bfd_support": false, "operational": true,
				"net_side_id": 999, "ix_side_id": nil,
				"created": "2026-04-01T00:00:00Z", "updated": "2026-04-01T00:00:00Z",
				"status": "ok",
			}})
		default:
			data = json.RawMessage(`[]`)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":`))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte(`}`))
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
		FKBackfillMaxPerCycle: 0, // disable backfill — test SET_NULL only
	}, slog.Default())

	if err := w.Sync(t.Context(), "full"); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// netixlan 10 must SURVIVE (the row is NOT dropped).
	got, err := client.NetworkIxLan.Get(t.Context(), 10)
	if err != nil {
		t.Fatalf("netixlan 10 missing — pre-2zl drop behavior leaked: %v", err)
	}
	// net_side_id should now be NULL (NetSideID is *int; nil means NULL).
	if got.NetSideID != nil {
		t.Errorf("net_side_id = %d, want nil (SET_NULL contract violated)", *got.NetSideID)
	}
	// ix_side_id was already nil in the fixture and should remain nil.
	if got.IxSideID != nil {
		t.Errorf("ix_side_id = %d, want nil", *got.IxSideID)
	}
}
