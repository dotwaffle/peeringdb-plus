package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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

// fetchCompletionScript GETs the served completion script for the given shell.
func fetchCompletionScript(t *testing.T, shell string) string {
	t.Helper()
	mux := newTestMux(t)
	req := httptest.NewRequest(http.MethodGet, "/ui/completions/"+shell, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET completions/%s: status %d", shell, rec.Code)
	}
	return rec.Body.String()
}

// TestCompletionPdbWrapper_JoinsArgsWithSlash asserts the served scripts carry
// the slash-joining pdb() body. The previous `curl -s ".../$@"` expanded each
// argument to a separate word, so `pdb asn 13335` requested /ui/asn AND a
// bogus second URL "13335"; the IFS=/ + "$*" form joins them into one path.
func TestCompletionPdbWrapper_JoinsArgsWithSlash(t *testing.T) {
	t.Parallel()
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			t.Parallel()
			body := fetchCompletionScript(t, shell)
			for _, want := range []string{"local IFS=/", `/ui/$*"`} {
				if !strings.Contains(body, want) {
					t.Errorf("%s completion script missing %q", shell, want)
				}
			}
			if strings.Contains(body, `/ui/$@`) {
				t.Errorf("%s completion script still uses the broken $@ expansion", shell)
			}
		})
	}
}

// TestCompletionZsh_SubcmdsExpand asserts the _arguments entity-type spec is
// double-quoted so $subcmds expands. The previous single-quoted form offered
// the literal string ${subcmds} as the only completion.
func TestCompletionZsh_SubcmdsExpand(t *testing.T) {
	t.Parallel()
	body := fetchCompletionScript(t, "zsh")
	if !strings.Contains(body, `"1:entity type:($subcmds)"`) {
		t.Error("zsh completion script missing double-quoted subcmds expansion")
	}
	if strings.Contains(body, `'1:entity type:(${subcmds})'`) {
		t.Error("zsh completion script still carries the unexpandable single-quoted subcmds spec")
	}
}

// TestCompletionPdbWrapper_ShellExecution sources each served script in its
// target shell with curl stubbed out and asserts the URL pdb() constructs.
// Hermetic: no network — the stub prints its arguments instead of fetching.
func TestCompletionPdbWrapper_ShellExecution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		shell   string // binary name + script endpoint
		prelude string // shell-specific stubs needed to source the script
	}{
		{shell: "bash", prelude: ""},
		// compdef only exists once compinit has run; stub it so the
		// script sources cleanly in a bare zsh -c.
		{shell: "zsh", prelude: "compdef() { : }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.shell, func(t *testing.T) {
			t.Parallel()
			bin, err := exec.LookPath(tc.shell)
			if err != nil {
				t.Skipf("%s not installed", tc.shell)
			}
			body := fetchCompletionScript(t, tc.shell)

			dir := t.TempDir()
			script := filepath.Join(dir, "completions."+tc.shell)
			if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
				t.Fatalf("write script: %v", err)
			}

			// Stub curl, source the served script, invoke the wrapper.
			driver := tc.prelude +
				`curl() { printf '%s\n' "$@"; }` + "\n" +
				`. ` + script + "\n" +
				`pdb asn 13335` + "\n"
			cmd := exec.CommandContext(t.Context(), bin, "-c", driver)
			cmd.Env = append(os.Environ(), "PDB_HOST=example.test")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s execution failed: %v\noutput:\n%s", tc.shell, err, out)
			}
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			got := lines[len(lines)-1]
			const want = "example.test/ui/asn/13335"
			if got != want {
				t.Errorf("pdb asn 13335 requested %q, want %q\nfull output:\n%s", got, want, out)
			}
		})
	}
}
