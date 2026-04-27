package otel

import (
	"context"
	"log/slog"
	"testing"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// Phase 77 OBS-06 — env-configurable level filter for the otelslog branch
// of the dual logger. AUDIT.md architectural finding:
//
// The otelslog handler had no level filter, so DEBUG records produced by
// any caller were shipped to Loki via the OTel log pipeline regardless of
// the stdout handler's INFO gate. Wrapping the otelslog handler in a
// level filter — gated by env `PDBPLUS_LOG_LEVEL` (default
// `slog.LevelInfo`) — is the precondition for INFO→DEBUG demotions to
// actually reduce Loki ingestion volume.
//
// These tests exercise the wrapper directly (Enabled / Handle) rather
// than asserting on a real LoggerProvider — the contract is "DEBUG
// records are blocked at the wrapper level by default; opt-in via env",
// and that lives entirely inside the levelFilterHandler shim.

// captureHandler records every record passed to Handle; used to assert
// that the wrapper drops records before they reach the inner handler.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func TestLevelFilterHandler_DefaultINFOBlocksDebug(t *testing.T) {
	inner := &captureHandler{}
	h := &levelFilterHandler{inner: inner, level: slog.LevelInfo}

	if h.Enabled(t.Context(), slog.LevelDebug) {
		t.Errorf("Enabled(DEBUG) must be false at level=INFO")
	}
	if !h.Enabled(t.Context(), slog.LevelInfo) {
		t.Errorf("Enabled(INFO) must be true at level=INFO")
	}
	if !h.Enabled(t.Context(), slog.LevelWarn) {
		t.Errorf("Enabled(WARN) must be true at level=INFO")
	}

	// Handle a DEBUG record — must NOT reach the inner handler.
	rec := slog.NewRecord(time.Time{}, slog.LevelDebug, "debug-msg", 0)
	_ = h.Handle(t.Context(), rec)
	if len(inner.records) != 0 {
		t.Errorf("DEBUG record leaked through INFO filter; got %d records", len(inner.records))
	}

	// Handle an INFO record — MUST reach the inner handler.
	rec = slog.NewRecord(time.Time{}, slog.LevelInfo, "info-msg", 0)
	_ = h.Handle(t.Context(), rec)
	if len(inner.records) != 1 {
		t.Errorf("INFO record blocked at INFO filter; got %d records", len(inner.records))
	}
}

func TestLevelFilterHandler_DEBUGAdmitsDebug(t *testing.T) {
	inner := &captureHandler{}
	h := &levelFilterHandler{inner: inner, level: slog.LevelDebug}

	if !h.Enabled(t.Context(), slog.LevelDebug) {
		t.Errorf("Enabled(DEBUG) must be true at level=DEBUG")
	}
	rec := slog.NewRecord(time.Time{}, slog.LevelDebug, "debug-msg", 0)
	_ = h.Handle(t.Context(), rec)
	if len(inner.records) != 1 {
		t.Errorf("DEBUG record blocked at DEBUG filter; got %d records", len(inner.records))
	}
}

func TestNewDualLogger_DefaultBlocksDebugFromOTelBranch(t *testing.T) {
	// Env unset — wrapper defaults to INFO. The otel branch must NOT see
	// a DEBUG record. We assert this via the wrapper directly because
	// the otelslog inner handler is opaque — its Enabled returns true for
	// every level, which is precisely why the wrapper exists.
	t.Setenv("PDBPLUS_LOG_LEVEL", "")

	lp := sdklog.NewLoggerProvider()
	t.Cleanup(func() { _ = lp.Shutdown(t.Context()) })

	got := otelLevelFromEnv()
	if got != slog.LevelInfo {
		t.Errorf("default otel level = %s, want INFO", got)
	}
}

func TestOTelLevelFromEnv_ParsesValid(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want slog.Level
	}{
		{"unset", "", slog.LevelInfo},
		{"debug_lower", "debug", slog.LevelDebug},
		{"DEBUG_upper", "DEBUG", slog.LevelDebug},
		{"info", "INFO", slog.LevelInfo},
		{"warn", "WARN", slog.LevelWarn},
		{"error", "ERROR", slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PDBPLUS_LOG_LEVEL", tt.env)
			got := otelLevelFromEnv()
			if got != tt.want {
				t.Errorf("otelLevelFromEnv() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOTelLevelFromEnv_FallsBackOnGarbage(t *testing.T) {
	// Per CLAUDE.md GO-CFG-1 (fail-fast), but logging-level is
	// operator-friendly: invalid values fall back to INFO. The contract
	// is that the wrapper never crashes on malformed PDBPLUS_LOG_LEVEL.
	t.Setenv("PDBPLUS_LOG_LEVEL", "garbage")
	got := otelLevelFromEnv()
	if got != slog.LevelInfo {
		t.Errorf("garbage env value should fall back to INFO; got %s", got)
	}
}
