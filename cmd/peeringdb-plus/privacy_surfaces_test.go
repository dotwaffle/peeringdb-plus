// Package main privacy_surfaces_test.go — Phase 60-02 per-surface privacy
// list-count regression tests against the canonical seed.Full fixture.
//
// This test is complementary to Phase 59's
// TestE2E_AnonymousCannotSeeUsersPoc (e2e_privacy_test.go), which asserts
// "row absent" on an ephemeral 1-POC fixture. Plan 60-02's version
// asserts list-count shapes against the canonical seed.Full corpus
// (3 POCs: 1 Public + 2 Users), so it catches regressions where the
// privacy filter passes scalar containment checks but drops the wrong
// count (off-by-one, duplicated row, mis-filtered parent row).
//
// Coverage (one sub-test per surface × scenario):
//
//   - pdbcompat /api/poc        — list count (exactly 1 row, id=500, visible="Public")
//   - pdbcompat /api/poc/9000   — detail returns 404
//   - entrest /rest/v1/pocs     — list count (exactly 1 row, id=500)
//   - entrest /rest/v1/pocs/9000 — detail returns 404
//   - GraphQL pocs(first: 10)   — totalCount == 1, single node id "500"
//   - ConnectRPC ListPocs       — len(resp.Pocs) == 1, id == 500
//   - ConnectRPC GetPoc(9000)   — connect.CodeNotFound (D-13/D-14)
//   - /ui/asn/13335             — rendered HTML does not contain Users POC name/email
//   - /ui/fragment/net/10/contacts — rendered HTML does not contain Users POC name/email
//
// All requests go through buildMiddlewareChain (the real production
// middleware chain) via httptest.NewServer — direct handler invocation
// via httptest.NewRecorder is explicitly forbidden by D-07 because it
// bypasses the privacy-tier middleware.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	pbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/graph"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
	"github.com/dotwaffle/peeringdb-plus/internal/web"
)

// surfacesDBCounter yields isolated in-memory SQLite DBs so parallel
// sub-tests within this file (and cross-file parallel tests) never share
// state.
var surfacesDBCounter atomic.Int64

// surfacesFixture bundles the live httptest.Server (wrapping the real
// production middleware chain) together with the seed.Result so sub-tests
// can target the exact IDs that were seeded.
type surfacesFixture struct {
	server *httptest.Server
	client *ent.Client
	seed   *seed.Result
}

