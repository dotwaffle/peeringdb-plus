package termrender

import (
	"strings"
	"unicode"
)

// sanitizeUpstream removes every control character from an upstream-sourced
// string before it is written into a terminal response.
//
// Mirrored PeeringDB records carry free-text fields (names, websites, notes,
// cities, addresses) supplied by arbitrary submitters. In ModeRich the renderer
// forces colorprofile.ANSI256, whose downsampler only rewrites SGR colour
// sequences; OSC, DCS, cursor, title, and clipboard escape sequences pass
// through verbatim. Emitting such bytes to a curl/wget client's terminal allows
// output spoofing and terminal hijack (OSC-8 hyperlinks, "\x1b]0;title\x07"
// window-title rewrites, OSC-52 clipboard writes, cursor manipulation).
//
// Stripping every rune for which unicode.IsControl reports true removes ESC
// (0x1B, the lead byte of all escape sequences), BEL (0x07), CR/LF, TAB, DEL
// (0x7F), and the C1 range (0x80-0x9F) — so no escape sequence can form. The
// renderer's own styling escapes are added AFTER sanitisation, so this is
// purely additive: callers sanitise the raw value, then apply styling.
//
// Terminal field values are single-line, so dropping newlines and tabs along
// with the other control bytes is the correct behaviour.
func sanitizeUpstream(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
