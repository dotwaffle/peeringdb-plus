package middleware_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// recoveryCaptureHandler is a slog.Handler that records all log records for
// panic recovery verification.
type recoveryCaptureHandler struct {
	records []slog.Record
	mu      sync.Mutex
}

func (h *recoveryCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recoveryCaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *recoveryCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler                { return h }
func (h *recoveryCaptureHandler) WithGroup(_ string) slog.Handler                     { return h }

func TestRecovery_NoPanic(t *testing.T) {
	t.Parallel()

	ch := &recoveryCaptureHandler{}
	logger := slog.New(ch)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})
	handler := middleware.Recovery(logger)(inner)

	req := httptest.NewRequest("GET", "/healthy", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.records) != 0 {
		t.Errorf("expected no log records on successful request, got %d", len(ch.records))
	}
}

func TestRecovery_PanicString(t *testing.T) {
	t.Parallel()

	ch := &recoveryCaptureHandler{}
	logger := slog.New(ch)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})
	handler := middleware.Recovery(logger)(inner)

	req := httptest.NewRequest("GET", "/panic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "internal server error")
	}

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(ch.records))
	}
	if ch.records[0].Level != slog.LevelError {
		t.Errorf("log level = %v, want %v", ch.records[0].Level, slog.LevelError)
	}
	if ch.records[0].Message != "panic recovered" {
		t.Errorf("log message = %q, want %q", ch.records[0].Message, "panic recovered")
	}
}

func TestRecovery_PanicError(t *testing.T) {
	t.Parallel()

	ch := &recoveryCaptureHandler{}
	logger := slog.New(ch)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(fmt.Errorf("error panic"))
	})
	handler := middleware.Recovery(logger)(inner)

	req := httptest.NewRequest("GET", "/panic-error", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRecovery_LogAttributes(t *testing.T) {
	t.Parallel()

	ch := &recoveryCaptureHandler{}
	logger := slog.New(ch)

	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("attr test")
	})
	handler := middleware.Recovery(logger)(inner)

	req := httptest.NewRequest("POST", "/check-attrs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if len(ch.records) != 1 {
		t.Fatalf("expected 1 log record, got %d", len(ch.records))
	}

	attrs := make(map[string]slog.Value)
	ch.records[0].Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})

	// Verify panic value is logged.
	if v, ok := attrs["panic"]; !ok {
		t.Error("missing 'panic' attribute in log record")
	} else if !strings.Contains(v.String(), "attr test") {
		t.Errorf("panic attr = %q, want it to contain %q", v.String(), "attr test")
	}

	// Verify stack trace is logged.
	if v, ok := attrs["stack"]; !ok {
		t.Error("missing 'stack' attribute in log record")
	} else if !strings.Contains(v.String(), "goroutine") {
		t.Errorf("stack attr does not appear to contain a stack trace: %q", v.String())
	}

	// Verify method is logged.
	if v, ok := attrs["method"]; !ok {
		t.Error("missing 'method' attribute in log record")
	} else if v.String() != "POST" {
		t.Errorf("method attr = %q, want %q", v.String(), "POST")
	}

	// Verify path is logged.
	if v, ok := attrs["path"]; !ok {
		t.Error("missing 'path' attribute in log record")
	} else if v.String() != "/check-attrs" {
		t.Errorf("path attr = %q, want %q", v.String(), "/check-attrs")
	}
}
