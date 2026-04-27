package otel

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// NewDualLogger creates a slog.Logger that sends every log record to both
// a JSON stdout handler and the OTel log pipeline via otelslog per D-03.
// This ensures logs are always visible on stdout even if the OTel backend
// is unavailable.
//
// The OTel branch is wrapped in a [levelFilterHandler] gated by
// [otelLevelFromEnv] (env var PDBPLUS_LOG_LEVEL, default INFO) so DEBUG
// records do not ship to the OTel logging pipeline (and from there to Loki)
// unless the operator explicitly opts in. The stdout branch keeps its
// existing INFO gate. See Phase 77 AUDIT.md for the audit finding.
func NewDualLogger(stdout io.Writer, logProvider *sdklog.LoggerProvider) *slog.Logger {
	otelHandler := otelslog.NewHandler("peeringdb-plus",
		otelslog.WithLoggerProvider(logProvider),
	)
	filteredOtel := &levelFilterHandler{inner: otelHandler, level: otelLevelFromEnv()}
	stdoutHandler := slog.NewJSONHandler(stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(&fanoutHandler{
		handlers: []slog.Handler{stdoutHandler, filteredOtel},
	})
}

// otelLevelFromEnv returns the slog.Level the otelslog branch should gate on.
// Reads PDBPLUS_LOG_LEVEL; defaults to slog.LevelInfo. Invalid values fall
// back to INFO without crashing — logging-level config is operator-friendly
// per the v1.18 OBS-06 audit (CLAUDE.md GO-CFG-1 prefers fail-fast, but a
// malformed log level should not take production down).
func otelLevelFromEnv() slog.Level {
	v := os.Getenv("PDBPLUS_LOG_LEVEL")
	if v == "" {
		return slog.LevelInfo
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		return slog.LevelInfo
	}
	return lvl
}

// levelFilterHandler wraps a slog.Handler with a minimum-level gate. Records
// below the gate are dropped before reaching the inner handler. Used to gate
// the otelslog branch of [NewDualLogger] independently of the stdout branch.
type levelFilterHandler struct {
	inner slog.Handler
	level slog.Level
}

func (h *levelFilterHandler) Enabled(ctx context.Context, l slog.Level) bool {
	if l < h.level {
		return false
	}
	return h.inner.Enabled(ctx, l)
}

func (h *levelFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < h.level {
		return nil
	}
	return h.inner.Handle(ctx, r)
}

func (h *levelFilterHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelFilterHandler{inner: h.inner.WithAttrs(attrs), level: h.level}
}

func (h *levelFilterHandler) WithGroup(name string) slog.Handler {
	return &levelFilterHandler{inner: h.inner.WithGroup(name), level: h.level}
}

// fanoutHandler dispatches slog records to multiple handlers.
// It implements slog.Handler.
type fanoutHandler struct {
	handlers []slog.Handler
}

// Enabled returns true if any sub-handler is enabled for the given level.
func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle dispatches the record to each enabled sub-handler.
// Errors from individual handlers are best-effort ignored to avoid
// one failing handler blocking the other.
func (h *fanoutHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			_ = handler.Handle(ctx, record)
		}
	}
	return nil
}

// WithAttrs returns a new fanoutHandler with the given attrs applied
// to each sub-handler.
func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: newHandlers}
}

// WithGroup returns a new fanoutHandler with the given group applied
// to each sub-handler.
func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &fanoutHandler{handlers: newHandlers}
}
