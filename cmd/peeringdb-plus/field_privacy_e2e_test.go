// Package main field_privacy_e2e_test.go — Phase 64 D-10 end-to-end
// field-level privacy contract for ixlan.ixf_ixp_member_list_url.
//
// Mirrors e2e_privacy_test.go's 5-surface pattern (Phase 59 D-15) but
// operates at field level instead of row level. Asserts:
//
//   - Anonymous callers (TierPublic) get NO ixf_ixp_member_list_url
//     key in responses for rows whose _visible is "Users" or "Private".
//   - Users-tier callers DO get the URL for _visible="Users" or "Public".
//   - _visible="Public" rows ALWAYS emit the URL regardless of tier
//     (id=101 seed row locks always-admit behaviour — proves the helper
//     does not over-redact).
//   - The companion _visible field is ALWAYS emitted regardless of
//     tier (D-05 — upstream parity).
//   - Fail-closed at surface level (D-03): bypassing the PrivacyTier
//     middleware at the ConnectRPC handler STILL redacts for id=100.
//
// Uses the shared buildE2EFixture(t, tier) helper from e2e_privacy_test.go,
// which seeds the two required ixlan rows (id=100 Users-gated and id=101
// Public) via the Phase 64 fixture extension.
//
// Web UI is skipped with a TODO — UI does not currently render the URL
// field (Phase 64 RESEARCH.md Finding). Re-enable when/if it does.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	pbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// =============================================================================
// TierPublic: URL redacted on Users-gated row (id=100), admitted on
// Public row (id=101), fail-closed at the bypass-middleware handler.
// =============================================================================

