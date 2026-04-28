package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/buildinfo"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
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
// package init via internal/buildinfo so tagged Docker builds emit
// `peeringdb-plus/v1.17`, post-tag dev builds emit `peeringdb-plus/v1.17-3-gabc1234`,
// and uninjected builds (go test, go run) emit short-sha or "unknown".
// Format follows the standard bot UA convention with a `+url` contact field.
var userAgent = "peeringdb-plus/" + buildinfo.Version() + " (+" + contactURL + ")"

// Client fetches data from the PeeringDB API with rate limiting,
// pagination, and retry logic.
type Client struct {
	http           *http.Client
	limiter        *rate.Limiter
	baseURL        string
	logger         *slog.Logger
	retryBaseDelay time.Duration
	apiKey         string
	// rps is the unauthenticated requests-per-second target. Captured
	// from WithRPS during options apply, then consumed by NewClient when
	// constructing the limiter. Authenticated path ignores this field.
	rps float64
}

// ClientOption configures optional Client behavior.
type ClientOption func(*Client)

// WithAPIKey sets the PeeringDB API key for authenticated requests.
// When set, requests include the Authorization header and the rate
// limiter increases to 60 req/min — overriding any WithRPS value.
// (The authenticated quota is fixed by upstream regardless of any
// operator preference; making this an override is the simplest way to
// preserve "auth → 1/sec" without re-deriving it from a float.)
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.apiKey = key
	}
}

// WithRPS sets the unauthenticated sustained requests-per-second cap.
// Quick task 260428-2zl: replaces the hardcoded 1/3s with a float knob
// driven by PDBPLUS_PEERINGDB_RPS. Burst stays at 1 — concurrent bursts
// against PeeringDB are not desirable. Authenticated clients (WithAPIKey)
// override this back to 60 req/min in NewClient — the upstream auth
// quota is fixed at 60/min regardless of operator preference.
//
// Values <= 0 are silently coerced to the default (2 RPS) so a misconfig
// in main.go cannot accidentally produce a zero-rate limiter that blocks
// every request forever.
func WithRPS(rps float64) ClientOption {
	return func(c *Client) {
		if rps > 0 {
			c.rps = rps
		}
	}
}

// defaultRPS is the unauthenticated rate-limit default when neither
// WithRPS nor WithAPIKey is supplied. 2 RPS is conservative against
// PeeringDB's anonymous ceiling and matches the post-2zl operator default
// (PDBPLUS_PEERINGDB_RPS=2.0).
const defaultRPS = 2.0

