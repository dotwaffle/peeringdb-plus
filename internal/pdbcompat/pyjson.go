package pdbcompat

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// This file ports the parts of CPython 3.13 that the DRF JSONParser
// uses on a request body (DRF 3.18.1 parsers.py:53-68): the decode of
// the body with the request charset (codecs StreamReader, which leaves
// an incomplete sequence at the end undecoded), and json.loads with the
// C scanner (Modules/_json.c) and DRF strict_constant. The error texts
// are the Python texts, which DRF returns as "JSON parse error - <text>".
// The results were checked against CPython 3.13.5 (pyjson_test.go);
// upstream runs Python 3.14.

// pyKind is the Python type of a decoded JSON value.
type pyKind int

const (
	pyNoneKind pyKind = iota
	pyBoolKind
	pyIntValue
	pyFloatValue
	pyStrValue
	pyListValue
	pyDictValue
	// pyReprValue is an object that only shows as its repr, held in
	// str: an uploaded file.
	pyReprValue
)

// pyValue is a value that json.loads returns. A str is kept as runes, so
// that a lone surrogate from a \u escape stays as it is.
type pyValue struct {
	kind pyKind
	b    bool
	// num is the Python str() of an int or float.
	num  string
	str  []rune
	list []pyValue
	// keys and vals are the items of a dict, in insertion order. A
	// repeated key keeps its first position and takes the last value,
	// as dict(pairs) does.
	keys [][]rune
	vals []pyValue
}

// get returns the value of the dict key name.
func (v pyValue) get(name string) (pyValue, bool) {
	for i, k := range v.keys {
		if string(k) == name {
			return v.vals[i], true
		}
	}
	return pyValue{}, false
}

// pyStr returns Python str(v): the text of a str, and the repr of every
// other value.
func (v pyValue) pyStr() []rune {
	if v.kind == pyStrValue {
		return v.str
	}
	return []rune(v.repr())
}

