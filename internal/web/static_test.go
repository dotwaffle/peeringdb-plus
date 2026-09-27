package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
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
