//go:build loadtest

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestSoak_RespectsDuration runs runSoak for ~200ms at 10 qps with 2
// workers against an httptest server; expects between 1 and ~5
// completed requests (rate-limit jitter + token bucket warmup).
func TestSoak_RespectsDuration(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := Config{
		Base:       srv.URL,
		HTTPClient: srv.Client(),
	}
	rep := NewReport()
	eps := []Endpoint{
		{Surface: SurfacePdbCompat, EntityType: "net", Shape: "list-default", Method: "GET", Path: "/api/net?limit=10"},
		{Surface: SurfaceWebUI, Shape: "ui-home", Method: "GET", Path: "/ui/"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := runSoak(ctx, cfg, 200*time.Millisecond, 2, 10.0, eps, rep); err != nil {
		t.Fatalf("runSoak: %v", err)
	}
	got := rep.Len()
	if got < 1 || got > 6 {
		t.Errorf("got %d results, want [1, 6]", got)
	}
}

// TestSoak_CtxCancellation asserts a cancelled ctx returns within
// 100ms even if --duration is much longer.
func TestSoak_CtxCancellation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := Config{
		Base:       srv.URL,
		HTTPClient: srv.Client(),
	}
	rep := NewReport()
	eps := []Endpoint{{Surface: SurfacePdbCompat, EntityType: "net", Method: "GET", Path: "/api/net"}}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runSoak(ctx, cfg, 10*time.Second, 2, 10.0, eps, rep)
	}()

	time.AfterFunc(50*time.Millisecond, cancel)

	select {
	case <-done:
		// success — runSoak returned promptly
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("runSoak did not return within 500ms after ctx cancel")
	}
}

// TestSoak_QPSCap asserts the observed request rate over a 1s window
// stays within ±30% of the requested qps cap. The cap is global
// across workers; concurrency=4 and qps=10 should still yield ~10
// req/s, not 40.
//
// We use ±30% rather than the plan's ±20% because httptest
// in-process latency is sub-µs and the rate.Limiter token-bucket
// burst (initial token) lets the first request through without
// waiting. ±30% absorbs that warm-up while still catching a broken
// limiter that runs at concurrency*qps.
func TestSoak_QPSCap(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := Config{
		Base:       srv.URL,
		HTTPClient: srv.Client(),
	}
	rep := NewReport()
	eps := []Endpoint{{Surface: SurfacePdbCompat, EntityType: "net", Method: "GET", Path: "/api/net"}}

	const targetQPS = 10.0
	const window = 1 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 2*window)
	defer cancel()

	if err := runSoak(ctx, cfg, window, 4, targetQPS, eps, rep); err != nil {
		t.Fatalf("runSoak: %v", err)
	}

	got := float64(hits.Load())
	want := targetQPS * window.Seconds()
	tolerance := 0.30 * want
	if got < want-tolerance || got > want+tolerance {
		t.Errorf("observed %v requests in %v, want %v ± %v", got, window, want, tolerance)
	}
}
