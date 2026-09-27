package web

import (
	"net/http"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/static"
)

// Compile the static Tailwind stylesheet from the templates tree.
// Mise installs the pinned standalone CLI directly from its GitHub release,
// so no Node.js toolchain is required. The output is committed; the CI drift
// gate re-runs this and fails on any difference.
//go:generate tailwindcss -i tailwind.input.css -o static/tailwind.css --minify

// StaticFS provides access to the embedded static files (htmx.min.js,
// etc.) at their base names.
var StaticFS = static.FS

// Cache-Control values of the static files. A request with the current
// content version in v may keep the file for a year, because a new
// version has a new URL. Other requests (relative URLs inside the
// stylesheets, favicon.ico, an old v after a deploy) keep it for a day
// and can revalidate it with the ETag.
const (
	staticCacheVersioned   = "public, max-age=31536000, immutable"
	staticCacheUnversioned = "public, max-age=86400"
)

// staticHandler serves the embedded files. r.URL.Path is the file name
// relative to the static directory, with or without a leading slash.
// Each file gets a weak ETag of its content: weak, because the
// compression middleware sends a gzip body under the same tag.
// http.FileServerFS answers a matching If-None-Match with 304.
func staticHandler() http.Handler {
	files := http.FileServerFS(StaticFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := static.Version(strings.TrimPrefix(r.URL.Path, "/")); v != "" {
			w.Header().Set("ETag", `W/"`+v+`"`)
			if r.URL.Query().Get("v") == v {
				w.Header().Set("Cache-Control", staticCacheVersioned)
			} else {
				w.Header().Set("Cache-Control", staticCacheUnversioned)
			}
		}
		files.ServeHTTP(w, r)
	})
}
