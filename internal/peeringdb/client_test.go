package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/time/rate"
)

// makeOrgPage creates a JSON response with n Organization objects starting at the given ID offset.
func makeOrgPage(startID, count int) []byte {
	var items []json.RawMessage
	for i := 0; i < count; i++ {
		raw, _ := json.Marshal(map[string]any{
			"id":      startID + i,
			"name":    fmt.Sprintf("Org %d", startID+i),
			"created": "2020-01-01T00:00:00Z",
			"updated": "2020-01-01T00:00:00Z",
			"status":  "ok",
		})
		items = append(items, raw)
	}
	resp := map[string]any{
		"meta": map[string]any{},
		"data": items,
	}
	b, _ := json.Marshal(resp)
	return b
}

// emptyResponse returns a valid PeeringDB response with an empty data array.
func emptyResponse() []byte {
	return []byte(`{"meta": {}, "data": []}`)
}

func TestFetchAllPagination(t *testing.T) {
	t.Parallel()

	// Server returns 250 items on page 0, 100 on page 1, empty on page 2.
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := requestCount.Add(1)
		switch page {
		case 1:
			_, _ = w.Write(makeOrgPage(1, 250))
		case 2:
			_, _ = w.Write(makeOrgPage(251, 100))
		default:
			_, _ = w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	// Use a fast rate limiter for testing.
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 350 {
		t.Errorf("got %d items, want 350", len(result.Data))
	}

	// Verify we made exactly 3 requests (250, 100, empty).
	if got := requestCount.Load(); got != 3 {
		t.Errorf("made %d requests, want 3", got)
	}
}

func TestFetchAllRetryOn429(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempt.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
			return
		}
		// Third attempt succeeds, then second request gets empty.
		if n == 3 {
			_, _ = w.Write(makeOrgPage(1, 5))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond // Speed up tests.

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 5 {
		t.Errorf("got %d items, want 5", len(result.Data))
	}
}

func TestFetchAllRetryOn5xx(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempt.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if n == 3 {
			_, _ = w.Write(makeOrgPage(1, 3))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 3 {
		t.Errorf("got %d items, want 3", len(result.Data))
	}
}

func TestFetchAllMaxRetries(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("error should contain 'fetch', got: %v", err)
	}

	// Should have attempted exactly 3 times.
	if got := attempt.Load(); got != 3 {
		t.Errorf("made %d attempts, want 3", got)
	}
}

func TestFetchAllNoRetryOn4xx(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err == nil {
		t.Fatal("expected error on 404, got nil")
	}

	// Should have attempted exactly once (no retry on 404).
	if got := attempt.Load(); got != 1 {
		t.Errorf("made %d attempts, want 1", got)
	}
}

func TestFetchAllContextCancellation(t *testing.T) {
	t.Parallel()

	// Cancel context before calling FetchAll. The rate limiter's Wait
	// will return immediately with a context error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	client := NewClient("http://127.0.0.1:1", slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, err := client.FetchAll(ctx, TypeOrg)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}

	if !strings.Contains(err.Error(), "context") {
		t.Errorf("error should mention context, got: %v", err)
	}
}

func TestFetchAllDepthZero(t *testing.T) {
	t.Parallel()

	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, err := client.FetchAll(context.Background(), TypeNet)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if !strings.Contains(capturedURL, "depth=0") {
		t.Errorf("URL should contain depth=0, got: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "limit=250") {
		t.Errorf("URL should contain limit=250, got: %s", capturedURL)
	}
	if !strings.Contains(capturedURL, "/api/net") {
		t.Errorf("URL should contain /api/net, got: %s", capturedURL)
	}
}

func TestFetchAllEmptyFirstPage(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypePoc)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 0 {
		t.Errorf("got %d items, want 0", len(result.Data))
	}

	if got := requestCount.Load(); got != 1 {
		t.Errorf("made %d requests, want 1", got)
	}
}

func TestFetchAllAccumulatesAllPages(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		page := requestCount.Add(1)
		switch page {
		case 1:
			_, _ = w.Write(makeOrgPage(1, 250))
		case 2:
			_, _ = w.Write(makeOrgPage(251, 250))
		case 3:
			_, _ = w.Write(makeOrgPage(501, 50))
		default:
			_, _ = w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 550 {
		t.Errorf("got %d items, want 550", len(result.Data))
	}
}

func TestFetchAllRateLimiter(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n <= 3 {
			_, _ = w.Write(makeOrgPage(int(n)*10, 10))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	// Set a moderate rate limit so we can verify timing.
	client.limiter.SetLimit(10) // 10 per second
	client.limiter.SetBurst(1)

	start := time.Now()
	result, err := client.FetchAll(context.Background(), TypeOrg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 30 {
		t.Errorf("got %d items, want 30", len(result.Data))
	}

	// With 4 requests at 10/sec (burst 1), we need at least ~300ms.
	if elapsed < 200*time.Millisecond {
		t.Errorf("completed in %v, expected rate limiting to slow it down", elapsed)
	}
}

func TestFetchAllUnknownFieldsIgnored(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			_, _ = w.Write([]byte(`{
				"meta": {},
				"data": [
					{
						"id": 1,
						"name": "Test",
						"brand_new_field": "ignored",
						"another_unknown": 42
					}
				]
			}`))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	// FetchAll returns json.RawMessage, so unknown fields are always preserved.
	// The key test is that the client doesn't error on unknown JSON fields.
	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(result.Data) != 1 {
		t.Errorf("got %d items, want 1", len(result.Data))
	}
}

func TestFetchTypeDeserialization(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			_, _ = w.Write([]byte(`{
				"meta": {},
				"data": [
					{
						"id": 1,
						"name": "Test Org",
						"aka": "",
						"name_long": "",
						"website": "",
						"social_media": [],
						"notes": "",
						"logo": null,
						"address1": "",
						"address2": "",
						"city": "Berlin",
						"state": "",
						"country": "DE",
						"zipcode": "",
						"suite": "",
						"floor": "",
						"latitude": 52.52,
						"longitude": 13.405,
						"created": "2020-01-01T00:00:00Z",
						"updated": "2020-01-01T00:00:00Z",
						"status": "ok"
					}
				]
			}`))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	orgs, err := FetchType[Organization](context.Background(), client, TypeOrg)
	if err != nil {
		t.Fatalf("FetchType: %v", err)
	}

	if len(orgs) != 1 {
		t.Fatalf("got %d orgs, want 1", len(orgs))
	}

	if orgs[0].City != "Berlin" {
		t.Errorf("City = %q, want %q", orgs[0].City, "Berlin")
	}
	if orgs[0].Country != "DE" {
		t.Errorf("Country = %q, want %q", orgs[0].Country, "DE")
	}
}

func TestUserAgent(t *testing.T) {
	t.Parallel()

	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, _ = client.FetchAll(context.Background(), TypeOrg)

	if capturedUA != "peeringdb-plus/1.0" {
		t.Errorf("User-Agent = %q, want %q", capturedUA, "peeringdb-plus/1.0")
	}
}

// setupTraceTest configures an in-memory span exporter as the global
// TracerProvider and returns it for span inspection. The provider is
// shut down automatically via t.Cleanup.
func setupTraceTest(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exporter
}

// findSpanByName returns the first span with the given name, or nil.
func findSpanByName(spans tracetest.SpanStubs, name string) *tracetest.SpanStub {
	for i := range spans {
		if spans[i].Name == name {
			return &spans[i]
		}
	}
	return nil
}

// findSpansByName returns all spans with the given name.
func findSpansByName(spans tracetest.SpanStubs, name string) []tracetest.SpanStub {
	var result []tracetest.SpanStub
	for _, s := range spans {
		if s.Name == name {
			result = append(result, s)
		}
	}
	return result
}

func TestFetchAllCreatesSpanHierarchy(t *testing.T) {
	// Not parallel: mutates global TracerProvider.
	exporter := setupTraceTest(t)

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			_, _ = w.Write(makeOrgPage(1, 5))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), "net")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(result.Data) != 5 {
		t.Errorf("got %d items, want 5", len(result.Data))
	}

	spans := exporter.GetSpans()

	// Verify parent span exists.
	fetchSpan := findSpanByName(spans, "peeringdb.fetch/net")
	if fetchSpan == nil {
		t.Fatal("expected peeringdb.fetch/net span, not found")
	}

	// Verify at least one request span exists.
	requestSpans := findSpansByName(spans, "peeringdb.request")
	if len(requestSpans) == 0 {
		t.Fatal("expected at least one peeringdb.request span, found none")
	}

	// Verify request spans are children of the fetch span.
	for _, rs := range requestSpans {
		if rs.Parent.SpanID() != fetchSpan.SpanContext.SpanID() {
			t.Errorf("peeringdb.request span parent=%s, want %s (peeringdb.fetch/net)",
				rs.Parent.SpanID(), fetchSpan.SpanContext.SpanID())
		}
	}

	// Verify resend_count attribute on first request span.
	found := false
	for _, attr := range requestSpans[0].Attributes {
		if attr.Key == "http.request.resend_count" && attr.Value == attribute.IntValue(0) {
			found = true
			break
		}
	}
	if !found {
		t.Error("first peeringdb.request span missing http.request.resend_count=0 attribute")
	}
}

func TestFetchAllRecordsPageEvents(t *testing.T) {
	// Not parallel: mutates global TracerProvider.
	exporter := setupTraceTest(t)

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		switch n {
		case 1:
			_, _ = w.Write(makeOrgPage(1, 250))
		case 2:
			_, _ = w.Write(makeOrgPage(251, 50))
		default:
			_, _ = w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), "org")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(result.Data) != 300 {
		t.Errorf("got %d items, want 300", len(result.Data))
	}

	spans := exporter.GetSpans()
	fetchSpan := findSpanByName(spans, "peeringdb.fetch/org")
	if fetchSpan == nil {
		t.Fatal("expected peeringdb.fetch/org span, not found")
	}

	// Count page.fetched events.
	var pageFetchedCount int
	for _, evt := range fetchSpan.Events {
		if evt.Name == "page.fetched" {
			pageFetchedCount++
		}
	}
	if pageFetchedCount < 2 {
		t.Fatalf("expected at least 2 page.fetched events, got %d", pageFetchedCount)
	}

	// Verify first page event attributes.
	firstEvt := fetchSpan.Events[0]
	if firstEvt.Name != "page.fetched" {
		t.Fatalf("first event name=%q, want page.fetched", firstEvt.Name)
	}
	assertEventAttr(t, firstEvt.Attributes, "page", attribute.IntValue(0))
	assertEventAttr(t, firstEvt.Attributes, "count", attribute.IntValue(250))

	// Verify second page event attributes.
	secondEvt := fetchSpan.Events[1]
	if secondEvt.Name != "page.fetched" {
		t.Fatalf("second event name=%q, want page.fetched", secondEvt.Name)
	}
	assertEventAttr(t, secondEvt.Attributes, "page", attribute.IntValue(1))
	assertEventAttr(t, secondEvt.Attributes, "count", attribute.IntValue(50))
}

// assertEventAttr checks that the given attributes contain a key with the expected value.
func assertEventAttr(t *testing.T, attrs []attribute.KeyValue, key string, want attribute.Value) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if a.Value != want {
				t.Errorf("attribute %s = %v, want %v", key, a.Value, want)
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}

func TestDoWithRetryCreatesPerAttemptSpans(t *testing.T) {
	// Not parallel: mutates global TracerProvider.
	exporter := setupTraceTest(t)

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempt.Add(1)
		switch n {
		case 1:
			// First request for page 0: return 429.
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
		case 2:
			// Second request for page 0: succeed.
			_, _ = w.Write(makeOrgPage(1, 5))
		default:
			// Page 1: empty (end pagination).
			_, _ = w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	result, err := client.FetchAll(context.Background(), "org")
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(result.Data) != 5 {
		t.Errorf("got %d items, want 5", len(result.Data))
	}

	spans := exporter.GetSpans()
	requestSpans := findSpansByName(spans, "peeringdb.request")

	// Expect at least 3 request spans: 2 for page 0 (429 then 200), 1 for page 1 (empty).
	if len(requestSpans) < 3 {
		t.Fatalf("expected at least 3 peeringdb.request spans, got %d", len(requestSpans))
	}

	// Verify first attempt has resend_count=0, second has resend_count=1.
	first := requestSpans[0]
	second := requestSpans[1]

	assertSpanAttr(t, first.Attributes, "http.request.resend_count", attribute.IntValue(0))
	assertSpanAttr(t, second.Attributes, "http.request.resend_count", attribute.IntValue(1))
}

// assertSpanAttr checks that the given attributes contain a key with the expected value.
func assertSpanAttr(t *testing.T, attrs []attribute.KeyValue, key string, want attribute.Value) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if a.Value != want {
				t.Errorf("span attribute %s = %v, want %v", key, a.Value, want)
			}
			return
		}
	}
	t.Errorf("span attribute %s not found", key)
}

