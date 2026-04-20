// Package config loads application configuration from environment variables.
// All configuration is immutable after initialization (CFG-2).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// SyncMode controls the sync strategy.
type SyncMode string

const (
	// SyncModeFull performs a complete re-fetch of all objects.
	SyncModeFull SyncMode = "full"
	// SyncModeIncremental fetches only objects modified since the last sync.
	SyncModeIncremental SyncMode = "incremental"
)

// privateIPNets are the RFC 1918 private-use IPv4 ranges that may appear in
// http:// URLs for PDBPLUS_PEERINGDB_URL (local dev carveout per SEC-06).
// Parsed once at package init so validate() is allocation-free on the hot
// path (even though validate is startup-only, package-level parsing keeps the
// CIDR strings in one place and eliminates per-call net.ParseCIDR failures).
var privateIPNets = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			// Constant input — unreachable at runtime, panic surfaces any future typo.
			panic(fmt.Sprintf("config: invalid builtin CIDR %q: %v", c, err))
		}
		nets = append(nets, n)
	}
	return nets
}()

// Config holds all application configuration. Immutable after Load returns.
type Config struct {
	// DBPath is the path to the SQLite database file.
	DBPath string

	// PeeringDBBaseURL is the base URL for the PeeringDB API.
	PeeringDBBaseURL string

	// SyncToken is the shared secret for on-demand sync trigger authentication.
	SyncToken string

	// SyncInterval is the duration between automatic sync runs.
	SyncInterval time.Duration

	// ListenAddr is the address the HTTP server binds to.
	ListenAddr string

	// CORSOrigins is a comma-separated list of allowed CORS origins.
	// Configured via PDBPLUS_CORS_ORIGINS. Default is "*".
	CORSOrigins string

	// DrainTimeout is the graceful shutdown drain timeout.
	// Configured via PDBPLUS_DRAIN_TIMEOUT. Default is 10 seconds.
	DrainTimeout time.Duration

	// OTelSampleRate is the trace sampling ratio (0.0 to 1.0).
	// Configured via PDBPLUS_OTEL_SAMPLE_RATE. Default is 1.0 (always sample) per D-02.
	OTelSampleRate float64

	// SyncStaleThreshold is the maximum age of sync data before health reports degraded.
	// Configured via PDBPLUS_SYNC_STALE_THRESHOLD. Default is 24h per D-12.
	SyncStaleThreshold time.Duration

	// SyncMode controls whether sync uses full re-fetch or incremental delta fetch.
	// Configured via PDBPLUS_SYNC_MODE. Default is "full".
	SyncMode SyncMode

	// PublicTier is the resolved visibility tier for anonymous HTTP callers.
	// Configured via PDBPLUS_PUBLIC_TIER. Default TierPublic admits only
	// visible="Public" rows. Setting PDBPLUS_PUBLIC_TIER=users elevates
	// anonymous callers to TierUsers for private-instance deployments
	// (D-11, SYNC-03). Parsed case-sensitive lowercase only (D-12); any
	// other value is a fail-fast startup error per GO-CFG-1.
	PublicTier privctx.Tier

	// PeeringDBAPIKey is the optional PeeringDB API key for authenticated access.
	// Configured via PDBPLUS_PEERINGDB_API_KEY. Empty string means unauthenticated.
	PeeringDBAPIKey string

	// StreamTimeout is the maximum duration for a single streaming RPC.
	// Configured via PDBPLUS_STREAM_TIMEOUT. Default is 60 seconds.
	StreamTimeout time.Duration

	// CSPEnforce controls whether the CSP middleware serves the enforcing
	// Content-Security-Policy header (true) or the Content-Security-Policy-Report-Only
	// header (false). Configured via PDBPLUS_CSP_ENFORCE. Default is false per SEC-07
	// rollout strategy — enforcement is opt-in per deploy through v1.13.
	CSPEnforce bool

	// SyncMemoryLimit is the peak Go heap ceiling (bytes) checked after
	// the sync worker's Phase A fetch pass completes and before the
	// database transaction opens. If runtime.ReadMemStats reports
	// HeapAlloc above this value, the sync aborts with a WARN log and
	// returns sync.ErrSyncMemoryLimitExceeded. The next scheduled sync
	// retries normally after the current batches are reclaimed by GC.
	//
	// Configured via PDBPLUS_SYNC_MEMORY_LIMIT. Default is 400MB —
	// matches the DEBT-03 regression gate in BenchmarkSyncWorker_FullMemoryPeak
	// and leaves 112 MB headroom under the 512 MB Fly.io VM cap per
	// v1.13 hard constraint. Set to 0 to disable the guardrail (local
	// dev only; guardrail is defense-in-depth against runtime memory
	// spikes that exceed what the benchmark harness measured).
	//
	// Unit suffix is required (KB/MB/GB/TB, base 1024); bare numbers
	// are rejected for unambiguous operator configuration.
	SyncMemoryLimit int64

	// ResponseMemoryLimit is the per-response memory budget in bytes.
	// Before streaming a pdbcompat list response, the handler runs a
	// pre-flight SELECT COUNT(*) and multiplies by a conservative
	// per-row byte estimate; if the product exceeds this budget, the
	// request is rejected with an RFC 9457 413 problem-detail BEFORE
	// any row data is materialised.
	//
	// Configured via PDBPLUS_RESPONSE_MEMORY_LIMIT. Default is 128 MiB
	// (134217728 bytes) per Phase 71 D-05 — sized against the 256 MB
	// replica total minus an 80 MB Go runtime baseline and 48 MB slack
	// for other in-flight requests + GC overhead. Unit suffix is
	// REQUIRED (KB/MB/GB/TB, base 1024); bare numbers are rejected.
	// Set to 0 to disable the check (local dev only; guardrail is the
	// reason Phase 68's limit=0 is safe to expose in prod).
	ResponseMemoryLimit int64

	// HeapWarnBytes is the peak Go heap threshold (bytes) above which the
	// sync worker emits slog.Warn("heap threshold crossed", ...) at the
	// end of each sync cycle. The OTel span attribute
	// pdbplus.sync.peak_heap_mib is emitted regardless; only the Warn is
	// threshold-gated. Configured via PDBPLUS_HEAP_WARN_MIB (integer MiB,
	// no unit suffix). Default 400 MiB matches the Fly 512 MB VM cap minus
	// a 112 MB safety margin (D-04). Zero disables the warn (attr still
	// emitted so dashboards retain timeseries).
	//
	// SEED-001: sustained breach is the escalation signal for considering
	// PDBPLUS_SYNC_MODE=incremental.
	HeapWarnBytes int64

	// RSSWarnBytes is the peak OS RSS threshold (bytes) above which the
	// sync worker emits slog.Warn at the end of each sync cycle. Read from
	// /proc/self/status VmHWM on Linux; skipped on other OSes (the RSS
	// attr is then omitted — it is not set to zero). Configured via
	// PDBPLUS_RSS_WARN_MIB (integer MiB, no unit suffix). Default 384 MiB.
	// Zero disables the warn.
	RSSWarnBytes int64
}

