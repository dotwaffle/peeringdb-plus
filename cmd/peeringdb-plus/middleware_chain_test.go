package main

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestMiddlewareChain_Order is a source-scan regression lock against
// reorder or accidental removal of any middleware in the production
// chain. It parses the body of buildMiddlewareChain directly from
// main.go and asserts that every expected middleware appears exactly
// once, in the innermost-first order.
//
// This is deliberately structural, not runtime: spinning up the real
// stack in-process would pull in the sync worker, the ent client, and
// OTel autoexport. A grep-level test is fragile by name but cheap, and
// any drift between the source and wantOrder fails CI immediately.
//
// If buildMiddlewareChain gains or loses a middleware, update wantOrder
// in lockstep with the code.
func TestMiddlewareChain_Order(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}

	const startMarker = "func buildMiddlewareChain("
	start := bytes.Index(src, []byte(startMarker))
	if start < 0 {
		t.Fatalf("%q not found in main.go", startMarker)
	}
	// The function body ends at the closing `return h` statement.
	tail := src[start:]
	before, _, ok := bytes.Cut(tail, []byte("return h"))
	if !ok {
		t.Fatalf("%q not found in buildMiddlewareChain", "return h")
	}
	body := string(before)

	// wantOrder is innermost-first (the order lines are wrapped in the
	// code). The runtime order a request traverses is the reverse:
	// Recovery runs first, Gzip runs last.
	//
	// Each entry includes the trailing "(" so that the match is guaranteed
	// to be a call site, not a substring of a type name (e.g. "middleware.CSP"
	// would otherwise match "middleware.CSPInput{}").
	//
	// PERF-07 (Plan 55-02): the caching middleware is now wrapped via
	// `cc.CachingState.Middleware()(h)` instead of `middleware.Caching(...)(h)`
	// because the ETag cache moved to an atomic.Pointer that is updated
	// from OnSyncComplete — the call-site pattern changed accordingly.
	wantOrder := []string{
		"middleware.Compression(",
		"cc.CachingState.Middleware(",
		"middleware.CSP(",
		"middleware.SecurityHeaders(",
		"readinessMiddleware(",
		"middleware.Logging(",
		"otelhttp.NewMiddleware(",
		"middleware.CORS(",
		"middleware.MaxBytesBody(",
		"middleware.Recovery(",
	}

	type hit struct {
		name string
		pos  int
	}
	hits := make([]hit, 0, len(wantOrder))
	for _, name := range wantOrder {
		idx := strings.Index(body, name)
		if idx < 0 {
			t.Errorf("middleware %q not found in buildMiddlewareChain body", name)
			continue
		}
		hits = append(hits, hit{name: name, pos: idx})
	}
	// Sort by source position to compare against wantOrder.
	sort.Slice(hits, func(i, j int) bool { return hits[i].pos < hits[j].pos })

	got := make([]string, len(hits))
	for i, h := range hits {
		got[i] = h.name
	}

	if len(got) != len(wantOrder) {
		t.Fatalf("middleware count mismatch: got %d middlewares (%v), want %d (%v)",
			len(got), got, len(wantOrder), wantOrder)
	}
	for i := range wantOrder {
		if got[i] != wantOrder[i] {
			t.Errorf("middleware chain order mismatch at position %d: got %q, want %q\n  full got:  %v\n  full want: %v",
				i, got[i], wantOrder[i], got, wantOrder)
		}
	}
}
