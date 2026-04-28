package peeringdb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// Quick task 260428-2zl moved transport-level concerns out of doWithRetry into
// this http.RoundTripper wrapper:
//
//   1. Per-request rate-limit Wait, recorded as a histogram on
//      pdbplus.peeringdb.rate_limit_wait_ms (in addition to the existing span
//      event).
//   2. Bounded retry on HTTP 429 honoring Retry-After (numeric or HTTP-date).
//      The cap below prevents a hostile / misconfigured upstream from
//      stalling the sync goroutine for hours; if Retry-After exceeds the
//      cap, the transport returns *RateLimitError without sleeping —
//      preserving the pre-existing short-circuit for PeeringDB's
//      1-request-per-hour unauthenticated quota.
//   3. WAF detection on HTTP 403: response body is sniffed for the well-known
//      block signatures (AWS WAF, Cloudfront "Request blocked", generic
//      "Access Denied", Django's "<title>403 Forbidden</title>"). On match
//      the response headers are logged at WARN and the transport returns
//      a typed error WITHOUT triggering the auth-key retry. Non-WAF 403
//      responses pass through to the existing API-key validation path in
//      doWithRetry.
//   4. Per-status-class request counters and per-cause retry counters on
//      pdbplus.peeringdb.{requests,retries}.
//
// The doWithRetry method retains application-level retry on 5xx — the split
// keeps "transport plumbing" (limiter, 429, WAF) separate from "request
// orchestration" (5xx ladder, context honoring, header set), which matches
// the http.RoundTripper contract: one RoundTrip == one logical request from
// the caller's perspective, even if the transport retries internally.

// retryAfterCap bounds the sleep the transport will accept on 429.
// PeeringDB's unauth quota emits Retry-After: 2200 (~36 minutes) — far
// beyond anything we want to sleep inside a sync goroutine. With the cap,
// such headers fail fast back to the sync scheduler which retries on the
// next interval (1h unauth / 15m auth — both larger than the cap so the
// next scheduled tick lands AFTER the upstream window has cleared).
const retryAfterCap = 60 * time.Second

// transportMaxAttempts is the bounded retry count for in-transport 429
// retries. Keep symmetric with maxRetries (the application-level 5xx
// ladder in doWithRetry); 3 attempts feels right for both axes.
const transportMaxAttempts = 3

// wafSignatures is the closed set of body substrings that classify a 403
// as a WAF / IP-block rejection rather than an API-key auth failure.
// Sniff the first 4 KiB only — the body is restored via NopCloser before
// the response is returned to the caller, so doWithRetry can still drain
// it normally if it chooses to.
//
// Order matters for grep symmetry only — the matcher is a simple loop.
var wafSignatures = []string{
	"AWS WAF",
	"Request blocked",
	"Access Denied",
	"<title>403 Forbidden</title>",
}

// wafBodySniffLimit caps the body bytes read for WAF signature detection.
// 4 KiB comfortably covers the HTML pages emitted by the common WAFs
// (AWS WAF / Cloudfront / generic Apache 403 pages typically <2 KiB).
const wafBodySniffLimit = 4 * 1024

// errWAFBlocked is the sentinel returned to doWithRetry when WAF
// signatures are detected on a 403. It bypasses the API-key retry path
// because retrying within the same source IP is pointless against an
// IP-level block. Wrapped with %w to preserve errors.Is semantics.
var errWAFBlocked = errors.New("peeringdb: WAF block detected (HTTP 403)")

// rateLimitedTransport wraps an inner http.RoundTripper with the
// transport-level concerns documented at the top of this file. The
// limiter is held by pointer so SetRateLimit on the parent Client can
// hot-swap it for tests without a transport rebuild.
type rateLimitedTransport struct {
	inner   http.RoundTripper
	limiter *rate.Limiter
	logger  *slog.Logger
}