// Load reads configuration from environment variables, applies defaults,
// validates required fields, and returns an immutable Config.
// It fails fast on invalid configuration per CFG-1.
func Load() (*Config, error) {
	// Resolve listen address: PDBPLUS_PORT takes precedence over PDBPLUS_LISTEN_ADDR per D-24.
	listenAddr := envOrDefault("PDBPLUS_LISTEN_ADDR", ":8080")
	if port := os.Getenv("PDBPLUS_PORT"); port != "" {
		listenAddr = ":" + port
	}

	cfg := &Config{
		DBPath:           envOrDefault("PDBPLUS_DB_PATH", "./peeringdb-plus.db"),
		PeeringDBBaseURL: envOrDefault("PDBPLUS_PEERINGDB_URL", "https://api.peeringdb.com"),
		SyncToken:        envOrDefault("PDBPLUS_SYNC_TOKEN", ""),
		ListenAddr:       listenAddr,
		CORSOrigins:      envOrDefault("PDBPLUS_CORS_ORIGINS", "*"),
		PeeringDBAPIKey:  envOrDefault("PDBPLUS_PEERINGDB_API_KEY", ""),
	}

	// PDBPLUS_SYNC_INTERVAL has an auth-conditional default: 15m when an API
	// key is configured (the authenticated rate-limit budget comfortably
	// absorbs a 4× sync frequency), 1h when unauthenticated (stay conservative
	// against the shared anonymous ceiling). An explicit override wins in
	// either case. os.LookupEnv distinguishes "unset" from "" so the operator
	// can deliberately revert to the unauthenticated default with an empty
	// string if desired — though in practice they would just unset it.
	syncIntervalRaw, intervalExplicit := os.LookupEnv("PDBPLUS_SYNC_INTERVAL")
	switch {
	case intervalExplicit && syncIntervalRaw != "":
		d, err := time.ParseDuration(syncIntervalRaw)
		if err != nil {
			return nil, fmt.Errorf("parsing PDBPLUS_SYNC_INTERVAL: invalid duration %q for PDBPLUS_SYNC_INTERVAL: %w", syncIntervalRaw, err)
		}
		cfg.SyncInterval = d
	case cfg.PeeringDBAPIKey != "":
		cfg.SyncInterval = 15 * time.Minute
	default:
		cfg.SyncInterval = 1 * time.Hour
	}

	// PDBPLUS_INCLUDE_DELETED was removed in v1.16 Phase 68 (D-01). Sync now
	// always persists deleted rows as tombstones (Phase 68 Plan 02 lands the
	// soft-delete flip); pdbcompat applies the upstream status × since matrix
	// regardless of any legacy gate. During the v1.16 → v1.17 grace period,
	// the variable is logged and ignored. Flipping to fail-fast in v1.17 is a
	// one-line swap (slog.Warn → return nil, errors.New(...)).
	if v := os.Getenv("PDBPLUS_INCLUDE_DELETED"); v != "" {
		// gosec G706 log-injection: value is operator-supplied env var
		// contents attached as a structured slog attribute (not interpolated
		// into the message), the variable is a boolean flag with no secret
		// content, and slog.String quotes the value on output. Safe per
		// GO-SEC-2 (no secrets in env) + threat register T-68-01-03.
		slog.Warn("PDBPLUS_INCLUDE_DELETED is deprecated and ignored; remove it from your environment. This will be a startup error in v1.17.", //nolint:gosec // G706: boolean flag, structured attr, threat register T-68-01-03
			slog.String("value", v),
		)
	}

	drainTimeout, err := parseDuration("PDBPLUS_DRAIN_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_DRAIN_TIMEOUT: %w", err)
	}
	cfg.DrainTimeout = drainTimeout

	sampleRate, err := parseFloat64("PDBPLUS_OTEL_SAMPLE_RATE", 1.0)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_OTEL_SAMPLE_RATE: %w", err)
	}
	cfg.OTelSampleRate = sampleRate

	syncStaleThreshold, err := parseDuration("PDBPLUS_SYNC_STALE_THRESHOLD", 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_SYNC_STALE_THRESHOLD: %w", err)
	}
	cfg.SyncStaleThreshold = syncStaleThreshold

	syncMode, err := parseSyncMode("PDBPLUS_SYNC_MODE", SyncModeFull)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_SYNC_MODE: %w", err)
	}
	cfg.SyncMode = syncMode

	publicTier, err := parsePublicTier("PDBPLUS_PUBLIC_TIER", privctx.TierPublic)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_PUBLIC_TIER: %w", err)
	}
	cfg.PublicTier = publicTier

	streamTimeout, err := parseDuration("PDBPLUS_STREAM_TIMEOUT", 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_STREAM_TIMEOUT: %w", err)
	}
	cfg.StreamTimeout = streamTimeout

	cspEnforce, err := parseBool("PDBPLUS_CSP_ENFORCE", false)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_CSP_ENFORCE: %w", err)
	}
	cfg.CSPEnforce = cspEnforce

	syncMemoryLimit, err := parseByteSize("PDBPLUS_SYNC_MEMORY_LIMIT", 400*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_SYNC_MEMORY_LIMIT: %w", err)
	}
	cfg.SyncMemoryLimit = syncMemoryLimit

	responseMemoryLimit, err := parseByteSize("PDBPLUS_RESPONSE_MEMORY_LIMIT", 128*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_RESPONSE_MEMORY_LIMIT: %w", err)
	}
	cfg.ResponseMemoryLimit = responseMemoryLimit

	heapWarn, err := parseMiB("PDBPLUS_HEAP_WARN_MIB", 400)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_HEAP_WARN_MIB: %w", err)
	}
	cfg.HeapWarnBytes = heapWarn

	rssWarn, err := parseMiB("PDBPLUS_RSS_WARN_MIB", 384)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_RSS_WARN_MIB: %w", err)
	}
	cfg.RSSWarnBytes = rssWarn

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	// Single-line operator-visible announcement of the effective sync interval
	// and which inputs produced it. The API key itself is NEVER logged — only
	// the boolean "was one configured" is emitted. `explicit_override` makes
	// support triage trivial: "did the operator set SYNC_INTERVAL, or is the
	// 15m/1h default in play?"
	slog.Info("sync interval configured",
		slog.Duration("interval", cfg.SyncInterval),
		slog.Bool("authenticated", cfg.PeeringDBAPIKey != ""),
		slog.Bool("explicit_override", intervalExplicit),
	)

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DBPath == "" {
		return errors.New("PDBPLUS_DB_PATH must not be empty")
	}
	if c.SyncInterval <= 0 {
		return errors.New("PDBPLUS_SYNC_INTERVAL must be greater than 0")
	}
	if c.OTelSampleRate < 0.0 || c.OTelSampleRate > 1.0 {
		return errors.New("PDBPLUS_OTEL_SAMPLE_RATE must be between 0.0 and 1.0")
	}
	if !strings.Contains(c.ListenAddr, ":") {
		return errors.New("PDBPLUS_LISTEN_ADDR must contain ':' (e.g., ':8080' or '0.0.0.0:8080')")
	}
	if err := validatePeeringDBURL(c.PeeringDBBaseURL); err != nil {
		return err
	}
	if c.DrainTimeout <= 0 {
		return errors.New("PDBPLUS_DRAIN_TIMEOUT must be greater than 0")
	}
	if c.SyncMemoryLimit < 0 {
		return errors.New("PDBPLUS_SYNC_MEMORY_LIMIT must be non-negative (0 = disabled)")
	}
	if c.ResponseMemoryLimit < 0 {
		return errors.New("PDBPLUS_RESPONSE_MEMORY_LIMIT must be non-negative (0 = disabled)")
	}
	if c.HeapWarnBytes < 0 {
		return errors.New("PDBPLUS_HEAP_WARN_MIB must be non-negative (0 = disabled)")
	}
	if c.RSSWarnBytes < 0 {
		return errors.New("PDBPLUS_RSS_WARN_MIB must be non-negative (0 = disabled)")
	}
	return nil
}

