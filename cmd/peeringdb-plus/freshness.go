package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// initialObjectCountsTimeout bounds the startup seed of the object-count
// gauge cache (see seedObjectCountCache).
const initialObjectCountsTimeout = 5 * time.Second

// freshnessReadTimeout bounds the sync_status read behind the freshness
// gauge so a slow or locked SQLite read cannot stall metric collection.
const freshnessReadTimeout = 2 * time.Second

// freshnessFromDB reads the last successful sync's completion time from the
// local database. Newer running or failed attempts do not reset data age.
// A read error or no successful sync produces no gauge observation.
func freshnessFromDB(ctx context.Context, db *sql.DB) (time.Time, bool) {
	ctx, cancel := context.WithTimeout(ctx, freshnessReadTimeout)
	defer cancel()
	status, err := pdbsync.GetLastSuccessfulStatus(ctx, db)
	if err != nil || status == nil {
		return time.Time{}, false
	}
	return status.LastSyncAt, true
}

func seedObjectCountCache(ctx context.Context, db *sql.DB, logger *slog.Logger) (map[string]int64, error) {
	seedCtx, cancel := context.WithTimeout(ctx, initialObjectCountsTimeout)
	defer cancel()

	counts, err := pdbsync.InitialObjectCounts(seedCtx, db)
	if err == nil {
		return counts, nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(seedCtx.Err(), context.DeadlineExceeded) {
		logger.Warn("initial object count seed timed out; continuing with zeroed gauges until first refresh",
			slog.Duration("timeout", initialObjectCountsTimeout))
		return zeroedObjectCounts(), nil
	}
	return nil, err
}

func zeroedObjectCounts() map[string]int64 {
	out := make(map[string]int64, len(pdbsync.StepOrder()))
	for _, t := range pdbsync.StepOrder() {
		out[t] = 0
	}
	return out
}
