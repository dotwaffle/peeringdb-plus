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
	"github.com/dotwaffle/peeringdb-plus/ent"
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
	h := NewHandler(client, nil)
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

	if rec.Header().Get("Vary") != "HX-Request, User-Agent, Accept" {
		t.Errorf("expected Vary: HX-Request, User-Agent, Accept, got %q", rec.Header().Get("Vary"))
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

			if got := rec.Header().Get("Vary"); got != "HX-Request, User-Agent, Accept" {
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

	checks := []string{"dark:bg-neutral-900", "dark:text-neutral-100", "emerald-500"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing color scheme reference %q", want)
		}
	}
}

func TestLayout_DarkModeInit(t *testing.T) {
	t.Parallel()
	inner := templ.Raw("<p>test</p>")
	body := renderComponent(t, templates.Layout("Test", inner))

	checks := []string{
		"localStorage.getItem('darkMode')",
		"prefers-color-scheme",
		"dark:bg-neutral-900",
		"@custom-variant dark",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing dark mode init element %q", want)
		}
	}
}

func TestNav_DarkModeToggle(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Nav())

	checks := []string{
		"dark-mode-toggle",
		"M12 3v1m0 16v1",       // Sun icon path
		"M20.354 15.354A9 9 0", // Moon icon path
		"dark:bg-neutral-800",
		"dark:border-neutral-700",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing dark mode toggle element %q", want)
		}
	}
}

func TestNav_Links(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Nav())

	links := []string{"/ui/", "/ui/compare", "/ui/about", "/graphql", "/rest/v1/", "/api/"}
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

// --- Error page tests ---

func TestNotFoundPage_Styled(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/nonexistent-path", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	checks := []string{"404", "Page not found"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("404 page missing %q", want)
		}
	}
	// Should contain a search form, not just the home page.
	if !strings.Contains(body, `name="q"`) {
		t.Error("404 page should contain search form input")
	}
	// Should NOT be the homepage (no API quick link cards).
	if strings.Contains(body, `<h3 class="text-emerald-400 font-mono font-bold text-lg mb-2">GraphQL</h3>`) {
		t.Error("404 page should NOT render the homepage API cards")
	}
}

