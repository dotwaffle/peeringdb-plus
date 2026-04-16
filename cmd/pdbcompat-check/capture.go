package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/dotwaffle/peeringdb-plus/internal/visbaseline"
)

// runCapture is the -capture mode entrypoint. Parses the capture-specific
// flags, constructs visbaseline.Config, and drives the walk with a
// signal-aware context so SIGINT/SIGTERM preserve the checkpoint.
func runCapture(cfg runConfig, logger *slog.Logger) error {
	baseURL, err := targetBaseURL(cfg.target)
	if err != nil {
		return err
	}

	modes, err := parseModes(cfg.mode)
	if err != nil {
		return err
	}

	// Prod + auth opt-in gate (phase 57 research Open Question 2).
	// Auth mode against prod requires an explicit -prod-auth flag so we
	// don't accidentally hit production with the rate-limit-hungry auth
	// walk. Graceful downgrade to anon when possible.
	if cfg.target == "prod" && slices.Contains(modes, "auth") && !cfg.prodAuth {
		logger.Warn("prod target with auth requires -prod-auth; dropping auth mode")
		modes = removeString(modes, "auth")
		if len(modes) == 0 {
			return errors.New("no modes remain after prod-auth downgrade; pass -prod-auth or change -mode")
		}
	}

	types := resolveTypes(cfg.types, cfg.target)
	outDir := cfg.outDir
	if outDir == "" {
		outDir = fmt.Sprintf("testdata/visibility-baseline/%s", cfg.target)
	}

	apiKey := cfg.apiKey
	if slices.Contains(modes, "auth") && apiKey == "" {
		return errors.New("-mode=auth (or both) requires -api-key or PDBPLUS_PEERINGDB_API_KEY env var")
	}

	statePath := cfg.statePath
	if statePath == "" {
		statePath = visbaseline.DefaultStatePath
	}

	vc := visbaseline.Config{
		Target:    cfg.target,
		BaseURL:   baseURL,
		Modes:     modes,
		Types:     types,
		Pages:     2,
		OutDir:    outDir,
		APIKey:    apiKey,
		StatePath: statePath,
		Logger:    logger,
	}

	capt, err := visbaseline.New(vc)
	if err != nil {
		return fmt.Errorf("capture init: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("interrupt received; checkpoint preserved, cancelling context")
		cancel()
	}()

	rawAuthDir, err := capt.Run(ctx)
	// Surface the raw auth staging dir on BOTH success and failure so operators
	// can `rm -rf` a leaked dir or resume. On errors the path may contain a
	// partial set of auth pages — still useful for cleanup or resume.
	if rawAuthDir != "" {
		logger.LogAttrs(ctx, slog.LevelInfo, "raw auth staging dir",
			slog.String("path", rawAuthDir),
		)
	}
	if err != nil {
		return fmt.Errorf("capture run: %w", err)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "capture complete",
		slog.String("anon_out", outDir),
		slog.String("raw_auth_dir", rawAuthDir),
	)
	fmt.Fprintf(os.Stdout,
		"\nCapture complete.\nAnon fixtures: %s\nRaw auth bytes (private, DO NOT COMMIT): %s\nNext: run redaction pass + diff (plan 03) then commit.\n",
		outDir, rawAuthDir)
	return nil
}

// targetBaseURL maps the -target flag value to the PeeringDB base URL.
func targetBaseURL(target string) (string, error) {
	switch target {
	case "beta":
		return "https://beta.peeringdb.com", nil
	case "prod":
		return "https://www.peeringdb.com", nil
	default:
		return "", fmt.Errorf("unknown -target %q (want beta | prod)", target)
	}
}

// parseModes expands the -mode flag into the set of modes to walk.
func parseModes(mode string) ([]string, error) {
	switch mode {
	case "anon":
		return []string{"anon"}, nil
	case "auth":
		return []string{"auth"}, nil
	case "both":
		return []string{"anon", "auth"}, nil
	default:
		return nil, fmt.Errorf("unknown -mode %q (want anon | auth | both)", mode)
	}
}

// resolveTypes produces the list of PeeringDB types to walk given the -types
// flag and target. An explicit comma-separated list overrides defaults.
func resolveTypes(typesFlag, target string) []string {
	if typesFlag != "" {
		var out []string
		for _, t := range strings.Split(typesFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	if target == "prod" {
		return slices.Clone(visbaseline.ProdTypes)
	}
	return slices.Clone(visbaseline.AllTypes)
}

// removeString returns xs without any element equal to s. Used for the
// prod-auth graceful downgrade.
func removeString(xs []string, s string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}
