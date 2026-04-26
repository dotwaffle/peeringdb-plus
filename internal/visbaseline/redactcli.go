package visbaseline

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RedactDirConfig parameterises a RedactDir run. Grouped per GO-CS-5 because
// the caller passes more than two arguments.
//
// Layout expectations:
//
//   - AuthSrc points at a raw-auth staging tree laid down by Capture, i.e.
//     .../auth/api/{type}/page-N.json. Both the "auth/api/..." subpath and
//     the flat "api/..." form are accepted: RedactDir walks AuthSrc and
//     treats every *.json file whose relative path contains ".../{type}/
//     page-N.json" as a page to redact.
//   - AnonDir points at the repo-side anon mirror, i.e. .../anon/api/{type}/
//     page-N.json. RedactDir maps each auth page to its anon counterpart by
//     {type}+{page}; missing pairs are an error (an anon page is the only
//     source of truth for "which fields are already public", so skipping it
//     would over-disclose).
//   - Dst is the repo-side redacted auth destination, i.e.
//     .../auth/api/{type}/page-N.json. RedactDir creates directories as
//     needed with mode 0700 and writes files with mode 0600.
type RedactDirConfig struct {
	AuthSrc string
	AnonDir string
	Dst     string
	Logger  *slog.Logger
}

// RedactDir walks a raw-auth staging tree, pairs each page with its anon
// counterpart, runs Redact, and writes the redacted bytes under Dst.
//
// Errors are fail-fast: a missing anon counterpart, a malformed JSON, or a
// failed write halts the walk. Partial output under Dst is left as-is for
// the operator to inspect — the caller is expected to `rm -rf` Dst before a
// retry. Raw auth input bytes are never echoed in error messages (T-57-05).
//
// RedactDir honours ctx between files: cancellation mid-walk returns
// ctx.Err() wrapped.
func RedactDir(ctx context.Context, cfg RedactDirConfig) error {
	if cfg.AuthSrc == "" {
		return errors.New("RedactDir: AuthSrc required")
	}
	if cfg.AnonDir == "" {
		return errors.New("RedactDir: AnonDir required")
	}
	if cfg.Dst == "" {
		return errors.New("RedactDir: Dst required")
	}
	if fi, err := os.Stat(cfg.AuthSrc); err != nil || !fi.IsDir() {
		return fmt.Errorf("RedactDir: AuthSrc %q not a readable directory: %w", cfg.AuthSrc, err)
	}
	if fi, err := os.Stat(cfg.AnonDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("RedactDir: AnonDir %q not a readable directory: %w", cfg.AnonDir, err)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	var pages int
	walkErr := filepath.WalkDir(cfg.AuthSrc, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		typeName, page, ok := parsePagePath(path)
		if !ok {
			// Silently skip files not matching the {type}/page-N.json shape —
			// stray files in the staging tree (e.g. operator notes) are
			// tolerated.
			return nil
		}

		anonPath := filepath.Join(cfg.AnonDir, "api", typeName, fmt.Sprintf("page-%d.json", page))
		// visbaseline is a CLI tool — paths are operator-supplied by contract.
		// filepath.Clean handles the genuine `..` traversal sub-class of gosec
		// G304 risk; the cleaned path also satisfies gosec's static analysis so
		// no nolint directive is required.
		anonPath = filepath.Clean(anonPath)
		anonBytes, err := os.ReadFile(anonPath)
		if err != nil {
			return fmt.Errorf("read anon %s/%d: %w", typeName, page, err)
		}
		// visbaseline is a CLI tool — paths are operator-supplied by contract.
		// filepath.Clean as defense-in-depth against `..` traversal; the cleaned
		// path also satisfies gosec G304's static analysis. G122 (TOCTOU in
		// WalkDir callback) is suppressed here because the redactor is a
		// single-tenant operator-driven CLI run against a staging tree the
		// operator controls — a symlink-race attacker would already have write
		// access to the operator's redaction workspace.
		path = filepath.Clean(path)
		authBytes, err := os.ReadFile(path) //nolint:gosec // G122: visbaseline CLI staging tree, see comment above
		if err != nil {
			return fmt.Errorf("read auth %s/%d: %w", typeName, page, err)
		}

		redacted, err := Redact(anonBytes, authBytes)
		if err != nil {
			// Never embed input bytes in the error — Redact already redacts
			// in-memory but the input bytes are unredacted auth.
			return fmt.Errorf("redact %s/%d: %w", typeName, page, err)
		}

		dstPath := filepath.Join(cfg.Dst, "api", typeName, fmt.Sprintf("page-%d.json", page))
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(dstPath), err)
		}
		if err := os.WriteFile(dstPath, redacted, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", dstPath, err)
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "redacted page",
			slog.String("type", typeName),
			slog.Int("page", page),
			slog.String("dst", dstPath),
		)
		pages++
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", cfg.AuthSrc, walkErr)
	}
	if pages == 0 {
		return fmt.Errorf("RedactDir: no page-N.json files found under %s", cfg.AuthSrc)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "redaction complete",
		slog.Int("pages", pages),
		slog.String("dst", cfg.Dst),
	)
	return nil
}

// parsePagePath extracts (type, page) from a path ending in
// ".../{type}/page-N.json". Returns (_, _, false) if the path does not match.
//
// Strict parsing: the page number must be composed exclusively of ASCII
// digits, with no sign, whitespace, or trailing garbage. fmt.Sscanf("%d")
// is too permissive — it accepts leading whitespace, signed values, and
// stops at the first non-matching byte without caring about trailing
// content (so "1abc" would parse as page 1, causing stray files like
// "page-1_backup.json" to collide with real captures).
func parsePagePath(path string) (string, int, bool) {
	base := filepath.Base(path)
	dir := filepath.Base(filepath.Dir(path))
	if !strings.HasPrefix(base, "page-") || !strings.HasSuffix(base, ".json") {
		return "", 0, false
	}
	numStr := strings.TrimSuffix(strings.TrimPrefix(base, "page-"), ".json")
	// Reject anything that is not a pure run of ASCII digits before handing
	// to strconv.Atoi. strconv.Atoi already rejects "+5" / " 5" / "1abc"
	// but belt-and-braces guards against a future Go that relaxes Atoi.
	if numStr == "" {
		return "", 0, false
	}
	for i := 0; i < len(numStr); i++ {
		if numStr[i] < '0' || numStr[i] > '9' {
			return "", 0, false
		}
	}
	n, err := strconv.Atoi(numStr)
	if err != nil || n < 1 {
		return "", 0, false
	}
	if dir == "" || dir == "." || dir == string(filepath.Separator) {
		return "", 0, false
	}
	return dir, n, true
}
