package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/dotwaffle/peeringdb-plus/internal/visbaseline"
)

// runRedact is the -redact mode entrypoint. It takes -in (raw auth staging
// dir, e.g. /tmp/pdb-vis-capture-xxx/auth) and -out (destination for
// redacted auth under the repo's visibility-baseline tree, e.g.
// testdata/visibility-baseline/beta/auth). Anon pairs are sourced from the
// sibling directory of -out (…/auth → …/anon). GO-CFG-1 fail-fast validation
// on -in and -out.
func runRedact(cfg runConfig, logger *slog.Logger) error {
	if cfg.inDir == "" {
		return errors.New("-redact requires -in pointing at a raw auth staging dir")
	}
	if cfg.outDir == "" {
		return errors.New("-redact requires -out pointing at the redacted auth destination")
	}

	// Derive anon dir by replacing the trailing "auth" component of -out
	// with "anon". This matches the capture layout and the orchestrator's
	// prescribed command:
	//     pdbcompat-check -redact -in=/tmp/beta-raw/auth
	//                     -out=testdata/visibility-baseline/beta/auth
	// → anon dir = testdata/visibility-baseline/beta/anon
	//
	// Use filepath.Dir/filepath.Base rather than filepath.Split: Split leaves
	// a trailing separator on the parent, and Clean already strips trailing
	// separators so a TrimRight dance is redundant. Base returns the final
	// path component and Dir returns everything before it.
	outClean := filepath.Clean(cfg.outDir)
	outParent := filepath.Dir(outClean)
	outLeaf := filepath.Base(outClean)
	if outLeaf != "auth" {
		return fmt.Errorf("-redact: -out %q must end in a /auth/ component so anon pair can be derived", cfg.outDir)
	}
	// Reject degenerate paths where -out has no meaningful parent directory.
	// filepath.Dir("auth") returns ".", and filepath.Dir("/auth") returns "/".
	// Both cases would write the anon sibling into the CWD or the filesystem
	// root — almost certainly an operator mistake.
	if outParent == "." || outParent == string(filepath.Separator) {
		return fmt.Errorf("-redact: -out %q must have a parent directory holding the anon/ sibling (e.g. testdata/visibility-baseline/beta/auth)", cfg.outDir)
	}
	anonDir := filepath.Join(outParent, "anon")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("interrupt received during redact; cancelling context")
		cancel()
	}()

	rcfg := visbaseline.RedactDirConfig{
		AuthSrc: cfg.inDir,
		AnonDir: anonDir,
		Dst:     cfg.outDir,
		Logger:  logger,
	}
	if err := visbaseline.RedactDir(ctx, rcfg); err != nil {
		return fmt.Errorf("redact: %w", err)
	}
	fmt.Fprintf(os.Stdout,
		"\nRedaction complete.\nRedacted auth fixtures: %s\nAnon source: %s\nRaw auth source: %s\nNext: run `pdbcompat-check -diff -out=%s` to emit DIFF.md + diff.json.\n",
		cfg.outDir, anonDir, cfg.inDir, filepath.Dir(outClean))
	return nil
}

// runDiff is the -diff mode entrypoint. It treats -out as the baseline root
// (either a single-target dir holding anon/+auth/ or a parent dir of
// per-target subdirs) and delegates to visbaseline.BuildReport.
//
// Output placement: DIFF.md + diff.json are written at the baseline root
// itself, not at a separate output dir. This matches phase 57 D-08 which
// specifies the artifact paths as testdata/visibility-baseline/DIFF.md
// and testdata/visibility-baseline/diff.json.
func runDiff(cfg runConfig, logger *slog.Logger) error {
	if cfg.outDir == "" {
		return errors.New("-diff requires -out pointing at the baseline root (containing anon/+auth/ or per-target subdirs)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("interrupt received during diff; cancelling context")
		cancel()
	}()

	bcfg := visbaseline.BuildReportConfig{
		BaselineRoot: cfg.outDir,
		OutDir:       cfg.outDir,
		Logger:       logger,
	}
	if err := visbaseline.BuildReport(ctx, bcfg); err != nil {
		return fmt.Errorf("diff: %w", err)
	}
	fmt.Fprintf(os.Stdout,
		"\nDiff report complete.\nWrote: %s/DIFF.md\n       %s/diff.json\nReview DIFF.md before committing fixtures.\n",
		cfg.outDir, cfg.outDir)
	return nil
}
