package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
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

// TestTemplates_TextContrast checks the text colors of every class list
// in the templates and ui.js against the shades that reach a 4.5:1
// contrast ratio: neutral 600 or darker and the accent colors 700 or
// darker on the light backgrounds, and neutral 400 or lighter on the
// dark backgrounds. A light-mode text color needs a dark: counterpart
// for the same state, because the dark shades are different. White text
// needs an accent background of shade 700 or darker.
func TestTemplates_TextContrast(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("templates/*.templ")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, "static/ui.js")
	literal := regexp.MustCompile(`"[^"\n]*"|'[^'\n]*'`)
	color := regexp.MustCompile(`^((?:[a-z-]+:)*)(text|placeholder)-(neutral|emerald|sky|violet|rose|cyan|amber|red|blue)-(\d+)(/\d+)?$`)
	checked := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range literal.FindAllString(string(src), -1) {
			type token struct {
				text, state, kind, color string
				shade                    int
			}
			var light []token
			dark := map[string]bool{}
			for field := range strings.FieldsSeq(strings.Trim(lit, `"'`)) {
				m := color.FindStringSubmatch(field)
				if m == nil {
					continue
				}
				checked++
				if m[5] != "" {
					t.Errorf("%s: %s: opacity lowers the contrast", f, field)
				}
				shade, _ := strconv.Atoi(m[4])
				if rest, ok := strings.CutPrefix(m[1], "dark:"); ok {
					dark[rest+m[2]] = true
					if m[3] == "neutral" && shade > 400 && shade < 700 {
						t.Errorf("%s: %s is too dark on the dark backgrounds", f, field)
					}
					continue
				}
				light = append(light, token{field, m[1], m[2], m[3], shade})
			}
			for _, tok := range light {
				minShade := 700
				switch {
				case tok.kind == "placeholder":
					minShade = 500
				case tok.color == "neutral":
					minShade = 600
				}
				if tok.shade < minShade {
					t.Errorf("%s: %s is too light on the light backgrounds", f, tok.text)
				}
				if !dark[tok.state+tok.kind] {
					t.Errorf("%s: %s has no dark: counterpart", f, tok.text)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no text colors found")
	}

	// White text needs a 700 or darker background, in every state.
	background := regexp.MustCompile(`^((?:[a-z-]+:)*)bg-(emerald|sky|violet|rose|cyan|amber|red|blue)-(\d+)$`)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, lit := range literal.FindAllString(string(src), -1) {
			if !strings.Contains(lit, "text-white") {
				continue
			}
			for field := range strings.FieldsSeq(strings.Trim(lit, `"'`)) {
				m := background.FindStringSubmatch(field)
				if m == nil {
					continue
				}
				if shade, _ := strconv.Atoi(m[3]); shade < 700 {
					t.Errorf("%s: %s is too light behind white text", f, field)
				}
			}
		}
	}
}

// TestLayout_SpotlightDialog checks that the spotlight overlay is a
// modal dialog with an accessible name and a labeled input, and that
// ui.js opens it with showModal(), which makes the page behind it inert.
func TestLayout_SpotlightDialog(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`<dialog id="spotlight" aria-label="Quick search"`,
		`<label for="spotlight-input" class="sr-only">`,
		`id="spotlight-panel"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page has no %q", want)
		}
	}
	if strings.Contains(body, "aria-modal") {
		t.Error("page still sets aria-modal; the dialog element is modal by itself")
	}

	js, err := fs.ReadFile(StaticFS, "ui.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "dialog.showModal()") {
		t.Error("ui.js does not open the spotlight with showModal()")
	}
}

// TestNav_TouchTargets checks that the icon buttons in the navigation
// have padding around their icons, so that each target is at least 24
// by 24 pixels (WCAG 2.5.8).
func TestNav_TouchTargets(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	if n := strings.Count(body, `<button type="button" class="dark-mode-toggle p-2 `); n != 2 {
		t.Errorf("%d padded theme toggles, want 2 (desktop and mobile)", n)
	}
	if !strings.Contains(body, `<button type="button" class="md:hidden -m-2 p-2 `) {
		t.Error("the menu button has no padding")
	}
}

// TestLayout_StatusRegions checks the live regions that announce
// changes a screen reader user cannot see: the number of search results
// on the home page and in the spotlight, and the result of a copy.
func TestLayout_StatusRegions(t *testing.T) {
	t.Parallel()
	mux := newTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{
		`<p id="search-status" role="status" class="sr-only"></p>`,
		`<p id="spotlight-status" role="status" class="sr-only"></p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page has no %q", want)
		}
	}

	js, err := fs.ReadFile(StaticFS, "ui.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"'search-results': 'search-status'", "'spotlight-results': 'spotlight-status'", "setAttribute('role', 'alert')"} {
		if !strings.Contains(string(js), want) {
			t.Errorf("ui.js has no %q", want)
		}
	}
}
