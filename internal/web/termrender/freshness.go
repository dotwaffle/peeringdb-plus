package termrender

import (
	"time"
)

// FormatFreshness formats a sync timestamp as a styled footer line for terminal
// responses. Returns "Data: {RFC3339}" with leading and trailing newlines for
// visual separation. Returns "" for zero time (footer omitted). (DIF-02, D-13,
// D-14, D-15)
//
// The output is intentionally free of wall-clock-relative phrasing ("N minutes
// ago"). The terminal footer is rendered into responses that are cached by
// the sync-time-keyed HTTP caching middleware, and any relative text would
// freeze at cache-creation time and mislead readers for up to a full sync
// interval. Readers who want a relative age can compute it locally from the
// absolute RFC3339 timestamp.
func FormatFreshness(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	line := "Data: " + t.UTC().Format(time.RFC3339)
	return "\n" + StyleMuted.Render(line) + "\n"
}
