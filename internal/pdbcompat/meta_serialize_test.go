package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// metaGet GETs path and returns the raw body and the decoded data rows.
func metaGet(t *testing.T, h http.Handler, path string) (string, []map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String())
	}
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || len(env.Data) == 0 {
		t.Fatalf("GET %s: decode: %v (rows=%d)", path, err, len(env.Data))
	}
	return rec.Body.String(), env.Data
}

// metaAt walks obj through the object keys in path and returns the value
// of its "meta" key. A key of the form "name[i]" indexes into an array.
func metaAt(t *testing.T, obj map[string]any, path ...string) any {
	t.Helper()
	cur := obj
	for _, key := range path {
		name, idx, isIdx := strings.Cut(strings.TrimSuffix(key, "]"), "[")
		next := cur[name]
		if isIdx {
			arr, _ := next.([]any)
			i, err := strconv.Atoi(idx)
			if err != nil || i >= len(arr) {
				t.Fatalf("%s: no element %s in %v", key, idx, next)
			}
			next = arr[i]
		}
		m, ok := next.(map[string]any)
		if !ok {
			t.Fatalf("%s is %T, want object", key, next)
		}
		cur = m
	}
	v, ok := cur["meta"]
	if !ok {
		t.Fatalf("meta key missing at %v", path)
	}
	return v
}

// TestMeta_EveryNetAndNetIxLanShape locks the PeeringDB 2.83.0 meta
// document on the /api surface: net and netixlan carry it in every shape
// (list, detail, depth sets, nested net objects), as an object, {} when
// no document is stored.
//
// upstream: serializers.py:3117 (NetworkIXLanSerializer fields, meta after
// ix_side_id), :3684 (NetworkSerializer fields, meta after logo);
// migrations/0159 (JSONField default=dict).
func TestMeta_EveryNetAndNetIxLanShape(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	netDoc := map[string]any{"preferred_ip_mtu": float64(9000), "rtbh_community": "65002:666"}
	plan := map[string]any{
		"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-31"},
		"rfc8950":               true,
	}
	empty := map[string]any{}

	org := c.Organization.Create().SetName("Meta Org").SetStatus("ok").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	fac := c.Facility.Create().SetName("Meta Fac").SetOrganization(org).SetStatus("ok").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	netA := c.Network.Create().SetName("Meta Net A").SetAsn(65001).SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	netB := c.Network.Create().SetName("Meta Net B").SetAsn(65002).SetOrganization(org).
		SetMeta(netDoc).SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	ix := c.InternetExchange.Create().SetName("Meta IX").SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	lan := c.IxLan.Create().SetInternetExchange(ix).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	nixlA := c.NetworkIxLan.Create().SetNetwork(netA).SetIxLan(lan).SetIxID(ix.ID).
		SetAsn(65001).SetSpeed(1000).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	nixlB := c.NetworkIxLan.Create().SetNetwork(netB).SetIxLan(lan).SetIxID(ix.ID).
		SetAsn(65002).SetSpeed(1000).SetMeta(plan).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	poc := c.Poc.Create().SetName("Meta Contact").SetRole("NOC").SetVisible("Public").
		SetNetwork(netB).SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	netfac := c.NetworkFacility.Create().SetNetwork(netB).SetFacility(fac).SetLocalAsn(65002).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)

	mux := newMuxForOrdering(c)

	// Wire order: meta follows logo on net and ix_side_id on netixlan.
	body, _ := metaGet(t, mux, fmt.Sprintf("/api/net?id=%d", netA.ID))
	if !strings.Contains(body, `"logo":null,"meta":{},`) {
		t.Errorf("/api/net: meta does not follow logo as {}: %s", body)
	}
	body, _ = metaGet(t, mux, fmt.Sprintf("/api/netixlan?id=%d", nixlA.ID))
	if !strings.Contains(body, `"ix_side_id":null,"meta":{},`) {
		t.Errorf("/api/netixlan: meta does not follow ix_side_id as {}: %s", body)
	}

	cases := []struct {
		name string
		path string
		at   []string // object keys from data[0] to the object that carries meta
		want map[string]any
	}{
		{"net list stored", fmt.Sprintf("/api/net?id=%d", netB.ID), nil, netDoc},
		{"net depth 0 empty", fmt.Sprintf("/api/net/%d?depth=0", netA.ID), nil, empty},
		{"net depth 0 stored", fmt.Sprintf("/api/net/%d?depth=0", netB.ID), nil, netDoc},
		{"net depth 2 stored", fmt.Sprintf("/api/net/%d?depth=2", netB.ID), nil, netDoc},
		{"net depth 2 netixlan_set", fmt.Sprintf("/api/net/%d?depth=2", netB.ID), []string{"netixlan_set[0]"}, plan},
		{"netixlan list stored", fmt.Sprintf("/api/netixlan?id=%d", nixlB.ID), nil, plan},
		{"netixlan depth 0 empty", fmt.Sprintf("/api/netixlan/%d?depth=0", nixlA.ID), nil, empty},
		{"netixlan depth 2 stored", fmt.Sprintf("/api/netixlan/%d?depth=2", nixlB.ID), nil, plan},
		{"netixlan depth 2 nested net", fmt.Sprintf("/api/netixlan/%d?depth=2", nixlB.ID), []string{"net"}, netDoc},
		{"poc depth 2 nested net", fmt.Sprintf("/api/poc/%d?depth=2", poc.ID), []string{"net"}, netDoc},
		{"netfac depth 2 nested net", fmt.Sprintf("/api/netfac/%d?depth=2", netfac.ID), []string{"net"}, netDoc},
		{"ixlan depth 2 net_set empty", fmt.Sprintf("/api/ixlan/%d?depth=2", lan.ID), []string{"net_set[0]"}, empty},
		{"ixlan depth 2 net_set stored", fmt.Sprintf("/api/ixlan/%d?depth=2", lan.ID), []string{"net_set[1]"}, netDoc},
		{"org depth 2 net_set", fmt.Sprintf("/api/org/%d?depth=2", org.ID), []string{"net_set[1]"}, netDoc},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, data := metaGet(t, mux, tc.path)
			if diff := cmp.Diff(tc.want, metaAt(t, data[0], tc.at...)); diff != "" {
				t.Errorf("%s meta at %v (-want +got):\n%s", tc.path, tc.at, diff)
			}
		})
	}
}
