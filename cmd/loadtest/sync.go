package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
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

// buildSyncEndpoints returns a 13-entry pdbcompat sequence in syncOrder.
// Full mode mirrors internal/peeringdb/client.go FetchRawPage's URL
// shape exactly: ?limit=250&skip=0&depth=0. Incremental mode appends
// &since=<unix-seconds>.
//
// The loadtest issues one page per type — full pagination is the live
// worker's responsibility on the server side; the operator goal here
// is endpoint exhaustion + dashboard warmup, not exhaustive fetch.
func buildSyncEndpoints(mode string, since time.Time) []Endpoint {
	out := make([]Endpoint, 0, len(syncOrder))
	for _, t := range syncOrder {
		path := fmt.Sprintf("/api/%s?limit=250&skip=0&depth=0", t)
		if mode == "incremental" {
			path += "&since=" + strconv.FormatInt(since.Unix(), 10)
		}
		out = append(out, Endpoint{
			Surface:    SurfacePdbCompat,
			EntityType: t,
			Shape:      "sync-" + mode,
			Method:     "GET",
			Path:       path,
		})
	}
	return out
}

// runSync issues exactly 13 GETs in syncOrder (FK dependency order)
// against the pdbcompat /api/<short> endpoint, sequentially. Honors
// ctx cancellation between requests.
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
			fmt.Fprintf(out, "  [%2d/%d] %-7s %s -> %d (%s)\n",
				i+1, len(eps), ep.Method, ep.Path, res.Status, res.Latency.Round(0))
		}
	}
	return nil
}
