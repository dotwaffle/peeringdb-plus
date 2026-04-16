package peeringdb_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// discardLogger returns a slog.Logger that swallows all output -- keeps test
// output clean.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fastClient returns a *peeringdb.Client wired to the given base URL with its
// rate limiter set to rate.Inf (no wait) and its retry base delay at 1ms so
// httptest-backed tests execute in sub-second time.
func fastClient(t *testing.T, baseURL string, opts ...peeringdb.ClientOption) *peeringdb.Client {
	t.Helper()
	c := peeringdb.NewClient(baseURL, discardLogger(), opts...)
	c.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	c.SetRetryBaseDelay(time.Millisecond)
	return c
}

func TestFetchRawPageHappyPath(t *testing.T) {
	t.Parallel()

	// Distinctive body so we can assert byte-for-byte equivalence.
	body := []byte(`{"meta":{"generated":0},"data":[{"id":1,"name":"example"}]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/poc") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL)
	got, err := c.FetchRawPage(context.Background(), "poc", 1)
	if err != nil {
		t.Fatalf("FetchRawPage: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("bytes mismatch\n got:  %s\n want: %s", got, body)
	}
}

func TestFetchRawPageRateLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL)
	_, err := c.FetchRawPage(context.Background(), "poc", 1)
	if err == nil {
		t.Fatal("FetchRawPage returned nil error, want *RateLimitError")
	}
	var rlErr *peeringdb.RateLimitError
	if !errors.As(err, &rlErr) {
		t.Fatalf("err type = %T, want *RateLimitError (err=%v)", err, err)
	}
	if rlErr.RetryAfter != 1*time.Second {
		t.Errorf("RetryAfter = %s, want 1s", rlErr.RetryAfter)
	}
	if rlErr.Status != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want %d", rlErr.Status, http.StatusTooManyRequests)
	}
}

func TestFetchRawPageAuthHeader(t *testing.T) {
	t.Parallel()

	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL, peeringdb.WithAPIKey("test-key-xyz"))
	if _, err := c.FetchRawPage(context.Background(), "poc", 1); err != nil {
		t.Fatalf("FetchRawPage: %v", err)
	}
	want := "Api-Key test-key-xyz"
	if gotHeader != want {
		t.Errorf("Authorization header = %q, want %q", gotHeader, want)
	}
}

func TestFetchRawPageURL(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL)
	if _, err := c.FetchRawPage(context.Background(), "poc", 1); err != nil {
		t.Fatalf("FetchRawPage: %v", err)
	}
	if gotPath != "/api/poc" {
		t.Errorf("path = %q, want /api/poc", gotPath)
	}
	want := "limit=250&skip=0&depth=0"
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestFetchRawPagePage2(t *testing.T) {
	t.Parallel()

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL)
	if _, err := c.FetchRawPage(context.Background(), "poc", 2); err != nil {
		t.Fatalf("FetchRawPage: %v", err)
	}
	want := "limit=250&skip=250&depth=0"
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestFetchRawPageInvalidPage(t *testing.T) {
	t.Parallel()

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer srv.Close()

	c := fastClient(t, srv.URL)
	_, err := c.FetchRawPage(context.Background(), "poc", 0)
	if err == nil {
		t.Fatal("FetchRawPage(page=0) returned nil, want error")
	}
	if hits != 0 {
		t.Errorf("server saw %d hits for invalid page; want 0 (fail-before-HTTP)", hits)
	}

	_, err = c.FetchRawPage(context.Background(), "poc", -1)
	if err == nil {
		t.Fatal("FetchRawPage(page=-1) returned nil, want error")
	}
	if hits != 0 {
		t.Errorf("server saw %d hits for invalid page; want 0", hits)
	}
}
