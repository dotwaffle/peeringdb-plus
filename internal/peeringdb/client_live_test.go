package peeringdb

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var peeringdbLive = flag.Bool("peeringdb-live", false, "run live meta.generated tests against beta.peeringdb.com")

// apiResponse is the envelope structure for PeeringDB API responses.
type apiResponse struct {
	Meta json.RawMessage   `json:"meta"`
	Data []json.RawMessage `json:"data"`
}

// TestMetaGeneratedLive verifies meta.generated field behavior across three
// PeeringDB API request patterns against beta.peeringdb.com. This test is
// gated by the -peeringdb-live flag and skipped during normal test runs.
//
// The three patterns tested:
//   - full_fetch: GET /api/{type}?depth=0 (cached response, meta.generated present)
//   - paginated_incremental: GET /api/{type}?depth=0&limit=250&skip=0&since=T (live query, meta.generated absent)
//   - empty_result: GET /api/{type}?depth=0&since=future (empty data, meta.generated absent)
//
// DATA-02: Confirms parseMeta returns zero time for paginated responses.
// Sync no longer relies on meta.generated for cursor advancement —
// cursors are derived from MAX(updated) per table (see
// internal/sync/cursor.go). 260428-mu0.
func TestMetaGeneratedLive(t *testing.T) {
	if !*peeringdbLive {
		t.Skip("skipping live meta.generated test (use -peeringdb-live to enable)")
	}

	apiKey := os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
	sleepDuration := 3 * time.Second
	if apiKey != "" {
		sleepDuration = 1 * time.Second
		t.Log("using API key for authenticated access (1s sleep between requests)")
	} else {
		t.Log("no API key configured, using unauthenticated access (3s sleep between requests)")
	}

	client := &http.Client{Timeout: 30 * time.Second}

	// Subtest 1: full_fetch - verifies meta.generated is present on cached responses.
	// Full fetch responses are served from PeeringDB's cache and include a
	// meta.generated float64 Unix epoch.
	t.Run("full_fetch", func(t *testing.T) {
		types := []string{"net", "ix", "fac", "org", "carrier"}
		for i, typeName := range types {
			if i > 0 {
				time.Sleep(sleepDuration)
			}

			url := fmt.Sprintf("https://beta.peeringdb.com/api/%s?depth=0", typeName)
			body := doGet(t, client, url, apiKey)

			var resp apiResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("unmarshal %s response: %v", typeName, err)
			}

			generated := parseMeta(resp.Meta)
			if generated.IsZero() {
				t.Errorf("%s: parseMeta returned zero time, expected non-zero (meta.generated should be present on full fetch)", typeName)
				continue
			}

			// Cache should be relatively recent (within 24 hours).
			age := time.Since(generated)
			if age > 24*time.Hour {
				t.Errorf("%s: meta.generated is %v old (>24h), cache may be stale", typeName, age)
			}

			t.Logf("%s: meta.generated = %v (age: %v, data count: %d)", typeName, generated, age, len(resp.Data))
		}
	})

	// Sleep between subtest groups to respect rate limits.
	time.Sleep(sleepDuration)

	// Subtest 2: paginated_incremental - verifies meta.generated is absent on
	// parameterized queries. Any request with limit, skip, or since parameters
	// bypasses PeeringDB's cache and returns meta: {}.
	// DATA-02: Historical: this used to feed worker.go's meta.generated-based
	// cursor; the 260428-mu0 cursor rewrite no longer reads meta.generated
	// for advancement. The test stays as documentation of upstream behaviour.
	t.Run("paginated_incremental", func(t *testing.T) {
		types := []string{"net", "ix", "fac"}
		since := time.Now().Add(-7 * 24 * time.Hour).Unix()

		for i, typeName := range types {
			if i > 0 {
				time.Sleep(sleepDuration)
			}

			url := fmt.Sprintf("https://beta.peeringdb.com/api/%s?depth=0&limit=250&skip=0&since=%d", typeName, since)
			body := doGet(t, client, url, apiKey)

			var resp apiResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("unmarshal %s response: %v", typeName, err)
			}

			generated := parseMeta(resp.Meta)
			if !generated.IsZero() {
				t.Errorf("%s: parseMeta returned %v, expected zero time (meta.generated should be absent on paginated/incremental)", typeName, generated)
			}

			t.Logf("%s: meta = %s (data count: %d)", typeName, string(resp.Meta), len(resp.Data))
		}
	})

	// Sleep between subtest groups to respect rate limits.
	time.Sleep(sleepDuration)

	// Subtest 3: empty_result - verifies meta.generated is absent when the result
	// set is empty (future since timestamp).
	t.Run("empty_result", func(t *testing.T) {
		since := time.Now().Add(24 * time.Hour).Unix()
		url := fmt.Sprintf("https://beta.peeringdb.com/api/net?depth=0&since=%d", since)
		body := doGet(t, client, url, apiKey)

		var resp apiResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		if len(resp.Data) != 0 {
			t.Errorf("expected empty data array for future since, got %d items", len(resp.Data))
		}

		generated := parseMeta(resp.Meta)
		if !generated.IsZero() {
			t.Errorf("parseMeta returned %v, expected zero time for empty result set", generated)
		}

		t.Logf("empty_result: meta = %s, data count: %d", string(resp.Meta), len(resp.Data))
	})
}

// doGet performs an HTTP GET request and returns the response body. It sets
// the User-Agent header and optionally the Authorization header if an API
// key is provided. Fails the test on non-200 responses.
func doGet(t *testing.T, client *http.Client, url string, apiKey string) []byte {
	t.Helper()

	ctx := t.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create request for %s: %v", url, err)
	}

	req.Header.Set("User-Agent", "peeringdb-plus/test")
	if apiKey != "" {
		req.Header.Set("Authorization", "Api-Key "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body from %s: %v", url, err)
	}

	return body
}
