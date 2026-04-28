package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// setupDepthTestData creates a rich graph of related entities for depth testing.
// Returns the ent client with test data: 1 org with 1 network, 1 facility,
// 1 IX, 1 carrier, 1 campus, and linking entities.
func setupDepthTestData(t *testing.T) (*Handler, *http.ServeMux) {
	t.Helper()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second).UTC()

	// Create org.
	org, err := client.Organization.Create().
		SetName("Test Org").
		SetAka("TO").
		SetNameLong("Test Organization Inc").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create network linked to org.
	net, err := client.Network.Create().
		SetName("Test Net").
		SetAsn(65001).
		SetOrganization(org).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	// Create facility linked to org.
	fac, err := client.Facility.Create().
		SetName("Test Facility").
		SetOrganization(org).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create facility: %v", err)
	}

	// Create campus linked to org.
	campus, err := client.Campus.Create().
		SetName("Test Campus").
		SetOrganization(org).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create campus: %v", err)
	}

	// Create carrier linked to org.
	carrier, err := client.Carrier.Create().
		SetName("Test Carrier").
		SetOrganization(org).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create carrier: %v", err)
	}

	// Create IX linked to org.
	ix, err := client.InternetExchange.Create().
		SetName("Test IX").
		SetOrganization(org).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ix: %v", err)
	}

	// Create IXLan linked to IX.
	ixlan, err := client.IxLan.Create().
		SetInternetExchange(ix).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixlan: %v", err)
	}

	// Create POC linked to network.
	_, err = client.Poc.Create().
		SetName("Admin Contact").
		SetRole("Abuse").
		SetNetwork(net).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create poc: %v", err)
	}

	// Create network facility linking net and fac.
	netfac, err := client.NetworkFacility.Create().
		SetNetwork(net).
		SetFacility(fac).
		SetLocalAsn(65001).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create netfac: %v", err)
	}

	// Create network ix lan linking net and ixlan.
	_, err = client.NetworkIxLan.Create().
		SetNetwork(net).
		SetIxLan(ixlan).
		SetSpeed(10000).
		SetAsn(65001).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create netixlan: %v", err)
	}

	// Create ix prefix linked to ixlan.
	_, err = client.IxPrefix.Create().
		SetIxLan(ixlan).
		SetProtocol("IPv4").
		SetPrefix("10.0.0.0/24").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixpfx: %v", err)
	}

	// Create ix facility linking ix and fac.
	_, err = client.IxFacility.Create().
		SetInternetExchange(ix).
		SetFacility(fac).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create ixfac: %v", err)
	}

	// Create carrier facility linking carrier and fac.
	_, err = client.CarrierFacility.Create().
		SetCarrier(carrier).
		SetFacility(fac).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create carrierfac: %v", err)
	}

	// Suppress unused variable warnings by using all IDs transitively.
	_ = campus
	_ = netfac

	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

