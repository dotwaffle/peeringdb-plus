// Package main e2e_privacy_test.go — Phase 59-06 D-15 end-to-end privacy
// contract.
//
// This test exercises the full production middleware chain
// (buildMiddlewareChain) wrapped around the real per-surface handlers
// (pdbcompat /api, entrest /rest/v1, ConnectRPC /peeringdb.v1.*,
// gqlgen /graphql, web /ui) against an in-memory ent client seeded with a
// single visible="Users" POC. The seeding goes through
// privacy.DecisionContext(Allow) — mirroring the sync worker's sole
// bypass (internal/sync/worker.go D-08/D-09) so the test proves Plan
// 59-05 and Plan 59-04 compose correctly end-to-end.
//
// The audit in internal/sync/bypass_audit_test.go exempts *_test.go, so
// the test-only bypass is legitimate (see its godoc).
//
// Two top-level tests:
//
//   - TestE2E_AnonymousCannotSeeUsersPoc   (TierPublic: row HIDDEN on all 5 surfaces)
//   - TestE2E_PublicTierUsersAdmitsRow     (TierUsers:  row VISIBLE on all 5 surfaces)
//
// Across both we cover 5 surfaces × detail + list pattern so a single
// surface regression doesn't mask others. Every sub-test runs against
// the same httptest.Server instance per tier to amortize setup cost.
//
// Per D-13/D-14, all surfaces must return their native not-found idiom
// (404 / CodeNotFound / GraphQL null) for hidden rows — NEVER 403 — so
// the existence-leak attack (distinguishing "filtered" from "missing")
// is closed.
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
	"entgo.io/ent/privacy"

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
	"github.com/dotwaffle/peeringdb-plus/internal/web"
)

// e2eDBCounter yields isolated in-memory SQLite DBs for parallel tests.
var e2eDBCounter atomic.Int64

// e2eUsersPocID is the fixed ID of the visible="Users" POC seeded by the
// fixture. Chosen well above seed.Full's range (<= ~700) to avoid any
// accidental collision if seed.Full is added in future.
const e2eUsersPocID = 900001

// e2eUsersNetworkID is the fixed ID of the owning network. The test must
// seed the network first because poc.net_id is the FK.
const e2eUsersNetworkID = 900001

// e2eUsersOrgID is the fixed ID of the owning org. Required because
// network.org_id is a non-optional FK.
const e2eUsersOrgID = 900001

// e2eAlwaysReady is the readiness bypass used by the test fixture. The
// production readinessMiddleware 503s every non-infrastructure path until
// HasCompletedSync() returns true. The E2E test is about privacy, not
// readiness, so we keep this loud-and-simple rather than plumbing a real
// sync worker into the fixture.
type e2eAlwaysReady struct{}

func (e2eAlwaysReady) HasCompletedSync() bool { return true }

// e2eFixture bundles the server and per-test metadata for both the
// TierPublic and TierUsers runs. The httptest.Server wraps the full
// production middleware chain (buildMiddlewareChain), which is the
// assertion's centre of gravity — any mis-ordering or missing wiring
// would make the TierUsers assertions fail (because the row is always
// hidden without the tier stamp).
type e2eFixture struct {
	server  *httptest.Server
	client  *ent.Client
	pocID   int
	netID   int
	pocName string
}

