package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// RateLimitError is returned when PeeringDB responds with HTTP 429 Too Many
// Requests. It carries the Retry-After delay parsed from the response header
// so upstream callers (sync worker retry loops) can short-circuit their own
// backoff ladders instead of burning more of our rate-limited quota on retries
// that are guaranteed to 429 again.
//
// Background: PeeringDB enforces 1 request per distinct query-string per hour
// for unauthenticated clients. Their 429 response includes a Retry-After
// header in seconds (e.g. "Retry-After: 2200" = 36m40s). The sync worker's
// default retry backoff ladder (30s, 2m, 8m) all fall well inside that window,
// so every retry within a single sync cycle is doomed — and each one consumes
// another slot against the hourly quota.
type RateLimitError struct {
	// URL is the request URL that was rate-limited.
	URL string
	// RetryAfter is the delay parsed from the Retry-After response header.
	// Zero if the header was absent or unparseable.
	RetryAfter time.Duration
	// Status is always http.StatusTooManyRequests for RateLimitError.
	Status int
}

// Error implements the error interface.
func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("fetch %s: HTTP %d (rate-limited, retry after %s)",
			e.URL, e.Status, e.RetryAfter)
	}
	return fmt.Sprintf("fetch %s: HTTP %d (rate-limited)", e.URL, e.Status)
}

// parseRetryAfter parses an HTTP Retry-After header value into a duration.
// Per RFC 7231 §7.1.3, Retry-After is either a non-negative integer number of
// seconds ("120") or an HTTP date ("Fri, 31 Dec 1999 23:59:59 GMT"). Returns
// zero duration if the header is empty or cannot be parsed — callers should
// treat zero as "header absent, use default backoff".
//
// The `now` argument is injectable for deterministic testing of the HTTP-date
// path; production callers pass time.Now().
func parseRetryAfter(header string, now time.Time) time.Duration {
	if header == "" {
		return 0
	}
	// Integer seconds (the common case — PeeringDB uses this form).
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	// HTTP date (the uncommon case — still RFC-valid so honor it).
	if t, err := http.ParseTime(header); err == nil {
		delta := t.Sub(now)
		if delta < 0 {
			return 0
		}
		return delta
	}
	return 0
}

const (
	// pageSize is the maximum number of objects per page (verified against PeeringDB API).
	pageSize = 250

	// maxRetries is the maximum number of attempts per request.
	maxRetries = 3

	// contactURL is the abuse / rate-limit contact landing page included in
	// the User-Agent. Hosted on the project's public GitHub so PeeringDB ops
	// have somewhere to file an issue if our traffic ever misbehaves.
	contactURL = "https://github.com/dotwaffle/peeringdb-plus"
)

// userAgent identifies this client to the PeeringDB API. Resolved once at
// package init via runtime/debug.ReadBuildInfo so tagged releases emit
// `peeringdb-plus/v1.16` and dev builds emit `peeringdb-plus/<short-sha>`.
// Format follows the standard bot UA convention with a `+url` contact field.
var userAgent = "peeringdb-plus/" + buildVersion() + " (+" + contactURL + ")"

// buildVersion returns the module version (for tagged builds) or the short
// VCS revision (for dev builds), mirroring internal/otel/provider.go's
// resolution logic so OTel resource and the User-Agent stay in lockstep.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return "unknown"
}

// Client fetches data from the PeeringDB API with rate limiting,
// pagination, and retry logic.
type Client struct {
	http           *http.Client
	limiter        *rate.Limiter
	baseURL        string
	logger         *slog.Logger
	retryBaseDelay time.Duration
	apiKey         string
}

// ClientOption configures optional Client behavior.
type ClientOption func(*Client)

// WithAPIKey sets the PeeringDB API key for authenticated requests.
// When set, requests include the Authorization header and the rate
// limiter increases from 20 req/min to 60 req/min.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// NewClient creates a PeeringDB API client with the given base URL and
// logger. By default, the client enforces a 20 req/min rate limit and a
// 30-second HTTP timeout. Use WithAPIKey to enable authenticated access
// with a higher 60 req/min rate limit.
//
// Internal requests bypass otelhttp instrumentation to avoid span bloat
// during bulk syncs (PERF-08). Metrics and retries are still recorded
// via events on the parent stream span.
func NewClient(baseURL string, logger *slog.Logger, opts ...ClientOption) *Client {
	c := &Client{
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: http.DefaultTransport,
		},
		// 20 requests per minute = 1 request per 3 seconds.
		limiter:        rate.NewLimiter(rate.Every(3*time.Second), 1),
		baseURL:        baseURL,
		logger:         logger,
		retryBaseDelay: 2 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.apiKey != "" {
		c.limiter = rate.NewLimiter(rate.Every(1*time.Second), 1) // 60 req/min authenticated
	}
	return c
}