func TestDepth(t *testing.T) {
	t.Parallel()
	_, mux := setupDepthTestData(t)

	// explicit_depth_zero locks the upstream-parity behaviour for an EXPLICIT
	// `?depth=0` override on a detail URL: upstream's
	// `peeringdb_server/serializers.py:prefetch_related` (line 852) returns
	// the bare queryset early when depth<=0, so no `_set` fields are
	// embedded. Mirror MUST honour the same escape hatch.
	t.Run("explicit_depth_zero", func(t *testing.T) {
		t.Parallel()
		// Get first org from list.
		req := httptest.NewRequest(http.MethodGet, "/api/org?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var env testEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var items []map[string]any
		if err := json.Unmarshal(env.Data, &items); err != nil {
			t.Fatalf("unmarshal items: %v", err)
		}
		if len(items) == 0 {
			t.Fatal("expected at least 1 org")
		}
		orgID := int(items[0]["id"].(float64))

		// Explicit ?depth=0 must return the bare row (matches upstream
		// rest.py:852 short-circuit).
		detReq := httptest.NewRequest(http.MethodGet, "/api/org/"+itoa(orgID)+"?depth=0", nil)
		detRec := httptest.NewRecorder()
		mux.ServeHTTP(detRec, detReq)
		if detRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", detRec.Code, detRec.Body.String())
		}

		var detEnv testEnvelope
		if err := json.Unmarshal(detRec.Body.Bytes(), &detEnv); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var detItems []map[string]any
		if err := json.Unmarshal(detEnv.Data, &detItems); err != nil {
			t.Fatalf("unmarshal items: %v", err)
		}
		if len(detItems) != 1 {
			t.Fatalf("expected 1 item, got %d", len(detItems))
		}

		obj := detItems[0]
		// explicit ?depth=0: should NOT have _set fields.
		for _, setField := range []string{"net_set", "fac_set", "ix_set", "carrier_set", "campus_set"} {
			if _, ok := obj[setField]; ok {
				t.Errorf("explicit ?depth=0: %q should not be present", setField)
			}
		}
	})

	// default_detail_uses_depth_two locks the upstream-parity behaviour for
	// a bare detail URL with NO `?depth=` query param: upstream's
	// `peeringdb_server/serializers.py:default_depth(is_list=False)` (line
	// 823) returns 2 for single-object GETs, causing
	// `prefetch_related` (rest.py:750) to fire and embed every per-type
	// `_set` collection plus the parent FK objects (`org` for net/fac/ix/
	// carrier/campus). This is the canonical generalisation of commit
	// 0d39654 — the IX `fac_set` fix at `?depth=2` — extended to fire on
	// every detail URL regardless of explicit depth.
	t.Run("default_detail_uses_depth_two", func(t *testing.T) {
		t.Parallel()
		// Get first org from list.
		req := httptest.NewRequest(http.MethodGet, "/api/org?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var env testEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var items []map[string]any
		if err := json.Unmarshal(env.Data, &items); err != nil {
			t.Fatalf("unmarshal items: %v", err)
		}
		if len(items) == 0 {
			t.Fatal("expected at least 1 org")
		}
		orgID := int(items[0]["id"].(float64))

		// Bare detail URL (no ?depth=): upstream defaults to depth=2,
		// so all _set fields must be embedded.
		detReq := httptest.NewRequest(http.MethodGet, "/api/org/"+itoa(orgID), nil)
		detRec := httptest.NewRecorder()
		mux.ServeHTTP(detRec, detReq)
		if detRec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", detRec.Code, detRec.Body.String())
		}

		var detEnv testEnvelope
		if err := json.Unmarshal(detRec.Body.Bytes(), &detEnv); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		var detItems []map[string]any
		if err := json.Unmarshal(detEnv.Data, &detItems); err != nil {
			t.Fatalf("unmarshal items: %v", err)
		}
		if len(detItems) != 1 {
			t.Fatalf("expected 1 item, got %d", len(detItems))
		}

		obj := detItems[0]
		for _, setField := range []string{"net_set", "fac_set", "ix_set", "carrier_set", "campus_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("default detail (depth=2 implicit): %q missing — upstream embeds it on every bare detail URL", setField)
				continue
			}
			arr, ok := val.([]any)
			if !ok {
				t.Errorf("default detail: %q is not an array", setField)
				continue
			}
			if len(arr) != 1 {
				t.Errorf("default detail: %q expected 1 item, got %d", setField, len(arr))
			}
		}
	})

	// default_detail_net_uses_depth_two extends the same upstream-parity
	// invariant to the Network detail surface: bare /api/net/<id> must
	// embed poc_set, netfac_set, netixlan_set, AND the expanded org
	// object (the four keys this debug session was opened to fix —
	// observed missing on /api/net/15169 against the deployed mirror
	// post-0d39654).
	t.Run("default_detail_net_uses_depth_two", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/net?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		if len(items) == 0 {
			t.Fatal("expected at least 1 net")
		}
		netID := int(items[0]["id"].(float64))

		detReq := httptest.NewRequest(http.MethodGet, "/api/net/"+itoa(netID), nil)
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
		for _, setField := range []string{"poc_set", "netfac_set", "netixlan_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("default detail net: %q missing — upstream embeds on bare URL", setField)
				continue
			}
			if _, ok := val.([]any); !ok {
				t.Errorf("default detail net: %q is not an array", setField)
			}
		}
		orgVal, ok := obj["org"]
		if !ok {
			t.Error("default detail net: expanded org missing — upstream embeds on bare URL")
		} else if orgObj, ok := orgVal.(map[string]any); !ok {
			t.Error("default detail net: org is not an object")
		} else if _, hasID := orgObj["id"]; !hasID {
			t.Error("default detail net: expanded org missing id field")
		}
	})

	t.Run("two_org", func(t *testing.T) {
		t.Parallel()
		// Get first org at depth=2.
		req := httptest.NewRequest(http.MethodGet, "/api/org?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		orgID := int(items[0]["id"].(float64))

		detReq := httptest.NewRequest(http.MethodGet, "/api/org/"+itoa(orgID)+"?depth=2", nil)
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
		// Should have all _set fields.
		for _, setField := range []string{"net_set", "fac_set", "ix_set", "carrier_set", "campus_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("depth=2: %q missing", setField)
				continue
			}
			arr, ok := val.([]any)
			if !ok {
				t.Errorf("depth=2: %q is not an array", setField)
				continue
			}
			// We created one of each, so each _set should have 1 item.
			if len(arr) != 1 {
				t.Errorf("depth=2: %q expected 1 item, got %d", setField, len(arr))
			}
		}
	})

	t.Run("two_net", func(t *testing.T) {
		t.Parallel()
		// Get network at depth=2: should have poc_set, netfac_set, netixlan_set
		// and expanded org.
		req := httptest.NewRequest(http.MethodGet, "/api/net?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		netID := int(items[0]["id"].(float64))

		detReq := httptest.NewRequest(http.MethodGet, "/api/net/"+itoa(netID)+"?depth=2", nil)
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

		// Check _set fields.
		for _, setField := range []string{"poc_set", "netfac_set", "netixlan_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("depth=2: %q missing", setField)
				continue
			}
			arr, ok := val.([]any)
			if !ok {
				t.Errorf("depth=2: %q is not an array", setField)
				continue
			}
			if len(arr) != 1 {
				t.Errorf("depth=2: %q expected 1 item, got %d", setField, len(arr))
			}
		}

		// Check that org is expanded (object, not just org_id).
		orgVal, ok := obj["org"]
		if !ok {
			t.Error("depth=2: expanded org missing")
		} else {
			orgObj, ok := orgVal.(map[string]any)
			if !ok {
				t.Error("depth=2: org is not an object")
			} else if _, hasID := orgObj["id"]; !hasID {
				t.Error("depth=2: expanded org missing id field")
			}
		}
	})

	// two_ix locks the upstream-parity InternetExchange depth=2 shape:
	// upstream PeeringDB's InternetExchangeSerializer
	// (peeringdb_server/serializers.py:3514) emits `fac_set` as a list of
	// expanded Facility objects via nested(FacilitySerializer,
	// through="ixfac_set", getter="facility"). It does NOT emit `ixfac_set`
	// (that surface only appears on the facility-side serializer). Regression
	// guard against the prior divergence where the mirror exposed raw
	// IxFacility join rows under the wrong key.
	t.Run("two_ix", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/ix?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		if len(items) == 0 {
			t.Fatal("expected at least 1 ix")
		}
		ixID := int(items[0]["id"].(float64))

		detReq := httptest.NewRequest(http.MethodGet, "/api/ix/"+itoa(ixID)+"?depth=2", nil)
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

		// Required _set fields.
		for _, setField := range []string{"ixlan_set", "fac_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("depth=2: %q missing", setField)
				continue
			}
			arr, ok := val.([]any)
			if !ok {
				t.Errorf("depth=2: %q is not an array", setField)
				continue
			}
			if len(arr) != 1 {
				t.Errorf("depth=2: %q expected 1 item, got %d", setField, len(arr))
			}
		}

		// Upstream parity: ixfac_set MUST NOT appear on the IX surface.
		if _, ok := obj["ixfac_set"]; ok {
			t.Error("depth=2: upstream omits ixfac_set on IX surface; mirror must not expose it")
		}

		// fac_set entries must be expanded Facility objects (not raw join rows).
		// The shape distinction: an IxFacility join row has ix_id+fac_id but no
		// address1/latitude/longitude; a Facility has address1, latitude,
		// longitude (via FacilitySerializer.Meta.fields).
		facSet, _ := obj["fac_set"].([]any)
		if len(facSet) > 0 {
			fac0, ok := facSet[0].(map[string]any)
			if !ok {
				t.Fatal("depth=2: fac_set[0] is not an object")
			}
			for _, expectedField := range []string{"id", "name", "address1", "latitude", "longitude"} {
				if _, has := fac0[expectedField]; !has {
					t.Errorf("depth=2: fac_set[0] missing expected Facility field %q", expectedField)
				}
			}
			// Negative shape check: a raw IxFacility row has ix_id; a Facility does not.
			if _, hasIxID := fac0["ix_id"]; hasIxID {
				t.Error("depth=2: fac_set[0] looks like raw IxFacility row (has ix_id); must be expanded Facility")
			}
		}

		// Check that org is expanded.
		orgVal, ok := obj["org"]
		if !ok {
			t.Error("depth=2: expanded org missing")
		} else if orgObj, ok := orgVal.(map[string]any); !ok {
			t.Error("depth=2: org is not an object")
		} else if _, hasID := orgObj["id"]; !hasID {
			t.Error("depth=2: expanded org missing id field")
		}
	})

	t.Run("empty_sets", func(t *testing.T) {
		t.Parallel()
		// Create an org with no related entities.
		client := testutil.SetupClient(t)
		ctx := t.Context()
		now := time.Now().Truncate(time.Second).UTC()
		org, err := client.Organization.Create().
			SetName("Empty Org").
			SetCreated(now).
			SetUpdated(now).
			SetStatus("ok").
			Save(ctx)
		if err != nil {
			t.Fatalf("create org: %v", err)
		}

		h := NewHandler(client, 0)
		mux := http.NewServeMux()
		h.Register(mux)

		detReq := httptest.NewRequest(http.MethodGet, "/api/org/"+itoa(org.ID)+"?depth=2", nil)
		detRec := httptest.NewRecorder()
		mux.ServeHTTP(detRec, detReq)

		var detEnv testEnvelope
		_ = json.Unmarshal(detRec.Body.Bytes(), &detEnv)
		var detItems []map[string]any
		_ = json.Unmarshal(detEnv.Data, &detItems)

		obj := detItems[0]
		for _, setField := range []string{"net_set", "fac_set", "ix_set", "carrier_set", "campus_set"} {
			val, ok := obj[setField]
			if !ok {
				t.Errorf("empty_sets: %q missing", setField)
				continue
			}
			arr, ok := val.([]any)
			if !ok {
				t.Errorf("empty_sets: %q is not an array", setField)
				continue
			}
			if len(arr) != 0 {
				t.Errorf("empty_sets: %q expected empty array, got %d items", setField, len(arr))
			}
		}
	})

	t.Run("list_ignores_depth", func(t *testing.T) {
		t.Parallel()
		// List endpoint should NOT have _set fields even with depth=2.
		req := httptest.NewRequest(http.MethodGet, "/api/org?depth=2", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		if len(items) == 0 {
			t.Fatal("expected at least 1 org in list")
		}

		obj := items[0]
		for _, setField := range []string{"net_set", "fac_set", "ix_set"} {
			if _, ok := obj[setField]; ok {
				t.Errorf("list_ignores_depth: %q should not be present on list endpoint", setField)
			}
		}
	})

	t.Run("leaf_entity", func(t *testing.T) {
		t.Parallel()
		// netfac at depth=2 should expand FK edges but have no _set fields.
		req := httptest.NewRequest(http.MethodGet, "/api/netfac?limit=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var env testEnvelope
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		var items []map[string]any
		_ = json.Unmarshal(env.Data, &items)
		if len(items) == 0 {
			t.Fatal("expected at least 1 netfac")
		}
		netfacID := int(items[0]["id"].(float64))

		detReq := httptest.NewRequest(http.MethodGet, "/api/netfac/"+itoa(netfacID)+"?depth=2", nil)
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

		// Should have expanded FK edges.
		netVal, ok := obj["net"]
		if !ok {
			t.Error("depth=2 leaf: expanded net missing")
		} else if netObj, ok := netVal.(map[string]any); !ok {
			t.Error("depth=2 leaf: net is not an object")
		} else if _, hasID := netObj["id"]; !hasID {
			t.Error("depth=2 leaf: expanded net missing id field")
		}

		facVal, ok := obj["fac"]
		if !ok {
			t.Error("depth=2 leaf: expanded fac missing")
		} else if facObj, ok := facVal.(map[string]any); !ok {
			t.Error("depth=2 leaf: fac is not an object")
		} else if _, hasID := facObj["id"]; !hasID {
			t.Error("depth=2 leaf: expanded fac missing id field")
		}
	})
}
