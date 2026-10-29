package parity

import (
	"net/http"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestReadOnlyOptions(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, testutil.SetupClient(t))
	for _, tc := range []struct{ path, name string }{
		{"/api/", "API Root"},
		{"/api/org", "Organization List"},
		{"/api/net", "Network List"},
		{"/api/net.json", "Network List"},
		{"/api/net/999", "Network Instance"},
		{"/api/net/999?depth=invalid&asn__lt=bad", "Network Instance"},
		{"/api/net/999.json", "Network Instance"},
		{"/api/poc/999", "Network Contact Instance"},
		{"/api/ix", "Internet Exchange List"},
		{"/api/ixpfx", "IX Lan Prefix List"},
		{"/api/netixlan", "Network IX Lan List"},
		{"/api/netfac", "Network Facility List"},
		{"/api/ixfac", "Internet Exchange Facility List"},
		{"/api/carrier", "Carrier List"},
		{"/api/carrierfac", "Carrier Facility List"},
		{"/api/campus", "Campus List"},
		{"/api/fac", "Facility List"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			code, hdr, body := httpDo(t, srv, http.MethodOptions, tc.path, nil)
			if code != 200 || hdr.Get("Allow") != "GET, HEAD, OPTIONS" {
				t.Fatalf("status %d, Allow %q, body %s", code, hdr.Get("Allow"), body)
			}
			checkDRFMetadata(t, tc.path, body, tc.name, "", nil)
		})
	}
	for _, tc := range []struct {
		method, path string
		code         int
		allow        string
	}{
		{"GET", "/api/net", 200, "GET, HEAD, OPTIONS"},
		{"HEAD", "/api/net", 200, "GET, HEAD, OPTIONS"},
		{"GET", "/api/net?asn__lt=bad", 400, "GET, HEAD, OPTIONS"},
		{"GET", "/api/net/999", 404, "GET, HEAD, OPTIONS"},
		{"POST", "/api/net", 405, "GET, HEAD, OPTIONS"},
		{"DELETE", "/api/net/999", 405, "GET, HEAD, OPTIONS"},
		{"OPTIONS", "/api/net/", 404, ""},
		{"OPTIONS", "/api/nope", 404, ""},
		{"OPTIONS", "/api/ixlan", 405, "GET"},
		{"OPTIONS", "/api/as_set", 405, "GET"},
	} {
		code, hdr, _ := httpDo(t, srv, tc.method, tc.path, nil)
		if code != tc.code || hdr.Get("Allow") != tc.allow {
			t.Errorf("%s %s: status %d, Allow %q; want %d, %q", tc.method, tc.path, code, hdr.Get("Allow"), tc.code, tc.allow)
		}
	}
	code, hdr, _ := httpDo(t, srv, "OPTIONS", "/api/net", http.Header{"Accept": {"text/html"}})
	if code != 406 || hdr.Get("Allow") != "GET, HEAD, OPTIONS" {
		t.Errorf("negotiation: status %d, Allow %q", code, hdr.Get("Allow"))
	}
}
