package pdbcompat

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseNameSearch(t *testing.T) {
	t.Parallel()
	ones := func(n int) string { return strings.Repeat("1", n) }
	tests := []struct {
		in      string
		mode    nameSearchMode
		words   []string
		digits  string
		asn     int64
		hasASN  bool
		prefix  string
		wantErr string
	}{
		{in: "Equinix FR5", mode: nsWords, words: []string{"equinix", "fr5"}},
		{in: "a AND b", mode: nsWords, words: []string{"a", "b"}},
		{in: "a OR b", mode: nsWords, words: []string{"a", "b"}},
		{in: "AND", mode: nsNone},
		{in: "  ", mode: nsNone},
		{in: "AND OR", mode: nsNone},
		{in: "13335", mode: nsASN, digits: "13335", asn: 13335, hasASN: true},
		{in: "AND 13335", mode: nsASN, digits: "13335", asn: 13335, hasASN: true},
		{in: "13335 AND", mode: nsASN, digits: "13335", asn: 13335, hasASN: true},
		// clean_term also removes AND inside a word (search_v2.py:618).
		{in: "12AND3", mode: nsASN, digits: "123", asn: 123, hasASN: true},
		{in: "013335", mode: nsASN, digits: "013335", asn: 13335, hasASN: true},
		{in: "99999999999999999999", mode: nsASN, digits: "99999999999999999999"},
		{in: "OR 13335", mode: nsWords, words: []string{"13335"}},
		// The escape backslash stays in clean_term, so a star between
		// digits is not a digit value.
		{in: "12*3", mode: nsWords, words: []string{"12*3"}},
		{in: "٦٤٥٠٠", mode: nsASN, digits: "٦٤٥٠٠", asn: 64500, hasASN: true},
		{in: "1٢", mode: nsASN, digits: "1٢", asn: 12, hasASN: true},
		{in: "²", wantErr: "invalid literal for int() with base 10: '²'"},
		{in: "①", wantErr: "invalid literal for int() with base 10: '①'"},
		{in: ones(4300), mode: nsASN, digits: ones(4300)},
		{in: ones(4301), wantErr: "Exceeds the limit (4300 digits) for integer string conversion: value has 4301 digits"},
		{in: "½", mode: nsWords, words: []string{"½"}},
		{in: "195.66", mode: nsIPv4, prefix: "195.66"},
		{in: "195.66.", mode: nsIPv4, prefix: "195.66."},
		{in: " 1.2.3.4 ", mode: nsIPv4, prefix: "1.2.3.4"},
		{in: "999.1.1.1", mode: nsWords, words: []string{"999.1.1.1"}},
		{in: "1.2.3.4.5", mode: nsWords, words: []string{"1.2.3.4.5"}},
		{in: "2001:", mode: nsASN, digits: "2001", asn: 2001, hasASN: true},
		{in: "6939:", mode: nsASN, digits: "6939", asn: 6939, hasASN: true},
		{in: "13:", mode: nsASN, digits: "13", asn: 13, hasASN: true},
		{in: "0:", mode: nsASN, digits: "0", asn: 0, hasASN: true},
		{in: "2001:7f8:", mode: nsIPv6, prefix: "2001:7f8"},
		{in: "2001:DB8::", mode: nsIPv6, prefix: "2001:db8::"},
		{in: "1234::", mode: nsIPv6, prefix: "1234::"},
		{in: "dead:", mode: nsIPv6, prefix: "dead"},
		{in: "dead:beef", mode: nsIPv6, prefix: "dead:beef"},
		{in: "de:cix", mode: nsWords, words: []string{"de:cix"}},
		{in: "de-cix", mode: nsWords, words: []string{"de-cix"}},
		{in: "12345:", mode: nsWords, words: []string{"12345:"}},
		// Python str.split also splits on U+001C..U+001F.
		{in: "alpha\x1cbeta", mode: nsWords, words: []string{"alpha", "beta"}},
	}
	for _, tt := range tests {
		got, err := parseNameSearch(tt.in)
		label := tt.in
		if len(label) > 20 {
			label = label[:20] + "..."
		}
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseNameSearch(%q): err = %v, want %q", label, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNameSearch(%q): err = %v", label, err)
			continue
		}
		if got.mode != tt.mode || !slices.Equal(got.words, tt.words) || got.digits != tt.digits ||
			got.asn != tt.asn || got.hasASN != tt.hasASN || got.prefix != tt.prefix {
			t.Errorf("parseNameSearch(%q) = %+v, want mode=%d words=%v digits=%q asn=%d hasASN=%v prefix=%q",
				label, got, tt.mode, tt.words, tt.digits, tt.asn, tt.hasASN, tt.prefix)
		}
	}
}