// newRateLimitedTransport constructs the wrapper. inner defaults to
// http.DefaultTransport if nil; limiter is required (panics on nil to
// surface mis-wiring at startup rather than at first request).
func newRateLimitedTransport(inner http.RoundTripper, limiter *rate.Limiter, logger *slog.Logger) *rateLimitedTransport {
	if inner == nil {
		inner = http.DefaultTransport
	}
	if limiter == nil {
		panic("peeringdb: newRateLimitedTransport requires non-nil limiter")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &rateLimitedTransport{inner: inner, limiter: limiter, logger: logger}
}

// RoundTrip implements http.RoundTripper. The limiter is consulted before
// every attempt (each retry counts as a separate request against the
// rate budget — an unauth 429 sleep + retry should NOT bypass the
// per-request quota). The 429 retry loop is bounded by transportMaxAttempts.
func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	span := trace.SpanFromContext(ctx)
	url := req.URL.String()

	var lastResp *http.Response
	var lastErr error
	for attempt := range transportMaxAttempts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if waitErr := t.waitLimiter(ctx, span, url); waitErr != nil {
			return nil, fmt.Errorf("rate limiter for %s: %w", url, waitErr)
		}

		// Clone the request per RoundTripper contract: a transport must
		// not mutate the caller's *http.Request, and on retry we need a
		// fresh request value (body re-reads are not relevant — the sync
		// fetch path issues GETs without bodies).
		attemptReq := req.Clone(ctx)
		resp, err := t.inner.RoundTrip(attemptReq)
		if err != nil {
			recordRequest(ctx, "network_error")
			return nil, err
		}
		recordRequest(ctx, classifyStatus(resp.StatusCode))

		// 429: parse Retry-After, bounded sleep + retry up to the cap.
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			// Drain + close before any sleep so the connection can be
			// reused by the next attempt's transport.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			// Short-circuit: header absent (== 0) or larger than cap →
			// surface the existing RateLimitError without burning more
			// quota. Header absent matches the pre-2zl behavior tested
			// by TestFetchAllShortCircuitsOn429NoHeader; > cap matches
			// the unauth 1/hr quota path tested by TestFetchAllShortCircuitsOn429.
			if retryAfter == 0 || retryAfter > retryAfterCap {
				rlErr := &RateLimitError{URL: url, RetryAfter: retryAfter, Status: resp.StatusCode}
				t.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB rate-limited, aborting (retry-after exceeds cap)",
					slog.String("url", url),
					slog.Duration("retry_after", retryAfter),
					slog.Duration("cap", retryAfterCap),
				)
				span.RecordError(rlErr)
				return nil, rlErr
			}

			// Within cap and retries remaining: sleep + retry.
			if attempt < transportMaxAttempts-1 {
				recordRetry(ctx, "429")
				t.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB 429, sleeping and retrying",
					slog.String("url", url),
					slog.Duration("retry_after", retryAfter),
					slog.Int("attempt", attempt+1),
				)
				select {
				case <-time.After(retryAfter):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				lastErr = &RateLimitError{URL: url, RetryAfter: retryAfter, Status: resp.StatusCode}
				continue
			}
			// Retries exhausted.
			rlErr := &RateLimitError{URL: url, RetryAfter: retryAfter, Status: resp.StatusCode}
			span.RecordError(rlErr)
			return nil, rlErr
		}

		// 403: classify as WAF (no retry; log headers) or pass through to
		// the API-key auth path in doWithRetry.
		if resp.StatusCode == http.StatusForbidden {
			body, drainErr := readAndRestoreBody(resp, wafBodySniffLimit)
			if drainErr != nil {
				return nil, drainErr
			}
			if matchesWAF(body) {
				t.logger.LogAttrs(ctx, slog.LevelWarn, "WAF block detected on 403; not retrying",
					slog.String("url", url),
					slog.Any("response_headers", resp.Header),
				)
				_ = resp.Body.Close()
				wrapped := fmt.Errorf("fetch %s: %w", url, errWAFBlocked)
				span.RecordError(wrapped)
				return nil, wrapped
			}
			// Non-WAF 403 → caller (doWithRetry) decides. Body has been
			// restored via NopCloser so doWithRetry can drain it.
			lastResp = resp
			return lastResp, nil
		}

		// Success or other status — return to caller. Application-level
		// retry on 5xx still happens in doWithRetry.
		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, fmt.Errorf("peeringdb transport: exhausted %d attempts on %s", transportMaxAttempts, url)
}

