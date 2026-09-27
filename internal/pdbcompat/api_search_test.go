package pdbcompat

import (
	"slices"
	"testing"
)

func TestSearchTerms(t *testing.T) {
	t.Parallel()
	// Expected values from the upstream extract_query,
	// process_near_search and process_in_search (2.83.0
	// views.py:3623-4098), run with no geocoder and no search index.
	cases := []struct{ q, want []string }{
		{[]string{"equinix in frankfurt"}, []string{"equinix"}},
		{[]string{"equinix near 50.1,8.6"}, []string{"equinix"}},
		{[]string{"equinix near 50.1, 8.6, 5km foo"}, []string{"equinix foo"}},
		{[]string{"a near b near 1.5,2.5 c"}, []string{"a near b c"}},
		{[]string{"x IN y", "z in"}, []string{"x", "z"}},
		{[]string{"near"}, []string{""}},
		{[]string{"in"}, []string{""}},
		{[]string{"foo, bar in x"}, []string{"foo,bar"}},
		{[]string{"near 91.0,1.0 x"}, []string{""}},
		{[]string{"NeAr 1.0,2.0"}, []string{""}},
		{[]string{"a near 1.0,2.0 b near c"}, []string{"a b near c"}},
		{[]string{"abc"}, []string{"abc"}},
		{[]string{"q in a in b"}, []string{"q"}},
		{[]string{"foo,  bar"}, []string{"foo,  bar"}},
	}
	for _, tc := range cases {
		if got := searchTerms(tc.q); !slices.Equal(got, tc.want) {
			t.Errorf("searchTerms(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
}

func TestSearchExactTerms(t *testing.T) {
	t.Parallel()
	// look_for_exact_matches (search_v2.py:897-909) lower-cased, plus
	// their join (:319-323).
	cases := map[string][]string{
		"Equinix FR5":     {"equinix", "fr5", "equinix fr5"},
		"a-b":             {`a\-b`, `a\-b`},
		"x AND y OR z":    {"x", "y", "z", "x y z"},
		"10.0.0":          {""},
		"2001:db8":        {""},
		"64500":           {"64500", "64500"},
		"":                {""},
		"and":             {"and", "and"},
		"Equinix  (FR5)?": {"equinix", `\(fr5\)\?`, `equinix \(fr5\)\?`},
	}
	for qs, want := range cases {
		if got := searchExactTerms(qs); !slices.Equal(got, want) {
			t.Errorf("searchExactTerms(%q) = %q, want %q", qs, got, want)
		}
	}
}

func TestSearchOrderHits(t *testing.T) {
	t.Parallel()
	// order_results_alphabetically (search_v2.py:298-356) with no
	// score: the input is in lower-case name order.
	hit := func(order, id int, name string) searchHit {
		return searchHit{ID: id, Name: name, order: order}
	}
	names := func(list []searchHit) []string {
		var out []string
		for _, h := range list {
			out = append(out, h.Name)
		}
		return out
	}
	hits := []searchHit{hit(0, 1, "Alpha"), hit(2, 5, "Beta Net"), hit(0, 2, "Beta"), hit(0, 3, "beta"), hit(0, 4, "Gamma Beta")}

	got := searchOrderHits(slices.Clone(hits), []string{"beta", "beta"}, "beta")
	if want := []string{"Beta", "Alpha", "beta", "Gamma Beta"}; !slices.Equal(names(got[0]), want) {
		t.Errorf("exact: fac = %q, want %q", names(got[0]), want)
	}
	if want := []string{"Beta Net"}; !slices.Equal(names(got[2]), want) {
		t.Errorf("exact: net = %q, want %q", names(got[2]), want)
	}
	if len(got) != len(searchTypes) || len(got[1]) != 0 {
		t.Errorf("exact: %d types, ix = %v; want %d types and no ix", len(got), got[1], len(searchTypes))
	}

	got = searchOrderHits(slices.Clone(hits), nil, "alpha OR  Gamma OR x")
	if want := []string{"Gamma Beta", "Alpha", "Beta", "beta"}; !slices.Equal(names(got[0]), want) {
		t.Errorf("OR: fac = %q, want %q", names(got[0]), want)
	}
}

func TestPyJSONString(t *testing.T) {
	t.Parallel()
	// Expected values from json.dumps(s, ensure_ascii=False).
	cases := map[string]string{
		`a"b\\c`:                    `"a\"b\\\\c"`,
		"\n\r\t\b\f\x01\x1f":        `"\n\r\t\b\f\u0001\u001f"`,
		"é  <>&\x7f":                "\"é  <>&\x7f\"",
		"日本":                        `"日本"`,
		"":                          `""`,
		"invalid literal for int()": `"invalid literal for int()"`,
	}
	for in, want := range cases {
		if got := pyJSONString(in); got != want {
			t.Errorf("pyJSONString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPyLatin1Fields(t *testing.T) {
	t.Parallel()
	// Expected values from len(s.split()) on the ISO-8859-1 text.
	cases := map[string]int{
		"Api-Key x":    2,
		"Api-Key  x ":  2,
		"Api-Key\x85x": 2,
		"Api-Key\xa0x": 2,
		"a\x1cb\x1dc":  3,
		"single":       1,
		"a b c":        3,
		"\x0bx y":      2,
	}
	for in, want := range cases {
		if got := len(pyLatin1Fields(in)); got != want {
			t.Errorf("len(pyLatin1Fields(%q)) = %d, want %d", in, got, want)
		}
	}
}

func TestPyCommaSpace(t *testing.T) {
	t.Parallel()
	// re.sub(r",\s*", ",", s).
	cases := map[string]string{
		"a, b":   "a,b",
		"a,\t　b": "a,b",
		"a ,b":   "a ,b",
		",, x":   ",,x",
		"none":   "none",
	}
	for in, want := range cases {
		if got := pyCommaSpace(in); got != want {
			t.Errorf("pyCommaSpace(%q) = %q, want %q", in, got, want)
		}
	}
}
