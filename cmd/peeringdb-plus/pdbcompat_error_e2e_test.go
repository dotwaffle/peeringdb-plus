package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// TestE2E_PdbcompatErrorEnvelope sends /api/ errors through the full
// production middleware chain (buildE2EFixture). It proves that no outer
// middleware answers first, and that the chain keeps Content-Type, Allow
// and the Accept token of Vary. Gzip adds Accept-Encoding to Vary with
// Header().Add, so the check matches tokens, not substrings.
func TestE2E_PdbcompatErrorEnvelope(t *testing.T) {
	t.Parallel()
	fix := buildE2EFixture(t, privctx.TierPublic)

	for _, tc := range []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantAllow  string
		wantError  string
	}{
		{
			name:       "post_405",
			method:     http.MethodPost,
			path:       "/api/net",
			wantStatus: http.StatusMethodNotAllowed,
			wantAllow:  "GET, HEAD",
			wantError:  `Method "POST" not allowed.`,
		},
		{
			name:       "bad_limit_400",
			method:     http.MethodGet,
			path:       "/api/net?limit=abc",
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(t.Context(), tc.method, fix.server.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("%s %s: status = %d, want %d; body=%s", tc.method, tc.path, resp.StatusCode, tc.wantStatus, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			if tc.wantAllow != "" {
				if got := resp.Header.Get("Allow"); got != tc.wantAllow {
					t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
				}
			}
			if !varyHasToken(resp.Header, "Accept") {
				t.Errorf("Vary = %q, want the Accept token", resp.Header.Values("Vary"))
			}

			var env struct {
				Meta map[string]any  `json:"meta"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("decode body: %v; body=%s", err, body)
			}
			if env.Data != nil {
				t.Errorf("error body has a data key: %s", body)
			}
			msg, _ := env.Meta["error"].(string)
			if msg == "" {
				t.Errorf("meta.error is empty: %s", body)
			}
			if tc.wantError != "" && msg != tc.wantError {
				t.Errorf("meta.error = %q, want %q", msg, tc.wantError)
			}
		})
	}
}

// varyHasToken reports whether a comma-separated Vary value is token.
func varyHasToken(h http.Header, token string) bool {
	for _, v := range h.Values("Vary") {
		for part := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
