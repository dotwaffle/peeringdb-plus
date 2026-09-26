package pdbcompat

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// pyIntMaxStrDigits is the CPython default of
// sys.int_info.default_max_str_digits (Python 3.11 and later). int()
// raises ValueError for a str with more digits than this. Leading zeros
// count, underscores and white space do not.
const pyIntMaxStrDigits = 4300

// asciiDigits maps a digit value to its ASCII digit.
const asciiDigits = "0123456789"

// errNotPyInt reports a value that Python int() does not accept.
var errNotPyInt = errors.New("not an integer")

// ndValue returns the value (0 to 9) of a Unicode Nd (decimal digit)
// rune, or -1 when r is not in unicode.Nd.
//
// Unicode assigns the Nd characters only in runs of ten, 0 to 9 in
// order, and every range of unicode.Nd has stride 1 and a length that is
// a multiple of ten (TestPyIntDigitRanges locks this). So the value is
// the distance from the start of the range, modulo ten. This is the
// value of unicodedata.decimal, which int() uses.
func ndValue(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	if r < 0x80 {
		return -1
	}
	c := int(r)
	for _, rg := range unicode.Nd.R16 {
		lo, hi := int(rg.Lo), int(rg.Hi)
		if c < lo {
			return -1
		}
		if c <= hi {
			return (c - lo) % 10
		}
	}
	for _, rg := range unicode.Nd.R32 {
		lo, hi := int(rg.Lo), int(rg.Hi)
		if c < lo {
			return -1
		}
		if c <= hi {
			return (c - lo) % 10
		}
	}
	return -1
}

// pyIntDigits parses s as CPython int(s) parses a str in base 10. It
// returns the sign and the canonical ASCII digits (no leading zeros, "0"
// for zero). ok is false when int(s) raises ValueError.
//
// The grammar: white space at both ends (unicode.IsSpace, the same code
// points that int() strips; U+001C..U+001F are not white space for
// int()), one optional '+' or '-', then one or more Unicode Nd digits
// with a single '_' allowed between two digits. Leading zeros are
// allowed. More than pyIntMaxStrDigits digits is an error.
func pyIntDigits(s string) (neg bool, digits string, ok bool) {
	s = strings.TrimFunc(s, unicode.IsSpace)
	if s != "" && (s[0] == '+' || s[0] == '-') {
		neg = s[0] == '-'
		s = s[1:]
	}
	var b strings.Builder
	count := 0
	prevDigit := false
	prevUnderscore := false
	for _, r := range s {
		if r == '_' {
			if !prevDigit {
				return false, "", false
			}
			prevDigit, prevUnderscore = false, true
			continue
		}
		d := ndValue(r)
		if d < 0 {
			return false, "", false
		}
		count++
		if count > pyIntMaxStrDigits {
			return false, "", false
		}
		// Drop leading zeros.
		if b.Len() > 0 || d != 0 {
			b.WriteByte(asciiDigits[d])
		}
		prevDigit, prevUnderscore = true, false
	}
	if count == 0 || prevUnderscore {
		return false, "", false
	}
	if b.Len() == 0 {
		return false, "0", true
	}
	return neg, b.String(), true
}

// pyInt parses v as Python int(v) parses a str in base 10 (see
// pyIntDigits for the grammar). Upstream uses int() for since, skip,
// limit and depth (2.83.0 rest.py:505-522, api_cache.py:77-80) and
// Django uses it for every integer lookup value
// (IntegerField.get_prep_value, db/models/fields/__init__.py:2123-2131).
//
// n is the value, saturated to math.MinInt or math.MaxInt when it does
// not fit: upstream parses any size. text is the canonical decimal form
// of the whole value (ASCII digits, '-' for a negative value, no '+', no
// '_', no leading zeros, "0" for zero), for messages that print the
// value. err is errNotPyInt when int() raises ValueError.
func pyInt(v string) (n int, text string, err error) {
	neg, digits, ok := pyIntDigits(v)
	if !ok {
		return 0, "", errNotPyInt
	}
	text = digits
	if neg {
		text = "-" + digits
	}
	// ParseInt returns the saturated value with ErrRange, the only
	// error it can return for canonical digits.
	parsed, _ := strconv.ParseInt(text, 10, strconv.IntSize)
	return int(parsed), text, nil
}