func TestWithAPIKeyHeader(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default(), WithAPIKey("my-secret-key"))
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if capturedAuth != "Api-Key my-secret-key" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Api-Key my-secret-key")
	}
}

func TestNoAPIKeyNoHeader(t *testing.T) {
	t.Parallel()

	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if capturedAuth != "" {
		t.Errorf("Authorization = %q, want empty (no API key)", capturedAuth)
	}
}

func TestAuthenticatedRateLimit(t *testing.T) {
	t.Parallel()

	client := NewClient("http://127.0.0.1:1", slog.Default(), WithAPIKey("key"))
	// Authenticated client should use 1 req/sec (rate.Every(1*time.Second)).
	wantLimit := rate.Every(1 * time.Second)
	if client.limiter.Limit() != wantLimit {
		t.Errorf("authenticated limiter rate = %v, want %v", client.limiter.Limit(), wantLimit)
	}
}

func TestUnauthenticatedRateLimit(t *testing.T) {
	t.Parallel()

	client := NewClient("http://127.0.0.1:1", slog.Default())
	// Unauthenticated client should use 1 req/3sec (rate.Every(3*time.Second)).
	wantLimit := rate.Every(3 * time.Second)
	if client.limiter.Limit() != wantLimit {
		t.Errorf("unauthenticated limiter rate = %v, want %v", client.limiter.Limit(), wantLimit)
	}
}

