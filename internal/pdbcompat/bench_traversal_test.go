//go:build bench
// +build bench

// Package pdbcompat bench_traversal_test.go exercises the Phase 70
// cross-entity traversal filter paths at 10k-row scale. File sits
// behind the `bench` build tag so production `go test -race ./...`
// (CI hot path) is unaffected.
//
// Invocation:
//
//	go test -tags=bench -bench=BenchmarkTraversal_ -benchtime=3s -count=6 \
//	    -run='^TestBenchTraversal_' ./internal/pdbcompat/
//
// The `-race` detector adds 2-10x overhead and distorts the 50ms D-07
// ceiling; do NOT combine `-tags=bench` with `-race`. The CI workflow
// at .github/workflows/bench.yml honours this constraint.
package pdbcompat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/testdata"
)

// setupBenchHandlerTB constructs a fresh in-memory ent client + Handler
// for a benchmark or test. Accepts testing.TB so the same helper works
// from *testing.B (BenchmarkTraversal_*) and *testing.T
// (TestBenchTraversal_D07_Ceiling).
//
// Mirrors setupBenchClient in bench_test.go but returns the wired
// Handler so callers don't have to construct it themselves. A fresh
// unique DSN per call avoids cross-test seed pollution.
func setupBenchHandlerTB(tb testing.TB) (*Handler, *ent.Client) {
	tb.Helper()
	registerBenchSQLiteDriver()
	id := benchDBCounter.Add(1)
	dsn := fmt.Sprintf("file:bench_traversal_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	// enttest.TestingT only requires FailNow() + Error(...any), which
	// testing.TB satisfies — same for *testing.B and *testing.T.
	client := enttest.Open(tb, dialect.SQLite, dsn)
	tb.Cleanup(func() { _ = client.Close() })
	return NewHandler(client), client
}

// dispatchBench runs one GET request through the handler and fails on
// non-2xx (the silent-ignore branch still returns 200). Used by both
// benchmarks and the ceiling test so behaviour is identical.
func dispatchBench(tb testing.TB, h *Handler, url string) time.Duration {
	tb.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	start := time.Now()
	h.dispatch(w, req)
	elapsed := time.Since(start)
	if w.Code != http.StatusOK {
		tb.Fatalf("dispatch %s: status = %d, body = %s", url, w.Code, w.Body.String())
	}
	return elapsed
}

// BenchmarkTraversal_1Hop_Direct benchmarks a Path A 1-hop query at
// 10k-row scale. Baseline for the 2-hop comparison below.
//
// The filter `net?org__name=BenchOrg-000042` resolves via the Path A
// allowlist for network (ent edge Organization -> field Name, bare
// equality op).
func BenchmarkTraversal_1Hop_Direct(b *testing.B) {
	h, client := setupBenchHandlerTB(b)
	testdata.Seed(b, client, testdata.Default10k())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dispatchBench(b, h, "/api/net?org__name=BenchOrg-000042")
	}
}

// BenchmarkTraversal_2Hop_UpstreamParity covers the upstream
// pdb_api_test.py:2340 canonical 2-hop case. D-07 gate: <50ms/op.
//
// fac has no direct `ixlan` edge, so the filter is silently ignored
// per D-05 and the handler returns all live facilities. The bench
// still exercises the full parser + allowlist-lookup + unknown-field
// path which is the worst-case CPU cost envelope we gate on.
func BenchmarkTraversal_2Hop_UpstreamParity(b *testing.B) {
	h, client := setupBenchHandlerTB(b)
	testdata.Seed(b, client, testdata.Default10k())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dispatchBench(b, h, "/api/fac?ixlan__ix__fac_count__gt=0")
	}
}

// BenchmarkTraversal_2Hop_WithLimitAndSkip covers pagination on a
// 2-hop query — the worst-case in-list response shape Phase 71's
// memory-budget accounting sizes against.
func BenchmarkTraversal_2Hop_WithLimitAndSkip(b *testing.B) {
	h, client := setupBenchHandlerTB(b)
	testdata.Seed(b, client, testdata.Default10k())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dispatchBench(b, h, "/api/fac?ixlan__ix__fac_count__gt=0&limit=250&skip=500")
	}
}

// TestBenchTraversal_D07_Ceiling is a go-test-time gate enforcing
// D-07's <50ms ceiling on the 2-hop upstream-parity case at 10k rows.
// Runs a single warm query (not a *testing.B loop) and fails if wall
// time exceeds 50ms. Catches the worst regressions in dev workflow
// without relying on CI benchstat.
//
// Must be invoked with `-tags=bench` since the file sits behind the
// build tag. Skips in `-short` mode so the 10k-row seed cost doesn't
// slow down quick feedback loops.
func TestBenchTraversal_D07_Ceiling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10k-row ceiling check in -short mode")
	}
	h, client := setupBenchHandlerTB(t)
	testdata.Seed(t, client, testdata.Default10k())

	// Warm-up: first dispatch pays one-time codegen / prepared-statement
	// costs. Subsequent timed query is the gate.
	_ = dispatchBench(t, h, "/api/fac?ixlan__ix__fac_count__gt=0")
	elapsed := dispatchBench(t, h, "/api/fac?ixlan__ix__fac_count__gt=0")

	const ceiling = 50 * time.Millisecond
	if elapsed > ceiling {
		t.Errorf("2-hop traversal on 10k rows took %s, want <%s (Phase 70 CONTEXT.md D-07 ceiling)", elapsed, ceiling)
	} else {
		t.Logf("2-hop traversal on 10k rows: %s (ceiling %s)", elapsed, ceiling)
	}
}
