package templates

import "time"

// DataFreshness holds sync status information for the About page display.
type DataFreshness struct {
	Available  bool
	LastSyncAt time.Time
	Age        time.Duration
}
