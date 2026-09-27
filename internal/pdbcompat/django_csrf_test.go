package pdbcompat

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFRejectBody(t *testing.T) {
	t.Parallel()
	// Expected values from Django 5.2.17 CsrfViewMiddleware with
	// CSRF_USE_SESSIONS, no session, SESSION_COOKIE_DOMAIN unset (so the
	// Referer must name the request host) and the upstream
	// view_http_error_csrf (2.83.0 views.py:348-359). https sets the
	// proxy header; the header values are ISO-8859-1 bytes.
	cases := []struct {
		method, host string
		https        bool
		hdr          map[string]string
		reason       string
	}{
		{"POST", "mirror.example", true, nil, `"Referer checking failed - no Referer."`},
		{"POST", "mirror.example", false, nil, `"Your session expired or cookies are blocked; reload and retry."`},
		{"PUT", "mirror.example", true, nil, `"Referer checking failed - no Referer."`},
		{"DELETE", "mirror.example", true, nil, `"Referer checking failed - no Referer."`},
		{"PATCH", "mirror.example", true, nil, `"Referer checking failed - no Referer."`},
		{"PROPFIND", "mirror.example", true, nil, `"Referer checking failed - no Referer."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "https://mirror.example"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "https://Mirror.example"}, `"Origin checking failed - https://Mirror.example does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "http://mirror.example"}, `"Origin checking failed - http://mirror.example does not match any trusted origins."`},
		{"POST", "mirror.example", false, map[string]string{"Origin": "http://mirror.example"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": ""}, `"Origin checking failed -  does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "https://evil.example\xe9\""}, `"Origin checking failed - https://evil.example\u00e9\" does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "null"}, `"Origin checking failed - null does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://mirror.example/x"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://MIRROR.example/x"}, `"Referer checking failed - https://MIRROR.example/x does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://mirror.example:443/x"}, `"Referer checking failed - https://mirror.example:443/x does not match any trusted origins."`},
		{"POST", "mirror.example:8443", true, map[string]string{"Referer": "https://mirror.example:8443/x"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "MIRROR.EXAMPLE", true, map[string]string{"Referer": "https://Mirror.example/x"}, `"Referer checking failed - https://Mirror.example/x does not match any trusted origins."`},
		{"POST", "MIRROR.EXAMPLE", true, map[string]string{"Referer": "https://mirror.example/x"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "http://mirror.example/x"}, `"Referer checking failed - Referer is insecure while host is secure."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "mirror.example/x"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": ""}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https:/x"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[::1/"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[1.2.3.4]/"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[v1.x]/"}, `"Referer checking failed - https://[v1.x]/ does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[vz]/"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[fe80::1%eth0]/p"}, `"Referer checking failed - https://[fe80::1%eth0]/p does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://[fe80::1%]/p"}, `"Referer checking failed - Referer is malformed."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "HTTPS://evil.example/p?q=1#f"}, `"Referer checking failed - https://evil.example/p?q=1#f does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://evil.example/p?#"}, `"Referer checking failed - https://evil.example/p does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://evil.example?a"}, `"Referer checking failed - https://evil.example?a does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://evil.example#a?b"}, `"Referer checking failed - https://evil.example#a?b does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://u@mirror.example/"}, `"Referer checking failed - https://u@mirror.example/ does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://ev\xe9il.example/\xff\x01"}, `"Referer checking failed - https://ev\u00e9il.example/\u00ff\u0001 does not match any trusted origins."`},
		{"POST", "mirror.example", false, map[string]string{"Referer": "https://evil.example/"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "mirror.example", true, map[string]string{"Referer": "https://.mirror.example/"}, `"Referer checking failed - https://.mirror.example/ does not match any trusted origins."`},
		{"POST", ".mirror.example", true, map[string]string{"Referer": "https://a.mirror.example/"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", ".mirror.example", true, map[string]string{"Referer": "https://mirror.example/"}, `"Your session expired or cookies are blocked; reload and retry."`},
		{"POST", "", true, map[string]string{"Referer": "https://x/"}, `"Referer checking failed - https://x/ does not match any trusted origins."`},
		{"POST", "mirror.example", true, map[string]string{"Origin": "https://mirror.example", "Referer": "http://evil/"}, `"Your session expired or cookies are blocked; reload and retry."`},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, "/api/search", nil)
		r.Host = tc.host
		if tc.https {
			r.Header.Set("X-Forwarded-Proto", "https")
		}
		for k, v := range tc.hdr {
			r.Header.Set(k, v)
		}
		want := `{"non_field_errors": [` + tc.reason + `]}`
		if csrfSafeMethod(tc.method) {
			t.Errorf("%s: csrfSafeMethod = true", tc.method)
		}
		if got := csrfRejectBody(r); got != want {
			t.Errorf("%s host %q https %v %q:\n got  %s\n want %s", tc.method, tc.host, tc.https, tc.hdr, got, want)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		if !csrfSafeMethod(m) {
			t.Errorf("csrfSafeMethod(%s) = false", m)
		}
	}
}

func TestPyJSONASCIIString(t *testing.T) {
	t.Parallel()
	// Expected values from json.dumps(s) (ensure_ascii=True).
	cases := map[string]string{
		"a\"b\\c":        `"a\"b\\c"`,
		"\n\x01\x7f":     `"\n\u0001\u007f"`,
		"\u00e9 \u00ff~": `"\u00e9 \u00ff~"`,
		"\u65e5":         `"\u65e5"`,
		"\U0001F600":     `"\ud83d\ude00"`,
	}
	for in, want := range cases {
		if got := pyJSONASCIIString(in); got != want {
			t.Errorf("pyJSONASCIIString(%q) = %s, want %s", in, got, want)
		}
	}
}
