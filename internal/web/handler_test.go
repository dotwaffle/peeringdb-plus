package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// renderComponent renders a templ.Component to a string for testing.
func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return buf.String()
}

// newTestMux creates a Handler with a test ent client and returns the mux.
func newTestMux(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// testHandlerTimestamp is a consistent timestamp for handler test data seeding.
var testHandlerTimestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)


func TestHomeHandler_FullPage(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	checks := []string{
		"<!doctype html>",
		"PeeringDB Plus",
		"htmx.min.js",
		"@tailwindcss/browser@4",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("full page response missing %q", want)
		}
	}
}

func TestHomeHandler_HtmxFragment(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<!doctype html>") || strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("htmx fragment should not contain DOCTYPE")
	}

	if rec.Header().Get("Vary") != "HX-Request" {
		t.Errorf("expected Vary: HX-Request, got %q", rec.Header().Get("Vary"))
	}
}

func TestHomeHandler_VaryHeader(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	tests := []struct {
		name      string
		hxRequest bool
	}{
		{"without HX-Request", false},
		{"with HX-Request", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
			if tt.hxRequest {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if got := rec.Header().Get("Vary"); got != "HX-Request" {
				t.Errorf("Vary header = %q, want %q", got, "HX-Request")
			}
		})
	}
}

func TestStaticAssets_HtmxJS(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "htmx") {
		t.Error("response body does not contain 'htmx'")
	}
}

func TestStaticAssets_NotFound(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/static/nonexistent.js", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestLayout_TailwindClasses(t *testing.T) {
	t.Parallel()
	inner := templ.Raw("<p>test content</p>")
	body := renderComponent(t, templates.Layout("Test", inner))

	checks := []string{"container", "mx-auto", "flex-col"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing Tailwind class %q", want)
		}
	}
}

func TestLayout_ColorScheme(t *testing.T) {
	t.Parallel()
	inner := templ.Raw("<p>test</p>")
	body := renderComponent(t, templates.Layout("Test", inner))

	checks := []string{"bg-neutral-900", "text-neutral-100", "emerald-500"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing color scheme reference %q", want)
		}
	}
}

func TestNav_Links(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Nav())

	links := []string{"/ui/", "/ui/compare", "/graphql", "/rest/v1/", "/api/"}
	for _, want := range links {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing link %q", want)
		}
	}
}

func TestNav_MobileMenu(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Nav())

	checks := []string{"mobile-menu", "md:hidden"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing mobile element %q", want)
		}
	}
}

func TestFooter_Content(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Footer())

	checks := []string{"PeeringDB Plus", "github.com/dotwaffle/peeringdb-plus"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("footer missing %q", want)
		}
	}
}

// --- Search endpoint integration tests ---

func TestSearchEndpoint_EmptyQuery(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Empty query should return empty search results (no type group headings)
	if strings.Contains(body, "Networks") {
		t.Error("empty query should not contain type group results")
	}
}

func TestSearchEndpoint_MinLength(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	// Single char query should return no results
	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=C", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "Cloudflare") {
		t.Error("single-char query should not return results")
	}
}

func TestSearchEndpoint_WithResults(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}
	_, err = client.InternetExchange.Create().
		SetID(20).SetName("DE-CIX Frankfurt").
		SetCity("Frankfurt").SetCountry("DE").
		SetOrgID(1).SetOrganization(org).
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating ix: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=Cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Cloudflare") {
		t.Error("search results should contain 'Cloudflare'")
	}
	if !strings.Contains(body, "AS13335") {
		t.Error("search results should contain ASN subtitle 'AS13335'")
	}
	if !strings.Contains(body, "Networks") {
		t.Error("search results should contain type group 'Networks'")
	}
}

func TestSearchEndpoint_HtmxFragment(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=Cloud", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, "<!doctype html>") || strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("htmx fragment should not contain DOCTYPE")
	}
	if !strings.Contains(body, "Cloudflare") {
		t.Error("htmx fragment should contain search results")
	}
}

func TestSearchEndpoint_HXReplaceUrl(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=Cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	hxURL := rec.Header().Get("HX-Replace-Url")
	if hxURL != "/ui/?q=Cloud" {
		t.Errorf("HX-Replace-Url = %q, want %q", hxURL, "/ui/?q=Cloud")
	}
}

func TestSearchEndpoint_HXReplaceUrl_EmptyQuery(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	hxURL := rec.Header().Get("HX-Replace-Url")
	if hxURL != "/ui/" {
		t.Errorf("HX-Replace-Url = %q, want %q", hxURL, "/ui/")
	}
}

func TestHomeWithQuery_PreRendered(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := context.Background()

	org, err := client.Organization.Create().
		SetID(1).SetName("TestOrg").
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}
	_, err = client.Network.Create().
		SetID(10).SetName("Cloudflare").SetAsn(13335).
		SetOrgID(1).SetOrganization(org).
		SetCreated(testHandlerTimestamp).SetUpdated(testHandlerTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)

	// Bookmarked URL: /ui/?q=Cloud should render full page with results
	req := httptest.NewRequest(http.MethodGet, "/ui/?q=Cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Full page should have DOCTYPE
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("bookmarked URL should render full page with DOCTYPE")
	}
	// Should contain pre-rendered search results
	if !strings.Contains(body, "Cloudflare") {
		t.Error("bookmarked URL should contain pre-rendered search results")
	}
	if !strings.Contains(body, "Networks") {
		t.Error("bookmarked URL should contain type group heading 'Networks'")
	}
}

func TestSearchEndpoint_VaryHeader(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=test", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if got := rec.Header().Get("Vary"); got != "HX-Request" {
		t.Errorf("Vary header = %q, want %q", got, "HX-Request")
	}
}
