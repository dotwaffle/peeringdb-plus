package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
