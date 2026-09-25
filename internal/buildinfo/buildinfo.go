// Package buildinfo exposes the build-time version string used by both the
// PeeringDB User-Agent and the OTel resource so they stay in lockstep.
//
// Resolution order:
//
//  1. injected: set with ldflags `-X github.com/dotwaffle/peeringdb-plus/internal/buildinfo.injected=<value>`
//     only when a Docker build gets an explicit VERSION build argument,
//     for a build context without .git.
//  2. Main.Version from runtime/debug.ReadBuildInfo. `go build` in a git
//     checkout stamps it (Go 1.24 and later): the tag on a tagged commit
//     (v1.32.0), a pseudo-version between tags
//     (v1.32.1-0.20260925001332-10674814d276), and a +dirty suffix when
//     the tree differs from the commit. This is the production path: both
//     Dockerfiles keep .git and every tracked file in the build context.
//     `go install` of a tagged module path also sets it.
//  3. vcs.revision (first 7 chars) from build settings, for a build that
//     recorded the commit but no module version.
//  4. Literal "unknown": last resort.
//
// A `go test` binary has no version stamp and falls through to (3) or
// (4). The peeringdb User-Agent test asserts the surrounding shape, not
// the version literal, so test runs stay stable.
package buildinfo

import (
	"runtime/debug"
	"time"
)

// injected is set via -ldflags only by a Docker build with an explicit
// VERSION build argument. Empty by default, so every other build falls
// through to runtime build info.
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

// SourceTime returns the UTC source timestamp recorded by the Go toolchain.
// It returns the zero time when build information or vcs.time is unavailable
// or malformed.
func SourceTime() time.Time {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return time.Time{}
	}
	return sourceTime(info)
}

func sourceTime(info *debug.BuildInfo) time.Time {
	for _, setting := range info.Settings {
		if setting.Key != "vcs.time" {
			continue
		}
		value, err := time.Parse(time.RFC3339, setting.Value)
		if err != nil {
			return time.Time{}
		}
		return value.UTC()
	}
	return time.Time{}
}
