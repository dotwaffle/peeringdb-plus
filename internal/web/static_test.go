package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFlagIcons_SelfHosted checks that pages load the flag-icons
// stylesheet from /static and name no CDN, and that every url() in the
// vendored stylesheet resolves to an embedded file that the server
// sends as SVG.
func TestFlagIcons_SelfHosted(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	const css = "/static/flag-icons/css/flag-icons.min.css"
	if !strings.Contains(body, `href="`+css+`"`) {
		t.Errorf("home page does not link %s", css)
	}
	if strings.Contains(body, "cdn.jsdelivr.net") {
		t.Error("home page still names cdn.jsdelivr.net")
	}

	sheet, err := fs.ReadFile(StaticFS, strings.TrimPrefix(css, "/static/"))
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`url\(([^)]+)\)`).FindAllStringSubmatch(string(sheet), -1)
	if len(refs) < 250 {
		t.Fatalf("stylesheet has %d url() references, want one per flag", len(refs))
	}
	for _, ref := range refs {
		file := path.Join(path.Dir(strings.TrimPrefix(css, "/static/")), ref[1])
		if _, err := fs.Stat(StaticFS, file); err != nil {
			t.Errorf("url(%s): %v", ref[1], err)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/static/flag-icons/flags/4x3/de.svg", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/svg+xml") {
		t.Errorf("de.svg: status %d, Content-Type %q; want 200 image/svg+xml", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// TestLayout_ReducedMotion checks that every page carries the
// prefers-reduced-motion rule, after the transition rules that it must
// override, and that the map script reads the same setting.
func TestLayout_ReducedMotion(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	rule := strings.Index(body, "@media (prefers-reduced-motion: reduce)")
	if rule < 0 {
		t.Fatal("page has no prefers-reduced-motion rule")
	}
	for _, earlier := range []string{".htmx-settling {", "html.theme-transition, html.theme-transition *"} {
		if i := strings.Index(body, earlier); i < 0 || i > rule {
			t.Errorf("%q must come before the reduced-motion rule", earlier)
		}
	}

	js, err := fs.ReadFile(StaticFS, "map-init.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "prefers-reduced-motion: reduce") {
		t.Error("map-init.js does not read prefers-reduced-motion")
	}
}

// TestTemplates_TablesLabeled checks that every data table has a
// caption for screen readers as its first child and that every header
// cell names its scope, and that the compiled stylesheet has sr-only.
func TestTemplates_TablesLabeled(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("templates/*.templ")
	if err != nil {
		t.Fatal(err)
	}
	table := regexp.MustCompile(`<table\b[^>]*>\s*(<caption class="sr-only">[^<]+</caption>)?`)
	th := regexp.MustCompile(`<th\b[^>]*>`)
	tables := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range table.FindAllStringSubmatch(string(src), -1) {
			tables++
			if m[1] == "" {
				t.Errorf("%s: %s has no sr-only caption as its first child", f, m[0])
			}
		}
		for _, tag := range th.FindAllString(string(src), -1) {
			if !strings.Contains(tag, `scope="col"`) {
				t.Errorf("%s: %s has no scope", f, tag)
			}
		}
	}
	if tables == 0 {
		t.Fatal("no tables found in templates")
	}
	css, err := fs.ReadFile(StaticFS, "tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".sr-only{") {
		t.Error("tailwind.css has no .sr-only rule")
	}
}