func TestE2E_FieldLevel_IxlanURL_RedactedAnon(t *testing.T) {
	t.Parallel()
	fix := buildE2EFixture(t, privctx.TierPublic)

	gatedIDStr := strconv.Itoa(fix.gatedIxLanID)
	publicIDStr := strconv.Itoa(fix.publicIxLanID)

	// -------------------------------------------------------------------------
	// Surface 1: pdbcompat /api
	// -------------------------------------------------------------------------
	t.Run("pdbcompat/detail/gated", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan/"+gatedIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", fix.gatedIxLanID, status, body)
		}
		row := extractPdbcompatFirst(t, body)
		assertHasKey(t, row, "ixf_ixp_member_list_url_visible") // D-05
		assertLacksKey(t, row, "ixf_ixp_member_list_url")       // VIS-09
	})

	t.Run("pdbcompat/detail/public", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan/"+publicIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", fix.publicIxLanID, status, body)
		}
		row := extractPdbcompatFirst(t, body)
		assertHasKey(t, row, "ixf_ixp_member_list_url_visible")
		assertStringValue(t, row, "ixf_ixp_member_list_url", e2ePublicIxlanURL)
	})

	t.Run("pdbcompat/list", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan")
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan: status=%d; body=%s", status, body)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /api/ixlan: %v\nbody=%s", err, body)
		}
		assertIxlanListShape(t, env.Data, fix.gatedIxLanID, fix.publicIxLanID)
	})

	// -------------------------------------------------------------------------
	// Surface 2: entrest /rest/v1/ix-lans
	// -------------------------------------------------------------------------
	t.Run("entrest/detail/gated", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans/"+gatedIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans/%d: status=%d; body=%s", fix.gatedIxLanID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode detail: %v\nbody=%s", err, body)
		}
		assertHasKey(t, obj, "ixf_ixp_member_list_url_visible")
		assertLacksKey(t, obj, "ixf_ixp_member_list_url")
	})

	t.Run("entrest/detail/public", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans/"+publicIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans/%d: status=%d; body=%s", fix.publicIxLanID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode detail: %v\nbody=%s", err, body)
		}
		assertHasKey(t, obj, "ixf_ixp_member_list_url_visible")
		assertStringValue(t, obj, "ixf_ixp_member_list_url", e2ePublicIxlanURL)
	})

	t.Run("entrest/list", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans")
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans: status=%d; body=%s", status, body)
		}
		var env struct {
			Content []map[string]any `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /rest/v1/ix-lans: %v\nbody=%s", err, body)
		}
		assertIxlanListShape(t, env.Content, fix.gatedIxLanID, fix.publicIxLanID)
	})

	// -------------------------------------------------------------------------
	// Surface 3: ConnectRPC IxLanService
	// -------------------------------------------------------------------------
	t.Run("connectrpc/get/gated", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetIxLan(t.Context(), &pbv1.GetIxLanRequest{Id: int64(fix.gatedIxLanID)})
		if err != nil {
			t.Fatalf("GetIxLan: %v", err)
		}
		if resp.IxLan.IxfIxpMemberListUrl != nil {
			t.Errorf("anon tier received url = %v, want nil", resp.IxLan.IxfIxpMemberListUrl)
		}
		if resp.IxLan.IxfIxpMemberListUrlVisible.GetValue() == "" {
			t.Error("expected _visible companion to remain populated (D-05)")
		}
	})

	t.Run("connectrpc/get/public", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetIxLan(t.Context(), &pbv1.GetIxLanRequest{Id: int64(fix.publicIxLanID)})
		if err != nil {
			t.Fatalf("GetIxLan public: %v", err)
		}
		if resp.IxLan.IxfIxpMemberListUrl == nil {
			t.Fatal("Public-visible row must always admit url")
		}
		if got := resp.IxLan.IxfIxpMemberListUrl.GetValue(); got != e2ePublicIxlanURL {
			t.Errorf("public url = %q, want %q", got, e2ePublicIxlanURL)
		}
	})

	t.Run("connectrpc/list", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.ListIxLans(t.Context(), &pbv1.ListIxLansRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("ListIxLans: %v", err)
		}
		var gatedSeen, publicSeen bool
		for _, il := range resp.IxLans {
			switch il.Id {
			case int64(fix.gatedIxLanID):
				gatedSeen = true
				if il.IxfIxpMemberListUrl != nil {
					t.Errorf("anon tier list received url for gated row id=%d", il.Id)
				}
			case int64(fix.publicIxLanID):
				publicSeen = true
				if il.IxfIxpMemberListUrl == nil {
					t.Errorf("public-visible row id=%d must admit url for all tiers", il.Id)
				}
			}
		}
		if !gatedSeen || !publicSeen {
			t.Fatalf("expected both gated=%d and public=%d ixlan rows in list, saw gated=%v public=%v",
				fix.gatedIxLanID, fix.publicIxLanID, gatedSeen, publicSeen)
		}
	})

	// D-03: surface-level fail-closed. Construct the request against the
	// raw ConnectRPC handler (no middleware chain), so the ctx reaching
	// the handler has no tier stamp. privfield.Redact MUST still blank
	// the URL — if it didn't, a future code path that forgets to route
	// through PrivacyTier would leak the URL.
	t.Run("fail-closed-bypass-middleware", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(
			context.Background(), // deliberate: no tier stamp on ctx
			http.MethodPost,
			fix.rawIxLanPath+"GetIxLan",
			strings.NewReader(`{"id":`+gatedIDStr+`}`),
		)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		fix.rawIxLanHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("handler status = %d (body=%s), want 200 (fail-closed must return the row minus the URL)",
				rec.Code, rec.Body.String())
		}
		// ConnectRPC uses protojson which emits camelCase proto field
		// names (not the snake_case `json:` struct tags that apply only
		// to the Go type); StringValue wrappers serialise as bare JSON
		// strings, not {"value":"…"} objects.
		var resp struct {
			IxLan struct {
				IxfIxpMemberListURL        *string `json:"ixfIxpMemberListUrl"`
				IxfIxpMemberListURLVisible string  `json:"ixfIxpMemberListUrlVisible"`
			} `json:"ixLan"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, rec.Body.String())
		}
		if resp.IxLan.IxfIxpMemberListURL != nil {
			t.Errorf("unstamped ctx leaked url = %v; fail-closed violated", *resp.IxLan.IxfIxpMemberListURL)
		}
		if resp.IxLan.IxfIxpMemberListURLVisible == "" {
			t.Errorf("_visible companion missing; D-05 regression\nbody=%s", rec.Body.String())
		}
	})

	// -------------------------------------------------------------------------
	// Surface 4: GraphQL
	// -------------------------------------------------------------------------
	t.Run("graphql/gated", func(t *testing.T) {
		q := fmt.Sprintf(
			`{"query":"{ ixLans(where:{id: %d}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }"}`,
			fix.gatedIxLanID,
		)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		node := extractGraphQLFirstIxLan(t, body)
		if got := node["ixfIxpMemberListURL"]; got != nil {
			t.Errorf("anon tier received URL = %v, want null", got)
		}
		if v, _ := node["ixfIxpMemberListURLVisible"].(string); v != "Users" {
			t.Errorf("_visible = %q, want %q (D-05)", v, "Users")
		}
	})

	t.Run("graphql/public", func(t *testing.T) {
		q := fmt.Sprintf(
			`{"query":"{ ixLans(where:{id: %d}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }"}`,
			fix.publicIxLanID,
		)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		node := extractGraphQLFirstIxLan(t, body)
		got, _ := node["ixfIxpMemberListURL"].(string)
		if got != e2ePublicIxlanURL {
			t.Errorf("public URL = %q, want %q", got, e2ePublicIxlanURL)
		}
	})

	// -------------------------------------------------------------------------
	// Surface 5: Web UI — the UI does not currently render
	// ixf_ixp_member_list_url. When a future phase adds it to
	// /ui/ixlan/{id} or a fragment, extend this sub-test to parse the
	// rendered HTML and assert the URL is NOT present at TierPublic.
	// -------------------------------------------------------------------------
	t.Run("webui", func(t *testing.T) {
		t.Skip("UI does not render ixf_ixp_member_list_url (Phase 64 RESEARCH)")
	})
}

