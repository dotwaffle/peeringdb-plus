package main

import (
	"context"
	"fmt"
	"io"
)

// runEndpoints executes one Hit per registry entry, sequentially, and
// records every Result on rep. Sequential is intentional: sweep mode
// is a once-per-run inventory pass for dashboards and capacity
// validation, NOT a stress test (soak handles concurrency).
//
// Returns the first context.Canceled / context.DeadlineExceeded
// observed, otherwise nil. Per-endpoint network errors are folded
// into Result.Err and do not abort the sweep.
func runEndpoints(ctx context.Context, cfg Config, eps []Endpoint, rep *Report, out io.Writer) error {
	if cfg.Verbose {
		fmt.Fprintf(out, "endpoints sweep: %d endpoints against %s\n", len(eps), cfg.Base)
	}
	for i, ep := range eps {
		if err := ctx.Err(); err != nil {
			return err
		}
		res := Hit(ctx, cfg.HTTPClient, cfg.Base, cfg.AuthToken, ep)
		rep.Append(res)
		if cfg.Verbose {
			fmt.Fprintf(out, "  [%3d/%d] %-10s %-7s %s -> %d (%s, %s)\n",
				i+1, len(eps), ep.Surface, ep.Method, ep.Path, res.Status,
				res.Latency.Round(0), humanBytes(res.Bytes))
		}
	}
	return nil
}
