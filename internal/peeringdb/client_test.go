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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := requestCount.Add(1)
		switch page {
		case 1:
			w.Write(makeOrgPage(1, 250))
		case 2:
			w.Write(makeOrgPage(251, 100))
		default:
			w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	// Use a fast rate limiter for testing.
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	items, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 350 {
		t.Errorf("got %d items, want 350", len(items))
	}

	// Verify we made exactly 3 requests (250, 100, empty).
	if got := requestCount.Load(); got != 3 {
		t.Errorf("made %d requests, want 3", got)
	}
}

func TestFetchAllRetryOn429(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempt.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"detail":"Rate limit exceeded"}`))
			return
		}
		// Third attempt succeeds, then second request gets empty.
		if n == 3 {
			w.Write(makeOrgPage(1, 5))
			return
		}
		w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond // Speed up tests.

	items, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 5 {
		t.Errorf("got %d items, want 5", len(items))
	}
}

func TestFetchAllRetryOn5xx(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempt.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if n == 3 {
			w.Write(makeOrgPage(1, 3))
			return
		}
		w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)
	client.retryBaseDelay = 1 * time.Millisecond

	items, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("got %d items, want 3", len(items))
	}
}

func TestFetchAllMaxRetries(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Write(emptyResponse())
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	items, err := client.FetchAll(context.Background(), TypePoc)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 0 {
		t.Errorf("got %d items, want 0", len(items))
	}

	if got := requestCount.Load(); got != 1 {
		t.Errorf("made %d requests, want 1", got)
	}
}

func TestFetchAllAccumulatesAllPages(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := requestCount.Add(1)
		switch page {
		case 1:
			w.Write(makeOrgPage(1, 250))
		case 2:
			w.Write(makeOrgPage(251, 250))
		case 3:
			w.Write(makeOrgPage(501, 50))
		default:
			w.Write(emptyResponse())
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	items, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 550 {
		t.Errorf("got %d items, want 550", len(items))
	}
}

func TestFetchAllRateLimiter(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n <= 3 {
			w.Write(makeOrgPage(int(n)*10, 10))
			return
		}
		w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	// Set a moderate rate limit so we can verify timing.
	client.limiter.SetLimit(10) // 10 per second
	client.limiter.SetBurst(1)

	start := time.Now()
	items, err := client.FetchAll(context.Background(), TypeOrg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 30 {
		t.Errorf("got %d items, want 30", len(items))
	}

	// With 4 requests at 10/sec (burst 1), we need at least ~300ms.
	if elapsed < 200*time.Millisecond {
		t.Errorf("completed in %v, expected rate limiting to slow it down", elapsed)
	}
}

func TestFetchAllUnknownFieldsIgnored(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			w.Write([]byte(`{
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
		w.Write(emptyResponse())
	}))
	defer server.Close()

	client := NewClient(server.URL, slog.Default())
	client.limiter.SetLimit(1000)
	client.limiter.SetBurst(1000)

	// FetchAll returns json.RawMessage, so unknown fields are always preserved.
	// The key test is that the client doesn't error on unknown JSON fields.
	items, err := client.FetchAll(context.Background(), TypeOrg)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("got %d items, want 1", len(items))
	}
}

func TestFetchTypeDeserialization(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			w.Write([]byte(`{
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
		w.Write(emptyResponse())
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
		w.Write(emptyResponse())
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