// =============================================================================
// TierUsers: URL admitted on both rows across all 5 surfaces.
// =============================================================================

func TestE2E_FieldLevel_IxlanURL_VisibleToUsersTier(t *testing.T) {
	t.Parallel()
	fix := buildE2EFixture(t, privctx.TierUsers)

	gatedIDStr := strconv.Itoa(fix.gatedIxLanID)
	publicIDStr := strconv.Itoa(fix.publicIxLanID)

	t.Run("pdbcompat/detail/gated", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan/"+gatedIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", fix.gatedIxLanID, status, body)
		}
		row := extractPdbcompatFirst(t, body)
		assertStringValue(t, row, "ixf_ixp_member_list_url", e2eGatedIxlanURL)
	})

	t.Run("pdbcompat/detail/public", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan/"+publicIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", fix.publicIxLanID, status, body)
		}
		row := extractPdbcompatFirst(t, body)
		assertStringValue(t, row, "ixf_ixp_member_list_url", e2ePublicIxlanURL)
	})

	t.Run("entrest/detail/gated", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans/"+gatedIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans/%d: status=%d; body=%s", fix.gatedIxLanID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, body)
		}
		assertStringValue(t, obj, "ixf_ixp_member_list_url", e2eGatedIxlanURL)
	})

	t.Run("entrest/detail/public", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans/"+publicIDStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans/%d: status=%d; body=%s", fix.publicIxLanID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, body)
		}
		assertStringValue(t, obj, "ixf_ixp_member_list_url", e2ePublicIxlanURL)
	})

	t.Run("connectrpc/get/gated", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetIxLan(t.Context(), &pbv1.GetIxLanRequest{Id: int64(fix.gatedIxLanID)})
		if err != nil {
			t.Fatalf("GetIxLan: %v", err)
		}
		if got := resp.IxLan.IxfIxpMemberListUrl.GetValue(); got != e2eGatedIxlanURL {
			t.Errorf("users tier, gated url = %q, want %q", got, e2eGatedIxlanURL)
		}
	})

	t.Run("connectrpc/get/public", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetIxLan(t.Context(), &pbv1.GetIxLanRequest{Id: int64(fix.publicIxLanID)})
		if err != nil {
			t.Fatalf("GetIxLan public: %v", err)
		}
		if got := resp.IxLan.IxfIxpMemberListUrl.GetValue(); got != e2ePublicIxlanURL {
			t.Errorf("users tier, public url = %q, want %q", got, e2ePublicIxlanURL)
		}
	})

	t.Run("connectrpc/list", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.ListIxLans(t.Context(), &pbv1.ListIxLansRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("ListIxLans: %v", err)
		}
		var gatedSeen, publicSeen bool
		for _, il := range resp.IxLans {
			switch il.Id {
			case int64(fix.gatedIxLanID):
				gatedSeen = true
				if got := il.IxfIxpMemberListUrl.GetValue(); got != e2eGatedIxlanURL {
					t.Errorf("users tier, gated list url = %q, want %q", got, e2eGatedIxlanURL)
				}
			case int64(fix.publicIxLanID):
				publicSeen = true
				if got := il.IxfIxpMemberListUrl.GetValue(); got != e2ePublicIxlanURL {
					t.Errorf("users tier, public list url = %q, want %q", got, e2ePublicIxlanURL)
				}
			}
		}
		if !gatedSeen || !publicSeen {
			t.Fatalf("users tier list missing rows: gated=%v public=%v", gatedSeen, publicSeen)
		}
	})

	t.Run("graphql/gated", func(t *testing.T) {
		q := fmt.Sprintf(
			`{"query":"{ ixLans(where:{id: %d}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }"}`,
			fix.gatedIxLanID,
		)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		node := extractGraphQLFirstIxLan(t, body)
		got, _ := node["ixfIxpMemberListURL"].(string)
		if got != e2eGatedIxlanURL {
			t.Errorf("users tier, gated URL = %q, want %q", got, e2eGatedIxlanURL)
		}
	})

	t.Run("graphql/public", func(t *testing.T) {
		q := fmt.Sprintf(
			`{"query":"{ ixLans(where:{id: %d}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }"}`,
			fix.publicIxLanID,
		)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		node := extractGraphQLFirstIxLan(t, body)
		got, _ := node["ixfIxpMemberListURL"].(string)
		if got != e2ePublicIxlanURL {
			t.Errorf("users tier, public URL = %q, want %q", got, e2ePublicIxlanURL)
		}
	})

	t.Run("webui", func(t *testing.T) {
		t.Skip("UI does not render ixf_ixp_member_list_url (Phase 64 RESEARCH)")
	})
}