// waitLimiter blocks on the rate limiter and records the wait duration as
// both a span event (compat with the pre-2zl behavior) and a histogram
// (new in 2zl for dashboard visibility).
//
// Telemetry instruments are nil-guarded because unit tests run without
// calling InitMetrics() — and a nil-instrument call panics inside the
// OTel SDK. Production codepath always has the instruments populated
// (cmd/peeringdb-plus/main.go calls InitMetrics before NewWorker).
func (t *rateLimitedTransport) waitLimiter(ctx context.Context, span trace.Span, url string) error {
	waitStart := time.Now()
	if err := t.limiter.Wait(ctx); err != nil {
		return err
	}
	wait := time.Since(waitStart)
	if pdbotel.PeeringDBRateLimitWaitMS != nil {
		pdbotel.PeeringDBRateLimitWaitMS.Record(ctx, float64(wait.Milliseconds()))
	}
	if wait > time.Millisecond {
		span.AddEvent("rate_limiter.wait",
			trace.WithAttributes(
				attribute.Float64("wait_duration_ms", float64(wait.Milliseconds())),
				attribute.String("url", url),
			),
		)
	}
	return nil
}

// recordRequest emits the per-request status_class counter. Nil-guarded
// for the same reason as waitLimiter — tests run without InitMetrics().
func recordRequest(ctx context.Context, statusClass string) {
	if pdbotel.PeeringDBRequests == nil {
		return
	}
	pdbotel.PeeringDBRequests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status_class", statusClass),
	))
}

// recordRetry emits the per-retry cause counter. Nil-guarded as above.
func recordRetry(ctx context.Context, cause string) {
	if pdbotel.PeeringDBRetries == nil {
		return
	}
	pdbotel.PeeringDBRetries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("cause", cause),
	))
}

// classifyStatus buckets HTTP status codes for the requests counter.
func classifyStatus(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	}
	return "other"
}

// readAndRestoreBody reads up to limit bytes from resp.Body and replaces
// resp.Body with a NopCloser over a buffer that yields the read bytes
// followed by the rest of the original body. Returns the bytes read for
// caller inspection.
//
// On read error, resp.Body is closed and the error is returned — the
// caller should treat this as a transport failure.
func readAndRestoreBody(resp *http.Response, limit int64) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}
	buf := make([]byte, 0, limit)
	lr := io.LimitReader(resp.Body, limit)
	read, err := io.ReadAll(lr)
	if err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("read response body: %w", err)
	}
	buf = append(buf, read...)
	// Rewire body so callers see the full original stream: the bytes we
	// already read, followed by whatever remains.
	resp.Body = struct {
		io.Reader
		io.Closer
	}{
		Reader: io.MultiReader(bytes.NewReader(buf), resp.Body),
		Closer: resp.Body,
	}
	return buf, nil
}

// matchesWAF reports whether body contains any of the well-known WAF /
// IP-block signatures. Uses byte-substring matching against the closed
// wafSignatures set; case-sensitive by design (the canonical pages emit
// the exact strings the upstream products produce).
func matchesWAF(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	for _, sig := range wafSignatures {
		if bytes.Contains(body, []byte(sig)) {
			return true
		}
	}
	return false
}

// IsWAFBlocked reports whether err wraps the transport's WAF-block sentinel.
// Exposed for callers (sync worker) that want to distinguish WAF errors
// from generic 403 auth failures.
func IsWAFBlocked(err error) bool {
	return errors.Is(err, errWAFBlocked)
}

// quietContains is a tiny helper for tests to assert WAF logging without
// having to set up a full slog handler. Production code does not use it.
func quietContains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

var _ = quietContains // keep helper available for tests without unused-symbol lint noise
