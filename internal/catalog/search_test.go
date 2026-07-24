package catalog

import "testing"

func TestParseASNQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int64
		ok   bool
	}{
		{name: "plain", in: "13335", want: 13335, ok: true},
		{name: "prefixed", in: "AS13335", want: 13335, ok: true},
		{name: "spaced prefix", in: "as 13335"},
		{name: "zero", in: "0"},
		{name: "too large", in: "4294967296"},
		{name: "name", in: "Cloudflare"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseASNQuery(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseASNQuery(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTrimPage(t *testing.T) {
	t.Parallel()

	got, more := trimPage([]int{1, 2, 3}, 2)
	if !more {
		t.Fatal("trimPage more = false, want true")
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("trimPage result = %v, want [1 2]", got)
	}
}
