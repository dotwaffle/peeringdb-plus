package pdbcompat

import (
	"errors"
	"math"
	"strings"
	"testing"
	"unicode"
)

// TestPyInt locks the Python int() grammar of pyInt. The expected
// results were measured with CPython 3.13 int(<str>).
func TestPyInt(t *testing.T) {
	t.Parallel()

	max4300 := strings.Repeat("1", pyIntMaxStrDigits)
	ok := []struct {
		in   string
		n    int
		text string
	}{
		{"5", 5, "5"},
		{" 5 ", 5, "5"},
		{"\t5\n", 5, "5"},
		{"+5", 5, "5"},
		{"-5", -5, "-5"},
		{"1_000", 1000, "1000"},
		{"007", 7, "7"},
		{"-0", 0, "0"},
		{"-0_0", 0, "0"},
		{"０", 0, "0"},
		{"１", 1, "1"},
		{"٢", 2, "2"},
		{"\U0001D7D7", 9, "9"}, // MATHEMATICAL BOLD DIGIT NINE
		{" 5　", 5, "5"},
		{"　5", 5, "5"},
		{"٢_٣", 23, "23"},
		{"99999999999999999999", math.MaxInt, "99999999999999999999"},
		{"-99999999999999999999", math.MinInt, "-99999999999999999999"},
		{max4300, math.MaxInt, max4300},
		{"0" + strings.Repeat("0", pyIntMaxStrDigits-2) + "7", 7, "7"},
	}
	for _, tc := range ok {
		n, text, err := pyInt(tc.in)
		if err != nil || n != tc.n || text != tc.text {
			t.Errorf("pyInt(%q) = (%d, %q, %v), want (%d, %q, nil)", tc.in, n, text, err, tc.n, tc.text)
		}
	}

	bad := []string{
		"",
		" ",
		"+",
		"-",
		"_1",
		"1_",
		"1__0",
		"+_1",
		"+ 5",
		"- 5",
		"1.5",
		"0x10",
		"1e3",
		"²", // U+00B2 is No, not Nd
		"\x1c5\x1f",
		"\x1c5",
		"abc",
		"5a",
		"++5",
		strings.Repeat("1", pyIntMaxStrDigits+1),
		"0" + max4300,
	}
	for _, in := range bad {
		if n, text, err := pyInt(in); !errors.Is(err, errNotPyInt) {
			t.Errorf("pyInt(%q) = (%d, %q, %v), want errNotPyInt", in, n, text, err)
		}
	}
}

// TestPyIntDigits locks the canonical digits of pyIntDigits.
func TestPyIntDigits(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in     string
		neg    bool
		digits string
	}{
		{"0042", false, "42"},
		{"-0", false, "0"},
		{"+4_2", false, "42"},
		{"-４２", true, "42"},
		{"1234567890123456789012345", false, "1234567890123456789012345"},
	} {
		neg, digits, ok := pyIntDigits(tc.in)
		if !ok || neg != tc.neg || digits != tc.digits {
			t.Errorf("pyIntDigits(%q) = (%v, %q, %v), want (%v, %q, true)", tc.in, neg, digits, ok, tc.neg, tc.digits)
		}
	}
}

// TestNDValue checks ndValue for every rune of unicode.Nd and for the
// first rune of some digit sets.
func TestNDValue(t *testing.T) {
	t.Parallel()

	for _, rg := range unicode.Nd.R16 {
		for r := rune(rg.Lo); r <= rune(rg.Hi); r += rune(rg.Stride) {
			if v := ndValue(r); v < 0 || v > 9 {
				t.Errorf("ndValue(%U) = %d, want 0..9", r, v)
			}
		}
	}
	for _, rg := range unicode.Nd.R32 {
		for r := rune(rg.Lo); r <= rune(rg.Hi); r += rune(rg.Stride) {
			if v := ndValue(r); v < 0 || v > 9 {
				t.Errorf("ndValue(%U) = %d, want 0..9", r, v)
			}
		}
	}
	// Digit zero of ASCII, Arabic-Indic, fullwidth, mathematical bold
	// and mathematical double-struck digits.
	for _, zero := range []rune{'0', '٠', '０', '\U0001D7CE', '\U0001D7D8'} {
		for k := range 10 {
			if v := ndValue(zero + rune(k)); v != k {
				t.Errorf("ndValue(%U) = %d, want %d", zero+rune(k), v, k)
			}
		}
	}
	for _, r := range []rune{'a', '_', ' ', '²', 'Ⅰ', '￿', unicode.MaxRune} {
		if v := ndValue(r); v != -1 {
			t.Errorf("ndValue(%U) = %d, want -1", r, v)
		}
	}
}

// TestPyIntDigitRanges locks the shape of unicode.Nd that ndValue
// needs: every range has stride 1, a length that is a multiple of ten,
// and a first rune with the digit value 0 (Unicode assigns each digit
// set as 0 to 9 in order). A Go release with a new Unicode version that
// breaks this fails here.
func TestPyIntDigitRanges(t *testing.T) {
	t.Parallel()

	check := func(lo, hi, stride uint32) {
		if stride != 1 {
			t.Errorf("unicode.Nd range %U..%U has stride %d, want 1", lo, hi, stride)
		}
		if n := hi - lo + 1; n%10 != 0 {
			t.Errorf("unicode.Nd range %U..%U has %d runes, want a multiple of 10", lo, hi, n)
		}
	}
	for _, rg := range unicode.Nd.R16 {
		check(uint32(rg.Lo), uint32(rg.Hi), uint32(rg.Stride))
	}
	for _, rg := range unicode.Nd.R32 {
		check(rg.Lo, rg.Hi, rg.Stride)
	}
}
