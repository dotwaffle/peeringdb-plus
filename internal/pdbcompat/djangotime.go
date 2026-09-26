package pdbcompat

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// errDjangoDateTime is the error of a value that Django
// DateTimeField.to_python rejects (a ValidationError upstream).
var errDjangoDateTime = errors.New("value has an invalid date/time format")

// djangoDateTime converts s as Django 5.2 DateTimeField.to_python does
// (django/db/models/fields/__init__.py), and then as upstream does
// (2.83.0 rest.py:647-653): a value without a time zone is UTC
// (settings TIME_ZONE "UTC"). The result is in UTC.
//
// to_python tries parse_datetime (django/utils/dateparse.py), which is
// datetime.fromisoformat and then datetime_re, and then parse_date,
// which is date.fromisoformat and then date_re. The fromisoformat steps
// follow the C datetime module of CPython 3.14 (the upstream image),
// including what it does not check. Upstream folds the value with
// unidecode first (rest.py:597), so a Unicode decimal digit reads as its
// ASCII digit.
func djangoDateTime(s string) (time.Time, error) {
	s = ndToASCII(s)
	if t, ok := pyFromISOFormat(s); ok {
		return t, nil
	}
	if t, ok, err := djangoDateTimeMatch(s); ok || err != nil {
		return t, err
	}
	// date.fromisoformat reads a date at the start of a string of 7, 8
	// or 10 bytes and does not check the rest.
	if n := len(s); n == 7 || n == 8 || n == 10 {
		if y, m, d, ok := parseISODate(s, n); ok {
			if t, err := pyDateTime(y, m, d, 0, 0, 0, 0, time.UTC); err == nil {
				return t, nil
			}
		}
	}
	if m := djangoDateRE.FindStringSubmatch(s); m != nil {
		return pyDateTime(atoi(m[1]), atoi(m[2]), atoi(m[3]), 0, 0, 0, 0, time.UTC)
	}
	return time.Time{}, errDjangoDateTime
}

// ndToASCII replaces each Unicode decimal digit of s with its ASCII
// digit, as unidecode does.
func ndToASCII(s string) string {
	return strings.Map(func(r rune) rune {
		if d := ndValue(r); d >= 0 {
			return rune(asciiDigits[d])
		}
		return r
	}, s)
}

// djangoDateTimeREPattern is datetime_re of django/utils/dateparse.py.
// Python \d reads ASCII digits here (ndToASCII ran first), Python \s
// is any Unicode white space, and Python $ also matches before a final
// newline.
const djangoDateTimeREPattern = `^(\d{4})-(\d{1,2})-(\d{1,2})[T ](\d{1,2}):(\d{1,2})` +
	`(?::(\d{1,2})(?:[.,](\d{1,6})\d{0,6})?)?` +
	`[\pZ\t\n\v\f\r\x1c-\x1f\x85]*(Z|[+-]\d{2}(?::?\d{2})?)?\n?\z`

