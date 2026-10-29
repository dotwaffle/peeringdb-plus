package sqlitesearch

import "testing"

func TestJSONText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ value, want string }{
		{`{ "z": 1, "a": ["München", "<x>"] }`, `{"a":["München","<x>"],"z":1}`},
		{`{"a":9007199254740993}`, `{"a":9007199254740993}`},
		{`[ ]`, `[]`},
		{`"a b"`, `"a b"`},
		{`twitter`, `twitter`},
		{`{"x":`, `{"x":`},
	} {
		if got := JSONText(tc.value); got != tc.want {
			t.Errorf("JSONText(%q) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
