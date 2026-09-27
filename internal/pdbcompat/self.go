package pdbcompat

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the /api/<tag>/self route of upstream PeeringDB 2.83.0
// (view_self_entity, rest.py:1600-1646, route rest.py:2088).

// selfRoute is the upstream route pattern of view_self_entity. It has no
// "^" and no "$", so Django matches it with re.search (django
// urls/resolvers.py RegexPattern.match): every path under /api/ that
// holds "<tag>/self" anywhere reaches the view, for example
// /api/net/self/, /api/net/selfie and /api/ixfac/self (tag fac). Go
// regexp picks the leftmost match and the first alternative there, as
// Python re does. The route comes before the router routes
// (rest.py:2120-2122).
var selfRoute = regexp.MustCompile(`(net|ix|org|fac|carrier|campus)/self`)

// defaultSelfIDs holds the DEFAULT_SELF_<TAG> settings (2.83.0
// settings/__init__.py:1689-1694): the object that the view sends an
// anonymous caller to.
var defaultSelfIDs = map[string]int{
	peeringdb.TypeOrg:     25554,
	peeringdb.TypeNet:     666,
	peeringdb.TypeIX:      4095,
	peeringdb.TypeFac:     13346,
	peeringdb.TypeCarrier: 66,
	peeringdb.TypeCampus:  25,
}

// selfTag returns the tag of the self route that rest, the path after
// /api/, matches.
func selfTag(rest string) (string, bool) {
	m := selfRoute.FindStringSubmatch(rest)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// serveSelf answers a request that the self route matches. The view is
// an api_view that maps GET only (DRF decorators.py:46-47), so DRF
// runs content negotiation first, then answers every other method,
// HEAD included, with 405 (views.py:513-521). The format suffix is not
// part of the route, so only ?format= selects a format. The mirror
// answers OPTIONS with 405 too (docs/API.md § Known Divergences).
//
// GET redirects with a 302 to the default object of the tag and keeps
// the query string, as upstream does for an anonymous caller. Upstream
// sends a caller with an API key to an object of the caller's own
// organization; the mirror has no user data (docs/API.md § Known
// Divergences). The redirect needs no database read: upstream loads
// the object by id with no status filter only to read the same id.
func serveSelf(w http.ResponseWriter, r *http.Request, tag string) {
	if !negotiate(w, r, "") {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r, "GET")
		return
	}
	target := "/api/" + tag + "/" + strconv.Itoa(defaultSelfIDs[tag])
	writeDjangoRedirect(w, target, r.URL.RawQuery, http.StatusFound)
}

// writeDjangoRedirect writes the redirect of a Django
// HttpResponseRedirect (302) or HttpResponsePermanentRedirect (301):
// Location is path, with "?" and the raw query string when there is
// one, quoted as iri_to_uri quotes it (django/http/response.py:633-638),
// and the body is empty.
func writeDjangoRedirect(w http.ResponseWriter, path, rawQuery string, status int) {
	if rawQuery != "" {
		path += "?" + iriToURI(rawQuery)
	}
	w.Header().Set("Location", path)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
}

// iriSafe holds the ASCII bytes that Django iri_to_uri leaves as they
// are: the unreserved characters of urllib.parse.quote and the safe
// argument "/#%[]=:;$&()+,!?*@'~" (django/utils/encoding.py:107-135).
const iriSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.-~/#%[]=:;$&()+,!?*@'"

// iriToURI quotes a raw query string as Django iri_to_uri quotes the
// WSGI QUERY_STRING. WSGI decodes the raw bytes as ISO-8859-1 (PEP
// 3333), and quote encodes each such character as UTF-8, so a byte of
// 0x80 or more becomes two escapes.
func iriToURI(s string) string {
	var b strings.Builder
	for i := range len(s) {
		c := s[i]
		switch {
		case c < 0x80 && strings.IndexByte(iriSafe, c) >= 0:
			b.WriteByte(c)
		case c < 0x80:
			writePctByte(&b, c)
		default:
			writePctByte(&b, 0xc0|c>>6)
			writePctByte(&b, 0x80|c&0x3f)
		}
	}
	return b.String()
}

// writePctByte writes c as a percent escape with upper-case hex digits,
// as urllib.parse.quote does.
func writePctByte(b *strings.Builder, c byte) {
	const hex = "0123456789ABCDEF"
	b.WriteByte('%')
	b.WriteByte(hex[c>>4])
	b.WriteByte(hex[c&0x0f])
}