// buildPrivacySurfacesFixture wires the 5-surface mux (GraphQL, entrest,
// pdbcompat, /ui/, ConnectRPC PocService) around a seed.Full-populated
// ent client, then wraps the mux in buildMiddlewareChain with
// DefaultTier=TierPublic — this is what an anonymous caller sees in
// production. The fixture pattern mirrors buildE2EFixture
// (e2e_privacy_test.go) but swaps the ephemeral 1-POC seed for seed.Full
// so list-count assertions run against the canonical mixed-visibility
// corpus (1 Public POC + 2 Users POCs).
//
// Duplicating ~80 lines rather than refactoring buildE2EFixture into a
// parameterised helper was intentional per the plan — a cross-test
// refactor would couple the two test files, and this test's fixture
// shape (seed.Full instead of a hand-rolled org/net/poc triple) differs
// enough that a shared helper would need an options pattern for limited
// gain.
func buildPrivacySurfacesFixture(t *testing.T) *surfacesFixture {
	t.Helper()

	// Isolated in-memory SQLite per test run. cache=shared lets the
	// second sql.DB handle (opened for the /ui/ stack) observe the same
	// in-memory database as the ent client.
	id := surfacesDBCounter.Add(1)
	dsn := fmt.Sprintf("file:privacy_surfaces_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = client.Close() })

	// Second handle for the /ui/ stack. /ui/ reads sync_status via raw
	// *sql.DB; the table does not exist in this fixture and h.getFreshness
	// tolerates that via zero-value time.Time (matches e2e_privacy_test.go
	// comment at line 122).
	rawDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	// Seed the canonical fixture. seed.Full installs:
	//   r.Poc       id=500  visible="Public" network=ASN 13335
	//   r.UsersPoc  id=9000 visible="Users"  network=ASN 13335
	//   r.UsersPoc2 id=9001 visible="Users"  network=ASN 6939
	// The UsersPoc rows are created via privacy.DecisionContext(Allow)
	// inside seed.Full (the sync worker's bypass pattern); the bypass
	// audit (internal/sync/bypass_audit_test.go) exempts testutil.
	r := seed.Full(t, client)

	// Wire the 5 surfaces onto a fresh mux. Order mirrors the TierPublic
	// leg of buildE2EFixture so any drift between the two fixtures is
	// easy to diff.
	mux := http.NewServeMux()

	// GraphQL (POST /graphql). Only POST — the GET playground is not
	// exercised here.
	resolver := graph.NewResolver(client, rawDB)
	gqlHandler := pdbgql.NewHandler(resolver)
	mux.Handle("POST /graphql", gqlHandler)

	// entrest (/rest/v1/). Wrap with the same restCORS + restErrorMiddleware
	// pair as production main.go so response shapes match the wire.
	restSrv, err := rest.NewServer(client, &rest.ServerConfig{BasePath: "/rest/v1"})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}
	restCORS := middleware.CORS(middleware.CORSInput{AllowedOrigins: "*"})
	mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))

	// pdbcompat (/api/…).
	compatHandler := pdbcompat.NewHandler(client)
	compatHandler.Register(mux)

	// Web UI (/ui/ and /static/, plus /favicon.ico).
	webHandler := web.NewHandler(web.NewHandlerInput{Client: client, DB: rawDB})
	webHandler.Register(mux)

	// ConnectRPC PocService. Only the Poc service — this plan's scope
	// is POCs; bringing up the other 12 services would pad setup without
	// adding coverage.
	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithoutServerPeerAttributes(),
		otelconnect.WithoutTraceEvents(),
	)
	if err != nil {
		t.Fatalf("create otel interceptor: %v", err)
	}
	handlerOpts := connect.WithInterceptors(otelInterceptor)
	pocPath, pocHandler := peeringdbv1connect.NewPocServiceHandler(
		&grpcserver.PocService{Client: client, StreamTimeout: 60 * time.Second},
		handlerOpts,
	)
	mux.Handle(pocPath, pocHandler)

	// Wrap the mux in the full production middleware chain with
	// DefaultTier=TierPublic. This is the assertion centre: flipping
	// DefaultTier to TierUsers would admit the Users POCs on every
	// surface (that's SYNC-03, covered by Phase 59's E2E TierUsers
	// tests, not re-tested here).
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cachingState := middleware.NewCachingState(1 * time.Hour)
	handler := buildMiddlewareChain(mux, chainConfig{
		Logger:      logger,
		CORSOrigins: "*",
		CSPInput: middleware.CSPInput{
			UIPolicy:      "default-src 'self'",
			GraphQLPolicy: "default-src 'self'",
			EnforcingMode: false,
		},
		CachingState: cachingState,
		SyncWorker:   e2eAlwaysReady{}, // reuse from e2e_privacy_test.go
		MaxBodyBytes: maxRequestBodySize,
		HSTSMaxAge:   0,
		DefaultTier:  privctx.TierPublic,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &surfacesFixture{
		server: srv,
		client: client,
		seed:   r,
	}
}

// surfacesGet issues a GET through the fixture server. Mirrors mustGet in
// e2e_privacy_test.go but namespaces the helper to this file so the two
// test files can evolve independently.
func surfacesGet(t *testing.T, url string) ([]byte, int) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	// Force HTML content negotiation on /ui/ so the browser path renders
	// (the default would yield ANSI terminal output per CLAUDE.md §"/ui/
	// content negotiation").
	req.Header.Set("User-Agent", "Mozilla/5.0 (privacy_surfaces_test)")
	req.Header.Set("Accept", "text/html,application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s body: %v", url, err)
	}
	return body, resp.StatusCode
}

// surfacesPostJSON issues a POST with a JSON body (GraphQL).
func surfacesPostJSON(t *testing.T, url, jsonBody string) ([]byte, int) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, strings.NewReader(jsonBody))
	if err != nil {
		t.Fatalf("build POST %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read POST %s body: %v", url, err)
	}
	return body, resp.StatusCode
}

