package pdbcompat

import (
	"net/url"
	"sync/atomic"
	"time"
)

// SyncClock holds the completion time of the newest successful sync of
// the local database. The handler sends it as meta.generated on a list
// that upstream serves from its API cache file, where meta.generated is
// the modification time of the file (2.83.0 api_cache.py:135). The
// zero value holds no time. It is safe for concurrent use.
type SyncClock struct {
	unixNano atomic.Int64
}

// Set stores t. A zero t clears the clock.
func (c *SyncClock) Set(t time.Time) {
	if t.IsZero() {
		c.unixNano.Store(0)
		return
	}
	c.unixNano.Store(t.UnixNano())
}

// Time returns the stored time, or the zero time when the clock is nil
// or holds no time.
func (c *SyncClock) Time() time.Time {
	if c == nil {
		return time.Time{}
	}
	ns := c.unixNano.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// generated returns the stored time as Unix seconds with a fraction, as
// Python os.path.getmtime returns it, or ok=false when the clock is nil
// or holds no time.
func (c *SyncClock) generated() (float64, bool) {
	t := c.Time()
	if t.IsZero() {
		return 0, false
	}
	return float64(t.UnixNano()) / 1e9, true
}

// SetSyncClock sets the clock of meta.generated. Without a clock, no
// response carries meta.generated.
func (h *Handler) SetSyncClock(c *SyncClock) {
	h.syncClock = c
}

// apiCacheLimit is the largest depth-0 limit that upstream serves from
// its live query (2.83.0 api_cache.py:100-106, API_CACHE_ALL_LIMITS
// unset).
const apiCacheLimit = 250

// servedFromCache reports whether upstream serves a list from its API
// cache file (2.83.0 api_cache.py:90-124). It needs what
// depthListIsLive checks, and also a depth of 0 or more (the cache has no
// file for a negative depth) and, at depth 0, no limit or a limit above
// apiCacheLimit.
func servedFromCache(lf listFilters, params url.Values, q string, suffixed bool, depth, limit int) bool {
	switch {
	case depth < 0:
		return false
	case depth == 0 && limit != 0 && limit <= apiCacheLimit:
		return false
	}
	return !depthListIsLive(lf, params, q, suffixed)
}

// listMeta returns the meta object of a list: meta.generated when
// upstream serves the list from its API cache and the clock holds a time,
// else an empty object.
func (h *Handler) listMeta(cached bool) any {
	if !cached {
		return struct{}{}
	}
	f, ok := h.syncClock.generated()
	if !ok {
		return struct{}{}
	}
	return map[string]any{"generated": f}
}