var (
	djangoDateTimeRegexp = regexp.MustCompile(djangoDateTimeREPattern)
	// djangoDateRE is date_re of django/utils/dateparse.py.
	djangoDateRE = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})\n?\z`)
)

// djangoDateTimeMatch is the datetime_re branch of parse_datetime. ok
// reports a match. A match with a field out of range is an error, as
// the datetime constructor raises ValueError.
func djangoDateTimeMatch(s string) (time.Time, bool, error) {
	m := djangoDateTimeRegexp.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, false, nil
	}
	micro := 0
	if m[7] != "" {
		micro = atoi((m[7] + "00000")[:6])
	}
	loc := time.UTC
	if tz := m[8]; tz != "" && tz != "Z" {
		mins := 0
		if len(tz) > 3 {
			mins = atoi(tz[len(tz)-2:])
		}
		offset := 60*atoi(tz[1:3]) + mins
		if tz[0] == '-' {
			offset = -offset
		}
		// get_fixed_timezone builds a Python timezone, which takes an
		// offset under 24 hours only.
		if offset <= -24*60 || offset >= 24*60 {
			return time.Time{}, true, errDjangoDateTime
		}
		loc = time.FixedZone("", offset*60)
	}
	t, err := pyDateTime(atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4]), atoi(m[5]), atoi(m[6]), micro, loc)
	return t, true, err
}

// pyFromISOFormat is datetime.fromisoformat. ok is false for a string
// that it rejects.
func pyFromISOFormat(s string) (time.Time, bool) {
	if len(s) < 7 {
		return time.Time{}, false
	}
	sep, ok := isoDateTimeSeparator(s)
	if !ok {
		return time.Time{}, false
	}
	y, mo, d, ok := parseISODate(s, sep)
	if !ok {
		return time.Time{}, false
	}
	var clock isoClock
	var offset time.Duration
	nextDay := false
	if len(s) > sep {
		// The separator is one character: the C code steps over it by
		// the length of its UTF-8 lead byte.
		p := sep + 1
		switch b := s[sep]; {
		case b&0x80 == 0:
		case b&0xf0 == 0xe0:
			p = sep + 3
		case b&0xf0 == 0xf0:
			p = sep + 4
		default:
			p = sep + 2
		}
		if clock, offset, nextDay, ok = parseISOTime(s[min(p, len(s)):]); !ok {
			return time.Time{}, false
		}
	}
	t, err := pyDateTime(y, mo, d, clock.hour, clock.minute, clock.second, clock.micro, time.UTC)
	if nextDay {
		t = t.Add(24 * time.Hour)
	}
	return t.Add(-offset), err == nil
}

// isoDateTimeSeparator is _find_isoformat_datetime_separator: the index
// where the date ends.
func isoDateTimeSeparator(s string) (int, bool) {
	n := len(s)
	if n == 7 {
		return 7, true
	}
	if s[4] == '-' {
		if s[5] != 'W' {
			return 10, true // YYYY-MM-DD
		}
		if n > 8 && s[8] == '-' {
			if n == 9 {
				return 0, false
			}
			if n > 10 && isASCIIDigit(s[10]) {
				return 8, true
			}
			return 10, true // YYYY-Www-D
		}
		return 8, true // YYYY-Www
	}
	if s[4] == 'W' {
		idx := 7
		for idx < n && isASCIIDigit(s[idx]) {
			idx++
		}
		if idx < 9 {
			return idx, true
		}
		if idx%2 == 0 {
			return 7, true // YYYYWwwd
		}
		return 8, true
	}
	return 8, true // YYYYMMDD
}

// parseISODate is parse_isoformat_date: YYYY-MM-DD, YYYYMMDD, YYYY-Www,
// YYYYWww, YYYY-Www-D and YYYYWwwD at the start of s. n is the length of
// the date part, which only tells whether a week date has a day. The C
// code does not check what follows the date. A field out of range is
// left to pyDateTime.
func parseISODate(s string, n int) (y, m, d int, ok bool) {
	if y, ok = digitsAt(s, 0, 4); !ok {
		return 0, 0, 0, false
	}
	p := 4
	hasSep := byteAt(s, p) == '-'
	if hasSep {
		p++
	}
	if byteAt(s, p) == 'W' {
		p++
		week, wok := digitsAt(s, p, 2)
		if !wok {
			return 0, 0, 0, false
		}
		p += 2
		day := 1
		if p < n {
			if hasSep {
				if byteAt(s, p) != '-' {
					return 0, 0, 0, false
				}
				p++
			}
			if day, ok = digitsAt(s, p, 1); !ok {
				return 0, 0, 0, false
			}
		}
		return isoWeekToGregorian(y, week, day)
	}
	if m, ok = digitsAt(s, p, 2); !ok {
		return 0, 0, 0, false
	}
	p += 2
	if hasSep {
		if byteAt(s, p) != '-' {
			return 0, 0, 0, false
		}
		p++
	}
	if d, ok = digitsAt(s, p, 2); !ok {
		return 0, 0, 0, false
	}
	return y, m, d, true
}

// isoWeekToGregorian is _isoweek_to_gregorian: the date of day (1-7) of
// ISO week of year.
func isoWeekToGregorian(year, week, day int) (y, m, d int, ok bool) {
	if year < 1 || year > 9999 || day < 1 || day > 7 {
		return 0, 0, 0, false
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	if week < 1 || week > 53 {
		return 0, 0, 0, false
	}
	if week == 53 {
		// Only years that start on a Thursday, and leap years that start
		// on a Wednesday, have 53 ISO weeks.
		first := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC).Weekday()
		leap := time.Date(year, time.February, 29, 0, 0, 0, 0, time.UTC).Month() == time.February
		if first != time.Thursday && (first != time.Wednesday || !leap) {
			return 0, 0, 0, false
		}
	}
	// Monday of week 1 is the Monday on or before January 4.
	offset := (int(jan4.Weekday()) + 6) % 7
	t := jan4.AddDate(0, 0, -offset+(week-1)*7+day-1)
	return t.Year(), int(t.Month()), t.Day(), true
}

// isoClock holds the time fields of an ISO time.
type isoClock struct {
	hour, minute, second, micro int
}

// parseISOTime is parse_isoformat_time: HH[:MM[:SS[.f+]]] with an
// optional zone Z, +HH[:MM[:SS[.f+]]] or -HH[...]. offset is the zone
// offset east of UTC, which can have microseconds in Python. nextDay
// reports the hour 24:00:00, which is midnight of the next day.
func parseISOTime(s string) (c isoClock, offset time.Duration, nextDay, ok bool) {
	tzPos := strings.IndexAny(s, "+-Z")
	end := len(s)
	if tzPos >= 0 {
		end = tzPos
	}
	c, more, ok := parseHHMMSSFF(s, end)
	if !ok {
		return isoClock{}, 0, false, false
	}
	if c.hour == 24 {
		if c.minute != 0 || c.second != 0 || c.micro != 0 {
			return isoClock{}, 0, false, false
		}
		c.hour, nextDay = 0, true
	}
	switch {
	case tzPos < 0:
		return c, 0, nextDay, !more
	case s[tzPos] == 'Z':
		return c, 0, nextDay, tzPos == len(s)-1
	}
	tz := s[tzPos+1:]
	off, more, ok := parseHHMMSSFF(tz, len(tz))
	if !ok || more {
		return isoClock{}, 0, false, false
	}
	// Python timezone() takes any offset under 24 hours, so +05:99 is
	// valid.
	offset = time.Duration(off.hour)*time.Hour + time.Duration(off.minute)*time.Minute +
		time.Duration(off.second)*time.Second + time.Duration(off.micro)*time.Microsecond
	if offset >= 24*time.Hour {
		return isoClock{}, 0, false, false
	}
	if s[tzPos] == '-' {
		offset = -offset
	}
	return c, offset, nextDay, true
}

// parseHHMMSSFF is parse_hh_mm_ss_ff over s[:end]: HH[:?MM[:?SS]] and a
// fraction. In the basic format (no colons) the fraction needs no
// decimal mark. As in the C code, the byte at end is read (it is the
// zone sign or the end of s), and more reports a byte that the caller
// must read after the fields. A field out of range is left to
// pyDateTime.
func parseHHMMSSFF(s string, end int) (c isoClock, more, ok bool) {
	vals := [3]*int{&c.hour, &c.minute, &c.second}
	p := 0
	hasSep := false
	for i := range 3 {
		v, dok := digitsAt(s, p, 2)
		if !dok {
			return isoClock{}, false, false
		}
		*vals[i] = v
		p += 2
		ch := byteAt(s, p)
		p++
		if i == 0 {
			hasSep = ch == ':'
		}
		if ch == '.' || ch == ',' {
			// CPython 3.14: a decimal mark only after the seconds, and
			// with at least one digit.
			if i < 2 || p >= end {
				return isoClock{}, false, false
			}
			break
		}
		if p >= end {
			return c, ch != 0, true
		}
		if hasSep && ch == ':' && i < 2 {
			continue
		}
		if hasSep {
			return isoClock{}, false, false
		}
		p--
	}
	n := min(end-p, 6)
	micro, dok := digitsAt(s, p, n)
	if !dok {
		return isoClock{}, false, false
	}
	for range 6 - n {
		micro *= 10
	}
	c.micro = micro
	p += n
	for isASCIIDigit(byteAt(s, p)) {
		p++
	}
	return c, p < len(s), true
}

// pyDateTime builds a time as the Python datetime constructor does: a
// field out of range is an error, not a carry.
func pyDateTime(y, mo, d, h, mi, sec, micro int, loc *time.Location) (time.Time, error) {
	if y < 1 || y > 9999 || mo < 1 || mo > 12 || d < 1 || h > 23 || mi > 59 || sec > 59 {
		return time.Time{}, errDjangoDateTime
	}
	t := time.Date(y, time.Month(mo), d, h, mi, sec, micro*1000, loc)
	if t.Day() != d {
		return time.Time{}, errDjangoDateTime
	}
	return t.UTC(), nil
}

// digitsAt reads n ASCII digits of s at pos.
func digitsAt(s string, pos, n int) (int, bool) {
	if pos < 0 || pos+n > len(s) {
		return 0, false
	}
	v := 0
	for i := pos; i < pos+n; i++ {
		if !isASCIIDigit(s[i]) {
			return 0, false
		}
		v = v*10 + int(s[i]-'0')
	}
	return v, true
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// byteAt returns s[i], or 0 past the end, where the C code reads the
// string terminator.
func byteAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// atoi converts a string of ASCII digits that a regexp matched.
func atoi(s string) int {
	v, _ := digitsAt(s, 0, len(s))
	return v
}
