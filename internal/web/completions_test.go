package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// testCompletionTimestamp is a consistent timestamp for completion test data seeding.
var testCompletionTimestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// seedCompletionData creates test records for completion search tests.
func seedCompletionData(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	org, err := client.Organization.Create().
		SetID(1).
		SetName("Test Org").
		SetCreated(testCompletionTimestamp).
		SetUpdated(testCompletionTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating organization: %v", err)
	}

	_, err = client.Network.Create().
		SetID(10).
		SetName("Cloudflare").
		SetAsn(13335).
		SetOrgID(1).
		SetOrganization(org).
		SetCreated(testCompletionTimestamp).
		SetUpdated(testCompletionTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating network: %v", err)
	}

	_, err = client.InternetExchange.Create().
		SetID(20).
		SetName("Cloud IX").
		SetCity("Frankfurt").
		SetCountry("DE").
		SetOrgID(1).
		SetOrganization(org).
		SetRegionContinent("Europe").
		SetMedia("Ethernet").
		SetCreated(testCompletionTimestamp).
		SetUpdated(testCompletionTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("creating internet exchange: %v", err)
	}

	h := NewHandler(NewHandlerInput{Client: client})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestCompletionBash_ContentType(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/bash", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestCompletionBash_ContainsFunction(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/bash", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()

	checks := []string{
		"_pdb_completions",
		"complete -F",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("bash completion script missing %q", want)
		}
	}
}

func TestCompletionBash_ContainsPdbWrapper(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/bash", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "pdb()") {
		t.Error("bash completion script missing pdb() wrapper function")
	}
}

func TestCompletionBash_ContainsPDBHost(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/bash", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "PDB_HOST") {
		t.Error("bash completion script missing PDB_HOST env var")
	}
}

func TestCompletionZsh_ContentType(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/zsh", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestCompletionZsh_ContainsFunction(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/zsh", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()

	checks := []string{
		"_pdb",
		"compdef",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("zsh completion script missing %q", want)
		}
	}
}

func TestCompletionZsh_ContainsPdbWrapper(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/zsh", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "pdb()") {
		t.Error("zsh completion script missing pdb() wrapper function")
	}
}

func TestCompletionSearch_ReturnsPlainText(t *testing.T) {
	t.Parallel()
	mux := seedCompletionData(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/search?q=cloud&type=net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestCompletionSearch_NewlineDelimited(t *testing.T) {
	t.Parallel()
	mux := seedCompletionData(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/search?q=cloud&type=net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty response body")
	}

	// Body should end with newline.
	if !strings.HasSuffix(body, "\n") {
		t.Error("response body should end with newline")
	}

	// For networks, results should be ASN identifiers (integers).
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one line of output")
	}
	// The network Cloudflare has ASN 13335.
	if lines[0] != "13335" {
		t.Errorf("expected first line to be ASN '13335', got %q", lines[0])
	}
}

func TestCompletionSearch_EmptyQuery(t *testing.T) {
	t.Parallel()
	mux := seedCompletionData(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/search?q=&type=net", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "" {
		t.Errorf("expected empty body for empty query, got %q", body)
	}
}

func TestCompletionSearch_NoType(t *testing.T) {
	t.Parallel()
	mux := seedCompletionData(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/search?q=cloud", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty response for unfiltered search")
	}

	// Should contain results from multiple types (network: 13335, ix: 20).
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 {
		t.Errorf("expected results from multiple types, got %d lines", len(lines))
	}
}

func TestCompletionSearch_IXType(t *testing.T) {
	t.Parallel()
	mux := seedCompletionData(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/completions/search?q=cloud&type=ix", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected non-empty response for IX search")
	}

	lines := strings.Split(strings.TrimSpace(body), "\n")
	// The IX "Cloud IX" has ID 20.
	if lines[0] != "20" {
		t.Errorf("expected first line to be IX ID '20', got %q", lines[0])
	}
}

func TestExtractID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		detailURL string
		typeSlug  string
		want      string
	}{
		{"network", "/ui/asn/13335", "net", "13335"},
		{"ix", "/ui/ix/20", "ix", "20"},
		{"facility", "/ui/fac/30", "fac", "30"},
		{"org", "/ui/org/1", "org", "1"},
		{"campus", "/ui/campus/40", "campus", "40"},
		{"carrier", "/ui/carrier/50", "carrier", "50"},
		{"unknown type", "/ui/foo/1", "foo", ""},
		{"empty url", "", "net", ""},
		{"empty slug", "/ui/asn/13335", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractID(tt.detailURL, tt.typeSlug)
			if got != tt.want {
				t.Errorf("extractID(%q, %q) = %q, want %q", tt.detailURL, tt.typeSlug, got, tt.want)
			}
		})
	}
}
