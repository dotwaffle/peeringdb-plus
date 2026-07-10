package otel

import (
	"context"
	"testing"
)

func TestPrewarmCounters_NoError(t *testing.T) {
	// PrewarmCounters reads package-level Counter vars bound at package
	// init, so it must not panic without any setup call.
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	PrewarmCounters(context.Background())
}
