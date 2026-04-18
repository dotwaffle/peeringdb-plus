package pdbcompat_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/conformance"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// anonParityTypes is the canonical ordered list of the 13 PeeringDB types
// whose anonymous fixtures live under testdata/visibility-baseline/beta/anon/api/.
// The same set is exercised by internal/conformance/live_test.go.
var anonParityTypes = []string{
	"campus",
	"carrier",
	"carrierfac",
	"fac",
	"ix",
	"ixfac",
	"ixlan",
	"ixpfx",
	"net",
	"netfac",
	"netixlan",
	"org",
	"poc",
}

// anonFixtureRoot is the committed phase-57 anon fixture corpus (VIS-01).
// Relative to this test file (internal/pdbcompat/) it sits two levels up.
const anonFixtureRoot = "../../testdata/visibility-baseline/beta/anon/api"

// knownDivergences lists (path, kind) tuples that CompareResponses flags
// but which are acknowledged differences between our pdbcompat layer and
// upstream PeeringDB. Keyed as "{type}|{path}|{kind}".
//
// Add to this list ONLY with a comment explaining the root cause and a
// tracking issue. Each entry must describe:
//   - The exact divergent field/path
//   - Why the divergence exists (schema history, upstream behaviour)
//   - Whether it is a privacy leak (if yes — NOT a known divergence; fix it)
//   - The tracking follow-up (plan/requirement id)
//
// The intent of this test is to catch shape drift, not launder it. Entries
// here require operator sign-off recorded in the plan SUMMARY.
//
// v1.15 Phase 63 (D-03): removed the "ixpfx|data[0].notes|extra_field"
// entry after dropping the ent schema field and the peeringdb.IxPrefix
// struct field — the divergence is now resolved at the source.
var knownDivergences = map[string]struct{}{}

// TestAnonParityFixtures replays each committed anonymous VIS-01 fixture
// against a local httptest.Server wrapping the pdbcompat handler with an
// anonymous privacy-tier stamp, and asserts that conformance.CompareResponses
// returns zero structural differences.
//
// Why this test lives here: the fixture corpus targets the pdbcompat /api/
// surface specifically. The full middleware chain is not wired — the only
// middleware that materially affects the response shape for this test is
// PrivacyTier, which we reproduce inline to avoid pulling the main package
// into internal/pdbcompat (import-cycle risk and unnecessary coupling).
// See 60-03-PLAN.md §1 design-decisions for the full rationale.
//
// The POC sub-test additionally asserts the absent-not-redacted invariant
// (VIS-07, D-08): rows seeded via seed.Full with visible="Users" (IDs 9000,
// 9001) must NOT appear in the anonymous /api/poc response at all — not
// present-with-null-PII.
func TestAnonParityFixtures(t *testing.T) {
	t.Parallel()

	client := testutil.SetupClient(t)
	_ = seed.Full(t, client)

	// Pdbcompat handler on a bare ServeMux + a tiny inline PrivacyTier
	// stamper. This is the minimum wiring that exercises the ent privacy
	// policy on the output path, matching what internal/middleware.PrivacyTier
	// does in production without the chainConfig dependency.
	mux := http.NewServeMux()
	pdbcompat.NewHandler(client).Register(mux)
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := privctx.WithTier(r.Context(), privctx.TierPublic)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Confirm all 13 fixture directories are present up front. A missing
	// fixture is a supply-chain problem the test should surface, not silently
	// skip.
	entries, err := os.ReadDir(anonFixtureRoot)
	if err != nil {
		t.Fatalf("read fixture root %s: %v", anonFixtureRoot, err)
	}
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			seen[e.Name()] = struct{}{}
		}
	}
	for _, typeName := range anonParityTypes {
		if _, ok := seen[typeName]; !ok {
			t.Fatalf("fixture directory missing for type %q under %s", typeName, anonFixtureRoot)
		}
	}

	for _, typeName := range anonParityTypes {
		t.Run(typeName, func(t *testing.T) {
			t.Parallel()

			fixturePath := filepath.Join(anonFixtureRoot, typeName, "page-1.json")
			refBody, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("read fixture %s: %v", fixturePath, err)
			}

			// ?limit=1 pins both sides to one representative row. The
			// CompareResponses helper probes array shape via data[0] (see
			// internal/conformance/compare.go:94-101), so one local row is
			// sufficient to characterise envelope + per-row structure.
			url := srv.URL + "/api/" + typeName + "?limit=1"
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET %s: %v", url, err)
			}
			ourBody, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status %d, body=%s", url, resp.StatusCode, ourBody)
			}

			diffs, err := conformance.CompareResponses(refBody, ourBody)
			if err != nil {
				t.Fatalf("compare responses: %v", err)
			}
			for _, d := range diffs {
				key := typeName + "|" + d.Path + "|" + d.Kind
				if _, ok := knownDivergences[key]; ok {
					t.Logf("known divergence %s: %s", key, d.Details)
					continue
				}
				t.Errorf("structural diff for %s: path=%q kind=%q details=%s",
					typeName, d.Path, d.Kind, d.Details)
			}

			// Absent-not-redacted invariant (VIS-07, D-08) for the POC
			// type: the seeded Users-tier POCs (IDs 9000, 9001) must be
			// structurally absent from the anonymous response, not present
			// with null PII.
			if typeName == "poc" {
				assertUsersPocsAbsent(t, ourBody)
			}
		})
	}
}

// assertUsersPocsAbsent fails the test if the anonymous /api/poc response
// body contains either of the seed.Full Users-tier POC IDs (9000, 9001),
// or any row with visible="Users". This is the VIS-07 leak check.
func assertUsersPocsAbsent(t *testing.T, body []byte) {
	t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal /api/poc body: %v", err)
	}
	for _, row := range env.Data {
		if id, ok := row["id"].(float64); ok {
			if int(id) == 9000 || int(id) == 9001 {
				t.Errorf("Users-tier POC id=%v leaked into anon /api/poc response", id)
			}
		}
		if vis, ok := row["visible"].(string); ok && vis == "Users" {
			t.Errorf("visible=%q row leaked into anon /api/poc response: id=%v", vis, row["id"])
		}
	}
}
