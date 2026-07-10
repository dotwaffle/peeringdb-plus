package main

import (
	"log/slog"
	"os"
)

// startupPolicy captures which primary-only startup steps this node runs.
type startupPolicy struct {
	ShouldMigrateSchema  bool
	ShouldInitSyncStatus bool
}

func newStartupPolicy(isPrimary bool) startupPolicy {
	return startupPolicy{
		ShouldMigrateSchema:  isPrimary,
		ShouldInitSyncStatus: isPrimary,
	}
}

// awaitShutdownInput holds the channels awaitShutdown selects over.
type awaitShutdownInput struct {
	SigChan  <-chan os.Signal
	ServeErr <-chan error
	Logger   *slog.Logger
}

// awaitShutdown blocks until an OS signal arrives or the HTTP server fails,
// returning the process exit code: 0 for a signal-driven graceful shutdown,
// 1 for a server failure. Routing the serve error here (instead of calling
// os.Exit inside the serve goroutine) lets main's deferred OTel flush and
// DB close run, so the log record explaining the failure reaches the OTel
// backend instead of dying in the batch processor buffer.
func awaitShutdown(in awaitShutdownInput) int {
	select {
	case sig := <-in.SigChan:
		in.Logger.Info("shutting down", slog.String("signal", sig.String()))
		return 0
	case err := <-in.ServeErr:
		in.Logger.Error("server error", slog.Any("error", err))
		return 1
	}
}
