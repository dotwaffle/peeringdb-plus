package otel

import (
	"context"
	"testing"
)

func TestPrewarmCounters_NoError(t *testing.T) {
	// PrewarmCounters reads package-level Counter vars populated by
	// InitMetrics(). The function nil-guards each counter and surfaces
	// missing instruments via otel.Handle rather than panicking, so this
	// test locks the happy path: a clean post-InitMetrics call doesn't
	// error and doesn't panic.
	//
	// Match the package-wide convention (see internal/otel/metrics_test.go:
	// 10 occurrences) of pinning OTEL_METRICS_EXPORTER=none so InitMetrics()
	// does not attempt to dial an OTLP endpoint via autoexport during the
	// test.
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}
	PrewarmCounters(context.Background())
}
