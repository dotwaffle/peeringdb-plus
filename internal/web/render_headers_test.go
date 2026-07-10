package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// TestRenderPage_PreservesOuterVary locks the 2026-06-10 audit fix: the
// outer Compression middleware (gzhttp) adds Vary: Accept-Encoding before
// dispatch, and renderPage previously called Header().Set("Vary", ...),
// clobbering it — a shared cache could then replay a gzipped variant to
// identity-encoding clients.
func TestRenderPage_PreservesOuterVary(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	// Simulate gzhttp's pre-dispatch header.
	rec.Header().Add("Vary", "Accept-Encoding")

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")

	page := PageContent{Title: "Home", Content: templates.NotFoundPage()}
	if err := renderPage(req.Context(), rec, req, page); err != nil {
		t.Fatalf("renderPage: %v", err)
	}

	vary := strings.Join(rec.Header().Values("Vary"), ", ")
	for _, want := range []string{"Accept-Encoding", "User-Agent", "Accept", "HX-Request"} {
		if strings.Contains(vary, want) {
			continue
		}
		t.Errorf("Vary %q missing %q", vary, want)
	}
}

// TestRenderPage_ErrorStatusAfterHeaders locks the WriteHeader-ordering
// fix: 404/500 pages previously called WriteHeader before renderPage set
// Vary/Content-Type, so net/http dropped those headers for non-gzip
// clients and the body sniffer replaced the JSON Content-Type with
// text/plain. The status now travels in PageContent.Status and is
// committed after the headers.
func TestRenderPage_ErrorStatusAfterHeaders(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/nonexistent", nil)
	// curl-style client: JSON mode via Accept, no Accept-Encoding.
	req.Header.Set("User-Agent", "curl/8.5.0")
	req.Header.Set("Accept", "application/json")

	page := PageContent{Title: "Not Found", Kind: KindNotFound, Status: http.StatusNotFound}
	if err := renderPage(req.Context(), rec, req, page); err != nil {
		t.Fatalf("renderPage: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}
	if got := rec.Header().Get("Vary"); got == "" {
		t.Error("Vary missing on 404 response")
	}
	if body := rec.Body.String(); strings.Contains(body, "doesn't exist") == false {
		t.Errorf("problem-detail body missing: %q", body)
	}
}

// TestRenderPage_EntityTitledHome locks the PageKind dispatch: "Home",
// "Not Found", and "Server Error" are legal entity names, so an entity
// page whose Title collides with one must still render as data, not be
// misrouted to the help/error output the magic-string switch produced.
func TestRenderPage_EntityTitledHome(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/net/1", nil)
	req.Header.Set("User-Agent", "curl/8.5.0")
	req.Header.Set("Accept", "application/json")

	page := PageContent{Title: "Home", Kind: KindEntity}
	if err := renderPage(req.Context(), rec, req, page); err != nil {
		t.Fatalf("renderPage: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"title"`) || !strings.Contains(body, "Home") {
		t.Errorf("entity page misrouted, body: %q", body)
	}
	if strings.Contains(body, "doesn't exist") {
		t.Errorf("entity page rendered as error page: %q", body)
	}
}
