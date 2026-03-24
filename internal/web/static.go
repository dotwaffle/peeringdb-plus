package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticFiles embed.FS

// StaticFS provides access to embedded static files (htmx.min.js, etc.)
// with the "static/" prefix stripped so files are served at their base names.
var StaticFS, _ = fs.Sub(staticFiles, "static")