func TestAuthErrorNotRetried_401(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid API key"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default(), WithAPIKey("bad-key"))
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain '401', got: %v", err)
	}
	if !strings.Contains(err.Error(), "API key may be invalid") {
		t.Errorf("error should contain 'API key may be invalid', got: %v", err)
	}

	// Should have attempted exactly once (no retry).
	if got := attempt.Load(); got != 1 {
		t.Errorf("made %d attempts, want 1", got)
	}
}

func TestAuthErrorNotRetried_403(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"Forbidden"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default(), WithAPIKey("bad-key"))
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	_, err := client.FetchAll(context.Background(), TypeOrg)
	if err == nil {
		t.Fatal("expected error on 403, got nil")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should contain '403', got: %v", err)
	}
	if !strings.Contains(err.Error(), "API key may be invalid") {
		t.Errorf("error should contain 'API key may be invalid', got: %v", err)
	}

	// Should have attempted exactly once (no retry).
	if got := attempt.Load(); got != 1 {
		t.Errorf("made %d attempts, want 1", got)
	}
}

func TestNewClientBackwardCompatible(t *testing.T) {
	t.Parallel()

	// NewClient without options should compile and work without panic.
	client := NewClient("http://127.0.0.1:1", slog.Default())
	if client.apiKey != "" {
		t.Errorf("apiKey = %q, want empty", client.apiKey)
	}
}

