// Command pdbcompat-check fetches responses from the PeeringDB API and
// compares their structure against local golden files to detect drift.
// It uses structural comparison only: field names, value types, and
// nesting depth are checked, but actual values are not.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/conformance"
)

// allTypes lists all PeeringDB object types in sorted order.
var allTypes = []string{
	"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
	"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
}

// runConfig holds parsed command-line flags.
type runConfig struct {
	baseURL   string
	typeName  string
	goldenDir string
	timeout   time.Duration
	apiKey    string

	// Phase 57 capture mode fields. Activated by -capture; the existing
	// default path is untouched when -capture is not passed.
	capture   bool
	target    string
	mode      string
	outDir    string
	types     string
	prodAuth  bool
	statePath string

	// Phase 57 plan 04 post-capture CLI modes.
	//
	//   -redact : read raw auth bytes under -in, pair with anon fixtures
	//             under the path derived from -out (…/auth → …/anon), run
	//             Redact on each page, and write the redacted form under
	//             -out. Never writes raw auth anywhere on-repo.
	//
	//   -diff   : walk -out as a visibility-baseline root (either a single
	//             target dir with anon/+auth/ or a parent dir of per-target
	//             subdirs), run Diff per type, and emit DIFF.md + diff.json
	//             (plus per-target DIFF-{target}.md in multi-target mode).
	//
	// The two flags are mutually exclusive and mutually exclusive with
	// -capture — `run` enforces this.
	redact bool
	diff   bool
	inDir  string
}

func main() {
	cfg := runConfig{}
	flag.StringVar(&cfg.baseURL, "url", "https://beta.peeringdb.com", "PeeringDB API base URL")
	flag.StringVar(&cfg.typeName, "type", "", "PeeringDB type to check (empty = all)")
	flag.StringVar(&cfg.goldenDir, "golden-dir", "", "path to golden file directory (default: auto-detect)")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	flag.StringVar(&cfg.apiKey, "api-key", "", "PeeringDB API key (overrides PDBPLUS_PEERINGDB_API_KEY env var)")

	// Phase 57 capture mode flags.
	flag.BoolVar(&cfg.capture, "capture", false, "capture visibility baseline instead of running the structural check")
	flag.StringVar(&cfg.target, "target", "beta", "capture target: beta | prod")
	flag.StringVar(&cfg.mode, "mode", "both", "capture mode: anon | auth | both")
	flag.StringVar(&cfg.outDir, "out", "", "output dir: capture=anon fixtures root, redact=redacted auth dst, diff=baseline root")
	flag.StringVar(&cfg.types, "types", "", "comma-separated types to capture (default: 13 for beta, poc,org,net for prod)")
	flag.BoolVar(&cfg.prodAuth, "prod-auth", false, "allow auth mode against prod target (requires API key; default false)")
	flag.StringVar(&cfg.statePath, "state", "", "checkpoint file path (default: /tmp/pdb-vis-capture-state.json)")

	// Phase 57 plan 04 post-capture flags.
	flag.BoolVar(&cfg.redact, "redact", false, "redact raw auth bytes under -in and write the redacted form under -out")
	flag.BoolVar(&cfg.diff, "diff", false, "build DIFF.md + diff.json from the baseline tree rooted at -out")
	flag.StringVar(&cfg.inDir, "in", "", "input dir: redact=raw auth staging dir (e.g. /tmp/pdb-vis-capture-xxx/auth)")

	flag.Parse()

	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(cfg, logger); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg runConfig, logger *slog.Logger) error {
	// Phase 57 mode dispatch. At most one of -capture / -redact / -diff may
	// be set; combining them is a user error.
	modeCount := 0
	if cfg.capture {
		modeCount++
	}
	if cfg.redact {
		modeCount++
	}
	if cfg.diff {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("at most one of -capture, -redact, -diff may be set")
	}
	if cfg.capture {
		return runCapture(cfg, logger)
	}
	if cfg.redact {
		return runRedact(cfg, logger)
	}
	if cfg.diff {
		return runDiff(cfg, logger)
	}
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
	return slices.Contains(allTypes, name)
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

func init() {
	// Ensure allTypes stays sorted.
	slices.Sort(allTypes)
}
