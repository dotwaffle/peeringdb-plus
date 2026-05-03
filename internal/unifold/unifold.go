// Package unifold folds Unicode strings into a normalised ASCII-lowercase
// form suitable for diacritic-insensitive equality and substring matching.
//
// The pipeline is: hand-mapped ligature substitution → NFKD normalisation
// (golang.org/x/text/unicode/norm) → drop combining marks (unicode.Mn) →
// ToLower. The hand map covers non-decomposable diacritics that NFKD alone
// leaves intact (ß, æ, ø, ł, þ, đ, and their upper-case variants).
//
// Scope: reproduces upstream PeeringDB's `unidecode.unidecode(v)` behaviour
// (rest.py:576) closely enough for filter-value matching in the pdbcompat
// layer. It is NOT a full Unicode-to-ASCII transliteration library — CJK,
// Arabic, Hebrew, and other non-Latin scripts pass through untouched so
// that foreign-language substring matches still work against the folded
// DB column. UNICODE-01 depends on the SAME fold being applied on both
// sides of the comparison; this package is the single source of truth.
package unifold

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// foldMap covers non-decomposable ligatures and stroke-letters that NFKD
// leaves unchanged. Both upper- and lower-case variants are mapped so the
// substitution is order-independent with respect to ToLower.
//
// Sourced from upstream `unidecode` behaviour for these specific code
// points; keep the list tight — expanding beyond the 6 pairs locked in
// Phase 69 D-02 requires a new decision.
var foldMap = map[rune]string{
	'ß': "ss", // U+00DF LATIN SMALL LETTER SHARP S
	'ẞ': "ss", // U+1E9E LATIN CAPITAL LETTER SHARP S
	'æ': "ae",
	'Æ': "ae",
	'ø': "o",
	'Ø': "o",
	'ł': "l",
	'Ł': "l",
	'þ': "th",
	'Þ': "th",
	'đ': "d",
	'Đ': "d",
}

// Fold normalises s for case-insensitive, diacritic-insensitive matching.
//
// Fold is total: any UTF-8 input — including invalid UTF-8 bytes, null
// bytes, control characters, combining marks, ZWJ sequences, RTL text,
// or strings exceeding 64 KB — returns a string without panicking. The
// empty string maps to the empty string.
//
// Fast-path contract: an input whose bytes are all within the 7-bit
// ASCII range (< 0x80) and contain no upper-case letters ('A'-'Z') is
// returned unchanged without allocating. The set admits digits,
// punctuation, and control characters in addition to 'a'-'z' — any
// such input is idempotent under the full fold pipeline (NFKD is a
// no-op on ASCII, the Mn guard never fires, ToLower is identity on
// non-letters and lower-case letters). This makes Fold cheap for the
// common case of folding an already-folded value (e.g. when the sync
// worker reads back a value it has just persisted into a `_fold`
// column).
func Fold(s string) string {
	if s == "" {
		return ""
	}
	if asciiLowerFastPath(s) {
		return s
	}
	// Phase 1: hand-map non-decomposable ligatures BEFORE NFKD so that
	// e.g. ß → "ss" is applied as a unit rather than after a no-op
	// decomposition. Allocates an intermediate builder sized to the
	// input; substitutions are bounded (longest expansion is "th").
	var sub strings.Builder
	sub.Grow(len(s))
	for _, r := range s {
		if rep, ok := foldMap[r]; ok {
			sub.WriteString(rep)
			continue
		}
		sub.WriteRune(r)
	}
	// Phase 2: NFKD decompose, then drop combining marks (Mn category)
	// and lower-case the survivors. NFKD turns "é" into "e\u0301"; the
	// Mn guard strips the combining acute. unicode.ToLower is a no-op
	// on already-lowercase code points (including all of CJK).
	decomposed := norm.NFKD.String(sub.String())
	var out strings.Builder
	out.Grow(len(decomposed))
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}

// asciiLowerFastPath reports whether every byte of s is in the ASCII
// range and not an upper-case letter. Strings passing this check are
// idempotent under Fold and can be returned without allocating.
//
// The check is intentionally conservative: any non-ASCII byte (≥ 0x80)
// or upper-case letter ('A'-'Z') forces the full pipeline. This avoids
// having to enumerate every code point that Fold leaves untouched.
func asciiLowerFastPath(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c >= 0x80 {
			return false
		}
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}
