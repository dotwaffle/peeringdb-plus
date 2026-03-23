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
}

// NewClient creates a PeeringDB API client with the given base URL and
// logger. The client enforces a 20 req/min rate limit and a 30-second
// HTTP timeout.
func NewClient(baseURL string, logger *slog.Logger) *Client {
	return &Client{
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
}

// FetchAll pages through all objects of the given type, returning each
// as a json.RawMessage. It loops until the API returns an empty data
// array. Each request is rate-limited and retried on transient errors.
func (c *Client) FetchAll(ctx context.Context, objectType string) ([]json.RawMessage, error) {
	tracer := otel.Tracer("peeringdb")
	ctx, span := tracer.Start(ctx, "peeringdb.fetch/"+objectType)
	defer span.End()

	var all []json.RawMessage

	for skip := 0; ; skip += pageSize {
		page := skip / pageSize
		url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0", c.baseURL, objectType, pageSize, skip)

		resp, err := c.doWithRetry(ctx, url)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("fetch %s page %d: %w", objectType, page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("read %s page %d body: %w", objectType, page, err)
		}

		var apiResp Response[json.RawMessage]
		if err := json.Unmarshal(body, &apiResp); err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("decode %s page %d response: %w", objectType, page, err)
		}

		if len(apiResp.Data) == 0 {
			break
		}

		all = append(all, apiResp.Data...)

		span.AddEvent("page.fetched",
			trace.WithAttributes(
				attribute.Int("page", page),
				attribute.Int("count", len(apiResp.Data)),
				attribute.Int("running_total", len(all)),
			),
		)

		c.logger.LogAttrs(ctx, slog.LevelDebug, "fetched page",
			slog.String("type", objectType),
			slog.Int("page", page),
			slog.Int("count", len(apiResp.Data)),
			slog.Int("total", len(all)),
		)
	}

	return all, nil
}

// FetchType pages through all objects of the given type and unmarshals
// each into the concrete type T. Unknown JSON fields are silently
// ignored per D-08. This is a package-level function because Go does
// not allow type parameters on methods.
func FetchType[T any](ctx context.Context, c *Client, objectType string) ([]T, error) {
	raw, err := c.FetchAll(ctx, objectType)
	if err != nil {
		return nil, err
	}

	result := make([]T, 0, len(raw))
	for i, item := range raw {
		var v T
		if err := json.Unmarshal(item, &v); err != nil {
			return nil, fmt.Errorf("unmarshal %s item %d: %w", objectType, i, err)
		}
		result = append(result, v)
	}

	return result, nil
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
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

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
