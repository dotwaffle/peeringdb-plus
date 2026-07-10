// Command pdbcompat-check validates PeeringDB API compatibility via four
// subcommands, each with its own flag set:
//
//	check    fetch live responses and compare their structure against
//	         local golden files (field names, value types, nesting depth —
//	         never actual values)
//	capture  walk the API anonymously and authenticated, writing
//	         visibility-baseline fixtures (raw auth bytes staged off-repo)
//	redact   pair raw auth bytes with anon fixtures and write the
//	         redacted auth form under the baseline tree
//	diff     build DIFF.md + diff.json from a captured baseline tree
//
// Run `pdbcompat-check <subcommand> -h` for per-mode flags.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/conformance"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
)

// allTypes lists all PeeringDB object types in sorted order.
var allTypes = pdbtypes.SortedNames()

// runConfig holds parsed command-line flags. Each subcommand registers
// only the fields it consumes; the rest stay zero-valued.
type runConfig struct {
	// check flags.
	baseURL   string
	typeName  string
	goldenDir string
	timeout   time.Duration
	apiKey    string // also capture

	// capture flags.
	target    string
	mode      string
	types     string
	prodAuth  bool
	statePath string

	// redact/diff flags. outDir is capture=anon fixtures root,
	// redact=redacted auth destination, diff=baseline root.
	outDir string
	inDir  string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// run dispatches on the subcommand in argv[0], parses that mode's flag
// set, and delegates to the matching runX entrypoint. Mode exclusivity
// is structural: exactly one subcommand runs per invocation.
func run(argv []string, stdout, stderr io.Writer) error {
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" {
		printHelp(stdout)
		if len(argv) == 0 {
			return errors.New("missing subcommand (check|capture|redact|diff)")
		}
		return nil
	}

	sub := argv[0]
	fs := flag.NewFlagSet(sub, flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := runConfig{}
	switch sub {
	case "check":
		fs.StringVar(&cfg.baseURL, "url", "https://beta.peeringdb.com", "PeeringDB API base URL")
		fs.StringVar(&cfg.typeName, "type", "", "PeeringDB type to check (empty = all)")
		fs.StringVar(&cfg.goldenDir, "golden-dir", "", "path to golden file directory (default: auto-detect)")
		fs.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "HTTP request timeout")
		fs.StringVar(&cfg.apiKey, "api-key", "", "PeeringDB API key (overrides PDBPLUS_PEERINGDB_API_KEY env var)")
	case "capture":
		fs.StringVar(&cfg.target, "target", "beta", "capture target: beta | prod")
		fs.StringVar(&cfg.mode, "mode", "both", "capture mode: anon | auth | both")
		fs.StringVar(&cfg.outDir, "out", "", "anon fixtures root (default: testdata/visibility-baseline/<target>)")
		fs.StringVar(&cfg.types, "types", "", "comma-separated types to capture (default: 13 for beta, poc,org,net for prod)")
		fs.BoolVar(&cfg.prodAuth, "prod-auth", false, "allow auth mode against prod target (requires API key; default false)")
		fs.StringVar(&cfg.statePath, "state", "", "checkpoint file path (default: /tmp/pdb-vis-capture-state.json)")
		fs.StringVar(&cfg.apiKey, "api-key", "", "PeeringDB API key (overrides PDBPLUS_PEERINGDB_API_KEY env var)")
	case "redact":
		fs.StringVar(&cfg.inDir, "in", "", "raw auth staging dir (e.g. /tmp/pdb-vis-capture-xxx/auth)")
		fs.StringVar(&cfg.outDir, "out", "", "redacted auth destination (must end in /auth so the anon sibling can be derived)")
	case "diff":
		fs.StringVar(&cfg.outDir, "out", "", "baseline root (containing anon/+auth/ or per-target subdirs)")
	default:
		printHelp(stderr)
		return fmt.Errorf("unknown subcommand %q (want check|capture|redact|diff)", sub)
	}

	if err := fs.Parse(argv[1:]); err != nil {
		return err
	}

	cfg.apiKey = resolveAPIKey(cfg.apiKey, os.Getenv("PDBPLUS_PEERINGDB_API_KEY"))

	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	switch sub {
	case "check":
		return runCheck(cfg, logger)
	case "capture":
		return runCapture(cfg, logger)
	case "redact":
		return runRedact(cfg, logger)
	default: // "diff" — the unknown-subcommand case returned above.
		return runDiff(cfg, logger)
	}
}