// buildE2EFixture constructs a fresh in-memory ent client with one
// visible="Users" POC, wires every production surface onto a new mux,
// and wraps the mux in the full production middleware chain using the
// caller-provided tier. The returned httptest.Server is torn down via
// t.Cleanup.
//
// The bypass seeding (privacy.DecisionContext) is the same mechanism the
// sync worker uses at runtime; the audit test in
// internal/sync/bypass_audit_test.go explicitly exempts *_test.go files,
// so there is no audit regression here.
func buildE2EFixture(t *testing.T, tier privctx.Tier) *e2eFixture {
	t.Helper()

	// Isolated in-memory SQLite per parallel test.
	id := e2eDBCounter.Add(1)
	dsn := fmt.Sprintf("file:e2e_privacy_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = client.Close() })

	// Open a second handle to the same in-memory DB for the /ui/ stack,
	// which reads sync_status via raw *sql.DB. The table is never
	// created — /ui/ queries tolerate a missing sync_status gracefully
	// via h.getFreshness (returns zero time.Time). We only need the
	// handle to exist so web.NewHandler can stash it.
	rawDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	ctx := t.Context()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed org + network so the POC's FK resolves. These rows are
	// visible-agnostic (Public by default); the network is what the
	// TierUsers list queries traverse to find the gated POC.
	org := client.Organization.Create().
		SetID(e2eUsersOrgID).
		SetName("E2E Privacy Org").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SaveX(ctx)

	net := client.Network.Create().
		SetID(e2eUsersNetworkID).
		SetName("E2E Privacy Net").
		SetAsn(64999).
		SetInfoUnicast(true).
		SetInfoMulticast(false).
		SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).
		SetPolicyRatio(false).
		SetAllowIxpUpdate(false).
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SetOrganization(org).
		SaveX(ctx)

	// Seed the visible="Users" POC via the privacy bypass. This is
	// what the sync worker does at runtime; here we do it directly in
	// the test to isolate the assertion from any sync machinery.
	bypass := privacy.DecisionContext(ctx, privacy.Allow)
	const pocName = "E2E Users-Only Contact"
	client.Poc.Create().
		SetID(e2eUsersPocID).
		SetNetworkID(net.ID).
		SetRole("NOC").
		SetName(pocName).
		SetEmail("users-only@example.invalid").
		SetVisible("Users").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SaveX(bypass)

	// Wire every production surface onto a fresh mux. This mirrors the
	// registration order in cmd/peeringdb-plus/main.go (main()) but
	// omits /sync, /healthz, /readyz, /{$}, and the OTel setup — none
	// of those affect the privacy assertion.
	mux := http.NewServeMux()

	// GraphQL (/graphql). Accept POST only — the production handler
	// also serves a GET playground but this test only issues POST
	// queries.
	resolver := graph.NewResolver(client, rawDB)
	gqlHandler := pdbgql.NewHandler(resolver)
	mux.Handle("POST /graphql", gqlHandler)

	// entrest (/rest/v1/). Matches main.go: wrap with restCORS and the
	// restErrorWriter so response shapes match production.
	restSrv, err := rest.NewServer(client, &rest.ServerConfig{BasePath: "/rest/v1"})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}
	restCORS := middleware.CORS(middleware.CORSInput{AllowedOrigins: "*"})
	mux.Handle("/rest/v1/", restCORS(restErrorMiddleware(restSrv.Handler())))

	// pdbcompat (/api/). Registers /api/{rest...} internally.
	compatHandler := pdbcompat.NewHandler(client)
	compatHandler.Register(mux)

	// Web UI (/ui/ and /static/, plus /favicon.ico).
	webHandler := web.NewHandler(web.NewHandlerInput{Client: client, DB: rawDB})
	webHandler.Register(mux)

	// ConnectRPC (/peeringdb.v1.PocService/*). We only need the Poc
	// service for the assertion — any other service on the prod mux
	// is privacy-irrelevant here and would pull in more setup.
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

	// Build the full production middleware chain. DefaultTier is the
	// lever we're testing: flipping it between TierPublic and TierUsers
	// must flip row visibility for every anonymous request across
	// every surface.
	//
	// Discarded slog logger keeps test output clean; the middlewares
	// still exercise their code paths, they just don't print.
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
		SyncWorker:   e2eAlwaysReady{},
		MaxBodyBytes: maxRequestBodySize,
		HSTSMaxAge:   0,
		DefaultTier:  tier,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &e2eFixture{
		server:  srv,
		client:  client,
		pocID:   e2eUsersPocID,
		netID:   e2eUsersNetworkID,
		pocName: pocName,
	}
}

