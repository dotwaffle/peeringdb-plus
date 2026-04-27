package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// TestOnSyncComplete_Signature locks the WorkerConfig.OnSyncComplete field
// type to `func(ctx context.Context, syncTime time.Time)`.
//
// Quick task 260427-ojm flipped the callback shape FROM
// `func(map[string]int, time.Time)` (per-cycle upsert deltas — the wrong
// value to feed into the gauge cache) TO a context-only signature so the
// callback can run a fresh `pdbsync.InitialObjectCounts(ctx, client)` and
// store live row counts. Compile failure on this test is the desired
// canary for any future signature drift — the cmd/peeringdb-plus callback
// in main.go is the SOLE consumer today, but anyone wiring a second
// consumer must agree on the new shape.
//
// The reflect-based check guards against a maintainer "fixing" the
// signature mismatch by adding `counts map[string]int` back as a third
// arg without thinking about why it was removed.
func TestOnSyncComplete_Signature(t *testing.T) {
	t.Parallel()

	wantSig := reflect.TypeOf(func(ctx context.Context, syncTime time.Time) {})

	cfgType := reflect.TypeOf(pdbsync.WorkerConfig{})
	field, ok := cfgType.FieldByName("OnSyncComplete")
	if !ok {
		t.Fatal("WorkerConfig.OnSyncComplete field missing — quick task 260427-ojm contract violated")
	}

	if field.Type != wantSig {
		t.Fatalf(
			"WorkerConfig.OnSyncComplete signature drift:\n  got:  %s\n  want: %s\n"+
				"The callback runs InitialObjectCounts(ctx, client) — it does NOT consume\n"+
				"per-cycle upsert deltas. See quick task 260427-ojm.",
			field.Type, wantSig,
		)
	}
}
