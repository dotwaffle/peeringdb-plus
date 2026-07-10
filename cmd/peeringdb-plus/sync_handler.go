package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// demotionMonitorPollInterval is how often the on-demand sync demotion
// monitor re-checks IsPrimary. Mirrors the 1s poll in the scheduler's
// internal/sync runSyncCycle wrapper.
const demotionMonitorPollInterval = 1 * time.Second

// monitoredSyncInput holds dependencies for runSyncWithDemotionMonitor.
type monitoredSyncInput struct {
	// IsPrimary is the live LiteFS primary-detection function.
	IsPrimary func() bool
	Logger    *slog.Logger
	// Sync is the underlying sync invocation (Worker.SyncWithRetry).
	Sync func(ctx context.Context, mode config.SyncMode) error
	// PollInterval overrides demotionMonitorPollInterval when > 0 (tests).
	PollInterval time.Duration
}

// runSyncWithDemotionMonitor wraps an on-demand sync in the same demotion
// monitor the scheduler applies via internal/sync's (unexported)
// runSyncCycle: a goroutine polls IsPrimary and cancels the cycle context
// the moment the node loses the LiteFS lease, aborting SyncWithRetry (and
// its retry ladder) early. Without it, a node demoted mid-cycle would keep
// burning upstream PeeringDB quota — concurrently with the new primary's
// own sync — until Phase B fails against the read-only replica mount.
func runSyncWithDemotionMonitor(ctx context.Context, mode config.SyncMode, in monitoredSyncInput) {
	cycleCtx, cycleCancel := context.WithCancel(ctx)
	defer cycleCancel()

	poll := in.PollInterval
	if poll <= 0 {
		poll = demotionMonitorPollInterval
	}

	// Monitor goroutine: polls IsPrimary and cancels on demotion.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(poll)
		defer ticker.Stop()
		for {
			select {
			case <-cycleCtx.Done():
				return
			case <-ticker.C:
				if !in.IsPrimary() {
					in.Logger.LogAttrs(cycleCtx, slog.LevelWarn,
						"demoted during on-demand sync, aborting cycle")
					cycleCancel()
					return
				}
			}
		}
	}()

	if err := in.Sync(cycleCtx, mode); err != nil {
		in.Logger.LogAttrs(ctx, slog.LevelError, "on-demand sync failed",
			slog.Any("error", err))
	}
	cycleCancel() // ensure monitor goroutine exits
	<-done        // wait for clean exit (no goroutine leak)
}

// SyncHandlerInput holds dependencies for the sync handler.
type SyncHandlerInput struct {
	IsPrimaryFn func() bool
	SyncToken   string
	DefaultMode config.SyncMode
	SyncFn      func(ctx context.Context, mode config.SyncMode)
	// SyncRunning reports whether a cycle is already in flight, so the
	// handler can answer 409 instead of a misleading 202 whose trigger
	// the worker's CAS guard would silently drop (the operator's
	// ?mode=full escape hatch must not no-op invisibly). Best-effort:
	// a race with a starting cycle degrades to the logged-drop path.
	SyncRunning func() bool
}

// newSyncHandler creates the POST /sync handler with fly-replay write forwarding.
// On Fly.io replicas (FLY_REGION set, not primary), it returns a fly-replay header
// routing to PRIMARY_REGION. In local dev (FLY_REGION empty, not primary), it
// returns 503 since there is no Fly proxy to replay the request.
func newSyncHandler(appCtx context.Context, in SyncHandlerInput) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !in.IsPrimaryFn() {
			// On Fly.io, replay to primary region.
			if flyRegion := os.Getenv("FLY_REGION"); flyRegion != "" {
				primaryRegion := os.Getenv("PRIMARY_REGION")
				w.Header().Set("fly-replay", "region="+primaryRegion)
				w.WriteHeader(http.StatusTemporaryRedirect)
				return
			}
			// Not on Fly.io (local dev) -- non-primary cannot handle sync.
			http.Error(w, "not primary", http.StatusServiceUnavailable)
			return
		}
		got := r.Header.Get("X-Sync-Token")
		if in.SyncToken == "" ||
			len(got) != len(in.SyncToken) ||
			subtle.ConstantTimeCompare([]byte(got), []byte(in.SyncToken)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		mode := in.DefaultMode
		if qm := r.URL.Query().Get("mode"); qm != "" {
			switch config.SyncMode(qm) {
			case config.SyncModeFull, config.SyncModeIncremental:
				mode = config.SyncMode(qm)
			default:
				http.Error(w, fmt.Sprintf("invalid mode %q: must be full or incremental", qm), http.StatusBadRequest)
				return
			}
		}
		// Use application root ctx, NOT r.Context() -- request context
		// is cancelled when the response is sent, which would kill the sync.
		//
		// A manually-triggered sync is traced by default (you asked for it,
		// you want to see it); ?trace=0 opts out. Scheduled timer syncs are
		// never traced (see internal/otel sampler). The force-trace flag rides
		// the app root ctx so it reaches the worker's root span.
		syncCtx := appCtx
		if r.URL.Query().Get("trace") != "0" {
			syncCtx = pdbsync.WithForceTrace(appCtx)
		}
		if in.SyncRunning != nil && in.SyncRunning() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"status":"conflict","detail":"a sync cycle is already running"}`)
			return
		}
		go in.SyncFn(syncCtx, mode)
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted"}`)
	}
}
