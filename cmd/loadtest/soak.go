//go:build loadtest

package main

import (
	"context"
	"errors"
	"time"
)

// runSoak is implemented in Task 3. Task 1 keeps a stub so the
// dispatch in main.go compiles and the --help wiring is exercised.
func runSoak(ctx context.Context, cfg Config, duration time.Duration, concurrency int, qps float64, eps []Endpoint, rep *Report) error {
	_ = ctx
	_ = cfg
	_ = duration
	_ = concurrency
	_ = qps
	_ = eps
	_ = rep
	return errors.New("soak mode: not yet implemented (Task 3)")
}
