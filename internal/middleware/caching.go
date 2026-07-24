package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// CachingState holds the HTTP caching middleware's mutable ETag value behind
// an atomic pointer so the hot request path can read it without locks or
// per-request SHA-256 computation. The hash work was moved
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
	// Cache-Control max-age is immutable across the process lifetime
	// because syncInterval is locked at construction. Format both the
	// public and private variants once to avoid a fmt.Sprintf per request.
	//
	// Public deployments (PDBPLUS_PUBLIC_TIER=public, the default) serve
	// only public data and may be retained by shared/CDN caches → "public".
	// A deployment configured for the Users tier serves private-audience
	// data, so its responses must not be stored by shared caches → "private"
	// (browser caching is still allowed). The per-request tier is read from
	// the context stamped by the PrivacyTier middleware earlier in the
	// chain; an unstamped context fails closed to TierPublic, which is the
	// same failure mode under which the privacy policy hides Users rows, so
	// "public" is then correct for the public-only body (audit S3).
	maxAge := int(s.syncInterval.Seconds()) + 120
	publicCacheCtrl := fmt.Sprintf("public, max-age=%d", maxAge)
	privateCacheCtrl := fmt.Sprintf("private, max-age=%d", maxAge)

	// Snapshot the skip paths into the closure so the request path
	// never touches the CachingState struct.
	skipPaths := slices.Clone(s.skipPaths)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			// Agent skill handlers derive their own content ETags. In
			// particular, the downloadable archive varies by request origin,
			// so the sync-time-keyed application ETag cannot safely short
			// circuit it. Leave the entire /skills/ namespace to its handler.
			if strings.HasPrefix(r.URL.Path, "/skills/") {
				next.ServeHTTP(w, r)
				return
			}

			// Embedded static assets change only on deploy, never on
			// sync, so the sync-time-keyed ETag is the wrong key: it
			// would invalidate every stylesheet and script each sync
			// cycle. A fixed day-long public max-age is appropriate for
			// content this stable (and self-corrects within a day of a
			// deploy that changes an asset).
			if strings.HasPrefix(r.URL.Path, "/static/") {
				w.Header().Set("Cache-Control", "public, max-age=86400")
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

			cacheCtrl := publicCacheCtrl
			if privctx.TierFrom(r.Context()) != privctx.TierPublic {
				cacheCtrl = privateCacheCtrl
			}
			w.Header().Set("Cache-Control", cacheCtrl)
			w.Header().Set("ETag", etag)

			if etagMatch(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}

			// Error responses must not inherit the public caching
			// headers: a shared cache replaying a 404/413/500 for a
			// full sync interval turns a transient failure into an
			// hour-long outage for that URL. The wrapper downgrades
			// to no-store at WriteHeader time for any status >= 400.
			next.ServeHTTP(&cacheStripWriter{ResponseWriter: w}, r)
		})
	}
}

// cacheStripWriter downgrades the caching headers to no-store when the
// wrapped handler responds with an error status. Implements http.Flusher
// and Unwrap per the middleware response-writer contract (CLAUDE.md
// Middleware section).
type cacheStripWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

// WriteHeader strips Cache-Control/ETag for error statuses, then
// delegates. Only the first call wins, matching net/http semantics.
func (w *cacheStripWriter) WriteHeader(code int) {
	if w.wroteHeader {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.wroteHeader = true
	if code >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write commits an implicit 200 on first write, like net/http.
func (w *cacheStripWriter) Write(b []byte) (int, error) {
	if w.wroteHeader {
		return w.ResponseWriter.Write(b)
	}
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for middleware-aware
// interface detection.
func (w *cacheStripWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush forwards to the underlying writer per the http.Flusher contract.
func (w *cacheStripWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// computeETag produces a weak ETag from the sync timestamp.
// Format: W/"<32 hex chars>" (SHA-256 truncated to 16 bytes).
//
// The input format (SHA-256 of RFC3339Nano-formatted sync time) is preserved
// byte-for-byte from the earlier implementation so existing client cache
// entries keep matching across the v1.13 deploy. Switching to a sync-ID
// counter was considered and rejected — the timestamp-hash form needs no
// persisted counter state and is stable across restarts.
func computeETag(syncTime time.Time) string {
	h := sha256.Sum256([]byte(syncTime.Format(time.RFC3339Nano)))
	return fmt.Sprintf(`W/"%x"`, h[:16])
}

// etagMatch reports whether the If-None-Match header value matches the
// current ETag. Per RFC 9110 §13.1.2 the header carries a comma-separated
// list of entity tags (or the "*" wildcard, which matches any ETag);
// clients that cached several representations legitimately send
// `W/"a", W/"b"` and expect a 304 when ANY member matches — the previous
// whole-string comparison failed every multi-tag request into a full 200.
func etagMatch(ifNoneMatch, etag string) bool {
	ifNoneMatch = strings.TrimSpace(ifNoneMatch)
	if ifNoneMatch == "" {
		return false
	}
	if ifNoneMatch == "*" {
		return true
	}
	// Commas cannot appear inside an entity-tag (RFC 9110 etagc excludes
	// them), so a plain split is a correct list parse.
	for tag := range strings.SplitSeq(ifNoneMatch, ",") {
		if strings.TrimSpace(tag) == etag {
			return true
		}
	}
	return false
}
