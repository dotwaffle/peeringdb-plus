package otel

import "context"

// dbSpansOffKey is the context key of WithoutDBSpans.
type dbSpansOffKey struct{}

// WithoutDBSpans marks ctx so that the traced database handle of
// internal/database emits no otelsql span for a statement run under it. The
// sync worker marks each cycle: a full cycle runs thousands of statements,
// and with their spans its trace is larger than the 5 MB per-trace limit of
// Grafana Cloud Tempo.
func WithoutDBSpans(ctx context.Context) context.Context {
	return context.WithValue(ctx, dbSpansOffKey{}, true)
}

// DBSpansOff reports whether ctx carries the mark of WithoutDBSpans.
func DBSpansOff(ctx context.Context) bool {
	off, _ := ctx.Value(dbSpansOffKey{}).(bool)
	return off
}