// mustGet issues a GET and returns (body, status). Fatals on transport
// errors. The caller is responsible for asserting status and content.
func mustGet(t *testing.T, url string) ([]byte, int) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	// Force HTML content negotiation on the /ui/ surface so the
	// browser-path code paths run (including /ui/asn/{asn}, which
	// otherwise returns ANSI-styled terminal text).
	req.Header.Set("User-Agent", "Mozilla/5.0 (e2e_privacy_test)")
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

// mustPostJSON issues a POST with the given JSON body.
func mustPostJSON(t *testing.T, url, jsonBody string) ([]byte, int) {
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

// decodeGraphQL parses a GraphQL response envelope.
type gqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

func decodeGraphQL(t *testing.T, body []byte) gqlResponse {
	t.Helper()
	var r gqlResponse
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode graphql response: %v\nbody=%s", err, body)
	}
	return r
}

// =============================================================================
// TierPublic: anonymous caller must NOT see the visible="Users" POC.
// =============================================================================

// TestE2E_AnonymousCannotSeeUsersPoc is the D-15 correctness contract
// for the default (anonymous) tier. With PDBPLUS_PUBLIC_TIER unset
// (chainConfig.DefaultTier == TierPublic), a visible="Users" POC
// seeded via the sync bypass MUST be absent from anonymous reads on
// all 5 surfaces.
//
// Each surface's sub-test asserts the surface-native not-found idiom
// per D-13/D-14 — NEVER 403. Distinguishing "filtered" from "missing"
// would leak row existence.
func TestE2E_AnonymousCannotSeeUsersPoc(t *testing.T) {
	t.Parallel()
	fix := buildE2EFixture(t, privctx.TierPublic)
	idStr := strconv.Itoa(fix.pocID)

	// -------------------------------------------------------------------------
	// Surface 1: pdbcompat /api (detail + list)
	// -------------------------------------------------------------------------
	t.Run("pdbcompat_detail_404", func(t *testing.T) {
		_, status := mustGet(t, fix.server.URL+"/api/poc/"+idStr)
		if status != http.StatusNotFound {
			t.Fatalf("GET /api/poc/%d: status=%d, want 404 (D-13: surface-native not-found)", fix.pocID, status)
		}
	})

	t.Run("pdbcompat_list_absent", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/poc")
		if status != http.StatusOK {
			t.Fatalf("GET /api/poc: status=%d, want 200; body=%s", status, body)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /api/poc: %v\nbody=%s", err, body)
		}
		for _, row := range env.Data {
			if idFloat, ok := row["id"].(float64); ok && int(idFloat) == fix.pocID {
				t.Fatalf("Users POC leaked into /api/poc list: %+v", row)
			}
		}
	})

	// -------------------------------------------------------------------------
	// Surface 2: entrest /rest/v1/pocs (detail + list)
	// -------------------------------------------------------------------------
	t.Run("rest_detail_404", func(t *testing.T) {
		_, status := mustGet(t, fix.server.URL+"/rest/v1/pocs/"+idStr)
		if status != http.StatusNotFound {
			t.Fatalf("GET /rest/v1/pocs/%d: status=%d, want 404", fix.pocID, status)
		}
	})

	t.Run("rest_list_absent", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/pocs")
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/pocs: status=%d, want 200; body=%s", status, body)
		}
		var env struct {
			Content []map[string]any `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /rest/v1/pocs: %v\nbody=%s", err, body)
		}
		for _, row := range env.Content {
			if idFloat, ok := row["id"].(float64); ok && int(idFloat) == fix.pocID {
				t.Fatalf("Users POC leaked into /rest/v1/pocs list: %+v", row)
			}
		}
	})

	// -------------------------------------------------------------------------
	// Surface 3: ConnectRPC PocService (Get + List)
	// -------------------------------------------------------------------------
	t.Run("grpc_get_CodeNotFound", func(t *testing.T) {
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		_, err := cl.GetPoc(t.Context(), &pbv1.GetPocRequest{Id: int64(fix.pocID)})
		if err == nil {
			t.Fatalf("GetPoc(%d): expected error (CodeNotFound), got nil", fix.pocID)
		}
		var ce *connect.Error
		if !errors.As(err, &ce) {
			t.Fatalf("GetPoc(%d): expected *connect.Error, got %T: %v", fix.pocID, err, err)
		}
		if ce.Code() != connect.CodeNotFound {
			t.Fatalf("GetPoc(%d): code=%s, want CodeNotFound (D-13)", fix.pocID, ce.Code())
		}
	})

	t.Run("grpc_list_absent", func(t *testing.T) {
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.ListPocs(t.Context(), &pbv1.ListPocsRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("ListPocs: %v", err)
		}
		for _, p := range resp.GetPocs() {
			if int(p.GetId()) == fix.pocID {
				t.Fatalf("Users POC leaked into ListPocs: %+v", p)
			}
		}
	})

	// -------------------------------------------------------------------------
	// Surface 4: GraphQL (node-by-id via where filter + list)
	//
	// The project's root Query does not expose a direct `poc(id:)` field
	// (see graph/schema.graphql). We query by the `pocs` connection with
	// a where filter on id — this is the canonical way to fetch a POC
	// by ID in gqlgen/entgql and still exercises the Poc privacy policy
	// because the resulting query materialises as *ent.PocQuery.
	// -------------------------------------------------------------------------
	t.Run("graphql_by_id_absent", func(t *testing.T) {
		q := fmt.Sprintf(`{"query":"{ pocs(where: {id: %d}) { edges { node { id role } } } }"}`, fix.pocID)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		r := decodeGraphQL(t, body)
		// Any GraphQL errors at this stage would indicate a wiring bug,
		// not a privacy behaviour. The privacy filter manifests as an
		// empty edges array, not an error.
		if len(r.Errors) > 0 {
			t.Fatalf("pocs(where:{id:%d}): unexpected errors: %+v", fix.pocID, r.Errors)
		}
		var data struct {
			Pocs struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Role string `json:"role"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"pocs"`
		}
		if err := json.Unmarshal(r.Data, &data); err != nil {
			t.Fatalf("decode pocs data: %v\ndata=%s", err, r.Data)
		}
		if len(data.Pocs.Edges) != 0 {
			t.Fatalf("pocs(where:{id:%d}): expected empty edges, got %+v", fix.pocID, data.Pocs.Edges)
		}
	})

	t.Run("graphql_list_absent", func(t *testing.T) {
		q := `{"query":"{ pocsList(limit: 100) { id role } }"}`
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		r := decodeGraphQL(t, body)
		if len(r.Errors) > 0 {
			t.Fatalf("pocsList: unexpected errors: %+v", r.Errors)
		}
		var data struct {
			PocsList []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"pocsList"`
		}
		if err := json.Unmarshal(r.Data, &data); err != nil {
			t.Fatalf("decode pocsList data: %v\ndata=%s", err, r.Data)
		}
		for _, p := range data.PocsList {
			if p.ID == strconv.Itoa(fix.pocID) || p.ID == fmt.Sprintf("%d", fix.pocID) {
				t.Fatalf("Users POC leaked into pocsList: %+v", p)
			}
		}
	})

	// -------------------------------------------------------------------------
	// Surface 5: /ui/
	//
	// The /ui/ surface does NOT expose a direct /ui/poc/{id} route —
	// POCs surface only through the network contacts fragment. The
	// fragment URL is /ui/fragment/net/{netID}/contacts. If the POC is
	// hidden by the privacy policy, its name must be absent from the
	// rendered HTML. We also assert the parent /ui/asn/{asn} page
	// renders successfully so we know the filter applies at the edge
	// traversal (network.QueryPocs) path, not just at the direct
	// client.Poc.Query path.
	// -------------------------------------------------------------------------
	t.Run("ui_network_page_renders", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/ui/asn/64999")
		if status != http.StatusOK {
			t.Fatalf("GET /ui/asn/64999: status=%d; body=%s", status, body)
		}
		// Sanity: network name is in the page (if it isn't, the rest of
		// the assertion is meaningless because nothing rendered).
		if !strings.Contains(string(body), "E2E Privacy Net") {
			t.Fatalf("GET /ui/asn/64999: network name missing from response; wrong page rendered?\nbody=%s", body)
		}
	})

	t.Run("ui_contacts_fragment_absent", func(t *testing.T) {
		url := fmt.Sprintf("%s/ui/fragment/net/%d/contacts", fix.server.URL, fix.netID)
		body, status := mustGet(t, url)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status=%d; body=%s", url, status, body)
		}
		if strings.Contains(string(body), fix.pocName) {
			t.Fatalf("Users POC name %q leaked into /ui/ contacts fragment HTML", fix.pocName)
		}
		// Also guard against the email leaking, which is a separate
		// PII-disclosure shape that would bypass a name-only scrub.
		if strings.Contains(string(body), "users-only@example.invalid") {
			t.Fatal("Users POC email leaked into /ui/ contacts fragment HTML")
		}
	})
}

