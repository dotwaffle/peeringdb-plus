package otel

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	sdklog "go.opentelemetry.io/otel/sdk/log"
)

// NewDualLogger creates a slog.Logger that sends every log record to both
// a JSON stdout handler and the OTel log pipeline via otelslog per D-03.
// This ensures logs are always visible on stdout even if the OTel backend
// is unavailable.
func NewDualLogger(stdout io.Writer, logProvider *sdklog.LoggerProvider) *slog.Logger {
	otelHandler := otelslog.NewHandler("peeringdb-plus",
		otelslog.WithLoggerProvider(logProvider),
	)
	stdoutHandler := slog.NewJSONHandler(stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(&fanoutHandler{
		handlers: []slog.Handler{stdoutHandler, otelHandler},
	})
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
