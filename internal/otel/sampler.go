// Package otel — sampler.go provides perRouteSampler, a composite
// sdktrace.Sampler that dispatches sampling decisions based on the HTTP
// route prefix read from SamplingParameters.Attributes (url.path /
// http.target). Per CONTEXT.md D-02 and AUDIT.md § Recommended sampling
// matrix, used to keep /healthz + /readyz at low ratio while
// /api/* /rest/v1/* /peeringdb.v1.* stay at full ratio. Wrapped in
// sdktrace.ParentBased at provider.go so child span decisions inherit
// from the root.
package otel

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// PerRouteSamplerInput configures NewPerRouteSampler.
//
// Routes maps URL-path prefix (e.g. "/healthz", "/api/") to a 0–1
// sampling ratio. The constructor normalises trailing slashes, so
// callers may use either "/api" or "/api/" interchangeably. Longest
// matching prefix wins at sample time, so a future "/api/auth/" entry
// can override the broader "/api/" entry.
//
// DefaultRatio applies when no prefix matches AND when no
// url.path / http.target attribute is present (sync-worker spans,
// internal traces).
type PerRouteSamplerInput struct {
	DefaultRatio float64
	Routes       map[string]float64
}

// NewPerRouteSampler builds a Sampler that dispatches per HTTP route
// prefix. The returned Sampler is safe for concurrent use after
// construction. Internally it pre-computes a sorted-by-length prefix
// list so longer prefixes win (e.g. "/api/auth/" beats "/api/").
//
// Per-route samplers are sdktrace.TraceIDRatioBased(ratio) so inherent
// TraceID-deterministic sampling is preserved across services.
func NewPerRouteSampler(in PerRouteSamplerInput) sdktrace.Sampler {
	defaultSampler := sdktrace.TraceIDRatioBased(in.DefaultRatio)

	entries := make([]routeEntry, 0, len(in.Routes))
	for prefix, ratio := range in.Routes {
		entries = append(entries, routeEntry{
			prefix:  normalisePrefix(prefix),
			sampler: sdktrace.TraceIDRatioBased(ratio),
			ratio:   ratio,
		})
	}
	// Sort by prefix length descending so the longest prefix wins on
	// match. Stable order isn't needed since prefixes are unique post-
	// normalisation (callers can't register both "/api" and "/api/" —
	// they collapse to the same key).
	slices.SortFunc(entries, func(a, b routeEntry) int {
		return cmp.Compare(len(b.prefix), len(a.prefix))
	})

	return &perRouteSampler{
		defaultSampler: defaultSampler,
		defaultRatio:   in.DefaultRatio,
		entries:        entries,
	}
}

// normalisePrefix strips a single trailing slash so "/api" and "/api/"
// collapse to the same canonical form. The check at sample time uses
// strings.HasPrefix; for "/api" to match "/api/network", the prefix
// stored is "/api" (no trailing slash). Single-slash root "/" is
// preserved as-is.
func normalisePrefix(p string) string {
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		return strings.TrimRight(p, "/")
	}
	return p
}

type routeEntry struct {
	prefix  string
	sampler sdktrace.Sampler
	ratio   float64
}

type perRouteSampler struct {
	defaultSampler sdktrace.Sampler
	defaultRatio   float64
	entries        []routeEntry
}

// ShouldSample decides whether to record-and-export a span.
//
// Implementation reads SamplingParameters.Attributes once, extracting
// url.path then http.target (legacy semconv fallback). Both keys are
// inspected because otelhttp's RequestTraceAttrs may emit either depending
// on the semconv version pinned in go.mod. The first non-empty match
// drives the prefix lookup; if neither is present (sync-worker span,
// internal trace), the default sampler is consulted.
//
// Hot-path allocation is bounded — the entries slice is pre-sorted at
// construction time and the SamplingResult is the only allocation.
func (s *perRouteSampler) ShouldSample(params sdktrace.SamplingParameters) sdktrace.SamplingResult {
	path := pathFromAttributes(params.Attributes)
	if path == "" {
		return s.defaultSampler.ShouldSample(params)
	}

	for _, e := range s.entries {
		if matchesPrefix(path, e.prefix) {
			return e.sampler.ShouldSample(params)
		}
	}
	return s.defaultSampler.ShouldSample(params)
}

// matchesPrefix returns true when path begins with prefix at a logical
// route boundary. Two boundary rules:
//
//  1. If the prefix ends in an alphanumeric character (e.g. "/api"),
//     the next character of path must be '/' or end-of-string. Prevents
//     "/api" from accidentally matching "/apifoo".
//  2. If the prefix ends in a non-alphanumeric character (e.g.
//     "/peeringdb.v1." for ConnectRPC, "/static/"), any next character
//     is accepted — the prefix already includes its own boundary.
//
// Exact match (path == prefix) is always accepted regardless of rule.
func matchesPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	if !isAlnum(prefix[len(prefix)-1]) {
		return true
	}
	return path[len(prefix)] == '/'
}

func isAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// pathFromAttributes returns the URL path from SamplingParameters.Attributes,
// preferring url.path (semconv v1.21+) over http.target (legacy semconv).
// Returns "" when neither is present.
func pathFromAttributes(attrs []attribute.KeyValue) string {
	var legacy string
	for _, kv := range attrs {
		switch kv.Key {
		case "url.path":
			if v := kv.Value.AsString(); v != "" {
				return v
			}
		case "http.target":
			if v := kv.Value.AsString(); v != "" {
				legacy = v
			}
		}
	}
	return legacy
}

// Description returns a stable human-readable identifier for OTel debug
// output. Includes the marker "PerRouteSampler" and the configured
// route count for at-a-glance diagnostics.
func (s *perRouteSampler) Description() string {
	return fmt.Sprintf("PerRouteSampler{routes=%d, default=TraceIDRatioBased(%v)}", len(s.entries), s.defaultRatio)
}
