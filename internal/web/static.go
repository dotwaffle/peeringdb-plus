package web

import (
	"embed"
	"io/fs"
)

// Compile the static Tailwind stylesheet from the templates tree.
// cmd/tailwind-build fetches the pinned Tailwind standalone CLI
// (sha256-verified, cached under the user cache dir) — no Node.js
// install anywhere. The output is committed; the CI drift gate re-runs
// this and fails on any difference.
//go:generate go run github.com/dotwaffle/peeringdb-plus/cmd/tailwind-build -in tailwind.input.css -out static/tailwind.css

//go:embed static
var staticFiles embed.FS

// StaticFS provides access to embedded static files (htmx.min.js, etc.)
// with the "static/" prefix stripped so files are served at their base names.
var StaticFS, _ = fs.Sub(staticFiles, "static")
