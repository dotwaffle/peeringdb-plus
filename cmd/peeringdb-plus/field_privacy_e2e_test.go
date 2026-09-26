// Package main field_privacy_e2e_test.go — end-to-end
// field-level privacy contract for ixlan.ixf_ixp_member_list_url.
//
// Mirrors e2e_privacy_test.go's 5-surface pattern but
// operates at field level instead of row level. Asserts:
//
//   - Anonymous callers (TierPublic) get NO ixf_ixp_member_list_url
//     key in responses for rows whose _visible is "Users" or "Private".
//   - Users-tier callers DO get the URL for _visible="Users" or "Public".
//   - _visible="Public" rows ALWAYS emit the URL regardless of tier
//     (id=101 seed row locks always-admit behaviour — proves the helper
//     does not over-redact).
//   - The companion _visible field is ALWAYS emitted regardless of
//     tier (upstream parity).
//   - Fail-closed at surface level: bypassing the PrivacyTier
//     middleware at the ConnectRPC handler STILL redacts for id=100.
//
// Uses the shared buildE2EFixture(t, tier) helper from e2e_privacy_test.go,
// which seeds the two required ixlan rows (id=100 Users-gated and id=101
// Public) via the fixture extension.
//
// Web UI is skipped with a TODO — UI does not currently render the URL
// field. Re-enable when/if it does.
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
	"time"

	pbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// =============================================================================
// Admitted empty or NULL URL: the /api key follows the omit flag of
// privfield.Redact, not the value.
// =============================================================================

// e2eKeyAbsent in a want map means "the key is not in the object". A nil
// want means the key is present with JSON null.
type e2eKeyAbsent struct{}

// TestE2E_FieldLevel_IxlanURL_AdmittedEmptyKeepsKey locks the pdbcompat
// key and value for an ixlan whose stored URL is empty or NULL. Upstream
// deletes the key only when the caller does not have the permission for
// the visibility (2.83.0 permissions.py:344-353). A caller that has the
// permission gets the key with the stored value: "" for "", and null for
// NULL (DRF renders None as null). The test covers the list, the depth=0
// detail and the ixlan_set of the parent ix at the default detail depth.
func TestE2E_FieldLevel_IxlanURL_AdmittedEmptyKeepsKey(t *testing.T) {
	t.Parallel()

	const (
		publicEmptyID  = 102
		usersEmptyID   = 103
		privateEmptyID = 104
		publicNullID   = 105
		usersNullID    = 106
	)
	absent := e2eKeyAbsent{}
	tiers := []struct {
		name string
		tier privctx.Tier
		want map[int]any // ixlan id -> url value, or e2eKeyAbsent
	}{
		{"anon", privctx.TierPublic, map[int]any{
			publicEmptyID: "", usersEmptyID: absent, privateEmptyID: absent,
			publicNullID: nil, usersNullID: absent,
		}},
		{"users", privctx.TierUsers, map[int]any{
			publicEmptyID: "", usersEmptyID: "", privateEmptyID: absent,
			publicNullID: nil, usersNullID: nil,
		}},
	}
	for _, tc := range tiers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fix := buildE2EFixture(t, tc.tier)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, r := range []struct {
				id      int
				visible string
				url     *string
			}{
				{publicEmptyID, "Public", new("")},
				{usersEmptyID, "Users", new("")},
				{privateEmptyID, "Private", new("")},
				{publicNullID, "Public", nil},
				{usersNullID, "Users", nil},
			} {
				fix.client.IxLan.Create().
					SetID(r.id).
					SetIxID(e2eIxID).
					SetNillableIxfIxpMemberListURL(r.url).
					SetIxfIxpMemberListURLVisible(r.visible).
					SetCreated(now).
					SetUpdated(now).
					SetStatus("ok").
					SaveX(t.Context())
			}

			check := func(t *testing.T, where string, row map[string]any) {
				t.Helper()
				idFloat, _ := row["id"].(float64)
				id := int(idFloat)
				want, ok := tc.want[id]
				if !ok {
					return
				}
				assertHasKey(t, row, "ixf_ixp_member_list_url_visible")
				got, present := row["ixf_ixp_member_list_url"]
				_, wantAbsent := want.(e2eKeyAbsent)
				switch {
				case wantAbsent && present:
					t.Errorf("%s ixlan %d: url key present (%#v), want absent", where, id, got)
				case !wantAbsent && !present:
					t.Errorf("%s ixlan %d: url key absent, want %#v", where, id, want)
				case !wantAbsent && got != want:
					t.Errorf("%s ixlan %d: url = %#v, want %#v", where, id, got, want)
				}
			}

			for id := range tc.want {
				body, status := mustGet(t, fmt.Sprintf("%s/api/ixlan/%d?depth=0", fix.server.URL, id))
				if status != http.StatusOK {
					t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", id, status, body)
				}
				check(t, "detail", extractPdbcompatFirst(t, body))
			}

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
			for _, row := range env.Data {
				check(t, "list", row)
			}

			body, status = mustGet(t, fmt.Sprintf("%s/api/ix/%d", fix.server.URL, e2eIxID))
			if status != http.StatusOK {
				t.Fatalf("GET /api/ix/%d: status=%d; body=%s", e2eIxID, status, body)
			}
			set, _ := extractPdbcompatFirst(t, body)["ixlan_set"].([]any)
			seen := 0
			for _, entry := range set {
				if row, ok := entry.(map[string]any); ok {
					if _, tracked := tc.want[int(row["id"].(float64))]; tracked {
						seen++
					}
					check(t, "ix.ixlan_set", row)
				}
			}
			if seen != len(tc.want) {
				t.Errorf("ix.ixlan_set holds %d of the %d seeded ixlans", seen, len(tc.want))
			}
		})
	}
}

