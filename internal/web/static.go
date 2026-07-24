package web

import (
	"embed"
	"io/fs"
)

// Compile the static Tailwind stylesheet from the templates tree.
// Mise installs the pinned standalone CLI directly from its GitHub release,
// so no Node.js toolchain is required. The output is committed; the CI drift
// gate re-runs this and fails on any difference.
//go:generate tailwindcss -i tailwind.input.css -o static/tailwind.css --minify

//go:embed static
var staticFiles embed.FS

// StaticFS provides access to embedded static files (htmx.min.js, etc.)
// with the "static/" prefix stripped so files are served at their base names.
var StaticFS, _ = fs.Sub(staticFiles, "static")
