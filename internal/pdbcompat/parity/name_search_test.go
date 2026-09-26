package parity

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_NameSearch locks the name_search key of the /api lists
// against PeeringDB 2.83.0.
//
// Upstream sends a non-empty value to its Elasticsearch index when the
// type has one (org, fac, ix, net, campus, carrier), keeps the ids of
// the hits of that type and filters the list by them. With no hit, or
// on the other 7 types, get_queryset returns qset.none() before its
// filter loop. The mirror runs the same kinds of match in SQL: words as
// substrings of the indexed fields, digits as an ASN prefix, and a
// partial IP address as a prefix of the live netixlan addresses.
//
// upstream: 2.83.0 peeringdb_server/rest.py:532-553 (name_search),
// :685-690 (id__in union), search_v2.py:861-978 (search_v2),
// documents.py:97-817 (indexed fields and status)
func TestParity_NameSearch(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	t.Run("words_ignore_case", func(t *testing.T) {
		t.Parallel()
		// upstream: documents.py:52-56 (name_analyzer lower-cases the
		// whole name), search_v2.py:560-571 (phrase_prefix), :574-585
		// (the wildcards of each word)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=ALPHA", want: []int{100}},
			{path: "/api/net?name_search=alpha%20networks", want: []int{100}},
		})
	})

	t.Run("words_and_across_fields", func(t *testing.T) {
		t.Parallel()
		// upstream: search_v2.py:255-280 (AND between the words),
		// :574-585 (query_string over the multi-field list)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			// aka AlphaNet and name Alpha Networks.
			{path: "/api/net?name_search=alphanet%20networks", want: []int{100}},
			{path: "/api/net?name_search=alpha%20beta", want: []int{}},
			// upstream: search_v2.py:902-906 (the AND operator word)
			{path: "/api/net?name_search=alpha%20AND%20networks", want: []int{100}},
		})
	})

	t.Run("fields_per_type", func(t *testing.T) {
		t.Parallel()
		// upstream: documents.py:662-664 (net irr_as_set), :293, :436,
		// :540 (org, fac and ix city; net has no city)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=as-alpha", want: []int{100}},
			{path: "/api/fac?name_search=frankfurt", want: []int{200}},
			{path: "/api/ix?name_search=frankfurt", want: []int{300}},
			{path: "/api/org?name_search=frankfurt", want: []int{1}},
			{path: "/api/net?name_search=frankfurt", want: []int{}},
			// upstream: documents.py:697-817 (campus and carrier)
			{path: "/api/carrier?name_search=alpha", want: []int{500}},
			{path: "/api/campus?name_search=campus", want: []int{400}},
			// Campus 400 has no aka or name_long (NULL). A NULL field
			// holds no word, so the row does not match.
			{path: "/api/campus?name_search=zzz", want: []int{}},
		})
	})

	t.Run("digits_asn_exact_and_prefix", func(t *testing.T) {
		t.Parallel()
		// upstream: search_v2.py:359-424 (ASN term and prefix, phrase
		// clauses on name, name_long and aka), :618-620 (digit test)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=64500", want: []int{100}},
			// Net 102 (64501) is deleted.
			{path: "/api/net?name_search=645", want: []int{100, 101}},
			{path: "/api/net?name_search=65", want: []int{103}},
			// The name 1234 Hosting.
			{path: "/api/net?name_search=1234", want: []int{103}},
			// AND around digits is removed: clean_term is 64500.
			{path: "/api/net?name_search=AND%2064500", want: []int{100}},
			// upstream: search_v2.py:892-896, :618-623 (the digit test
			// runs before the IPv6 query, so 645: is an ASN query)
			{path: "/api/net?name_search=645:", want: []int{100, 101}},
			{path: "/api/net?name_search=2001:", want: []int{}},
		})
	})

	t.Run("non_ascii_digits", func(t *testing.T) {
		t.Parallel()
		// upstream: search_v2.py:619 (str.isdigit), :385, :393 (int()),
		// rest.py:824-827 (ValueError is 400). CPython int() accepts the
		// Unicode Nd digits of any script, rejects ² (a digit that is
		// not decimal), and rejects more than 4300 digits.
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=%D9%A6%D9%A4%D9%A5%D9%A0%D9%A0", want: []int{100}},
			// No search index on poc: search_v2 is never called.
			{path: "/api/poc?name_search=%C2%B2", want: []int{}},
		})
		assertFilterError(t, srv, "/api/net?name_search=%C2%B2", "invalid literal for int() with base 10")
		assertFilterError(t, srv, "/api/net?name_search="+strings.Repeat("1", 4301), "Exceeds the limit (4300 digits)")
	})

	t.Run("ip_prefix", func(t *testing.T) {
		t.Parallel()
		// upstream: search_v2.py:427-522 (the prefix clause on the .raw
		// address), documents.py:140-176 (net: live netixlans; ix: live
		// netixlans of ok ixlans; other types have no address)
		c := seedNameSearch(t, t0)
		// Netixlan 5003 (ok) is on ixlan 3002 of ix 301, which is
		// deleted: the ix index skips it (documents.py:150-157), the
		// net index does not check the ixlan (:147-149).
		c.IxLan.Create().
			SetID(3002).SetName("AMS-IX Old LAN").SetIxID(301).
			SetStatus("deleted").SetCreated(t0).SetUpdated(t0).SaveX(t.Context())
		c.NetworkIxLan.Create().
			SetID(5003).SetNetID(103).SetIxlanID(3002).SetIxID(301).
			SetAsn(65001).SetSpeed(10000).SetIpaddr4("80.82.1.1").
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(t.Context())
		srv := newTestServer(t, c)
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/ix?name_search=80.82", want: []int{}},
			{path: "/api/net?name_search=80.82", want: []int{103}},
			{path: "/api/net?name_search=80.81.192", want: []int{100}},
			// Netixlan 5001 is deleted.
			{path: "/api/net?name_search=80.81.193", want: []int{}},
			// Netixlan 5002 is not-operational, a live status.
			{path: "/api/net?name_search=80.249", want: []int{103}},
			{path: "/api/ix?name_search=80.81", want: []int{300}},
			{path: "/api/fac?name_search=80.81", want: []int{}},
			{path: "/api/net?name_search=2001:7f8:", want: []int{100}},
			{path: "/api/ix?name_search=2001:7F8", want: []int{300}},
		})
	})

	t.Run("non_indexed_types_empty", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:536-553. The 7 types without a search index
		// return qset.none() before finalize_query_params and the
		// filter loop, so a bad value in a model-field key, id__in
		// included, does not raise.
		srv := newTestServer(t, seedNameSearch(t, t0))
		cases := []silentIgnoreCase{
			{path: "/api/netixlan?name_search=x&speed=abc", want: []int{}},
			{path: "/api/poc?name_search=x&id__in=abc", want: []int{}},
			{path: "/api/netixlan?name_search=x&ipaddr6__in=", want: []int{}},
		}
		for _, typ := range []string{"poc", "ixlan", "ixpfx", "netixlan", "netfac", "ixfac", "carrierfac"} {
			cases = append(cases, silentIgnoreCase{path: "/api/" + typ + "?name_search=x", want: []int{}})
		}
		assertNameSearchIDs(t, srv, cases)
	})

	t.Run("prepare_query_errors_win", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:488-500 (prepare_query and its 400) runs
		// before name_search (:532-553), so a prepare_query error is a
		// 400 also when name_search matches no row. zzz is a search
		// with no hit, AND a value that can match nothing, and the
		// non-indexed types never search.
		srv := newTestServer(t, seedNameSearch(t, t0))
		for _, tc := range []struct{ path, wantErr string }{
			{"/api/netixlan?name_search=x&ix=abc", "filter ix:"},
			// models.py:5191: ipaddress.ip_address raises ValueError.
			{"/api/ixpfx?name_search=x&whereis=abc", "filter whereis:"},
			{"/api/ix?name_search=AND&capacity=abc", `"abc" is not an integer`},
			// serializers.py:2119-2124: the fac net_count seed.
			{"/api/fac?name_search=zzz&net_count=abc", "filter net_count:"},
			{"/api/fac?name_search=AND&net_count=abc", "filter net_count:"},
			{"/api/fac?name_search=AND&net_count__gt=abc", "filter net_count__gt:"},
			// models.py:2457-2458: one ASN.
			{"/api/fac?name_search=zzz&asn_overlap=64500", "filter asn_overlap:"},
			{"/api/fac?name_search=AND&asn_overlap=64500", "filter asn_overlap:"},
			// A relation key with an unknown field (FieldError).
			{"/api/net?name_search=AND&ix__bogus=1", "Invalid query"},
			// A search runs and an empty id__in matches no row, but
			// prepare_query fails first.
			{"/api/fac?name_search=x&id__in=&asn_overlap=64500", "filter asn_overlap:"},
			{"/api/net?name_search=alpha&id__in=&not_ix=abc", "filter not_ix:"},
			// search_v2 raises ValueError on the digit ² (:385, :393)
			// after prepare_query, so the prepare_query error wins.
			{"/api/net?name_search=%C2%B2&not_ix=abc", "filter not_ix:"},
			// A key of the filter loop is read after name_search.
			{"/api/net?name_search=%C2%B2&asn__lt=abc", "filter name_search:"},
			// since is parsed before name_search (rest.py:505-510).
			{"/api/poc?name_search=x&since=abc", "'since' needs to be a unix timestamp"},
		} {
			assertFilterError(t, srv, tc.path, tc.wantErr)
		}
		// The bare netixlan ipaddr6 key is a model-field key: it is
		// not read.
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/netixlan?name_search=x&ipaddr6=abc", want: []int{}},
		})
	})

	t.Run("empty_value_and_operators", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:535 (an empty value is falsy: no search).
		// synthesised: a value of white space or an operator only
		// gives the index an empty query; the mirror matches no row.
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=", want: []int{100, 101, 103}},
			{path: "/api/poc?name_search=", want: []int{600}},
			{path: "/api/net?name_search=%20", want: []int{}},
			{path: "/api/net?name_search=AND", want: []int{}},
		})
	})

	t.Run("key_forms", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:533 (query_params.get returns the last
		// value, django/utils/datastructures.py:116-125); :616-630,
		// :670 (name_search__contains is not a field key: ignored)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=zzz&name_search=alpha", want: []int{100}},
			{path: "/api/net?name_search__contains=alpha", want: []int{100, 101, 103}},
		})
	})

	t.Run("id_in_union", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:663-666 (id__in), :685-690 (the search ids
		// are appended to id__in), :550-553 (no hit: qset.none()
		// first), :718-748 (status matrix)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=alpha&id__in=101", want: []int{100, 101}},
			{path: "/api/net?name_search=zzz&id__in=101", want: []int{}},
			// Net 102 (Alpha Deleted) is deleted: the matrix drops the
			// listed id, and a match is an ok row only.
			{path: "/api/net?name_search=alpha&id__in=102", want: []int{100}},
			{path: "/api/net?name_search=alpha&id__in=102&since=1", want: []int{100, 102}},
			// Net 102 matches alpha but is deleted: since does not
			// admit it through the search.
			{path: "/api/net?name_search=alpha&id__in=101&since=1", want: []int{100, 101}},
			// The other filters AND with the union.
			{path: "/api/net?name_search=645&name__contains=beta", want: []int{101}},
		})
		assertFilterError(t, srv, "/api/net?name_search=alpha&id__in=abc", "filter id__in:")
	})

	t.Run("id_key_ands_and_404", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:683 (id is id__iexact: it ANDs), :809-815
		// (a unique query with no row is 404 Entity not found)
		srv := newTestServer(t, seedNameSearch(t, t0))
		for _, path := range []string{
			"/api/net?name_search=alpha&id=101",
			"/api/poc?name_search=x&id=600",
		} {
			status, body := httpGet(t, srv, path)
			if status != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404; body=%s", path, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != "Entity not found" {
				t.Errorf("%s: meta.error = %q, want Entity not found", path, got)
			}
		}
	})

	t.Run("ok_only_with_since", func(t *testing.T) {
		t.Parallel()
		// upstream: documents.py:97-136 and search_v2.py:774 (the index
		// holds ok objects only), rest.py:718-746 (since admits deleted
		// and, on campus, pending)
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=alpha&since=1", want: []int{100}},
			{path: "/api/campus?name_search=alpha&since=1", want: []int{400}},
			{path: "/api/campus?name_search=alpha", want: []int{400}},
		})
	})

	t.Run("negative_skip", func(t *testing.T) {
		t.Parallel()
		// upstream: rest.py:550-553 returns qset.none() before the
		// slice (:755-760), so a negative skip raises no error when the
		// search has no hit: a list is 200 [] (or the unique-query 404,
		// :809-815), a single-object GET the miss 404. With a hit,
		// Django rejects the negative slice (query.py:410-417), and
		// list() returns 400 (:824-827).
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=zzz&skip=-1", want: []int{}},
			{path: "/api/net?name_search=AND&skip=-1", want: []int{}},
			{path: "/api/poc?name_search=x&skip=-1", want: []int{}},
			{path: "/api/net?name_search=zzz&id__in=100&skip=-1", want: []int{}},
		})
		assertFilterError(t, srv, "/api/net?name_search=alpha&skip=-1", "Negative indexing is not supported.")
		// A hit that other filters exclude still slices.
		assertFilterError(t, srv, "/api/net?name_search=alpha&asn=1&skip=-1", "Negative indexing is not supported.")
		assertFilterError(t, srv, "/api/fac/200?name_search=equinix&skip=-1", "Negative indexing is not supported.")
		for _, tc := range []struct{ path, wantErr string }{
			{"/api/net?name_search=zzz&id=100&skip=-1", "Entity not found"},
			{"/api/fac/200?name_search=zzz&skip=-1", "No Facility matches the given query."},
			{"/api/poc/600?name_search=x&skip=-1", "No NetworkContact matches the given query."},
			// int() of the pk fails on the none() queryset too.
			{"/api/fac/abc?name_search=zzz&skip=-1", "Not found."},
		} {
			status, body := httpGet(t, srv, tc.path)
			if status != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404; body=%s", tc.path, status, string(body))
				continue
			}
			if got := mustDecodeMetaError(t, body).Error; got != tc.wantErr {
				t.Errorf("%s: meta.error = %q, want %q", tc.path, got, tc.wantErr)
			}
		}
	})

	t.Run("many_words_one_array", func(t *testing.T) {
		t.Parallel()
		// synthesised: the words bind as one JSON array, so a long
		// value adds no SQL term per word (no SQLite expression-depth
		// error).
		srv := newTestServer(t, seedNameSearch(t, t0))
		words := strings.TrimSuffix(strings.Repeat("alpha%20", 2000), "%20")
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=" + words, want: []int{100}},
		})
	})

	t.Run("DIVERGENCE_name_search_substring_approximation", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream searches an Elasticsearch index. The
		// mirror keeps the rows where each word is a case-insensitive
		// substring of one indexed field.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: search_v2.py:525-596 (name query), :359-424 (ASN
		// query), :618 (clean_term removes AND inside a word),
		// documents.py:52-65 (analyzers). The 1000-hit cap
		// (settings/__init__.py:1516, search_v2.py:962-967) is in the
		// row only: a test would need 1001 hits.
		c := seedNameSearch(t, t0)
		ctx := t.Context()
		mustNet(ctx, t, c, 104, "Rocket Net", 64520, 2, t0)
		mustNet(ctx, t, c, 105, "MÜNCHEN IX", 64521, 2, t0)
		c.Network.Create().
			SetID(106).SetName("Gamma Networks").SetNameFold(unifold.Fold("Gamma Networks")).
			SetAka("Gamma Net").SetAsn(64522).SetOrgID(2).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		srv := newTestServer(t, c)
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			// Upstream: the ASN query matches a name that starts with
			// 5; Equinix FR5 does not.
			{path: "/api/fac?name_search=5", want: []int{200}},
			// Upstream: OR makes either word enough ([100, 101]).
			{path: "/api/net?name_search=alpha%20OR%20beta", want: []int{}},
			// Upstream: clean_term R prefix-matches Rocket Net.
			{path: "/api/net?name_search=RAND", want: []int{}},
			// Upstream: the ES lowercase filter matches MÜNCHEN IX.
			{path: "/api/net?name_search=m%C3%BCnchen", want: []int{}},
			// Upstream: the phrase clause matches the aka tokens gamma
			// and net of net 106.
			{path: "/api/net?name_search=gamma-net", want: []int{}},
		})
	})

	t.Run("DIVERGENCE_name_search_error_status", func(t *testing.T) {
		t.Parallel()
		// DIVERGENCE: upstream answers 500 for a partial IPv4 address
		// with an octet above 255 (the search term stays a list, and
		// construct_query_body fails on it), and 200 [] for a search
		// with no hit and a bad model-field value (get_queryset
		// returns before the filter loop). The mirror searches the
		// address as text, and checks every filter value before it
		// runs the query.
		// See docs/API.md § Known Divergences.
		// This test ASSERTS the divergence (it is NOT a parity match).
		// upstream: search_v2.py:888-891, :618; rest.py:550-553
		srv := newTestServer(t, seedNameSearch(t, t0))
		assertNameSearchIDs(t, srv, []silentIgnoreCase{
			{path: "/api/net?name_search=999.1.1.1", want: []int{}},
		})
		assertFilterError(t, srv, "/api/net?name_search=nomatch&asn__lt=abc", "filter asn__lt:")
		assertFilterError(t, srv, "/api/net?name_search=nomatch&id__in=abc", "filter id__in:")
	})
}