// FetchOption configures optional FetchAll behavior.
type FetchOption func(*fetchConfig)

type fetchConfig struct {
	since time.Time
}

// WithSince sets the ?since= parameter for delta fetches.
// Only objects modified after the given time are returned.
func WithSince(t time.Time) FetchOption {
	return func(c *fetchConfig) {
		c.since = t
	}
}

// FetchMeta contains metadata from PeeringDB API responses.
type FetchMeta struct {
	// Generated is the earliest meta.generated epoch across all pages.
	// Zero value means no generated timestamp was found.
	Generated time.Time
}

// FetchResult contains the fetched data and response metadata.
type FetchResult struct {
	Data []json.RawMessage
	Meta FetchMeta
}

// parseMeta extracts the generated epoch from a PeeringDB API response meta field.
// Returns zero time if meta is empty or generated field is absent.
func parseMeta(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var meta struct {
		Generated float64 `json:"generated"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil || meta.Generated == 0 {
		return time.Time{}
	}
	return time.Unix(int64(meta.Generated), 0)
}

// FetchAll pages through all objects of the given type, collecting each
// element as a json.RawMessage in a FetchResult. It is a thin wrapper
// over StreamAll — new callers should prefer StreamAll directly to avoid
// the allocation of a full []json.RawMessage slice. FetchAll exists for
// the pdbcompat-check CLI and the conformance test, which consume the
// full result set as a batch.
//
// The handler clones each raw message because json.Decoder reuses its
// internal buffer across Decode calls — without the clone, every slice
// entry would alias the same memory.
func (c *Client) FetchAll(ctx context.Context, objectType string, opts ...FetchOption) (FetchResult, error) {
	var data []json.RawMessage
	handler := func(raw json.RawMessage) error {
		clone := make(json.RawMessage, len(raw))
		copy(clone, raw)
		data = append(data, clone)
		return nil
	}
	meta, err := c.StreamAll(ctx, objectType, handler, opts...)
	if err != nil {
		return FetchResult{}, err
	}
	return FetchResult{Data: data, Meta: meta}, nil
}

// FetchType streams objects of the given type and unmarshals each directly
// into the concrete type T. Unknown JSON fields are silently ignored per
// D-08. This is a package-level function because Go does not allow type
// parameters on methods. Unlike FetchAll, FetchType does not clone the
// raw bytes — each element is decoded into a fresh T immediately and the
// underlying decoder buffer can be reused for the next element.
func FetchType[T any](ctx context.Context, c *Client, objectType string, opts ...FetchOption) ([]T, error) {
	var items []T
	handler := func(raw json.RawMessage) error {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("unmarshal %s item %d: %w", objectType, len(items), err)
		}
		items = append(items, v)
		return nil
	}
	if _, err := c.StreamAll(ctx, objectType, handler, opts...); err != nil {
		return nil, err
	}
	return items, nil
}

// FetchRawPage fetches a single page of objects for the given type and
// returns the raw response bytes verbatim. Unlike FetchAll (which pages
// internally and parses), this is intended for callers that write
// byte-for-byte fixtures — see internal/visbaseline.
//
// page is 1-based. Page 1 -> skip=0, page 2 -> skip=250, etc.
//
// FetchRawPage reuses the client's rate limiter, API key, and 429
// short-circuit from doWithRetry. On 429, callers get a *RateLimitError
// back; they MUST sleep for RetryAfter before re-calling. Do not retry
// inside this method — the short-circuit at client.go:312-327 is
// intentional to avoid burning quota on failed retries.
func (c *Client) FetchRawPage(ctx context.Context, objectType string, page int) ([]byte, error) {
	if page < 1 {
		return nil, fmt.Errorf("FetchRawPage: page must be >= 1, got %d", page)
	}
	skip := (page - 1) * pageSize
	url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0",
		c.baseURL, objectType, pageSize, skip)

	resp, err := c.doWithRetry(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s page %d body: %w", objectType, page, err)
	}
	return body, nil
}

// doWithRetry executes an HTTP GET with rate limiting and exponential
// backoff retry on transient errors (429, 500, 502, 503, 504).
// Non-retryable 4xx errors return immediately.
//
// Redundant per-request spans are omitted (PERF-08); errors and
// rate-limiting events are recorded on the parent span.
func (c *Client) doWithRetry(ctx context.Context, url string) (*http.Response, error) {
	span := trace.SpanFromContext(ctx)
	var lastErr error

	for attempt := range maxRetries {
		// Honor context cancellation between retries.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}

		// Wait for rate limiter.
		waitStart := time.Now()
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter for %s: %w", url, err)
		}
		if waitDuration := time.Since(waitStart); waitDuration > time.Millisecond {
			span.AddEvent("rate_limiter.wait",
				trace.WithAttributes(
					attribute.Float64("wait_duration_ms", float64(waitDuration.Milliseconds())),
					attribute.String("url", url),
				),
			)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request for %s: %w", url, err)
		}
		req.Header.Set("User-Agent", userAgent)
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Api-Key "+c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// Network-level error -- may be context cancellation.
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}

		// Success.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// Capture Retry-After before draining the body — it's a response
		// header so body reads don't affect it, but we read it here to keep
		// the 429 short-circuit path below self-contained.
		retryAfterHeader := resp.Header.Get("Retry-After")

		// Read and discard body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		// Auth errors indicate invalid API key -- log and fail immediately.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB API key may be invalid",
				slog.Int("status", resp.StatusCode),
				slog.String("url", url),
			)
			authErr := fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", url, resp.StatusCode)
			return nil, authErr
		}

		// Rate limit short-circuit: PeeringDB's 1 req/hr unauth limit means
		// the within-request 2s/8s retry ladder is pure waste — every retry
		// lands inside the Retry-After window and burns another slot against
		// our quota. Parse Retry-After, return a typed RateLimitError, and
		// let the sync-level caller decide whether to back off further.
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfter(retryAfterHeader, time.Now())
			rlErr := &RateLimitError{
				URL:        url,
				RetryAfter: retryAfter,
				Status:     resp.StatusCode,
			}
			c.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB rate-limited, aborting request retries",
				slog.String("url", url),
				slog.Int("status", resp.StatusCode),
				slog.Duration("retry_after", retryAfter),
			)
			span.RecordError(rlErr)
			return nil, rlErr
		}

		// Determine if retryable.
		if isRetryable(resp.StatusCode) {
			lastErr = fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
			if attempt < maxRetries-1 {
				delay := c.retryDelay(attempt)
				c.logger.LogAttrs(ctx, slog.LevelWarn, "retrying request",
					slog.String("url", url),
					slog.Int("status", resp.StatusCode),
					slog.Int("attempt", attempt+1),
					slog.Duration("delay", delay),
				)
				span.AddEvent("request.retry",
					trace.WithAttributes(
						attribute.Int("attempt", attempt+1),
						attribute.Int("status", resp.StatusCode),
						attribute.String("url", url),
					),
				)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return nil, fmt.Errorf("fetch %s: %w", url, ctx.Err())
				}
			}
			continue
		}

		// Non-retryable error.
		nonRetryErr := fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
		return nil, nonRetryErr
	}

	return nil, lastErr
}

// isRetryable reports whether the HTTP status code warrants a retry.
func isRetryable(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryDelay calculates the backoff delay for the given attempt number.
// Base delay is 2s with a 4x multiplier: 2s, 8s, 32s.
func (c *Client) retryDelay(attempt int) time.Duration {
	delay := c.retryBaseDelay
	for range attempt {
		delay *= 4
	}
	return delay
}

// SetRateLimit overrides the default rate limiter. Intended for testing.
func (c *Client) SetRateLimit(limiter *rate.Limiter) {
	c.limiter = limiter
}

// SetRetryBaseDelay overrides the default retry base delay. Intended for testing.
func (c *Client) SetRetryBaseDelay(d time.Duration) {
	c.retryBaseDelay = d
}
