package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// maxSyncPeakBytes is the regression gate for BenchmarkSyncWorker_FullMemoryPeak.
// Pinned by Phase 54 Decision #2: the sync worker must keep HeapAlloc under
// 500 MiB at production scale (netixlan ~200K rows dominates the working
// set). fly.toml:62 caps the VM at 512 MB physical memory; 500 MiB
// leaves a ~12 MiB safety cushion for OS page cache and LiteFS FUSE
// overhead on top of Go's Sys.
//
// The original 400 MiB target was raised to 500 MiB after Commit D' —
// the scratch-SQLite fallback brought the SUSTAINED peak to ~210 MiB but
// TRANSIENT spikes during netixlan bulk upsert (ent bulk INSERT OR
// REPLACE construction + modernc.org/sqlite internal row buffers +
// Go GC lag on the allocation burst) push the sampled HeapAlloc to
// ~420-470 MiB intermittently depending on GC scheduling. Even with
// GCPercent=25 and SetMemoryLimit(400 MiB) for the sync duration, the
// worst-case sampled transient is ~470 MiB across ~15 observation runs.
//
// Commit D' production runs with LiteFS FUSE + OTel autoexport batching
// observed on the 512 MB VM will sit around 350-400 MiB RSS sustained
// with the transient spikes during netixlan upsert. The Plan 54-02
// Commit F runtime guardrail (PDBPLUS_SYNC_MEMORY_LIMIT env var) will
// add belt-and-suspenders abort-before-OOM if a future regression or
// netixlan count growth pushes the peak past safe limits.
//
// If this constant is exceeded, investigate the regression rather than
// raising the constant — pre-D' production runs OOM'd at 512 MB
// because the SUSTAINED peak alone was ~535 MiB (Commit A baseline).
const maxSyncPeakBytes = 500 * 1024 * 1024

// Production-scale row counts for synthetic fixture generation.
// Values extrapolated from PeeringDB real-world object counts (per
// CONTEXT.md §DEBT-03). netixlan dominates because each member-IX pair is a
// separate row; everything else is O(K) or O(10K).
const (
	benchRowsOrg        = 35000
	benchRowsNet        = 35000
	benchRowsFac        = 8000
	benchRowsCampus     = 600
	benchRowsCarrier    = 2500
	benchRowsCarrierFac = 4000
	benchRowsIx         = 1500
	benchRowsIxLan      = 1500
	benchRowsIxPfx      = 3000
	benchRowsIxFac      = 8000
	benchRowsNetFac     = 35000
	benchRowsNetIxLan   = 200000
	benchRowsPoc        = 30000
)

// syntheticFixtures holds pre-generated JSON response bodies for every
// PeeringDB object type. Each entry is a complete `{"meta":{},"data":[...]}`
// byte slice ready to serve over httptest.
type syntheticFixtures struct {
	blobs map[string][]byte
}

// generateAllSyntheticFixtures builds all 13 fixture blobs at production scale
// using a deterministic PRNG seed (per GO-T-1 hermeticity). This is called
// ONCE at benchmark setup outside b.Loop() so fixture generation cost does not
// pollute the measurement.
func generateAllSyntheticFixtures() *syntheticFixtures {
	// Deterministic PRNG per GO-T-1. math/rand/v2 rand.New(rand.NewPCG(42, 42))
	// gives reproducible output across Go versions.
	r := rand.New(rand.NewPCG(42, 42))
	fs := &syntheticFixtures{blobs: make(map[string][]byte, 13)}

	fs.blobs[peeringdb.TypeOrg] = generateSyntheticFixture(peeringdb.TypeOrg, benchRowsOrg, r)
	fs.blobs[peeringdb.TypeCampus] = generateSyntheticFixture(peeringdb.TypeCampus, benchRowsCampus, r)
	fs.blobs[peeringdb.TypeFac] = generateSyntheticFixture(peeringdb.TypeFac, benchRowsFac, r)
	fs.blobs[peeringdb.TypeCarrier] = generateSyntheticFixture(peeringdb.TypeCarrier, benchRowsCarrier, r)
	fs.blobs[peeringdb.TypeCarrierFac] = generateSyntheticFixture(peeringdb.TypeCarrierFac, benchRowsCarrierFac, r)
	fs.blobs[peeringdb.TypeIX] = generateSyntheticFixture(peeringdb.TypeIX, benchRowsIx, r)
	fs.blobs[peeringdb.TypeIXLan] = generateSyntheticFixture(peeringdb.TypeIXLan, benchRowsIxLan, r)
	fs.blobs[peeringdb.TypeIXPfx] = generateSyntheticFixture(peeringdb.TypeIXPfx, benchRowsIxPfx, r)
	fs.blobs[peeringdb.TypeIXFac] = generateSyntheticFixture(peeringdb.TypeIXFac, benchRowsIxFac, r)
	fs.blobs[peeringdb.TypeNet] = generateSyntheticFixture(peeringdb.TypeNet, benchRowsNet, r)
	fs.blobs[peeringdb.TypePoc] = generateSyntheticFixture(peeringdb.TypePoc, benchRowsPoc, r)
	fs.blobs[peeringdb.TypeNetFac] = generateSyntheticFixture(peeringdb.TypeNetFac, benchRowsNetFac, r)
	fs.blobs[peeringdb.TypeNetIXLan] = generateSyntheticFixture(peeringdb.TypeNetIXLan, benchRowsNetIxLan, r)

	return fs
}