// validatePeeringDBURL enforces SEC-06: PDBPLUS_PEERINGDB_URL must be https://,
// or http:// against loopback (localhost / 127.0.0.1 / ::1) or RFC 1918 private
// ranges (10/8, 172.16/12, 192.168/16). Each rejection class produces a distinct
// error message so operators can diagnose misconfiguration from a log line.
//
// Trap avoided (PITFALLS.md §mp-7): url.Parse("example.com") succeeds with an
// empty Scheme — the caller MUST check Scheme explicitly, not just err.
func validatePeeringDBURL(raw string) error {
	if raw == "" {
		return errors.New("PDBPLUS_PEERINGDB_URL must not be empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("PDBPLUS_PEERINGDB_URL is not a valid URL (got %q): %w", raw, err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("PDBPLUS_PEERINGDB_URL missing scheme — expected https:// (got %q)", raw)
	}
	// Scheme is checked BEFORE host — a URL like "file:///tmp/foo" has an
	// empty host by design, but the scheme rejection is the more useful
	// diagnostic, so it wins classification.
	switch u.Scheme {
	case "https":
		if u.Host == "" {
			return fmt.Errorf("PDBPLUS_PEERINGDB_URL has empty host (got %q)", raw)
		}
		return nil
	case "http":
		if u.Host == "" {
			return fmt.Errorf("PDBPLUS_PEERINGDB_URL has empty host (got %q)", raw)
		}
		if isLocalOrPrivateHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("PDBPLUS_PEERINGDB_URL uses http:// against a non-local host %q — use https:// for production or a loopback/RFC-1918 host for local dev", u.Hostname())
	default:
		return fmt.Errorf("PDBPLUS_PEERINGDB_URL has unsupported scheme %q — expected https (or http for local dev)", u.Scheme)
	}
}

// isLocalOrPrivateHost reports whether host is the string "localhost", a
// loopback IP literal, or an RFC 1918 private IPv4 literal. Hostnames other
// than "localhost" are NOT resolved via DNS — production hosts cannot bypass
// the https:// requirement by setting an A record to 127.0.0.1.
func isLocalOrPrivateHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, n := range privateIPNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q for %s: %w", v, key, err)
	}
	return d, nil
}