// assertNameSearchIDs checks that each request returns HTTP 200 and
// exactly the wanted ids, in any order.
func assertNameSearchIDs(t *testing.T, srv *httptest.Server, cases []silentIgnoreCase) {
	t.Helper()
	for _, tc := range cases {
		status, body := httpGet(t, srv, tc.path)
		label := tc.path
		if len(label) > 120 {
			label = label[:120] + "..."
		}
		if status != http.StatusOK {
			t.Errorf("%s: status = %d, want 200; body=%s", label, status, string(body))
			continue
		}
		got := extractIDs(t, body)
		slices.Sort(got)
		want := slices.Sorted(slices.Values(tc.want))
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", label, got, want)
		}
	}
}

// seedNameSearch seeds the rows of TestParity_NameSearch:
//
//   - org 1 Search Org (city Frankfurt), org 2 Other Org.
//   - net 100 Alpha Networks (asn 64500, aka AlphaNet, irr_as_set
//     AS-ALPHA), net 101 Beta Carrier Net (asn 64510, name_long Beta
//     Long), net 102 Alpha Deleted (asn 64501, deleted), net 103 1234
//     Hosting (asn 65001).
//   - fac 200 Equinix FR5 (Frankfurt), fac 201 Telehouse North (London).
//   - ix 300 DE-CIX Frankfurt with ixlan 3000, ix 301 AMS-IX
//     (Amsterdam) with ixlan 3001.
//   - netixlan 5000 (net 100, ixlan 3000, 80.81.192.10,
//     2001:7f8::fbf0:0:1, ok), 5001 (net 101, ixlan 3001, 80.81.193.5,
//     deleted), 5002 (net 103, ixlan 3001, 80.249.208.1,
//     not-operational).
//   - campus 400 Alpha Campus (ok), campus 401 Alpha Pending (pending).
//   - carrier 500 Alpha Carrier.
//   - poc 600 (Public), ixpfx 700, netfac 800, ixfac 900 and carrierfac
//     1000, so the empty results of the 7 types without a search index
//     are not an empty table.
func seedNameSearch(t *testing.T, t0 time.Time) *ent.Client {
	t.Helper()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	c.Organization.Create().
		SetID(1).SetName("Search Org").SetNameFold(unifold.Fold("Search Org")).
		SetCity("Frankfurt").SetCityFold(unifold.Fold("Frankfurt")).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mustOrg(ctx, t, c, 2, "Other Org", t0)

	c.Network.Create().
		SetID(100).SetName("Alpha Networks").SetNameFold(unifold.Fold("Alpha Networks")).
		SetAka("AlphaNet").SetIrrAsSet("AS-ALPHA").SetAsn(64500).SetOrgID(1).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	c.Network.Create().
		SetID(101).SetName("Beta Carrier Net").SetNameFold(unifold.Fold("Beta Carrier Net")).
		SetNameLong("Beta Long").SetAsn(64510).SetOrgID(2).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	c.Network.Create().
		SetID(102).SetName("Alpha Deleted").SetNameFold(unifold.Fold("Alpha Deleted")).
		SetAsn(64501).SetOrgID(1).
		SetStatus("deleted").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mustNet(ctx, t, c, 103, "1234 Hosting", 65001, 2, t0)

	for _, f := range []struct {
		id         int
		name, city string
		org        int
	}{{200, "Equinix FR5", "Frankfurt", 1}, {201, "Telehouse North", "London", 2}} {
		c.Facility.Create().
			SetID(f.id).SetName(f.name).SetNameFold(unifold.Fold(f.name)).
			SetCity(f.city).SetCityFold(unifold.Fold(f.city)).SetCountry("DE").SetOrgID(f.org).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	}
	for _, x := range []struct {
		id, lan    int
		name, city string
	}{{300, 3000, "DE-CIX Frankfurt", "Frankfurt"}, {301, 3001, "AMS-IX", "Amsterdam"}} {
		c.InternetExchange.Create().
			SetID(x.id).SetName(x.name).SetNameFold(unifold.Fold(x.name)).
			SetCity(x.city).SetCityFold(unifold.Fold(x.city)).SetCountry("DE").
			SetRegionContinent("Europe").SetMedia("Ethernet").SetOrgID(1).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
		mustIxLan(ctx, t, c, x.lan, x.name+" LAN", x.id, t0)
	}
	for _, n := range []struct {
		id, net, lan, ix int
		ip4, ip6, status string
	}{
		{5000, 100, 3000, 300, "80.81.192.10", "2001:7f8::fbf0:0:1", "ok"},
		{5001, 101, 3001, 301, "80.81.193.5", "", "deleted"},
		{5002, 103, 3001, 301, "80.249.208.1", "", "not-operational"},
	} {
		create := c.NetworkIxLan.Create().
			SetID(n.id).SetNetID(n.net).SetIxlanID(n.lan).SetIxID(n.ix).
			SetAsn(64500).SetSpeed(10000).SetIpaddr4(n.ip4).
			SetStatus(n.status).SetCreated(t0).SetUpdated(t0)
		if n.ip6 != "" {
			create.SetIpaddr6(n.ip6)
		}
		create.SaveX(ctx)
	}

	mustCampus(ctx, t, c, 400, "Alpha Campus", 1, t0)
	c.Campus.Create().
		SetID(401).SetName("Alpha Pending").SetNameFold(unifold.Fold("Alpha Pending")).SetOrgID(1).
		SetStatus("pending").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	c.Carrier.Create().
		SetID(500).SetName("Alpha Carrier").SetNameFold(unifold.Fold("Alpha Carrier")).SetOrgID(1).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)

	c.Poc.Create().
		SetID(600).SetNetID(100).SetRole("NOC").SetName("Alpha NOC").SetVisible("Public").
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	mustIxPfx(ctx, t, c, 700, "80.81.192.0/22", 3000, t0)
	c.NetworkFacility.Create().
		SetID(800).SetNetID(100).SetFacID(200).SetLocalAsn(64500).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	c.IxFacility.Create().
		SetID(900).SetIxID(300).SetFacID(200).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	c.CarrierFacility.Create().
		SetID(1000).SetCarrierID(500).SetFacID(200).
		SetStatus("ok").SetCreated(t0).SetUpdated(t0).SaveX(ctx)
	return c
}
