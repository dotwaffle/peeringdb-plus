package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

// CachingState holds the HTTP caching middleware's mutable ETag value behind
// an atomic pointer so the hot request path can read it without locks or
// per-request SHA-256 computation. PERF-07 (Plan 55-02) moved the hash work
// out of the request path; the SHA-256 now runs exactly once per sync
// completion inside UpdateETag, driven by the sync worker's OnSyncComplete
// callback. Construct via NewCachingState.
//
// A zero-value etag pointer (never called UpdateETag, i.e. pre-sync startup)
// is the documented pre-sync state: the Middleware skips Cache-Control and
// ETag headers and passes the request straight to the inner handler. This
// preserves the v1.0-v1.12 "no caching headers before first successful sync"
// semantic verbatim.
type CachingState struct {
	// etag holds the current weak ETag string; nil pointer means pre-sync.
	// Readers use etag.Load(); the writer is UpdateETag. Embedded as a value
	// (not *atomic.Pointer[string]) because atomic.Pointer methods take pointer
	// receivers and CachingState is itself always heap-allocated via
	// NewCachingState — one allocation, not two.
	etag atomic.Pointer[string]

	// syncInterval is the configured duration between automatic sync runs.
	// Used to calculate Cache-Control max-age (interval + 120s buffer). Captured
	// once at construction; immutable thereafter.
	syncInterval time.Duration

	// skipPaths is the list of request paths (exact r.URL.Path match) that
	// must not be cached by the sync-time-keyed ETag. Intended for pages
	// containing wall-clock-relative text (e.g. "5 minutes ago") that goes
	// stale at wall-clock cadence rather than sync cadence. Captured once
	// at construction via NewCachingState; the Middleware closure snapshots
	// it so per-request reads never touch the CachingState struct.
	skipPaths []string
}

// NewCachingState returns a CachingState with a nil ETag pointer (pre-sync)
// and the given sync interval. Call UpdateETag once the first sync completes;
// until then, Middleware skips caching headers.
//
// Any additional skipPaths are treated as exact-match r.URL.Path opt-outs:
// matching requests receive Cache-Control: no-store, bypass the 304
// short-circuit, and always reach the inner handler. Use for pages that
// contain wall-clock-relative rendering (e.g. "5 minutes ago") which would
// freeze at cache-creation time under the sync-time-keyed ETag and mislead
// users for up to a full sync interval.
func NewCachingState(syncInterval time.Duration, skipPaths ...string) *CachingState {
	return &CachingState{
		syncInterval: syncInterval,
		skipPaths:    slices.Clone(skipPaths),
	}
}

// UpdateETag computes a fresh weak ETag from syncTime and stores it atomically.
// Safe to call concurrently with any number of Middleware reads. Intended to
// be called from the sync worker's OnSyncComplete callback exactly once per
// successful sync (i.e. once per hour at default settings), not on the request
// path. The SHA-256 cost thus moves from O(requests) to O(syncs).
func (s *CachingState) UpdateETag(syncTime time.Time) {
	v := computeETag(syncTime)
	s.etag.Store(&v)
}

// Middleware returns the HTTP caching middleware factory. The returned factory
// captures syncInterval and the pre-computed Cache-Control header value once,
// then on every request issues exactly one atomic load of the current ETag —
// no SHA-256, no time formatting, no DB access. If the ETag pointer is nil
// (pre-sync state), caching headers are skipped and the request passes through
// unchanged.
//
// Only GET and HEAD requests receive caching treatment; mutation methods pass
// through untouched. Conditional requests with a matching If-None-Match (or
// the "*" wildcard) return 304 Not Modified with an empty body.
//
// Paths registered via NewCachingState's skipPaths argument are opted out
// entirely: they receive Cache-Control: no-store, bypass the 304 short-circuit,
// and always reach the inner handler.
func (s *CachingState) Middleware() func(http.Handler) http.Handler {
	// Cache-Control value is immutable across the process lifetime because
	// syncInterval is locked at construction. Format once to avoid a
	// fmt.Sprintf per request.
	maxAge := int(s.syncInterval.Seconds()) + 120
	cacheCtrl := fmt.Sprintf("public, max-age=%d", maxAge)

	// Snapshot the skip paths into the closure so the request path
	// never touches the CachingState struct.
	skipPaths := slices.Clone(s.skipPaths)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			// Opt-out list: pages with wall-clock-relative rendering
			// (e.g. /ui/about's "5 minutes ago") must never be cached
			// by the sync-time-keyed ETag, or the relative text freezes
			// at cache-creation time and misleads users for up to a
			// full sync interval. Exact path match is sufficient for
			// current use — prefix matching can be added if needed.
			if slices.Contains(skipPaths, r.URL.Path) {
				w.Header().Set("Cache-Control", "no-store")
				next.ServeHTTP(w, r)
				return
			}

			etagPtr := s.etag.Load()
			if etagPtr == nil {
				// Pre-sync: no ETag available yet, skip caching headers
				// to preserve v1.0 "no sync time -> no headers" contract.
				next.ServeHTTP(w, r)
				return
			}
			etag := *etagPtr

			w.Header().Set("Cache-Control", cacheCtrl)
			w.Header().Set("ETag", etag)

			if etagMatch(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// computeETag produces a weak ETag from the sync timestamp.
// Format: W/"<32 hex chars>" (SHA-256 truncated to 16 bytes).
//
// The input format (SHA-256 of RFC3339Nano-formatted sync time) is preserved
// byte-for-byte from the pre-PERF-07 implementation so existing client cache
// entries keep matching across the v1.13 deploy. Switching to a sync-ID
// counter was considered and rejected at plan time — see 55-02-PLAN.md.
func computeETag(syncTime time.Time) string {
	h := sha256.Sum256([]byte(syncTime.Format(time.RFC3339Nano)))
	return fmt.Sprintf(`W/"%x"`, h[:16])
}

// etagMatch reports whether the If-None-Match header value matches the
// current ETag. Supports the "*" wildcard which matches any ETag.
func etagMatch(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	return strings.TrimSpace(ifNoneMatch) == etag
}