// makeOrgPageWithMeta creates a JSON response with n Organization objects and a meta.generated epoch.
func makeOrgPageWithMeta(startID, count int, generated float64) []byte {
	var items []json.RawMessage
	for i := 0; i < count; i++ {
		raw, _ := json.Marshal(map[string]any{
			"id":      startID + i,
			"name":    fmt.Sprintf("Org %d", startID+i),
			"created": "2020-01-01T00:00:00Z",
			"updated": "2020-01-01T00:00:00Z",
			"status":  "ok",
		})
		items = append(items, raw)
	}
	resp := map[string]any{
		"meta": map[string]any{"generated": generated},
		"data": items,
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestFetchAllWithSince(t *testing.T) {
	t.Parallel()

	sinceTime := time.Unix(1711234567, 0)
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	_, err := client.FetchAll(context.Background(), TypeOrg, WithSince(sinceTime))
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	expected := fmt.Sprintf("since=%d", sinceTime.Unix())
	if !strings.Contains(capturedURL, expected) {
		t.Errorf("URL should contain %s, got: %s", expected, capturedURL)
	}
}

func TestFetchMetaParsing(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			_, _ = w.Write(makeOrgPageWithMeta(1, 5, 1711234567.0))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	want := time.Unix(1711234567, 0)
	if !result.Meta.Generated.Equal(want) {
		t.Errorf("Meta.Generated = %v, want %v", result.Meta.Generated, want)
	}
}

func TestFetchMetaMissing(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			_, _ = w.Write(makeOrgPage(1, 3))
			return
		}
		_, _ = w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if !result.Meta.Generated.IsZero() {
		t.Errorf("Meta.Generated should be zero, got %v", result.Meta.Generated)
	}
}

func TestFetchMetaEarliestAcrossPages(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := requestCount.Add(1)
		switch n {
		case 1:
			// Page 1: newer generated timestamp
			_, _ = w.Write(makeOrgPageWithMeta(1, 250, 1711234567.0))
		case 2:
			// Page 2: older generated timestamp (this should be used)
			_, _ = w.Write(makeOrgPageWithMeta(251, 50, 1711234500.0))
		default:
			_, _ = w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	result, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	want := time.Unix(1711234500, 0)
	if !result.Meta.Generated.Equal(want) {
		t.Errorf("Meta.Generated = %v, want %v (should use earliest)", result.Meta.Generated, want)
	}
}
