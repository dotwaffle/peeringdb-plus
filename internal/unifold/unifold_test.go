package unifold

import (
	"strings"
	"testing"
)

func TestFold(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"ascii-lower", "hello", "hello"},
		{"ascii-upper", "HELLO", "hello"},
		{"diacritic-u-umlaut", "Zürich", "zurich"},
		{"all-caps-umlaut", "ZÜRICH", "zurich"},
		{"round-trip-ascii-vs-diacritic", "zurich", "zurich"},
		{"ligature-eszett", "Straße", "strasse"},
		{"ligature-ae", "Æsir", "aesir"},
		{"ligature-oslash", "Øresund", "oresund"},
		{"ligature-lstroke", "Łódź", "lodz"},
		{"ligature-thorn", "Þorvaldur", "thorvaldur"},
		{"ligature-dstroke", "ĐàNẵng", "danang"},
		{"cjk-passthrough", "日本語", "日本語"},
		{"combining-e-acute", "e\u0301", "e"},
		{"combining-a-diaeresis", "a\u0308", "a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Fold(tc.in)
			if got != tc.want {
				t.Fatalf("Fold(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFold_RoundTrip locks the canonical UNICODE-01 invariant: an ASCII query
// and its diacritic-bearing counterpart fold to the same value, and that value
// is itself idempotent under Fold.
func TestFold_RoundTrip(t *testing.T) {
	t.Parallel()
	a := Fold("Zürich")
	b := Fold("zurich")
	if a != b {
		t.Fatalf("Fold(Zürich)=%q != Fold(zurich)=%q", a, b)
	}
	if a != "zurich" {
		t.Fatalf("Fold(Zürich)=%q, want %q", a, "zurich")
	}
	if Fold(a) != a {
		t.Fatalf("Fold not idempotent: Fold(%q)=%q", a, Fold(a))
	}
}

// TestFold_NoPanic exercises the contract that Fold is total: any UTF-8 input
// (including invalid bytes, control characters, ZWJ sequences, RTL scripts,
// or zalgo strings with thousands of combining marks on a single base) must
// return without panicking. This underwrites Plan 69-05's fuzz harness.
func TestFold_NoPanic(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"\x00\xff\xfe",                       // invalid UTF-8 + null
		strings.Repeat("A", 70_000),          // long string >64 KB
		"\u0001\u0002\u0003",                 // control chars
		"👨\u200d👩\u200d👧",                    // ZWJ family emoji
		"שלום עברית",                         // RTL Hebrew
		"\u202e\u200f",                       // RTL/LTR overrides
		"a" + strings.Repeat("\u0301", 1000), // zalgo: many combining marks on one base
	}
	for _, in := range inputs {
		// Contract: Fold must not panic on any input. We do not assert
		// the output value — see TestFold for behavioural cases.
		_ = Fold(in)
	}
}
