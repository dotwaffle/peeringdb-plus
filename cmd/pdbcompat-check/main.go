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
	"sort"
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
	baseURL    string
	typeName   string
	goldenDir  string
	timeout    time.Duration
}

func main() {
	cfg := runConfig{}
	flag.StringVar(&cfg.baseURL, "url", "https://beta.peeringdb.com", "PeeringDB API base URL")
	flag.StringVar(&cfg.typeName, "type", "", "PeeringDB type to check (empty = all)")
	flag.StringVar(&cfg.goldenDir, "golden-dir", "", "path to golden file directory (default: auto-detect)")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(cfg, logger); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(cfg runConfig, logger *slog.Logger) error {
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

		diffs, err := checkType(ctx, client, cfg.baseURL, goldenDir, typeName)
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "check failed",
				slog.String("type", typeName),
				slog.String("error", err.Error()),
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
func checkType(ctx context.Context, client *http.Client, baseURL, goldenDir, typeName string) ([]conformance.Difference, error) {
	// Fetch from PeeringDB.
	url := fmt.Sprintf("%s/api/%s?limit=1", baseURL, typeName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", typeName, err)
	}
	req.Header.Set("User-Agent", "pdbcompat-check/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", typeName, err)
	}
	defer resp.Body.Close()

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
	for _, t := range allTypes {
		if t == name {
			return true
		}
	}
	return false
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
	sort.Strings(allTypes)
}
