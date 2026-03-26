package termrender

import (
	"fmt"
	"time"
)

// FormatFreshness formats a sync timestamp as a styled footer line for terminal
// responses. Returns "Data: {RFC3339} ({relative})" with leading and trailing
// newlines for visual separation. Returns "" for zero time (footer omitted).
// (DIF-02, D-13, D-14, D-15)
func FormatFreshness(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	age := time.Since(t)
	ageStr := formatRelativeAge(age)
	line := fmt.Sprintf("Data: %s (%s)", t.UTC().Format(time.RFC3339), ageStr)

	return "\n" + StyleMuted.Render(line) + "\n"
}

// formatRelativeAge converts a duration to a human-readable relative string.
func formatRelativeAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < 2*time.Minute:
		return "1 minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "1 hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "1 day ago"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