func TestNotFoundPage_HasSearchBox(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/bogus", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/ui/search"`) {
		t.Error("404 page should contain htmx search form with hx-get=\"/ui/search\"")
	}
}

func TestServerError_Render(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.ServerErrorPage())

	checks := []string{"500", "Something went wrong", "/ui/"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("500 page missing %q", want)
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

	h := NewHandler(client, nil)
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

	h := NewHandler(client, nil)
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

	h := NewHandler(client, nil)
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

func TestSearchEndpoint_HXPushUrl(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=Cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	hxURL := rec.Header().Get("HX-Push-Url")
	if hxURL != "/ui/?q=Cloud" {
		t.Errorf("HX-Push-Url = %q, want %q", hxURL, "/ui/?q=Cloud")
	}
}

func TestSearchEndpoint_HXPushUrl_EmptyQuery(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/search", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	hxURL := rec.Header().Get("HX-Push-Url")
	if hxURL != "/ui/" {
		t.Errorf("HX-Push-Url = %q, want %q", hxURL, "/ui/")
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

	h := NewHandler(client, nil)
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

	if got := rec.Header().Get("Vary"); got != "HX-Request, User-Agent, Accept" {
		t.Errorf("Vary header = %q, want %q", got, "HX-Request")
	}
}

// --- Compare endpoint integration tests ---

// seedCompareHandlerTestData creates two networks with shared presences
// for handler-level compare tests. Reuses seedCompareTestData from compare_test.go.
func seedCompareHandlerTestData(t *testing.T, client *ent.Client) {
	t.Helper()
	seedCompareTestData(t, client)
}

// setupCompareMux creates a mux with compare test data seeded.
func setupCompareMux(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	seedCompareHandlerTestData(t, client)
	h := NewHandler(client, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestCompareFormPage(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	checks := []string{"compare-asn1", "compare-asn2", "Compare Networks"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("compare form page missing %q", want)
		}
	}
}

func TestCompareFormPagePreFilled(t *testing.T) {
	t.Parallel()
	mux := setupCompareMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare/13335", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "13335") {
		t.Error("pre-filled form should contain ASN 13335")
	}
	if !strings.Contains(body, "Compare Networks") {
		t.Error("pre-filled form should contain title")
	}
}

func TestCompareResultsPage(t *testing.T) {
	t.Parallel()
	mux := setupCompareMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare/13335/15169", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	checks := []string{"Cloudflare", "Google", "DE-CIX Frankfurt", "Equinix FR5"}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("compare results missing %q", want)
		}
	}
}

func TestCompareResultsPage_FullView(t *testing.T) {
	t.Parallel()
	mux := setupCompareMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare/13335/15169?view=full", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	// Full view should show the Full View button as active (emerald style).
	if !strings.Contains(body, "Full View") {
		t.Error("full view page should contain Full View toggle")
	}
	// Full view should show non-shared IXPs like AMS-IX.
	if !strings.Contains(body, "AMS-IX") {
		t.Error("full view should include non-shared IXPs like AMS-IX")
	}
	// Full view should show non-shared facilities like Equinix AM5.
	if !strings.Contains(body, "Equinix AM5") {
		t.Error("full view should include non-shared facilities like Equinix AM5")
	}
}

func TestCompareResultsPage_InvalidASN(t *testing.T) {
	t.Parallel()
	mux := setupCompareMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare/13335/99999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestCompareResultsPage_NonNumericASN(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/compare/abc/def", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func TestNetworkDetailPage_CompareButton(t *testing.T) {
	t.Parallel()
	mux := setupCompareMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "/ui/compare/13335") {
		t.Error("network detail page should contain compare link /ui/compare/13335")
	}
	if !strings.Contains(body, "Compare with") {
		t.Error("network detail page should contain 'Compare with' button text")
	}
}

// --- About page tests ---

func TestAboutPage(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/about", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	checks := []string{
		"About PeeringDB Plus",
		"/graphql",
		"/rest/v1/",
		"/api/",
		"github.com/dotwaffle/peeringdb-plus",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("about page missing %q", want)
		}
	}
}

func TestAboutPage_NoSync(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/about", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Sync status unavailable") {
		t.Error("about page with nil db should show 'Sync status unavailable'")
	}
}

// --- Dark mode and CSS animation tests ---

func TestLayout_CSSAnimations(t *testing.T) {
	t.Parallel()
	inner := templ.Raw("<p>test</p>")
	body := renderComponent(t, templates.Layout("Test", inner))

	checks := []string{
		"@keyframes fadeIn",
		".htmx-swapping",
		"global-indicator",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing CSS animation element %q", want)
		}
	}
}

func TestFooter_DarkMode(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.Footer())

	if !strings.Contains(body, "dark:bg-neutral-800") {
		t.Error("footer missing dark:bg-neutral-800")
	}
	if !strings.Contains(body, "dark:border-neutral-700") {
		t.Error("footer missing dark:border-neutral-700")
	}
}

func TestSearchResults_ARIARoles(t *testing.T) {
	t.Parallel()
	groups := []templates.SearchGroup{
		{
			TypeName:    "Networks",
			TypeSlug:    "net",
			AccentColor: "emerald",
			HasMore:     false,
			Results: []templates.SearchResult{
				{Name: "Cloudflare", Subtitle: "AS13335", DetailURL: "/ui/asn/13335"},
			},
		},
	}
	body := renderComponent(t, templates.SearchResults(groups))

	checks := []string{
		`role="option"`,
		`tabindex="-1"`,
		`aria-selected="false"`,
		"focus:ring-2",
		"focus:ring-emerald-500",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("search results missing ARIA attribute %q", want)
		}
	}
}

func TestSearchForm_ListboxRole(t *testing.T) {
	t.Parallel()
	body := renderComponent(t, templates.SearchForm("", nil))

	checks := []string{
		`role="listbox"`,
		"autofocus",
		`aria-label="Search results"`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("search form missing %q", want)
		}
	}
}

func TestSearchResults_FadeIn(t *testing.T) {
	t.Parallel()
	groups := []templates.SearchGroup{
		{
			TypeName:    "Networks",
			TypeSlug:    "net",
			AccentColor: "emerald",
			HasMore:     false,
			Results: []templates.SearchResult{
				{Name: "Cloudflare", Subtitle: "AS13335", DetailURL: "/ui/asn/13335"},
			},
		},
	}
	body := renderComponent(t, templates.SearchResults(groups))

	if !strings.Contains(body, "animate-fade-in") {
		t.Error("search results missing animate-fade-in class")
	}
	if !strings.Contains(body, "dark:border-neutral-700") {
		t.Error("search results missing dark:border-neutral-700")
	}
	if !strings.Contains(body, "dark:bg-neutral-800") {
		t.Error("search results missing dark:bg-neutral-800")
	}
}

func TestLayout_KeyboardNavScript(t *testing.T) {
	t.Parallel()
	inner := templ.Raw("<p>test</p>")
	body := renderComponent(t, templates.Layout("Test", inner))

	checks := []string{
		"ArrowDown",
		"ArrowUp",
		"aria-selected",
		"htmx:afterSwap",
		`role="option"`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("layout missing keyboard nav element %q", want)
		}
	}
}

// --- Terminal detection integration tests ---

// truncateBody returns the first n characters of s for error message context.
func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func TestTerminalDetection(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	tests := []struct {
		name        string
		path        string
		userAgent   string
		accept      string
		wantStatus  int
		wantCT      string   // Content-Type prefix
		wantContain []string // strings body must contain
		wantAbsent  []string // strings body must NOT contain
		wantVary    string   // Vary header must contain
	}{
		{
			name:        "curl /ui/ gets help text",
			path:        "/ui/",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus", "Usage:", "curl peeringdb-plus.fly.dev/ui/asn/"},
			wantVary:    "User-Agent",
		},
		{
			name:        "wget /ui/ gets text",
			path:        "/ui/",
			userAgent:   "Wget/1.21",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus"},
			wantVary:    "User-Agent",
		},
		{
			name:       "httpie /ui/ gets text",
			path:       "/ui/",
			userAgent:  "HTTPie/3.2.4",
			wantStatus: 200,
			wantCT:     "text/plain",
			wantVary:   "User-Agent",
		},
		{
			name:        "browser /ui/ gets HTML",
			path:        "/ui/",
			userAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
			wantStatus:  200,
			wantCT:      "text/html",
			wantContain: []string{"<!doctype html>"},
			wantVary:    "User-Agent",
		},
		{
			name:       "?format=json returns JSON",
			path:       "/ui/?format=json",
			userAgent:  "curl/8.5.0",
			wantStatus: 200,
			wantCT:     "application/json",
			wantVary:   "User-Agent",
		},
		{
			name:        "?T returns plain text without ANSI",
			path:        "/ui/?T",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus"},
			wantAbsent:  []string{"\x1b["},
			wantVary:    "User-Agent",
		},
		{
			name:        "?nocolor strips ANSI from rich output",
			path:        "/ui/?nocolor",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus"},
			wantAbsent:  []string{"\x1b["},
			wantVary:    "User-Agent",
		},
		{
			name:        "curl rich mode has ANSI codes",
			path:        "/ui/",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"\x1b["},
		},
		{
			name:        "curl 404 returns text error",
			path:        "/ui/asn/99999999",
			userAgent:   "curl/8.5.0",
			wantStatus:  404,
			wantCT:      "text/plain",
			wantContain: []string{"404", "Not Found"},
			wantAbsent:  []string{"<!doctype html>"},
		},
		{
			name:        "browser 404 returns HTML error",
			path:        "/ui/asn/99999999",
			userAgent:   "Mozilla/5.0",
			wantStatus:  404,
			wantCT:      "text/html",
			wantContain: []string{"Page not found"},
		},
		{
			name:       "Accept application/json overrides browser UA",
			path:       "/ui/",
			userAgent:  "Mozilla/5.0",
			accept:     "application/json",
			wantStatus: 200,
			wantCT:     "application/json",
		},
		{
			name:        "Accept text/plain triggers rich mode for browser",
			path:        "/ui/",
			userAgent:   "Mozilla/5.0",
			accept:      "text/plain",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus"},
		},
		{
			name:        "?format=plain overrides Accept JSON",
			path:        "/ui/?format=plain",
			userAgent:   "curl/8.5.0",
			accept:      "application/json",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"PeeringDB Plus"},
			wantAbsent:  []string{"\x1b["},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, tt.wantCT) {
				t.Errorf("Content-Type = %q, want prefix %q", ct, tt.wantCT)
			}
			body := rec.Body.String()
			for _, s := range tt.wantContain {
				if !strings.Contains(body, s) {
					t.Errorf("body missing %q (first 500 chars: %s)", s, truncateBody(body, 500))
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(body, s) {
					t.Errorf("body unexpectedly contains %q", s)
				}
			}
			if tt.wantVary != "" {
				vary := rec.Header().Get("Vary")
				if !strings.Contains(vary, tt.wantVary) {
					t.Errorf("Vary = %q, want it to contain %q", vary, tt.wantVary)
				}
			}
		})
	}
}

func TestTerminal404JSON(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/asn/99999999?format=json", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json prefix", ct)
	}
	// RFC 9457 Problem Detail format per ARCH-01.
	body := rec.Body.String()
	if !strings.Contains(body, `"type"`) || !strings.Contains(body, `about:blank`) {
		t.Errorf("JSON 404 body missing RFC 9457 type field: %s", body)
	}
	if !strings.Contains(body, `"status"`) {
		t.Errorf("JSON 404 body missing RFC 9457 status field: %s", body)
	}
	if !strings.Contains(body, `"title"`) {
		t.Errorf("JSON 404 body missing RFC 9457 title field: %s", body)
	}
}

func TestDetailPages_DispatchModes(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	tests := []struct {
		name        string
		path        string
		userAgent   string
		wantStatus  int
		wantCT      string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "network terminal rich",
			path:        "/ui/asn/13335",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"\x1b[", "Cloudflare"},
			wantAbsent:  []string{"<!doctype html>"},
		},
		{
			name:        "network JSON",
			path:        "/ui/asn/13335?format=json",
			wantStatus:  200,
			wantCT:      "application/json",
			wantContain: []string{"{", "Cloudflare"},
		},
		{
			name:        "network WHOIS",
			path:        "/ui/asn/13335?format=whois",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"aut-num:", "Cloudflare"},
		},
		{
			name:        "network short",
			path:        "/ui/asn/13335?format=short",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"AS13335"},
		},
		{
			name:        "ix terminal rich",
			path:        "/ui/ix/20",
			userAgent:   "curl/8.5.0",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"\x1b[", "DE-CIX"},
		},
		{
			name:        "ix WHOIS",
			path:        "/ui/ix/20?format=whois",
			wantStatus:  200,
			wantCT:      "text/plain",
			wantContain: []string{"DE-CIX"},
		},
		{
			name:        "facility JSON",
			path:        "/ui/fac/30?format=json",
			wantStatus:  200,
			wantCT:      "application/json",
			wantContain: []string{"{", "Equinix"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, tt.wantCT) {
				t.Fatalf("Content-Type = %q, want prefix %q", ct, tt.wantCT)
			}
			body := rec.Body.String()
			for _, s := range tt.wantContain {
				if !strings.Contains(body, s) {
					t.Errorf("body missing %q (first 500 chars: %s)", s, truncateBody(body, 500))
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(body, s) {
					t.Errorf("body unexpectedly contains %q", s)
				}
			}
		})
	}
}

func TestKeyboardNav_Integration(t *testing.T) {
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

	h := NewHandler(client, nil)
	mux := http.NewServeMux()
	h.Register(mux)

	// Bookmarked URL with query should render full page with both keyboard nav and ARIA
	req := httptest.NewRequest(http.MethodGet, "/ui/?q=Cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	checks := []struct {
		want string
		desc string
	}{
		{"ArrowDown", "keyboard nav script"},
		{`role="option"`, "ARIA option role on results"},
		{`role="listbox"`, "ARIA listbox role on container"},
		{"Cloudflare", "search result content"},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.want) {
			t.Errorf("integration page missing %s (%q)", c.desc, c.want)
		}
	}
}

func TestHandleServerError(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	h := NewHandler(client, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/500", nil)

	h.handleServerError(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Server Error") {
		t.Errorf("response body missing %q, got %q", "Server Error", body)
	}
}
