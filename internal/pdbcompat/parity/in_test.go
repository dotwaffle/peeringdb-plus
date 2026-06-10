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
//   - 5001-element id list returns all 5001 rows. Exercises the
//     json_each rewrite that bypasses SQLite's 999-variable limit.
//     Implementations without the rewrite would 500 or truncate.
//   - empty `__in` value short-circuits to an empty result set
//     (matches Django ORM Model.objects.filter(id__in=[])).
//   - (control): malformed CSV (`?asn__in=13335,abc`) returns
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

	t.Run("5001_elements_returns_all_no_sqlite_999_var_trip", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py (Django ORM passes id__in straight to
		// the SQL backend; on PostgreSQL there is no analogous limit).
		// The json_each rewrite ensures SQLite's 999-variable cap
		// doesn't bite at the 5001-id boundary derived from
		// InFixtures (literal IDs 100000..105000).
		c := testutil.SetupClient(t)
		ctx := t.Context()
		// Seed exactly the InFixtures id range (matches the literal
		// query string that operators would form when filtering by
		// the sentinel block).
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
			t.Fatalf("5001-id query status = %d; body[:200]=%s",
				status, headBody(body, 200))
		}
		// SQLite errors leak through to the body if the rewrite fails;
		// guard against silent regression.
		assertNoSQLiteVariableLimit(t, body)

		got := decodeDataArray(t, body)
		want := hi - lo + 1
		if len(got) != want {
			t.Errorf("got %d rows, want %d (5001-element json_each rewrite)", len(got), want)
		}
	})

	t.Run("empty_returns_empty_data", func(t *testing.T) {
		t.Parallel()
		// upstream: Django ORM Model.objects.filter(id__in=[])
		// returns an empty queryset without issuing SQL.
		// pdbcompat short-circuits via opts.EmptyResult in handler.go
		// before any predicate runs.
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
			t.Errorf("empty __in: got %d rows, want 0", len(got))
		}
	})

	t.Run("string_in_is_case_insensitive_and_folded", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:576 (`v = unidecode.unidecode(v)` applies
		// to ALL filter values, including __in) + MySQL's
		// case-insensitive utf8mb4 collation. ?name=decix matching
		// "DECIX" while ?name__in=decix missed it was an internal
		// inconsistency on the same field.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		seed := func(id int, name string) {
			if _, err := c.Network.Create().
				SetID(id).SetName(name).SetNameFold(unifold.Fold(name)).
				SetAsn(64500 + id).SetStatus("ok").
				SetCreated(t0).SetUpdated(t0).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", id, err)
			}
		}
		seed(1, "DECIX Test")
		seed(2, "Drüben Networks")
		seed(3, "Other")

		srv := newTestServer(t, c)

		// Case-insensitive: lowercase query matches uppercase row.
		status, body := httpGet(t, srv, "/api/net?name__in=decix%20test")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 1 {
			t.Errorf("case-insensitive __in: got %v, want [1]", ids)
		}

		// Diacritic-folded: ASCII query matches the umlaut row via the
		// name_fold shadow column.
		status, body = httpGet(t, srv, "/api/net?name__in=druben%20networks")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		if ids := extractIDs(t, body); len(ids) != 1 || ids[0] != 2 {
			t.Errorf("fold-routed __in: got %v, want [2]", ids)
		}
	})

	t.Run("malformed_int_csv_returns_400_problem_json", func(t *testing.T) {
		t.Parallel()
		// v1.16 behaviour lock: malformed values in a typed-int
		// __in list propagate as a 400 (filter_test.go:632 covers
		// the predicate-layer error). Locking this here surfaces a
		// future transition to upstream's silent-skip semantics as
		// an intentional, reviewable change.
		// synthesised: the strict-typing 400 is novel to this fork;
		// upstream Django ORM coerces silently.
		c := testutil.SetupClient(t)
		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net?asn__in=13335,abc")
		if status != http.StatusBadRequest {
			t.Errorf("malformed asn__in: got %d, want 400; body=%s",
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
			t.Errorf("response body leaks SQL error fragment %q: body[:300]=%s",
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
