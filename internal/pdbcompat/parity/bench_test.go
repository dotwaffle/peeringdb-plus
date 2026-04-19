package parity

// Parity benchmark companion (Phase 72 CONTEXT.md D-07).
//
// Locks cost envelopes on the v1.16 performance-sensitive pdbcompat
// paths so a future PR that regresses performance (re-materialises
// limit=0, drops the json_each(?) __in rewrite, or re-introduces
// per-row N+1 traversal lookups) shows up on the benchstat diff even
// when the correctness tests in this package still pass.
//
// All three benchmarks follow the modern b.Loop() idiom per GO-TOOL-1
// and the Phase 46 / projection_bench_test.go precedent — no
// hand-rolled `for i := 0; i < b.N; i++` loops.
//
// CI workflow: these benchmarks run on pushes to main via a
// benchstat-comparing job (not per-PR — benchmark numbers are noisy
// and would block merges). Benchmarks skip automatically under the
// default `go test ./...` invocation since b.Loop is test-suite-inert
// without -bench.
//
// Local dev commands:
//
//	# Quick smoke (one iteration per benchmark):
//	go test -run=^$ -bench=BenchmarkParity -benchtime=1x \
//	    ./internal/pdbcompat/parity/
//
//	# benchstat-ready comparison run:
//	go test -run=^$ -bench=BenchmarkParity -benchtime=5x -count=6 \
//	    ./internal/pdbcompat/parity/ | benchstat -
//
// The benchmarks share the parity harness (seedFixtures, newTestServer,
// httpGet) which was widened from *testing.T to testing.TB in this
// plan so the same setup works from both *testing.B and *testing.T
// call sites.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	parityfix "github.com/dotwaffle/peeringdb-plus/internal/testutil/parity"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// BenchmarkParity_TwoHopTraversal measures the canonical Phase 70
// Path A 2-hop case: `/api/ixpfx?ixlan__ix__id=N`. The underlying
// filter pipeline resolves via ent edge ixpfx → ixlan → ix, both of
// which exist in the schema and in the Allowlist.
//
// Regression signal: the D-07 wall-clock ceiling lives in
// internal/pdbcompat/bench_traversal_test.go (at 10k-row scale). This
// parity-companion benchmark tracks the smaller-N shape that the
// category-split parity suite locks for correctness, so a bench-shape
// mismatch would show up here without needing the full 10k seed.
//
// Upstream citation: pdb_api_test.py:3203 (ixpfx scoped via ixlan →
// ix is the canonical Path A 2-hop site; pdb_api_test.py:5081 is its
// sibling org-scoped 1-hop form).
func BenchmarkParity_TwoHopTraversal(b *testing.B) {
	b.ReportAllocs()

	c := testutil.SetupClient(b)
	ctx := b.Context()
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// Inline seeds — mirrors TRAVERSAL-02 in traversal_test.go. The
	// mustOrg/mustIX/... helpers in that file take *testing.T and
	// aren't callable here; widening them further would bloat the
	// plan's widening footprint without benefit.
	if _, err := c.Organization.Create().
		SetID(1).SetName("IXOrg").SetNameFold(unifold.Fold("IXOrg")).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed org: %v", err)
	}
	if _, err := c.InternetExchange.Create().
		SetID(20).SetName("TargetIX").SetNameFold(unifold.Fold("TargetIX")).
		SetOrgID(1).SetCity("TestCity").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ix 20: %v", err)
	}
	if _, err := c.InternetExchange.Create().
		SetID(21).SetName("OtherIX").SetNameFold(unifold.Fold("OtherIX")).
		SetOrgID(1).SetCity("TestCity").SetCountry("DE").
		SetRegionContinent("Europe").SetMedia("Ethernet").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ix 21: %v", err)
	}
	if _, err := c.IxLan.Create().
		SetID(200).SetName("TargetLan").SetIxID(20).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ixlan 200: %v", err)
	}
	if _, err := c.IxLan.Create().
		SetID(210).SetName("OtherLan").SetIxID(21).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ixlan 210: %v", err)
	}
	if _, err := c.IxPrefix.Create().
		SetID(1000).SetPrefix("10.0.0.0/24").SetProtocol("IPv4").
		SetIxlanID(200).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ixpfx 1000: %v", err)
	}
	if _, err := c.IxPrefix.Create().
		SetID(1001).SetPrefix("10.0.1.0/24").SetProtocol("IPv4").
		SetIxlanID(200).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ixpfx 1001: %v", err)
	}
	if _, err := c.IxPrefix.Create().
		SetID(2000).SetPrefix("10.1.0.0/24").SetProtocol("IPv4").
		SetIxlanID(210).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).
		Save(ctx); err != nil {
		b.Fatalf("seed ixpfx 2000: %v", err)
	}

	srv := newTestServer(b, c)
	const path = "/api/ixpfx?ixlan__ix__id=20"

	b.ResetTimer()
	for b.Loop() {
		status, body := httpGet(b, srv, path)
		if status != 200 {
			b.Fatalf("want 200; got %d", status)
		}
		// Sanity floor: envelope alone is > 20 bytes; a truncated
		// body here would silently inflate ns/op.
		if len(body) < 50 {
			b.Fatalf("suspiciously small body: %d bytes", len(body))
		}
	}
}

