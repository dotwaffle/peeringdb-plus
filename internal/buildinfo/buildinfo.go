// Package buildinfo exposes the build-time version string used by both the
// PeeringDB User-Agent and the OTel resource so they stay in lockstep.
//
// Resolution order:
//
//  1. injected — set via ldflags `-X github.com/dotwaffle/peeringdb-plus/internal/buildinfo.injected=<value>`
//     at Docker build time (computed from `git describe --tags --always --dirty`).
//     This is the production path: tagged releases emit `v1.17`, post-tag
//     dev builds emit `v1.17-3-gabc1234`, dirty trees emit a `-dirty` suffix.
//  2. Main.Version from runtime/debug.ReadBuildInfo — populated for
//     `go install` of a tagged module path (the module-proxy path).
//  3. vcs.revision (first 7 chars) from build settings — `go build` from
//     a local checkout with `.git` present.
//  4. Literal "unknown" — last resort.
//
// Without injection (e.g. under `go test`), the test-mode binaries fall
// through to (3) or (4); the peeringdb User-Agent test asserts the
// surrounding shape, not the version literal, so test runs stay stable.
package buildinfo

import "runtime/debug"

// injected is set via -ldflags at Docker build time. Empty by default so
// non-Docker builds (go test, go run, local go build) fall through to
// runtime build info.
var injected = ""

// Version returns the resolved build version. See package doc for the
// resolution order. The result is computed once per call but is cheap;
// callers may still cache at package init if they need a stable string.
func Version() string {
	if injected != "" {
		return injected
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return "unknown"
}