func TestIPPrefixUpperBound(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"195.66":     "195.67",
		"2001:7f8":   "2001:7f9",
		"2001:db8::": "2001:db8:;",
		"::":         ":;",
		"80.81.":     "80.81/",
	} {
		if got := ipPrefixUpperBound(in); got != want {
			t.Errorf("ipPrefixUpperBound(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveNameSearch_Consumes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		typ      string
		params   url.Values
		consumed []string
		pred     bool
		none     bool
		empty    bool
		wantErr  string
	}{
		{name: "absent", typ: "net", params: url.Values{"id__in": {"1"}}},
		{name: "empty value", typ: "net", params: url.Values{"name_search": {""}}, consumed: []string{"name_search"}},
		{name: "words", typ: "net", params: url.Values{"name_search": {"alpha"}}, consumed: []string{"name_search"}, pred: true},
		{name: "last value", typ: "net", params: url.Values{"name_search": {"alpha", ""}}, consumed: []string{"name_search"}},
		{name: "id__in union", typ: "net", params: url.Values{"name_search": {"alpha"}, "id__in": {"1,2"}}, consumed: []string{"id__in", "name_search"}, pred: true},
		{name: "empty id__in", typ: "net", params: url.Values{"name_search": {"alpha"}, "id__in": {""}}, consumed: []string{"id__in", "name_search"}, empty: true},
		{name: "bad id__in", typ: "net", params: url.Values{"name_search": {"alpha"}, "id__in": {"abc"}}, wantErr: "filter id__in:"},
		{name: "no index", typ: "poc", params: url.Values{"name_search": {"x"}, "id__in": {"abc"}}, consumed: []string{"name_search"}, none: true},
		{name: "no index not parsed", typ: "poc", params: url.Values{"name_search": {"²"}}, consumed: []string{"name_search"}, none: true},
		{name: "operator only", typ: "net", params: url.Values{"name_search": {"AND"}, "id__in": {"abc"}}, consumed: []string{"name_search"}, none: true},
		{name: "ip without ip fields", typ: "fac", params: url.Values{"name_search": {"80.81"}}, consumed: []string{"name_search"}, none: true},
		{name: "bad digit", typ: "net", params: url.Values{"name_search": {"²"}}, wantErr: "filter name_search: invalid literal"},
	}
	for _, tt := range tests {
		res, err := resolveNameSearch(Registry[tt.typ], tt.params)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: err = %v", tt.name, err)
			continue
		}
		var consumed []string
		for k := range res.consumed {
			consumed = append(consumed, k)
		}
		slices.Sort(consumed)
		if !slices.Equal(consumed, tt.consumed) || (res.pred != nil) != tt.pred || res.none != tt.none || res.empty != tt.empty {
			t.Errorf("%s: consumed=%v pred=%v none=%v empty=%v, want consumed=%v pred=%v none=%v empty=%v",
				tt.name, consumed, res.pred != nil, res.none, res.empty, tt.consumed, tt.pred, tt.none, tt.empty)
		}
	}
}

func TestIsPrepareQueryKey(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		typ, key string
		want     bool
	}{
		{"netixlan", "ix", true},
		{"netixlan", "ix__name__contains", true},
		{"net", "not_ix", true},
		{"fac", "asn_overlap", true},
		{"ix", "ipblock", true},
		{"ixpfx", "whereis", true},
		{"ixpfx", "whereis__in", true},
		{"ix", "capacity", true},
		{"ix", "capacity__gte", true},
		{"fac", "net_count", true},
		{"fac", "net_count__gt", true},
		{"ix", "fac_count__lte", true},
		{"netixlan", "speed", false},
		{"netixlan", "ipaddr6", false},
		{"netixlan", "meta__rfc8950", false},
		{"net", "info_type", false},
		{"net", "asn__lt", false},
		{"net", "id__in", false},
		{"ix", "whereis", false},
		{"fac", "fac_count", false},
		{"carrier", "fac_count", false},
	} {
		if got := isPrepareQueryKey(Registry[tt.typ], tt.key); got != tt.want {
			t.Errorf("isPrepareQueryKey(%s, %s) = %v, want %v", tt.typ, tt.key, got, tt.want)
		}
	}
}