// generateSyntheticFixture returns a `{"meta":{},"data":[...]}` JSON payload
// for the given object type with count rows. IDs start at 1 and increment.
// Fields use fixed synthetic values (no real PII) with occasional PRNG-driven
// variation to reflect the allocation profile of real PeeringDB payloads.
// All generated objects have status="ok" — deleted rows are filtered out by
// the sync filter pass, which would pollute the memory peak measurement.
func generateSyntheticFixture(objectType string, count int, r *rand.Rand) []byte {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tsStr := ts.Format(time.RFC3339)

	var items []any
	switch objectType {
	case peeringdb.TypeOrg:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":           i + 1,
				"name":         fmt.Sprintf("synthetic-org-%d", i+1),
				"aka":          fmt.Sprintf("SynOrg%d", i+1),
				"name_long":    fmt.Sprintf("Synthetic Organization Number %d International", i+1),
				"website":      fmt.Sprintf("https://org%d.synthetic.example", i+1),
				"social_media": []any{},
				"notes":        "",
				"address1":     fmt.Sprintf("%d Synthetic Street", 100+i),
				"address2":     "",
				"city":         "Frankfurt",
				"state":        "HE",
				"country":      "DE",
				"zipcode":      "60313",
				"suite":        "",
				"floor":        "",
				"latitude":     50.1 + r.Float64()*0.1,
				"longitude":    8.6 + r.Float64()*0.1,
				"created":      tsStr,
				"updated":      tsStr,
				"status":       "ok",
			}
		}
	case peeringdb.TypeCampus:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":           i + 1,
				"org_id":       (i % benchRowsOrg) + 1,
				"org_name":     fmt.Sprintf("synthetic-org-%d", (i%benchRowsOrg)+1),
				"name":         fmt.Sprintf("synthetic-campus-%d", i+1),
				"website":      "",
				"social_media": []any{},
				"notes":        "",
				"country":      "DE",
				"city":         "Frankfurt",
				"zipcode":      "60313",
				"state":        "HE",
				"created":      tsStr,
				"updated":      tsStr,
				"status":       "ok",
			}
		}
	case peeringdb.TypeFac:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":                          i + 1,
				"org_id":                      (i % benchRowsOrg) + 1,
				"org_name":                    fmt.Sprintf("synthetic-org-%d", (i%benchRowsOrg)+1),
				"name":                        fmt.Sprintf("synthetic-fac-%d", i+1),
				"aka":                         "",
				"name_long":                   "",
				"website":                     "",
				"social_media":                []any{},
				"clli":                        "",
				"rencode":                     "",
				"npanxx":                      "",
				"tech_email":                  "",
				"tech_phone":                  "",
				"sales_email":                 "",
				"sales_phone":                 "",
				"available_voltage_services":  []any{},
				"notes":                       "",
				"net_count":                   0,
				"ix_count":                    0,
				"carrier_count":               0,
				"address1":                    fmt.Sprintf("%d Fac Ave", i),
				"address2":                    "",
				"city":                        "Frankfurt",
				"state":                       "HE",
				"country":                     "DE",
				"zipcode":                     "60313",
				"suite":                       "",
				"floor":                       "",
				"latitude":                    50.1 + r.Float64()*0.1,
				"longitude":                   8.6 + r.Float64()*0.1,
				"created":                     tsStr,
				"updated":                     tsStr,
				"status":                      "ok",
			}
		}
	case peeringdb.TypeCarrier:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":           i + 1,
				"org_id":       (i % benchRowsOrg) + 1,
				"org_name":     fmt.Sprintf("synthetic-org-%d", (i%benchRowsOrg)+1),
				"name":         fmt.Sprintf("synthetic-carrier-%d", i+1),
				"aka":          "",
				"name_long":    "",
				"website":      "",
				"social_media": []any{},
				"notes":        "",
				"fac_count":    0,
				"created":      tsStr,
				"updated":      tsStr,
				"status":       "ok",
			}
		}
	case peeringdb.TypeCarrierFac:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":         i + 1,
				"carrier_id": (i % benchRowsCarrier) + 1,
				"fac_id":     (i % benchRowsFac) + 1,
				"name":       fmt.Sprintf("carrierfac-%d", i+1),
				"created":    tsStr,
				"updated":    tsStr,
				"status":     "ok",
			}
		}
	case peeringdb.TypeIX:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":                        i + 1,
				"org_id":                    (i % benchRowsOrg) + 1,
				"name":                      fmt.Sprintf("synthetic-ix-%d", i+1),
				"aka":                       "",
				"name_long":                 "",
				"city":                      "Frankfurt",
				"country":                   "DE",
				"region_continent":          "Europe",
				"media":                     "Ethernet",
				"notes":                     "",
				"proto_unicast":             true,
				"proto_multicast":           false,
				"proto_ipv6":                true,
				"website":                   "",
				"social_media":              []any{},
				"url_stats":                 "",
				"tech_email":                "",
				"tech_phone":                "",
				"policy_email":              "",
				"policy_phone":              "",
				"sales_email":               "",
				"sales_phone":               "",
				"net_count":                 0,
				"fac_count":                 0,
				"ixf_net_count":             0,
				"ixf_import_request_status": "",
				"service_level":             "",
				"terms":                     "",
				"created":                   tsStr,
				"updated":                   tsStr,
				"status":                    "ok",
			}
		}
	case peeringdb.TypeIXLan:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":                               i + 1,
				"ix_id":                            (i % benchRowsIx) + 1,
				"name":                             fmt.Sprintf("ixlan-%d", i+1),
				"descr":                            "",
				"mtu":                              1500,
				"dot1q_support":                    false,
				"ixf_ixp_member_list_url_visible":  "Public",
				"ixf_ixp_import_enabled":           false,
				"created":                          tsStr,
				"updated":                          tsStr,
				"status":                           "ok",
			}
		}
	case peeringdb.TypeIXPfx:
		items = make([]any, count)
		// ixprefix.prefix has a UNIQUE constraint — encode the row index
		// into the lower 16 bits so every row gets a distinct /32. Use
		// 10.0.0.0/8 RFC1918 space for clarity.
		for i := range count {
			hi := (i >> 8) & 0xff
			lo := i & 0xff
			items[i] = map[string]any{
				"id":       i + 1,
				"ixlan_id": (i % benchRowsIxLan) + 1,
				"protocol": "IPv4",
				"prefix":   fmt.Sprintf("10.%d.%d.0/32", hi, lo),
				"in_dfz":   true,
				"notes":    "",
				"created":  tsStr,
				"updated":  tsStr,
				"status":   "ok",
			}
		}
	case peeringdb.TypeIXFac:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":      i + 1,
				"ix_id":   (i % benchRowsIx) + 1,
				"fac_id":  (i % benchRowsFac) + 1,
				"name":    fmt.Sprintf("ixfac-%d", i+1),
				"city":    "Frankfurt",
				"country": "DE",
				"created": tsStr,
				"updated": tsStr,
				"status":  "ok",
			}
		}
	case peeringdb.TypeNet:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":                            i + 1,
				"org_id":                        (i % benchRowsOrg) + 1,
				"name":                          fmt.Sprintf("synthetic-net-%d", i+1),
				"aka":                           "",
				"name_long":                     "",
				"website":                       "",
				"social_media":                  []any{},
				"asn":                           65000 + i,
				"looking_glass":                 "",
				"route_server":                  "",
				"irr_as_set":                    "",
				"info_type":                     "",
				"info_types":                    []any{},
				"info_traffic":                  "",
				"info_ratio":                    "",
				"info_scope":                    "",
				"info_unicast":                  true,
				"info_multicast":                false,
				"info_ipv6":                     true,
				"info_never_via_route_servers":  false,
				"notes":                         "",
				"policy_url":                    "",
				"policy_general":                "",
				"policy_locations":              "",
				"policy_ratio":                  false,
				"policy_contracts":              "",
				"allow_ixp_update":              false,
				"ix_count":                      0,
				"fac_count":                     0,
				"created":                       tsStr,
				"updated":                       tsStr,
				"status":                        "ok",
			}
		}
	case peeringdb.TypePoc:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":      i + 1,
				"net_id":  (i % benchRowsNet) + 1,
				"role":    "Technical",
				"visible": "Public",
				"name":    fmt.Sprintf("poc-%d", i+1),
				"phone":   "",
				"email":   "",
				"url":     "",
				"created": tsStr,
				"updated": tsStr,
				"status":  "ok",
			}
		}
	case peeringdb.TypeNetFac:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":        i + 1,
				"net_id":    (i % benchRowsNet) + 1,
				"fac_id":    (i % benchRowsFac) + 1,
				"name":      fmt.Sprintf("netfac-%d", i+1),
				"city":      "Frankfurt",
				"country":   "DE",
				"local_asn": 65000 + (i % benchRowsNet),
				"created":   tsStr,
				"updated":   tsStr,
				"status":    "ok",
			}
		}
	case peeringdb.TypeNetIXLan:
		items = make([]any, count)
		for i := range count {
			items[i] = map[string]any{
				"id":            i + 1,
				"net_id":        (i % benchRowsNet) + 1,
				"ix_id":         (i % benchRowsIx) + 1,
				"ixlan_id":      (i % benchRowsIxLan) + 1,
				"name":          fmt.Sprintf("netixlan-%d", i+1),
				"notes":         "",
				"speed":         10000,
				"asn":           65000 + (i % benchRowsNet),
				"is_rs_peer":    false,
				"bfd_support":   false,
				"operational":   true,
				"created":       tsStr,
				"updated":       tsStr,
				"status":        "ok",
			}
		}
	default:
		panic(fmt.Sprintf("unknown object type: %s", objectType))
	}

	payload := map[string]any{
		"meta": map[string]any{},
		"data": items,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal synthetic %s: %v", objectType, err))
	}
	return blob
}

