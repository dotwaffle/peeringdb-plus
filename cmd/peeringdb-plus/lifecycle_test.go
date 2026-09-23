package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestAwaitShutdown verifies the exit-code contract: signal-driven shutdown
// returns 0, server failure returns 1. Returning (instead of os.Exit in the
// serve goroutine) is what lets main's deferred OTel flush run on failure.
func TestAwaitShutdown(t *testing.T) {
	t.Parallel()

	t.Run("signal returns zero", func(t *testing.T) {
		t.Parallel()
		sigChan := make(chan os.Signal, 1)
		sigChan <- syscall.SIGTERM
		got := awaitShutdown(awaitShutdownInput{
			SigChan:  sigChan,
			ServeErr: make(chan error, 1),
			Logger:   discardLogger(),
		})
		if got != 0 {
			t.Errorf("exit code = %d, want 0", got)
		}
	})

	t.Run("serve error returns one", func(t *testing.T) {
		t.Parallel()
		serveErr := make(chan error, 1)
		serveErr <- errors.New("listen tcp :8080: address already in use")
		got := awaitShutdown(awaitShutdownInput{
			SigChan:  make(chan os.Signal, 1),
			ServeErr: serveErr,
			Logger:   discardLogger(),
		})
		if got != 1 {
			t.Errorf("exit code = %d, want 1", got)
		}
	})
}

// TestRunSyncWithDemotionMonitor_CancelsOnDemotion verifies the on-demand
// sync path inherits the scheduler's demotion semantics: when IsPrimary
// flips false mid-cycle, the sync context is cancelled so SyncWithRetry
// (and its retry ladder) aborts instead of burning upstream quota.
func TestRunSyncWithDemotionMonitor_CancelsOnDemotion(t *testing.T) {
	t.Parallel()

	var primary atomic.Bool
	primary.Store(true)

	syncStarted := make(chan struct{})
	syncCancelled := make(chan struct{})

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		runSyncWithDemotionMonitor(context.Background(), config.SyncModeIncremental, monitoredSyncInput{
			IsPrimary:    primary.Load,
			Logger:       discardLogger(),
			PollInterval: time.Millisecond,
			Sync: func(ctx context.Context, _ config.SyncMode) error {
				close(syncStarted)
				select {
				case <-ctx.Done():
					close(syncCancelled)
					return ctx.Err()
				case <-time.After(5 * time.Second):
					return errors.New("sync ctx never cancelled after demotion")
				}
			},
		})
	}()

	<-syncStarted
	primary.Store(false) // demote mid-cycle

	select {
	case <-syncCancelled:
		// sync ctx cancelled by the demotion monitor — expected
	case <-time.After(5 * time.Second):
		t.Fatal("sync context was not cancelled after demotion")
	}

	select {
	case <-finished:
		// wrapper returned, monitor goroutine joined
	case <-time.After(5 * time.Second):
		t.Fatal("runSyncWithDemotionMonitor did not return after sync aborted")
	}
}

// TestRunSyncWithDemotionMonitor_PrimaryCompletes verifies the happy path:
// a node that stays primary runs the sync to completion with an uncancelled
// context, and the monitor goroutine exits cleanly.
func TestRunSyncWithDemotionMonitor_PrimaryCompletes(t *testing.T) {
	t.Parallel()

	// IsPrimary closes polled on call wantPolls. The monitor calls
	// IsPrimary again only after it has acted on the previous answer, so
	// at that point it has acted on wantPolls-1 "still primary" answers.
	// A monitor that cancels the cycle of a primary within those polls has
	// done so before Sync reads ctx.Err(). Eleven calls cover the ten 1ms
	// polls that the former 10ms sleep allowed at best.
	const wantPolls = 11
	var polls atomic.Int32
	polled := make(chan struct{})
	isPrimary := func() bool {
		if polls.Add(1) == wantPolls {
			close(polled)
		}
		return true
	}

	finished := make(chan struct{})
	var ctxErr error
	go func() {
		defer close(finished)
		runSyncWithDemotionMonitor(context.Background(), config.SyncModeFull, monitoredSyncInput{
			IsPrimary:    isPrimary,
			Logger:       discardLogger(),
			PollInterval: time.Millisecond,
			Sync: func(ctx context.Context, _ config.SyncMode) error {
				select {
				case <-polled:
				case <-ctx.Done():
				case <-time.After(5 * time.Second):
				}
				ctxErr = ctx.Err()
				return nil
			},
		})
	}()

	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("runSyncWithDemotionMonitor did not return")
	}
	if ctxErr != nil {
		t.Errorf("sync ctx err = %v, want nil (still primary)", ctxErr)
	}
	if n := polls.Load(); n < wantPolls {
		t.Errorf("monitor polled IsPrimary %d times during sync, want >= %d", n, wantPolls)
	}
}