// resolveAPIKey applies the env-var fallback: an explicit flag value
// wins, otherwise the PDBPLUS_PEERINGDB_API_KEY env value is used.
func resolveAPIKey(flagVal, envVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return envVal
}

// printHelp writes the subcommand overview.
func printHelp(w io.Writer) {
	fmt.Fprint(w, `Usage: pdbcompat-check <subcommand> [flags]

Subcommands:
  check    compare live PeeringDB response structure against golden files
  capture  capture visibility-baseline fixtures (anon + auth walks)
  redact   redact raw auth bytes against their anon pairs
  diff     build DIFF.md + diff.json from a captured baseline tree

Run 'pdbcompat-check <subcommand> -h' for that mode's flags.
`)
}

// runCheck is the check subcommand entrypoint: the structural drift
// check comparing live responses against golden files.
func runCheck(cfg runConfig, logger *slog.Logger) error {
	client := &http.Client{Timeout: cfg.timeout}

	types := allTypes
	if cfg.typeName != "" {
		if !isValidType(cfg.typeName) {
			return fmt.Errorf("unknown type %q", cfg.typeName)
		}
		types = []string{cfg.typeName}
	}

	goldenDir := cfg.goldenDir
	if goldenDir == "" {
		goldenDir = findGoldenDir()
	}

	ctx := context.Background()
	var totalDiffs int
	var failures int

	for i, typeName := range types {
		if i > 0 {
			// Respect PeeringDB rate limits: 1 request per 3 seconds.
			time.Sleep(3 * time.Second)
		}

		logger.LogAttrs(ctx, slog.LevelInfo, "checking type",
			slog.String("type", typeName),
		)

		diffs, err := checkType(ctx, client, cfg.baseURL, goldenDir, typeName, cfg.apiKey)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "check failed",
				slog.String("type", typeName),
				slog.Any("error", err),
			)
			failures++
			continue
		}

		if len(diffs) == 0 {
			fmt.Printf("%-12s OK\n", typeName)
		} else {
			fmt.Printf("%-12s %d difference(s):\n", typeName, len(diffs))
			for _, d := range diffs {
				fmt.Printf("  %-14s %-20s %s\n", d.Kind, d.Path, d.Details)
			}
			totalDiffs += len(diffs)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d type(s) failed to check", failures)
	}
	if totalDiffs > 0 {
		return fmt.Errorf("%d structural difference(s) found", totalDiffs)
	}
	return nil
}

// checkType fetches a PeeringDB response for the given type and compares
// its structure against the corresponding golden file.
func checkType(ctx context.Context, client *http.Client, baseURL, goldenDir, typeName, apiKey string) ([]conformance.Difference, error) {
	// Fetch from PeeringDB.
	url := fmt.Sprintf("%s/api/%s?limit=1", baseURL, typeName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", typeName, err)
	}
	req.Header.Set("User-Agent", "pdbcompat-check/1.0")
	if apiKey != "" {
		req.Header.Set("Authorization", "Api-Key "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", typeName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", typeName, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: HTTP %d", typeName, resp.StatusCode)
	}

	liveBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", typeName, err)
	}

	// Read golden file.
	goldenPath := filepath.Join(goldenDir, typeName, "list.json")
	goldenBody, err := os.ReadFile(goldenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("golden file not found: %s (run golden tests with -update to create)", goldenPath)
		}
		return nil, fmt.Errorf("read golden file %s: %w", goldenPath, err)
	}

	return conformance.CompareResponses(goldenBody, liveBody)
}

// isValidType reports whether the given name is a known PeeringDB type.
func isValidType(name string) bool {
	return pdbtypes.Valid(name)
}

// findGoldenDir attempts to locate the golden file directory relative to the
// current working directory or common project layouts.
func findGoldenDir() string {
	candidates := []string{
		"internal/pdbcompat/testdata/golden",
		"../internal/pdbcompat/testdata/golden",
		"../../internal/pdbcompat/testdata/golden",
	}

	// Also try from GOMOD root.
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}

	// Fall back to CWD-relative path.
	return "internal/pdbcompat/testdata/golden"
}
