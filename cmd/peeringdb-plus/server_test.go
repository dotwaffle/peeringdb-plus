package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// TestServer_NoWriteTimeoutOnStreamingPaths is a signature-lock regression
// test for SEC-05. It asserts the production http.Server literal built by
// buildServer carries exact values for all four timeout fields, so any
// future refactor that "fixes" the deliberate WriteTimeout=0 (or drifts the
// other three values) fails CI.
//
// WriteTimeout must remain 0 because StreamEntities in
// internal/grpcserver/generic.go already enforces cfg.StreamTimeout per
// stream via context.WithTimeout; a server-wide WriteTimeout would race
// with it and silently truncate streams (see PITFALLS.md §CP-2).
func TestServer_NoWriteTimeoutOnStreamingPaths(t *testing.T) {
	t.Parallel()

	srv := buildServer(":0", http.NotFoundHandler(), nil)

	tests := []struct {
		name  string
		got   time.Duration
		want  time.Duration
		fatal string
	}{
		{
			name:  "WriteTimeout",
			got:   srv.WriteTimeout,
			want:  0,
			fatal: "WriteTimeout must be 0 — StreamEntities uses per-stream context deadlines (PITFALLS.md §CP-2). Do NOT 'fix' this.",
		},
		{
			name:  "ReadHeaderTimeout",
			got:   srv.ReadHeaderTimeout,
			want:  10 * time.Second,
			fatal: "ReadHeaderTimeout regression — slowloris header-stall mitigation must stay at 10s",
		},
		{
			name:  "ReadTimeout",
			got:   srv.ReadTimeout,
			want:  30 * time.Second,
			fatal: "ReadTimeout regression — slowloris body-stall mitigation requires 30s (SEC-05)",
		},
		{
			name:  "IdleTimeout",
			got:   srv.IdleTimeout,
			want:  120 * time.Second,
			fatal: "IdleTimeout regression — keep-alive idle cap must stay at 120s",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %v, want %v — %s", tc.name, tc.got, tc.want, tc.fatal)
			}
		})
	}
}

// TestServer_ReadTimeoutCutsOff exercises net/http's timeout enforcement
// against both slowloris attack shapes: header-stall (ReadHeaderTimeout)
// and body-stall (ReadTimeout). It uses an httptest.Server with scaled-down
// timeouts so the test completes quickly while still driving the same
// code paths as production.
//
// This is a behavioural regression lock against future removal of the
// timeouts; the hardcoded field assertions in
// TestServer_NoWriteTimeoutOnStreamingPaths are the primary value lock.
func TestServer_ReadTimeoutCutsOff(t *testing.T) {
	t.Parallel()

	const (
		headerTimeout = 200 * time.Millisecond
		readTimeout   = 500 * time.Millisecond
		// Total budget for observing the server-side close: well above
		// readTimeout but far below any CI watchdog.
		observeBudget = 2 * time.Second
	)

	newServer := func(t *testing.T) (addr string) {
		t.Helper()
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		ts.Config.ReadHeaderTimeout = headerTimeout
		ts.Config.ReadTimeout = readTimeout
		ts.Start()
		t.Cleanup(ts.Close)

		u, err := url.Parse(ts.URL)
		if err != nil {
			t.Fatalf("parse ts.URL: %v", err)
		}
		return u.Host
	}

	t.Run("header_stall", func(t *testing.T) {
		t.Parallel()
		addr := newServer(t)

		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		// Write a partial request line and a single header, but NOT
		// the terminating CRLF — net/http will sit in ReadHeaderTimeout.
		if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")); err != nil {
			t.Fatalf("write partial headers: %v", err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(observeBudget)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}

		start := time.Now()
		_, err = io.Copy(io.Discard, conn)
		elapsed := time.Since(start)

		// The server should close (EOF) well before the test budget.
		if !isServerClose(err) {
			t.Fatalf("expected EOF / connection close after header stall, got err=%v", err)
		}
		if elapsed >= observeBudget {
			t.Fatalf("server did not close within budget: elapsed=%v budget=%v", elapsed, observeBudget)
		}
		if elapsed < headerTimeout/2 {
			t.Fatalf("server closed suspiciously fast: elapsed=%v headerTimeout=%v", elapsed, headerTimeout)
		}
	})

	t.Run("body_stall", func(t *testing.T) {
		t.Parallel()
		addr := newServer(t)

		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		// Full headers with Content-Length: 100, then only 10 bytes of
		// body. net/http will hit ReadTimeout and close the connection.
		req := "POST / HTTP/1.1\r\n" +
			"Host: x\r\n" +
			"Content-Length: 100\r\n" +
			"Content-Type: application/octet-stream\r\n" +
			"\r\n" +
			"0123456789"
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("write partial body: %v", err)
		}

		if err := conn.SetReadDeadline(time.Now().Add(observeBudget)); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}

		// net/http may write a response (e.g. 408 Request Timeout) before
		// closing, or it may just close. Read to EOF either way via
		// bufio to keep the framing explicit.
		start := time.Now()
		_, err = io.Copy(io.Discard, bufio.NewReader(conn))
		elapsed := time.Since(start)

		if !isServerClose(err) {
			t.Fatalf("expected EOF / connection close after body stall, got err=%v", err)
		}
		if elapsed >= observeBudget {
			t.Fatalf("server did not close within budget: elapsed=%v budget=%v", elapsed, observeBudget)
		}
		if elapsed < readTimeout/2 {
			t.Fatalf("server closed suspiciously fast: elapsed=%v readTimeout=%v", elapsed, readTimeout)
		}
	})
}

// isServerClose returns true if err represents a normal server-initiated
// connection close: EOF, net.ErrClosed, or a successful completion (nil).
// A nil error is acceptable because io.Copy returns nil on EOF.
func isServerClose(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	// Some platforms surface ECONNRESET as a *net.OpError wrapping syscall.Errno.
	_, ok := errors.AsType[*net.OpError](err)
	return ok
}
