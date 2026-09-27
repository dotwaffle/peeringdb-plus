package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// cspReportFixture is a CSP report handler on a mux, with its counted
// directives and its log output.
type cspReportFixture struct {
	mux     *http.ServeMux
	counted []string
	log     bytes.Buffer
}

func newCSPReportFixture(t *testing.T, limit *rate.Limiter) *cspReportFixture {
	t.Helper()
	f := &cspReportFixture{mux: http.NewServeMux()}
	f.mux.Handle("POST "+cspReportPath, newCSPReportHandler(cspReportInput{
		Logger:   slog.New(slog.NewTextHandler(&f.log, nil)),
		LogLimit: limit,
		Count: func(_ context.Context, directive string) {
			f.counted = append(f.counted, directive)
		},
	}))
	return f
}

func (f *cspReportFixture) post(contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, cspReportPath, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestCSPReport_Legacy(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(rate.Inf, 0))

	rec := f.post("application/csp-report", `{"csp-report":{
		"document-uri":"https://mirror.example/ui/?q=secret+search",
		"violated-directive":"script-src-elem 'self'",
		"effective-directive":"script-src-elem",
		"blocked-uri":"https://evil.example/x.js?token=abc#frag",
		"source-file":"https://mirror.example/static/ui.js?v=0123",
		"line-number":12,
		"disposition":"report"}}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !slices.Equal(f.counted, []string{"script-src-elem"}) {
		t.Errorf("counted = %q, want [script-src-elem]", f.counted)
	}
	log := f.log.String()
	for _, want := range []string{
		`msg="csp violation"`,
		"directive=script-src-elem",
		"blocked_uri=https://evil.example/x.js ",
		"document_uri=https://mirror.example/ui/ ",
		"line=12",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("log has no %q:\n%s", want, log)
		}
	}
	for _, leak := range []string{"secret", "token", "frag"} {
		if strings.Contains(log, leak) {
			t.Errorf("log keeps the query or fragment %q:\n%s", leak, log)
		}
	}
}

func TestCSPReport_ViolatedDirectiveOnly(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(rate.Inf, 0))

	rec := f.post("application/csp-report", `{"csp-report":{"violated-directive":"img-src 'self' data:","blocked-uri":"data"}}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !slices.Equal(f.counted, []string{"img-src"}) {
		t.Errorf("counted = %q, want [img-src]", f.counted)
	}
}

func TestCSPReport_ReportingAPI(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(rate.Inf, 0))

	rec := f.post("application/reports+json", `[
		{"type":"csp-violation","body":{"effectiveDirective":"style-src-attr","blockedURL":"inline"}},
		{"type":"deprecation","body":{}},
		{"type":"csp-violation","body":{"effectiveDirective":"made-up-directive","blockedURL":"eval"}}
	]`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if !slices.Equal(f.counted, []string{"style-src-attr", "other"}) {
		t.Errorf("counted = %q, want [style-src-attr other]", f.counted)
	}
}

func TestCSPReport_EntryCap(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(rate.Inf, 0))

	entry := `{"type":"csp-violation","body":{"effectiveDirective":"img-src"}}`
	body := "[" + strings.TrimSuffix(strings.Repeat(entry+",", cspReportMaxEntries+5), ",") + "]"
	if rec := f.post("application/reports+json", body); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(f.counted) != cspReportMaxEntries {
		t.Errorf("counted %d reports, want %d", len(f.counted), cspReportMaxEntries)
	}
}

func TestCSPReport_LogLimit(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(0, 1))

	for range 3 {
		f.post("application/csp-report", `{"csp-report":{"effective-directive":"img-src"}}`)
	}
	if len(f.counted) != 3 {
		t.Errorf("counted %d reports, want 3: the limit applies to the log only", len(f.counted))
	}
	if n := strings.Count(f.log.String(), "csp violation"); n != 1 {
		t.Errorf("%d log lines, want 1", n)
	}
}

func TestCSPReport_Rejects(t *testing.T) {
	t.Parallel()
	f := newCSPReportFixture(t, rate.NewLimiter(rate.Inf, 0))

	tests := []struct {
		name        string
		method      string
		contentType string
		body        string
		want        int
	}{
		{"not json", http.MethodPost, "application/csp-report", "{", http.StatusBadRequest},
		{"wrong media type", http.MethodPost, "text/plain", "{}", http.StatusUnsupportedMediaType},
		{"too large", http.MethodPost, "application/csp-report", `{"csp-report":{"blocked-uri":"` + strings.Repeat("a", cspReportMaxBytes) + `"}}`, http.StatusRequestEntityTooLarge},
		{"get", http.MethodGet, "", "", http.StatusMethodNotAllowed},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, cspReportPath, strings.NewReader(tt.body))
		if tt.contentType != "" {
			req.Header.Set("Content-Type", tt.contentType)
		}
		rec := httptest.NewRecorder()
		f.mux.ServeHTTP(rec, req)
		if rec.Code != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, rec.Code, tt.want)
		}
	}
	if len(f.counted) != 0 {
		t.Errorf("counted %q from rejected bodies", f.counted)
	}
}
