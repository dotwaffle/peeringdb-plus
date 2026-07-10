package main

import (
	"log/slog"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// logStartupClassification emits sync-mode classification
// lines. Called once from main() after config parse and
// after slog.SetDefault, before the HTTP listener starts.
//
//   - slog.Info "sync mode" (always): auth = "authenticated" | "anonymous",
//     public_tier = "public" | "users". Exactly those two attrs, in that order.
//   - slog.Warn "public tier override active" (only when tier == TierUsers):
//     public_tier = "users", env = "PDBPLUS_PUBLIC_TIER". Exactly those two attrs.
//
// Attribute shapes are a wire contract — Grafana Loki filters and external
// parsers key off the literal strings. Do not rename without a coordinated
// dashboard update.
//
// The tests in startup_logging_test.go capture
// slog records and assert the attrs directly; changing attr keys or values
// is a breaking change to the operator contract.
func logStartupClassification(logger *slog.Logger, cfg *config.Config) {
	auth := "anonymous"
	if cfg.PeeringDBAPIKey != "" {
		auth = "authenticated"
	}
	publicTier := "public"
	if cfg.PublicTier == privctx.TierUsers {
		publicTier = "users"
	}
	logger.Info("sync mode",
		slog.String("auth", auth),
		slog.String("public_tier", publicTier),
	)
	if cfg.PublicTier == privctx.TierUsers {
		logger.Warn("public tier override active",
			slog.String("public_tier", "users"),
			slog.String("env", "PDBPLUS_PUBLIC_TIER"),
		)
	}
}