// =============================================================================
// Helpers
// =============================================================================

// extractPdbcompatFirst decodes a pdbcompat {data:[…]} envelope and
// returns the first row. Fatals if the body doesn't decode or the
// envelope is empty.
func extractPdbcompatFirst(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode pdbcompat envelope: %v\nbody=%s", err, body)
	}
	if len(env.Data) == 0 {
		t.Fatalf("pdbcompat envelope has no data rows\nbody=%s", body)
	}
	return env.Data[0]
}

// extractGraphQLFirstIxLan decodes a GraphQL ixLans query response and
// returns the first edge's node as a generic map for key-existence
// assertions.
func extractGraphQLFirstIxLan(t *testing.T, body []byte) map[string]any {
	t.Helper()
	r := decodeGraphQL(t, body)
	if len(r.Errors) > 0 {
		t.Fatalf("unexpected graphql errors: %+v", r.Errors)
	}
	var data struct {
		IxLans struct {
			Edges []struct {
				Node map[string]any `json:"node"`
			} `json:"edges"`
		} `json:"ixLans"`
	}
	if err := json.Unmarshal(r.Data, &data); err != nil {
		t.Fatalf("decode ixLans data: %v\ndata=%s", err, r.Data)
	}
	if len(data.IxLans.Edges) == 0 {
		t.Fatalf("ixLans query returned 0 edges; data=%s", r.Data)
	}
	return data.IxLans.Edges[0].Node
}

// assertHasKey fatals if the given key is absent from obj.
func assertHasKey(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	if _, ok := obj[key]; !ok {
		t.Fatalf("expected key %q to be present; obj=%+v", key, obj)
	}
}

// assertLacksKey fatals if the given key is present in obj.
func assertLacksKey(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Fatalf("expected key %q to be absent; obj=%+v", key, obj)
	}
}

// assertStringValue fatals if obj[key] is missing or not equal to want.
func assertStringValue(t *testing.T, obj map[string]any, key, want string) {
	t.Helper()
	got, ok := obj[key].(string)
	if !ok {
		t.Fatalf("expected key %q to be a string; obj=%+v", key, obj)
	}
	if got != want {
		t.Fatalf("key %q: got %q, want %q", key, got, want)
	}
}

// assertIxlanListShape asserts the two seeded rows are present in the
// list with the correct URL-admit/redact behaviour for anon callers.
// Applied to pdbcompat /api/ixlan and entrest /rest/v1/ix-lans list
// responses.
func assertIxlanListShape(t *testing.T, rows []map[string]any, gatedID, publicID int) {
	t.Helper()
	var gatedSeen, publicSeen bool
	for _, row := range rows {
		idFloat, ok := row["id"].(float64)
		if !ok {
			continue
		}
		assertHasKey(t, row, "ixf_ixp_member_list_url_visible")
		switch int(idFloat) {
		case gatedID:
			gatedSeen = true
			assertLacksKey(t, row, "ixf_ixp_member_list_url")
		case publicID:
			publicSeen = true
			assertStringValue(t, row, "ixf_ixp_member_list_url", e2ePublicIxlanURL)
		}
	}
	if !gatedSeen || !publicSeen {
		t.Fatalf("expected both rows in list: gated(id=%d)=%v public(id=%d)=%v",
			gatedID, gatedSeen, publicID, publicSeen)
	}
}

// Compile-time reference to io so "unused import" lint doesn't trip if
// the helpers get refactored to a different body-reader path. The
// existing e2e_privacy_test.go already depends on io; this is defensive.
var _ = io.Discard
