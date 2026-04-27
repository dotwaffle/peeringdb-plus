//go:build loadtest

package main

import (
	"context"
	"errors"
	"io"
	"time"
)

// runSync is implemented in Task 2. Task 1 keeps a stub so the
// dispatch in main.go compiles and the --help wiring is exercised.
func runSync(ctx context.Context, cfg Config, mode string, since time.Time, rep *Report, out io.Writer) error {
	_ = ctx
	_ = cfg
	_ = mode
	_ = since
	_ = rep
	_ = out
	return errors.New("sync mode: not yet implemented (Task 2)")
}
