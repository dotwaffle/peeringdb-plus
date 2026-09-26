package pdbcompat

import (
	"errors"
	"fmt"
	"math"
	"regexp"
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

// pyFloatKind classifies a str for Python float().
type pyFloatKind int

const (
	// pyFloatInvalid is a value that float() rejects with ValueError.
	pyFloatInvalid pyFloatKind = iota
	// pyFloatFinite is a finite value.
	pyFloatFinite
	// pyFloatInf is an infinite value: inf, infinity, or a number too
	// large for a float, such as 1e999.
	pyFloatInf
	// pyFloatNaN is nan.
	pyFloatNaN
)

// pyFloatLiteral is the Python float literal grammar without the sign:
// digits with single underscores between them, an optional fraction and
// an optional exponent (Python doc behavior, "Floating-point literals").
// A value with no digit before the point needs one after it.
var pyFloatLiteral = regexp.MustCompile(`^(?:[0-9](?:_?[0-9])*(?:\.(?:[0-9](?:_?[0-9])*)?)?|\.[0-9](?:_?[0-9])*)(?:[eE][+-]?[0-9](?:_?[0-9])*)?$`)

// classifyPyFloat returns the pyFloatKind of float(s). float() strips
// white space as int() does, reads any Unicode Nd digit, and accepts
// inf, infinity and nan in any case, with a sign.
func classifyPyFloat(s string) pyFloatKind {
	s = strings.TrimFunc(s, unicode.IsSpace)
	var b strings.Builder
	for _, r := range s {
		if d := ndValue(r); d >= 0 {
			b.WriteByte(asciiDigits[d])
			continue
		}
		b.WriteRune(r)
	}
	t := b.String()
	body := t
	if body != "" && (body[0] == '+' || body[0] == '-') {
		body = body[1:]
	}
	switch strings.ToLower(body) {
	case "inf", "infinity":
		return pyFloatInf
	case "nan":
		return pyFloatNaN
	}
	if !pyFloatLiteral.MatchString(body) {
		return pyFloatInvalid
	}
	// The grammar check leaves only values that ParseFloat reads. A
	// value out of range is +Inf or -Inf with ErrRange.
	f, _ := strconv.ParseFloat(strings.ReplaceAll(t, "_", ""), 64)
	if math.IsInf(f, 0) {
		return pyFloatInf
	}
	return pyFloatFinite
}

// pyIntValueError returns the message of the ValueError that Python
// int(s) raises in base 10: invalid literal for int() with base 10:
// followed by repr(s).
func pyIntValueError(s string) string {
	return "invalid literal for int() with base 10: " + pyRepr(s)
}

// pyRepr returns the Python repr of the str s: single quotes, or double
// quotes when s holds a single quote and no double quote. A backslash,
// the quote character, \t, \n, \r and every rune that Python does not
// print are escaped.
func pyRepr(s string) string {
	quote := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == '\\' || r == rune(quote):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case !pyPrintable(r):
			switch {
			case r < 0x100:
				fmt.Fprintf(&b, `\x%02x`, r)
			case r < 0x10000:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// pyPrintable reports whether Python str.isprintable is true for r: a
// rune in the Unicode categories other than Other and Separator, or the
// ASCII space.
func pyPrintable(r rune) bool {
	if r == ' ' {
		return true
	}
	return !unicode.In(r, unicode.C, unicode.Z)
}
