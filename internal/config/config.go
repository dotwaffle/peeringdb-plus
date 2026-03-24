// Package config loads application configuration from environment variables.
// All configuration is immutable after initialization (CFG-2).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// SyncMode controls the sync strategy.
type SyncMode string

const (
	// SyncModeFull performs a complete re-fetch of all objects.
	SyncModeFull SyncMode = "full"
	// SyncModeIncremental fetches only objects modified since the last sync.
	SyncModeIncremental SyncMode = "incremental"
)

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

	// IncludeDeleted controls whether objects with status=deleted are synced.
	IncludeDeleted bool

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
	}

	syncInterval, err := parseDuration("PDBPLUS_SYNC_INTERVAL", 1*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_SYNC_INTERVAL: %w", err)
	}
	cfg.SyncInterval = syncInterval

	includeDeleted, err := parseBool("PDBPLUS_INCLUDE_DELETED", false)
	if err != nil {
		return nil, fmt.Errorf("parsing PDBPLUS_INCLUDE_DELETED: %w", err)
	}
	cfg.IncludeDeleted = includeDeleted

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

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

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
	return nil
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
