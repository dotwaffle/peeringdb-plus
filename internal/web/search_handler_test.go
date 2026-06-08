package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// setupSearchPagingMux seeds n networks named "PageNet NN" (zero-padded so they
// sort numerically) and returns a mux ready for view-all page testing.
func setupSearchPagingMux(t *testing.T, n int) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	seedNetworksForPaging(t, client, n)
	h := NewHandler(NewHandlerInput{Client: client})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestSearchTypePage_InitialPage(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 60)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=PageNet&type=net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Full page chrome (layout) on a non-htmx navigation.
	if !strings.Contains(strings.ToLower(body), "<!doctype html>") {
		t.Error("initial page should be a full HTML document")
	}
	// Exact total and the first page of rows.
	if !strings.Contains(body, "60 results") {
		t.Error("page missing exact total '60 results'")
	}
	wantPresent := []string{"PageNet 01", "PageNet 50", "Load more"}
	for _, w := range wantPresent {
		if !strings.Contains(body, w) {
			t.Errorf("page missing %q", w)
		}
	}
	// Page size 50: rows beyond the first page must not be present yet.
	if strings.Contains(body, "PageNet 51") {
		t.Error("initial page should not contain PageNet 51 (beyond page size)")
	}
	// The Load more button points at the next offset.
	if !strings.Contains(body, "type=net") || !strings.Contains(body, "offset=50") {
		t.Error("Load more button missing next-page URL (type=net&offset=50)")
	}
}

func TestSearchTypePage_LoadMoreFragment(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 60)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=PageNet&type=net&offset=50", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	// Bare fragment: no layout.
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Error("load-more fragment should not include the full document")
	}
	// The trailing rows (51..60) appear.
	if !strings.Contains(body, "PageNet 51") || !strings.Contains(body, "PageNet 60") {
		t.Error("load-more fragment missing trailing rows 51..60")
	}
	// Page-1 rows are not re-sent.
	if strings.Contains(body, "PageNet 01") {
		t.Error("load-more fragment should not repeat page-1 rows")
	}
	// Exhausted: no further Load more button.
	if strings.Contains(body, "Load more") {
		t.Error("load-more fragment should omit the button once results are exhausted")
	}
}

func TestSearchTypePage_NoLoadMoreWhenUnderPageSize(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 5)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=PageNet&type=net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "5 results") {
		t.Error("page missing '5 results'")
	}
	if strings.Contains(body, "Load more") {
		t.Error("page with 5 results (< pageSize) should not show Load more")
	}
}

func TestSearchTypePage_UnknownTypeIs404(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 3)

	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=PageNet&type=bogus", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown type", rec.Code)
	}
}

func TestSearchTypePage_NonNumericOffsetClampsToFirstPage(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 60)

	// A garbage offset must behave like the initial page, not error.
	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=PageNet&type=net&offset=abc", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PageNet 01") {
		t.Error("non-numeric offset should fall back to the first page")
	}
}

func TestSearchResults_ViewAllLink(t *testing.T) {
	t.Parallel()
	mux := setupSearchPagingMux(t, 60)

	// The grouped quick-search (home page) should expose a "View all 60" link
	// into the per-type page when a type overflows displayLimit.
	req := httptest.NewRequest(http.MethodGet, "/ui/?q=PageNet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "View all 60") {
		t.Error("grouped results missing 'View all 60' link text")
	}
	if !strings.Contains(body, "type=net") {
		t.Error("View all link missing per-type URL (type=net)")
	}
}