func parseFloat64(key string, defaultVal float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid float %q for %s: %w", v, key, err)
	}
	return f, nil
}

func parseBool(key string, defaultVal bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid bool %q for %s: %w", v, key, err)
	}
	return b, nil
}

func parseSyncMode(key string, defaultVal SyncMode) (SyncMode, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	switch SyncMode(v) {
	case SyncModeFull, SyncModeIncremental:
		return SyncMode(v), nil
	default:
		return "", fmt.Errorf("invalid sync mode %q for %s: must be 'full' or 'incremental'", v, key)
	}
}

// parsePublicTier reads key from the environment and maps its value to a
// privctx.Tier. Empty / unset → defaultVal. Only the lowercase strings
// "public" and "users" are accepted (D-12). Any other value — including
// case variants ("Users", "PUBLIC") and whitespace-adjacent forms
// ("public ") — is a hard error so the process fails fast at startup per
// GO-CFG-1. Mirrors parseSyncMode verbatim.
//
// The strict switch (not strings.ToLower + parse) is a deliberate fail-
// safe-closed choice: a typo like PDBPLUS_PUBLIC_TIER=Users must not
// silently default to either tier (T-59-05 threat mitigation).
func parsePublicTier(key string, defaultVal privctx.Tier) (privctx.Tier, error) {
	v := os.Getenv(key)
	switch v {
	case "":
		return defaultVal, nil
	case "public":
		return privctx.TierPublic, nil
	case "users":
		return privctx.TierUsers, nil
	default:
		return 0, fmt.Errorf("invalid value %q for %s: must be 'public' or 'users'", v, key)
	}
}

