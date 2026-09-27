package pdbcompat

import (
	"net/http"
	"net/netip"
	"net/textproto"
	"regexp"
	"strings"
)

// This file ports the Django 5.2 CSRF check that upstream PeeringDB
// 2.83.0 runs before a plain Django view such as /api/search
// (CsrfViewMiddleware.process_view, django/middleware/csrf.py:414-469).
// DRF views are exempt from it. Upstream sets CSRF_USE_SESSIONS
// (settings/__init__.py:1930), so the CSRF secret is in the session. The
// mirror has no sessions, so every request that the check does not
// accept fails, and only the reason differs.

// csrfSafeMethod reports whether the CSRF check accepts method without
// a token (csrf.py:425-427).
func csrfSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	}
	return false
}

// csrfNoSession is the reason for a request without a CSRF secret.
// Upstream view_http_error_csrf replaces the Django text "CSRF cookie
// not set." with it (views.py:348-359).
const csrfNoSession = "Your session expired or cookies are blocked; reload and retry."

// csrfRejectBody returns the 403 body of view_http_error_csrf, the
// CSRF_FAILURE_VIEW (settings/__init__.py:1717, views.py:348-359):
// JsonResponse({"non_field_errors": [reason]}), which escapes every
// character that is not printable ASCII.
func csrfRejectBody(r *http.Request) string {
	return `{"non_field_errors": [` + pyJSONASCIIString(latin1(csrfReason(r))) + `]}`
}

// csrfReason returns the reason of the CSRF check for a request with an
// unsafe method (csrf.py:436-469):
//
//  1. With an Origin header, the check fails unless the value is the
//     scheme and host of the request. Upstream also trusts its
//     BASE_URL, which is its own host (settings/__init__.py:1932).
//  2. Else, on a secure request, the Referer header must be an https
//     URL of the host of the request (csrfRefererReason).
//  3. Then the check needs the CSRF secret of the session, which the
//     mirror never has.
//
// A header value is read as WSGI reads it: ISO-8859-1, and the values
// of a repeated header joined with ",". Django reads the host from the
// Host header and the scheme from the proxy header (request.py:165-181,
// :307-323), as requestScheme does.
func csrfReason(r *http.Request) string {
	scheme, _, _ := strings.Cut(requestScheme(r), ",")
	scheme = strings.TrimSpace(scheme)
	if origin, ok := csrfHeader(r.Header, "Origin"); ok {
		if origin != scheme+"://"+r.Host {
			return "Origin checking failed - " + origin + " does not match any trusted origins."
		}
	} else if scheme == "https" {
		if reason := csrfRefererReason(r.Header, r.Host); reason != "" {
			return reason
		}
	}
	return csrfNoSession
}

// csrfHeader returns the values of the header name joined with ",",
// and whether the request has the header.
func csrfHeader(h http.Header, name string) (string, bool) {
	vals, ok := h[textproto.CanonicalMIMEHeaderKey(name)]
	return strings.Join(vals, ","), ok
}

// csrfRefererReason returns the reason of the Referer check
// (csrf.py:297-340), or "" when the Referer names host. Upstream
// compares the Referer host with SESSION_COOKIE_DOMAIN, its own host;
// the mirror uses the host of the request.
func csrfRefererReason(h http.Header, host string) string {
	referer, ok := csrfHeader(h, "Referer")
	if !ok {
		return "Referer checking failed - no Referer."
	}
	u, ok := pyURLSplit(referer)
	if !ok || u.scheme == "" || u.netloc == "" {
		return "Referer checking failed - Referer is malformed."
	}
	if u.scheme != "https" {
		return "Referer checking failed - Referer is insecure while host is secure."
	}
	if djangoSameDomain(u.netloc, host) {
		return ""
	}
	return "Referer checking failed - " + u.geturl() + " does not match any trusted origins."
}

// djangoSameDomain is Django is_same_domain (django/utils/http.py:
// 224-241): pattern, lower-cased, is host, or it starts with "." and
// host is that domain or a subdomain of it. host is not lower-cased.
func djangoSameDomain(host, pattern string) bool {
	if pattern == "" {
		return false
	}
	pattern = latin1Lower(pattern)
	return pattern[0] == '.' && (strings.HasSuffix(host, pattern) || host == pattern[1:]) || pattern == host
}

// pyURL holds the parts of a URL that Python urllib.parse.urlsplit
// returns.
type pyURL struct {
	scheme, netloc, path, query, fragment string
}

// pyURLSplit splits s as Python 3.13 urllib.parse.urlsplit does, with s
// read as ISO-8859-1. ok is false where urlsplit raises ValueError: a
// netloc with only one of "[" and "]", or a bracketed host that is not
// an IPv6 address or a valid IPvFuture literal. The NFKC check of a
// non-ASCII netloc never fails for an ISO-8859-1 character.
func pyURLSplit(s string) (u pyURL, ok bool) {
	// Remove leading C0 controls and spaces, and every tab, CR and LF.
	s = strings.TrimLeft(s, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f"+
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	s = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(s)
	if i := strings.IndexByte(s, ':'); i > 0 && isASCIILetter(s[0]) &&
		strings.Trim(s[:i], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-.") == "" {
		u.scheme, s = strings.ToLower(s[:i]), s[i+1:]
	}
	if strings.HasPrefix(s, "//") {
		end := len(s)
		if i := strings.IndexAny(s[2:], "/?#"); i >= 0 {
			end = i + 2
		}
		u.netloc, s = s[2:end], s[end:]
		open, closed := strings.Contains(u.netloc, "["), strings.Contains(u.netloc, "]")
		if open != closed {
			return u, false
		}
		if open {
			_, after, _ := strings.Cut(u.netloc, "[")
			bracketed, _, _ := strings.Cut(after, "]")
			if !pyBracketedHostOK(bracketed) {
				return u, false
			}
		}
	}
	s, u.fragment, _ = strings.Cut(s, "#")
	u.path, u.query, _ = strings.Cut(s, "?")
	return u, true
}

// ipvFuture is the IPvFuture pattern of urllib.parse
// _check_bracketed_host.
var ipvFuture = regexp.MustCompile(`\Av[a-fA-F0-9]+\..+\z`)

// pyBracketedHostOK reports whether urllib.parse _check_bracketed_host
// accepts host: an IPvFuture literal, or an address that Python
// ipaddress.ip_address parses as IPv6. A scope id must not be empty or
// hold "%".
func pyBracketedHostOK(host string) bool {
	if strings.HasPrefix(host, "v") {
		return ipvFuture.MatchString(host)
	}
	if _, zone, ok := strings.Cut(host, "%"); ok && (zone == "" || strings.Contains(zone, "%")) {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.Is6()
}

// geturl joins the parts as urlunsplit does for a URL with a scheme and
// a netloc: an empty query or fragment has no "?" or "#".
func (u pyURL) geturl() string {
	s := u.scheme + "://" + u.netloc + u.path
	if u.query != "" {
		s += "?" + u.query
	}
	if u.fragment != "" {
		s += "#" + u.fragment
	}
	return s
}

// isASCIILetter reports whether c is an ASCII letter.
func isASCIILetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// latin1Lower lower-cases s, read as ISO-8859-1, as Python str.lower
// does: A-Z and 0xc0-0xde except 0xd7.
func latin1Lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' || 0xc0 <= c && c <= 0xde && c != 0xd7 {
			b[i] = c + 0x20
		}
	}
	return string(b)
}
