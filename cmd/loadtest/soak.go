//go:build loadtest

package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

// runSoak drives sustained mixed-surface load against cfg.Base for
// duration, capping the global RPS via golang.org/x/time/rate at qps
// req/s with burst 1.
//
// concurrency goroutines share the limiter; each loops:
//  1. limiter.Wait(ctx) — blocks until a token is available
//  2. pick a random Endpoint from eps (math/rand/v2)
//  3. Hit() → append Result to the shared Report
//  4. terminate when ctx is Done OR the soak deadline fires
//
// Defaults (set by main.go flags): duration=30s, concurrency=4, qps=5.
// These are conservative for shared-cpu-1x replicas; bumping qps
// above ~50 against a Fly app risks tripping middleware rate-limiting
// (documented in cmd/loadtest/README.md).
func runSoak(ctx context.Context, cfg Config, duration time.Duration, concurrency int, qps float64, eps []Endpoint, rep *Report) error {
	if duration <= 0 {
		return errors.New("--duration must be > 0")
	}
	if concurrency < 1 {
		return errors.New("--concurrency must be >= 1")
	}
	if qps <= 0 {
		return errors.New("--qps must be > 0")
	}
	if len(eps) == 0 {
		return errors.New("soak: empty endpoint registry")
	}

	soakCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// rate.Every(time.Second/qps) with burst=1 matches the project's
	// internal/peeringdb/client.go rate-limiter pattern. burst=1 keeps
	// the cap tight; burst>1 would let bursts of N workers fire
	// simultaneously, breaking the global QPS guarantee.
	interval := time.Duration(float64(time.Second) / qps)
	limiter := rate.NewLimiter(rate.Every(interval), 1)

	g, gctx := errgroup.WithContext(soakCtx)
	for w := range concurrency {
		_ = w
		g.Go(func() error {
			for {
				if err := limiter.Wait(gctx); err != nil {
					// rate.Limiter.Wait returns context.Canceled /
					// context.DeadlineExceeded directly when the
					// ctx fires, but it also returns its own
					// "would exceed context deadline" sentinel
					// (fmt.Errorf, not wrapped) when the next
					// token would arrive after the deadline. Both
					// are graceful termination, not soak failures.
					if isRateLimiterDone(gctx, err) {
						return nil
					}
					return err
				}
				ep := eps[rand.IntN(len(eps))] //nolint:gosec // load-test endpoint shuffle; not security-relevant
				res := Hit(gctx, cfg.HTTPClient, cfg.Base, cfg.AuthToken, ep)
				rep.Append(res)
			}
		})
	}

	if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("soak workers: %w", err)
	}
	// Honor outer-ctx cancellation explicitly so the caller's
	// signal-handler path returns the right error.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// isRateLimiterDone reports whether err from limiter.Wait represents
// graceful termination — either ctx cancelled / deadline exceeded,
// or rate.Limiter's "would exceed context deadline" sentinel that
// fires when no more tokens will arrive before the soak window ends.
// Both mean "no more work", not "broken limiter".
func isRateLimiterDone(_ context.Context, err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "would exceed context deadline")
}