// repr returns Python repr(v).
func (v pyValue) repr() string {
	switch v.kind {
	case pyNoneKind:
		return "None"
	case pyBoolKind:
		if v.b {
			return "True"
		}
		return "False"
	case pyIntValue, pyFloatValue:
		return v.num
	case pyStrValue:
		return pyReprRunes(v.str)
	case pyReprValue:
		return string(v.str)
	case pyListValue:
		parts := make([]string, len(v.list))
		for i, e := range v.list {
			parts[i] = e.repr()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case pyDictValue:
		parts := make([]string, len(v.keys))
		for i, k := range v.keys {
			parts[i] = pyReprRunes(k) + ": " + v.vals[i].repr()
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

// pyJSONError is a json.JSONDecodeError: msg at the rune index pos of
// doc.
type pyJSONError struct {
	msg string
	doc []rune
	pos int
}

func (e *pyJSONError) Error() string {
	line := 1
	lastNL := -1
	for i := 0; i < e.pos; i++ {
		if e.doc[i] == '\n' {
			line++
			lastNL = i
		}
	}
	return fmt.Sprintf("%s: line %d column %d (char %d)", e.msg, line, e.pos-lastNL, e.pos)
}

// pyValueError is a ValueError that is not a JSONDecodeError, with its
// Python text.
type pyValueError string

func (e pyValueError) Error() string { return string(e) }

// pyJSONMaxDepth is the nesting depth above which the mirror stops, as
// the C scanner raises RecursionError near the C recursion limit.
// RecursionError is not a ValueError, so DRF does not catch it and
// upstream answers 500 (errPyJSONDepth).
const pyJSONMaxDepth = 1000

// errPyJSONDepth is the error of a document nested deeper than
// pyJSONMaxDepth.
var errPyJSONDepth = pyValueError("maximum recursion depth exceeded")

// pyJSONLoads parses doc as json.loads(doc, parse_constant=
// strict_constant) does.
func pyJSONLoads(doc []rune) (pyValue, error) {
	if len(doc) > 0 && doc[0] == 0xfeff {
		return pyValue{}, &pyJSONError{msg: "Unexpected UTF-8 BOM (decode using utf-8-sig)", doc: doc, pos: 0}
	}
	p := pyJSONParser{doc: doc}
	idx := p.skipSpace(0)
	v, end, err := p.scanOnce(idx, 0)
	if err != nil {
		return pyValue{}, err
	}
	end = p.skipSpace(end)
	if end != len(doc) {
		return pyValue{}, p.fail("Extra data", end)
	}
	return v, nil
}

// pyJSONParser holds the document of one pyJSONLoads call.
type pyJSONParser struct {
	doc []rune
}

func (p *pyJSONParser) fail(msg string, pos int) error {
	return &pyJSONError{msg: msg, doc: p.doc, pos: pos}
}

// isSpace reports whether r is JSON white space.
func pyJSONSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

// skipSpace returns the index of the first rune at or after i that is
// not JSON white space.
func (p *pyJSONParser) skipSpace(i int) int {
	for i < len(p.doc) && pyJSONSpace(p.doc[i]) {
		i++
	}
	return i
}

// at returns the rune at i, or -1 past the end.
func (p *pyJSONParser) at(i int) rune {
	if i < len(p.doc) {
		return p.doc[i]
	}
	return -1
}

// hasPrefix reports whether the document holds s at i.
func (p *pyJSONParser) hasPrefix(i int, s string) bool {
	for _, r := range s {
		if p.at(i) != r {
			return false
		}
		i++
	}
	return true
}

// scanOnce parses the value at idx (scan_once_unicode). A position with
// no value is "Expecting value" at idx.
func (p *pyJSONParser) scanOnce(idx, depth int) (pyValue, int, error) {
	if idx >= len(p.doc) {
		return pyValue{}, 0, p.fail("Expecting value", idx)
	}
	switch c := p.doc[idx]; {
	case c == '"':
		s, end, err := p.scanString(idx + 1)
		return pyValue{kind: pyStrValue, str: s}, end, err
	case c == '{' || c == '[':
		if depth >= pyJSONMaxDepth {
			return pyValue{}, 0, errPyJSONDepth
		}
		if c == '{' {
			return p.parseObject(idx+1, depth+1)
		}
		return p.parseArray(idx+1, depth+1)
	case c == 'n' && p.hasPrefix(idx, "null"):
		return pyValue{kind: pyNoneKind}, idx + 4, nil
	case c == 't' && p.hasPrefix(idx, "true"):
		return pyValue{kind: pyBoolKind, b: true}, idx + 4, nil
	case c == 'f' && p.hasPrefix(idx, "false"):
		return pyValue{kind: pyBoolKind}, idx + 5, nil
	case c == 'N' && p.hasPrefix(idx, "NaN"):
		return pyValue{}, 0, pyStrictConstant("NaN")
	case c == 'I' && p.hasPrefix(idx, "Infinity"):
		return pyValue{}, 0, pyStrictConstant("Infinity")
	case c == '-' && p.hasPrefix(idx, "-Infinity"):
		return pyValue{}, 0, pyStrictConstant("-Infinity")
	}
	return p.matchNumber(idx)
}

// pyStrictConstant is the ValueError of DRF strict_constant
// (utils/json.py:18-19).
func pyStrictConstant(name string) error {
	return pyValueError("Out of range float values are not JSON compliant: " + pyRepr(name))
}

// pyIntMaxDigits is sys.int_info.default_max_str_digits.
const pyIntMaxDigits = 4300

// matchNumber parses the number at start (_match_number_unicode).
func (p *pyJSONParser) matchNumber(start int) (pyValue, int, error) {
	idx := start
	if p.at(idx) == '-' {
		idx++
	}
	isDigit := func(r rune) bool { return '0' <= r && r <= '9' }
	switch c := p.at(idx); {
	case '1' <= c && c <= '9':
		for isDigit(p.at(idx)) {
			idx++
		}
	case c == '0':
		idx++
	default:
		return pyValue{}, 0, p.fail("Expecting value", start)
	}
	isFloat := false
	if p.at(idx) == '.' && isDigit(p.at(idx+1)) {
		isFloat = true
		idx += 2
		for isDigit(p.at(idx)) {
			idx++
		}
	}
	if c := p.at(idx); c == 'e' || c == 'E' {
		eStart := idx
		idx++
		if c := p.at(idx); c == '+' || c == '-' {
			idx++
		}
		digits := idx
		for isDigit(p.at(idx)) {
			idx++
		}
		if idx > digits {
			isFloat = true
		} else {
			idx = eStart
		}
	}
	text := string(p.doc[start:idx])
	if isFloat {
		f, _ := strconv.ParseFloat(text, 64)
		return pyValue{kind: pyFloatValue, num: pyFloatRepr(f)}, idx, nil
	}
	digits := strings.TrimPrefix(text, "-")
	if len(digits) > pyIntMaxDigits {
		return pyValue{}, 0, pyValueError(fmt.Sprintf(
			"Exceeds the limit (%d digits) for integer string conversion: value has %d digits; use sys.set_int_max_str_digits() to increase the limit",
			pyIntMaxDigits, len(digits)))
	}
	if digits == "0" {
		text = "0"
	}
	return pyValue{kind: pyIntValue, num: text}, idx, nil
}

// pyFloatRepr returns Python repr(f): the shortest text that reads back
// as f, in fixed notation when the decimal exponent is from -4 to 15
// (with ".0" for a whole number), else in exponent notation with at
// least two exponent digits.
func pyFloatRepr(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // -d.ddde±dd
	mant, expText, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expText)
	sign := ""
	if strings.HasPrefix(mant, "-") {
		sign, mant = "-", mant[1:]
	}
	digits := strings.Replace(mant, ".", "", 1)
	if exp < -4 || exp >= 16 {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		es := "+"
		if exp < 0 {
			es, exp = "-", -exp
		}
		return fmt.Sprintf("%s%se%s%02d", sign, m, es, exp)
	}
	if exp < 0 {
		return sign + "0." + strings.Repeat("0", -exp-1) + digits
	}
	if len(digits) <= exp+1 {
		return sign + digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
	}
	return sign + digits[:exp+1] + "." + digits[exp+1:]
}

// scanString parses the string whose first rune after the opening quote
// is at end (scanstring_unicode, strict).
func (p *pyJSONParser) scanString(end int) ([]rune, int, error) {
	begin := end - 1
	var out []rune
	for {
		if end >= len(p.doc) {
			return nil, 0, p.fail("Unterminated string starting at", begin)
		}
		c := p.doc[end]
		switch {
		case c == '"':
			return out, end + 1, nil
		case c <= 0x1f:
			return nil, 0, p.fail("Invalid control character at", end)
		case c != '\\':
			out = append(out, c)
			end++
			continue
		}
		end++
		if end >= len(p.doc) {
			return nil, 0, p.fail("Unterminated string starting at", begin)
		}
		esc := p.doc[end]
		if esc != 'u' {
			r, ok := map[rune]rune{'"': '"', '\\': '\\', '/': '/', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t'}[esc]
			if !ok {
				return nil, 0, p.fail("Invalid \\escape", end-1)
			}
			out = append(out, r)
			end++
			continue
		}
		next := end + 1
		end = next + 4
		if end >= len(p.doc) {
			return nil, 0, p.fail("Invalid \\uXXXX escape", next-1)
		}
		u, ok := p.hex4(next)
		if !ok {
			return nil, 0, p.fail("Invalid \\uXXXX escape", end-5)
		}
		if 0xd800 <= u && u <= 0xdbff && end+6 < len(p.doc) && p.doc[end] == '\\' && p.doc[end+1] == 'u' {
			u2, ok := p.hex4(end + 2)
			if !ok {
				return nil, 0, p.fail("Invalid \\uXXXX escape", end+1)
			}
			if 0xdc00 <= u2 && u2 <= 0xdfff {
				u = 0x10000 + ((u - 0xd800) << 10) | (u2 - 0xdc00)
				end += 6
			}
		}
		out = append(out, u)
	}
}

// hex4 parses the four hex digits at i.
func (p *pyJSONParser) hex4(i int) (rune, bool) {
	var u rune
	for _, c := range p.doc[i : i+4] {
		var d rune
		switch {
		case '0' <= c && c <= '9':
			d = c - '0'
		case 'a' <= c && c <= 'f':
			d = c - 'a' + 10
		case 'A' <= c && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		u = u<<4 | d
	}
	return u, true
}

// parseObject parses the object whose first rune after "{" is at idx
// (_parse_object_unicode).
func (p *pyJSONParser) parseObject(idx, depth int) (pyValue, int, error) {
	v := pyValue{kind: pyDictValue}
	idx = p.skipSpace(idx)
	if p.at(idx) == '}' {
		return v, idx + 1, nil
	}
	for {
		if p.at(idx) != '"' {
			return pyValue{}, 0, p.fail("Expecting property name enclosed in double quotes", idx)
		}
		key, end, err := p.scanString(idx + 1)
		if err != nil {
			return pyValue{}, 0, err
		}
		idx = p.skipSpace(end)
		if p.at(idx) != ':' {
			return pyValue{}, 0, p.fail("Expecting ':' delimiter", idx)
		}
		idx = p.skipSpace(idx + 1)
		val, end, err := p.scanOnce(idx, depth)
		if err != nil {
			return pyValue{}, 0, err
		}
		v.set(key, val)
		idx = p.skipSpace(end)
		switch p.at(idx) {
		case '}':
			return v, idx + 1, nil
		case ',':
		default:
			return pyValue{}, 0, p.fail("Expecting ',' delimiter", idx)
		}
		comma := idx
		idx = p.skipSpace(idx + 1)
		if p.at(idx) == '}' {
			return pyValue{}, 0, p.fail("Illegal trailing comma before end of object", comma)
		}
	}
}

// set sets the dict key to val, as dict(pairs) does.
func (v *pyValue) set(key []rune, val pyValue) {
	for i, k := range v.keys {
		if slices.Equal(k, key) {
			v.vals[i] = val
			return
		}
	}
	v.keys = append(v.keys, key)
	v.vals = append(v.vals, val)
}

// parseArray parses the array whose first rune after "[" is at idx
// (_parse_array_unicode).
func (p *pyJSONParser) parseArray(idx, depth int) (pyValue, int, error) {
	v := pyValue{kind: pyListValue}
	idx = p.skipSpace(idx)
	if p.at(idx) == ']' {
		return v, idx + 1, nil
	}
	for {
		val, end, err := p.scanOnce(idx, depth)
		if err != nil {
			return pyValue{}, 0, err
		}
		v.list = append(v.list, val)
		idx = p.skipSpace(end)
		switch p.at(idx) {
		case ']':
			return v, idx + 1, nil
		case ',':
		default:
			return pyValue{}, 0, p.fail("Expecting ',' delimiter", idx)
		}
		comma := idx
		idx = p.skipSpace(idx + 1)
		if p.at(idx) == ']' {
			return pyValue{}, 0, p.fail("Illegal trailing comma before end of array", comma)
		}
	}
}

// pyCodec is a Python text codec that the mirror can decode.
type pyCodec int

const (
	pyUTF8 pyCodec = iota
	pyLatin1
	pyASCII
)

// pyCodecNames maps the normalized Python names and aliases of the
// supported codecs (encodings/aliases.py) to the codec.
var pyCodecNames = map[string]pyCodec{
	"utf_8": pyUTF8, "utf8": pyUTF8, "u8": pyUTF8, "utf": pyUTF8, "utf8_ucs2": pyUTF8, "utf8_ucs4": pyUTF8, "cp65001": pyUTF8,
	"latin_1": pyLatin1, "latin1": pyLatin1, "iso_8859_1": pyLatin1, "iso8859_1": pyLatin1, "8859": pyLatin1,
	"cp819": pyLatin1, "l1": pyLatin1, "latin": pyLatin1, "iso_ir_100": pyLatin1, "csisolatin1": pyLatin1,
	"ibm819": pyLatin1, "iso_8859_1_1987": pyLatin1,
	"ascii": pyASCII, "us_ascii": pyASCII, "646": pyASCII, "ansi_x3.4_1968": pyASCII, "ansi_x3_4_1968": pyASCII,
	"ansi_x3.4_1986": pyASCII, "cp367": pyASCII, "csascii": pyASCII, "ibm367": pyASCII, "iso646_us": pyASCII,
	"iso_646.irv_1991": pyASCII, "iso_ir_6": pyASCII, "us": pyASCII,
}

// pyCodecOf returns the codec of the charset parameter of a request.
// Django keeps a charset that codecs.lookup knows (request.py:103-114)
// and DRF falls back to utf-8 (request.py:190). The mirror decodes the
// charsets of pyCodecNames and reads every other name as utf-8.
func pyCodecOf(charset string) pyCodec {
	name := strings.ToLower(strings.TrimSpace(charset))
	name = strings.NewReplacer("-", "_", " ", "_").Replace(name)
	if c, ok := pyCodecNames[name]; ok {
		return c
	}
	return pyUTF8
}

// pyDecode decodes b as a codecs StreamReader.read() does: strict
// errors, and an incomplete UTF-8 sequence at the end is not an error
// and is not in the result (final=False).
func pyDecode(b []byte, codec pyCodec) ([]rune, error) {
	switch codec {
	case pyLatin1:
		return pyLatin1Decode(b), nil
	case pyASCII:
		out := make([]rune, 0, len(b))
		for i, c := range b {
			if c >= 0x80 {
				return nil, pyValueError(fmt.Sprintf("'ascii' codec can't decode byte 0x%02x in position %d: ordinal not in range(128)", c, i))
			}
			out = append(out, rune(c))
		}
		return out, nil
	case pyUTF8:
	}
	return pyUTF8Decode(b, false, false)
}

// pyDecodeReplace decodes b as bytes.decode(codec, "replace") does:
// each bad sequence, and an incomplete sequence at the end, becomes
// U+FFFD.
func pyDecodeReplace(b []byte, codec pyCodec) []rune {
	switch codec {
	case pyLatin1:
		return pyLatin1Decode(b)
	case pyASCII:
		out := make([]rune, 0, len(b))
		for _, c := range b {
			if c >= 0x80 {
				out = append(out, 0xfffd)
			} else {
				out = append(out, rune(c))
			}
		}
		return out
	case pyUTF8:
	}
	out, _ := pyUTF8Decode(b, true, true)
	return out
}

// pyDecodeStrict decodes b as bytes.decode(codec) does: every bad or
// incomplete sequence is an error.
func pyDecodeStrict(b []byte, codec pyCodec) ([]rune, error) {
	if codec == pyUTF8 {
		return pyUTF8Decode(b, true, false)
	}
	return pyDecode(b, codec)
}

// pyLatin1Decode decodes b as ISO-8859-1.
func pyLatin1Decode(b []byte) []rune {
	out := make([]rune, len(b))
	for i, c := range b {
		out[i] = rune(c)
	}
	return out
}

// pyUTF8Decode decodes b with the CPython utf-8 codec. An incomplete
// sequence at the end is left out when final is false. With replace,
// each bad sequence, and with final an incomplete end, becomes U+FFFD;
// else the first one is the UnicodeDecodeError text.
func pyUTF8Decode(b []byte, final, replace bool) ([]rune, error) {
	out := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			out = append(out, rune(c))
			i++
			continue
		}
		n, lo, hi := utf8SeqInfo(c)
		bad, reason := 0, ""
		if n == 0 {
			bad, reason = 1, "invalid start byte"
		}
		r := rune(c) & (0xff >> (n + 1))
		for k := 1; k < n && bad == 0; k++ {
			if i+k >= len(b) {
				if !final {
					return out, nil
				}
				bad, reason = k, "unexpected end of data"
				break
			}
			cc := b[i+k]
			if k == 1 && (cc < lo || cc > hi) || k > 1 && (cc < 0x80 || cc > 0xbf) {
				bad, reason = k, "invalid continuation byte"
				break
			}
			r = r<<6 | rune(cc&0x3f)
		}
		if bad == 0 {
			out = append(out, r)
			i += n
			continue
		}
		if !replace {
			if bad == 1 {
				return nil, pyValueError(fmt.Sprintf("'utf-8' codec can't decode byte 0x%02x in position %d: %s", c, i, reason))
			}
			return nil, pyValueError(fmt.Sprintf("'utf-8' codec can't decode bytes in position %d-%d: %s", i, i+bad-1, reason))
		}
		out = append(out, 0xfffd)
		i += bad
	}
	return out, nil
}

// utf8SeqInfo returns the length of the UTF-8 sequence that the start
// byte c begins (0 for a byte that cannot start one) and the range of
// its second byte.
func utf8SeqInfo(c byte) (n int, lo, hi byte) {
	switch {
	case 0xc2 <= c && c <= 0xdf:
		return 2, 0x80, 0xbf
	case c == 0xe0:
		return 3, 0xa0, 0xbf
	case c == 0xed:
		return 3, 0x80, 0x9f
	case 0xe1 <= c && c <= 0xef:
		return 3, 0x80, 0xbf
	case c == 0xf0:
		return 4, 0x90, 0xbf
	case c == 0xf4:
		return 4, 0x80, 0x8f
	case 0xf1 <= c && c <= 0xf3:
		return 4, 0x80, 0xbf
	}
	return 0, 0, 0
}

// pyB64Decode is base64.b64decode(s) of CPython 3.13 (validate=False):
// a str with a rune above ASCII fails, a rune outside the alphabet is
// skipped, "=" never ends the input, and the input fails when it ends
// one rune into a quantum, or two or three runes into a quantum with
// fewer "=" after the last data rune than the quantum lacks.
func pyB64Decode(s []rune) ([]byte, bool) {
	var out []byte
	var left byte
	quad, pads := 0, 0
	for _, r := range s {
		if r >= 0x80 {
			return nil, false
		}
		if r == '=' {
			pads++
			continue
		}
		d := b64Value[r]
		if d == 0xff {
			continue
		}
		pads = 0
		switch quad {
		case 0:
			left = d
		case 1:
			out = append(out, left<<2|d>>4)
			left = d & 0x0f
		case 2:
			out = append(out, left<<4|d>>2)
			left = d & 0x03
		case 3:
			out = append(out, left<<6|d)
		}
		quad = (quad + 1) % 4
	}
	if quad == 1 || quad > 1 && quad+pads < 4 {
		return nil, false
	}
	return out, true
}

// b64Value maps an ASCII rune to its value in the standard base64
// alphabet, or 0xff.
var b64Value = func() (t [128]byte) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for i := range t {
		t[i] = 0xff
	}
	for i := range byte(len(alphabet)) {
		t[alphabet[i]] = i
	}
	return t
}()