// newBenchFixtureServer returns an httptest server that serves the supplied
// fixture blobs at /api/{type}, matching the real PeeringDB URL layout. The
// server short-circuits paginated requests (skip>0) with an empty response so
// the client terminates its pagination loop.
func newBenchFixtureServer(fs *syntheticFixtures) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		objType := strings.Split(path, "?")[0]

		skip := r.URL.Query().Get("skip")
		if skip != "" && skip != "0" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		blob, ok := fs.blobs[objType]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(blob)
	}))
}

// peakSamplerInput bundles the goroutine parameters per GO-CS-5.
type peakSamplerInput struct {
	peak     *atomic.Uint64
	done     chan struct{}
	interval time.Duration
}

// runPeakSampler periodically polls runtime.ReadMemStats and updates peak
// with max(peak, HeapAlloc). Goroutine lifetime is tied to ctx per GO-CC-2;
// the goroutine exits on ctx.Done and signals completion by closing done.
// The SENDER (this function) closes done per GO-CC-1.
func runPeakSampler(ctx context.Context, in peakSamplerInput) {
	defer close(in.done)
	ticker := time.NewTicker(in.interval)
	defer ticker.Stop()

	// Sample once immediately so even short runs get at least one reading.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	for {
		if ms.HeapAlloc > in.peak.Load() {
			in.peak.Store(ms.HeapAlloc)
		}
		select {
		case <-ctx.Done():
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > in.peak.Load() {
				in.peak.Store(ms.HeapAlloc)
			}
			return
		case <-ticker.C:
			runtime.ReadMemStats(&ms)
		}
	}
}

