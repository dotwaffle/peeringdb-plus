package main

import (
	"context"
	"fmt"
	"io"
	"time"
)

// syncOrder is the loadtest's mirror of internal/sync.canonicalStepOrder.
// The TestSync_OrderingMatchesWorker parity guard fails the build if
// the live worker reorders syncSteps() without an accompanying update
// here — that's the design intent (drift detection).
//
// Held as a flat slice (not a function) so the test can DeepEqual it
// against syncpkg.StepOrder() without any Worker construction.
var syncOrder = []string{
	"org", "campus", "fac", "carrier", "carrierfac",
	"ix", "ixlan", "ixpfx", "ixfac",
	"net", "poc", "netfac", "netixlan",
}

// syncDepths is the set of `?depth=` values the loadtest issues per
// entity type per cycle. depth=0 mirrors the project's own sync (see
// internal/peeringdb/stream.go StreamAll); depth=1 and depth=2 cover
// what real-world PeeringDB API clients commonly request when they
// want eagerly-resolved nested relations (e.g. admin UIs, custom
// reporting tools).
var syncDepths = []int{0, 1, 2}

// buildSyncEndpoints returns a 39-entry pdbcompat sequence (13 types ×
// 3 depths) in syncOrder using URL shapes that mirror real client
// traffic against the mirror:
//
//   - Full mode: /api/<type>?depth=N — bare URL, matches what real
//     PeeringDB clients send (and what internal/peeringdb/stream.go
//     StreamAll sends to upstream). The mirror serves all rows on
//     bare URLs since the DefaultLimit-fix on 2026-04-28 (see
//     internal/pdbcompat/response.go); the Phase 71 response-memory
//     budget is the sole DoS gate.
//   - Incremental mode: /api/<type>?limit=250&skip=0&depth=N&since=M
//     — first page of the production paginated incremental fetch per
//     (type, depth). Mirrors internal/peeringdb/stream.go's
//     paginated branch verbatim.
//
// Issuing all three depths per type matches the diversity of real
// client traffic the mirror serves: depth=0 (the project's own sync,
// raw FK ids only), depth=1 (single-level nested), depth=2 (two
// levels — heavy responses used by admin tooling). All ordered per
// syncOrder so FK dependency parents come before children within
// each depth band.
//
// Note: pdbcompat silently drops `?depth=N` on list endpoints
// (LIMIT-02 documented divergence), so depth=0/1/2 currently produce
// identical bodies. The depth band still has value as a regression
// signal — if/when LIMIT-02 is reverted, the loadtest will surface it
// immediately via response-size drift across the bands.
func buildSyncEndpoints(mode string, since time.Time) []Endpoint {
	out := make([]Endpoint, 0, len(syncOrder)*len(syncDepths))
	for _, depth := range syncDepths {
		for _, t := range syncOrder {
			var path string
			if mode == "incremental" {
				path = fmt.Sprintf("/api/%s?limit=250&skip=0&depth=%d&since=%d",
					t, depth, since.Unix())
			} else {
				path = fmt.Sprintf("/api/%s?depth=%d", t, depth)
			}
			out = append(out, Endpoint{
				Surface:    SurfacePdbCompat,
				EntityType: t,
				Shape:      fmt.Sprintf("sync-%s-d%d", mode, depth),
				Method:     "GET",
				Path:       path,
			})
		}
	}
	return out
}

// runSync issues 39 GETs (13 types × 3 depths) in syncOrder × ascending
// depth against the pdbcompat /api/<short> endpoint, sequentially.
// Honors ctx cancellation between requests.
//
// Returns the first context.Canceled / context.DeadlineExceeded
// observed, otherwise nil. Per-request errors are folded into the
// Result so the cycle continues.
func runSync(ctx context.Context, cfg Config, mode string, since time.Time, rep *Report, out io.Writer) error {
	if mode != "full" && mode != "incremental" {
		return fmt.Errorf("--mode=%q: want full or incremental", mode)
	}
	eps := buildSyncEndpoints(mode, since)
	if cfg.Verbose || mode == "incremental" {
		fmt.Fprintf(out, "sync mode=%s base=%s steps=%d", mode, cfg.Base, len(eps))
		if mode == "incremental" {
			fmt.Fprintf(out, " since=%s (unix=%d)", since.Format(time.RFC3339), since.Unix())
		}
		fmt.Fprintln(out)
	}
	for i, ep := range eps {
		if err := ctx.Err(); err != nil {
			return err
		}
		res := Hit(ctx, cfg.HTTPClient, cfg.Base, cfg.AuthToken, ep)
		rep.Append(res)
		if cfg.Verbose {
			fmt.Fprintf(out, "  [%2d/%d] %-7s %s -> %d (%s, %s)\n",
				i+1, len(eps), ep.Method, ep.Path, res.Status,
				res.Latency.Round(0), humanBytes(res.Bytes))
		}
	}
	return nil
}
