package peeringdb

import (
	"log/slog"
	"net/http"
	"testing"
)

// TestNewClient_NoWholeRequestTimeout locks the transport timeout shape that
// replaced the former 30s http.Client.Timeout. A whole-request timeout is
// wrong for this client: it aborts a large full-sync stream mid-body and it
// truncates the transport's bounded 429 Retry-After wait (up to
// retryAfterCap), which downgrades a *RateLimitError — the worker's quota
// short-circuit signal — into a generic context-deadline error. Overall time
// is bounded by the caller's context deadline instead; a granular
// ResponseHeaderTimeout still guards against a server that never responds.
func TestNewClient_NoWholeRequestTimeout(t *testing.T) {
	t.Parallel()
	c := NewClient("https://example.invalid", slog.New(slog.DiscardHandler))

	if c.http.Timeout != 0 {
		t.Errorf("http.Client.Timeout = %v, want 0 (a whole-request timeout truncates full-sync streams and the Retry-After wait)", c.http.Timeout)
	}

	rt, ok := c.http.Transport.(*rateLimitedTransport)
	if !ok {
		t.Fatalf("Transport type = %T, want *rateLimitedTransport", c.http.Transport)
	}
	base, ok := rt.inner.(*http.Transport)
	if !ok {
		t.Fatalf("inner transport type = %T, want *http.Transport", rt.inner)
	}
	if base.ResponseHeaderTimeout <= 0 {
		t.Error("ResponseHeaderTimeout = 0: with the whole-request timeout removed, an unresponsive server would have no I/O bound")
	}
}