// NewClient creates a PeeringDB API client with the given base URL and
// logger. By default, the client enforces a 2 req/sec rate limit and a
// 30-second HTTP timeout. Use WithAPIKey to enable authenticated access
// with a higher 60 req/min rate limit; use WithRPS to override the
// unauthenticated default.
//
// Internal requests bypass otelhttp instrumentation to avoid span bloat
// during bulk syncs (PERF-08). Metrics and retries are still recorded
// via events on the parent stream span. The transport wrapper added in
// 260428-2zl handles per-request limiter wait, bounded 429 retry, and
// WAF detection on 403 — see internal/peeringdb/transport.go.
func NewClient(baseURL string, logger *slog.Logger, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:        baseURL,
		logger:         logger,
		retryBaseDelay: 2 * time.Second,
		rps:            defaultRPS,
	}
	for _, opt := range opts {
		opt(c)
	}
	// Construct limiter: auth path overrides RPS to the upstream-fixed
	// 60/min quota; unauth honors WithRPS / defaultRPS.
	if c.apiKey != "" {
		c.limiter = rate.NewLimiter(rate.Every(1*time.Second), 1) // 60 req/min authenticated
	} else {
		c.limiter = rate.NewLimiter(rate.Limit(c.rps), 1)
	}
	c.http = &http.Client{
		Timeout:   30 * time.Second,
		Transport: newRateLimitedTransport(http.DefaultTransport, c.limiter, logger),
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

// FetchRaw issues a single non-paginated request with the given query
// params and returns each top-level data element as a json.RawMessage
// clone. Quick task 260428-2zl Task 3 — used by the sync FK-backfill
// path (fetch one row at a time via ?since=1&id__in=N to recover
// missing parents from upstream before declaring an orphan).
//
// The request goes through doWithRetry (so it observes the rate
// limiter, the WAF detector, and the bounded 429 ladder). The decoder
// clones each raw element because json.Decoder reuses its internal
// buffer between Decode calls — without the clone, every slice entry
// would alias the same memory.
//
// Empty data → returns an empty slice (not nil) so callers can range
// over the result without a separate len-check.
func (c *Client) FetchRaw(ctx context.Context, objectType string, params url.Values) ([]json.RawMessage, error) {
	u := fmt.Sprintf("%s/api/%s", c.baseURL, objectType)
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.doWithRetry(ctx, u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var env struct {
		Data []json.RawMessage `json:"data"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&env); decErr != nil {
		return nil, fmt.Errorf("decode %s: %w", u, decErr)
	}
	out := make([]json.RawMessage, 0, len(env.Data))
	for _, raw := range env.Data {
		clone := make(json.RawMessage, len(raw))
		copy(clone, raw)
		out = append(out, clone)
	}
	return out, nil
}

// FetchByIDsBatchSize caps the number of IDs concatenated into a single
// ?id__in= query value so the assembled URL stays well under typical
// 8 KiB request-line limits. PeeringDB does not document an explicit
// id__in cardinality limit, but 100 IDs at ~7 characters each (decimal
// + comma) keeps the query string under 1 KiB with comfortable
// headroom for path + other params. Quick task 260428-5xt.
//
// Exported so the FK-backfill cap accounting (in internal/sync) can
// compute how many underlying HTTP requests a given FetchByIDs(ids)
// call will issue, without re-deriving the chunk size.
const FetchByIDsBatchSize = 100

// FetchByIDs issues one or more ?since=1&id__in=<csv> requests against
// the named object type and returns the concatenated raw rows in fetch
// order (chunk-1 rows then chunk-2 rows then chunk-3 rows). Each chunk
// of up to FetchByIDsBatchSize IDs becomes ONE HTTP request through the
// rate-limited transport (consuming ONE limiter token regardless of
// chunk size). 250 IDs => 3 sequential requests through the limiter.
//
// Quick task 260428-5xt — the FK-backfill dataloader path uses this
// to collapse N per-row HTTP requests (the old worst-case during
// truncate-and-resync recovery) into ⌈N/100⌉ batched requests, well
// under upstream's API_THROTTLE_REPEATED_REQUEST cap.
//
// Empty / nil ids returns (nil, nil) without issuing any HTTP request.
// On any chunk error, returns (nil, err) — partial results from prior
// chunks are NOT returned (callers treat this as a transport failure
// and fall back to drop-on-miss for the affected IDs).
//
// Order within a chunk follows upstream's response order; the function
// does NOT sort or deduplicate ids — callers are responsible for any
// upstream-side ordering they need.
func (c *Client) FetchByIDs(ctx context.Context, objectType string, ids []int) ([]json.RawMessage, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []json.RawMessage
	for start := 0; start < len(ids); start += FetchByIDsBatchSize {
		end := min(start+FetchByIDsBatchSize, len(ids))
		chunk := ids[start:end]
		// Build "1,2,3,…" without fmt.Sprintf allocations on the hot path.
		parts := make([]string, len(chunk))
		for i, id := range chunk {
			parts[i] = strconv.Itoa(id)
		}
		params := url.Values{}
		params.Set("since", "1")
		params.Set("id__in", strings.Join(parts, ","))
		raws, err := c.FetchRaw(ctx, objectType, params)
		if err != nil {
			return nil, fmt.Errorf("fetch %s id__in chunk [%d:%d]: %w", objectType, start, end, err)
		}
		out = append(out, raws...)
	}
	return out, nil
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

// doWithRetry executes an HTTP GET with application-level retry on 5xx.
// Quick task 260428-2zl: per-request rate-limit Wait, 429 handling, and
// WAF detection moved to the rateLimitedTransport wrapper installed in
// NewClient — doWithRetry now handles only application-level concerns
// (5xx ladder, auth failures, context honoring, header set).
//
// Redundant per-request spans are omitted (PERF-08); errors and
// rate-limiting events are recorded on the parent span (limiter waits)
// or on dedicated counters (per-status-class request counts).
func (c *Client) doWithRetry(ctx context.Context, url string) (*http.Response, error) {
	span := trace.SpanFromContext(ctx)
	var lastErr error

	for attempt := range maxRetries {
		// Honor context cancellation between retries.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("fetch %s: %w", url, err)
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
			// Wrap so callers can errors.Is for *RateLimitError /
			// errWAFBlocked propagated up from the transport. The
			// wrapping must use %w to preserve the chain.
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}

		// Success.
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}

		// Read and discard body so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		// Auth errors indicate invalid API key -- log and fail immediately.
		// Note: WAF-blocked 403 responses never reach here — the transport
		// short-circuits them with errWAFBlocked.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			c.logger.LogAttrs(ctx, slog.LevelWarn, "PeeringDB API key may be invalid",
				slog.Int("status", resp.StatusCode),
				slog.String("url", url),
			)
			authErr := fmt.Errorf("fetch %s: HTTP %d (API key may be invalid)", url, resp.StatusCode)
			return nil, authErr
		}

		// Determine if retryable. 429 is intentionally absent — the
		// transport handles it (with bounded sleep + Retry-After) and
		// returns either *RateLimitError (caught by the c.http.Do err
		// branch above) or a 200/4xx/5xx status the transport could not
		// resolve. Including 429 here would double-bounce the request.
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
				if pdbotel.PeeringDBRetries != nil {
					pdbotel.PeeringDBRetries.Add(ctx, 1, metric.WithAttributes(
						attribute.String("cause", "5xx"),
					))
				}
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

// isRetryable reports whether the HTTP status code warrants an
// application-level retry (5xx ladder in doWithRetry). 429 is excluded
// because the transport wrapper handles it with bounded Retry-After
// honoring — including 429 here would double-bounce the request through
// both layers.
func isRetryable(status int) bool {
	switch status {
	case http.StatusInternalServerError,
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
// Also rewires the transport wrapper's limiter pointer so the change
// takes effect on the very next request — without this, the transport
// would keep using the limiter captured at NewClient time and tests
// like TestFetchAllPagination (which set rate.Inf for fast iteration)
// would still see the default 2 RPS limit applied per request.
func (c *Client) SetRateLimit(limiter *rate.Limiter) {
	c.limiter = limiter
	if rt, ok := c.http.Transport.(*rateLimitedTransport); ok {
		rt.limiter = limiter
	}
}

// SetRetryBaseDelay overrides the default retry base delay. Intended for testing.
func (c *Client) SetRetryBaseDelay(d time.Duration) {
	c.retryBaseDelay = d
}