// snipBody returns up to 1 KB of a response body, with an ellipsis
// marker if truncated. Used in failure messages so privacy regressions
// aren't silent — a "wrong count" is easy to debug when the actual rows
// are in the error output.
func snipBody(body []byte) string {
	const limit = 1024
	if len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "…(truncated)"
}

// TestPrivacySurfaces asserts the per-surface privacy contract against
// the canonical seed.Full corpus: an anonymous caller through the real
// production middleware chain sees exactly 1 POC (r.Poc, id=500) on
// every list endpoint, and the Users-tier IDs (9000/9001) return each
// surface's native not-found idiom (404 / CodeNotFound / empty GraphQL
// edges / HTML without the gated strings).
//
// This is the list-count/shape companion to Phase 59's
// TestE2E_AnonymousCannotSeeUsersPoc. That test asserts "row absent" on
// a 1-POC fixture; this test asserts "list contains exactly the Public
// row(s), no more, no fewer" on the 3-POC fixture. The two catch
// different failure modes (pass-through vs miscount).
func TestPrivacySurfaces(t *testing.T) {
	t.Parallel()
	fix := buildPrivacySurfacesFixture(t)

	// Convenience locals. r.Poc is the single Public POC; r.UsersPoc and
	// r.UsersPoc2 are the gated rows that must never leak.
	publicPocID := fix.seed.Poc.ID           // 500
	usersPocID := fix.seed.UsersPoc.ID       // 9000
	usersPoc2ID := fix.seed.UsersPoc2.ID     // 9001
	usersPocName := fix.seed.UsersPoc.Name   // "Users-Tier NOC"
	usersPocEmail := fix.seed.UsersPoc.Email // "users-noc@example.invalid"
	usersPoc2Name := fix.seed.UsersPoc2.Name // "Users-Tier Policy"
	networkASN := fix.seed.Network.Asn       // 13335
	networkID := fix.seed.Network.ID         // 10

	// --------------------------------------------------------------------
	// Surface 1: pdbcompat /api
	// --------------------------------------------------------------------
	t.Run("pdbcompat_list_count", func(t *testing.T) {
		t.Parallel()
		body, status := surfacesGet(t, fix.server.URL+"/api/poc")
		if status != http.StatusOK {
			t.Fatalf("GET /api/poc: status=%d, want 200; body=%s", status, snipBody(body))
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /api/poc: %v\nbody=%s", err, snipBody(body))
		}
		if len(env.Data) != 1 {
			t.Fatalf("GET /api/poc: len(data)=%d, want 1 (only r.Poc id=%d should be visible); body=%s",
				len(env.Data), publicPocID, snipBody(body))
		}
		row := env.Data[0]
		gotID, ok := row["id"].(float64)
		if !ok || int(gotID) != publicPocID {
			t.Fatalf("GET /api/poc: data[0].id=%v, want %d; row=%+v", row["id"], publicPocID, row)
		}
		if vis, _ := row["visible"].(string); vis != "Public" {
			t.Fatalf("GET /api/poc: data[0].visible=%q, want %q; row=%+v", vis, "Public", row)
		}
		// Belt-and-braces: explicitly assert the Users IDs are absent.
		// A wrong-count test that still passed with a leaked row would
		// be a logic bug in the assertion; this makes the contract
		// unambiguous.
		for _, r := range env.Data {
			if idF, ok := r["id"].(float64); ok {
				idI := int(idF)
				if idI == usersPocID || idI == usersPoc2ID {
					t.Fatalf("Users POC leaked into /api/poc: row=%+v", r)
				}
			}
		}
	})

	t.Run("pdbcompat_detail_404", func(t *testing.T) {
		t.Parallel()
		url := fmt.Sprintf("%s/api/poc/%d", fix.server.URL, usersPocID)
		body, status := surfacesGet(t, url)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s: status=%d, want 404 (D-13: surface-native not-found, NOT 403); body=%s",
				url, status, snipBody(body))
		}
	})

	// --------------------------------------------------------------------
	// Surface 2: entrest /rest/v1/
	// --------------------------------------------------------------------
	t.Run("rest_list_count", func(t *testing.T) {
		t.Parallel()
		body, status := surfacesGet(t, fix.server.URL+"/rest/v1/pocs")
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/pocs: status=%d, want 200; body=%s", status, snipBody(body))
		}
		var env struct {
			Content []map[string]any `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /rest/v1/pocs: %v\nbody=%s", err, snipBody(body))
		}
		if len(env.Content) != 1 {
			t.Fatalf("GET /rest/v1/pocs: len(content)=%d, want 1 (only r.Poc id=%d should be visible); body=%s",
				len(env.Content), publicPocID, snipBody(body))
		}
		row := env.Content[0]
		gotID, ok := row["id"].(float64)
		if !ok || int(gotID) != publicPocID {
			t.Fatalf("GET /rest/v1/pocs: content[0].id=%v, want %d; row=%+v", row["id"], publicPocID, row)
		}
		for _, r := range env.Content {
			if idF, ok := r["id"].(float64); ok {
				idI := int(idF)
				if idI == usersPocID || idI == usersPoc2ID {
					t.Fatalf("Users POC leaked into /rest/v1/pocs: row=%+v", r)
				}
			}
		}
	})

	t.Run("rest_detail_404", func(t *testing.T) {
		t.Parallel()
		url := fmt.Sprintf("%s/rest/v1/pocs/%d", fix.server.URL, usersPocID)
		body, status := surfacesGet(t, url)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s: status=%d, want 404; body=%s", url, status, snipBody(body))
		}
	})

	// --------------------------------------------------------------------
	// Surface 3: GraphQL
	//
	// The `pocs(first: N)` connection takes paginated args and returns
	// `edges { node { ... } } totalCount`. This is the canonical list
	// shape in entgql-generated schemas. ID is a String in the generated
	// schema (see graph/schema.graphql line 5039: `id: ID!`), so the
	// decoded struct uses `string`.
	// --------------------------------------------------------------------
	t.Run("graphql_list_count", func(t *testing.T) {
		t.Parallel()
		q := `{"query":"{ pocs(first: 10) { edges { node { id name visible } } totalCount } }"}`
		body, status := surfacesPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, snipBody(body))
		}
		var env struct {
			Data struct {
				Pocs struct {
					Edges []struct {
						Node struct {
							ID      string `json:"id"`
							Name    string `json:"name"`
							Visible string `json:"visible"`
						} `json:"node"`
					} `json:"edges"`
					TotalCount int `json:"totalCount"`
				} `json:"pocs"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /graphql: %v\nbody=%s", err, snipBody(body))
		}
		if len(env.Errors) > 0 {
			t.Fatalf("graphql errors: %+v; body=%s", env.Errors, snipBody(body))
		}
		if env.Data.Pocs.TotalCount != 1 {
			t.Fatalf("pocs.totalCount=%d, want 1; body=%s", env.Data.Pocs.TotalCount, snipBody(body))
		}
		if len(env.Data.Pocs.Edges) != 1 {
			t.Fatalf("len(pocs.edges)=%d, want 1; body=%s", len(env.Data.Pocs.Edges), snipBody(body))
		}
		gotID := env.Data.Pocs.Edges[0].Node.ID
		wantID := strconv.Itoa(publicPocID)
		if gotID != wantID {
			t.Fatalf("pocs.edges[0].node.id=%q, want %q; body=%s", gotID, wantID, snipBody(body))
		}
		for _, e := range env.Data.Pocs.Edges {
			if e.Node.Visible == "Users" {
				t.Fatalf("Users POC leaked into graphql pocs: node=%+v", e.Node)
			}
			if e.Node.ID == strconv.Itoa(usersPocID) || e.Node.ID == strconv.Itoa(usersPoc2ID) {
				t.Fatalf("Users POC id %s leaked into graphql pocs: node=%+v", e.Node.ID, e.Node)
			}
		}
	})

	// --------------------------------------------------------------------
	// Surface 4: ConnectRPC PocService
	// --------------------------------------------------------------------
	t.Run("grpc_list_count", func(t *testing.T) {
		t.Parallel()
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.ListPocs(t.Context(), &pbv1.ListPocsRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("ListPocs: %v", err)
		}
		pocs := resp.GetPocs()
		if len(pocs) != 1 {
			t.Fatalf("ListPocs: len(pocs)=%d, want 1; got=%+v", len(pocs), pocs)
		}
		if got := pocs[0].GetId(); got != int64(publicPocID) {
			t.Fatalf("ListPocs: pocs[0].id=%d, want %d", got, publicPocID)
		}
		for _, p := range pocs {
			id := int(p.GetId())
			if id == usersPocID || id == usersPoc2ID {
				t.Fatalf("Users POC leaked into ListPocs: poc=%+v", p)
			}
		}
	})

	t.Run("grpc_detail_notfound", func(t *testing.T) {
		t.Parallel()
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		_, err := cl.GetPoc(t.Context(), &pbv1.GetPocRequest{Id: int64(usersPocID)})
		if err == nil {
			t.Fatalf("GetPoc(%d): expected error (CodeNotFound), got nil", usersPocID)
		}
		// D-13/D-14: the idiom MUST be CodeNotFound. CodePermissionDenied
		// (the 403 analogue) would distinguish "filtered" from "missing"
		// and leak row existence.
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			var ce *connect.Error
			if errors.As(err, &ce) {
				t.Fatalf("GetPoc(%d): code=%s, want CodeNotFound (D-13); message=%q",
					usersPocID, code, ce.Message())
			}
			t.Fatalf("GetPoc(%d): code=%s, want CodeNotFound (D-13); err=%v",
				usersPocID, code, err)
		}
	})

	// --------------------------------------------------------------------
	// Surface 5: /ui/
	//
	// The /ui/asn/{asn} page renders the network detail and includes a
	// POC count (not full POC content — see internal/web/query_network.go).
	// Asserting name/email absence on that page is trivially satisfied
	// because the template never includes those fields. We also exercise
	// /ui/fragment/net/{netID}/contacts, which IS where POC rows render,
	// so this is the meaningful end of the /ui/ privacy contract.
	// --------------------------------------------------------------------
	t.Run("ui_network_detail_no_leak", func(t *testing.T) {
		t.Parallel()
		url := fmt.Sprintf("%s/ui/asn/%d", fix.server.URL, networkASN)
		body, status := surfacesGet(t, url)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status=%d, want 200; body=%s", url, status, snipBody(body))
		}
		// Sanity: the network name must be present — otherwise the
		// wrong page rendered and the "no Users POC leak" assertion is
		// meaningless.
		if !strings.Contains(string(body), fix.seed.Network.Name) {
			t.Fatalf("GET %s: network name %q missing from page; wrong page rendered?\nbody=%s",
				url, fix.seed.Network.Name, snipBody(body))
		}
		// Strong contract: gated POC fields must not appear in the
		// rendered HTML under any codepath.
		if strings.Contains(string(body), usersPocName) {
			t.Fatalf("Users POC name %q leaked into /ui/asn/%d HTML", usersPocName, networkASN)
		}
		if strings.Contains(string(body), usersPocEmail) {
			t.Fatalf("Users POC email %q leaked into /ui/asn/%d HTML", usersPocEmail, networkASN)
		}
	})

	t.Run("ui_contacts_fragment_no_leak", func(t *testing.T) {
		t.Parallel()
		// The contacts fragment is the /ui/ surface where POC rows
		// actually render. Phase 59's E2E test covers this on the
		// 1-POC fixture; re-asserting on seed.Full proves the privacy
		// filter composes with a real mixed corpus (r.Poc visible,
		// r.UsersPoc gated on the same network).
		url := fmt.Sprintf("%s/ui/fragment/net/%d/contacts", fix.server.URL, networkID)
		body, status := surfacesGet(t, url)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status=%d; body=%s", url, status, snipBody(body))
		}
		if strings.Contains(string(body), usersPocName) {
			t.Fatalf("Users POC name %q leaked into /ui/ contacts fragment HTML on network %d",
				usersPocName, networkID)
		}
		if strings.Contains(string(body), usersPocEmail) {
			t.Fatalf("Users POC email %q leaked into /ui/ contacts fragment HTML on network %d",
				usersPocEmail, networkID)
		}
		// Also cover r.UsersPoc2 (different network) — it should never
		// appear on r.Network's fragment regardless of privacy state.
		if strings.Contains(string(body), usersPoc2Name) {
			t.Fatalf("Cross-network Users POC2 name %q leaked into /ui/ contacts fragment for network %d",
				usersPoc2Name, networkID)
		}
	})
}
