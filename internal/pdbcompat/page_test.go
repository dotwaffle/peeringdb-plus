package pdbcompat

import (
	"crypto/tls"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPerPage(t *testing.T) {
	t.Parallel()
	// DRF _positive_int(strict=True, cutoff=250) with the fallback to
	// PAGE_SIZE (pagination.py:23-32, :256-264).
	cases := map[string]int{
		"":                                 250,
		"per_page=10":                      10,
		"per_page=250":                     250,
		"per_page=251":                     250,
		"per_page=0":                       250,
		"per_page=-1":                      250,
		"per_page=abc":                     250,
		"per_page=":                        250,
		"per_page=%201_0%20":               10,
		"per_page=5&per_page=7":            7,
		"per_page=99999999999999999999999": 250,
	}
	for raw, want := range cases {
		params, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := perPage(params); got != want {
			t.Errorf("perPage(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestResolvePage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		value       string
		count, size int
		page, pages int
		ok          bool
	}{
		{"", 0, 250, 1, 1, true},
		{"1", 0, 250, 1, 1, true},
		{"2", 0, 250, 0, 1, false},
		{"last", 0, 250, 1, 1, true},
		{"last", 501, 250, 3, 3, true},
		{"3", 501, 250, 3, 3, true},
		{"4", 501, 250, 0, 3, false},
		{"0", 10, 5, 0, 2, false},
		{"-1", 10, 5, 0, 2, false},
		{"abc", 10, 5, 0, 2, false},
		{"1.0", 10, 5, 0, 2, false},
		{" 2 ", 10, 5, 2, 2, true},
		{"Last", 10, 5, 0, 2, false},
		{"99999999999999999999999", 10, 5, 0, 2, false},
	}
	for _, c := range cases {
		page, pages, ok := resolvePage(c.value, c.count, c.size)
		if page != c.page || pages != c.pages || ok != c.ok {
			t.Errorf("resolvePage(%q, %d, %d) = %d, %d, %v, want %d, %d, %v",
				c.value, c.count, c.size, page, pages, ok, c.page, c.pages, c.ok)
		}
	}
}

func TestPageValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw    string
		cached bool
		value  string
		ok     bool
	}{
		{"", false, "", false},
		{"page=", false, "", true},
		{"page=", true, "", false},
		{"page=2&page=", true, "", false},
		{"page=&page=2", true, "2", true},
		{"page=3", true, "3", true},
	}
	for _, c := range cases {
		params, err := url.ParseQuery(c.raw)
		if err != nil {
			t.Fatal(err)
		}
		value, ok := pageValue(params, c.cached)
		if value != c.value || ok != c.ok {
			t.Errorf("pageValue(%q, %v) = %q, %v, want %q, %v", c.raw, c.cached, value, ok, c.value, c.ok)
		}
	}
}

// TestPageLink compares the links with the output of the DRF helpers
// replace_query_param and remove_query_param (CPython 3.14 urllib).
func TestPageLink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		page int
		want string
	}{
		{"page=2&asn=1&name=a+b", 3, "http://example.com/api/net?asn=1&name=a+b&page=3"},
		{"b=1&a=2&b=0&page=2", 1, "http://example.com/api/net?a=2&b=1&b=0"},
		{"page=2", 1, "http://example.com/api/net"},
		{"x=%zz&y=%E2%82A&z=%FF%FE&w=a;b&v&&page=1", 2, "http://example.com/api/net?page=2&v=&w=a%3Bb&x=%25zz&y=%EF%BF%BDA&z=%EF%BF%BD%EF%BF%BD"},
		{"name__in=a,b&q=%C3%A9+x/y:z~", 2, "http://example.com/api/net?name__in=a%2Cb&page=2&q=%C3%A9+x%2Fy%3Az~"},
		{"%ED%A0%80=1&k=%F0%90%80&page=last", 2, "http://example.com/api/net?k=%EF%BF%BD&page=2&%EF%BF%BD%EF%BF%BD%EF%BF%BD=1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://example.com/api/net?"+c.raw, nil)
		if got := *pageLink(r, c.page); got != c.want {
			t.Errorf("pageLink(%q, %d) = %q, want %q", c.raw, c.page, got, c.want)
		}
	}
}

func TestRequestScheme(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "http://example.com/api/net", nil)
	if got := requestScheme(r); got != "http" {
		t.Errorf("plain: %q", got)
	}
	r.TLS = &tls.ConnectionState{}
	if got := requestScheme(r); got != "https" {
		t.Errorf("tls: %q", got)
	}
	r.TLS = nil
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := requestScheme(r); got != "https" {
		t.Errorf("forwarded: %q", got)
	}
}

// TestPyDecodeUTF8 compares with bytes.decode("utf-8", "replace") of
// CPython 3.14: one U+FFFD for each maximal subpart.
func TestPyDecodeUTF8(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"\xe2\x82A":        "�A",
		"\xff\xfe":         "��",
		"\xed\xa0\x80":     "���",
		"\xf0\x90\x80":     "�",
		"\xf4\x90\x80\x80": "����",
		"\xc0\xaf":         "��",
		"a\xe0\x80b":       "a��b",
		"\xf0\x9f\x98\x80": "\U0001F600",
		"plain":            "plain",
	}
	for in, want := range cases {
		if got := pyDecodeUTF8([]byte(in)); got != want {
			t.Errorf("pyDecodeUTF8(%q) = %q, want %q", in, got, want)
		}
	}
}