// parseByteSize parses an env var as a byte count with a MANDATORY unit
// suffix (KB, MB, GB, TB — base 1024). An empty env var falls back to
// defaultVal. The literal "0" is accepted as an explicit disable value.
// A bare number without a unit is REJECTED to eliminate ambiguity in
// operator configuration (PERF-05 ergonomics) — was the 500 meant as
// 500 bytes, 500 KB, or 500 MB? Force the operator to be explicit.
//
// Accepted forms: "0", "100KB", "400MB", "2GB", "1TB" (case-insensitive
// suffix). The short forms "K", "M", "G", "T" are accepted as aliases
// for "KB", "MB", "GB", "TB" respectively.
//
// Examples: "400MB" -> 400 * 1024 * 1024; "1GB" -> 1024^3; "0" -> 0
// (guardrail disabled).
//
// parseMiB parses an env var as a non-negative integer count of MiB
// (mebibytes; 1 MiB = 1024*1024 bytes). Returns the value in BYTES
// so callers can compare directly against runtime.MemStats fields.
//
// Unlike parseByteSize, no unit suffix is accepted — the variable name
// encodes the unit (PDBPLUS_HEAP_WARN_MIB, PDBPLUS_RSS_WARN_MIB).
// Operators attempting "400MB" get a clear error rather than silent
// coercion.
//
// Accepted: "", "0", "400", "16" (bare non-negative integers).
// Rejected: "-5", "abc", "400MB", "1.5" — all fail-fast per GO-CFG-1.
func parseMiB(key string, defaultMiB int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultMiB * 1024 * 1024, nil
	}
	mib, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid MiB value %q for %s: must be a non-negative integer (no unit suffix)", v, key)
	}
	if mib < 0 {
		return 0, fmt.Errorf("invalid MiB value %q for %s: must be non-negative", v, key)
	}
	const bytesPerMiB = int64(1024 * 1024)
	if mib > math.MaxInt64/bytesPerMiB {
		return 0, fmt.Errorf("invalid MiB value %q for %s: overflows int64", v, key)
	}
	return mib * bytesPerMiB, nil
}

func parseByteSize(key string, defaultVal int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	// Allow literal "0" as an explicit disable value.
	if v == "0" {
		return 0, nil
	}
	// Locate the first unit-suffix rune. Accept upper- and lower-case.
	idx := strings.IndexFunc(v, func(r rune) bool {
		switch r {
		case 'K', 'M', 'G', 'T', 'k', 'm', 'g', 't':
			return true
		}
		return false
	})
	if idx < 0 {
		return 0, fmt.Errorf("invalid byte size %q for %s: missing unit (KB/MB/GB/TB)", v, key)
	}
	if idx == 0 {
		return 0, fmt.Errorf("invalid byte size %q for %s: missing numeric prefix", v, key)
	}
	num, err := strconv.ParseInt(v[:idx], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q for %s: %w", v, key, err)
	}
	if num < 0 {
		return 0, fmt.Errorf("invalid byte size %q for %s: must be non-negative", v, key)
	}
	unit := strings.ToUpper(v[idx:])
	var mult int64
	switch unit {
	case "K", "KB":
		mult = 1024
	case "M", "MB":
		mult = 1024 * 1024
	case "G", "GB":
		mult = 1024 * 1024 * 1024
	case "T", "TB":
		mult = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid byte size %q for %s: unknown unit %q (want KB/MB/GB/TB)", v, key, unit)
	}
	// Overflow guard: num*mult must fit in int64. Operators realistically
	// never configure petabyte heap limits, but the helper is shaped as a
	// general-purpose parser; defense in depth. See 54-REVIEW.md WR-02.
	if num != 0 && mult != 0 && num > math.MaxInt64/mult {
		return 0, fmt.Errorf("invalid byte size %q for %s: overflows int64", v, key)
	}
	return num * mult, nil
}