// TestE2E_FieldLevel_IxlanURL_NullStored locks each surface for a Public
// ixlan with a NULL URL at TierPublic: /api, /rest/v1/ and GraphQL send
// the key with null, and ConnectRPC sends no wrapper. A Users ixlan with
// a NULL URL stays redacted at the raw ConnectRPC handler (fail-closed).
func TestE2E_FieldLevel_IxlanURL_NullStored(t *testing.T) {
	t.Parallel()

	const (
		publicNullID = 105
		usersNullID  = 106
	)
	fix := buildE2EFixture(t, privctx.TierPublic)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for id, visible := range map[int]string{publicNullID: "Public", usersNullID: "Users"} {
		fix.client.IxLan.Create().
			SetID(id).
			SetIxID(e2eIxID).
			SetIxfIxpMemberListURLVisible(visible).
			SetCreated(now).
			SetUpdated(now).
			SetStatus("ok").
			SaveX(t.Context())
	}
	idStr := strconv.Itoa(publicNullID)

	assertNullURL := func(t *testing.T, where string, obj map[string]any) {
		t.Helper()
		got, present := obj["ixf_ixp_member_list_url"]
		if !present || got != nil {
			t.Errorf("%s: url = %#v (present=%v), want null", where, got, present)
		}
		assertStringValue(t, obj, "ixf_ixp_member_list_url_visible", "Public")
	}

	t.Run("pdbcompat", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/ixlan/"+idStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/ixlan/%d: status=%d; body=%s", publicNullID, status, body)
		}
		assertNullURL(t, "/api", extractPdbcompatFirst(t, body))
	})

	t.Run("entrest", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/ix-lans/"+idStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/ix-lans/%d: status=%d; body=%s", publicNullID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode detail: %v\nbody=%s", err, body)
		}
		assertNullURL(t, "/rest/v1", obj)
	})

	t.Run("graphql", func(t *testing.T) {
		q := fmt.Sprintf(
			`{"query":"{ ixLans(where:{id: %d}) { edges { node { id ixfIxpMemberListURL ixfIxpMemberListURLVisible } } } }"}`,
			publicNullID,
		)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		node := extractGraphQLFirstIxLan(t, body)
		if got, present := node["ixfIxpMemberListURL"]; !present || got != nil {
			t.Errorf("url = %#v (present=%v), want null", got, present)
		}
		if v, _ := node["ixfIxpMemberListURLVisible"].(string); v != "Public" {
			t.Errorf("_visible = %q, want %q", v, "Public")
		}
	})

	t.Run("connectrpc", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetIxLan(t.Context(), &pbv1.GetIxLanRequest{Id: publicNullID})
		if err != nil {
			t.Fatalf("GetIxLan: %v", err)
		}
		if resp.IxLan.IxfIxpMemberListUrl != nil {
			t.Errorf("NULL url sent a wrapper = %v, want nil", resp.IxLan.IxfIxpMemberListUrl)
		}
		if got := resp.IxLan.IxfIxpMemberListUrlVisible.GetValue(); got != "Public" {
			t.Errorf("_visible = %q, want %q", got, "Public")
		}
	})

	// The raw ConnectRPC handler gets a context with no tier stamp.
	// privfield.Redact must fail closed for the Users row.
	t.Run("fail-closed-bypass-middleware", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(
			context.Background(), // deliberate: no tier stamp on ctx
			http.MethodPost,
			fix.rawIxLanPath+"GetIxLan",
			strings.NewReader(`{"id":`+strconv.Itoa(usersNullID)+`}`),
		)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		fix.rawIxLanHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("handler status = %d (body=%s), want 200", rec.Code, rec.Body.String())
		}
		var resp struct {
			IxLan map[string]any `json:"ixLan"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, rec.Body.String())
		}
		if got, present := resp.IxLan["ixfIxpMemberListUrl"]; present {
			t.Errorf("unstamped ctx sent url = %#v; want no wrapper", got)
		}
		if v, _ := resp.IxLan["ixfIxpMemberListUrlVisible"].(string); v != "Users" {
			t.Errorf("_visible = %q, want %q\nbody=%s", v, "Users", rec.Body.String())
		}
	})
}

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
		assertHasKey(t, row, "ixf_ixp_member_list_url_visible") // companion always emitted
		assertLacksKey(t, row, "ixf_ixp_member_list_url")       // gated URL redacted
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

	// Eager-loaded edges on sibling entrest endpoints — regression for the
	// 2026-06-10 audit finding: entrest unconditionally eager-loads the
	// ix_lans edge on internet-exchange responses, so redaction must reach
	// ixlan objects embedded under edges.ix_lans, not just /rest/v1/ix-lans*.
	t.Run("entrest/embedded/ix-detail", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/internet-exchanges/"+strconv.Itoa(e2eIxID))
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/internet-exchanges/%d: status=%d; body=%s", e2eIxID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode ix detail: %v\nbody=%s", err, body)
		}
		assertIxlanListShape(t, extractEmbeddedIxLans(t, obj), fix.gatedIxLanID, fix.publicIxLanID)
	})

	t.Run("entrest/embedded/ix-list", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/internet-exchanges")
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/internet-exchanges: status=%d; body=%s", status, body)
		}
		var env struct {
			Content []map[string]any `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /rest/v1/internet-exchanges: %v\nbody=%s", err, body)
		}
		for _, ix := range env.Content {
			if id, ok := ix["id"].(float64); ok && int(id) == e2eIxID {
				assertIxlanListShape(t, extractEmbeddedIxLans(t, ix), fix.gatedIxLanID, fix.publicIxLanID)
				return
			}
		}
		t.Fatalf("seeded IX id=%d not found in list; body=%s", e2eIxID, body)
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
			t.Error("expected _visible companion to remain populated")
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

	// Stream surface: the StreamIxLans Convert closure captures the
	// handler ctx by reference via an adapter; this asserts the
	// captured tier reaches privfield.Redact so the gated URL is blanked
	// on the streaming path, not just the unary Get/List paths.
	t.Run("connectrpc/stream", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		stream, err := cl.StreamIxLans(t.Context(), &pbv1.StreamIxLansRequest{})
		if err != nil {
			t.Fatalf("StreamIxLans: %v", err)
		}
		defer func() { _ = stream.Close() }()
		var gatedSeen, publicSeen bool
		for stream.Receive() {
			il := stream.Msg()
			switch il.Id {
			case int64(fix.gatedIxLanID):
				gatedSeen = true
				if il.IxfIxpMemberListUrl != nil {
					t.Errorf("anon tier stream received url for gated row id=%d", il.Id)
				}
			case int64(fix.publicIxLanID):
				publicSeen = true
				if il.IxfIxpMemberListUrl == nil {
					t.Errorf("public-visible row id=%d must admit url for all tiers", il.Id)
				}
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if !gatedSeen || !publicSeen {
			t.Fatalf("expected both gated=%d and public=%d ixlan rows in stream, saw gated=%v public=%v",
				fix.gatedIxLanID, fix.publicIxLanID, gatedSeen, publicSeen)
		}
	})

	// Surface-level fail-closed. Construct the request against the
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
			t.Errorf("_visible companion missing; regression\nbody=%s", rec.Body.String())
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
			t.Errorf("_visible = %q, want %q", v, "Users")
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
	// ixf_ixp_member_list_url. When it is later added to
	// /ui/ixlan/{id} or a fragment, extend this sub-test to parse the
	// rendered HTML and assert the URL is NOT present at TierPublic.
	// -------------------------------------------------------------------------
	t.Run("webui", func(t *testing.T) {
		t.Skip("UI does not render ixf_ixp_member_list_url")
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

	// Users tier must still see the gated URL inside eager-loaded edges —
	// guards the recursive redaction walker against over-redacting.
	t.Run("entrest/embedded/ix-detail", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/internet-exchanges/"+strconv.Itoa(e2eIxID))
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/internet-exchanges/%d: status=%d; body=%s", e2eIxID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode ix detail: %v\nbody=%s", err, body)
		}
		var gatedSeen bool
		for _, row := range extractEmbeddedIxLans(t, obj) {
			if id, ok := row["id"].(float64); ok && int(id) == fix.gatedIxLanID {
				gatedSeen = true
				assertStringValue(t, row, "ixf_ixp_member_list_url", e2eGatedIxlanURL)
			}
		}
		if !gatedSeen {
			t.Fatalf("gated ixlan id=%d not embedded in IX detail; body=%s", fix.gatedIxLanID, body)
		}
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

	t.Run("connectrpc/stream", func(t *testing.T) {
		cl := peeringdbv1connect.NewIxLanServiceClient(http.DefaultClient, fix.server.URL)
		stream, err := cl.StreamIxLans(t.Context(), &pbv1.StreamIxLansRequest{})
		if err != nil {
			t.Fatalf("StreamIxLans: %v", err)
		}
		defer func() { _ = stream.Close() }()
		var gatedSeen, publicSeen bool
		for stream.Receive() {
			il := stream.Msg()
			switch il.Id {
			case int64(fix.gatedIxLanID):
				gatedSeen = true
				if got := il.IxfIxpMemberListUrl.GetValue(); got != e2eGatedIxlanURL {
					t.Errorf("users tier, gated stream url = %q, want %q", got, e2eGatedIxlanURL)
				}
			case int64(fix.publicIxLanID):
				publicSeen = true
				if got := il.IxfIxpMemberListUrl.GetValue(); got != e2ePublicIxlanURL {
					t.Errorf("users tier, public stream url = %q, want %q", got, e2ePublicIxlanURL)
				}
			}
		}
		if err := stream.Err(); err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if !gatedSeen || !publicSeen {
			t.Fatalf("users tier stream missing rows: gated=%v public=%v", gatedSeen, publicSeen)
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
		t.Skip("UI does not render ixf_ixp_member_list_url")
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

// extractEmbeddedIxLans pulls the eager-loaded edges.ix_lans array out of
// an entrest internet-exchange JSON object.
func extractEmbeddedIxLans(t *testing.T, obj map[string]any) []map[string]any {
	t.Helper()
	edges, ok := obj["edges"].(map[string]any)
	if !ok {
		t.Fatalf("internet-exchange object has no edges map: %+v", obj)
	}
	raw, ok := edges["ix_lans"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("internet-exchange edges has no eager-loaded ix_lans: %+v", edges)
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		if m, ok := entry.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

// Compile-time reference to io so "unused import" lint doesn't trip if
// the helpers get refactored to a different body-reader path. The
// existing e2e_privacy_test.go already depends on io; this is defensive.
var _ = io.Discard
