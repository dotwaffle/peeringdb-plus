package parity

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_In locks the v1.16 `__in` operator semantics:
//
//   - IN-01: 5001-element id list returns all 5001 rows. Exercises
//     the Phase 69 D-05 json_each rewrite that bypasses SQLite's
//     999-variable limit. Pre-Phase-69 implementations would 500 or
//     truncate.
//   - IN-02: empty `__in` value short-circuits to an empty result
//     set (matches Django ORM Model.objects.filter(id__in=[])).
//   - IN-03 (control): malformed CSV (`?asn__in=13335,abc`) returns
//     HTTP 400 application/problem+json. This is the v1.16
//     behaviour locked by filter_test.go:632; the parity test
//     records it here so a future move toward upstream's
//     silent-skip semantics on int-coercion failures is a
//     deliberate choice rather than an accidental regression.
//
// upstream: peeringdb_server/rest.py (Django ORM __in semantics)
// upstream: pdb_api_test.py (multiple sites; bulk lookups via
// id__in/asn__in are common across the corpus)
func TestParity_In(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("IN-01_5001_elements_returns_all_no_sqlite_999_var_trip", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py (Django ORM passes id__in straight to
		// the SQL backend; on PostgreSQL there is no analogous limit).
		// Phase 69 D-05 added the json_each rewrite so SQLite's
		// 999-variable cap doesn't bite at the 5001-id boundary
		// derived from InFixtures (literal IDs 100000..105000 per
		// CONTEXT.md plan 72-03 D-04).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed exactly the InFixtures id range (matches the literal
		// query string that operators would form when filtering by
		// the Phase 72-03 sentinel block).
		const lo, hi = 100000, 105000
		for id := lo; id <= hi; id++ {
			if _, err := c.Network.Create().
				SetID(id).SetName("InBulk").SetNameFold(unifold.Fold("InBulk")).
				SetAsn(4_300_000_000 + (id - lo)).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net id=%d: %v", id, err)
			}
		}
		srv := newTestServer(t, c)

		// Form the 5001-element CSV. id__in is THE canonical
		// surface for bulk fetches across the codebase.
		ids := make([]string, 0, hi-lo+1)
		for id := lo; id <= hi; id++ {
			ids = append(ids, strconv.Itoa(id))
		}
		path := "/api/net?id__in=" + strings.Join(ids, ",") + "&limit=0"
		status, body := httpGet(t, srv, path)
		if status != http.StatusOK {
			t.Fatalf("IN-01 5001-id query status = %d; body[:200]=%s",
				status, headBody(body, 200))
		}
		// SQLite errors leak through to the body if the rewrite fails;
		// guard against silent regression.
		assertNoSQLiteVariableLimit(t, body)

		got := decodeDataArray(t, body)
		want := hi - lo + 1
		if len(got) != want {
			t.Errorf("IN-01: got %d rows, want %d (5001-element json_each rewrite)", len(got), want)
		}
	})

	t.Run("IN-02_empty_returns_empty_data", func(t *testing.T) {
		t.Parallel()
		// upstream: Django ORM Model.objects.filter(id__in=[])
		// returns an empty queryset without issuing SQL.
		// Phase 69 D-06: pdbcompat short-circuits via opts.EmptyResult
		// in handler.go before any predicate runs.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed a row that WOULD match the open query — proves the
		// short-circuit, not just an empty corpus.
		if _, err := c.Network.Create().
			SetID(1).SetName("InEmptyProbe").SetNameFold(unifold.Fold("InEmptyProbe")).
			SetAsn(4_300_005_001).SetStatus("ok").
			SetCreated(t0).SetUpdated(t0).
			Save(ctx); err != nil {
			t.Fatalf("seed net: %v", err)
		}
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?id__in=")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		got := decodeDataArray(t, body)
		if len(got) != 0 {
			t.Errorf("IN-02 empty __in: got %d rows, want 0", len(got))
		}
	})

	t.Run("IN-03_malformed_int_csv_returns_400_problem_json", func(t *testing.T) {
		t.Parallel()
		// v1.16 behaviour lock: malformed values in a typed-int
		// __in list propagate as a 400 (filter_test.go:632 covers
		// the predicate-layer error). Locking this here surfaces a
		// future transition to upstream's silent-skip semantics as
		// an intentional, reviewable change.
		// synthesised: phase69-plan-02 (the strict-typing 400 is
		// novel to this fork; upstream Django ORM coerces
		// silently).
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?asn__in=13335,abc")
		if status != http.StatusBadRequest {
			t.Errorf("IN-03 malformed asn__in: got %d, want 400; body=%s",
				status, string(body))
		}
	})
}

// assertNoSQLiteVariableLimit asserts the response body does not
// contain SQLite-error fragments that would indicate the 999-variable
// limit (or any other runtime SQL error) leaked through serialisation.
func assertNoSQLiteVariableLimit(t *testing.T, body []byte) {
	t.Helper()
	for _, frag := range []string{
		"too many SQL variables",
		"SQL logic error",
		"sqlite",
	} {
		if strings.Contains(strings.ToLower(string(body)), frag) {
			t.Errorf("IN-01: response body leaks SQL error fragment %q: body[:300]=%s",
				frag, headBody(body, 300))
			return
		}
	}
}

// headBody returns up to n bytes of body for inclusion in test failure
// messages. Avoids dumping a 5001-row payload on a single failure.
func headBody(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "...(truncated)"
}
