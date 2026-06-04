package middleware_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// captureHandler is a slog.Handler that records all log records for verification.
type captureHandler struct {
	records []slog.Record
	mu      sync.Mutex
}

// Handle stores the log record.
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

// Enabled returns true for all levels.
func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// WithAttrs returns the handler unchanged (sufficient for testing).
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

// WithGroup returns the handler unchanged (sufficient for testing).
func (h *captureHandler) WithGroup(_ string) slog.Handler { return h }

func TestLogging_StatusCapture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		wantStatus int
	}{
		{name: "200 OK", statusCode: 200, wantStatus: 200},
		{name: "404 Not Found", statusCode: 404, wantStatus: 404},
		{name: "500 Internal Server Error", statusCode: 500, wantStatus: 500},
		{name: "default status (no WriteHeader)", statusCode: 0, wantStatus: 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ch := &captureHandler{}
			logger := slog.New(ch)

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.statusCode != 0 {
					w.WriteHeader(tc.statusCode)
				}
			})
			handler := middleware.Logging(logger)(inner)

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			ch.mu.Lock()
			defer ch.mu.Unlock()
			if len(ch.records) != 1 {
				t.Fatalf("expected 1 log record, got %d", len(ch.records))
			}

			var gotStatus int
			ch.records[0].Attrs(func(a slog.Attr) bool {
				if a.Key == "status" {
					gotStatus = int(a.Value.Int64())
					return false
				}
				return true
			})
			if gotStatus != tc.wantStatus {
				t.Errorf("logged status = %d, want %d", gotStatus, tc.wantStatus)
			}
		})
	}
}

func TestLogging_Attributes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Logging(logger)(inner)

	req := httptest.NewRequest("GET", "/test/path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := buf.String()

	for _, want := range []string{
		`method=GET`,
		`path=/test/path`,
		`status=200`,
		`duration=`,
	} {
		if !bytes.Contains([]byte(output), []byte(want)) {
			t.Errorf("log output missing %q; got: %s", want, output)
		}
	}
}

// TestLogging_QueryAttribute verifies the request query string is recorded on
// the access log (as `query`) when present and omitted when absent — the fix
// for /api traffic being uninspectable from url.path alone.
func TestLogging_QueryAttribute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		target    string
		wantQuery string
		wantHas   bool
	}{
		{"with query", "/api/net?asn=6939&since=1", "asn=6939&since=1", true},
		{"no query", "/api/net", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch := &captureHandler{}
			logger := slog.New(ch)
			inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
			handler := middleware.Logging(logger)(inner)

			req := httptest.NewRequest("GET", tc.target, nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)

			ch.mu.Lock()
			defer ch.mu.Unlock()
			if len(ch.records) != 1 {
				t.Fatalf("expected 1 record, got %d", len(ch.records))
			}
			var gotQuery string
			var hasQuery bool
			ch.records[0].Attrs(func(a slog.Attr) bool {
				if a.Key == "query" {
					gotQuery, hasQuery = a.Value.String(), true
				}
				return true
			})
			if hasQuery != tc.wantHas {
				t.Errorf("query attr present = %v, want %v", hasQuery, tc.wantHas)
			}
			if tc.wantHas && gotQuery != tc.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
		})
	}
}

// TestLogging_SpanGetsQueryAttribute verifies the query string is stamped onto
// the active server span as url.query, so a trace of an /api request shows the
// filter the caller sent (the original gap this enrichment closes).
func TestLogging_SpanGetsQueryAttribute(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	ctx, span := tp.Tracer("test").Start(context.Background(), "GET /api/{rest...}")

	logger := slog.New(&captureHandler{})
	inner := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
	handler := middleware.Logging(logger)(inner)

	req := httptest.NewRequest("GET", "/api/net?asn=6939", nil).WithContext(ctx)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	span.End()

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}
	var got string
	for _, a := range spans[0].Attributes() {
		if string(a.Key) == "url.query" {
			got = a.Value.AsString()
		}
	}
	if got != "asn=6939" {
		t.Errorf("span url.query = %q, want %q", got, "asn=6939")
	}
}

// mockFlusher is a ResponseWriter that also implements http.Flusher.
type mockFlusher struct {
	http.ResponseWriter
	flushed bool
}

func (f *mockFlusher) Flush() { f.flushed = true }

func TestLogging_Flush(t *testing.T) {
	t.Parallel()

	ch := &captureHandler{}
	logger := slog.New(ch)

	var flusherCalled bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
			flusherCalled = true
		}
	})
	handler := middleware.Logging(logger)(inner)

	mock := &mockFlusher{ResponseWriter: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/flush-test", nil)
	handler.ServeHTTP(mock, req)

	if !flusherCalled {
		t.Error("Flush was not called through the wrapper")
	}
	if !mock.flushed {
		t.Error("underlying Flusher.Flush was not delegated to")
	}
}

func TestLogging_SkipsHealthAndReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{name: "healthz suppressed", path: "/healthz"},
		{name: "readyz suppressed", path: "/readyz"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ch := &captureHandler{}
			logger := slog.New(ch)

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := middleware.Logging(logger)(inner)

			req := httptest.NewRequest("GET", tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			ch.mu.Lock()
			defer ch.mu.Unlock()
			if got := len(ch.records); got != 0 {
				t.Errorf("expected 0 access log records for %s, got %d", tc.path, got)
			}
		})
	}
}

// TestLogging_NonSkippedPathStillLogs guards against accidentally extending
// the skip set to swallow real traffic.
func TestLogging_NonSkippedPathStillLogs(t *testing.T) {
	t.Parallel()

	ch := &captureHandler{}
	logger := slog.New(ch)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.Logging(logger)(inner)

	req := httptest.NewRequest("GET", "/api/net", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ch.mu.Lock()
	defer ch.mu.Unlock()
	if got := len(ch.records); got != 1 {
		t.Fatalf("expected 1 record for /api/net, got %d", got)
	}
}

func TestLogging_Unwrap(t *testing.T) {
	t.Parallel()

	ch := &captureHandler{}
	logger := slog.New(ch)

	var unwrapped http.ResponseWriter
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The responseWriter wraps the original; Unwrap should return it.
		type unwrapper interface {
			Unwrap() http.ResponseWriter
		}
		if u, ok := w.(unwrapper); ok {
			unwrapped = u.Unwrap()
		}
	})
	handler := middleware.Logging(logger)(inner)

	original := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/unwrap-test", nil)
	handler.ServeHTTP(original, req)

	if unwrapped != original {
		t.Errorf("Unwrap returned %v, want original recorder %v", unwrapped, original)
	}
}