// =============================================================================
// TierUsers: anonymous-but-elevated caller MUST see the visible="Users" POC.
// =============================================================================

// TestE2E_PublicTierUsersAdmitsRow proves that flipping the tier
// (PDBPLUS_PUBLIC_TIER=users in production, or DefaultTier: TierUsers
// in the test fixture) admits the row across every surface. This
// closes SYNC-03 by showing the env-var override flows correctly
// through the middleware → privctx → ent Policy chain.
func TestE2E_PublicTierUsersAdmitsRow(t *testing.T) {
	t.Parallel()
	fix := buildE2EFixture(t, privctx.TierUsers)
	idStr := strconv.Itoa(fix.pocID)

	// Surface 1: pdbcompat /api
	t.Run("pdbcompat_detail_200", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/poc/"+idStr)
		if status != http.StatusOK {
			t.Fatalf("GET /api/poc/%d: status=%d, want 200; body=%s", fix.pocID, status, body)
		}
		// pdbcompat wraps the single object in an array at env.data.
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /api/poc/%d: %v\nbody=%s", fix.pocID, err, body)
		}
		if len(env.Data) != 1 {
			t.Fatalf("GET /api/poc/%d: want len(data)=1, got %d", fix.pocID, len(env.Data))
		}
		if gotID, ok := env.Data[0]["id"].(float64); !ok || int(gotID) != fix.pocID {
			t.Fatalf("GET /api/poc/%d: data[0].id=%v, want %d", fix.pocID, env.Data[0]["id"], fix.pocID)
		}
	})

	t.Run("pdbcompat_list_present", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/api/poc")
		if status != http.StatusOK {
			t.Fatalf("GET /api/poc: status=%d; body=%s", status, body)
		}
		var env struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /api/poc: %v\nbody=%s", err, body)
		}
		found := false
		for _, row := range env.Data {
			if idFloat, ok := row["id"].(float64); ok && int(idFloat) == fix.pocID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Users POC not present in /api/poc under TierUsers — SYNC-03 regression")
		}
	})

	// Surface 2: entrest /rest/v1/pocs
	t.Run("rest_detail_200", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/pocs/"+idStr)
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/pocs/%d: status=%d, want 200; body=%s", fix.pocID, status, body)
		}
		var obj map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			t.Fatalf("decode /rest/v1/pocs/%d: %v\nbody=%s", fix.pocID, err, body)
		}
		if gotID, ok := obj["id"].(float64); !ok || int(gotID) != fix.pocID {
			t.Fatalf("GET /rest/v1/pocs/%d: id=%v, want %d", fix.pocID, obj["id"], fix.pocID)
		}
	})

	t.Run("rest_list_present", func(t *testing.T) {
		body, status := mustGet(t, fix.server.URL+"/rest/v1/pocs")
		if status != http.StatusOK {
			t.Fatalf("GET /rest/v1/pocs: status=%d; body=%s", status, body)
		}
		var env struct {
			Content []map[string]any `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode /rest/v1/pocs: %v\nbody=%s", err, body)
		}
		found := false
		for _, row := range env.Content {
			if idFloat, ok := row["id"].(float64); ok && int(idFloat) == fix.pocID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Users POC not present in /rest/v1/pocs under TierUsers — SYNC-03 regression")
		}
	})

	// Surface 3: ConnectRPC PocService
	t.Run("grpc_get_ok", func(t *testing.T) {
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.GetPoc(t.Context(), &pbv1.GetPocRequest{Id: int64(fix.pocID)})
		if err != nil {
			t.Fatalf("GetPoc(%d) under TierUsers: %v — SYNC-03 regression", fix.pocID, err)
		}
		if got := resp.GetPoc().GetId(); got != int64(fix.pocID) {
			t.Fatalf("GetPoc(%d).Poc.Id = %d, want %d", fix.pocID, got, fix.pocID)
		}
		if got := resp.GetPoc().GetVisible().GetValue(); got != "Users" {
			t.Fatalf("GetPoc(%d).Poc.Visible = %q, want \"Users\"", fix.pocID, got)
		}
	})

	t.Run("grpc_list_present", func(t *testing.T) {
		cl := peeringdbv1connect.NewPocServiceClient(http.DefaultClient, fix.server.URL)
		resp, err := cl.ListPocs(t.Context(), &pbv1.ListPocsRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("ListPocs: %v", err)
		}
		found := false
		for _, p := range resp.GetPocs() {
			if int(p.GetId()) == fix.pocID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Users POC not present in ListPocs under TierUsers — SYNC-03 regression")
		}
	})

	// Surface 4: GraphQL
	t.Run("graphql_by_id_present", func(t *testing.T) {
		q := fmt.Sprintf(`{"query":"{ pocs(where: {id: %d}) { edges { node { id role } } } }"}`, fix.pocID)
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		r := decodeGraphQL(t, body)
		if len(r.Errors) > 0 {
			t.Fatalf("pocs(where:{id:%d}): unexpected errors: %+v", fix.pocID, r.Errors)
		}
		var data struct {
			Pocs struct {
				Edges []struct {
					Node struct {
						ID   string `json:"id"`
						Role string `json:"role"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"pocs"`
		}
		if err := json.Unmarshal(r.Data, &data); err != nil {
			t.Fatalf("decode pocs data: %v\ndata=%s", err, r.Data)
		}
		if len(data.Pocs.Edges) != 1 {
			t.Fatalf("pocs(where:{id:%d}) under TierUsers: want 1 edge, got %d — SYNC-03 regression", fix.pocID, len(data.Pocs.Edges))
		}
	})

	t.Run("graphql_list_present", func(t *testing.T) {
		q := `{"query":"{ pocsList(limit: 100) { id role } }"}`
		body, status := mustPostJSON(t, fix.server.URL+"/graphql", q)
		if status != http.StatusOK {
			t.Fatalf("POST /graphql: status=%d; body=%s", status, body)
		}
		r := decodeGraphQL(t, body)
		if len(r.Errors) > 0 {
			t.Fatalf("pocsList: unexpected errors: %+v", r.Errors)
		}
		var data struct {
			PocsList []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"pocsList"`
		}
		if err := json.Unmarshal(r.Data, &data); err != nil {
			t.Fatalf("decode pocsList data: %v\ndata=%s", err, r.Data)
		}
		found := false
		want := strconv.Itoa(fix.pocID)
		for _, p := range data.PocsList {
			if p.ID == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Users POC not present in pocsList under TierUsers — SYNC-03 regression")
		}
	})

	// Surface 5: /ui/ contacts fragment — the POC name must now appear.
	t.Run("ui_contacts_fragment_present", func(t *testing.T) {
		url := fmt.Sprintf("%s/ui/fragment/net/%d/contacts", fix.server.URL, fix.netID)
		body, status := mustGet(t, url)
		if status != http.StatusOK {
			t.Fatalf("GET %s: status=%d; body=%s", url, status, body)
		}
		if !strings.Contains(string(body), fix.pocName) {
			t.Fatalf("Users POC name %q missing from /ui/ contacts fragment under TierUsers — SYNC-03 regression", fix.pocName)
		}
	})
}