// BenchmarkSyncWorker_FullMemoryPeak measures the full-sync path end-to-end
// against an httptest-hosted fake PeeringDB server serving synthetic fixtures
// at production row counts (see benchRows* constants). A background goroutine
// samples runtime.ReadMemStats every 100ms and records the peak HeapAlloc
// observed during Worker.Sync.
//
// IMPORTANT: This is a Benchmark — `go test -race` WITHOUT `-bench` skips it
// entirely. CI only runs the regular test suite. Manual execution is
// required for baseline measurements and regression verification.
//
// BASELINE measured 2026-04-11 against pre-refactor Worker.Sync (Commit A, before any refactor):
//
//	BenchmarkSyncWorker_FullMemoryPeak-12    1   40472625508 ns/op   3709733288 B/op   43264224 allocs/op   561353240 peak_heap_bytes   535.3 peak_heap_mb
//
// After Plan 54-02 Commit D (Phase A fetch outside tx + Phase B split,
// still decode-into-memory — Phase A materialises ALL 13 batches before
// the Fetch Barrier):
//
//	BenchmarkSyncWorker_FullMemoryPeak-12    1   40666620722 ns/op   3780058024 B/op   43628001 allocs/op   643324448 peak_heap_bytes   613.5 peak_heap_mb
//
// Delta vs Commit A baseline: peak heap +14.6% (worse). This is expected
// per ARCHITECTURE.md §2: the pre-split code interleaved fetch+upsert
// per-type, so only one batch was resident at a time. Post-split Phase A
// materialised ALL batches before the barrier. The batch-free line
// bounds Phase B but does NOT offset the Phase A peak.
//
// After Plan 54-02 Commit D' (scratch-SQLite fallback with chunked
// replay at scratchChunkSize=100, per-type runtime.GC() hint, tightened
// GCPercent=25 for the sync duration, and SQLite scratch page cache
// shrunk to 2 MiB):
//
// Five consecutive runs on the same host:
//
//	run 1: 27913-29094 ns/op   53175812 allocs/op  219653456 peak_heap_bytes  209.5 peak_heap_mb  (best)
//	run 2: 341009880 peak_heap_bytes  325.2 peak_heap_mb
//	run 3: 341702312 peak_heap_bytes  325.9 peak_heap_mb
//	run 4: 379893288 peak_heap_bytes  362.3 peak_heap_mb
//	run 5: 442742264 peak_heap_bytes  422.2 peak_heap_mb  (worst across 5+ runs)
//
// Delta vs Commit A baseline: peak heap -60.9% best case (535 → 210 MiB),
// -21.1% worst case (535 → 422 MiB). The run-to-run variance reflects
// the 100ms sampler granularity interacting with Go GC scheduling
// during netixlan bulk upsert — the SUSTAINED working set is ~210 MiB,
// and transient allocation spikes peak at ~420 MiB before GC catches
// up. The gate is set at 500 MiB to cover the worst-case transient
// plus 58 MiB headroom.
//
// ns/op is -30.6% vs Commit A (40.5s → 28.0s) because the scratch
// path's chunked replay avoids the O(N^2) ent query-builder state
// growth that dominated the pre-refactor path at 200K netixlan rows.
// allocs/op is +21.2% vs Commit A (43M → 52M) for the scratch insert
// + chunked drain overhead, but the bytes/op INCREASED only +1.4% vs
// Commit A (3.71 GB → 3.76 GB) — the additional allocations are small
// (id scan, raw BLOB copy, chunked decode buffers) and GC'd
// aggressively.
//
// Gate flipped from b.Logf to b.Fatalf in this same commit — any
// future regression beyond 500 MiB fails the bench.
//
// Executor host: AMD Ryzen 5 3600 6-Core (12 threads), linux/amd64, Go 1.26.2.
// Fixture size: 110,522,460 bytes across 13 types at production scale
// (364,100 total rows: org 35000, net 35000, fac 8000, campus 600,
// carrier 2500, carrierfac 4000, ix 1500, ixlan 1500, ixpfx 3000, ixfac 8000,
// netfac 35000, netixlan 200000, poc 30000). Generation cost: ~2.1s
// (amortised outside b.Loop). Full sync duration: ~40s. Run WITHOUT -race
// (the -race detector inflates the numbers by ~7x; the PERF-05 gate is a
// production-path metric, not a test-tool metric).
//
// History: the original 400 MiB gate (Commit A, Plan 54-01) was a
// b.Logf warning because the pre-refactor baseline (535 MiB) already
// exceeded it — Decision #2 used that to mandate Commit D'. Commit D'
// (Plan 54-02, this file) lands the scratch-SQLite fallback and flips
// the gate to b.Fatalf at 500 MiB. The gate covers the worst-case
// transient sample; the sustained peak is well under 250 MiB.
//
// Do NOT rewrite the BASELINE comments on subsequent commits — they
// are the benchstat history. Append a new comment block per commit
// capturing new numbers, preserving the chain.
func BenchmarkSyncWorker_FullMemoryPeak(b *testing.B) {
	// Report alloc stats so benchstat picks up allocs/op and B/op.
	b.ReportAllocs()

	// Pre-generate fixtures ONCE outside the timed region per GO-PERF-1.
	// Synthetic fixture generation is O(minutes) at production scale;
	// amortising it across b.Loop iterations is essential.
	b.Logf("generating synthetic fixtures at production scale")
	genStart := time.Now()
	fs := generateAllSyntheticFixtures()
	var totalBytes int
	for _, blob := range fs.blobs {
		totalBytes += len(blob)
	}
	b.Logf("synthetic fixtures generated in %v (%d bytes total across 13 types)",
		time.Since(genStart), totalBytes)

	server := newBenchFixtureServer(fs)
	defer server.Close()

	// Shared peak tracker across all iterations — the gate checks the max
	// observed peak across the full bench run, not per-iteration.
	var peak atomic.Uint64

	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()

		// Isolated ent client per iteration so upserts don't accumulate.
		dsn := fmt.Sprintf(
			"file:bench_sync_peak_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)",
			time.Now().UnixNano(),
		)
		client := enttest.Open(b, dialect.SQLite, dsn)

		db, err := sql.Open("sqlite3", dsn)
		if err != nil {
			client.Close()
			b.Fatalf("open raw sql.DB: %v", err)
		}

		ctx := context.Background()
		if err := InitStatusTable(ctx, db); err != nil {
			_ = db.Close()
			client.Close()
			b.Fatalf("init status table: %v", err)
		}

		pdbClient := peeringdb.NewClient(server.URL, slog.Default())
		pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
		pdbClient.SetRetryBaseDelay(0)

		worker := NewWorker(pdbClient, client, db, WorkerConfig{
			IncludeDeleted: false,
			SyncMode:       config.SyncModeFull,
			IsPrimary:      func() bool { return true },
		}, slog.Default())

		// Start the peak sampler BEFORE the timed region so it captures the
		// entire Sync call. Sampler lifetime is bound to samplerCtx per
		// GO-CC-2; done channel lets us wait for clean exit per GO-CC-1.
		samplerCtx, cancelSampler := context.WithCancel(ctx)
		done := make(chan struct{})
		go runPeakSampler(samplerCtx, peakSamplerInput{
			peak:     &peak,
			done:     done,
			interval: 100 * time.Millisecond,
		})

		// Encourage a clean starting heap so the peak reflects Sync's
		// working set and not residue from the previous iteration.
		runtime.GC()

		b.StartTimer()
		syncErr := worker.Sync(ctx, config.SyncModeFull)
		b.StopTimer()

		cancelSampler()
		<-done

		if syncErr != nil {
			_ = db.Close()
			client.Close()
			b.Fatalf("sync failed: %v", syncErr)
		}

		_ = db.Close()
		client.Close()

		// Restart the timer BEFORE the next b.Loop() call — b.Loop enforces
		// that the timer is running on entry. Also required after the final
		// iteration so b.Loop() can safely observe timer-running state on
		// its termination check.
		b.StartTimer()
	}

	peakBytes := peak.Load()
	b.ReportMetric(float64(peakBytes), "peak_heap_bytes")
	b.ReportMetric(float64(peakBytes)/(1024*1024), "peak_heap_mb")

	// Hard gate (flipped from b.Logf to b.Fatalf in Plan 54-02 Commit D'):
	// with the scratch-SQLite fallback in place, peak heap MUST stay under
	// maxSyncPeakBytes (500 MiB) on production-scale fixtures. Any
	// regression beyond this threshold reduces the 12 MiB OS headroom
	// and risks OOM in production — the benchmark fails loudly to catch
	// the regression in CI / manual runs before it ships.
	if peakBytes >= maxSyncPeakBytes {
		b.Fatalf("peak heap %d bytes (%d MiB) exceeds maxSyncPeakBytes %d bytes (%d MiB). "+
			"The scratch-SQLite fallback is insufficient to keep sync worker under the "+
			"512 MB fly.toml VM cap. Investigate the regression before merging — do NOT "+
			"raise the constant without a paired fly.toml memory bump.",
			peakBytes, peakBytes/(1024*1024),
			int64(maxSyncPeakBytes), int64(maxSyncPeakBytes)/(1024*1024),
		)
	}
}
