package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CachingInput holds configuration for the HTTP caching middleware.
type CachingInput struct {
	// SyncTimeFn returns the last successful sync completion time.
	// Returns zero time if no successful sync has occurred.
	SyncTimeFn func() time.Time

	// SyncInterval is the configured duration between automatic sync runs.
	// Used to calculate Cache-Control max-age (interval + 120s buffer).
	SyncInterval time.Duration
}

// Caching returns middleware that sets Cache-Control and ETag headers on
// GET/HEAD responses, and returns 304 Not Modified for conditional requests
// when data has not changed since the last sync.
//
// Only GET and HEAD methods receive caching headers. POST and other mutation
// methods pass through unchanged. If no successful sync has occurred (zero
// sync time), no caching headers are set.
func Caching(in CachingInput) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			syncTime := in.SyncTimeFn()
			if syncTime.IsZero() {
				next.ServeHTTP(w, r)
				return
			}

			etag := computeETag(syncTime)
			maxAge := int(in.SyncInterval.Seconds()) + 120

			w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
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
