package parity

import (
	"net/http"
	"net/http/httptest"
	"slices"
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
//     HTTP 400. This is the v1.16
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
		// upstream: 2.83.0 rest.py:597 (`v = unidecode.unidecode(v)` applies
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

	t.Run("bool_values_like_upstream", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:680-681 (a plain key selects true
		// for "true" in any case or "1", false for any other value),
		// :664-669 (an operator passes the value to Django, and
		// BooleanField.to_python accepts only t, True, 1, f, False and
		// 0; __in splits on commas and does not strip). An invalid value
		// raises ValidationError, and the list handler returns 400
		// (rest.py:693-701, :828-831). diverse_serving_substations is
		// nullable (django-peeringdb abstract.py:259): an empty __in
		// item is None, which In drops.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "BoolOrg", t0)
		for _, n := range []struct {
			id      int
			unicast bool
		}{{1, true}, {2, false}} {
			name := "BoolNet" + strconv.Itoa(n.id)
			c.Network.Create().
				SetID(n.id).SetName(name).SetNameFold(unifold.Fold(name)).
				SetAsn(64500 + n.id).SetOrgID(1).SetInfoUnicast(n.unicast).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		for _, f := range []struct {
			id  int
			dss *bool
		}{{400, new(true)}, {401, new(false)}, {402, nil}} {
			name := "BoolFac" + strconv.Itoa(f.id)
			c.Facility.Create().
				SetID(f.id).SetName(name).SetNameFold(unifold.Fold(name)).
				SetOrgID(1).SetCity("C").SetCityFold("c").SetCountry("DE").
				SetNillableDiverseServingSubstations(f.dss).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		srv := newTestServer(t, c)

		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/net?info_unicast=TRUE", []int{1}},
			{"/api/net?info_unicast=1", []int{1}},
			{"/api/net?info_unicast=t", []int{2}},
			{"/api/net?info_unicast=yes", []int{2}},
			{"/api/net?info_unicast=", []int{2}},
			{"/api/net?info_unicast__in=t,False", []int{1, 2}},
			{"/api/net?info_unicast__in=True", []int{1}},
			{"/api/net?info_unicast__in=0", []int{2}},
			{"/api/net?info_unicast__lt=True", []int{2}},
			{"/api/net?info_unicast__gte=t", []int{1}},
			{"/api/fac?diverse_serving_substations=0", []int{401}},
			{"/api/fac?diverse_serving_substations__in=,1", []int{400}},
			{"/api/fac?diverse_serving_substations__in=", []int{}},
			{"/api/fac?diverse_serving_substations__gt=f", []int{400}},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Errorf("%s: status %d, want 200; body=%s", tc.path, status, body)
				continue
			}
			if ids := extractIDs(t, body); !slices.Equal(ids, tc.want) {
				t.Errorf("%s: ids %v, want %v", tc.path, ids, tc.want)
			}
		}
		for _, path := range []string{
			"/api/net?info_unicast__in=true",
			"/api/net?info_unicast__in=T",
			"/api/net?info_unicast__in=1,%201",
			"/api/net?info_unicast__in=",
			"/api/net?info_unicast__in=1,",
			"/api/net?info_unicast__lt=false",
			"/api/fac?diverse_serving_substations__lt=",
		} {
			if status, body := httpGet(t, srv, path); status != http.StatusBadRequest {
				t.Errorf("%s: status %d, want 400; body=%s", path, status, body)
			}
		}
	})

	t.Run("numeric_text_like_upstream", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:659-662 turns contains and startswith
		// into icontains and istartswith, and :683 makes a plain key
		// __iexact. These lookups do not convert the value
		// (prepare_rhs=False, django/db/models/lookups.py), so MySQL
		// compares the column as text with LIKE: an integer as decimal
		// text, a boolean as 1 or 0, and latitude/longitude
		// (DecimalField, 6 decimals, django-peeringdb abstract.py:94-113)
		// as for example 52.500000.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "NumOrg", t0)
		mustNet(ctx, t, c, 1, "NumNet1", 64512, 1, t0)
		mustNet(ctx, t, c, 2, "NumNet2", 13335, 1, t0)
		for _, f := range []struct {
			id  int
			lat *float64
		}{{400, new(52.5)}, {401, new(-0.25)}, {402, nil}} {
			name := "NumFac" + strconv.Itoa(f.id)
			c.Facility.Create().
				SetID(f.id).SetName(name).SetNameFold(unifold.Fold(name)).
				SetOrgID(1).SetCity("C").SetCityFold("c").SetCountry("DE").
				SetNillableLatitude(f.lat).
				SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		}
		srv := newTestServer(t, c)
		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/net?asn__contains=451", []int{1}},
			{"/api/net?asn__startswith=1333", []int{2}},
			{"/api/net?asn__contains=x", []int{}},
			{"/api/net?id__startswith=2", []int{2}},
			{"/api/net?info_unicast__contains=0", []int{1, 2}},
			{"/api/net?info_unicast__startswith=false", []int{}},
			{"/api/fac?latitude=52.5", []int{}},
			{"/api/fac?latitude=52.500000", []int{400}},
			{"/api/fac?latitude=-0.250000", []int{401}},
			{"/api/fac?latitude__contains=.5", []int{400}},
			{"/api/fac?latitude__startswith=-0.2", []int{401}},
			{"/api/fac?latitude__startswith=0", []int{}},
			{"/api/fac?latitude=abc", []int{}},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Errorf("%s: status %d, want 200; body=%s", tc.path, status, body)
				continue
			}
			if ids := extractIDs(t, body); !slices.Equal(ids, tc.want) {
				t.Errorf("%s: ids %v, want %v", tc.path, ids, tc.want)
			}
		}
	})

	// seedDateNets seeds net 1 created 2024-01-01 12:30:45 and net 2
	// created 2024-01-02 00:00:00, both UTC.
	seedDateNets := func(t *testing.T) *httptest.Server {
		t.Helper()
		c := testutil.SetupClient(t)
		ctx := t.Context()
		mustOrg(ctx, t, c, 1, "DateOrg", t0)
		for id, created := range map[int]time.Time{
			1: time.Date(2024, 1, 1, 12, 30, 45, 0, time.UTC),
			2: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		} {
			name := "DateNet" + strconv.Itoa(id)
			c.Network.Create().
				SetID(id).SetName(name).SetNameFold(unifold.Fold(name)).
				SetAsn(64500 + id).SetOrgID(1).
				SetStatus("ok").SetCreated(created).SetUpdated(t0).SaveX(ctx)
		}
		return newTestServer(t, c)
	}

	t.Run("date_operators_like_upstream", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:640-662. For gt and lte, a value of
		// 10 characters gets " 23:59:59.999". Django
		// DateTimeField.to_python converts the value (fromisoformat,
		// then datetime_re, then parse_date), a naive value is UTC
		// (settings TIME_ZONE), and a value that it rejects raises
		// ValidationError, which the list returns as 400 (rest.py:647-651,
		// :828-831). contains and startswith compare str(datetime), which
		// ends in "+00:00", with the MySQL text of the column, so they
		// match no row.
		srv := seedDateNets(t)
		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/net?created__lt=2024-01-02", []int{1}},
			{"/api/net?created__lte=2024-01-01", []int{1}},
			{"/api/net?created__gt=2024-01-01", []int{2}},
			{"/api/net?created__gte=2024-01-01T12:30:45Z", []int{1, 2}},
			{"/api/net?created__gt=2024-01-01T13:30:45%2B01:00", []int{2}},
			{"/api/net?created__lt=2024-01-01T12:30:45.5", []int{1}},
			{"/api/net?created__lt=2024W011", []int{}},
			{"/api/net?created__gte=2024-1-1%201:2", []int{1, 2}},
			{"/api/net?created__lt=2024-01-01T24:00", []int{1}},
			{"/api/net?created__contains=2024-01-01", []int{}},
			{"/api/net?created__startswith=2024-01-01%2012:30:45", []int{}},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Errorf("%s: status %d, want 200; body=%s", tc.path, status, body)
				continue
			}
			if ids := extractIDs(t, body); !slices.Equal(ids, tc.want) {
				t.Errorf("%s: ids %v, want %v", tc.path, ids, tc.want)
			}
		}
		for _, path := range []string{
			"/api/net?created__gt=1700000000",
			"/api/net?created__lt=2024",
			"/api/net?created__lt=2024-01-01T12:30z",
			"/api/net?created__startswith=2024",
			"/api/net?created__contains=x",
			"/api/net?created__lte=2024-02-30",
		} {
			if status, body := httpGet(t, srv, path); status != http.StatusBadRequest {
				t.Errorf("%s: status %d, want 400; body=%s", path, status, body)
			}
		}
	})

	t.Run("date_key_without_operator_is_text_prefix", func(t *testing.T) {
		t.Parallel()
		// upstream: 2.83.0 rest.py:678-679 filters a date key without an
		// operator with __startswith, which does not convert the value
		// (prepare_rhs=False), so MySQL matches it as a prefix of the
		// DATETIME(6) text "YYYY-MM-DD HH:MM:SS.ffffff" (LIKE BINARY,
		// with % and _ escaped).
		srv := seedDateNets(t)
		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/net?created=2024-01-0", []int{1, 2}},
			{"/api/net?created=2024-01-01", []int{1}},
			{"/api/net?created=2024-01-01%2012:30:45", []int{1}},
			{"/api/net?created=2024-01-01%2012:30:45.", []int{1}},
			{"/api/net?created=2024-01-01T12:30:45", []int{}},
			{"/api/net?created=2024-01-01%2012:30:45%2B00:00", []int{}},
			{"/api/net?created=1704112245", []int{}},
			{"/api/net?created=2024-01-01%25", []int{}},
			{"/api/net?created=x", []int{}},
			{"/api/net?created=", []int{1, 2}},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Errorf("%s: status %d, want 200; body=%s", tc.path, status, body)
				continue
			}
			if ids := extractIDs(t, body); !slices.Equal(ids, tc.want) {
				t.Errorf("%s: ids %v, want %v", tc.path, ids, tc.want)
			}
		}
	})

	t.Run("DIVERGENCE_date_compare_whole_seconds", func(t *testing.T) {
		t.Parallel()
		// upstream: created and updated are DATETIME(6), so a row
		// created at 12:30:45.5 is later than 12:30:45 and matches
		// created__gt=2024-01-01T12:30:45 (rest.py:640-669). The API
		// shows whole seconds, and the mirror stores the second it
		// shows, so the row matches __lte instead. A key without an
		// operator that names microseconds matches the whole second.
		srv := seedDateNets(t)
		for _, tc := range []struct {
			path string
			want []int
		}{
			{"/api/net?created__gt=2024-01-01T12:30:45", []int{2}},
			{"/api/net?created__lte=2024-01-01T12:30:45", []int{1}},
			// Upstream matches the microseconds of the text; the
			// mirror matches every row of the second.
			{"/api/net?created=2024-01-01%2012:30:45.5", []int{1}},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusOK {
				t.Errorf("%s: status %d, want 200; body=%s", tc.path, status, body)
				continue
			}
			if ids := extractIDs(t, body); !slices.Equal(ids, tc.want) {
				t.Errorf("%s: ids %v, want %v", tc.path, ids, tc.want)
			}
		}
	})

	t.Run("DIVERGENCE_date_in_filters", func(t *testing.T) {
		t.Parallel()
		// upstream: the date branch converts the whole __in value with
		// to_python (rest.py:647-653). A list of two or more values
		// fails it (400), and for one value v.split(",") then raises
		// AttributeError on the datetime (rest.py:666), a 500. The
		// mirror returns the rows at one of the listed times.
		srv := seedDateNets(t)
		status, body := httpGet(t, srv, "/api/net?created__in=2024-01-01T12:30:45Z,2024-01-02T00:00:00Z")
		if status != http.StatusOK {
			t.Fatalf("status %d, want 200; body=%s", status, body)
		}
		if ids := extractIDs(t, body); !slices.Equal(ids, []int{1, 2}) {
			t.Errorf("ids %v, want [1 2]", ids)
		}
	})

	t.Run("malformed_int_csv_returns_400", func(t *testing.T) {
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
