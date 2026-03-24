package conformance_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/conformance"
)

var peeringdbLive = flag.Bool("peeringdb-live", false, "run live conformance tests against beta.peeringdb.com")

// allTypes lists all PeeringDB object types in sorted order.
var allTypes = []string{
	"campus", "carrier", "carrierfac", "fac", "ix", "ixfac",
	"ixlan", "ixpfx", "net", "netfac", "netixlan", "org", "poc",
}

// TestLiveConformance fetches responses from beta.peeringdb.com and compares
// their structure against golden files. This test is gated by the
// -peeringdb-live flag and skipped during normal test runs.
func TestLiveConformance(t *testing.T) {
	if !*peeringdbLive {
		t.Skip("skipping live conformance test (use -peeringdb-live to enable)")
	}

	apiKey := os.Getenv("PDBPLUS_PEERINGDB_API_KEY")
	sleepDuration := 3 * time.Second
	if apiKey != "" {
		sleepDuration = 1 * time.Second
		t.Log("using API key for authenticated access (1s sleep)")
	} else {
		t.Log("no API key configured, using unauthenticated access (3s sleep)")
	}

	// NOT parallel: sequential to respect PeeringDB rate limits.
	client := &http.Client{Timeout: 30 * time.Second}
	goldenDir := findGoldenDir(t)

	for i, typeName := range allTypes {
		if i > 0 {
			time.Sleep(sleepDuration)
		}

		t.Run(typeName, func(t *testing.T) {
			// NOT parallel: sequential to respect rate limits.
			ctx := context.Background()

			// Fetch from beta.peeringdb.com.
			url := fmt.Sprintf("https://beta.peeringdb.com/api/%s?limit=1", typeName)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("create request for %s: %v", typeName, err)
			}
			req.Header.Set("User-Agent", "pdbcompat-check-test/1.0")
			if apiKey != "" {
				req.Header.Set("Authorization", "Api-Key "+apiKey)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("fetch %s: %v", typeName, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("fetch %s: HTTP %d", typeName, resp.StatusCode)
			}

			liveBody, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read %s response: %v", typeName, err)
			}

			// Verify meta.generated field presence in PeeringDB responses.
			checkMetaGenerated(t, typeName, liveBody)

			// Compare against golden file if available.
			goldenPath := filepath.Join(goldenDir, typeName, "list.json")
			goldenBody, err := os.ReadFile(goldenPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Logf("golden file %s not found, skipping structural comparison", goldenPath)
					return
				}
				t.Fatalf("read golden file %s: %v", goldenPath, err)
			}

			diffs, err := conformance.CompareResponses(goldenBody, liveBody)
			if err != nil {
				t.Fatalf("compare responses for %s: %v", typeName, err)
			}

			for _, d := range diffs {
				t.Errorf("structural difference: %s %s: %s", d.Kind, d.Path, d.Details)
			}
		})
	}
}

// checkMetaGenerated verifies that the PeeringDB response contains a meta
// object. PeeringDB responses include a "meta" field (typically empty or
// containing a "generated" timestamp).
func checkMetaGenerated(t *testing.T, typeName string, body []byte) {
	t.Helper()

	var envelope struct {
		Meta json.RawMessage `json:"meta"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Errorf("%s: failed to parse response envelope: %v", typeName, err)
		return
	}

	if envelope.Meta == nil {
		t.Errorf("%s: response missing meta field", typeName)
		return
	}

	// Check if meta contains generated field (present on paginated responses).
	var meta map[string]any
	if err := json.Unmarshal(envelope.Meta, &meta); err != nil {
		// meta might be an empty object {} which is valid.
		return
	}

	if _, ok := meta["generated"]; ok {
		t.Logf("%s: meta.generated field present", typeName)
	}
}

// findGoldenDir locates the golden file directory relative to the test
// package directory.
func findGoldenDir(t *testing.T) string {
	t.Helper()

	// When running from the conformance package directory, the golden files
	// are at a relative path.
	candidates := []string{
		"../pdbcompat/testdata/golden",
		"../../internal/pdbcompat/testdata/golden",
		"internal/pdbcompat/testdata/golden",
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}

	// Fall back and let the individual test skip if golden files don't exist.
	return "../pdbcompat/testdata/golden"
}
