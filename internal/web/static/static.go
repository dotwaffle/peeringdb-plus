// Package static holds the embedded web assets and their content
// versions. It is a leaf package so that the templates and the
// readiness middleware can link versioned asset URLs.
//
// flag-icons is flag-icons 7.5.0 (MIT, flag-icons/LICENSE) from the npm
// tarball: the 4x3 flags, and css/flag-icons.min.css without the 1x1
// (".fis") rules, whose flags are not vendored. The UI uses only the
// "fi fi-<cc>" classes (templates.CountryFlag).
package static

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
)

// FS holds the assets, served under /static/ at their names in this
// directory.
//
//go:embed *.css *.js *.svg *.ico images flag-icons
var FS embed.FS

// versionLen is the number of hex digits of a file's SHA-256 that make
// its version.
const versionLen = 12

// versions maps each file name to its version. The embedded files are
// fixed at build time, so the map is computed once.
var versions = hashFiles(FS)

func hashFiles(fsys fs.FS) map[string]string {
	out := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[name] = hex.EncodeToString(sum[:])[:versionLen]
		return nil
	})
	if err != nil {
		panic("static: hash embedded files: " + err.Error())
	}
	return out
}

// Version returns the content version of the named file, or "" when no
// such file is embedded.
func Version(name string) string {
	return versions[name]
}

// URL returns the URL of the named file with its version in the v
// query parameter, for example /static/ui.js?v=0123456789ab. The URL
// changes when the content changes, so the static handler lets
// browsers keep a file fetched through it for a year. URL panics for a
// name that is not embedded, so a mistyped asset fails in tests.
func URL(name string) string {
	v, ok := versions[name]
	if !ok {
		panic("static: no embedded file " + name)
	}
	return "/static/" + name + "?v=" + v
}
