package sqlitesearch

import "testing"

func TestTokens(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ value, want string }{
		{"Gamma-Net", "gamma net"},
		{"Frankfurt 51", "frankfurt 51"},
		{"MÜNCHEN", "münchen"},
		{"Köln", "köln"},
		{"A\u0308 B", "a\u0308 b"},
		{"  . ,  ", ""},
		{"x') OR 1=1 --", "x or 1 1"},
	} {
		if got := Tokens(tc.value); got != tc.want {
			t.Errorf("Tokens(%q)=%q, want %q", tc.value, got, tc.want)
		}
	}
}
