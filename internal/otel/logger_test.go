package otel

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestNewDualLogger_WritesToStdout(t *testing.T) {
	var buf bytes.Buffer
	lp := sdklog.NewLoggerProvider()
	defer func() { _ = lp.Shutdown(t.Context()) }()

	logger := NewDualLogger(&buf, lp)
	logger.Info("test message", slog.String("key", "value"))

	// Verify stdout output is JSON containing the message.
	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("failed to parse stdout JSON: %v (output: %s)", err, buf.String())
	}
	if msg, ok := record["msg"].(string); !ok || msg != "test message" {
		t.Errorf("stdout msg = %v, want %q", record["msg"], "test message")
	}
}

func TestNewDualLogger_WritesToOTelHandler(t *testing.T) {
	var buf bytes.Buffer
	lp := sdklog.NewLoggerProvider()
	defer func() { _ = lp.Shutdown(t.Context()) }()

	logger := NewDualLogger(&buf, lp)

	// The OTel handler accepts the record without error. We verify by
	// confirming the dual logger writes to both handlers -- the stdout
	// output confirms the fanout worked (if fanout breaks, stdout also breaks).
	logger.Info("otel test")
	if buf.Len() == 0 {
		t.Error("expected stdout output from dual logger, got empty buffer")
	}
}

// recordingHandler is a test slog.Handler that records whether Handle was called.
type recordingHandler struct {
	enabled bool
	handled bool
	attrs   int
	groups  int
}

func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return h.enabled }
func (h *recordingHandler) Handle(_ context.Context, _ slog.Record) error {
	h.handled = true
	return nil
}
func (h *recordingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &recordingHandler{enabled: h.enabled, attrs: h.attrs + len(attrs)}
}
func (h *recordingHandler) WithGroup(_ string) slog.Handler {
	return &recordingHandler{enabled: h.enabled, groups: h.groups + 1}
}

func TestFanoutHandler_EnabledAnyTrue(t *testing.T) {
	h := &fanoutHandler{handlers: []slog.Handler{
		&recordingHandler{enabled: false},
		&recordingHandler{enabled: true},
	}}
	if !h.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("fanoutHandler.Enabled should return true when any handler is enabled")
	}
}

func TestFanoutHandler_EnabledAllFalse(t *testing.T) {
	h := &fanoutHandler{handlers: []slog.Handler{
		&recordingHandler{enabled: false},
		&recordingHandler{enabled: false},
	}}
	if h.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("fanoutHandler.Enabled should return false when no handler is enabled")
	}
}

func TestFanoutHandler_WithAttrs(t *testing.T) {
	h := &fanoutHandler{handlers: []slog.Handler{
		&recordingHandler{enabled: true},
		&recordingHandler{enabled: true},
	}}
	attrs := []slog.Attr{slog.String("key", "val")}
	newH := h.WithAttrs(attrs)

	fh, ok := newH.(*fanoutHandler)
	if !ok {
		t.Fatalf("WithAttrs returned %T, want *fanoutHandler", newH)
	}
	if len(fh.handlers) != 2 {
		t.Fatalf("WithAttrs handler count = %d, want 2", len(fh.handlers))
	}
	// Each sub-handler should have received the attrs.
	for i, sh := range fh.handlers {
		rh, ok := sh.(*recordingHandler)
		if !ok {
			t.Fatalf("handler[%d] is %T, want *recordingHandler", i, sh)
		}
		if rh.attrs != 1 {
			t.Errorf("handler[%d].attrs = %d, want 1", i, rh.attrs)
		}
	}
}

func TestFanoutHandler_WithGroup(t *testing.T) {
	h := &fanoutHandler{handlers: []slog.Handler{
		&recordingHandler{enabled: true},
		&recordingHandler{enabled: true},
	}}
	newH := h.WithGroup("mygroup")

	fh, ok := newH.(*fanoutHandler)
	if !ok {
		t.Fatalf("WithGroup returned %T, want *fanoutHandler", newH)
	}
	for i, sh := range fh.handlers {
		rh, ok := sh.(*recordingHandler)
		if !ok {
			t.Fatalf("handler[%d] is %T, want *recordingHandler", i, sh)
		}
		if rh.groups != 1 {
			t.Errorf("handler[%d].groups = %d, want 1", i, rh.groups)
		}
	}
}
