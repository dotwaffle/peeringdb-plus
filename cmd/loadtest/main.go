//go:build loadtest

// Command loadtest is an operator tool that exercises every API
// surface of a peeringdb-plus deployment (default
// https://peeringdb-plus.fly.dev) for capacity validation, dashboard
// warmup, and load reproduction.
//
// SAFETY: this binary is build-tag-isolated behind `//go:build
// loadtest` so `go build ./...`, `go test ./...`, and CI never
// compile or run it. NEVER point --base at https://www.peeringdb.com
// — upstream PeeringDB enforces 1 req/hour per IP and will block
// your address.
//
// Three modes are supported:
//
//	loadtest endpoints [flags]   one-shot inventory sweep across all 5 surfaces
//	loadtest sync       [flags]  replay the 13-step ordered sync sequence
//	loadtest soak       [flags]  sustained QPS-capped mixed-surface load
//
// Build:  go build -tags loadtest -o loadtest ./cmd/loadtest
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// safetyBanner is printed at the top of every --help output AND
// re-printed in cmd/loadtest/README.md. The exact phrase
// "NEVER point --base at https://www.peeringdb.com" is greppable and
// is checked by the plan's verification step.
const safetyBanner = `WARNING: peeringdb-plus loadtest tool

This tool drives sustained traffic against a peeringdb-plus mirror.
Default --base is https://peeringdb-plus.fly.dev.

NEVER point --base at https://www.peeringdb.com — upstream PeeringDB
enforces a 1-request-per-hour rate limit per IP address and will
block your IP if you exceed it. This tool is for the MIRROR only.

Do NOT run this tool from CI. It is intentionally build-tag-isolated
(//go:build loadtest) so that go build ./... and go test ./... ignore it.

`

// Config carries flag-parsed runtime state through the three mode
// dispatchers. Per GO-CTX-1 the context is NEVER stored on Config —
// it is always passed as the first argument to every helper.
type Config struct {
	Base       string
	Timeout    time.Duration
	Verbose    bool
	AuthToken  string
	HTTPClient *http.Client

	// Sync flags.
	SyncMode  string
	SinceFlag string

	// Soak flags.
	SoakDuration    time.Duration
	SoakConcurrency int
	SoakQPS         float64
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(1)
	}
}

func run(argv []string, stdout, stderr *os.File) error {
	if len(argv) == 0 || argv[0] == "-h" || argv[0] == "--help" {
		printHelp(stdout)
		if len(argv) == 0 {
			return errors.New("missing subcommand (endpoints|sync|soak)")
		}
		return nil
	}

	mode := argv[0]
	rest := argv[1:]

	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := Config{}
	fs.StringVar(&cfg.Base, "base", "https://peeringdb-plus.fly.dev",
		"base URL of the peeringdb-plus deployment to load-test (e.g. http://localhost:8080)")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "per-request timeout")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "emit per-request log lines")

	switch mode {
	case "sync":
		fs.StringVar(&cfg.SyncMode, "mode", "full", "sync mode: full or incremental")
		fs.StringVar(&cfg.SinceFlag, "since", "",
			"incremental cursor (RFC3339 or unix seconds); defaults to now-1h when --mode=incremental and unset")
	case "soak":
		fs.DurationVar(&cfg.SoakDuration, "duration", 30*time.Second, "total soak duration")
		fs.IntVar(&cfg.SoakConcurrency, "concurrency", 4, "number of concurrent workers")
		fs.Float64Var(&cfg.SoakQPS, "qps", 5.0,
			"global request-per-second cap (rate-limited via golang.org/x/time/rate)")
	}

	fs.Usage = func() {
		fmt.Fprint(stderr, safetyBanner)
		fmt.Fprintf(stderr, "Usage: loadtest %s [flags]\n\n", mode)
		fs.PrintDefaults()
	}

	if err := fs.Parse(rest); err != nil {
		return err
	}

	cfg.AuthToken = os.Getenv("PDBPLUS_LOADTEST_AUTH_TOKEN")
	cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}

	// Print the safety banner before every run (not just --help) so
	// operators can't miss it.
	fmt.Fprint(stdout, safetyBanner)
	fmt.Fprintf(stdout, "mode=%s base=%s timeout=%s auth=%s\n\n",
		mode, cfg.Base, cfg.Timeout, authPresence(cfg.AuthToken))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	rep := NewReport()
	var err error
	switch mode {
	case "endpoints":
		err = runEndpoints(ctx, cfg, registryAll(), rep, stdout)
	case "sync":
		var since time.Time
		since, err = parseSinceFlag(cfg.SyncMode, cfg.SinceFlag)
		if err != nil {
			return err
		}
		err = runSync(ctx, cfg, cfg.SyncMode, since, rep, stdout)
	case "soak":
		err = runSoak(ctx, cfg, cfg.SoakDuration, cfg.SoakConcurrency, cfg.SoakQPS, registryAll(), rep)
	default:
		return fmt.Errorf("unknown mode %q (want endpoints|sync|soak)", mode)
	}

	rep.Print(stdout, mode)
	return err
}

func printHelp(w *os.File) {
	fmt.Fprint(w, safetyBanner)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  loadtest endpoints [--base URL] [--timeout DUR] [--verbose]")
	fmt.Fprintln(w, "  loadtest sync      [--mode full|incremental] [--since RFC3339|unix] [--base URL]")
	fmt.Fprintln(w, "  loadtest soak      [--duration DUR] [--concurrency N] [--qps F] [--base URL]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Auth: set PDBPLUS_LOADTEST_AUTH_TOKEN to send 'Authorization: Bearer <token>' on every request.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run `loadtest <mode> --help` for mode-specific flag defaults.")
}

func authPresence(token string) string {
	if token == "" {
		return "anonymous"
	}
	return "bearer-token-set"
}

// parseSinceFlag interprets --since as RFC3339 first, then unix
// seconds. When mode==incremental and the flag is empty, default to
// now-1h. Full mode ignores --since entirely.
func parseSinceFlag(mode, since string) (time.Time, error) {
	if mode != "incremental" {
		return time.Time{}, nil
	}
	if since == "" {
		return time.Now().Add(-time.Hour), nil
	}
	if t, err := time.Parse(time.RFC3339, since); err == nil {
		return t, nil
	}
	if n, err := strconv.ParseInt(since, 10, 64); err == nil {
		return time.Unix(n, 0), nil
	}
	return time.Time{}, fmt.Errorf("--since=%q: not RFC3339 or unix seconds", since)
}