// TestParseListFilters_NameSearchNone checks the no-hit path: when
// name_search can match no row, the result is empty, only the
// prepare_query keys are read, and their errors win.
func TestParseListFilters_NameSearchNone(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		typ     string
		query   string
		wantErr string
	}{
		{"netixlan", "name_search=x&speed=abc&meta__rfc8950__lt=x", ""},
		{"netixlan", "name_search=x&ipaddr6=abc", ""},
		{"poc", "name_search=x&id__in=abc&unknown=1", ""},
		{"net", "name_search=AND&asn__lt=abc", ""},
		{"netixlan", "name_search=x&ix=abc", "filter ix:"},
		{"ixpfx", "name_search=x&whereis=abc", "filter whereis:"},
		{"ix", "name_search=AND&capacity=abc", "filter capacity:"},
		{"fac", "name_search=AND&net_count=abc", "filter net_count:"},
		{"fac", "name_search=AND&net_count__gt=abc", "filter net_count__gt:"},
		{"fac", "name_search=AND&asn_overlap=64500", "filter asn_overlap:"},
		{"net", "name_search=AND&ix__bogus=1", "filter ix__bogus:"},
	} {
		params, err := url.ParseQuery(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		ctx := WithUnknownFields(t.Context())
		lf, err := parseListFilters(ctx, params, Registry[tt.typ])
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s?%s: err = %v, want %q", tt.typ, tt.query, err, tt.wantErr)
			}
			continue
		}
		if err != nil || !lf.emptyResult || !lf.none || lf.preds != nil {
			t.Errorf("%s?%s: lf=%+v err=%v, want an empty result with none", tt.typ, tt.query, lf, err)
		}
		if unknown := UnknownFieldsFromCtx(ctx); len(unknown) != 0 {
			t.Errorf("%s?%s: unknown fields %v, want none", tt.typ, tt.query, unknown)
		}
	}
}

// TestNameSearchPlan checks that the name_search predicates keep the
// list on its order index. The IP modes read the netixlan address
// index; with since they sort the few matched rows. The id__in gate
// is an uncorrelated scalar subquery (a plan row that starts with
// SCALAR SUBQUERY); the words predicate has correlated ones.
func TestNameSearchPlan(t *testing.T) {
	t.Parallel()
	since := time.Unix(1, 0)
	tests := []struct {
		name      string
		typ       string
		query     string
		since     bool
		wantIndex string
		tempOK    bool
		wantGate  bool
	}{
		{name: "net words", typ: "net", query: "name_search=alpha", wantIndex: "network_status"},
		{name: "net words since", typ: "net", query: "name_search=alpha", since: true, wantIndex: "network_updated"},
		{name: "net ipv4", typ: "net", query: "name_search=80.81", wantIndex: "networkixlan_ipaddr4"},
		{name: "ix ipv4", typ: "ix", query: "name_search=80.81", wantIndex: "networkixlan_ipaddr4"},
		{name: "net ipv6", typ: "net", query: "name_search=2001:7f8:", wantIndex: "networkixlan_ipaddr6"},
		{name: "net ipv4 since", typ: "net", query: "name_search=80.81", since: true, wantIndex: "networkixlan_ipaddr4", tempOK: true},
		{name: "net words id__in", typ: "net", query: "name_search=alpha&id__in=1,2", wantIndex: "network_status", wantGate: true},
	}
	for _, tt := range tests {
		params, err := url.ParseQuery(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		preds, empty, err := ParseFiltersCtx(t.Context(), params, Registry[tt.typ])
		if err != nil || empty || len(preds) != 1 {
			t.Fatalf("%s: preds=%d empty=%v err=%v, want one predicate", tt.name, len(preds), empty, err)
		}
		opts := QueryOptions{Filters: preds, Limit: 250}
		if tt.since {
			opts.Since = &since
		}
		list, _ := listPlans(t, tt.typ, opts)
		if !strings.Contains(list, tt.wantIndex) {
			t.Errorf("%s: list plan = %q, want %s", tt.name, list, tt.wantIndex)
		}
		if !tt.tempOK && strings.Contains(list, "TEMP B-TREE") {
			t.Errorf("%s: list plan = %q, want no temp B-tree", tt.name, list)
		}
		gate := false
		for row := range strings.SplitSeq(list, " | ") {
			if strings.HasPrefix(row, "SCALAR SUBQUERY") {
				gate = true
			}
		}
		if gate != tt.wantGate {
			t.Errorf("%s: list plan = %q, uncorrelated scalar subquery = %v, want %v", tt.name, list, gate, tt.wantGate)
		}
	}
}