// BenchmarkParity_LimitZeroStreaming exercises Phase 71 stream.go
// end-to-end at a 5000-row population. `?limit=0` is the "unbounded"
// surface (see LIMIT-01 in limit_test.go); the streaming path must
// emit one row at a time through the json.Encoder without
// materialising the full result slice.
//
// Regression signal: a PR that re-introduces full-slice
// materialisation would 10x+ allocs/op here (one *ent.Network per
// row retained through serialisation), making the regression visible
// on benchstat even if the correctness assertions in LIMIT-01 still
// hold (they only check row count, not memory shape).
func BenchmarkParity_LimitZeroStreaming(b *testing.B) {
	b.ReportAllocs()

	c := testutil.SetupClient(b)
	ctx := b.Context()
	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// 5000 rows. > DefaultLimit (250) by 20x and > MaxLimit (1000) by
	// 5x so the ?limit=0 code path is the only one that could return
	// them all.
	const seedN = 5000
	for i := 1; i <= seedN; i++ {
		if _, err := c.Network.Create().
			SetID(i).
			SetName("StreamBenchNet").
			SetNameFold(unifold.Fold("StreamBenchNet")).
			SetAsn(60000 + i).
			SetStatus("ok").
			SetCreated(t0).
			SetUpdated(t0).
			Save(ctx); err != nil {
			b.Fatalf("seed net id=%d: %v", i, err)
		}
	}

	srv := newTestServer(b, c)

	b.ResetTimer()
	for b.Loop() {
		status, body := httpGet(b, srv, "/api/net?limit=0")
		if status != 200 {
			b.Fatalf("want 200; got %d", status)
		}
		// 5000 rows × ~60B/row JSON shape = ~300 KB. A truncated
		// response would silently inflate ns/op; guard against a
		// body < 100 KB.
		if len(body) < 100_000 {
			b.Fatalf("truncated streaming body: %d bytes (want > 100k)", len(body))
		}
	}
}

// BenchmarkParity_InFiveThousandElements measures the Phase 69 D-05
// json_each(?) single-bind rewrite at the 5001-ID boundary that the
// raw SQLite variable limit (999) would otherwise hit. The IN-01
// test in in_test.go locks correctness; this benchmark locks the
// cost shape so a PR that rolls back to bound-per-element (re-tripping
// the 999 cap) surfaces as a wall-clock spike and/or allocation
// explosion before it reaches prod.
//
// Reuses InFixtures (parity fixture set Plan 72-03) via seedFixtures.
// The InFixtures block holds network rows with IDs 100000..105000
// inclusive (5001 rows); this benchmark queries the exact same range.
func BenchmarkParity_InFiveThousandElements(b *testing.B) {
	b.ReportAllocs()

	c := testutil.SetupClient(b)
	seedFixtures(b, c, parityfix.InFixtures)

	srv := newTestServer(b, c)

	const lo, hi = 100000, 105000 // matches in_test.go IN-01
	ids := make([]string, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		ids = append(ids, strconv.Itoa(id))
	}
	path := "/api/net?id__in=" + strings.Join(ids, ",") + "&limit=0"

	b.ResetTimer()
	for b.Loop() {
		status, body := httpGet(b, srv, path)
		if status != 200 {
			b.Fatalf("want 200; got %d\nbody[:200]=%s",
				status, headBody(body, 200))
		}
		// 5001 rows × ~60B = ~300 KB. Under-sized bodies mean the
		// json_each rewrite silently truncated.
		if len(body) < 100_000 {
			b.Fatalf("truncated __in body: %d bytes (want > 100k)", len(body))
		}
	}
}
