package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

const (
	// pageSize is the maximum number of objects per page (verified against PeeringDB API).
	pageSize = 250

	// maxRetries is the maximum number of attempts per request.
	maxRetries = 3

	// userAgent identifies this client to the PeeringDB API.
	userAgent = "peeringdb-plus/1.0"
)

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
func NewClient(baseURL string, logger *slog.Logger, opts ...ClientOption) *Client {
	c := &Client{
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
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

// doWithRetry executes an HTTP GET with rate limiting and exponential
// backoff retry on transient errors (429, 500, 502, 503, 504).
// Non-retryable 4xx errors return immediately.
func (c *Client) doWithRetry(ctx context.Context, url string) (*http.Response, error) {
	tracer := otel.Tracer("peeringdb")
	var lastErr error

	for attempt := range maxRetries {
		// Honor context cancellation between retries.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}

		// Create per-attempt span as a child of the FetchAll span.
		// CRITICAL: use ctx (not attemptCtx) so attempts are siblings, not chained.
		attemptCtx, attemptSpan := tracer.Start(ctx, "peeringdb.request",
			trace.WithAttributes(
				attribute.Int("http.request.resend_count", attempt),
			),
		)

		// Wait for rate limiter.
		waitStart := time.Now()
		if err := c.limiter.Wait(attemptCtx); err != nil {
			attemptSpan.End()
			return nil, fmt.Errorf("rate limiter for %s: %w", url, err)
		}
		if waitDuration := time.Since(waitStart); waitDuration > time.Millisecond {
			attemptSpan.AddEvent("rate_limiter.wait",
				trace.WithAttributes(
					attribute.Float64("wait_duration_ms", float64(waitDuration.Milliseconds())),
				),
			)
		}

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
		if err != nil {
			attemptSpan.End()
			return nil, fmt.Errorf("create request for %s: %w", url, err)
		}
		req.Header.Set("User-Agent", userAgent)
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Api-Key "+c.apiKey)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// Network-level error -- may be context cancellation.
			attemptSpan.RecordError(err)
			attemptSpan.End()
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}

		// Success.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			attemptSpan.End()
			return resp, nil
		}

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
			attemptSpan.RecordError(authErr)
			attemptSpan.End()
			return nil, authErr
		}

		// Determine if retryable.
		if isRetryable(resp.StatusCode) {
			lastErr = fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
			attemptSpan.RecordError(lastErr)
			attemptSpan.End()
			if attempt < maxRetries-1 {
				delay := c.retryDelay(attempt)
				c.logger.LogAttrs(ctx, slog.LevelWarn, "retrying request",
					slog.String("url", url),
					slog.Int("status", resp.StatusCode),
					slog.Int("attempt", attempt+1),
					slog.Duration("delay", delay),
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
		attemptSpan.RecordError(nonRetryErr)
		attemptSpan.End()
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
