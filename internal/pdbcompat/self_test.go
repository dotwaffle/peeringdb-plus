package pdbcompat

import "testing"

func TestSelfTag(t *testing.T) {
	t.Parallel()
	// Expected values from Python re.search with the upstream pattern
	// (2.83.0 rest.py:2088).
	cases := map[string]string{
		"net/self":         "net",
		"net/self/":        "net",
		"net/selfie":       "net",
		"ixfac/self":       "fac",
		"netfac/self":      "fac",
		"carrierfac/self":  "fac",
		"foo/org/self":     "org",
		"ix/self.json":     "ix",
		"campus/self":      "campus",
		"carrier/self":     "carrier",
		"xnet/self":        "net",
		"org/selfnet/self": "org",
		"net/1/self":       "",
		"nets/self":        "",
		"ixlan/self":       "",
		"as_set/self":      "",
		"NET/self":         "",
		"net/Self":         "",
		"net/1":            "",
	}
	for rest, want := range cases {
		got, ok := selfTag(rest)
		if ok != (want != "") || got != want {
			t.Errorf("selfTag(%q) = %q, %v; want %q", rest, got, ok, want)
		}
	}
}

func TestIRIToURI(t *testing.T) {
	t.Parallel()
	// Expected values from Django 5.2 iri_to_uri on the ISO-8859-1
	// decoded bytes (django/utils/encoding.py:107-135).
	cases := map[string]string{
		"a=1&b=2":               "a=1&b=2",
		`x="<>`:                 "x=%22%3C%3E",
		"q=\xe9":                "q=%C3%A9",
		"p=%zz%20":              "p=%zz%20",
		"c=^`{|}\\":             "c=%5E%60%7B%7C%7D%5C",
		"d=a b":                 "d=a%20b",
		"e=[]#@!$()*+,;:'~/?":   "e=[]#@!$()*+,;:'~/?",
		"f=\x7f\x01":            "f=%7F%01",
		"g=%C3%A9_-.~":          "g=%C3%A9_-.~",
		"h=\xc3\xa9":            "h=%C3%83%C2%A9",
		"":                      "",
		"UPPER=lower&0123=4567": "UPPER=lower&0123=4567",
	}
	for in, want := range cases {
		if got := iriToURI(in); got != want {
			t.Errorf("iriToURI(%q) = %q, want %q", in, got, want)
		}
	}
}
